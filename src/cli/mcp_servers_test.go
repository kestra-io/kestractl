package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testMcpServerJSON = `{"id":"ops","description":"Operations flows","instructions":"Only call matching flows.",` +
	`"serverType":"PRIVATE","authType":"BASIC","disabled":false,"isDefault":false}`

// mcpServersServer answers GET with getBody and any other method with writeBody,
// recording every request so tests can check the read-then-write sequences.
func mcpServersServer(t *testing.T, getBody, writeBody string) (*httptest.Server, *[]recordedRequest) {
	t.Helper()
	var requests []recordedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		requests = append(requests, recordedRequest{method: r.Method, path: r.URL.Path, query: r.URL.RawQuery, body: string(raw)})
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(getBody))
			return
		}
		_, _ = w.Write([]byte(writeBody))
	}))
	t.Cleanup(server.Close)
	return server, &requests
}

func TestMcpServersCommand_Structure(t *testing.T) {
	cmd := newMcpServersCommand()
	subNames := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subNames[sub.Name()] = true
	}
	for _, want := range []string{"list", "get", "tools", "create", "update", "delete", "enable", "disable"} {
		if !subNames[want] {
			t.Errorf("expected subcommand %q to exist", want)
		}
	}
}

func TestMcpServersUpdateCommand_RequiresAFlag(t *testing.T) {
	_, err := executeCommand(newMcpServersUpdateCommand(), "ops")
	if err == nil || !strings.Contains(err.Error(), "nothing to update") {
		t.Fatalf("expected a 'nothing to update' error, got: %v", err)
	}
}

func TestMcpServers_RefusedOnALegacyServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/configs" {
			t.Errorf("a 1.x server must not be called for MCP servers, got %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"1.3.35"}`))
	}))
	t.Cleanup(server.Close)

	const want = "MCP servers is only available on Kestra 2.0 or later (the server runs 1.3.35)"
	client := newTestClient(t, server.URL)

	var buf bytes.Buffer
	description := "x"
	calls := map[string]func() error{
		"list":   func() error { return runMcpServersList(client, 1, 100, nil, newTableRenderer(&buf)) },
		"get":    func() error { return runMcpServersGet(client, "ops", newTableRenderer(&buf)) },
		"tools":  func() error { return runMcpServersTools(client, "ops", newTableRenderer(&buf)) },
		"create": func() error { return runMcpServersCreate(client, "ops", "", "", newTableRenderer(&buf)) },
		"update": func() error {
			return runMcpServersUpdate(client, "ops", mcpServerChanges{description: &description}, newTableRenderer(&buf))
		},
		"delete": func() error {
			return runMcpServersDelete(client, "ops", true, strings.NewReader(""), newTableRenderer(&buf))
		},
		"disable": func() error { return runMcpServersSetDisabled(client, "ops", true, newTableRenderer(&buf)) },
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

func TestRunMcpServersList(t *testing.T) {
	server, requests := mcpServersServer(t, `{"results":[
		{"id":"default","description":"Default MCP server","serverType":"PRIVATE","authType":"BASIC","disabled":false,"isDefault":true},
		{"id":"ops","serverType":"PRIVATE","authType":"BASIC","disabled":true,"isDefault":false}
	],"total":2}`, "")

	var buf bytes.Buffer
	if err := runMcpServersList(newTestClient(t, server.URL), 2, 20, nil, newTableRenderer(&buf)); err != nil {
		t.Fatalf("runMcpServersList error: %v", err)
	}

	last := (*requests)[len(*requests)-1]
	if want := "/api/v1/main/mcp-servers"; last.path != want {
		t.Errorf("path = %q, want %q", last.path, want)
	}
	for _, want := range []string{"page=2", "size=20"} {
		if !strings.Contains(last.query, want) {
			t.Errorf("query %q missing %q", last.query, want)
		}
	}

	out := buf.String()
	for _, want := range []string{"ID", "DEFAULT", "DISABLED", "default", "Default MCP server", "Total MCP servers: 2"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if !hasRowEndingWith(out, "ops", "-") {
		t.Errorf("expected a dash for the missing description:\n%s", out)
	}
}

func TestRunMcpServersList_JSONRendersAnEmptyArrayForNoResults(t *testing.T) {
	server, _ := mcpServersServer(t, `{"results":null,"total":0}`, "")

	var buf bytes.Buffer
	if err := runMcpServersList(newTestClient(t, server.URL), 1, 100, nil, newJSONRenderer(&buf)); err != nil {
		t.Fatalf("runMcpServersList error: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "[]" {
		t.Errorf("output = %q, want %q", got, "[]")
	}
}

func TestRunMcpServersTools(t *testing.T) {
	server, requests := mcpServersServer(t, `[
		{"toolName":"restart_service","triggerId":"mcp","namespace":"company.ops","flowId":"restart","disabled":false}
	]`, "")

	var buf bytes.Buffer
	if err := runMcpServersTools(newTestClient(t, server.URL), "default", newTableRenderer(&buf)); err != nil {
		t.Fatalf("runMcpServersTools error: %v", err)
	}

	last := (*requests)[len(*requests)-1]
	if want := "/api/v1/main/mcp-servers/default/tools"; last.path != want {
		t.Errorf("path = %q, want %q", last.path, want)
	}
	out := buf.String()
	for _, want := range []string{"restart_service", "company.ops", "restart", "mcp", "Total tools: 1"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunMcpServersCreate(t *testing.T) {
	server, requests := mcpServersServer(t, "", testMcpServerJSON)

	var buf bytes.Buffer
	if err := runMcpServersCreate(newTestClient(t, server.URL), "ops", "Operations flows", "Only call matching flows.", newTableRenderer(&buf)); err != nil {
		t.Fatalf("runMcpServersCreate error: %v", err)
	}

	last := (*requests)[len(*requests)-1]
	if last.method != http.MethodPost || last.path != "/api/v1/main/mcp-servers" {
		t.Errorf("request = %s %s, want POST /api/v1/main/mcp-servers", last.method, last.path)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(last.body), &sent); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if sent["id"] != "ops" || sent["description"] != "Operations flows" || sent["instructions"] != "Only call matching flows." {
		t.Errorf("unexpected body: %s", last.body)
	}
	if !strings.Contains(buf.String(), "MCP server 'ops' created.") {
		t.Errorf("unexpected output: %s", buf.String())
	}
}

// PUT replaces the whole server, so update must send back the fields the user
// did not change, including `disabled`.
func TestRunMcpServersUpdate_KeepsUnchangedFields(t *testing.T) {
	current := `{"id":"ops","description":"Operations flows","instructions":"Only call matching flows.",` +
		`"serverType":"PRIVATE","authType":"BASIC","disabled":true,"isDefault":false}`
	server, requests := mcpServersServer(t, current, current)

	description := "Operations and on-call flows"
	var buf bytes.Buffer
	if err := runMcpServersUpdate(newTestClient(t, server.URL), "ops", mcpServerChanges{description: &description}, newTableRenderer(&buf)); err != nil {
		t.Fatalf("runMcpServersUpdate error: %v", err)
	}

	last := (*requests)[len(*requests)-1]
	if last.method != http.MethodPut || last.path != "/api/v1/main/mcp-servers/ops" {
		t.Fatalf("request = %s %s, want PUT /api/v1/main/mcp-servers/ops", last.method, last.path)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(last.body), &sent); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if sent["description"] != description {
		t.Errorf("description = %v, want %q", sent["description"], description)
	}
	if sent["instructions"] != "Only call matching flows." {
		t.Errorf("instructions were not kept: %v", sent["instructions"])
	}
	if sent["disabled"] != true {
		t.Errorf("a disabled server must stay disabled after update, sent disabled=%v", sent["disabled"])
	}
}

func TestRunMcpServersSetDisabled(t *testing.T) {
	server, requests := mcpServersServer(t, testMcpServerJSON, testMcpServerJSON)

	var buf bytes.Buffer
	if err := runMcpServersSetDisabled(newTestClient(t, server.URL), "ops", true, newTableRenderer(&buf)); err != nil {
		t.Fatalf("runMcpServersSetDisabled error: %v", err)
	}

	last := (*requests)[len(*requests)-1]
	if last.method != http.MethodPut || last.path != "/api/v1/main/mcp-servers/ops" {
		t.Fatalf("request = %s %s, want PUT /api/v1/main/mcp-servers/ops", last.method, last.path)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(last.body), &sent); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if sent["disabled"] != true || sent["description"] != "Operations flows" {
		t.Errorf("unexpected body: %s", last.body)
	}
	if !strings.Contains(buf.String(), "MCP server 'ops' disabled.") {
		t.Errorf("unexpected output: %s", buf.String())
	}
}

// Disabling an already disabled server must not write anything: the API only
// has a toggle, so a blind write path would turn it back on.
func TestRunMcpServersSetDisabled_AlreadyInState(t *testing.T) {
	disabled := strings.Replace(testMcpServerJSON, `"disabled":false`, `"disabled":true`, 1)
	server, requests := mcpServersServer(t, disabled, disabled)

	var buf bytes.Buffer
	if err := runMcpServersSetDisabled(newTestClient(t, server.URL), "ops", true, newJSONRenderer(&buf)); err != nil {
		t.Fatalf("runMcpServersSetDisabled error: %v", err)
	}

	for _, r := range *requests {
		if r.method != http.MethodGet {
			t.Errorf("expected no write, got %s %s", r.method, r.path)
		}
	}
	var status map[string]any
	if err := json.Unmarshal(buf.Bytes(), &status); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, buf.String())
	}
	if status["changed"] != false || status["status"] != "disabled" {
		t.Errorf("unexpected status: %v", status)
	}
}

func TestRunMcpServersDelete_Cancelled(t *testing.T) {
	server, requests := mcpServersServer(t, "", "")

	var buf bytes.Buffer
	if err := runMcpServersDelete(newTestClient(t, server.URL), "ops", false, strings.NewReader("n\n"), newTableRenderer(&buf)); err != nil {
		t.Fatalf("runMcpServersDelete error: %v", err)
	}
	for _, r := range *requests {
		if r.method == http.MethodDelete {
			t.Errorf("expected no DELETE after declining, got %s", r.path)
		}
	}
	if !strings.Contains(buf.String(), "Cancelled.") {
		t.Errorf("unexpected output: %s", buf.String())
	}
}

func TestRunMcpServersDelete(t *testing.T) {
	server, requests := mcpServersServer(t, "", "")

	var buf bytes.Buffer
	if err := runMcpServersDelete(newTestClient(t, server.URL), "ops", true, strings.NewReader(""), newTableRenderer(&buf)); err != nil {
		t.Fatalf("runMcpServersDelete error: %v", err)
	}
	last := (*requests)[len(*requests)-1]
	if last.method != http.MethodDelete || last.path != "/api/v1/main/mcp-servers/ops" {
		t.Errorf("request = %s %s, want DELETE /api/v1/main/mcp-servers/ops", last.method, last.path)
	}
	if !strings.Contains(buf.String(), "MCP server 'ops' deleted.") {
		t.Errorf("unexpected output: %s", buf.String())
	}
}
