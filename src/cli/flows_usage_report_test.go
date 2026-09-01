package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	kestra "github.com/kestra-io/client-sdk/go-sdk/kestra_api_client"
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
	if report.Scope != usageReportScope || report.Totals.Count != 1 {
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

func TestUsageReportTenants_SingleTenantOnV1(t *testing.T) {
	tenants, note := usageReportTenants(&Client{Tenant: "acme"})

	if len(tenants) != 1 || tenants[0] != "acme" {
		t.Errorf("unexpected tenants: %v", tenants)
	}
	if note != "" {
		t.Errorf("unexpected note: %q", note)
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
