package cli

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestAssetsCommand_Structure(t *testing.T) {
	cmd := newAssetsCommand()
	subNames := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subNames[sub.Name()] = true
	}
	for _, want := range []string{"list", "get", "create", "delete"} {
		if !subNames[want] {
			t.Errorf("expected subcommand %q to exist", want)
		}
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
