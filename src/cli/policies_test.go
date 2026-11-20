package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func policyServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(s.Close)
	return s
}

func TestPoliciesCommand_Structure(t *testing.T) {
	cmd := newPoliciesCommand()
	want := map[string]bool{"list": false, "get": false, "create": false, "update": false,
		"delete": false, "validate": false, "export": false}
	for _, sub := range cmd.Commands() {
		want[strings.Fields(sub.Use)[0]] = true
	}
	for name, found := range want {
		if !found {
			t.Errorf("missing subcommand %q", name)
		}
	}
}

func TestRunPoliciesList(t *testing.T) {
	s := policyServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/main/policies/search" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"id":"p1","displayName":"No latest","scope":"TENANT","enforcement":"ENFORCE","rules":{"mutate":1,"validate":2}}],"total":1}`))
	})
	var buf bytes.Buffer
	if err := runPoliciesList(newTestClient(t, s.URL), 1, 100, nil, newTableRenderer(&buf)); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ID", "p1", "No latest", "ENFORCE", "Total policies: 1"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("output missing %q:\n%s", want, buf.String())
		}
	}
}

func TestRunPoliciesGet(t *testing.T) {
	s := policyServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/main/policies/p1" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"p1","displayName":"No latest","scope":"TENANT","enforcement":"WARN","rules":[{"a":1}],"source":"id: p1"}`))
	})
	var buf bytes.Buffer
	if err := runPoliciesGet(newTestClient(t, s.URL), "p1", newTableRenderer(&buf)); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"p1", "WARN", "Rules:", "id: p1"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("output missing %q:\n%s", want, buf.String())
		}
	}
}

func TestRunPoliciesCreateUpdate(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	s := policyServer(t, func(w http.ResponseWriter, r *http.Request) {
		b := new(bytes.Buffer)
		_, _ = b.ReadFrom(r.Body)
		gotMethod, gotPath, gotBody = r.Method, r.URL.Path, b.String()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"p1"}`))
	})
	file := filepath.Join(t.TempDir(), "p.yml")
	if err := os.WriteFile(file, []byte("id: p1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := newTestClient(t, s.URL)

	if err := runPoliciesCreate(c, file, newTableRenderer(&bytes.Buffer{})); err != nil {
		t.Fatal(err)
	}
	if gotMethod != "POST" || gotPath != "/api/v1/main/policies" || !strings.Contains(gotBody, "id: p1") {
		t.Errorf("create: %s %s %q", gotMethod, gotPath, gotBody)
	}

	if err := runPoliciesUpdate(c, "p1", file, newTableRenderer(&bytes.Buffer{})); err != nil {
		t.Fatal(err)
	}
	if gotMethod != "PUT" || gotPath != "/api/v1/main/policies/p1" {
		t.Errorf("update: %s %s", gotMethod, gotPath)
	}

	if err := runPoliciesCreate(c, "/nonexistent.yml", newTableRenderer(&bytes.Buffer{})); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestRunPoliciesDelete(t *testing.T) {
	called := false
	s := policyServer(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Method != "DELETE" || r.URL.Path != "/api/v1/main/policies/p1" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	c := newTestClient(t, s.URL)

	var buf bytes.Buffer
	if err := runPoliciesDelete(c, "p1", false, strings.NewReader("n\n"), newTableRenderer(&buf)); err != nil {
		t.Fatal(err)
	}
	if called || !strings.Contains(buf.String(), "Cancelled") {
		t.Fatalf("declined delete must not call API: called=%v out=%q", called, buf.String())
	}

	buf.Reset()
	if err := runPoliciesDelete(c, "p1", true, strings.NewReader(""), newTableRenderer(&buf)); err != nil {
		t.Fatal(err)
	}
	if !called || !strings.Contains(buf.String(), "deleted") {
		t.Fatalf("expected delete: called=%v out=%q", called, buf.String())
	}
}

func TestRunPoliciesValidate(t *testing.T) {
	s := policyServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/main/policies/validate" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"index":0}`))
	})
	file := filepath.Join(t.TempDir(), "p.yml")
	_ = os.WriteFile(file, []byte("id: p1\n"), 0o600)
	if err := runPoliciesValidate(newTestClient(t, s.URL), file, newTableRenderer(&bytes.Buffer{})); err != nil {
		t.Fatal(err)
	}
}

func TestRunPoliciesExport(t *testing.T) {
	var gotPath string
	s := policyServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte("id: p1"))
	})
	c := newTestClient(t, s.URL)
	out := filepath.Join(t.TempDir(), "p.yml")

	if err := runPoliciesExport(c, nil, out); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(gotPath, "/policies/export") {
		t.Errorf("path %s", gotPath)
	}
	if b, _ := os.ReadFile(out); string(b) != "id: p1" {
		t.Errorf("file content %q", b)
	}

	if err := runPoliciesExport(c, []string{"p1"}, out); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(gotPath, "/policies/export/by-ids") {
		t.Errorf("path %s", gotPath)
	}
}
