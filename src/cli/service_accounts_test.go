package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServiceAccountsCommand_Structure(t *testing.T) {
	cmd := newServiceAccountsCommand()
	if cmd.Use != "service-accounts" {
		t.Fatalf("expected use 'service-accounts', got %q", cmd.Use)
	}

	want := map[string]bool{
		"list": false, "get": false, "create": false, "update": false, "delete": false, "tokens": false,
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

func TestServiceAccountsTokensCommand_Structure(t *testing.T) {
	cmd := newServiceAccountsTokensCommand()
	want := map[string]bool{"list": false, "create": false, "delete": false}
	for _, sub := range cmd.Commands() {
		name := strings.Fields(sub.Use)[0]
		if _, ok := want[name]; ok {
			want[name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("missing tokens subcommand %q", name)
		}
	}
}

func TestServiceAccountsCreateCommand_RequiredName(t *testing.T) {
	cmd := newServiceAccountsCreateCommand()
	if cmd.Flags().Lookup("name") == nil {
		t.Fatal("expected --name flag")
	}
	if _, err := executeCommand(cmd); err == nil {
		t.Fatal("expected error when --name is missing")
	}
}

func TestServiceAccountsDeleteCommand_HasYesFlag(t *testing.T) {
	cmd := newServiceAccountsDeleteCommand()
	f := cmd.Flags().Lookup("yes")
	if f == nil {
		t.Fatal("expected --yes flag")
	}
	if f.Shorthand != "y" {
		t.Fatalf("expected -y shorthand, got %q", f.Shorthand)
	}
}

func TestRunServiceAccountsList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[
			{"id":"sa1","name":"ci-bot","instanceOwner":false,"tenants":[{"id":"main"}]},
			{"id":"sa2","name":"ops-bot","instanceOwner":true}
		],"total":2}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	if err := runServiceAccountsList(newTestClient(t, server.URL), 1, 100, nil, newTableRenderer(&buf)); err != nil {
		t.Fatalf("runServiceAccountsList error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"ci-bot", "ops-bot", "main", "Total service accounts: 2"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunServiceAccountsGet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"sa1","name":"ci-bot","description":"CI pipeline",
			"instanceOwner":false,"tenants":[{"id":"main"},{"id":"dev"}]}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	if err := runServiceAccountsGet(newTestClient(t, server.URL), "sa1", newTableRenderer(&buf)); err != nil {
		t.Fatalf("runServiceAccountsGet error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"ci-bot", "CI pipeline", "main, dev"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunServiceAccountsCreate_SendsBody(t *testing.T) {
	var gotBody map[string]any
	var gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"new","name":"ci-bot"}`))
	}))
	t.Cleanup(server.Close)

	m := serviceAccountMutation{
		name:           "ci-bot",
		description:    "CI pipeline",
		tenants:        []string{"main"},
		nameSet:        true,
		descriptionSet: true,
		tenantsSet:     true,
	}
	var buf bytes.Buffer
	if err := runServiceAccountsCreate(newTestClient(t, server.URL), m, newTableRenderer(&buf)); err != nil {
		t.Fatalf("runServiceAccountsCreate error: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if gotBody["name"] != "ci-bot" {
		t.Errorf("expected name in body, got %v", gotBody["name"])
	}
	if gotBody["description"] != "CI pipeline" {
		t.Errorf("expected description in body, got %v", gotBody["description"])
	}
	tenants, ok := gotBody["tenants"].([]any)
	if !ok || len(tenants) != 1 || tenants[0] != "main" {
		t.Errorf("expected tenants [main], got %v", gotBody["tenants"])
	}
}

func TestRunServiceAccountsUpdate_FillsNameFromCurrent(t *testing.T) {
	var gotBody map[string]any
	var gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"id":"sa1","name":"ci-bot","description":"old"}`))
		default:
			gotMethod = r.Method
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_, _ = w.Write([]byte(`{"id":"sa1","name":"ci-bot","description":"new"}`))
		}
	}))
	t.Cleanup(server.Close)

	// Only --description is set; name must be backfilled from the GET.
	m := serviceAccountMutation{description: "new", descriptionSet: true}
	var buf bytes.Buffer
	if err := runServiceAccountsUpdate(newTestClient(t, server.URL), "sa1", m, newTableRenderer(&buf)); err != nil {
		t.Fatalf("runServiceAccountsUpdate error: %v", err)
	}

	if gotMethod != http.MethodPatch {
		t.Errorf("expected PATCH, got %s", gotMethod)
	}
	if gotBody["name"] != "ci-bot" {
		t.Errorf("expected name backfilled to 'ci-bot', got %v", gotBody["name"])
	}
	if gotBody["description"] != "new" {
		t.Errorf("expected description 'new', got %v", gotBody["description"])
	}
}

func TestRunServiceAccountsUpdate_NothingToUpdate(t *testing.T) {
	hit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runServiceAccountsUpdate(newTestClient(t, server.URL), "sa1", serviceAccountMutation{}, newTableRenderer(&buf))
	if err == nil {
		t.Fatal("expected error when no fields are set")
	}
	if hit {
		t.Error("expected no HTTP call when nothing to update")
	}
}

func TestRunServiceAccountsDelete_Confirmed(t *testing.T) {
	var gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	if err := runServiceAccountsDelete(newTestClient(t, server.URL), "sa1", true, strings.NewReader(""), newTableRenderer(&buf)); err != nil {
		t.Fatalf("runServiceAccountsDelete error: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("expected DELETE, got %s", gotMethod)
	}
	if !strings.Contains(buf.String(), "deleted") {
		t.Errorf("expected deletion message, got %q", buf.String())
	}
}

func TestRunServiceAccountsTokensList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"total":1,"results":[
			{"id":"tok1","name":"deploy","prefix":"abc","expired":false}
		]}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	if err := runServiceAccountsTokensList(newTestClient(t, server.URL), "sa1", newTableRenderer(&buf)); err != nil {
		t.Fatalf("runServiceAccountsTokensList error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"tok1", "deploy", "abc", "Total tokens: 1"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunServiceAccountsTokensCreate_ShowsFullToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"tok1","name":"deploy","fullToken":"secret-token-value"}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	if err := runServiceAccountsTokensCreate(newTestClient(t, server.URL), "sa1", "deploy", "", "", false, newTableRenderer(&buf)); err != nil {
		t.Fatalf("runServiceAccountsTokensCreate error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"deploy", "secret-token-value", "will not be shown again"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunServiceAccountsTokensDelete(t *testing.T) {
	var gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	if err := runServiceAccountsTokensDelete(newTestClient(t, server.URL), "sa1", "tok1", newTableRenderer(&buf)); err != nil {
		t.Fatalf("runServiceAccountsTokensDelete error: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("expected DELETE, got %s", gotMethod)
	}
	if !strings.Contains(buf.String(), "deleted") {
		t.Errorf("expected deletion message, got %q", buf.String())
	}
}

func TestServiceAccountsSetSuperAdminCommand_NoArgs(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	cmd := newServiceAccountsSetSuperAdminCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestRunServiceAccountsSetSuperAdmin_Grant(t *testing.T) {
	hit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		if r.Method != http.MethodPatch {
			http.Error(w, "wrong method", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runServiceAccountsSetSuperAdmin(newTestClient(t, server.URL), "sa1", true, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runServiceAccountsSetSuperAdmin error: %v", err)
	}
	if !hit {
		t.Error("expected PATCH request to be made")
	}
	if !strings.Contains(buf.String(), "granted") {
		t.Errorf("expected 'granted' message, got:\n%s", buf.String())
	}
}

func TestRunServiceAccountsSetSuperAdmin_Revoke(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runServiceAccountsSetSuperAdmin(newTestClient(t, server.URL), "sa1", false, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runServiceAccountsSetSuperAdmin error: %v", err)
	}
	if !strings.Contains(buf.String(), "revoked") {
		t.Errorf("expected 'revoked' message, got:\n%s", buf.String())
	}
}
