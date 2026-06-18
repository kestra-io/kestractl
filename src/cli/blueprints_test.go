package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestBlueprintsCommand_Structure(t *testing.T) {
	cmd := newBlueprintsCommand()
	subNames := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subNames[sub.Name()] = true
	}
	for _, want := range []string{"community", "flow"} {
		if !subNames[want] {
			t.Errorf("expected subcommand %q to exist", want)
		}
	}
}

func TestBlueprintsCommunityCommand_Structure(t *testing.T) {
	cmd := newBlueprintsCommunityCommand()
	subNames := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subNames[sub.Name()] = true
	}
	for _, want := range []string{"search", "get", "source"} {
		if !subNames[want] {
			t.Errorf("expected subcommand %q to exist", want)
		}
	}
}

func TestBlueprintsFlowCommand_Structure(t *testing.T) {
	cmd := newBlueprintsFlowCommand()
	subNames := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subNames[sub.Name()] = true
	}
	for _, want := range []string{"list", "get", "create", "update", "delete"} {
		if !subNames[want] {
			t.Errorf("expected subcommand %q to exist", want)
		}
	}
}

func TestRunBlueprintsCommunitySearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[
			{"id":"bp1","title":"HTTP Request","tags":["http","rest"]},
			{"id":"bp2","title":"Slack Notification","tags":["slack"]}
		],"total":2}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runBlueprintsCommunitySearch(newTestClient(t, server.URL), "FLOW", "", nil, 1, 25, nil, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runBlueprintsCommunitySearch error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"bp1", "HTTP Request", "http", "bp2", "Slack", "Total: 2"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunBlueprintsCommunitySearch_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"search failed"}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runBlueprintsCommunitySearch(newTestClient(t, server.URL), "FLOW", "", nil, 1, 25, nil, newTableRenderer(&buf))
	if err == nil {
		t.Fatal("expected error from failing API")
	}
	if !strings.Contains(err.Error(), "search failed") {
		t.Errorf("expected formatted SDK error, got: %v", err)
	}
}

func TestBlueprintsCommunityGetCommand_NoArgs(t *testing.T) {
	cmd := newBlueprintsCommunityGetCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestRunBlueprintsCommunityGet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"bp1","title":"HTTP Request","description":"Makes HTTP calls","tags":["http"]}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runBlueprintsCommunityGet(newTestClient(t, server.URL), "bp1", "FLOW", newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runBlueprintsCommunityGet error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"bp1", "HTTP Request", "Makes HTTP calls", "http"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunBlueprintsFlowList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[
			{"id":"fb1","title":"My ETL Blueprint","tags":["etl"],"deleted":false},
			{"id":"fb2","title":"Ingestion Blueprint","tags":[],"deleted":false}
		],"total":2}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runBlueprintsFlowList(newTestClient(t, server.URL), "", nil, 1, 100, nil, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runBlueprintsFlowList error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"fb1", "My ETL Blueprint", "fb2", "Ingestion", "Total: 2"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestBlueprintsFlowGetCommand_NoArgs(t *testing.T) {
	cmd := newBlueprintsFlowGetCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestRunBlueprintsFlowGet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"fb1","title":"My Blueprint","description":"A custom blueprint","tags":["etl"]}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runBlueprintsFlowGet(newTestClient(t, server.URL), "fb1", newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runBlueprintsFlowGet error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"fb1", "My Blueprint", "A custom blueprint", "etl"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestBlueprintsFlowCreateCommand_NoTitle(t *testing.T) {
	cmd := newBlueprintsFlowCreateCommand()
	_, err := executeCommand(cmd, "--source-file", "flow.yml")
	if err == nil {
		t.Fatal("expected error when --title is missing")
	}
	if !strings.Contains(err.Error(), "--title is required") {
		t.Fatalf("expected --title error, got: %v", err)
	}
}

func TestBlueprintsFlowCreateCommand_NoSourceFile(t *testing.T) {
	cmd := newBlueprintsFlowCreateCommand()
	_, err := executeCommand(cmd, "--title", "My Blueprint")
	if err == nil {
		t.Fatal("expected error when --source-file is missing")
	}
	if !strings.Contains(err.Error(), "--source-file is required") {
		t.Fatalf("expected --source-file error, got: %v", err)
	}
}

func TestRunBlueprintsFlowCreate(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"fb1","title":"My Blueprint"}`))
	}))
	t.Cleanup(server.Close)

	f, err := os.CreateTemp("", "flow-*.yml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	_, _ = f.WriteString("id: myflow\nnamespace: prod")
	f.Close()

	var buf bytes.Buffer
	err = runBlueprintsFlowCreate(newTestClient(t, server.URL), "My Blueprint", "", f.Name(), nil, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runBlueprintsFlowCreate error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"fb1", "My Blueprint", "created"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if gotBody["title"] != "My Blueprint" {
		t.Errorf("expected title in request body, got: %v", gotBody)
	}
}

func TestRunBlueprintsFlowDelete(t *testing.T) {
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
	err := runBlueprintsFlowDelete(newTestClient(t, server.URL), "fb1", newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runBlueprintsFlowDelete error: %v", err)
	}
	if !hit {
		t.Error("expected API request")
	}
	if !strings.Contains(buf.String(), "deleted") {
		t.Errorf("expected deletion message, got:\n%s", buf.String())
	}
}

func TestBlueprintsFlowListCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newBlueprintsFlowListCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}
