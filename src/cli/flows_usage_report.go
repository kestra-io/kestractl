package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	kestra "github.com/kestra-io/client-sdk/go-sdk/v2/kestra_api_client"
	"github.com/spf13/cobra"
)

// The scopes a report can cover, decided by usageReportTenants.
const (
	scopeSingleTenant = "single-tenant"
	scopeAllTenants   = "all-tenants"
)

// tenantPageSize is how many tenants one enumeration request asks for.
const tenantPageSize = 100

// usageReportOptions are the command's selectors.
type usageReportOptions struct {
	Namespace string
	Anonymize bool
	Detailed  bool

	// ExplicitTenant records that the caller pinned a tenant for this
	// invocation, which restricts the report to it. A tenant stored in the
	// auth context is "the tenant I usually work in", not a restriction, so
	// only the flag and the environment variable count.
	ExplicitTenant bool
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
is given, so the report can be shared as-is.

The report covers every tenant the caller can see; pass --tenant (or set
KESTRACTL_TENANT) to restrict it to a single one.

The markdown report is a summary; --detailed adds the per-namespace table and
the affected-flow list of every signal. '--output json' always contains the
full data, whether or not --detailed is given.`,
		Example: `  # Markdown report for the active tenant
	  kestractl flows usage-report

	  # Machine-readable report
	  kestractl flows usage-report --output json

	  # Restrict to one namespace and keep the real names
	  kestractl flows usage-report --namespace my.namespace --anonymize=false

	  # Add the per-namespace table and the affected-flow lists
	  kestractl flows usage-report --detailed`,
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

			opts.ExplicitTenant = explicitTenantRequested(cmd)

			return runFlowsUsageReport(client, opts, renderer)
		},
	}

	cmd.Flags().StringVarP(&opts.Namespace, "namespace", "n", "", "Only report on flows in this namespace")
	cmd.Flags().BoolVar(&opts.Anonymize, "anonymize", true, "Replace tenant, namespace and flow names with stable hashes")
	cmd.Flags().BoolVar(&opts.Detailed, "detailed", false, "Add the per-namespace table and the affected-flow lists to the markdown report (JSON output always contains them)")

	return cmd
}

func runFlowsUsageReport(client *Client, opts usageReportOptions, renderer *Renderer) error {
	tenants, scope, tenantNote := usageReportTenants(client, opts.ExplicitTenant)

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
	report.Scope = scope
	// The instance version is a nice-to-have header field: read it once, and
	// note the failure rather than losing the whole report over it.
	kestraVersion, err := fetchKestraVersion(client)
	if err != nil {
		report.Notes = append(report.Notes, "the Kestra server version could not be read: "+err.Error())
	}
	report.KestraVersion = kestraVersion
	if tenantNote != "" {
		report.Notes = append([]string{tenantNote}, report.Notes...)
	}

	if renderer.IsJSON() {
		return renderer.RenderJSON(report)
	}
	return renderUsageReportMarkdown(report, renderer.Writer(), opts.Detailed)
}

// usageReportTenants resolves the tenants to scan, the scope label for the
// report header, and an optional note. It is the single version-specific seam
// of this command: kestractl v1 can only target the configured tenant, while
// v2 enumerates every tenant the caller can see.
//
// A tenant pinned for this invocation is taken as a deliberate restriction and
// skips enumeration entirely.
func usageReportTenants(client *Client, explicitTenant bool) ([]string, string, string) {
	if explicitTenant {
		return []string{client.Tenant}, scopeSingleTenant, ""
	}

	tenants, err := listAllTenants(client)
	if err != nil {
		// Enumeration needs superadmin rights; a regular user simply gets the
		// tenant they are configured for, plus a note saying so.
		return []string{client.Tenant}, scopeSingleTenant,
			"tenant enumeration failed (" + err.Error() + "); report covers only the configured tenant"
	}
	if len(tenants) == 0 {
		return []string{client.Tenant}, scopeSingleTenant, ""
	}
	return tenants, scopeAllTenants, ""
}

// explicitTenantRequested reports whether this invocation pinned a tenant. Only
// the --tenant flag and KESTRACTL_TENANT count: the active auth context almost
// always carries a tenant, and viper cannot tell that default apart from a
// deliberate restriction (root.go seeds the key with viper.SetDefault, which
// makes viper.IsSet true for everyone).
func explicitTenantRequested(cmd *cobra.Command) bool {
	// Cobra merges the parents' persistent flags into Flags(); the root
	// lookup is a safeguard for a command run outside the root tree.
	if cmd.Flags().Changed(FlagTenant) || cmd.Root().PersistentFlags().Changed(FlagTenant) {
		return true
	}
	// Viper binds the environment with the KESTRACTL prefix (root.go:142).
	return strings.TrimSpace(os.Getenv("KESTRACTL_TENANT")) != ""
}

// listAllTenants pages through the tenant list.
func listAllTenants(client *Client) ([]string, error) {
	size := tenantPageSize
	tenants := make([]string, 0)

	for page := 1; ; page++ {
		pageParam := page
		resp, err := client.Kestra.Tenants().SearchTenants(client.Ctx, &pageParam, &size, nil, nil)
		if err != nil {
			return nil, formatSDKError(err)
		}
		if resp == nil || len(resp.Results) == 0 {
			break
		}

		for _, tenant := range resp.Results {
			if id := tenant.GetId(); id != "" {
				tenants = append(tenants, id)
			}
		}
		if int64(len(tenants)) >= resp.Total {
			break
		}
	}

	return tenants, nil
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

// fetchKestraVersion reads the instance version from the (tenant-independent)
// configuration endpoint.
func fetchKestraVersion(client *Client) (string, error) {
	config, err := client.Kestra.Misc().Configuration(client.Ctx)
	if err != nil {
		return "", formatSDKError(err)
	}

	version, ok := config["version"].(string)
	if !ok || version == "" {
		return "", fmt.Errorf("the configuration endpoint returned no version")
	}
	return version, nil
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
