package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRolesCommand_Structure(t *testing.T) {
	cmd := newRolesCommand()
	if cmd.Use != "roles" {
		t.Fatalf("expected use 'roles', got %q", cmd.Use)
	}

	want := map[string]bool{
		"list": false, "get": false, "create": false, "update": false, "delete": false,
	}
	for _, sub := range cmd.Commands() {
		name := strings.Fields(sub.Use)[0]
		if _, ok := want[name]; ok {
			want[name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("missing subcommand %q", name)
		}
	}
}

func TestRolesCreateCommand_RequiredName(t *testing.T) {
	cmd := newRolesCreateCommand()
	if cmd.Flags().Lookup("name") == nil {
		t.Fatal("expected --name flag")
	}
	if cmd.Flags().Lookup("permission") == nil {
		t.Fatal("expected --permission flag")
	}
	if cmd.Flags().Lookup("permissions-file") == nil {
		t.Fatal("expected --permissions-file flag")
	}
	// name is marked required
	if _, err := executeCommand(cmd); err == nil {
		t.Fatal("expected error when --name is missing")
	}
}

func TestRolesDeleteCommand_HasYesFlag(t *testing.T) {
	cmd := newRolesDeleteCommand()
	f := cmd.Flags().Lookup("yes")
	if f == nil {
		t.Fatal("expected --yes flag")
	}
	if f.Shorthand != "y" {
		t.Fatalf("expected -y shorthand, got %q", f.Shorthand)
	}
}

func TestParsePermPair(t *testing.T) {
	cases := []struct {
		in       string
		wantKey  string
		wantLvls []string
		wantErr  bool
	}{
		{"FLOW:READ,CREATE", "FLOW", []string{"READ", "CREATE"}, false},
		{"flow:read", "FLOW", []string{"READ"}, false},
		{" execution : read , update ", "EXECUTION", []string{"READ", "UPDATE"}, false},
		{"FLOW", "", nil, true},
		{":READ", "", nil, true},
		{"FLOW:", "", nil, true},
		{"FLOW:,", "", nil, true},
	}
	for _, c := range cases {
		key, lvls, err := parsePermPair(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parsePermPair(%q): expected error, got none", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parsePermPair(%q): unexpected error: %v", c.in, err)
			continue
		}
		if key != c.wantKey {
			t.Errorf("parsePermPair(%q): key = %q, want %q", c.in, key, c.wantKey)
		}
		if !reflect.DeepEqual(lvls, c.wantLvls) {
			t.Errorf("parsePermPair(%q): levels = %v, want %v", c.in, lvls, c.wantLvls)
		}
	}
}

func TestParsePermissions_FlagAndFileConflict(t *testing.T) {
	m := roleMutation{permPairs: []string{"FLOW:READ"}, permFile: "perms.yaml"}
	if _, err := m.parsePermissions(); err == nil {
		t.Fatal("expected error when both --permission and --permissions-file are set")
	}
}

func TestParsePermissions_FromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "perms.yaml")
	content := "FLOW:\n  - READ\n  - CREATE\nEXECUTION:\n  - READ\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	m := roleMutation{permFile: path}
	perms, err := m.parsePermissions()
	if err != nil {
		t.Fatalf("parsePermissions error: %v", err)
	}
	if !reflect.DeepEqual(perms.FLOW, []string{"READ", "CREATE"}) {
		t.Errorf("FLOW = %v, want [READ CREATE]", perms.FLOW)
	}
	if !reflect.DeepEqual(perms.EXECUTION, []string{"READ"}) {
		t.Errorf("EXECUTION = %v, want [READ]", perms.EXECUTION)
	}
}

func TestRunRolesList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[
			{"id":"r1","name":"admin","isDefault":true,"isManaged":true},
			{"id":"r2","name":"editor","isDefault":false,"isManaged":false}
		],"total":2}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runRolesList(newTestClient(t, server.URL), "", 1, 100, nil, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runRolesList error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"admin", "editor", "Total roles: 2"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunRolesGet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"r1","name":"editor","description":"Editors",
			"isDefault":false,"permissions":{"FLOW":["READ","CREATE"],"EXECUTION":["READ"]}}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runRolesGet(newTestClient(t, server.URL), "r1", newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runRolesGet error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"editor", "Editors", "FLOW", "READ, CREATE", "EXECUTION", "Permissions:"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunRolesCreate_SendsBody(t *testing.T) {
	var gotBody map[string]any
	var gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"new","name":"editor"}`))
	}))
	t.Cleanup(server.Close)

	m := roleMutation{
		name:      "editor",
		permPairs: []string{"FLOW:READ,CREATE", "EXECUTION:READ"},
		nameSet:   true,
		permsSet:  true,
	}
	var buf bytes.Buffer
	if err := runRolesCreate(newTestClient(t, server.URL), m, newTableRenderer(&buf)); err != nil {
		t.Fatalf("runRolesCreate error: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if gotBody["name"] != "editor" {
		t.Errorf("expected name in body, got %v", gotBody["name"])
	}
	perms, ok := gotBody["permissions"].(map[string]any)
	if !ok {
		t.Fatalf("expected permissions object in body, got %v", gotBody["permissions"])
	}
	flow, ok := perms["FLOW"].([]any)
	if !ok || len(flow) != 2 {
		t.Errorf("expected FLOW with 2 levels, got %v", perms["FLOW"])
	}
}

func TestRunRolesCreate_RequiresPermissions(t *testing.T) {
	hit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	m := roleMutation{name: "editor", nameSet: true}
	var buf bytes.Buffer
	if err := runRolesCreate(newTestClient(t, server.URL), m, newTableRenderer(&buf)); err == nil {
		t.Fatal("expected error when no permissions are provided")
	}
	if hit {
		t.Error("expected no API request when permissions are missing")
	}
}

func TestRunRolesUpdate_OverlaysExisting(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"id":"r1","name":"editor","description":"orig",
				"permissions":{"FLOW":["READ"]}}`))
		case http.MethodPut:
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_, _ = w.Write([]byte(`{"id":"r1","name":"editor","description":"updated"}`))
		}
	}))
	t.Cleanup(server.Close)

	// Only change the description; name and permissions must be preserved from GET.
	m := roleMutation{description: "updated", descriptionSet: true}
	var buf bytes.Buffer
	if err := runRolesUpdate(newTestClient(t, server.URL), "r1", m, newTableRenderer(&buf)); err != nil {
		t.Fatalf("runRolesUpdate error: %v", err)
	}

	if gotBody["name"] != "editor" {
		t.Errorf("expected preserved name 'editor', got %v", gotBody["name"])
	}
	if gotBody["description"] != "updated" {
		t.Errorf("expected updated description, got %v", gotBody["description"])
	}
	perms, ok := gotBody["permissions"].(map[string]any)
	if !ok || perms["FLOW"] == nil {
		t.Errorf("expected preserved permissions, got %v", gotBody["permissions"])
	}
}

func TestRunRolesDelete_Confirmed(t *testing.T) {
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
	err := runRolesDelete(newTestClient(t, server.URL), "r1", false, strings.NewReader("y\n"), newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runRolesDelete error: %v", err)
	}
	if !hit {
		t.Error("expected API request when deletion is confirmed")
	}
	if !strings.Contains(buf.String(), "deleted") {
		t.Errorf("expected deletion message, got:\n%s", buf.String())
	}
}

func TestRunRolesDelete_CancelMakesNoRequest(t *testing.T) {
	hit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runRolesDelete(newTestClient(t, server.URL), "r1", false, strings.NewReader("n\n"), newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runRolesDelete error: %v", err)
	}
	if hit {
		t.Error("expected no API request when deletion is cancelled")
	}
	if !strings.Contains(buf.String(), "Cancelled") {
		t.Errorf("expected cancellation message, got:\n%s", buf.String())
	}
}

func TestRolesAutocompleteCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newRolesAutocompleteCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestRunRolesAutocomplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"r1","name":"admin"},{"id":"r2","name":"editor"}]`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runRolesAutocomplete(newTestClient(t, server.URL), "", newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runRolesAutocomplete error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"r1", "admin", "editor", "Showing 2 role(s)"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRolesListFromIdsCommand_NoArgs(t *testing.T) {
	cmd := newRolesListFromIdsCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
}

func TestRolesListFromIdsCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newRolesListFromIdsCommand()
	_, err := executeCommand(cmd, "r1")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestRunRolesListFromIds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"r1","name":"admin","isDefault":true,"isManaged":true,"deleted":false},{"id":"r2","name":"editor","isDefault":false,"isManaged":false,"deleted":false}]`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runRolesListFromIds(newTestClient(t, server.URL), []string{"r1", "r2"}, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runRolesListFromIds error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"r1", "admin", "editor", "Showing 2 role(s)"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}
