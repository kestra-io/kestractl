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

func newTableRenderer(out io.Writer) *Renderer {
	r, _ := NewRenderer(OutputTable, out)
	return r
}

func newJSONRenderer(out io.Writer) *Renderer {
	r, _ := NewRenderer(OutputJSON, out)
	return r
}

// captureBody spins up a server that records the decoded JSON request body.
func captureBody(t *testing.T, status int, respBody string) (*httptest.Server, *map[string]any) {
	t.Helper()
	body := map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(respBody))
	}))
	t.Cleanup(server.Close)
	return server, &body
}

func TestUsersCommand_Structure(t *testing.T) {
	cmd := newUsersCommand()
	if cmd.Use != "users" {
		t.Fatalf("expected use 'users', got %q", cmd.Use)
	}

	want := map[string]bool{
		"list": false, "get": false, "create": false, "update": false,
		"delete": false, "set-groups": false, "set-password": false, "tokens": false,
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

func TestUsersCreateCommand_RequiredEmail(t *testing.T) {
	cmd := newUsersCreateCommand()
	if cmd.Flags().Lookup("email") == nil {
		t.Fatal("expected --email flag")
	}
	if cmd.Flags().Lookup("tenant-grant") == nil {
		t.Fatal("expected --tenant-grant flag")
	}
	// email is marked required
	if _, err := executeCommand(cmd); err == nil {
		t.Fatal("expected error when --email is missing")
	}
}

func TestUsersDeleteCommand_HasYesFlag(t *testing.T) {
	cmd := newUsersDeleteCommand()
	f := cmd.Flags().Lookup("yes")
	if f == nil {
		t.Fatal("expected --yes flag")
	}
	if f.Shorthand != "y" {
		t.Fatalf("expected -y shorthand, got %q", f.Shorthand)
	}
}

func TestConfirm(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"y\n", true},
		{"Y\n", true},
		{"yes\n", true},
		{"YES\n", true},
		{"n\n", false},
		{"\n", false},
		{"nope\n", false},
		{"", false}, // EOF
	}
	for _, c := range cases {
		var buf bytes.Buffer
		got, err := confirm(strings.NewReader(c.in), &buf, "prompt: ")
		if err != nil {
			t.Fatalf("confirm(%q) error: %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("confirm(%q) = %v, want %v", c.in, got, c.want)
		}
		if !strings.Contains(buf.String(), "prompt: ") {
			t.Errorf("confirm did not write prompt for input %q", c.in)
		}
	}
}

func TestRunUsersList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[
			{"id":"u1","username":"alice","displayName":"Alice","superAdmin":true},
			{"id":"u2","username":"bob","displayName":"Bob","superAdmin":false}
		],"total":2}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runUsersList(newTestClient(t, server.URL), "", 1, 100, nil, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runUsersList error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"alice", "bob", "Alice", "true", "Total users: 2"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunUsersGet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"u1","username":"alice","email":"alice@example.com",
			"superAdmin":true,"groups":[{"id":"g1"}],"tenants":[{"id":"main"}]}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runUsersGet(newTestClient(t, server.URL), "u1", newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runUsersGet error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"alice@example.com", "g1", "main", "Super Admin:"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunUsersCreate_SendsBody(t *testing.T) {
	var gotBody map[string]any
	var gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"new","username":"alice","email":"alice@example.com"}`))
	}))
	t.Cleanup(server.Close)

	m := userMutation{
		email:        "alice@example.com",
		firstName:    "Alice",
		groups:       []string{"g1"},
		emailSet:     true,
		firstNameSet: true,
		groupsSet:    true,
	}
	var buf bytes.Buffer
	if err := runUsersCreate(newTestClient(t, server.URL), m, newTableRenderer(&buf)); err != nil {
		t.Fatalf("runUsersCreate error: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if gotBody["email"] != "alice@example.com" {
		t.Errorf("expected email in body, got %v", gotBody["email"])
	}
	if gotBody["firstName"] != "Alice" {
		t.Errorf("expected firstName in body, got %v", gotBody["firstName"])
	}
	if !strings.Contains(buf.String(), "alice@example.com") {
		t.Errorf("expected created user rendered, got:\n%s", buf.String())
	}
}

func TestRunUsersDelete_CancelMakesNoRequest(t *testing.T) {
	hit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runUsersDelete(newTestClient(t, server.URL), "u1", false, strings.NewReader("n\n"), newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runUsersDelete error: %v", err)
	}
	if hit {
		t.Error("expected no API request when deletion is cancelled")
	}
	if !strings.Contains(buf.String(), "Cancelled") {
		t.Errorf("expected cancellation message, got:\n%s", buf.String())
	}
}

func TestRunUsersDelete_Confirmed(t *testing.T) {
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
	err := runUsersDelete(newTestClient(t, server.URL), "u1", false, strings.NewReader("y\n"), newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runUsersDelete error: %v", err)
	}
	if !hit {
		t.Error("expected API request when deletion is confirmed")
	}
	if !strings.Contains(buf.String(), "deleted") {
		t.Errorf("expected deletion message, got:\n%s", buf.String())
	}
}

func TestRunUsersDelete_SkipConfirm(t *testing.T) {
	hit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	// Empty reader: must not block or cancel because confirmation is skipped.
	err := runUsersDelete(newTestClient(t, server.URL), "u1", true, strings.NewReader(""), newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runUsersDelete error: %v", err)
	}
	if !hit {
		t.Error("expected API request when --yes is set")
	}
}

func TestRunUsersTokensCreate_ShowsFullToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"t1","name":"ci","fullToken":"SECRET-TOKEN-VALUE"}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runUsersTokensCreate(newTestClient(t, server.URL), "u1", "ci", "", "", false, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runUsersTokensCreate error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "SECRET-TOKEN-VALUE") {
		t.Errorf("expected full token in output, got:\n%s", out)
	}
	if !strings.Contains(out, "not be shown again") {
		t.Errorf("expected one-time warning, got:\n%s", out)
	}
}

func TestUsersCreateCommand_NoGlobalPasswordCollision(t *testing.T) {
	// Regression: the user-password flag must not be named "password", which
	// would shadow the global basic-auth --password flag.
	cmd := newUsersCreateCommand()
	if cmd.Flags().Lookup("user-password") == nil {
		t.Fatal("expected --user-password flag")
	}
	if f := cmd.Flags().Lookup("password"); f != nil {
		t.Fatal("users create must not define a local --password flag (collides with global basic-auth password)")
	}
}

func TestRunUsersUpdate_PreservesExistingSuperAdmin(t *testing.T) {
	// The current user (returned by the GET) is a super-admin; the operator
	// updates only the first name. The full-replace PUT must carry superAdmin
	// true so the user is not silently demoted.
	server, body := captureBody(t, http.StatusOK,
		`{"id":"u1","email":"u1@b.com","superAdmin":true}`)

	m := userMutation{firstName: "Renamed", firstNameSet: true}
	var buf bytes.Buffer
	if err := runUsersUpdate(newTestClient(t, server.URL), "u1", m, newTableRenderer(&buf)); err != nil {
		t.Fatalf("runUsersUpdate error: %v", err)
	}
	if (*body)["superAdmin"] != true {
		t.Errorf("update must preserve existing superAdmin=true, body: %v", *body)
	}
	if (*body)["firstName"] != "Renamed" {
		t.Errorf("update must apply the changed firstName, body: %v", *body)
	}
	if (*body)["email"] != "u1@b.com" {
		t.Errorf("update must preserve the existing email, body: %v", *body)
	}
}

func TestRunUsersUpdate_AppliesExplicitSuperAdminFalse(t *testing.T) {
	// Existing user is a super-admin, operator explicitly passes --superadmin=false.
	server, body := captureBody(t, http.StatusOK,
		`{"id":"u1","email":"u1@b.com","superAdmin":true}`)

	m := userMutation{superAdmin: false, superAdminSet: true}
	var buf bytes.Buffer
	if err := runUsersUpdate(newTestClient(t, server.URL), "u1", m, newTableRenderer(&buf)); err != nil {
		t.Fatalf("runUsersUpdate error: %v", err)
	}
	if (*body)["superAdmin"] != false {
		t.Errorf("explicit --superadmin=false must be applied, body: %v", *body)
	}
}

func TestRunUsersSetGroups_ClearsWithEmptyArray(t *testing.T) {
	server, body := captureBody(t, http.StatusOK, ``)

	// No groups → must send "groupIds": [] (not omit it) so membership is cleared.
	var buf bytes.Buffer
	if err := runUsersSetGroups(newTestClient(t, server.URL), "u1", nil, newTableRenderer(&buf)); err != nil {
		t.Fatalf("runUsersSetGroups error: %v", err)
	}
	v, ok := (*body)["groupIds"]
	if !ok {
		t.Fatalf("groupIds must be present (empty array) to clear groups, body: %v", *body)
	}
	if arr, isArr := v.([]any); !isArr || len(arr) != 0 {
		t.Errorf("expected groupIds=[], got %v", v)
	}
}

func TestRunUsersDelete_JSONOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runUsersDelete(newTestClient(t, server.URL), "u1", true, strings.NewReader(""), newJSONRenderer(&buf))
	if err != nil {
		t.Fatalf("runUsersDelete error: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("delete -o json must emit valid JSON, got %q: %v", buf.String(), err)
	}
	if got["status"] != "deleted" || got["id"] != "u1" {
		t.Errorf("unexpected JSON payload: %v", got)
	}
}

func TestRunUsersList_EmptyJSONIsArray(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":null,"total":0}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	if err := runUsersList(newTestClient(t, server.URL), "", 1, 100, nil, newJSONRenderer(&buf)); err != nil {
		t.Fatalf("runUsersList error: %v", err)
	}
	if !strings.Contains(buf.String(), "[]") || strings.Contains(buf.String(), "null") {
		t.Errorf("empty list must render [] not null, got: %s", buf.String())
	}
}

func TestRunUsersList_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"boom"}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runUsersList(newTestClient(t, server.URL), "", 1, 100, nil, newTableRenderer(&buf))
	if err == nil {
		t.Fatal("expected error from failing API")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("expected formatted SDK error, got: %v", err)
	}
}
