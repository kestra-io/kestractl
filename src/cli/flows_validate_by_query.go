package cli

import (
	"fmt"
	"path"
	"strings"

	kestra "github.com/kestra-io/client-sdk/go-sdk/v2/kestra_api_client"
	"github.com/spf13/cobra"
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
    type removed in 2.0). Such a flow is absent from an export, so a plain
    export-then-validate round-trip reports it as valid

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
		namespace, flowID := flowIdentityFromZipEntry(entry.Name)
		sources = append(sources, entry.Source)
		seeds = append(seeds, ValidateResult{
			FilePath:  entry.Name,
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
	broken, err := undeserializableFlows(client, filters)
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
// listed but could not deserialize. These never reach the validate endpoint —
// they are missing from the export, and there is no source to send.
func undeserializableFlows(client *Client, filters []kestra.QueryFilter) ([]ValidateResult, error) {
	inventory, err := listAllFlowsForTenantFiltered(client, client.Tenant, filters)
	if err != nil {
		return nil, err
	}

	results := make([]ValidateResult, 0)
	for _, flow := range inventory {
		if flow.Exception == "" {
			continue
		}
		results = append(results, ValidateResult{
			FilePath:  flow.Namespace + "/" + flow.ID,
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

// flowIdentityFromZipEntry splits a flow export entry name into its namespace
// and flow id. Kestra names them "<namespace>/<flow-id>.yaml"; anything that
// does not look like that yields an empty namespace, and the renderer falls
// back to the entry name.
func flowIdentityFromZipEntry(name string) (namespace, flowID string) {
	trimmed := strings.TrimPrefix(path.Clean(name), "/")
	dir, file := path.Split(trimmed)

	flowID = strings.TrimSuffix(file, path.Ext(file))
	namespace = strings.ReplaceAll(strings.Trim(dir, "/"), "/", ".")

	return namespace, flowID
}
