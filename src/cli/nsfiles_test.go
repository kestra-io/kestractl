package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
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
	compat.era = c.serverEra
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

// Regression test for #130: `delete /qa/sub --recursive` used to DELETE each
// enumerated child before the directory itself, which wiped /qa. The server
// prunes a directory once its last child goes, and a DELETE on a path that no
// longer exists deletes the parent. One slash-prefixed DELETE on the directory
// is the only safe request.
func TestRunNamespaceFilesDelete_RecursiveIssuesOneDirectoryDelete(t *testing.T) {
	var deletedPaths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Query().Get("path")
		switch {
		case r.Method == http.MethodDelete:
			deletedPaths = append(deletedPaths, path)
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/files/stats"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"fileName":"sub","type":"Directory","size":0}`))
		case strings.HasSuffix(r.URL.Path, "/files/directory"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"fileName":"nested.py","type":"File","size":3}]`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	var out bytes.Buffer
	renderer, err := NewRenderer("json", &out)
	if err != nil {
		t.Fatalf("NewRenderer() returned error: %v", err)
	}

	if err := runNamespaceFilesDelete(newTestClient(t, server.URL), "qa.ns", "/qa/sub", true, false, renderer); err != nil {
		t.Fatalf("runNamespaceFilesDelete() returned error: %v", err)
	}

	if len(deletedPaths) != 1 || deletedPaths[0] != "/qa/sub" {
		t.Fatalf("expected exactly one DELETE for [/qa/sub], got %v", deletedPaths)
	}

	// The single request removes the subtree, but the report still names every
	// path that went away.
	for _, reported := range []string{"qa/sub/nested.py", "qa/sub"} {
		if !strings.Contains(out.String(), reported) {
			t.Errorf("expected %q in the delete report, got: %s", reported, out.String())
		}
	}
}

// A recursive delete is one request, so a failure is one failure — however many
// paths the report names.
func TestRunNamespaceFilesDelete_RecursiveFailureCountsOnce(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusInternalServerError)
		case strings.HasSuffix(r.URL.Path, "/files/stats"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"fileName":"sub","type":"Directory","size":0}`))
		case strings.HasSuffix(r.URL.Path, "/files/directory"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"fileName":"a.py","type":"File","size":3},{"fileName":"b.py","type":"File","size":3}]`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	var out bytes.Buffer
	renderer, err := NewRenderer("json", &out)
	if err != nil {
		t.Fatalf("NewRenderer() returned error: %v", err)
	}

	err = runNamespaceFilesDelete(newTestClient(t, server.URL), "qa.ns", "/qa/sub", true, false, renderer)
	if err == nil {
		t.Fatal("expected an error when the DELETE fails")
	}
	if !strings.Contains(err.Error(), "1 error(s)") {
		t.Errorf("expected the summary to report 1 error, got: %v", err)
	}

	// Every reported row still carries the failure, so the report stays useful.
	var summary namespaceFileDeleteSummary
	if err := json.Unmarshal(out.Bytes(), &summary); err != nil {
		t.Fatalf("could not decode the delete report: %v (%s)", err, out.String())
	}
	if len(summary.Results) != 3 {
		t.Fatalf("expected 3 reported rows, got %d: %s", len(summary.Results), out.String())
	}
	for _, result := range summary.Results {
		if result.Success {
			t.Errorf("expected row %q to be marked failed", result.Path)
		}
		if result.Error == "" {
			t.Errorf("expected row %q to carry the error", result.Path)
		}
	}
}

func TestRunNamespaceFilesDelete_SingleFileSendsSlashPrefixedPath(t *testing.T) {
	var deletedPaths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete:
			deletedPaths = append(deletedPaths, r.URL.Query().Get("path"))
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/files/stats"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"fileName":"hello.txt","type":"File","size":3}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	renderer, err := NewRenderer("json", io.Discard)
	if err != nil {
		t.Fatalf("NewRenderer() returned error: %v", err)
	}

	if err := runNamespaceFilesDelete(newTestClient(t, server.URL), "qa.ns", "qa/hello.txt", false, false, renderer); err != nil {
		t.Fatalf("runNamespaceFilesDelete() returned error: %v", err)
	}

	if len(deletedPaths) != 1 || deletedPaths[0] != "/qa/hello.txt" {
		t.Fatalf("expected DELETE path [/qa/hello.txt], got %v", deletedPaths)
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
