package cli

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestAssetsCommand_Structure(t *testing.T) {
	cmd := newAssetsCommand()
	subNames := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subNames[sub.Name()] = true
	}
	for _, want := range []string{
		"list", "get", "create", "delete",
		"dependencies", "delete-by-ids", "delete-by-query",
		"lineage-events", "usages",
	} {
		if !subNames[want] {
			t.Errorf("expected subcommand %q to exist", want)
		}
	}
}

func TestAssetsSubgroups_Structure(t *testing.T) {
	for _, tc := range []struct {
		group *cobra.Command
		want  []string
	}{
		{newAssetsLineageEventsCommand(), []string{"list", "delete-by-query"}},
		{newAssetsUsagesCommand(), []string{"list", "delete-by-query"}},
	} {
		subNames := make(map[string]bool)
		for _, sub := range tc.group.Commands() {
			subNames[sub.Name()] = true
		}
		for _, want := range tc.want {
			if !subNames[want] {
				t.Errorf("%s: expected subcommand %q", tc.group.Name(), want)
			}
		}
	}
}

func TestRunAssetsDependencies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/assets/asset1/dependencies") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodes":[
			{"uid":"n1","namespace":"prod","type":"ASSET"},
			{"uid":"n2","namespace":"dev","type":"FLOW"}
		],"edges":[{"source":"n1","target":"n2"}]}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runAssetsDependencies(newTestClient(t, server.URL), "asset1", false, false, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runAssetsDependencies error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"n1", "n2", "Nodes: 2, Edges: 1"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunAssetsDeleteByIds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/assets/by-ids") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"count":3}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runAssetsDeleteByIds(newTestClient(t, server.URL), []string{"a", "b", "c"}, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runAssetsDeleteByIds error: %v", err)
	}
	if !strings.Contains(buf.String(), "3 asset(s) affected") {
		t.Errorf("expected bulk count, got:\n%s", buf.String())
	}
}

func TestRunAssetsDeleteByQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/assets/by-query") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"count":5}`))
	}))
	t.Cleanup(server.Close)

	filters, err := parseQueryFilters("prod", nil)
	if err != nil {
		t.Fatalf("parseQueryFilters error: %v", err)
	}

	var buf bytes.Buffer
	err = runAssetsDeleteByQuery(newTestClient(t, server.URL), filters, false, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runAssetsDeleteByQuery error: %v", err)
	}
	if !strings.Contains(buf.String(), "5 asset(s) affected") {
		t.Errorf("expected bulk count, got:\n%s", buf.String())
	}
}

func TestAssetsDeleteByQueryCommand_NoFilter(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		t.Fatal("client should not be created without a filter")
		return nil, nil
	}
	defer func() { newClientFunc = original }()

	cmd := newAssetsDeleteByQueryCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no selection filter is provided")
	}
	if !strings.Contains(err.Error(), "selection filter is required") {
		t.Fatalf("expected filter-required error, got: %v", err)
	}
}

func TestRunAssetsLineageEventsList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/assets/lineage-events/search") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[
			{"uid":"e1","namespace":"prod","flowId":"f1","executionId":"x1","state":"SUCCESS"}
		],"total":1}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runAssetsLineageEventsList(newTestClient(t, server.URL), 1, 100, nil, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runAssetsLineageEventsList error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"e1", "prod", "f1", "SUCCESS", "Total lineage events: 1"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunAssetsUsagesList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/assets/usages/search") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[
			{"assetId":"a1","namespace":"prod","flowId":"f1","executionId":"x1","taskId":"t1"}
		],"total":1}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runAssetsUsagesList(newTestClient(t, server.URL), 1, 100, nil, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runAssetsUsagesList error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"a1", "prod", "f1", "t1", "Total usages: 1"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestAssetsLineageEventsDeleteByQuery_Command(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/assets/lineage-events/by-query") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"count":2}`))
	}))
	t.Cleanup(server.Close)

	original := newClientFunc
	newClientFunc = func() (*Client, error) { return newTestClient(t, server.URL), nil }
	defer func() { newClientFunc = original }()

	cmd := newAssetsLineageEventsDeleteByQueryCommand()
	out, err := executeCommand(cmd, "--namespace", "prod")
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if !strings.Contains(out, "2 asset(s) affected") {
		t.Errorf("expected bulk count, got:\n%s", out)
	}
}

func TestAssetsGetCommand_NoArgs(t *testing.T) {
	cmd := newAssetsGetCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestRunAssetsList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[
			{"id":"asset1","namespace":"prod","type":"TABLE","displayName":"My Table"},
			{"id":"asset2","namespace":"dev","type":"VIEW","displayName":"My View"}
		],"total":2}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runAssetsList(newTestClient(t, server.URL), 1, 100, nil, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runAssetsList error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"asset1", "prod", "TABLE", "My Table", "Total assets: 2"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunAssetsList_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"list failed"}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runAssetsList(newTestClient(t, server.URL), 1, 100, nil, newTableRenderer(&buf))
	if err == nil {
		t.Fatal("expected error from failing API")
	}
	if !strings.Contains(err.Error(), "list failed") {
		t.Errorf("expected formatted SDK error, got: %v", err)
	}
}

func TestRunAssetsGet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"asset1","namespace":"prod","type":"TABLE","displayName":"My Table","description":"A table","deleted":false}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runAssetsGet(newTestClient(t, server.URL), "asset1", newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runAssetsGet error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"asset1", "prod", "TABLE", "My Table", "A table"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestAssetsCreateCommand_NoFile(t *testing.T) {
	cmd := newAssetsCreateCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when --file is missing")
	}
	if !strings.Contains(err.Error(), "--file is required") {
		t.Fatalf("expected --file error, got: %v", err)
	}
}

func TestRunAssetsCreate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"asset1","namespace":"prod","type":"TABLE","displayName":"My Table"}`))
	}))
	t.Cleanup(server.Close)

	f, err := os.CreateTemp("", "asset-*.yml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	_, _ = f.WriteString("id: asset1\nnamespace: prod\ntype: TABLE")
	f.Close()

	var buf bytes.Buffer
	err = runAssetsCreate(newTestClient(t, server.URL), f.Name(), newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runAssetsCreate error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"asset1", "prod", "TABLE", "created"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunAssetsDelete_Cancelled(t *testing.T) {
	hit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runAssetsDelete(newTestClient(t, server.URL), "asset1", false, strings.NewReader("n\n"), newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runAssetsDelete error: %v", err)
	}
	if hit {
		t.Error("expected no API request when deletion is cancelled")
	}
	if !strings.Contains(buf.String(), "Cancelled") {
		t.Errorf("expected cancellation message, got:\n%s", buf.String())
	}
}

func TestRunAssetsDelete_Confirmed(t *testing.T) {
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
	err := runAssetsDelete(newTestClient(t, server.URL), "asset1", false, strings.NewReader("y\n"), newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runAssetsDelete error: %v", err)
	}
	if !hit {
		t.Error("expected API request when deletion is confirmed")
	}
	if !strings.Contains(buf.String(), "deleted") {
		t.Errorf("expected deletion message, got:\n%s", buf.String())
	}
}

func TestRunAssetsDelete_SkipConfirm(t *testing.T) {
	hit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runAssetsDelete(newTestClient(t, server.URL), "asset1", true, strings.NewReader(""), newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runAssetsDelete error: %v", err)
	}
	if !hit {
		t.Error("expected API request when --yes is set")
	}
}

func TestAssetsListCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newAssetsListCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}
