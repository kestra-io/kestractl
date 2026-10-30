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

func TestTestSuitesListCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newTestSuitesListCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestTestSuitesGetCommand_NoArgs(t *testing.T) {
	cmd := newTestSuitesGetCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 2 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestTestSuitesGetCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newTestSuitesGetCommand()
	_, err := executeCommand(cmd, "my.ns", "my-suite")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestTestSuitesDeleteCommand_NoArgs(t *testing.T) {
	cmd := newTestSuitesDeleteCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 2 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestTestSuitesDeleteCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newTestSuitesDeleteCommand()
	_, err := executeCommand(cmd, "my.ns", "my-suite")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestTestSuitesRunCommand_NoArgs(t *testing.T) {
	cmd := newTestSuitesRunCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 2 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestTestSuitesRunCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newTestSuitesRunCommand()
	_, err := executeCommand(cmd, "my.ns", "my-suite")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestTestSuitesCreateCommand_MissingFile(t *testing.T) {
	cmd := newTestSuitesCreateCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when --file not provided")
	}
	if !strings.Contains(err.Error(), "file") {
		t.Fatalf("expected file flag error, got: %v", err)
	}
}

func TestTestSuitesUpdateCommand_NoArgs(t *testing.T) {
	cmd := newTestSuitesUpdateCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 2 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestTestSuitesValidateCommand_MissingFile(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	cmd := newTestSuitesValidateCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when --file not provided")
	}
	if !strings.Contains(err.Error(), "--file") {
		t.Fatalf("expected --file error, got: %v", err)
	}
}

func TestTestSuitesValidateCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	f, err := os.CreateTemp("", "suite-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString("id: my-suite\nnamespace: my.ns\n")
	f.Close()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newTestSuitesValidateCommand()
	_, err = executeCommand(cmd, "--file", f.Name())
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestTestSuitesDeleteBulkCommand_NoArgs(t *testing.T) {
	cmd := newTestSuitesDeleteBulkCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
}

func TestTestSuitesDeleteBulkCommand_InvalidFormat(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	cmd := newTestSuitesDeleteBulkCommand()
	_, err := executeCommand(cmd, "bad-format")
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
	if !strings.Contains(err.Error(), "namespace") {
		t.Fatalf("expected namespace error, got: %v", err)
	}
}

func TestTestSuitesDeleteBulkCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newTestSuitesDeleteBulkCommand()
	_, err := executeCommand(cmd, "my.namespace/my-suite")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestTestSuitesDisableBulkCommand_NoArgs(t *testing.T) {
	cmd := newTestSuitesDisableBulkCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
}

func TestTestSuitesEnableBulkCommand_NoArgs(t *testing.T) {
	cmd := newTestSuitesEnableBulkCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
}

func TestTestSuitesCommand_Structure(t *testing.T) {
	cmd := newTestSuitesCommand()
	if cmd.Use != "test-suites" {
		t.Fatalf("expected use 'test-suites', got '%s'", cmd.Use)
	}
	var hasListCmd bool
	for _, sub := range cmd.Commands() {
		if sub.Use == "list" {
			hasListCmd = true
			break
		}
	}
	if !hasListCmd {
		t.Fatal("expected 'list' subcommand")
	}
}

func TestTestSuitesSearchResultsCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newTestSuitesSearchResultsCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestTestSuitesLastResultCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newTestSuitesLastResultCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestTestSuitesRunByQueryCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newTestSuitesRunByQueryCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestTestSuitesGetResultCommand_NoArgs(t *testing.T) {
	cmd := newTestSuitesGetResultCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestTestSuitesGetResultCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newTestSuitesGetResultCommand()
	_, err := executeCommand(cmd, "result-id-123")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestRunTestSuitesGetResult(t *testing.T) {
	payload := map[string]any{
		"id":          "result-id-123",
		"testSuiteId": "suite-id-abc",
		"namespace":   "my.namespace",
		"flowId":      "my-flow",
		"state":       "SUCCESS",
		"startDate":   "2025-01-01T10:00:00Z",
		"endDate":     "2025-01-01T10:05:00Z",
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runTestSuitesGetResult(newTestClient(t, server.URL), "result-id-123", newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runTestSuitesGetResult error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"result-id-123", "suite-id-abc", "my.namespace", "my-flow", "SUCCESS"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestTestSuiteRunOutcome(t *testing.T) {
	for _, format := range []string{"table", "json"} {
		for _, tc := range []struct {
			state, caseState string
			wantErr          bool
		}{
			{"SUCCESS", "SUCCESS", false}, {"SKIPPED", "SKIPPED", false},
			{"FAILED", "SUCCESS", true}, {"ERROR", "SUCCESS", true},
			{"SUCCESS", "FAILED", true}, {"SUCCESS", "ERROR", true},
		} {
			t.Run(format+"/"+tc.state+"/"+tc.caseState, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(map[string]any{"id": "result", "testSuiteId": "suite", "state": tc.state, "namespace": "ns", "flowId": "flow", "startDate": "2026-10-30T00:00:00Z", "endDate": "2026-10-30T00:00:01Z", "results": []map[string]any{{"testId": "case", "testType": "UNIT", "assertionResults": []any{}, "errors": []any{}, "state": tc.caseState}}})
				}))
				defer server.Close()
				var buf bytes.Buffer
				renderer, _ := NewRenderer(format, &buf)
				err := runTestSuitesRun(newTestClient(t, server.URL), "ns", "suite", nil, renderer)
				if (err != nil) != tc.wantErr {
					t.Errorf("error = %v, want error %v", err, tc.wantErr)
				}
				if !strings.Contains(buf.String(), tc.state) {
					t.Errorf("missing state in output: %s", &buf)
				}
				if format == "json" {
					var result map[string]any
					if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
						t.Fatal(err)
					}
					if tc.caseState == "SKIPPED" && (result["failed"] != float64(0) || result["skipped"] != float64(1)) {
						t.Errorf("incorrect skipped counts: %v", result)
					}
				}
			})
		}
	}
}

func TestTestSuiteValidationOutcome(t *testing.T) {
	for _, format := range []string{"table", "json"} {
		for _, constraints := range []string{"", "invalid assertion"} {
			t.Run(format+"/"+constraints, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(map[string]any{"index": 0, "constraints": constraints})
				}))
				defer server.Close()
				var buf bytes.Buffer
				renderer, _ := NewRenderer(format, &buf)
				err := runTestSuitesValidate(newTestClient(t, server.URL), "id: suite", renderer)
				if (err != nil) != (constraints != "") {
					t.Errorf("error = %v, constraints = %q", err, constraints)
				}
				if constraints != "" && !strings.Contains(buf.String(), constraints) {
					t.Errorf("missing violations: %s", &buf)
				}
			})
		}
	}
}

func TestTestSuiteCreateConflictDiagnostic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"type":"about:blank","title":"Conflict","status":409,"detail":"Test suite already exists"}`))
	}))
	defer server.Close()
	file := t.TempDir() + "/suite.yaml"
	if err := os.WriteFile(file, []byte("id: suite"), 0600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	err := runTestSuitesCreate(newTestClient(t, server.URL), file, newTableRenderer(&buf))
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected actionable conflict, got %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("unexpected success output: %s", &buf)
	}
}

func TestRunByQueryOutcomes(t *testing.T) {
	for _, format := range []string{"table", "json"} {
		for _, state := range []string{"SUCCESS", "SKIPPED", "FAILED", "ERROR"} {
			t.Run(format+"/"+state, func(t *testing.T) {
				result := map[string]any{"id": "result", "testSuiteId": "suite", "namespace": "ns", "flowId": "flow", "state": state, "startDate": "2026-10-30T00:00:00Z", "endDate": "2026-10-30T00:00:01Z"}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(map[string]any{"numberOfTestSuitesToBeRun": 1, "numberOfTestCasesToBeRun": 0, "results": []any{result}})
				}))
				defer server.Close()
				var buf bytes.Buffer
				renderer, _ := NewRenderer(format, &buf)
				err := runTestSuitesRunByQuery(newTestClient(t, server.URL), "ns", "flow", false, renderer)
				wantErr := state == "FAILED" || state == "ERROR"
				if (err != nil) != wantErr {
					t.Fatalf("state %s: error %v", state, err)
				}
				if !strings.Contains(buf.String(), state) {
					t.Fatalf("missing result: %s", &buf)
				}
				if format == "json" {
					var decoded map[string]any
					if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
}

func TestRunByQueryMixedAndAbsentResults(t *testing.T) {
	result := func(state, caseState string) map[string]any {
		r := map[string]any{"id": "result", "testSuiteId": "suite", "namespace": "ns", "flowId": "flow", "state": state, "startDate": "2026-10-30T00:00:00Z", "endDate": "2026-10-30T00:00:01Z"}
		if caseState != "" {
			r["results"] = []any{map[string]any{"testId": "case", "testType": "UNIT", "state": caseState, "assertionResults": []any{}, "errors": []any{}}}
		}
		return r
	}
	for _, tc := range []struct {
		name    string
		results []any
		count   int
		wantErr bool
	}{
		{"mixed", []any{result("SUCCESS", ""), result("FAILED", "")}, 2, true},
		{"failed case", []any{result("SUCCESS", "FAILED")}, 1, true},
		{"error case", []any{result("SUCCESS", "ERROR")}, 1, true},
		{"skipped case", []any{result("SUCCESS", "SKIPPED")}, 1, false},
		{"absent results", nil, 1, false},
		{"no matches", []any{}, 0, false},
	} {
		for _, format := range []string{"table", "json"} {
			t.Run(tc.name+"/"+format, func(t *testing.T) {
				payload := map[string]any{"numberOfTestSuitesToBeRun": tc.count, "numberOfTestCasesToBeRun": 0}
				if tc.results != nil {
					payload["results"] = tc.results
				}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(payload)
				}))
				defer server.Close()
				var buf bytes.Buffer
				renderer, _ := NewRenderer(format, &buf)
				err := runTestSuitesRunByQuery(newTestClient(t, server.URL), "ns", "flow", false, renderer)
				if (err != nil) != tc.wantErr {
					t.Fatalf("error=%v, want error=%v", err, tc.wantErr)
				}
				if tc.results == nil && strings.Contains(buf.String(), "SUCCESS") {
					t.Fatal("missing results must not be described as successful completion")
				}
				if format == "json" {
					var got map[string]any
					if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
						t.Fatal(err)
					}
					if got["numberOfTestSuitesToBeRun"] != float64(tc.count) {
						t.Fatalf("lost count: %v", got)
					}
				}
			})
		}
	}
}

type testSuiteFailWriter struct{ err error }

func (w testSuiteFailWriter) Write([]byte) (int, error) { return 0, w.err }

func TestTestSuiteFailuresPropagate(t *testing.T) {
	for _, format := range []string{"table", "json"} {
		for _, scenario := range []string{"api", "transport", "writer"} {
			t.Run(format+"/"+scenario, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if scenario == "api" {
						w.WriteHeader(http.StatusForbidden)
						w.Write([]byte(`{"message":"access denied"}`))
						return
					}
					w.Write([]byte(`{"numberOfTestSuitesToBeRun":0,"numberOfTestCasesToBeRun":0}`))
				}))
				defer server.Close()
				if scenario == "transport" {
					server.Close()
				}
				var buf bytes.Buffer
				renderer, _ := NewRenderer(format, &buf)
				writeErr := errors.New("output unavailable")
				if scenario == "writer" {
					renderer, _ = NewRenderer(format, testSuiteFailWriter{writeErr})
				}
				err := runTestSuitesRunByQuery(newTestClient(t, server.URL), "ns", "flow", false, renderer)
				if err == nil {
					t.Fatal("expected failure")
				}
				if scenario == "writer" && !errors.Is(err, writeErr) {
					t.Fatalf("lost write error: %v", err)
				}
				if scenario == "api" && !strings.Contains(err.Error(), "access denied") {
					t.Fatalf("lost API diagnostic: %v", err)
				}
				if scenario != "writer" && buf.Len() != 0 {
					t.Fatalf("rendered success after request failure: %s", &buf)
				}
			})
		}
	}
}
