package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	kestra "github.com/kestra-io/client-sdk/go-sdk/v2/kestra_api_client"
	"github.com/spf13/viper"
)

const usageReportFlowA = `
id: order-sync
namespace: prod.orders
tasks:
  - id: each
    type: io.kestra.plugin.core.flow.ForEach
    tasks:
      - id: log
        type: io.kestra.plugin.core.log.Log
        message: "{{ json(inputs.payload) }}"
pluginDefaults:
  - type: io.kestra.plugin.core.log.Log
    forced: true
    values:
      level: INFO
`

const usageReportFlowB = `
id: nightly
namespace: prod.reports
triggers:
  - id: schedule
    type: io.kestra.plugin.core.trigger.Schedule
    cron: "0 2 * * *"
    conditions:
      - type: io.kestra.plugin.core.condition.DayWeekInMonth
tasks:
  - id: call
    type: io.kestra.plugin.core.flow.Subflow
    namespace: prod.orders
    flowId: order-sync
`

// usageReportServer serves the endpoints `flows usage-report` calls. Handlers
// that are nil answer 500, which is how the degradation cases are built.
type usageReportServer struct {
	export     http.HandlerFunc
	deprecated http.HandlerFunc
	search     http.HandlerFunc
	flow       http.HandlerFunc
	configs    http.HandlerFunc
	tenants    http.HandlerFunc

	// tenantsHits counts the requests that reached the tenant-enumeration
	// endpoint, so a test can assert it was never called.
	tenantsHits *int
}

func newUsageReportServer(t *testing.T, routes usageReportServer) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var handler http.HandlerFunc
		switch {
		case strings.HasSuffix(r.URL.Path, "/flows/export/by-query"):
			handler = routes.export
		case strings.HasSuffix(r.URL.Path, "/flows/deprecated"):
			handler = routes.deprecated
		case strings.HasSuffix(r.URL.Path, "/flows/search"):
			handler = routes.search
		case strings.HasSuffix(r.URL.Path, "/configs"):
			handler = routes.configs
		case strings.HasSuffix(r.URL.Path, "/tenants/search"):
			if routes.tenantsHits != nil {
				*routes.tenantsHits++
			}
			handler = routes.tenants
		default:
			handler = routes.flow
		}
		if handler == nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	return server
}

func zipHandler(archive []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(archive)
	}
}

func jsonHandler(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

func TestNewFlowsUsageReportCommand_Structure(t *testing.T) {
	cmd := newFlowsUsageReportCommand()

	if cmd.Use != "usage-report" {
		t.Errorf("use: got %q", cmd.Use)
	}
	if cmd.Args == nil {
		t.Error("expected the command to reject positional arguments")
	}

	namespace := cmd.Flags().Lookup("namespace")
	if namespace == nil || namespace.Shorthand != "n" {
		t.Fatalf("unexpected --namespace flag: %+v", namespace)
	}

	anonymize := cmd.Flags().Lookup("anonymize")
	if anonymize == nil {
		t.Fatal("missing the --anonymize flag")
	}
	if anonymize.DefValue != "true" {
		t.Errorf("--anonymize default: got %q, want \"true\"", anonymize.DefValue)
	}

	detailed := cmd.Flags().Lookup("detailed")
	if detailed == nil {
		t.Fatal("missing the --detailed flag")
	}
	if detailed.DefValue != "false" {
		t.Errorf("--detailed default: got %q, want \"false\"", detailed.DefValue)
	}
}

func TestFlowsUsageReportCommandIsRegistered(t *testing.T) {
	for _, cmd := range newFlowsCommand().Commands() {
		if cmd.Name() == "usage-report" {
			return
		}
	}
	t.Fatal("usage-report is not registered under the flows command")
}

func TestBuildUsageReportSearchFilters(t *testing.T) {
	if filters := buildUsageReportSearchFilters(""); len(filters) != 0 {
		t.Errorf("expected no filter without a namespace, got %+v", filters)
	}

	filters := buildUsageReportSearchFilters("prod.orders")
	if len(filters) != 1 {
		t.Fatalf("filters: got %d, want 1", len(filters))
	}
	if filters[0].Field != kestra.FilterNamespace || filters[0].Operation != kestra.OpEquals || filters[0].Value != "prod.orders" {
		t.Errorf("unexpected filter: %+v", filters[0])
	}
}

func TestRunFlowsUsageReport_HappyPathMarkdown(t *testing.T) {
	archive := buildFlowZip(t, map[string]string{
		"prod.orders/order-sync.yml": usageReportFlowA,
		"prod.reports/nightly.yml":   usageReportFlowB,
	})
	server := newUsageReportServer(t, usageReportServer{
		configs:    jsonHandler(http.StatusOK, `{"uuid":"abc","version":"1.3.2","edition":"EE"}`),
		export:     zipHandler(archive),
		deprecated: jsonHandler(http.StatusOK, `[{"namespace":"prod.orders","flowId":"order-sync","revision":2,"deprecatedTasks":[{"taskId":"each","taskType":"io.kestra.plugin.core.flow.ForEach","replacement":"io.kestra.plugin.core.flow.ForEachItem"}]}]`),
	})

	var out bytes.Buffer
	err := runFlowsUsageReport(newTestClient(t, server.URL), usageReportOptions{Anonymize: true, Detailed: true}, newTableRenderer(&out))
	if err != nil {
		t.Fatalf("runFlowsUsageReport returned an error: %v", err)
	}

	markdown := collapseSpaces(out.String())
	for _, want := range []string{
		"# Kestra usage report",
		"### Flows per namespace",
		"## Affected flows",
		"| Task ForEach | 1 | 1 |",
		"| Trigger conditions/preconditions | 1 | 1 |",
		"| Flows | 2 |",
		"Server-reported deprecations",
		"io.kestra.plugin.core.flow.Subflow",
		"- Kestra version: 1.3.2",
		"## Deprecated task types (server-reported)",
		"| `io.kestra.plugin.core.flow.ForEach` | `io.kestra.plugin.core.flow.ForEachItem` | 1 |",
	} {
		if !strings.Contains(markdown, want) {
			t.Errorf("markdown is missing %q", want)
		}
	}
	// The server also returns the task id; it is a user identifier and is
	// dropped on the way into the report.
	if strings.Contains(markdown, "each") && strings.Contains(markdown, "taskId") {
		t.Error("the report must not carry task ids")
	}
	if strings.Contains(markdown, "prod.orders") {
		t.Error("the anonymized report leaked a namespace name")
	}
}

func TestRunFlowsUsageReport_JSONAndAnonymizeOff(t *testing.T) {
	archive := buildFlowZip(t, map[string]string{"prod.orders/order-sync.yml": usageReportFlowA})
	server := newUsageReportServer(t, usageReportServer{
		configs:    jsonHandler(http.StatusOK, `{"version":"1.3.2"}`),
		export:     zipHandler(archive),
		deprecated: jsonHandler(http.StatusOK, `[]`),
	})

	var out bytes.Buffer
	err := runFlowsUsageReport(newTestClient(t, server.URL), usageReportOptions{Anonymize: false}, newJSONRenderer(&out))
	if err != nil {
		t.Fatalf("runFlowsUsageReport returned an error: %v", err)
	}

	var report usageReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("failed to decode the JSON report: %v", err)
	}
	if report.Anonymized {
		t.Error("expected the report to be flagged as not anonymized")
	}
	if report.KestraVersion != "1.3.2" {
		t.Errorf("kestra version: got %q, want \"1.3.2\"", report.KestraVersion)
	}
	if report.Scope != scopeSingleTenant || report.Totals.Count != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}
	if len(report.Tenants) != 1 || report.Tenants[0].Tenant != "main" {
		t.Fatalf("unexpected tenant breakdown: %+v", report.Tenants)
	}
	if report.Tenants[0].Namespaces[0].Namespace != "prod.orders" {
		t.Errorf("expected the real namespace with --anonymize=false, got %q", report.Tenants[0].Namespaces[0].Namespace)
	}
	if report.Signals.PluginDefaults.Entries != 1 || report.Signals.PluginDefaults.ForcedEntries != 1 {
		t.Errorf("unexpected plugin defaults signal: %+v", report.Signals.PluginDefaults)
	}
	if report.Signals.PebbleJsonFunction.Occurrences != 1 {
		t.Errorf("unexpected pebble signal: %+v", report.Signals.PebbleJsonFunction)
	}

	// The JSON dump is complete regardless of --detailed.
	if len(report.Signals.RemovedTasks["ForEach"].FlowRefs) != 1 {
		t.Errorf("expected flow refs in the JSON output, got %+v", report.Signals.RemovedTasks["ForEach"])
	}
	if len(report.Tenants[0].Namespaces) != 1 {
		t.Errorf("expected the per-namespace breakdown in the JSON output, got %+v", report.Tenants[0].Namespaces)
	}
}

func TestRunFlowsUsageReport_ExportForbiddenFallsBack(t *testing.T) {
	server := newUsageReportServer(t, usageReportServer{
		export:     jsonHandler(http.StatusForbidden, `{"message":"insufficient permissions"}`),
		search:     jsonHandler(http.StatusOK, `{"results":[{"id":"order-sync","namespace":"prod.orders"}],"total":1}`),
		flow:       jsonHandler(http.StatusOK, `{"id":"order-sync","namespace":"prod.orders","revision":1,"source":`+mustJSONString(t, usageReportFlowA)+`}`),
		deprecated: jsonHandler(http.StatusOK, `[]`),
	})

	var out bytes.Buffer
	err := runFlowsUsageReport(newTestClient(t, server.URL), usageReportOptions{Anonymize: true}, newJSONRenderer(&out))
	if err != nil {
		t.Fatalf("runFlowsUsageReport returned an error: %v", err)
	}

	var report usageReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("failed to decode the JSON report: %v", err)
	}
	if report.Totals.Count != 1 {
		t.Fatalf("expected the fallback to recover 1 flow, got %d", report.Totals.Count)
	}
	if report.Signals.RemovedTasks["ForEach"].Occurrences != 1 {
		t.Error("expected the fallback sources to be analyzed")
	}
	if !strings.Contains(strings.Join(report.Notes, "\n"), "fell back to fetching flow sources one by one") {
		t.Errorf("expected a fallback note, got %v", report.Notes)
	}
}

func TestRunFlowsUsageReport_AllSourcesUnavailable(t *testing.T) {
	server := newUsageReportServer(t, usageReportServer{
		export: jsonHandler(http.StatusForbidden, `{"message":"insufficient permissions"}`),
	})

	var out bytes.Buffer
	err := runFlowsUsageReport(newTestClient(t, server.URL), usageReportOptions{Anonymize: true}, newTableRenderer(&out))
	if err == nil {
		t.Fatal("expected an error when no flow source could be collected")
	}
	if !strings.Contains(err.Error(), "failed to collect flow sources") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunFlowsUsageReport_UnparsableFlowIsNoted(t *testing.T) {
	archive := buildFlowZip(t, map[string]string{
		"prod.orders/order-sync.yml": usageReportFlowA,
		"prod.broken/broken.yml":     "id: broken\n\tmessage: \"{{ json(x) }}\"\n",
	})
	server := newUsageReportServer(t, usageReportServer{
		export:     zipHandler(archive),
		deprecated: jsonHandler(http.StatusOK, `[]`),
	})

	var out bytes.Buffer
	err := runFlowsUsageReport(newTestClient(t, server.URL), usageReportOptions{Anonymize: true}, newJSONRenderer(&out))
	if err != nil {
		t.Fatalf("runFlowsUsageReport returned an error: %v", err)
	}

	var report usageReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("failed to decode the JSON report: %v", err)
	}
	if report.Totals.Count != 1 {
		t.Errorf("only the parsable flow should be inventoried, got %d", report.Totals.Count)
	}
	if report.Tenants[0].ParseFails != 1 {
		t.Errorf("parse fails: got %d, want 1", report.Tenants[0].ParseFails)
	}
	if !strings.Contains(strings.Join(report.Notes, "\n"), "could not be parsed as YAML") {
		t.Errorf("expected a parse-failure note, got %v", report.Notes)
	}
	// The text heuristic runs even on sources that failed to parse.
	if report.Signals.PebbleJsonFunction.Occurrences != 2 {
		t.Errorf("pebble json occurrences: got %d, want 2", report.Signals.PebbleJsonFunction.Occurrences)
	}
}

func TestRunFlowsUsageReport_DeprecationEndpointUnavailable(t *testing.T) {
	archive := buildFlowZip(t, map[string]string{"prod.orders/order-sync.yml": usageReportFlowA})
	server := newUsageReportServer(t, usageReportServer{
		export:     zipHandler(archive),
		deprecated: jsonHandler(http.StatusNotFound, `{"message":"not found"}`),
	})

	var out bytes.Buffer
	err := runFlowsUsageReport(newTestClient(t, server.URL), usageReportOptions{Anonymize: true}, newJSONRenderer(&out))
	if err != nil {
		t.Fatalf("runFlowsUsageReport returned an error: %v", err)
	}

	var report usageReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("failed to decode the JSON report: %v", err)
	}
	if report.Signals.ServerDeprecationsAvailable {
		t.Error("expected the server deprecation cross-check to be unavailable")
	}
	if !strings.Contains(strings.Join(report.Notes, "\n"), "deprecation check is unavailable") {
		t.Errorf("expected a deprecation note, got %v", report.Notes)
	}
}

func TestRunFlowsUsageReport_SummaryOmitsDetailedBlocks(t *testing.T) {
	archive := buildFlowZip(t, map[string]string{"prod.orders/order-sync.yml": usageReportFlowA})
	server := newUsageReportServer(t, usageReportServer{
		configs:    jsonHandler(http.StatusOK, `{"version":"1.3.2"}`),
		export:     zipHandler(archive),
		deprecated: jsonHandler(http.StatusOK, `[]`),
	})

	var out bytes.Buffer
	err := runFlowsUsageReport(newTestClient(t, server.URL), usageReportOptions{Anonymize: true}, newTableRenderer(&out))
	if err != nil {
		t.Fatalf("runFlowsUsageReport returned an error: %v", err)
	}

	markdown := collapseSpaces(out.String())
	for _, unwanted := range []string{"### Flows per namespace", "## Affected flows"} {
		if strings.Contains(markdown, unwanted) {
			t.Errorf("the default report must not contain %q", unwanted)
		}
	}
	if !strings.Contains(markdown, "_Run with --detailed to list the affected flows._") {
		t.Error("expected the default report to point at --detailed")
	}
	// The signal counts are unaffected by the flag.
	if !strings.Contains(markdown, "| Task ForEach | 1 | 1 |") || !strings.Contains(markdown, "| Namespaces | 1 |") {
		t.Error("the default report lost data that --detailed must not gate")
	}
}

func TestRunFlowsUsageReport_ServerVersionUnavailable(t *testing.T) {
	archive := buildFlowZip(t, map[string]string{"prod.orders/order-sync.yml": usageReportFlowA})
	server := newUsageReportServer(t, usageReportServer{
		configs:    jsonHandler(http.StatusNotFound, `{"message":"not found"}`),
		export:     zipHandler(archive),
		deprecated: jsonHandler(http.StatusOK, `[]`),
	})

	var out bytes.Buffer
	err := runFlowsUsageReport(newTestClient(t, server.URL), usageReportOptions{Anonymize: true}, newTableRenderer(&out))
	if err != nil {
		t.Fatalf("runFlowsUsageReport returned an error: %v", err)
	}

	markdown := collapseSpaces(out.String())
	if !strings.Contains(markdown, "- Kestra version: unknown") {
		t.Error("expected the Kestra version to render as unknown")
	}
	if !strings.Contains(markdown, "the Kestra server version could not be read") {
		t.Error("expected a scan note about the unreadable server version")
	}
	// The rest of the report is unaffected.
	if !strings.Contains(markdown, "| Flows | 1 |") {
		t.Error("expected the report to be produced anyway")
	}
}

func TestUsageReportTenants_ExplicitTenantSkipsEnumeration(t *testing.T) {
	tenants, scope, note := usageReportTenants(&Client{Tenant: "acme"}, true)

	if len(tenants) != 1 || tenants[0] != "acme" {
		t.Errorf("unexpected tenants: %v", tenants)
	}
	if scope != scopeSingleTenant {
		t.Errorf("scope: got %q, want %q", scope, scopeSingleTenant)
	}
	if note != "" {
		t.Errorf("unexpected note: %q", note)
	}
}

// tenantsPage renders one page of the tenant search response.
func tenantsPage(total int, ids ...string) string {
	results := make([]string, 0, len(ids))
	for _, id := range ids {
		results = append(results, `{"id":"`+id+`","name":"`+id+`","deleted":false}`)
	}
	return `{"results":[` + strings.Join(results, ",") + `],"total":` + strconv.Itoa(total) + `}`
}

func TestRunFlowsUsageReport_AllTenants(t *testing.T) {
	archives := map[string][]byte{
		"alpha": buildFlowZip(t, map[string]string{"prod.orders/order-sync.yml": usageReportFlowA}),
		"beta":  buildFlowZip(t, map[string]string{"prod.reports/nightly.yml": usageReportFlowB}),
	}
	server := newUsageReportServer(t, usageReportServer{
		configs: jsonHandler(http.StatusOK, `{"version":"2.0.0"}`),
		tenants: jsonHandler(http.StatusOK, tenantsPage(2, "alpha", "beta")),
		export: func(w http.ResponseWriter, r *http.Request) {
			for tenant, archive := range archives {
				if strings.Contains(r.URL.Path, "/"+tenant+"/") {
					zipHandler(archive)(w, r)
					return
				}
			}
			w.WriteHeader(http.StatusNotFound)
		},
		deprecated: jsonHandler(http.StatusOK, `[]`),
	})

	var jsonOut bytes.Buffer
	client := newTestClient(t, server.URL)
	if err := runFlowsUsageReport(client, usageReportOptions{Anonymize: false}, newJSONRenderer(&jsonOut)); err != nil {
		t.Fatalf("runFlowsUsageReport returned an error: %v", err)
	}

	var report usageReport
	if err := json.Unmarshal(jsonOut.Bytes(), &report); err != nil {
		t.Fatalf("failed to decode the JSON report: %v", err)
	}
	if report.Scope != scopeAllTenants {
		t.Errorf("scope: got %q, want %q", report.Scope, scopeAllTenants)
	}
	if report.Totals.TenantsScanned != 2 || report.Totals.Count != 2 {
		t.Fatalf("unexpected totals: %+v", report.Totals)
	}
	if len(report.Tenants) != 2 || report.Tenants[0].Tenant != "alpha" || report.Tenants[1].Tenant != "beta" {
		t.Fatalf("unexpected tenant breakdown: %+v", report.Tenants)
	}
	// Both tenants' flows are aggregated into the shared signal counts.
	if report.Signals.RemovedTasks["ForEach"].Occurrences != 1 || report.Signals.TriggerConditions.Occurrences != 1 {
		t.Errorf("unexpected merged signals: %+v", report.Signals)
	}

	var markdown bytes.Buffer
	if err := runFlowsUsageReport(client, usageReportOptions{Anonymize: true}, newTableRenderer(&markdown)); err != nil {
		t.Fatalf("runFlowsUsageReport returned an error: %v", err)
	}
	if !strings.Contains(markdown.String(), "- Scope: all-tenants") {
		t.Error("expected the all-tenants scope in the markdown header")
	}
	if !strings.Contains(collapseSpaces(markdown.String()), "| Tenants scanned | 2 |") {
		t.Error("expected both tenants in the inventory")
	}
}

func TestRunFlowsUsageReport_TenantEnumerationForbidden(t *testing.T) {
	archive := buildFlowZip(t, map[string]string{"prod.orders/order-sync.yml": usageReportFlowA})
	server := newUsageReportServer(t, usageReportServer{
		configs:    jsonHandler(http.StatusOK, `{"version":"2.0.0"}`),
		tenants:    jsonHandler(http.StatusForbidden, `{"message":"insufficient permissions"}`),
		export:     zipHandler(archive),
		deprecated: jsonHandler(http.StatusOK, `[]`),
	})

	var out bytes.Buffer
	if err := runFlowsUsageReport(newTestClient(t, server.URL), usageReportOptions{Anonymize: true}, newJSONRenderer(&out)); err != nil {
		t.Fatalf("runFlowsUsageReport returned an error: %v", err)
	}

	var report usageReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("failed to decode the JSON report: %v", err)
	}
	if report.Scope != scopeSingleTenant {
		t.Errorf("scope: got %q, want %q", report.Scope, scopeSingleTenant)
	}
	if report.Totals.TenantsScanned != 1 || report.Totals.Count != 1 {
		t.Fatalf("unexpected totals: %+v", report.Totals)
	}
	note := strings.Join(report.Notes, "\n")
	if !strings.Contains(note, "tenant enumeration failed") || !strings.Contains(note, "covers only the configured tenant") {
		t.Errorf("expected the enumeration fallback note, got %v", report.Notes)
	}
}

func TestRunFlowsUsageReport_ExplicitTenantSkipsEnumeration(t *testing.T) {
	hits := 0
	archive := buildFlowZip(t, map[string]string{"prod.orders/order-sync.yml": usageReportFlowA})
	server := newUsageReportServer(t, usageReportServer{
		configs:     jsonHandler(http.StatusOK, `{"version":"2.0.0"}`),
		tenants:     jsonHandler(http.StatusOK, tenantsPage(2, "alpha", "beta")),
		tenantsHits: &hits,
		export:      zipHandler(archive),
		deprecated:  jsonHandler(http.StatusOK, `[]`),
	})

	var out bytes.Buffer
	opts := usageReportOptions{Anonymize: false, ExplicitTenant: true}
	if err := runFlowsUsageReport(newTestClient(t, server.URL), opts, newJSONRenderer(&out)); err != nil {
		t.Fatalf("runFlowsUsageReport returned an error: %v", err)
	}

	var report usageReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("failed to decode the JSON report: %v", err)
	}
	if hits != 0 {
		t.Errorf("the tenant endpoint was called %d time(s) despite an explicit --tenant", hits)
	}
	if report.Scope != scopeSingleTenant || len(report.Tenants) != 1 || report.Tenants[0].Tenant != "main" {
		t.Fatalf("unexpected report scope or tenants: %q %+v", report.Scope, report.Tenants)
	}
	if len(report.Notes) != 0 {
		t.Errorf("an explicit tenant must not produce a note, got %v", report.Notes)
	}
}

func TestRunFlowsUsageReport_ContextTenantStillEnumerates(t *testing.T) {
	hits := 0
	archive := buildFlowZip(t, map[string]string{"prod.orders/order-sync.yml": usageReportFlowA})
	server := newUsageReportServer(t, usageReportServer{
		configs:     jsonHandler(http.StatusOK, `{"version":"2.0.0"}`),
		tenants:     jsonHandler(http.StatusOK, tenantsPage(1, "engineering")),
		tenantsHits: &hits,
		export:      zipHandler(archive),
		deprecated:  jsonHandler(http.StatusOK, `[]`),
	})

	// A tenant coming from the auth context is not a restriction: the client
	// is pointed at it, yet the report must still enumerate.
	client := newTestClient(t, server.URL)
	client.Tenant = "engineering"

	var out bytes.Buffer
	if err := runFlowsUsageReport(client, usageReportOptions{Anonymize: false}, newJSONRenderer(&out)); err != nil {
		t.Fatalf("runFlowsUsageReport returned an error: %v", err)
	}

	var report usageReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("failed to decode the JSON report: %v", err)
	}
	if hits == 0 {
		t.Error("expected the tenant endpoint to be called when no tenant was pinned")
	}
	if report.Scope != scopeAllTenants {
		t.Errorf("scope: got %q, want %q", report.Scope, scopeAllTenants)
	}
}

// TestFlowsUsageReportCommand_ExplicitTenantWiring drives the real command so
// the RunE wiring of ExplicitTenant is covered, not just runFlowsUsageReport.
func TestFlowsUsageReportCommand_ExplicitTenantWiring(t *testing.T) {
	t.Setenv("KESTRACTL_TELEMETRY_DISABLED", "true")
	viper.Reset()
	t.Cleanup(viper.Reset)

	// The command writes through the global output flag; put it back.
	previousOutput := globalFlags.Output
	t.Cleanup(func() { globalFlags.Output = previousOutput })

	hits := 0
	archive := buildFlowZip(t, map[string]string{"prod.orders/order-sync.yml": usageReportFlowA})
	server := newUsageReportServer(t, usageReportServer{
		configs:     jsonHandler(http.StatusOK, `{"version":"2.0.0"}`),
		tenants:     jsonHandler(http.StatusOK, tenantsPage(2, "alpha", "beta")),
		tenantsHits: &hits,
		export:      zipHandler(archive),
		deprecated:  jsonHandler(http.StatusOK, `[]`),
	})

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		client := newTestClient(t, server.URL)
		client.Tenant = "main"
		return client, nil
	}
	t.Cleanup(func() { newClientFunc = original })

	run := func(args ...string) string {
		t.Helper()
		var out bytes.Buffer
		root := NewRootCommand()
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Fatalf("command %v failed: %v", args, err)
		}
		return out.String()
	}

	out := run("flows", "usage-report", "--tenant", "main", "--output", "json")
	if hits != 0 {
		t.Errorf("--tenant must skip enumeration, but the endpoint was called %d time(s)", hits)
	}
	if !strings.Contains(out, `"scope": "`+scopeSingleTenant+`"`) {
		t.Errorf("expected the single-tenant scope, got:\n%s", out)
	}

	// Without the flag the same command enumerates.
	hits = 0
	viper.Reset()
	out = run("flows", "usage-report", "--output", "json")
	if hits == 0 {
		t.Error("expected enumeration when --tenant is absent")
	}
	if !strings.Contains(out, `"scope": "`+scopeAllTenants+`"`) {
		t.Errorf("expected the all-tenants scope, got:\n%s", out)
	}
}

func mustJSONString(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("failed to encode %q: %v", value, err)
	}
	return string(encoded)
}
