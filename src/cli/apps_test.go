package cli

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestAppsCommand_Structure(t *testing.T) {
	cmd := newAppsCommand()
	subNames := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subNames[sub.Name()] = true
	}
	for _, want := range []string{"list", "get", "deploy", "update", "delete", "enable", "disable", "export", "import"} {
		if !subNames[want] {
			t.Errorf("expected subcommand %q to exist", want)
		}
	}
}

func TestAppsGetCommand_NoArgs(t *testing.T) {
	cmd := newAppsGetCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestRunAppsList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[
			{"uid":"app-uid-1","id":"app1","name":"My App","namespace":"prod","enabled":true},
			{"uid":"app-uid-2","id":"app2","name":"Another App","namespace":"dev","enabled":false}
		],"total":2}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runAppsList(newTestClient(t, server.URL), "", "", "", nil, 1, 100, nil, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runAppsList error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"app-uid-1", "My App", "prod", "Another App", "Total apps: 2"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunAppsList_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"list failed"}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runAppsList(newTestClient(t, server.URL), "", "", "", nil, 1, 100, nil, newTableRenderer(&buf))
	if err == nil {
		t.Fatal("expected error from failing API")
	}
	if !strings.Contains(err.Error(), "list failed") {
		t.Errorf("expected formatted SDK error, got: %v", err)
	}
}

func TestRunAppsGet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"uid":"app-uid-1","name":"My App","namespace":"prod","disabled":false,"tags":["tag1","tag2"]}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runAppsGet(newTestClient(t, server.URL), "app-uid-1", newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runAppsGet error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"app-uid-1", "My App", "prod", "false", "tag1"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunAppsGet_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"not found"}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runAppsGet(newTestClient(t, server.URL), "missing-uid", newTableRenderer(&buf))
	if err == nil {
		t.Fatal("expected error from failing API")
	}
}

func TestAppsDeployCommand_NoFile(t *testing.T) {
	cmd := newAppsDeployCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when --file is missing")
	}
	if !strings.Contains(err.Error(), "--file is required") {
		t.Fatalf("expected --file error, got: %v", err)
	}
}

func TestRunAppsDeploy(t *testing.T) {
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body error: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"uid":"app-uid-1","name":"My App","namespace":"prod"}`))
	}))
	t.Cleanup(server.Close)

	f, err := os.CreateTemp("", "app-*.yml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	content := "uid: my-app\nname: My App"
	_, _ = f.WriteString(content)
	f.Close()

	var buf bytes.Buffer
	err = runAppsDeploy(newTestClient(t, server.URL), f.Name(), newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runAppsDeploy error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"app-uid-1", "My App", "deployed"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(string(gotBody), "My App") {
		t.Errorf("expected YAML body to be forwarded, got: %s", gotBody)
	}
}

func TestAppsUpdateCommand_NoFile(t *testing.T) {
	cmd := newAppsUpdateCommand()
	_, err := executeCommand(cmd, "some-uid")
	if err == nil {
		t.Fatal("expected error when --file is missing")
	}
	if !strings.Contains(err.Error(), "--file is required") {
		t.Fatalf("expected --file error, got: %v", err)
	}
}

func TestRunAppsUpdate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)

	f, err := os.CreateTemp("", "app-*.yml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	_, _ = f.WriteString("uid: app-uid-1\nname: Updated App")
	f.Close()

	var buf bytes.Buffer
	err = runAppsUpdate(newTestClient(t, server.URL), "app-uid-1", f.Name(), newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runAppsUpdate error: %v", err)
	}
	if !strings.Contains(buf.String(), "updated") {
		t.Errorf("expected updated message, got: %s", buf.String())
	}
}

func TestRunAppsDelete_Cancelled(t *testing.T) {
	hit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runAppsDelete(newTestClient(t, server.URL), "app-uid-1", false, strings.NewReader("n\n"), newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runAppsDelete error: %v", err)
	}
	if hit {
		t.Error("expected no API request when deletion is cancelled")
	}
	if !strings.Contains(buf.String(), "Cancelled") {
		t.Errorf("expected cancellation message, got:\n%s", buf.String())
	}
}

func TestRunAppsDelete_Confirmed(t *testing.T) {
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
	err := runAppsDelete(newTestClient(t, server.URL), "app-uid-1", false, strings.NewReader("y\n"), newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runAppsDelete error: %v", err)
	}
	if !hit {
		t.Error("expected API request when deletion is confirmed")
	}
	if !strings.Contains(buf.String(), "deleted") {
		t.Errorf("expected deletion message, got:\n%s", buf.String())
	}
}

func TestRunAppsDelete_SkipConfirm(t *testing.T) {
	hit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runAppsDelete(newTestClient(t, server.URL), "app-uid-1", true, strings.NewReader(""), newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runAppsDelete error: %v", err)
	}
	if !hit {
		t.Error("expected API request when --yes is set")
	}
}

func TestRunAppsToggle_Enable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"uid":"app-uid-1","name":"My App","enabled":true}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runAppsToggle(newTestClient(t, server.URL), "app-uid-1", true, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runAppsToggle error: %v", err)
	}
	if !strings.Contains(buf.String(), "enabled") {
		t.Errorf("expected enabled message, got:\n%s", buf.String())
	}
}

func TestRunAppsToggle_Disable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"uid":"app-uid-1","name":"My App","enabled":false}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runAppsToggle(newTestClient(t, server.URL), "app-uid-1", false, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runAppsToggle error: %v", err)
	}
	if !strings.Contains(buf.String(), "disabled") {
		t.Errorf("expected disabled message, got:\n%s", buf.String())
	}
}

func TestRunAppsExport_ToFile(t *testing.T) {
	zipData := []byte("PK\x03\x04fake-zip-content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(zipData)
	}))
	t.Cleanup(server.Close)

	tmpFile, err := os.CreateTemp("", "apps-export-*.zip")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	var buf bytes.Buffer
	cmd := newAppsExportCommand()
	cmd.SetOut(&buf)
	err = runAppsExport(newTestClient(t, server.URL), tmpFile.Name(), cmd)
	if err != nil {
		t.Fatalf("runAppsExport error: %v", err)
	}
	if !strings.Contains(buf.String(), "exported") {
		t.Errorf("expected export message, got:\n%s", buf.String())
	}
	written, _ := os.ReadFile(tmpFile.Name())
	if !bytes.Equal(written, zipData) {
		t.Errorf("file contents mismatch")
	}
}

func TestAppsImportCommand_NoFile(t *testing.T) {
	cmd := newAppsImportCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when --file is missing")
	}
	if !strings.Contains(err.Error(), "--file is required") {
		t.Fatalf("expected --file error, got: %v", err)
	}
}

func TestRunAppsImport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":["uid-1","uid-2"],"errors":[]}`))
	}))
	t.Cleanup(server.Close)

	// Create a dummy zip file for import
	f, err := os.CreateTemp("", "apps-*.zip")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	_, _ = f.Write([]byte("PK\x03\x04dummy"))
	f.Close()

	var buf bytes.Buffer
	err = runAppsImport(newTestClient(t, server.URL), f.Name(), newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runAppsImport error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "2 app(s) imported") {
		t.Errorf("expected import count, got:\n%s", out)
	}
}

func TestRunAppsImport_WithErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":["uid-1"],"errors":[{"source":"bad-app.yml","message":"invalid schema"}]}`))
	}))
	t.Cleanup(server.Close)

	f, err := os.CreateTemp("", "apps-*.zip")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	_, _ = f.Write([]byte("PK\x03\x04dummy"))
	f.Close()

	var buf bytes.Buffer
	err = runAppsImport(newTestClient(t, server.URL), f.Name(), newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runAppsImport error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "bad-app.yml") {
		t.Errorf("expected error source in output, got:\n%s", out)
	}
	if !strings.Contains(out, "invalid schema") {
		t.Errorf("expected error message in output, got:\n%s", out)
	}
}

func TestAppsListCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newAppsListCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}
