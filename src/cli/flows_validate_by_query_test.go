package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	kestra "github.com/kestra-io/client-sdk/go-sdk/v2/kestra_api_client"
)

// validateByQueryServer stands in for a Kestra instance: it serves an export
// archive, a validate endpoint driven by the caller, and a flow inventory.
type validateByQueryServer struct {
	archive []byte
	violate func(body string) []map[string]any

	// searched records a hit on /flows/search, which this command must never
	// make: it validates what the export gives it and nothing else.
	searched bool

	mu     sync.Mutex
	bodies []string
}

func (s *validateByQueryServer) start(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/flows/export/by-query"):
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(s.archive)
		case strings.HasSuffix(r.URL.Path, "/flows/validate"):
			buf := new(bytes.Buffer)
			_, _ = buf.ReadFrom(r.Body)

			s.mu.Lock()
			s.bodies = append(s.bodies, buf.String())
			s.mu.Unlock()

			var violations []map[string]any
			if s.violate != nil {
				violations = s.violate(buf.String())
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(violations)
		case strings.HasSuffix(r.URL.Path, "/flows/search"):
			s.mu.Lock()
			s.searched = true
			s.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}, "total": 0})
		default:
			t.Errorf("unexpected request to %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func (s *validateByQueryServer) requestBodies() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.bodies...)
}

func flowSource(id string) string {
	return "id: " + id + "\nnamespace: company.team\ntasks: []\n"
}

func TestFlowSourcesFromZip_KeepsEntryNames(t *testing.T) {
	archive := buildFlowZip(t, map[string]string{
		"company.team/first.yaml": flowSource("first"),
		"company.team/second.yml": flowSource("second"),
		"company.team/README.md":  "not a flow",
		"company.team/nested/":    "",
		"company.team/notes.txt":  "not a flow either",
	})

	entries, skipped, err := flowSourcesFromZip(archive)
	if err != nil {
		t.Fatalf("flowSourcesFromZip error: %v", err)
	}
	if skipped != 0 {
		t.Errorf("expected no skipped entries, got %d", skipped)
	}
	if len(entries) != 2 {
		t.Fatalf("expected the 2 YAML entries, got %d: %+v", len(entries), entries)
	}

	byName := map[string]string{}
	for _, entry := range entries {
		byName[entry.Name] = entry.Source
	}
	if byName["company.team/first.yaml"] != flowSource("first") {
		t.Errorf("first.yaml source not preserved: %q", byName["company.team/first.yaml"])
	}
	if _, ok := byName["company.team/second.yml"]; !ok {
		t.Errorf("second.yml missing from %v", byName)
	}
}

func TestFlowsFromZip_WrapperStillReturnsSourcesOnly(t *testing.T) {
	archive := buildFlowZip(t, map[string]string{
		"company.team/only.yaml": flowSource("only"),
	})

	sources, skipped, err := flowsFromZip(archive)
	if err != nil {
		t.Fatalf("flowsFromZip error: %v", err)
	}
	if skipped != 0 {
		t.Errorf("expected no skipped entries, got %d", skipped)
	}
	if len(sources) != 1 || sources[0] != flowSource("only") {
		t.Errorf("unexpected sources: %+v", sources)
	}
}

func TestFlowIdentityFromSource(t *testing.T) {
	cases := []struct {
		name      string
		source    string
		namespace string
		flowID    string
	}{
		{
			name:      "a flow with a hyphen in both halves",
			source:    "id: my-flow\nnamespace: company.team-eu\ntasks: []\n",
			namespace: "company.team-eu",
			flowID:    "my-flow",
		},
		{
			name:      "key order does not matter",
			source:    "namespace: company.team\nid: my-flow\n",
			namespace: "company.team",
			flowID:    "my-flow",
		},
		{
			name:   "a source that does not parse leaves it to the server",
			source: "id: [unterminated\n",
		},
		{
			name:   "an empty source",
			source: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			namespace, flowID := flowIdentityFromSource(tc.source)
			if namespace != tc.namespace || flowID != tc.flowID {
				t.Errorf("flowIdentityFromSource(%q) = (%q, %q), want (%q, %q)",
					tc.source, namespace, flowID, tc.namespace, tc.flowID)
			}
		})
	}
}

// TestValidateSourcesInBatches_OffsetsViolationIndices is the regression test
// for the batching: the endpoint numbers violations from zero within each
// request, so without the offset a violation lands on the wrong flow.
func TestValidateSourcesInBatches_OffsetsViolationIndices(t *testing.T) {
	sources := make([]string, 7)
	for i := range sources {
		sources[i] = flowSource(fmt.Sprintf("flow-%d", i))
	}

	server := &validateByQueryServer{
		violate: func(body string) []map[string]any {
			// Fail the second flow of every batch, which is index 1 locally
			// and 1, 4 and (in the final 1-source batch) nothing globally.
			if strings.Count(body, "\n---\n") < 1 {
				return nil
			}
			return []map[string]any{
				{"index": 1, "constraints": "boom", "flow": "x", "namespace": "company.team"},
			}
		},
	}
	client := newTestClient(t, server.start(t))

	violations, err := validateSourcesInBatches(client, sources, 3)
	if err != nil {
		t.Fatalf("validateSourcesInBatches error: %v", err)
	}

	bodies := server.requestBodies()
	if len(bodies) != 3 {
		t.Fatalf("expected 3 batches for 7 sources at batch size 3, got %d", len(bodies))
	}
	if got := strings.Count(bodies[0], "\n---\n"); got != 2 {
		t.Errorf("expected 3 sources in the first batch, got %d separators", got)
	}
	if strings.Contains(bodies[2], "\n---\n") {
		t.Errorf("expected a single source in the last batch, got: %q", bodies[2])
	}

	got := make([]int32, 0, len(violations))
	for _, violation := range violations {
		got = append(got, violation.GetIndex())
	}
	want := []int32{1, 4}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("violation indices = %v, want %v", got, want)
	}
}

func TestValidateSourcesInBatches_SingleRequestWhenBatchIsLarger(t *testing.T) {
	server := &validateByQueryServer{}
	client := newTestClient(t, server.start(t))

	if _, err := validateSourcesInBatches(client, []string{flowSource("a"), flowSource("b")}, 100); err != nil {
		t.Fatalf("validateSourcesInBatches error: %v", err)
	}
	if bodies := server.requestBodies(); len(bodies) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(bodies))
	}
}

func TestBuildValidateResults_MapsViolationsOntoSeeds(t *testing.T) {
	seeds := []ValidateResult{
		{FilePath: "a.yaml", Success: true},
		{FilePath: "b.yaml", Success: true},
	}
	violations := []kestra.ValidateConstraintViolation{
		newViolation(1, "invalid task", []string{"deprecated"}),
	}

	results, failed := buildValidateResults(violations, seeds)
	if failed != 1 {
		t.Fatalf("expected 1 failure, got %d", failed)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if !results[0].Success {
		t.Errorf("a.yaml should have passed: %+v", results[0])
	}
	if results[1].Success || len(results[1].Constraints) != 1 {
		t.Errorf("b.yaml should have failed with one constraint: %+v", results[1])
	}
	if len(results[1].Warnings) != 1 {
		t.Errorf("expected the warning to be kept: %+v", results[1])
	}
	// A warning alone must never fail validation.
	if _, failedOnWarning := buildValidateResults(
		[]kestra.ValidateConstraintViolation{newViolation(0, "", []string{"deprecated"})},
		seeds,
	); failedOnWarning != 0 {
		t.Errorf("a warning must not fail validation, got %d failures", failedOnWarning)
	}
}

func TestBuildValidateResults_OutOfRangeIndexGetsItsOwnRow(t *testing.T) {
	seeds := []ValidateResult{{FilePath: "a.yaml", Success: true}}
	results, failed := buildValidateResults(
		[]kestra.ValidateConstraintViolation{newViolation(9, "boom", nil)},
		seeds,
	)

	if failed != 1 {
		t.Fatalf("expected 1 failure, got %d", failed)
	}
	if len(results) != 2 || results[1].FilePath != "<index 9>" {
		t.Fatalf("expected a synthetic <index 9> row, got %+v", results)
	}
	if !results[0].Success {
		t.Errorf("the seeded row must be untouched: %+v", results[0])
	}
}

func TestRunFlowsValidateByQuery_AllValid(t *testing.T) {
	server := &validateByQueryServer{
		archive: buildFlowZip(t, map[string]string{
			"company.team-good.yml": flowSource("good"),
		}),
	}
	client := newTestClient(t, server.start(t))

	var out bytes.Buffer
	if err := runFlowsValidateByQuery(client, nil, validateBatchSize, newTableRenderer(&out)); err != nil {
		t.Fatalf("expected no error for a clean instance, got: %v", err)
	}
	if !strings.Contains(out.String(), "1 valid flow(s), 0 failed") {
		t.Errorf("unexpected output:\n%s", out.String())
	}
}

func TestFlowsValidateByQueryCommand_RejectsPositionalArgs(t *testing.T) {
	cmd := newFlowsValidateByQueryCommand()
	if _, err := executeCommand(cmd, "./flows/"); err == nil {
		t.Fatal("expected validate-by-query to reject a path argument")
	}
}

func newViolation(index int32, constraints string, warnings []string) kestra.ValidateConstraintViolation {
	violation := kestra.NewValidateConstraintViolation(index)
	if constraints != "" {
		violation.SetConstraints(constraints)
	}
	if len(warnings) > 0 {
		violation.SetWarnings(warnings)
	}
	return *violation
}

// TestRunFlowsValidateByQuery_ReportsBrokenStoredFlows covers what live QA on
// an instance migrated in place from 1.3.37 to 2.0.0 actually produces: the
// export copies the stored source text without parsing it, so a flow whose
// task type no longer exists still reaches the validate endpoint and is
// reported there — once.
func TestRunFlowsValidateByQuery_ReportsBrokenStoredFlows(t *testing.T) {
	legacy := "id: legacy\nnamespace: company.team\ntasks:\n  - id: loop\n    type: io.kestra.plugin.core.flow.EachSequential\n"
	server := &validateByQueryServer{
		archive: buildFlowZip(t, map[string]string{
			"company.team-legacy.yml": legacy,
			"company.team-good.yml":   flowSource("good"),
		}),
		violate: func(body string) []map[string]any {
			index := 0
			if strings.Index(body, "id: legacy") > strings.Index(body, "id: good") {
				index = 1
			}
			return []map[string]any{
				{"index": index, "constraints": "Validation error: Invalid type: io.kestra.plugin.core.flow.EachSequential",
					"flow": "legacy", "namespace": "company.team"},
			}
		},
	}
	client := newTestClient(t, server.start(t))

	var out bytes.Buffer
	err := runFlowsValidateByQuery(client, nil, validateBatchSize, newTableRenderer(&out))
	if err == nil {
		t.Fatal("expected a non-nil error so the command exits non-zero")
	}
	if !strings.Contains(err.Error(), "validation failed for 1 flow(s)") {
		t.Errorf("the broken flow must be counted once, got: %v", err)
	}

	rendered := out.String()
	if got := strings.Count(rendered, "company.team/legacy"); got != 1 {
		t.Errorf("expected exactly 1 row for the broken flow, got %d:\n%s", got, rendered)
	}
	if !strings.Contains(rendered, "Invalid type: io.kestra.plugin.core.flow.EachSequential") {
		t.Errorf("expected the server's message in the output:\n%s", rendered)
	}
	if !strings.Contains(rendered, "1 valid flow(s), 1 failed") {
		t.Errorf("unexpected summary:\n%s", rendered)
	}

	server.mu.Lock()
	defer server.mu.Unlock()
	if server.searched {
		t.Error("validate-by-query must not call /flows/search")
	}
}

// Validating a whole instance has to be asked for by name, so a bare
// invocation is a usage mistake and prints the help.
func TestFlowsValidateByQueryCommand_RequiresASelection(t *testing.T) {
	cmd := newFlowsValidateByQueryCommand()
	out, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected a bare validate-by-query to be rejected")
	}
	if !strings.Contains(err.Error(), "a selection is required") {
		t.Errorf("unexpected error: %v", err)
	}
	if cmd.SilenceUsage {
		t.Error("a usage mistake must print the help")
	}
	if !strings.Contains(out, "--all") {
		t.Errorf("the help should point at --all, got:\n%s", out)
	}
}

func TestFlowsValidateByQueryCommand_AllIsExclusive(t *testing.T) {
	for _, flag := range []string{"--namespace=company.team", "--flow=my-flow", "--filter=NAMESPACE:EQUALS:x"} {
		cmd := newFlowsValidateByQueryCommand()
		if _, err := executeCommand(cmd, "--all", flag); err == nil {
			t.Errorf("expected --all %s to be rejected", flag)
		} else if !strings.Contains(err.Error(), "cannot be combined") {
			t.Errorf("--all %s: unexpected error: %v", flag, err)
		}
	}
}

// The counterpart to the two above: a run that reaches the server must not
// bury its report under the flag list, whatever the outcome.
func TestFlowsValidateByQueryCommand_KeepsUsageSilencedOnceRunning(t *testing.T) {
	cmd := newFlowsValidateByQueryCommand()
	if _, err := executeCommand(cmd, "--all"); err == nil {
		t.Fatal("expected the run to fail without a reachable server")
	}
	if !cmd.SilenceUsage {
		t.Error("a failure past the selection check must not print the help")
	}
}
