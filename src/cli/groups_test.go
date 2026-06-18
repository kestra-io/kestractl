package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGroupsCommand_Structure(t *testing.T) {
	cmd := newGroupsCommand()
	if cmd.Use != "groups" {
		t.Fatalf("expected use 'groups', got %q", cmd.Use)
	}

	want := map[string]bool{
		"list": false, "get": false, "create": false,
		"update": false, "delete": false, "members": false,
	}
	membersFound := false
	memberWant := map[string]bool{"list": false, "add": false, "remove": false}
	for _, sub := range cmd.Commands() {
		name := strings.Fields(sub.Use)[0]
		if _, ok := want[name]; ok {
			want[name] = true
		}
		if name == "members" {
			membersFound = true
			for _, m := range sub.Commands() {
				mname := strings.Fields(m.Use)[0]
				if _, ok := memberWant[mname]; ok {
					memberWant[mname] = true
				}
			}
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("missing subcommand %q", name)
		}
	}
	if !membersFound {
		t.Fatal("missing 'members' subcommand")
	}
	for name, found := range memberWant {
		if !found {
			t.Errorf("missing members subcommand %q", name)
		}
	}
}

func TestGroupsCreateCommand_RequiredName(t *testing.T) {
	cmd := newGroupsCreateCommand()
	if cmd.Flags().Lookup("name") == nil {
		t.Fatal("expected --name flag")
	}
	if cmd.Flags().Lookup("member") == nil {
		t.Fatal("expected --member flag")
	}
	if _, err := executeCommand(cmd); err == nil {
		t.Fatal("expected error when --name is missing")
	}
}

func TestGroupsDeleteCommand_HasYesFlag(t *testing.T) {
	cmd := newGroupsDeleteCommand()
	f := cmd.Flags().Lookup("yes")
	if f == nil {
		t.Fatal("expected --yes flag")
	}
	if f.Shorthand != "y" {
		t.Fatalf("expected -y shorthand, got %q", f.Shorthand)
	}
}

func TestRunGroupsList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[
			{"id":"g1","name":"admins"},
			{"id":"g2","name":"developers"}
		],"total":2}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runGroupsList(newTestClient(t, server.URL), "", 1, 100, nil, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runGroupsList error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"g1", "admins", "developers", "Total groups: 2"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunGroupsList_EmptyJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[],"total":0}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runGroupsList(newTestClient(t, server.URL), "", 1, 100, nil, newJSONRenderer(&buf))
	if err != nil {
		t.Fatalf("runGroupsList error: %v", err)
	}
	// Empty results must render as [] rather than null.
	if got := strings.TrimSpace(buf.String()); got != "[]" {
		t.Errorf("expected empty JSON array, got %q", got)
	}
}

func TestRunGroupsGet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"g1","name":"admins","description":"Platform admins"}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runGroupsGet(newTestClient(t, server.URL), "g1", newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runGroupsGet error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"g1", "admins", "Platform admins", "Description:"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunGroupsCreate_SendsBody(t *testing.T) {
	var gotBody map[string]any
	var gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"new","name":"admins","description":"Platform admins"}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runGroupsCreate(newTestClient(t, server.URL), "admins", "Platform admins", []string{"u1", "u2"}, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runGroupsCreate error: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if gotBody["name"] != "admins" {
		t.Errorf("expected name in body, got %v", gotBody["name"])
	}
	if gotBody["description"] != "Platform admins" {
		t.Errorf("expected description in body, got %v", gotBody["description"])
	}
	members, ok := gotBody["membersId"].([]any)
	if !ok || len(members) != 2 {
		t.Errorf("expected 2 membersId in body, got %v", gotBody["membersId"])
	}
	if !strings.Contains(buf.String(), "admins") {
		t.Errorf("expected created group rendered, got:\n%s", buf.String())
	}
}

func TestRunGroupsUpdate_PreservesExistingName(t *testing.T) {
	// The operator updates only the description; the full-replace PUT must carry
	// the existing name so the group is not renamed to an empty string.
	server, body := captureBody(t, http.StatusOK,
		`{"id":"g1","name":"admins","description":"old"}`)

	var buf bytes.Buffer
	if err := runGroupsUpdate(newTestClient(t, server.URL), "g1", "", false, "new desc", true, newTableRenderer(&buf)); err != nil {
		t.Fatalf("runGroupsUpdate error: %v", err)
	}
	if (*body)["name"] != "admins" {
		t.Errorf("update must preserve existing name, body: %v", *body)
	}
	if (*body)["description"] != "new desc" {
		t.Errorf("update must apply the changed description, body: %v", *body)
	}
}

func TestRunGroupsUpdate_AppliesChangedName(t *testing.T) {
	server, body := captureBody(t, http.StatusOK,
		`{"id":"g1","name":"admins","description":"keep"}`)

	var buf bytes.Buffer
	if err := runGroupsUpdate(newTestClient(t, server.URL), "g1", "renamed", true, "", false, newTableRenderer(&buf)); err != nil {
		t.Fatalf("runGroupsUpdate error: %v", err)
	}
	if (*body)["name"] != "renamed" {
		t.Errorf("update must apply the changed name, body: %v", *body)
	}
	if (*body)["description"] != "keep" {
		t.Errorf("update must preserve existing description, body: %v", *body)
	}
}

func TestRunGroupsDelete_CancelMakesNoRequest(t *testing.T) {
	hit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runGroupsDelete(newTestClient(t, server.URL), "g1", false, strings.NewReader("n\n"), newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runGroupsDelete error: %v", err)
	}
	if hit {
		t.Error("expected no API request when deletion is cancelled")
	}
	if !strings.Contains(buf.String(), "Cancelled") {
		t.Errorf("expected cancellation message, got:\n%s", buf.String())
	}
}

func TestRunGroupsDelete_Confirmed(t *testing.T) {
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
	err := runGroupsDelete(newTestClient(t, server.URL), "g1", false, strings.NewReader("y\n"), newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runGroupsDelete error: %v", err)
	}
	if !hit {
		t.Error("expected API request when deletion is confirmed")
	}
	if !strings.Contains(buf.String(), "deleted") {
		t.Errorf("expected deletion message, got:\n%s", buf.String())
	}
}

func TestRunGroupsMembersList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[
			{"id":"u1","username":"alice","displayName":"Alice"},
			{"id":"u2","username":"bob","displayName":"Bob"}
		],"total":2}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runGroupsMembersList(newTestClient(t, server.URL), "g1", "", 1, 100, nil, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runGroupsMembersList error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"alice", "bob", "Alice", "Total members: 2"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunGroupsMembersAdd(t *testing.T) {
	hit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"u1","username":"alice"}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runGroupsMembersAdd(newTestClient(t, server.URL), "g1", "u1", newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runGroupsMembersAdd error: %v", err)
	}
	if !hit {
		t.Error("expected API request")
	}
	if !strings.Contains(buf.String(), "added to group") {
		t.Errorf("expected add confirmation, got:\n%s", buf.String())
	}
}

func TestRunGroupsMembersRemove(t *testing.T) {
	hit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"u1","username":"alice"}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runGroupsMembersRemove(newTestClient(t, server.URL), "g1", "u1", newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runGroupsMembersRemove error: %v", err)
	}
	if !hit {
		t.Error("expected API request")
	}
	if !strings.Contains(buf.String(), "removed from group") {
		t.Errorf("expected remove confirmation, got:\n%s", buf.String())
	}
}

func TestGroupsAutocompleteCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newGroupsAutocompleteCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}
