package cli

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecretsCommand_Structure(t *testing.T) {
	cmd := newSecretsCommand()
	if cmd.Use != "secrets" {
		t.Fatalf("expected use 'secrets', got '%s'", cmd.Use)
	}

	expected := map[string]bool{
		"list":   false,
		"set":    false,
		"delete": false,
		"patch":  false,
	}
	for _, sub := range cmd.Commands() {
		if _, ok := expected[sub.Name()]; ok {
			expected[sub.Name()] = true
		}
	}
	for name, found := range expected {
		if !found {
			t.Fatalf("expected '%s' subcommand", name)
		}
	}
}

func TestSecretsListCommand_NoArgs(t *testing.T) {
	cmd := newSecretsListCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestSecretsListCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newSecretsListCommand()
	_, err := executeCommand(cmd, "my.namespace")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

// runSecretsList must hit the tenant-scoped /secrets search rather than the
// namespace-scoped /namespaces/{namespace}/secrets endpoint, which a Kestra EE
// server can 403 on even for a token valid for every other command (issue #172).
func TestRunSecretsList_UsesFilteredTenantEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/main/secrets" {
			t.Errorf("unexpected request path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("filters[namespace][EQUALS]"); got != "my.namespace" {
			t.Errorf("expected namespace filter 'my.namespace', got %q", got)
		}
		if got := r.URL.Query().Get("page"); got != "1" {
			t.Errorf("expected page '1', got %q", got)
		}
		if got := r.URL.Query().Get("size"); got != "50" {
			t.Errorf("expected size '50', got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"readOnly":false,"results":[{"key":"MY_KEY","namespace":"my.namespace","description":"a secret","tags":[]}],"total":1}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runSecretsList(newTestClient(t, server.URL), "my.namespace", 1, 50, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runSecretsList error: %v", err)
	}

	out := buf.String()
	for _, want := range []string{"MY_KEY", "my.namespace", "a secret", "Showing 1 secret(s) (page 1, total 1)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestRunSecretsList_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"readOnly":false,"results":[],"total":0}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runSecretsList(newTestClient(t, server.URL), "system", 1, 50, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runSecretsList error: %v", err)
	}
	if !strings.Contains(buf.String(), "Showing 0 secret(s) (page 1, total 0)") {
		t.Fatalf("expected empty summary line, got:\n%s", buf.String())
	}
}

func TestRunSecretsList_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"title":"Access denied","status":403,"detail":"Access denied"}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runSecretsList(newTestClient(t, server.URL), "system", 1, 50, newTableRenderer(&buf))
	if err == nil {
		t.Fatal("expected an error for a 403 response")
	}
	if !strings.Contains(err.Error(), "Access denied") {
		t.Fatalf("expected the server message in the error, got: %v", err)
	}
}

func TestSecretsSetCommand_NoArgs(t *testing.T) {
	cmd := newSecretsSetCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 3 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestSecretsSetCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newSecretsSetCommand()
	_, err := executeCommand(cmd, "my.namespace", "MY_KEY", "my-value")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestSecretsDeleteCommand_NoArgs(t *testing.T) {
	cmd := newSecretsDeleteCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 2 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestSecretsDeleteCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newSecretsDeleteCommand()
	_, err := executeCommand(cmd, "my.namespace", "MY_KEY")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestSecretsPatchCommand_NoArgs(t *testing.T) {
	cmd := newSecretsPatchCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 2 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestSecretsPatchCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newSecretsPatchCommand()
	_, err := executeCommand(cmd, "my.namespace", "MY_KEY")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}
