package cli

import (
	"fmt"
	"strings"

	kestra "github.com/kestra-io/client-sdk/go-sdk/v2/kestra_api_client"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newFlowsValidateByQueryCommand() *cobra.Command {
	var filterFlags byQueryFilterFlags
	var batchSize int

	cmd := &cobra.Command{
		Use:          "validate-by-query",
		Short:        "Validate the flows already stored on the instance.",
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		Long: `Validate the flow sources already stored on the instance, without exporting
them to disk first. This is the post-migration check: 'flows validate <path>'
answers "are my local files valid?", this answers "is what is on the instance
valid?".

Without a selection flag every flow of the tenant is validated. --namespace,
--flow and --filter narrow it down, exactly as they do for 'flows export-by-query'.

Two failure classes are reported:

  - a flow the server can read but that has constraint violations
  - a flow whose stored source the server cannot deserialize at all (e.g. a task
    type removed in 2.0), including one the export could not produce and that a
    plain export-then-validate round-trip therefore never sees

Validation fails if any flow has constraint violations or cannot be deserialized.
Warnings, infos, deprecations and outdated flags are reported but do not fail.

Drafts are not covered: the export the sources are read from deliberately skips
them, so a draft-headed flow is validated at its last saved revision and a
draft-only flow is not validated at all.`,
		Example: `  # Validate every flow stored on the instance
	  kestractl flows validate-by-query

	  # Scope to one namespace
	  kestractl flows validate-by-query --namespace company.team

	  # Gate a migration in CI (exits non-zero when anything fails)
	  kestractl flows validate-by-query --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutputFormat(); err != nil {
				return err
			}
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			renderer.WithErrWriter(cmd.ErrOrStderr())

			filters, err := filterFlags.resolveOptional()
			if err != nil {
				return err
			}

			client, err := newClientFunc()
			if err != nil {
				return err
			}

			return runFlowsValidateByQuery(client, filters, batchSize, renderer)
		},
	}

	cmd.Flags().IntVar(&batchSize, "batch-size", validateBatchSize,
		"Number of flow sources sent per validation request")
	addByQueryFilterFlags(cmd, &filterFlags)

	return cmd
}

func runFlowsValidateByQuery(client *Client, filters []kestra.QueryFilter, batchSize int, renderer *Renderer) error {
	archive, err := client.Kestra.Flows().ExportFlowsByQuery(client.Ctx, client.Tenant, queryFiltersToSearchFilters(filters))
	if err != nil {
		return formatSDKError(err)
	}

	entries, skipped, err := flowSourcesFromZip(archive)
	if err != nil {
		return err
	}
	if skipped > 0 {
		fmt.Fprintf(renderer.ErrWriter(),
			"warning: %d archive entry/entries were skipped (unreadable or larger than %d MiB)\n",
			skipped, maxZipEntrySize>>20)
	}

	sources := make([]string, 0, len(entries))
	seeds := make([]ValidateResult, 0, len(entries))
	for _, entry := range entries {
		namespace, flowID := flowIdentityFromSource(entry.Source)
		sources = append(sources, entry.Source)
		seeds = append(seeds, ValidateResult{
			FilePath:  flowLabel(namespace, flowID, entry.Name),
			Namespace: namespace,
			FlowID:    flowID,
			Success:   true,
		})
	}

	violations, err := validateSourcesInBatches(client, sources, batchSize)
	if err != nil {
		return err
	}

	results, failed := buildValidateResults(violations, seeds)

	// The undeserializable flows are appended after the mapping, because their
	// positions must not shift the violation indices above.
	broken, err := undeserializableFlows(client, filters, results)
	if err != nil {
		fmt.Fprintf(renderer.ErrWriter(),
			"warning: the undeserializable-flow check was skipped (%s); only the exported sources were validated\n", err)
	}
	results = append(results, broken...)
	failed += len(broken)

	if len(results) == 0 {
		fmt.Fprintln(renderer.ErrWriter(), "no flows matched the query")
	}

	return renderValidateResults(results, failed, "FLOW", renderer)
}

// undeserializableFlows returns a failing result for every flow the server
// listed but could not deserialize, skipping the ones already covered by
// validated is the export-derived result set.
//
// Most such flows are in the export too: it writes the stored source text
// back out without deserializing it, so the validate endpoint sees them and
// reports the same breakage in its own words. Reporting both would count one
// broken flow twice. What this catches is the narrower case of a flow the
// inventory knows about but the export did not produce.
func undeserializableFlows(client *Client, filters []kestra.QueryFilter, validated []ValidateResult) ([]ValidateResult, error) {
	inventory, err := listAllFlowsForTenantFiltered(client, client.Tenant, filters)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool, len(validated))
	for _, result := range validated {
		if result.Namespace != "" && result.FlowID != "" {
			seen[result.Namespace+"/"+result.FlowID] = true
		}
	}

	results := make([]ValidateResult, 0)
	for _, flow := range inventory {
		if flow.Exception == "" || seen[flow.Namespace+"/"+flow.ID] {
			continue
		}
		results = append(results, ValidateResult{
			FilePath:  flowLabel(flow.Namespace, flow.ID, flow.ID),
			Namespace: flow.Namespace,
			FlowID:    flow.ID,
			Constraints: []string{
				"the stored flow source cannot be deserialized by the server: " +
					strings.ReplaceAll(flow.Exception, "\n", "; "),
			},
			Success: false,
		})
	}
	return results, nil
}

// flowLabel names a row in the result table. A stored flow has no path, so it
// is identified as "<namespace>/<flow-id>" — not by its archive entry name,
// which is a flattened "<namespace>-<flow-id>.yml" and reads worse. fallback
// covers a source whose identity could not be read.
func flowLabel(namespace, flowID, fallback string) string {
	if namespace == "" || flowID == "" {
		return fallback
	}
	return namespace + "/" + flowID
}

// flowIdentityFromSource reads the namespace and id out of a flow source.
//
// The archive entry name is not usable for this: a Kestra export flattens the
// archive to "<namespace>-<flow-id>.yml", and since both halves may contain a
// hyphen, that name cannot be split back apart unambiguously. The source is
// authoritative anyway.
//
// This is only a seed. The validate endpoint returns the identity of every
// source it was given, valid ones included, and that overwrites what is set
// here; this is what a row falls back to if the server ever omits one.
func flowIdentityFromSource(source string) (namespace, flowID string) {
	var root struct {
		ID        string `yaml:"id"`
		Namespace string `yaml:"namespace"`
	}
	// A source that does not parse is exactly what this command reports on, so
	// a failure here is expected and leaves the identity to the server.
	if err := yaml.Unmarshal([]byte(source), &root); err != nil {
		return "", ""
	}
	return root.Namespace, root.ID
}
