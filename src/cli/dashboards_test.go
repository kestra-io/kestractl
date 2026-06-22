package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDashboardsCommand_Structure(t *testing.T) {
	cmd := newDashboardsCommand()
	subNames := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subNames[sub.Name()] = true
	}
	for _, want := range []string{
		"list", "get", "create", "update", "delete",
		"defaults", "validate", "validate-chart", "preview-chart",
		"chart-data", "export-chart-csv", "export-chart-data-csv",
	} {
		if !subNames[want] {
			t.Errorf("expected subcommand %q to exist", want)
		}
	}
}

func TestDashboardsGetCommand_NoArgs(t *testing.T) {
	cmd := newDashboardsGetCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestRunDashboardsList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[
			{"id":"d1","title":"My Dashboard","deleted":false},
			{"id":"d2","title":"Another Dashboard","deleted":true}
		],"total":2}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runDashboardsList(newTestClient(t, server.URL), "", 1, 100, nil, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runDashboardsList error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"d1", "My Dashboard", "d2", "Another Dashboard", "Total dashboards: 2"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunDashboardsList_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"list failed"}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runDashboardsList(newTestClient(t, server.URL), "", 1, 100, nil, newTableRenderer(&buf))
	if err == nil {
		t.Fatal("expected error from failing API")
	}
	if !strings.Contains(err.Error(), "list failed") {
		t.Errorf("expected formatted SDK error, got: %v", err)
	}
}

func TestRunDashboardsGet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"d1","title":"My Dashboard","description":"A test dashboard","deleted":false}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runDashboardsGet(newTestClient(t, server.URL), "d1", newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runDashboardsGet error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"d1", "My Dashboard", "A test dashboard"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestDashboardsCreateCommand_NoFile(t *testing.T) {
	cmd := newDashboardsCreateCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when --file is missing")
	}
	if !strings.Contains(err.Error(), "--file is required") {
		t.Fatalf("expected --file error, got: %v", err)
	}
}

func TestRunDashboardsCreate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"d1","title":"My Dashboard","deleted":false}`))
	}))
	t.Cleanup(server.Close)

	f, err := os.CreateTemp("", "dashboard-*.yml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	_, _ = f.WriteString("title: My Dashboard\ncharts: []")
	f.Close()

	var buf bytes.Buffer
	err = runDashboardsCreate(newTestClient(t, server.URL), f.Name(), newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runDashboardsCreate error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"d1", "My Dashboard", "created"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestDashboardsUpdateCommand_NoFile(t *testing.T) {
	cmd := newDashboardsUpdateCommand()
	_, err := executeCommand(cmd, "d1")
	if err == nil {
		t.Fatal("expected error when --file is missing")
	}
	if !strings.Contains(err.Error(), "--file is required") {
		t.Fatalf("expected --file error, got: %v", err)
	}
}

func TestRunDashboardsUpdate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"d1","title":"Updated Dashboard","deleted":false}`))
	}))
	t.Cleanup(server.Close)

	f, err := os.CreateTemp("", "dashboard-*.yml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	_, _ = f.WriteString("title: Updated Dashboard\ncharts: []")
	f.Close()

	var buf bytes.Buffer
	err = runDashboardsUpdate(newTestClient(t, server.URL), "d1", f.Name(), newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runDashboardsUpdate error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"d1", "Updated Dashboard", "updated"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunDashboardsDelete_Cancelled(t *testing.T) {
	hit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runDashboardsDelete(newTestClient(t, server.URL), "d1", false, strings.NewReader("n\n"), newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runDashboardsDelete error: %v", err)
	}
	if hit {
		t.Error("expected no API request when deletion is cancelled")
	}
	if !strings.Contains(buf.String(), "Cancelled") {
		t.Errorf("expected cancellation message, got:\n%s", buf.String())
	}
}

func TestRunDashboardsDelete_Confirmed(t *testing.T) {
	hit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runDashboardsDelete(newTestClient(t, server.URL), "d1", false, strings.NewReader("y\n"), newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runDashboardsDelete error: %v", err)
	}
	if !hit {
		t.Error("expected API request when deletion is confirmed")
	}
	if !strings.Contains(buf.String(), "deleted") {
		t.Errorf("expected deletion message, got:\n%s", buf.String())
	}
}

func TestRunDashboardsDelete_SkipConfirm(t *testing.T) {
	hit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runDashboardsDelete(newTestClient(t, server.URL), "d1", true, strings.NewReader(""), newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runDashboardsDelete error: %v", err)
	}
	if !hit {
		t.Error("expected API request when --yes is set")
	}
}

func TestRunDashboardsDefaults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/dashboards/settings/default-dashboards") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"defaultHomeDashboard":"home-1","defaultFlowOverviewDashboard":"flow-1"}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runDashboardsDefaults(newTestClient(t, server.URL), newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runDashboardsDefaults error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"home-1", "flow-1", "(none)"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunDashboardsValidate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/dashboards/validate") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"index":0,"constraints":"","warnings":["watch out"]}`))
	}))
	t.Cleanup(server.Close)

	f := writeTempFile(t, "title: My Dashboard\ncharts: []")
	var buf bytes.Buffer
	err := runDashboardsValidate(newTestClient(t, server.URL), f, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runDashboardsValidate error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"passed", "watch out"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunDashboardsValidateChart(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/dashboards/validate/chart") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"index":0,"constraints":"bad chart"}`))
	}))
	t.Cleanup(server.Close)

	f := writeTempFile(t, "type: bar")
	var buf bytes.Buffer
	err := runDashboardsValidateChart(newTestClient(t, server.URL), f, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runDashboardsValidateChart error: %v", err)
	}
	if !strings.Contains(buf.String(), "bad chart") {
		t.Errorf("expected validation failure, got:\n%s", buf.String())
	}
}

func TestRunDashboardsPreviewChart(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/dashboards/charts/preview") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"x":"a"},{"x":"b"}],"total":2}`))
	}))
	t.Cleanup(server.Close)

	f := writeTempFile(t, "id: c1\ntype: bar")
	var buf bytes.Buffer
	err := runDashboardsPreviewChart(newTestClient(t, server.URL), f, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runDashboardsPreviewChart error: %v", err)
	}
	// The chart field must be sent as a raw string, not a nested object.
	if got, ok := gotBody["chart"].(string); !ok || !strings.Contains(got, "type: bar") {
		t.Errorf("expected chart sent as a YAML string, got body: %v", gotBody)
	}
	out := buf.String()
	for _, want := range []string{"Rows", "Total", "2"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunDashboardsChartData(t *testing.T) {
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/dashboards/my-dashboard/charts/my-chart") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"x":"a"}],"total":1}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runDashboardsChartData(newTestClient(t, server.URL), "my-dashboard", "my-chart", "", newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runDashboardsChartData error: %v", err)
	}
	// The endpoint requires a body; with no --file we must still send "{}".
	if strings.TrimSpace(gotBody) != "{}" {
		t.Errorf("expected empty-object filters body, got: %q", gotBody)
	}
	if !strings.Contains(buf.String(), "Rows") {
		t.Errorf("expected chart data summary, got:\n%s", buf.String())
	}
}

func TestRunDashboardsExportChartCSV_Stdout(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/dashboards/charts/export/to-csv") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte("col1,col2\n1,2\n"))
	}))
	t.Cleanup(server.Close)

	f := writeTempFile(t, "id: c1\ntype: bar")
	var buf bytes.Buffer
	err := runDashboardsExportChartCSV(newTestClient(t, server.URL), f, "", &buf)
	if err != nil {
		t.Fatalf("runDashboardsExportChartCSV error: %v", err)
	}
	if !strings.Contains(buf.String(), "col1,col2") {
		t.Errorf("expected CSV output, got:\n%s", buf.String())
	}
	// The chart field must be sent as a raw string, not a nested object.
	if got, ok := gotBody["chart"].(string); !ok || !strings.Contains(got, "type: bar") {
		t.Errorf("expected chart sent as a YAML string, got body: %v", gotBody)
	}
}

func TestRunDashboardsExportChartDataCSV_File(t *testing.T) {
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/dashboards/my-dashboard/charts/my-chart/export/to-csv") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte("a,b\n3,4\n"))
	}))
	t.Cleanup(server.Close)

	outPath := filepath.Join(t.TempDir(), "chart.csv")
	var buf bytes.Buffer
	err := runDashboardsExportChartDataCSV(newTestClient(t, server.URL), "my-dashboard", "my-chart", "", outPath, &buf)
	if err != nil {
		t.Fatalf("runDashboardsExportChartDataCSV error: %v", err)
	}
	// The endpoint requires a body; with no --file we must still send "{}".
	if strings.TrimSpace(gotBody) != "{}" {
		t.Errorf("expected empty-object global filter body, got: %q", gotBody)
	}
	if !strings.Contains(buf.String(), "exported to") {
		t.Errorf("expected export confirmation, got:\n%s", buf.String())
	}
	data, readErr := os.ReadFile(outPath)
	if readErr != nil {
		t.Fatalf("read output file: %v", readErr)
	}
	if !strings.Contains(string(data), "a,b") {
		t.Errorf("expected CSV in file, got: %s", string(data))
	}
}

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "input.yml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}

func TestDashboardsListCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newDashboardsListCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}
