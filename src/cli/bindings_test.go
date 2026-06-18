package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestBindingsCommand_Structure(t *testing.T) {
	cmd := newBindingsCommand()
	if cmd.Use != "bindings" {
		t.Fatalf("expected use 'bindings', got %q", cmd.Use)
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

func TestBindingsCreateCommand_RequiredFlags(t *testing.T) {
	cmd := newBindingsCreateCommand()
	for _, flag := range []string{"type", "external-id", "role", "namespace"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Fatalf("expected --%s flag", flag)
		}
	}
	// type, external-id and role are marked required
	if _, err := executeCommand(cmd); err == nil {
		t.Fatal("expected error when required flags are missing")
	}
}

func TestBindingsDeleteCommand_HasYesFlag(t *testing.T) {
	cmd := newBindingsDeleteCommand()
	f := cmd.Flags().Lookup("yes")
	if f == nil {
		t.Fatal("expected --yes flag")
	}
	if f.Shorthand != "y" {
		t.Fatalf("expected -y shorthand, got %q", f.Shorthand)
	}
}

func TestParseBindingType(t *testing.T) {
	if v, err := parseBindingType(""); err != nil || v != nil {
		t.Fatalf("expected nil,nil for empty input, got %v, %v", v, err)
	}
	if v, err := parseBindingType("group"); err != nil || string(*v) != "GROUP" {
		t.Fatalf("expected GROUP for lowercase input, got %v, %v", v, err)
	}
	if _, err := parseBindingType("BOGUS"); err == nil || !strings.Contains(err.Error(), "USER") {
		t.Fatalf("expected error listing allowed values, got %v", err)
	}
}

func TestRunBindingsList(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[
			{"id":"b1","type":"USER","role":{"id":"r1","name":"admin"},
			 "user":{"id":"u1","username":"jane"},"namespace":"company.team"},
			{"id":"b2","type":"GROUP","role":{"id":"r2","name":"editor"},
			 "group":{"id":"g1","name":"devs"}}
		],"total":2}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runBindingsList(newTestClient(t, server.URL), "user", "u1", "company.team", 1, 100, nil, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runBindingsList error: %v", err)
	}

	for _, want := range []string{"type=USER", "id=u1", "namespace=company.team"} {
		if !strings.Contains(gotQuery, want) {
			t.Errorf("expected %q in query, got %q", want, gotQuery)
		}
	}
	out := buf.String()
	for _, want := range []string{"jane", "devs", "admin", "editor", "company.team", "Total bindings: 2"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunBindingsList_EmptyBodyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runBindingsList(newTestClient(t, server.URL), "", "", "", 1, 100, nil, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("expected empty list on empty-body response, got error: %v", err)
	}
	if !strings.Contains(buf.String(), "Total bindings: 0") {
		t.Errorf("expected empty list output, got:\n%s", buf.String())
	}
}

func TestRunBindingsCreate_EmptyBodyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runBindingsCreate(newTestClient(t, server.URL), "USER", "u1", "r1", "", newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("expected status message on empty-body response, got error: %v", err)
	}
	if !strings.Contains(buf.String(), "Binding created") {
		t.Errorf("expected creation message, got:\n%s", buf.String())
	}
}

func TestRunBindingsList_InvalidType(t *testing.T) {
	hit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runBindingsList(newTestClient(t, server.URL), "BOGUS", "", "", 1, 100, nil, newTableRenderer(&buf))
	if err == nil {
		t.Fatal("expected error for invalid type")
	}
	if hit {
		t.Error("expected no API request for invalid type")
	}
}

func TestRunBindingsGet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"b1","type":"USER","namespace":"company.team",
			"role":{"id":"r1","name":"admin"},
			"user":{"id":"u1","username":"jane","displayName":"Jane Doe"}}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runBindingsGet(newTestClient(t, server.URL), "b1", newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runBindingsGet error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"b1", "USER", "jane", "admin (r1)", "company.team", "Jane Doe"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunBindingsCreate_SendsBody(t *testing.T) {
	var gotBody map[string]any
	var gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"new","type":"GROUP","role":{"id":"r1","name":"editor"},
			"group":{"id":"g1","name":"devs"}}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runBindingsCreate(newTestClient(t, server.URL), "group", "g1", "r1", "company.team", newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runBindingsCreate error: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if gotBody["type"] != "GROUP" {
		t.Errorf("expected type GROUP in body, got %v", gotBody["type"])
	}
	if gotBody["externalId"] != "g1" {
		t.Errorf("expected externalId in body, got %v", gotBody["externalId"])
	}
	if gotBody["roleId"] != "r1" {
		t.Errorf("expected roleId in body, got %v", gotBody["roleId"])
	}
	if gotBody["namespaceId"] != "company.team" {
		t.Errorf("expected namespaceId in body, got %v", gotBody["namespaceId"])
	}
	if !strings.Contains(buf.String(), "devs") {
		t.Errorf("expected binding detail output, got:\n%s", buf.String())
	}
}

func TestRunBindingsCreate_OmitsNamespaceWhenUnset(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"new","type":"USER"}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runBindingsCreate(newTestClient(t, server.URL), "USER", "u1", "r1", "", newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runBindingsCreate error: %v", err)
	}
	if _, present := gotBody["namespaceId"]; present {
		t.Errorf("expected namespaceId omitted, got %v", gotBody["namespaceId"])
	}
}

func TestRunBindingsDelete_Confirmed(t *testing.T) {
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
	err := runBindingsDelete(newTestClient(t, server.URL), "b1", false, strings.NewReader("y\n"), newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runBindingsDelete error: %v", err)
	}
	if !hit {
		t.Error("expected API request when deletion is confirmed")
	}
	if !strings.Contains(buf.String(), "deleted") {
		t.Errorf("expected deletion message, got:\n%s", buf.String())
	}
}

func TestRunBindingsDelete_CancelMakesNoRequest(t *testing.T) {
	hit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runBindingsDelete(newTestClient(t, server.URL), "b1", false, strings.NewReader("n\n"), newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runBindingsDelete error: %v", err)
	}
	if hit {
		t.Error("expected no API request when deletion is cancelled")
	}
	if !strings.Contains(buf.String(), "Cancelled") {
		t.Errorf("expected cancellation message, got:\n%s", buf.String())
	}
}

func TestBindingsBulkCreateCommand_MissingFile(t *testing.T) {
	cmd := newBindingsBulkCreateCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when --file not provided")
	}
	if !strings.Contains(err.Error(), "--file") {
		t.Fatalf("expected --file error, got: %v", err)
	}
}

func TestBindingsBulkCreateCommand_ClientError(t *testing.T) {
	f, err := os.CreateTemp("", "bindings-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(`[{"type":"USER","externalId":"u1","roleId":"r1"}]`)
	f.Close()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newBindingsBulkCreateCommand()
	_, err = executeCommand(cmd, "--file", f.Name())
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestRunBindingsBulkCreate(t *testing.T) {
	payload := []map[string]any{
		{"id": "b1", "type": "USER", "namespace": "my.ns"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "wrong method", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}))
	t.Cleanup(server.Close)

	f, err := os.CreateTemp("", "bindings-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(`[{"type":"USER","externalId":"u1","roleId":"r1"}]`)
	f.Close()

	var buf bytes.Buffer
	err = runBindingsBulkCreate(newTestClient(t, server.URL), f.Name(), newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runBindingsBulkCreate error: %v", err)
	}
	if !strings.Contains(buf.String(), "b1") {
		t.Errorf("expected binding ID in output, got:\n%s", buf.String())
	}
}
