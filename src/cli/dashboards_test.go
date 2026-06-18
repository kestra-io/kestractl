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

func TestDashboardsCommand_Structure(t *testing.T) {
	cmd := newDashboardsCommand()
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
