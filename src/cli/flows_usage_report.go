package cli

import (
	"fmt"
	"strings"
	"time"

	kestra "github.com/kestra-io/client-sdk/go-sdk/kestra_api_client"
	"github.com/spf13/cobra"
)

// usageReportScope labels what the report covers. kestractl v1 talks to a
// single tenant (the v1 SDK has no tenant listing), so the scope is fixed here
// and only changes together with usageReportTenants.
const usageReportScope = "single-tenant"

// usageReportOptions are the command's selectors.
type usageReportOptions struct {
	Namespace string
	Anonymize bool
}

func newFlowsUsageReportCommand() *cobra.Command {
	opts := usageReportOptions{}

	cmd := &cobra.Command{
		Use:   "usage-report",
		Short: "Build a migration usage report from the flow sources.",
		Long: `Export every flow source and summarize how the tenant uses Kestra.

The report is a markdown document listing the flow inventory (namespaces, task
and trigger types, plugin families) together with the migration signals that
matter for a Kestra 1.x to 2.0 upgrade: pluginDefaults, removed flow tasks such
as ForEach, trigger 'conditions'/'preconditions', the 'condition' property, the
Pebble 'json()' function and 'fs.local.Delete'.

Only flow sources are read — no execution, log or database data. Tenant,
namespace and flow names are replaced by stable hashes unless --anonymize=false
is given, so the report can be shared as-is.`,
		Example: `  # Markdown report for the active tenant
	  kestractl flows usage-report

	  # Machine-readable report
	  kestractl flows usage-report --output json

	  # Restrict to one namespace and keep the real names
	  kestractl flows usage-report --namespace my.namespace --anonymize=false`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := NewClient()
			if err != nil {
				return err
			}

			return runFlowsUsageReport(client, opts, renderer)
		},
	}

	cmd.Flags().StringVarP(&opts.Namespace, "namespace", "n", "", "Only report on flows in this namespace")
	cmd.Flags().BoolVar(&opts.Anonymize, "anonymize", true, "Replace tenant, namespace and flow names with stable hashes")

	return cmd
}

func runFlowsUsageReport(client *Client, opts usageReportOptions, renderer *Renderer) error {
	tenants, tenantNote := usageReportTenants(client)

	scans := make([]tenantScan, 0, len(tenants))
	failed := 0
	for _, tenant := range tenants {
		scan := scanTenant(client, tenant, opts.Namespace)
		if len(scan.Errors) > 0 && len(scan.Flows) == 0 {
			failed++
		}
		scans = append(scans, scan)
	}

	// A single failing tenant only degrades the report; every tenant failing
	// means there is nothing to report on.
	if failed == len(scans) && len(scans) > 0 {
		return fmt.Errorf("failed to collect flow sources: %s", strings.Join(scans[len(scans)-1].Errors, "; "))
	}

	anonymizer := newAnonymizer(opts.Anonymize)
	report := aggregateReport(scans, anonymizer, time.Now())
	report.Scope = usageReportScope
	if tenantNote != "" {
		report.Notes = append([]string{tenantNote}, report.Notes...)
	}

	if renderer.IsJSON() {
		return renderer.RenderJSON(report)
	}
	return renderUsageReportMarkdown(report, renderer.Writer())
}

// usageReportTenants resolves the tenants to scan. It is the single
// version-specific seam of this command: kestractl v1 targets the configured
// tenant, while the v2 port enumerates every tenant it can see.
func usageReportTenants(client *Client) ([]string, string) {
	return []string{client.Tenant}, ""
}

// scanTenant fetches and analyzes one tenant's flows, degrading into notes
// rather than failing whenever part of the data cannot be read.
func scanTenant(client *Client, tenant, namespace string) tenantScan {
	scan := tenantScan{Tenant: tenant}

	sources, notes, err := fetchTenantFlowSources(client, tenant, namespace)
	scan.Notes = append(scan.Notes, notes...)
	if err != nil {
		scan.Errors = append(scan.Errors, err.Error())
		return scan
	}

	for _, source := range sources {
		// A parse failure is not fatal: analyzeFlowSource still returns the
		// text-based signals and flags the flow as unparsed.
		analysis, _ := analyzeFlowSource(source)
		scan.Flows = append(scan.Flows, analysis)
	}

	deprecated, err := fetchTenantDeprecations(client, tenant, namespace)
	if err != nil {
		scan.Notes = append(scan.Notes, "the server-side deprecation check is unavailable: "+err.Error())
	} else {
		scan.Deprecated = deprecated
		scan.DeprecatedAvailable = true
	}

	return scan
}

// fetchTenantFlowSources returns every flow source of a tenant. The export
// endpoint answers with a ZIP of all sources in one request; when it is not
// available the flows are listed and fetched one by one instead, which is
// slower but works with narrower permissions.
func fetchTenantFlowSources(client *Client, tenant, namespace string) ([]string, []string, error) {
	var notes []string

	archive, err := client.Kestra.Flows().ExportFlowsByQuery(client.Ctx, tenant, buildUsageReportSearchFilters(namespace))
	if err == nil {
		sources, skipped, zipErr := flowsFromZip(archive)
		if zipErr == nil {
			if skipped > 0 {
				notes = append(notes, fmt.Sprintf("%d archive entry/entries were skipped (unreadable or larger than %d MiB)", skipped, maxZipEntrySize>>20))
			}
			return sources, notes, nil
		}
		err = zipErr
	} else {
		err = formatSDKError(err)
	}

	notes = append(notes, "the flow export endpoint could not be used ("+err.Error()+"); fell back to fetching flow sources one by one")

	sources, fallbackNotes, fallbackErr := fetchFlowSourcesIndividually(client, tenant, namespace)
	notes = append(notes, fallbackNotes...)
	if fallbackErr != nil {
		return nil, notes, fallbackErr
	}
	return sources, notes, nil
}

// fetchFlowSourcesIndividually is the export fallback: list the flows, then
// read each source. Individual flows that cannot be read are reported as notes
// so a partial report is still produced.
func fetchFlowSourcesIndividually(client *Client, tenant, namespace string) ([]string, []string, error) {
	flows, err := listAllFlowsForTenant(client, tenant)
	if err != nil {
		return nil, nil, err
	}

	var notes []string
	sources := make([]string, 0, len(flows))
	unreadable := 0
	for _, flow := range flows {
		if namespace != "" && flow.Namespace != namespace {
			continue
		}

		source, err := fetchFlowSource(client, tenant, flow.Namespace, flow.ID)
		if err != nil {
			unreadable++
			continue
		}
		sources = append(sources, source)
	}

	if unreadable > 0 {
		notes = append(notes, fmt.Sprintf("%d flow source(s) could not be read and are missing from this report", unreadable))
	}
	return sources, notes, nil
}

// fetchFlowSource reads a single flow's YAML source.
func fetchFlowSource(client *Client, tenant, namespace, flowID string) (string, error) {
	flow, _, err := client.API.FlowsAPI.Flow(client.Ctx, namespace, flowID, tenant).
		Source(true).
		AllowDeleted(false).
		Execute()
	if err != nil {
		// The generated client cannot decode array-format labels (#83);
		// recover the flow from the raw response body.
		parsed, ok := tryParseFlowFromError(err)
		if !ok {
			return "", formatSDKError(err)
		}
		return parsed.Source, nil
	}
	if flow == nil {
		return "", fmt.Errorf("flow not found")
	}
	return flow.GetSource(), nil
}

// fetchTenantDeprecations cross-checks the client-side signals with the
// deprecations the instance itself reports.
func fetchTenantDeprecations(client *Client, tenant, namespace string) ([]deprecatedFlow, error) {
	var ns *string
	if namespace != "" {
		ns = &namespace
	}

	flows, err := client.Kestra.Flows().ListDeprecated(client.Ctx, tenant, ns)
	if err != nil {
		return nil, formatSDKError(err)
	}

	result := make([]deprecatedFlow, 0, len(flows))
	for _, flow := range flows {
		deprecated := flow.GetDeprecatedTasks()
		// Only the plugin type and its replacement are kept: the task id the
		// server also returns is a user-chosen identifier.
		tasks := make([]deprecatedTask, 0, len(deprecated))
		for _, task := range deprecated {
			tasks = append(tasks, deprecatedTask{
				TaskType:    task.GetTaskType(),
				Replacement: task.GetReplacement(),
			})
		}
		result = append(result, deprecatedFlow{
			Namespace: flow.GetNamespace(),
			FlowID:    flow.GetFlowId(),
			TaskCount: len(deprecated),
			Tasks:     tasks,
		})
	}
	return result, nil
}

// buildUsageReportSearchFilters assembles the filter list for the flow export.
// The hand-written client takes SearchFilter values, not the QueryFilter values
// buildFlowExportFilters produces.
func buildUsageReportSearchFilters(namespace string) []kestra.SearchFilter {
	if namespace == "" {
		return nil
	}
	return []kestra.SearchFilter{{
		Field:     kestra.FilterNamespace,
		Operation: kestra.OpEquals,
		Value:     namespace,
	}}
}
