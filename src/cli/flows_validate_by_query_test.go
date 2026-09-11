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
		"company.team-first.yml":   flowSource("first"),
		"company.team-second.yaml": flowSource("second"),
		"company.team-README.md":   "not a flow",
		"nested/":                  "",
		"company.team-notes.txt":   "not a flow either",
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
	if byName["company.team-first.yml"] != flowSource("first") {
		t.Errorf("first.yml source not preserved: %q", byName["company.team-first.yml"])
	}
	if _, ok := byName["company.team-second.yaml"]; !ok {
		t.Errorf("second.yaml missing from %v", byName)
	}
}

func TestFlowsFromZip_WrapperStillReturnsSourcesOnly(t *testing.T) {
	archive := buildFlowZip(t, map[string]string{
		"company.team-only.yml": flowSource("only"),
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

// TestTrimDocumentMarkers covers the join hazard: a stored flow source keeps
// whatever the author wrote, and a leading "---" is ordinary YAML style.
func TestTrimDocumentMarkers(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   string
	}{
		{"plain source is untouched", "id: a\nnamespace: n", "id: a\nnamespace: n"},
		{"leading separator", "---\nid: a", "id: a"},
		{"leading separator after a blank line", "\n---\nid: a", "id: a"},
		{"trailing separator", "id: a\n---", "id: a"},
		{"trailing end-of-document marker", "id: a\n...", "id: a"},
		{"trailing newline", "id: a\n", "id: a"},
		{"both ends", "---\nid: a\n...\n", "id: a"},
		{"a --- inside a string is not a marker", "id: a\nmessage: \"---\"", "id: a\nmessage: \"---\""},
		{"an indented --- is not a marker", "id: a\nmessage: |\n  ---", "id: a\nmessage: |\n  ---"},
		{"only a separator", "---", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := trimDocumentMarkers(tc.source); got != tc.want {
				t.Errorf("trimDocumentMarkers(%q) = %q, want %q", tc.source, got, tc.want)
			}
		})
	}
}

// TestValidateSourcesInBatches_StripsSeparatorsSoIndicesLineUp is the
// regression test for the live failure: two valid flows, the second written
// with a leading "---", produced a spurious "No content to map due to
// end-of-input" on the second flow plus a phantom <index 2> row, because the
// joined body split into three documents instead of two.
func TestValidateSourcesInBatches_StripsSeparatorsSoIndicesLineUp(t *testing.T) {
	var got string
	server := &validateByQueryServer{
		violate: func(body string) []map[string]any {
			got = body
			return nil
		},
	}
	client := newTestClient(t, server.start(t))

	sources := []string{"id: first\nnamespace: n\n", "---\nid: second\nnamespace: n\n"}
	if _, err := validateSourcesInBatches(client, sources, 10); err != nil {
		t.Fatalf("validateSourcesInBatches error: %v", err)
	}

	if strings.Contains(got, "---\n---") {
		t.Errorf("the joined body must not contain an empty document:\n%s", got)
	}
	if want := "id: first\nnamespace: n\n---\nid: second\nnamespace: n"; got != want {
		t.Errorf("joined body =\n%q\nwant\n%q", got, want)
	}
}

// A source can still hold a document separator we cannot strip — one in the
// middle. Guessing which flow a shifted index belongs to would silently
// mis-report, so the run fails instead.
func TestValidateSourcesInBatches_RefusesToMapAnOutOfRangeIndex(t *testing.T) {
	for _, index := range []int{2, -1} {
		server := &validateByQueryServer{
			violate: func(string) []map[string]any {
				return []map[string]any{{"index": index, "constraints": "boom"}}
			},
		}
		client := newTestClient(t, server.start(t))

		_, err := validateSourcesInBatches(client, []string{"id: a\n", "id: b\n"}, 10)
		if err == nil {
			t.Fatalf("index %d: expected the desync to be reported", index)
		}
		if !strings.Contains(err.Error(), "cannot map validation results back to flows") {
			t.Errorf("index %d: unexpected error: %v", index, err)
		}
	}
}

// The server's identity wins over what we parsed out of the source, so a row
// is labelled correctly even when the source could not be read.
func TestRunFlowsValidateByQuery_LabelsFromServerIdentity(t *testing.T) {
	server := &validateByQueryServer{
		// A source our own YAML read cannot get an id out of.
		archive: buildFlowZip(t, map[string]string{
			"company.team-mystery.yml": "id: [unterminated\n",
		}),
		violate: func(string) []map[string]any {
			return []map[string]any{
				{"index": 0, "constraints": "Illegal Flow source", "flow": "mystery", "namespace": "company.team"},
			}
		},
	}
	client := newTestClient(t, server.start(t))

	var out bytes.Buffer
	if err := runFlowsValidateByQuery(client, nil, validateBatchSize, newTableRenderer(&out)); err == nil {
		t.Fatal("expected the unreadable flow to fail validation")
	}
	if !strings.Contains(out.String(), "company.team/mystery") {
		t.Errorf("expected the server's identity as the label, got:\n%s", out.String())
	}
	if strings.Contains(out.String(), "company.team-mystery.yml") {
		t.Errorf("the archive entry name should have been replaced:\n%s", out.String())
	}
}

func TestFlowsValidateByQueryCommand_RejectsANonPositiveBatchSize(t *testing.T) {
	for _, size := range []string{"0", "-5"} {
		cmd := newFlowsValidateByQueryCommand()
		_, err := executeCommand(cmd, "--all", "--batch-size", size)
		if err == nil {
			t.Errorf("--batch-size %s should be rejected", size)
			continue
		}
		if !strings.Contains(err.Error(), "--batch-size must be at least 1") {
			t.Errorf("--batch-size %s: unexpected error: %v", size, err)
		}
		if cmd.SilenceUsage {
			t.Errorf("--batch-size %s: a flag mistake should print the help", size)
		}
	}
}
