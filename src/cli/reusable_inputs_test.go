package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

const testReusableInputsYAML = `id: my-block
namespace: my.namespace
description: Shared inputs
inputs:
  - id: name
    type: STRING
`

// recordedRequest captures what the CLI actually sent.
type recordedRequest struct {
	method string
	path   string
	query  string
	body   string
}

func reusableInputsServer(t *testing.T, status int, body string) (*httptest.Server, *recordedRequest) {
	t.Helper()
	recorded := &recordedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		recorded.method, recorded.path, recorded.query, recorded.body = r.Method, r.URL.Path, r.URL.RawQuery, string(raw)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server, recorded
}

// hasRowEndingWith reports whether the tabwriter row starting with prefix ends
// with suffix, ignoring the padding tabwriter inserted between columns.
func hasRowEndingWith(out, prefix, suffix string) bool {
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, prefix) && strings.HasSuffix(strings.TrimSpace(line), suffix) {
			return true
		}
	}
	return false
}

func TestReusableInputsCommand_Structure(t *testing.T) {
	cmd := newReusableInputsCommand()
	subNames := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subNames[sub.Name()] = true
	}
	for _, want := range []string{"list", "get", "revisions", "create", "update", "delete", "namespaces"} {
		if !subNames[want] {
			t.Errorf("expected subcommand %q to exist", want)
		}
	}
}

func TestReusableInputsGetCommand_WrongArgCount(t *testing.T) {
	cmd := newReusableInputsGetCommand()
	_, err := executeCommand(cmd, "only-namespace")
	if err == nil {
		t.Fatal("expected error when a single arg is provided")
	}
	if !strings.Contains(err.Error(), "accepts 2 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestReusableInputsSaveCommands_MissingFileFlag(t *testing.T) {
	for name, newCmd := range map[string]func() *cobra.Command{
		"create": newReusableInputsCreateCommand,
		"update": newReusableInputsUpdateCommand,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := executeCommand(newCmd(), "my.namespace", "my-block")
			if err == nil {
				t.Fatal("expected error when --file is missing")
			}
			if !strings.Contains(err.Error(), "--file is required") {
				t.Fatalf("expected --file error, got: %v", err)
			}
		})
	}
}

func TestReusableInputs_RefusedOnALegacyServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/configs" {
			t.Errorf("a 1.x server must not be called for reusable inputs, got %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"1.3.35"}`))
	}))
	t.Cleanup(server.Close)

	const want = "reusable inputs is only available on Kestra 2.0 or later (the server runs 1.3.35)"
	client := newTestClient(t, server.URL)

	var buf bytes.Buffer
	calls := map[string]func() error{
		"list": func() error { return runReusableInputsList(client, "my.namespace", 1, 100, newTableRenderer(&buf)) },
		"get": func() error {
			return runReusableInputsGet(client, "my.namespace", "my-block", nil, newTableRenderer(&buf))
		},
		"revisions": func() error {
			return runReusableInputsRevisions(client, "my.namespace", "my-block", newTableRenderer(&buf))
		},
		"save": func() error {
			return runReusableInputsSave(client, "my.namespace", "my-block", "unused.yaml", true, newTableRenderer(&buf))
		},
		"namespaces": func() error { return runReusableInputsNamespaces(client, newTableRenderer(&buf)) },
		"delete": func() error {
			return runReusableInputsDelete(client, "my.namespace", "my-block", false, strings.NewReader("y\n"), newTableRenderer(&buf))
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			err := call()
			if err == nil || err.Error() != want {
				t.Fatalf("got %v, want %q", err, want)
			}
		})
	}
}

func TestRunReusableInputsList(t *testing.T) {
	server, recorded := reusableInputsServer(t, http.StatusOK, `{"results":[
		{"id":"child-block","namespace":"my.namespace","revision":2,"inputs":[{"id":"name","type":"STRING"}],"description":"Shared inputs"},
		{"id":"parent-block","namespace":"my","revision":1,"inputs":[]}
	],"total":2}`)

	var buf bytes.Buffer
	if err := runReusableInputsList(newTestClient(t, server.URL), "my.namespace", 2, 20, newTableRenderer(&buf)); err != nil {
		t.Fatalf("runReusableInputsList error: %v", err)
	}

	if want := "/api/v1/main/namespaces/my.namespace/reusable-inputs"; recorded.path != want {
		t.Errorf("path = %q, want %q", recorded.path, want)
	}
	for _, want := range []string{"page=2", "size=20"} {
		if !strings.Contains(recorded.query, want) {
			t.Errorf("query %q missing %q", recorded.query, want)
		}
	}

	out := buf.String()
	// The parent-owned block is listed with its own namespace: inheritance is resolved server-side.
	for _, want := range []string{"child-block", "parent-block", "my.namespace", "Shared inputs", "Total reusable inputs blocks: 2"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if !hasRowEndingWith(out, "parent-block", "-") {
		t.Errorf("expected a dash for the missing description:\n%s", out)
	}
}

func TestRunReusableInputsList_JSONRendersAnEmptyArrayForNoResults(t *testing.T) {
	server, _ := reusableInputsServer(t, http.StatusOK, `{"results":null,"total":0}`)

	var buf bytes.Buffer
	if err := runReusableInputsList(newTestClient(t, server.URL), "my.namespace", 1, 100, newJSONRenderer(&buf)); err != nil {
		t.Fatalf("runReusableInputsList error: %v", err)
	}

	// A null `results` must not surface as a JSON null.
	if got := strings.TrimSpace(buf.String()); got != "[]" {
		t.Errorf("output = %q, want %q", got, "[]")
	}
}

func TestRunReusableInputsList_APIError(t *testing.T) {
	server, _ := reusableInputsServer(t, http.StatusInternalServerError, `{"message":"list failed"}`)

	var buf bytes.Buffer
	err := runReusableInputsList(newTestClient(t, server.URL), "my.namespace", 1, 100, newTableRenderer(&buf))
	if err == nil {
		t.Fatal("expected error from failing API")
	}
	if !strings.Contains(err.Error(), "list failed") {
		t.Errorf("expected formatted SDK error, got: %v", err)
	}
}

func TestRunReusableInputsGet_PrintsSource(t *testing.T) {
	server, recorded := reusableInputsServer(t, http.StatusOK,
		`{"id":"my-block","namespace":"my.namespace","revision":2,"inputs":[],"source":"id: my-block\nnamespace: my.namespace\n"}`)

	var buf bytes.Buffer
	if err := runReusableInputsGet(newTestClient(t, server.URL), "my.namespace", "my-block", nil, newTableRenderer(&buf)); err != nil {
		t.Fatalf("runReusableInputsGet error: %v", err)
	}

	if recorded.query != "" {
		t.Errorf("expected no revision query when unset, got %q", recorded.query)
	}
	if got, want := buf.String(), "id: my-block\nnamespace: my.namespace\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestRunReusableInputsGet_PinsRevision(t *testing.T) {
	server, recorded := reusableInputsServer(t, http.StatusOK,
		`{"id":"my-block","namespace":"my.namespace","revision":1,"last":false,"inputs":[],"source":"id: my-block\n"}`)

	revision := 1
	var buf bytes.Buffer
	if err := runReusableInputsGet(newTestClient(t, server.URL), "my.namespace", "my-block", &revision, newJSONRenderer(&buf)); err != nil {
		t.Fatalf("runReusableInputsGet error: %v", err)
	}

	if want := "revision=1"; !strings.Contains(recorded.query, want) {
		t.Errorf("query %q missing %q", recorded.query, want)
	}

	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("json output not decodable: %v", err)
	}
	if decoded["last"] != false {
		t.Errorf("expected last=false on a pinned revision, got %v", decoded["last"])
	}
}

func TestRunReusableInputsRevisions(t *testing.T) {
	server, recorded := reusableInputsServer(t, http.StatusOK, `[
		{"id":"my-block","namespace":"my.namespace","revision":1,"last":false,"inputs":[],"updated":"2026-09-11T10:00:00Z"},
		{"id":"my-block","namespace":"my.namespace","revision":2,"last":true,"inputs":[],"description":"Shared inputs"}
	]`)

	var buf bytes.Buffer
	if err := runReusableInputsRevisions(newTestClient(t, server.URL), "my.namespace", "my-block", newTableRenderer(&buf)); err != nil {
		t.Fatalf("runReusableInputsRevisions error: %v", err)
	}

	if want := "/api/v1/main/namespaces/my.namespace/reusable-inputs/my-block/revisions"; recorded.path != want {
		t.Errorf("path = %q, want %q", recorded.path, want)
	}

	out := buf.String()
	for _, want := range []string{"REVISION", "2026-09-11T10:00:00Z", "false", "true", "Total revisions: 2"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunReusableInputsSave_CreateSendsSourceVerbatim(t *testing.T) {
	server, recorded := reusableInputsServer(t, http.StatusOK,
		`{"id":"my-block","namespace":"my.namespace","revision":1,"inputs":[]}`)

	path := filepath.Join(t.TempDir(), "block.yaml")
	if err := os.WriteFile(path, []byte(testReusableInputsYAML), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	var buf bytes.Buffer
	if err := runReusableInputsSave(newTestClient(t, server.URL), "my.namespace", "my-block", path, true, newTableRenderer(&buf)); err != nil {
		t.Fatalf("runReusableInputsSave error: %v", err)
	}

	if recorded.method != http.MethodPut {
		t.Errorf("method = %q, want PUT", recorded.method)
	}
	if want := "/api/v1/main/namespaces/my.namespace/reusable-inputs/my-block"; recorded.path != want {
		t.Errorf("path = %q, want %q", recorded.path, want)
	}
	if want := "failIfExists=true"; !strings.Contains(recorded.query, want) {
		t.Errorf("query %q missing %q", recorded.query, want)
	}
	// Byte-for-byte: the server stores the request body as the block's source.
	if recorded.body != testReusableInputsYAML {
		t.Errorf("body = %q, want %q", recorded.body, testReusableInputsYAML)
	}
	if !strings.Contains(buf.String(), "Revision:") {
		t.Errorf("output missing the saved revision:\n%s", buf.String())
	}
}

func TestRunReusableInputsSave_UpdateDoesNotFailIfExists(t *testing.T) {
	server, recorded := reusableInputsServer(t, http.StatusOK,
		`{"id":"my-block","namespace":"my.namespace","revision":2,"inputs":[]}`)

	path := filepath.Join(t.TempDir(), "block.yaml")
	if err := os.WriteFile(path, []byte(testReusableInputsYAML), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	var buf bytes.Buffer
	if err := runReusableInputsSave(newTestClient(t, server.URL), "my.namespace", "my-block", path, false, newTableRenderer(&buf)); err != nil {
		t.Fatalf("runReusableInputsSave error: %v", err)
	}

	if want := "failIfExists=false"; !strings.Contains(recorded.query, want) {
		t.Errorf("query %q missing %q", recorded.query, want)
	}
}

func TestRunReusableInputsSave_MissingFile(t *testing.T) {
	server, _ := reusableInputsServer(t, http.StatusOK, `{}`)

	var buf bytes.Buffer
	err := runReusableInputsSave(newTestClient(t, server.URL), "my.namespace", "my-block",
		filepath.Join(t.TempDir(), "absent.yaml"), true, newTableRenderer(&buf))
	if err == nil {
		t.Fatal("expected an error for a missing file")
	}
	if !strings.Contains(err.Error(), "failed to read file") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunReusableInputsSave_Conflict(t *testing.T) {
	server, _ := reusableInputsServer(t, http.StatusConflict, `{"message":"reusable inputs block already exists"}`)

	path := filepath.Join(t.TempDir(), "block.yaml")
	if err := os.WriteFile(path, []byte(testReusableInputsYAML), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	var buf bytes.Buffer
	err := runReusableInputsSave(newTestClient(t, server.URL), "my.namespace", "my-block", path, true, newTableRenderer(&buf))
	if err == nil {
		t.Fatal("expected a conflict error")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunReusableInputsDelete(t *testing.T) {
	server, recorded := reusableInputsServer(t, http.StatusOK, ``)

	var buf bytes.Buffer
	if err := runReusableInputsDelete(newTestClient(t, server.URL), "my.namespace", "my-block", true, strings.NewReader(""), newTableRenderer(&buf)); err != nil {
		t.Fatalf("runReusableInputsDelete error: %v", err)
	}

	if recorded.method != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", recorded.method)
	}
	if want := "/api/v1/main/namespaces/my.namespace/reusable-inputs/my-block"; recorded.path != want {
		t.Errorf("path = %q, want %q", recorded.path, want)
	}
	if !strings.Contains(buf.String(), "deleted") {
		t.Errorf("output missing the deletion message:\n%s", buf.String())
	}
}

func TestRunReusableInputsDelete_CancelledPromptSkipsTheCall(t *testing.T) {
	// Only DELETEs are counted: the Kestra 2.0 guard probes the configuration
	// endpoint before the prompt, so any-request counting would always trip.
	deleted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleted = true
		}
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	if err := runReusableInputsDelete(newTestClient(t, server.URL), "my.namespace", "my-block", false, strings.NewReader("n\n"), newTableRenderer(&buf)); err != nil {
		t.Fatalf("runReusableInputsDelete error: %v", err)
	}
	if deleted {
		t.Error("the API must not be called when the prompt is declined")
	}
	if !strings.Contains(buf.String(), "Cancelled.") {
		t.Errorf("output missing the cancellation message:\n%s", buf.String())
	}
}

func TestRunReusableInputsNamespaces(t *testing.T) {
	server, recorded := reusableInputsServer(t, http.StatusOK, `["my","my.namespace"]`)

	var buf bytes.Buffer
	if err := runReusableInputsNamespaces(newTestClient(t, server.URL), newTableRenderer(&buf)); err != nil {
		t.Fatalf("runReusableInputsNamespaces error: %v", err)
	}

	if want := "/api/v1/main/reusable-inputs/namespaces"; recorded.path != want {
		t.Errorf("path = %q, want %q", recorded.path, want)
	}

	out := buf.String()
	for _, want := range []string{"NAMESPACE", "my.namespace", "Total namespaces: 2"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}
