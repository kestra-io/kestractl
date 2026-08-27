package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInvitationsCommand_Structure(t *testing.T) {
	cmd := newInvitationsCommand()
	if cmd.Use != "invitations" {
		t.Fatalf("expected use 'invitations', got %q", cmd.Use)
	}

	want := map[string]bool{
		"list": false, "get": false, "create": false, "delete": false,
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

func TestInvitationsCreateCommand_RequiredEmail(t *testing.T) {
	cmd := newInvitationsCreateCommand()
	for _, flag := range []string{"email", "role", "group", "superadmin", "create-user-if-not-exist"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Fatalf("expected --%s flag", flag)
		}
	}
	// email is marked required
	if _, err := executeCommand(cmd); err == nil {
		t.Fatal("expected error when --email is missing")
	}
}

func TestInvitationsDeleteCommand_HasYesFlag(t *testing.T) {
	cmd := newInvitationsDeleteCommand()
	f := cmd.Flags().Lookup("yes")
	if f == nil {
		t.Fatal("expected --yes flag")
	}
	if f.Shorthand != "y" {
		t.Fatalf("expected -y shorthand, got %q", f.Shorthand)
	}
}

func TestRunInvitationsList(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[
			{"id":"i1","email":"jane@example.com","status":"PENDING","sentAt":"2026-06-01T10:00:00Z","superAdmin":false},
			{"id":"i2","email":"john@example.com","status":"ACCEPTED","superAdmin":true}
		],"total":2}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runInvitationsList(newTestClient(t, server.URL), "", "pending", 1, 100, nil, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runInvitationsList error: %v", err)
	}

	if !strings.Contains(gotQuery, "status=PENDING") {
		t.Errorf("expected status=PENDING in query, got %q", gotQuery)
	}
	out := buf.String()
	for _, want := range []string{"jane@example.com", "john@example.com", "PENDING", "ACCEPTED", "Total invitations: 2"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunInvitationsList_EmptyBodyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runInvitationsList(newTestClient(t, server.URL), "", "", 1, 100, nil, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("expected empty list on empty-body response, got error: %v", err)
	}
	if !strings.Contains(buf.String(), "Total invitations: 0") {
		t.Errorf("expected empty list output, got:\n%s", buf.String())
	}
}

func TestRunInvitationsList_InvalidStatus(t *testing.T) {
	hit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runInvitationsList(newTestClient(t, server.URL), "", "BOGUS", 1, 100, nil, newTableRenderer(&buf))
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
	if !strings.Contains(err.Error(), "PENDING") {
		t.Errorf("expected allowed values in error, got: %v", err)
	}
	if hit {
		t.Error("expected no API request for invalid status")
	}
}

func TestRunInvitationsGet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"i1","email":"jane@example.com","status":"PENDING",
			"roles":[{"id":"r1","name":"editor"}],"groups":[{"id":"g1","name":"devs"}],
			"link":"https://kestra.example.com/invitations/i1"}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runInvitationsGet(newTestClient(t, server.URL), "i1", newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runInvitationsGet error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"jane@example.com", "PENDING", "editor", "devs", "https://kestra.example.com/invitations/i1"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunInvitationsCreate_SendsBody(t *testing.T) {
	var gotBody map[string]any
	var gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"new","email":"jane@example.com","status":"PENDING"}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runInvitationsCreate(newTestClient(t, server.URL), "jane@example.com",
		[]string{"r1", "r2"}, []string{"g1"}, true, false, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runInvitationsCreate error: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if gotBody["email"] != "jane@example.com" {
		t.Errorf("expected email in body, got %v", gotBody["email"])
	}
	roles, ok := gotBody["roles"].([]any)
	if !ok || len(roles) != 2 {
		t.Fatalf("expected 2 roles in body, got %v", gotBody["roles"])
	}
	if first, _ := roles[0].(map[string]any); first["id"] != "r1" {
		t.Errorf("expected first role id 'r1', got %v", roles[0])
	}
	groups, ok := gotBody["groups"].([]any)
	if !ok || len(groups) != 1 || groups[0] != "g1" {
		t.Errorf("expected groups [g1] in body, got %v", gotBody["groups"])
	}
	if gotBody["instanceOwner"] != true {
		t.Errorf("expected instanceOwner true in body, got %v", gotBody["instanceOwner"])
	}
	if _, present := gotBody["createUserIfNotExist"]; present {
		t.Errorf("expected createUserIfNotExist omitted, got %v", gotBody["createUserIfNotExist"])
	}
	if !strings.Contains(buf.String(), "jane@example.com") {
		t.Errorf("expected invitation detail output, got:\n%s", buf.String())
	}
}

func TestRunInvitationsCreate_DirectAccessGrant(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The server answers 204 when access is granted directly (existing
		// user or createUserIfNotExist) — no invitation detail is returned.
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runInvitationsCreate(newTestClient(t, server.URL), "jane@example.com",
		nil, nil, false, true, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runInvitationsCreate error: %v", err)
	}
	if !strings.Contains(buf.String(), "Access granted directly") {
		t.Errorf("expected direct-access message, got:\n%s", buf.String())
	}
}

func TestRunInvitationsDelete_Confirmed(t *testing.T) {
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
	err := runInvitationsDelete(newTestClient(t, server.URL), "i1", false, strings.NewReader("y\n"), newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runInvitationsDelete error: %v", err)
	}
	if !hit {
		t.Error("expected API request when deletion is confirmed")
	}
	if !strings.Contains(buf.String(), "deleted") {
		t.Errorf("expected deletion message, got:\n%s", buf.String())
	}
}

func TestRunInvitationsDelete_CancelMakesNoRequest(t *testing.T) {
	hit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runInvitationsDelete(newTestClient(t, server.URL), "i1", false, strings.NewReader("n\n"), newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runInvitationsDelete error: %v", err)
	}
	if hit {
		t.Error("expected no API request when deletion is cancelled")
	}
	if !strings.Contains(buf.String(), "Cancelled") {
		t.Errorf("expected cancellation message, got:\n%s", buf.String())
	}
}

func TestInvitationsListByEmailCommand_NoArgs(t *testing.T) {
	cmd := newInvitationsListByEmailCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestRunInvitationsListByEmail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"id":"i1","email":"jane@example.com","status":"PENDING"},
			{"id":"i2","email":"jane@example.com","status":"ACCEPTED"}
		]`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	if err := runInvitationsListByEmail(newTestClient(t, server.URL), "jane@example.com", newTableRenderer(&buf)); err != nil {
		t.Fatalf("runInvitationsListByEmail error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"i1", "i2", "jane@example.com", "PENDING", "ACCEPTED", "Total invitations: 2"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunInvitationsListByEmail_Empty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	if err := runInvitationsListByEmail(newTestClient(t, server.URL), "nobody@example.com", newTableRenderer(&buf)); err != nil {
		t.Fatalf("runInvitationsListByEmail error: %v", err)
	}
	if !strings.Contains(buf.String(), "Total invitations: 0") {
		t.Errorf("expected zero count, got:\n%s", buf.String())
	}
}
