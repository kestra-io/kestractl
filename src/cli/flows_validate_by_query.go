package cli

import (
	"fmt"

	kestra "github.com/kestra-io/client-sdk/go-sdk/v2/kestra_api_client"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newFlowsValidateByQueryCommand() *cobra.Command {
	var filterFlags byQueryFilterFlags
	var batchSize int
	var all bool

	cmd := &cobra.Command{
		Use:          "validate-by-query",
		Short:        "Validate the flows already stored on the instance.",
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		Long: `Validate the flow sources already stored on the instance, without exporting
them to disk first. This is the post-migration check: 'flows validate <path>'
answers "are my local files valid?", this answers "is what is on the instance
valid?".

A selection is required. --namespace, --flow and --filter narrow it down,
exactly as they do for 'flows export-by-query'; --all validates every flow of
the tenant and cannot be combined with the other three. Validating a whole
instance is deliberately something you have to ask for by name.

This catches both a flow with constraint violations and one whose stored source
the server can no longer deserialize at all (e.g. a task type removed in 2.0):
the export copies the stored source text without parsing it, so a flow that no
longer loads is still sent to the validate endpoint and reported.

Validation fails if any flow has constraint violations. Warnings, infos,
deprecations and outdated flags are reported but do not fail.

Drafts are not covered: the export the sources are read from deliberately skips
them, so a draft-headed flow is validated at its last saved revision and a
draft-only flow is not validated at all.`,
		Example: `  # Scope to one namespace
	  kestractl flows validate-by-query --namespace company.team

	  # Every flow stored on the instance
	  kestractl flows validate-by-query --all

	  # Gate a migration in CI (exits non-zero when anything fails)
	  kestractl flows validate-by-query --all --output json`,
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

			// A missing selection is a usage mistake, not a failed run, so it
			// is the one error here that prints the help. SilenceUsage stays on
			// for everything else: a flow that fails validation must not bury
			// the report under the flag list.
			switch {
			case all && len(filters) > 0:
				cmd.SilenceUsage = false
				return fmt.Errorf("--all validates every flow of the tenant and cannot be combined with --namespace, --flow or --filter")
			case !all && len(filters) == 0:
				cmd.SilenceUsage = false
				return fmt.Errorf("a selection is required: use --namespace, --flow or --filter to scope the run, or --all to validate every flow of the tenant")
			case batchSize < 1:
				cmd.SilenceUsage = false
				return fmt.Errorf("--batch-size must be at least 1, got %d", batchSize)
			}

			client, err := newClientFunc()
			if err != nil {
				return err
			}

			return runFlowsValidateByQuery(client, filters, batchSize, renderer)
		},
	}

	cmd.Flags().BoolVar(&all, "all", false,
		"Validate every flow of the tenant (mutually exclusive with --namespace, --flow and --filter)")
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
	relabelFromServerIdentity(results)

	if len(results) == 0 {
		fmt.Fprintln(renderer.ErrWriter(), "no flows matched the query")
	}

	return renderValidateResults(results, failed, "FLOW", renderer)
}

// relabelFromServerIdentity rewrites each row's label from the identity the
// validate endpoint returned. The seed label is parsed out of the source, so
// it is only as good as our own YAML read of it; the server's answer is
// authoritative and covers a source we could not parse at all.
func relabelFromServerIdentity(results []ValidateResult) {
	for i := range results {
		if results[i].Namespace != "" && results[i].FlowID != "" {
			results[i].FilePath = results[i].Namespace + "/" + results[i].FlowID
		}
	}
}

// flowLabel names a row in the result table. A stored flow has no path, so it
// is identified as "<namespace>/<flow-id>" — not by its archive entry name,
// which is a flattened "<namespace>-<flow-id>.yml" and reads worse. fallback
// covers a source whose identity could not be read, which is the archive
// entry name.
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
// This is only a seed: relabelFromServerIdentity replaces it with the identity
// the validate endpoint returns, which is authoritative. It is what a row
// falls back to when the server omits one.
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
