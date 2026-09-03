package cli

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	kestra "github.com/kestra-io/client-sdk/go-sdk/v2/kestra_api_client"
)

func newTestClient(t *testing.T, serverURL string) *Client {
	t.Helper()
	// Wired like newClientDefault: one HTTP client with the Kestra 1.x response
	// shim in front of both SDK clients.
	httpClient, compat := newCompatHTTPClient()
	cfg := kestra.NewConfiguration()
	cfg.Servers = kestra.ServerConfigurations{{URL: serverURL}}
	cfg.HTTPClient = httpClient
	c := &Client{
		API:    kestra.NewAPIClient(cfg),
		Kestra: kestra.NewClient(serverURL, kestra.WithHTTPClient(httpClient)),
		Ctx:    context.Background(),
		Tenant: "main",
	}
	compat.legacyServer = c.isLegacyServer
	return c
}

func TestNamespaceExists_OSSResponseMissingDeleted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"main"}`))
	}))
	t.Cleanup(server.Close)

	exists, err := namespaceExists(newTestClient(t, server.URL), "main")
	if err != nil {
		t.Fatalf("expected no error for 200 response, got: %v", err)
	}
	if !exists {
		t.Fatal("expected namespace to be reported as existing")
	}
}

func TestNamespaceExists_404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	exists, err := namespaceExists(newTestClient(t, server.URL), "missing")
	if err != nil {
		t.Fatalf("expected no error for 404 response, got: %v", err)
	}
	if exists {
		t.Fatal("expected namespace to be reported as missing")
	}
}

func TestNamespaceExists_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	exists, err := namespaceExists(newTestClient(t, server.URL), "main")
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
	if exists {
		t.Fatal("expected namespace to be reported as missing on server error")
	}
}

func TestNamespaceFilesCommand_Structure(t *testing.T) {
	cmd := newNamespaceFilesCommand()
	if cmd.Use != "nsfiles" {
		t.Fatalf("expected Use to be 'nsfiles', got '%s'", cmd.Use)
	}

	subcommands := map[string]bool{}
	for _, sub := range cmd.Commands() {
		subcommands[sub.Name()] = true
	}

	if !subcommands["list"] {
		t.Fatal("expected 'list' subcommand to be registered")
	}
	if !subcommands["get"] {
		t.Fatal("expected 'get' subcommand to be registered")
	}
	if !subcommands["upload"] {
		t.Fatal("expected 'upload' subcommand to be registered")
	}
	if !subcommands["delete"] {
		t.Fatal("expected 'delete' subcommand to be registered")
	}
}

func TestNamespaceFilesListCommand_Flags(t *testing.T) {
	cmd := newNamespaceFilesListCommand()

	flags := []string{"path", "recursive"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Fatalf("expected flag --%s to exist", flag)
		}
	}
}

func TestNamespaceFilesGetCommand_Flags(t *testing.T) {
	cmd := newNamespaceFilesGetCommand()

	flags := []string{"revision"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Fatalf("expected flag --%s to exist", flag)
		}
	}
}

func TestNamespaceFilesUploadCommand_Flags(t *testing.T) {
	cmd := newNamespaceFilesUploadCommand()

	flags := []string{"allow-missing-namespace", "override", "fail-fast", "no-root"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Fatalf("expected flag --%s to exist", flag)
		}
	}
}

func TestBuildNamespaceUploadItems(t *testing.T) {
	files := []localNamespaceUploadFile{
		{Path: "/tmp/scripts/a.py", Relative: "a.py", Size: 9},
		{Path: "/tmp/scripts/sub/c.py", Relative: "sub/c.py", Size: 12},
	}

	tests := []struct {
		name   string
		noRoot bool
		want   []string
	}{
		{
			name:   "default nests source directory name",
			noRoot: false,
			want:   []string{"resources/scripts/a.py", "resources/scripts/sub/c.py"},
		},
		{
			name:   "no-root uploads contents directly, preserving subdirs",
			noRoot: true,
			want:   []string{"resources/a.py", "resources/sub/c.py"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := buildNamespaceUploadItems(files, "/tmp/scripts", "resources", tt.noRoot)
			if len(items) != len(tt.want) {
				t.Fatalf("expected %d items, got %d", len(tt.want), len(items))
			}
			for i, item := range items {
				if item.Destination != tt.want[i] {
					t.Errorf("item %d: expected destination %q, got %q", i, tt.want[i], item.Destination)
				}
			}
		})
	}
}

func TestNamespaceFilesDeleteCommand_Flags(t *testing.T) {
	cmd := newNamespaceFilesDeleteCommand()

	flags := []string{"recursive", "force"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Fatalf("expected flag --%s to exist", flag)
		}
	}
}

func TestNamespaceFilesListCommand_Help(t *testing.T) {
	cmd := newNamespaceFilesListCommand()
	output, err := executeCommand(cmd, "--help")
	if err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}

	expected := []string{
		"list <namespace>",
		"--path",
		"--recursive",
	}
	for _, s := range expected {
		if !strings.Contains(output, s) {
			t.Fatalf("expected help to contain '%s', got: %s", s, output)
		}
	}
}

func TestNamespaceFilesGetCommand_Help(t *testing.T) {
	cmd := newNamespaceFilesGetCommand()
	output, err := executeCommand(cmd, "--help")
	if err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}

	expected := []string{
		"get <namespace> <path>",
		"--revision",
	}
	for _, s := range expected {
		if !strings.Contains(output, s) {
			t.Fatalf("expected help to contain '%s', got: %s", s, output)
		}
	}
}

func TestNamespaceFilesUploadCommand_Help(t *testing.T) {
	cmd := newNamespaceFilesUploadCommand()
	output, err := executeCommand(cmd, "--help")
	if err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}

	expected := []string{
		"upload <namespace> <local-path> <path>",
		"--allow-missing-namespace",
		"--override",
		"--fail-fast",
		"--no-root",
	}
	for _, s := range expected {
		if !strings.Contains(output, s) {
			t.Fatalf("expected help to contain '%s', got: %s", s, output)
		}
	}
}

func TestNamespaceFilesDeleteCommand_Help(t *testing.T) {
	cmd := newNamespaceFilesDeleteCommand()
	output, err := executeCommand(cmd, "--help")
	if err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}

	expected := []string{
		"delete <namespace> <path>",
		"--recursive",
		"--force",
	}
	for _, s := range expected {
		if !strings.Contains(output, s) {
			t.Fatalf("expected help to contain '%s', got: %s", s, output)
		}
	}
}

func TestNamespaceFilesRevisionsCommand_NoArgs(t *testing.T) {
	cmd := newNamespaceFilesRevisionsCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") && !strings.Contains(err.Error(), "required") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestNamespaceFilesRevisionsCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newNamespaceFilesRevisionsCommand()
	_, err := executeCommand(cmd, "my.namespace", "--path", "workflows/test.yml")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestNamespaceFilesMoveCommand_NoArgs(t *testing.T) {
	cmd := newNamespaceFilesMoveCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 3 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestNamespaceFilesMoveCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newNamespaceFilesMoveCommand()
	_, err := executeCommand(cmd, "my.namespace", "old.yml", "new.yml")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestNamespaceFilesSearchCommand_NoArgs(t *testing.T) {
	cmd := newNamespaceFilesSearchCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestNamespaceFilesSearchCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newNamespaceFilesSearchCommand()
	_, err := executeCommand(cmd, "my.namespace")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestNamespaceFilesExportCommand_NoArgs(t *testing.T) {
	cmd := newNamespaceFilesExportCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestNamespaceFilesExportCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newNamespaceFilesExportCommand()
	_, err := executeCommand(cmd, "my.namespace")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}
