package cli

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// bulkDeployServer is a fake Kestra that records what a directory deploy sends.
type bulkDeployServer struct {
	mu sync.Mutex

	validateBodies []string
	bulkBodies     []string
	bulkQueries    []url.Values
	bulkTypes      []string
	searches       []string // namespace of each search
	perFlow        []string // "METHOD path" of per-flow requests

	violations   string                     // JSON array returned by /validate
	existing     map[string][]string        // namespace -> existing flow ids
	failBulk     func(call int) bool        // true -> answer 500
	bulkResponse func(body string) string   // overrides the echoed response
	failPerFlow  map[string]int             // flow id -> status for PUT/POST
	existingBody func(ns, id string) string // label-bearing search results
}

func (s *bulkDeployServer) start(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/flows/validate"):
			s.validateBodies = append(s.validateBodies, string(body))
			if s.violations == "" {
				_, _ = w.Write([]byte("[]"))
				return
			}
			_, _ = w.Write([]byte(s.violations))
		case strings.HasSuffix(path, "/flows/search"):
			ns := r.URL.Query().Get("filters[namespace][EQUALS]")
			if ns == "" {
				for _, v := range r.URL.Query() {
					ns = v[0]
				}
			}
			s.searches = append(s.searches, ns)
			results := []string{}
			for _, id := range s.existing[ns] {
				results = append(results, fmt.Sprintf(`{"id":%q,"namespace":%q,"revision":1,"labels":[{"key":"a","value":"b"}]}`, id, ns))
			}
			_, _ = fmt.Fprintf(w, `{"results":[%s],"total":%d}`, strings.Join(results, ","), len(results))
		case strings.HasSuffix(path, "/flows/bulk"):
			s.bulkBodies = append(s.bulkBodies, string(body))
			s.bulkQueries = append(s.bulkQueries, r.URL.Query())
			s.bulkTypes = append(s.bulkTypes, r.Header.Get("Content-Type"))
			if s.failBulk != nil && s.failBulk(len(s.bulkBodies)) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"message":"boom"}`))
				return
			}
			if s.bulkResponse != nil {
				_, _ = w.Write([]byte(s.bulkResponse(string(body))))
				return
			}
			out := []string{}
			for _, doc := range strings.Split(string(body), "\n---\n") {
				ns, id := docIdentity(doc)
				out = append(out, fmt.Sprintf(`{"id":%q,"namespace":%q,"revision":1,"labels":[{"key":"a","value":"b"}]}`, id, ns))
			}
			_, _ = w.Write([]byte("[" + strings.Join(out, ",") + "]"))
		default:
			s.perFlow = append(s.perFlow, r.Method+" "+path)
			parts := strings.Split(strings.Trim(path, "/"), "/")
			id := parts[len(parts)-1]
			ns, docID := docIdentity(string(body))
			if r.Method == http.MethodPost {
				id = docID
			}
			if status, ok := s.failPerFlow[id]; ok && r.Method != http.MethodGet {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"message":"denied"}`))
				return
			}
			if r.Method == http.MethodGet {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = fmt.Fprintf(w, `{"id":%q,"namespace":%q,"revision":7,"disabled":false,"deleted":false,"tasks":[]}`, id, ns)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func docIdentity(doc string) (ns, id string) {
	for _, line := range strings.Split(doc, "\n") {
		if v, ok := strings.CutPrefix(line, "namespace: "); ok {
			ns = strings.TrimSpace(v)
		}
		if v, ok := strings.CutPrefix(line, "id: "); ok {
			id = strings.TrimSpace(v)
		}
	}
	return ns, id
}

func flowYAML(ns, id string) string {
	return "id: " + id + "\nnamespace: " + ns + "\ntriggers:\n  - id: sched\n    type: io.kestra.plugin.core.trigger.Schedule\n    cron: \"* * * * *\"\ntasks:\n  - id: t\n    type: io.kestra.plugin.core.log.Log\n    message: hi\n"
}

func writeFlowDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func numberedFlows(ns string, n int) map[string]string {
	files := map[string]string{}
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("flow-%03d", i)
		files[id+".yaml"] = flowYAML(ns, id)
	}
	return files
}

func deployDir(t *testing.T, s *bulkDeployServer, dir string, nsOverride, prefix string, disableTriggers, override, failFast bool) []DeployResult {
	t.Helper()
	server := s.start(t)
	files, err := collectFlowFiles(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	return deployFlowsBulk(newTestClient(t, server.URL), files, nsOverride, prefix, disableTriggers, override, failFast)
}

func requireSuccess(t *testing.T, results []DeployResult, want int) {
	t.Helper()
	if len(results) != want {
		t.Fatalf("expected %d results, got %d: %+v", want, len(results), results)
	}
	for _, r := range results {
		if !r.Success {
			t.Fatalf("expected success, got %+v", r)
		}
	}
}

func TestDeployBulk_RequestCountAndShape(t *testing.T) {
	s := &bulkDeployServer{}
	dir := writeFlowDir(t, numberedFlows("my.ns", 250))
	results := deployDir(t, s, dir, "", "", false, false, false)

	requireSuccess(t, results, 250)
	if len(s.validateBodies) != 3 || len(s.bulkBodies) != 3 {
		t.Fatalf("expected 3 validate and 3 bulk requests, got %d and %d", len(s.validateBodies), len(s.bulkBodies))
	}
	if len(s.searches) != 1 {
		t.Fatalf("expected one search for one namespace, got %v", s.searches)
	}
	if len(s.perFlow) != 0 {
		t.Fatalf("expected no per-flow requests, got %v", s.perFlow)
	}
	for i, q := range s.bulkQueries {
		if q.Get("delete") != "false" {
			t.Errorf("bulk %d: delete=%q, want false", i, q.Get("delete"))
		}
		if _, has := q["namespace"]; has {
			t.Errorf("bulk %d: unexpected namespace param", i)
		}
		if s.bulkTypes[i] != "application/yaml" {
			t.Errorf("bulk %d: content type %q", i, s.bulkTypes[i])
		}
	}
	if results[0].Revision != 1 {
		t.Errorf("expected revision from the bulk response (array labels included), got %d", results[0].Revision)
	}
}

func TestDeployBulk_ExistingFlowsFailWithoutOverride(t *testing.T) {
	s := &bulkDeployServer{existing: map[string][]string{"my.ns": {"flow-001"}}}
	dir := writeFlowDir(t, numberedFlows("my.ns", 3))
	results := deployDir(t, s, dir, "", "", false, false, false)

	if results[1].Success || !strings.Contains(results[1].Error, "already exists") {
		t.Fatalf("expected flow-001 to fail as existing, got %+v", results[1])
	}
	if !results[0].Success || !results[2].Success {
		t.Fatalf("other flows must deploy: %+v", results)
	}
	if strings.Contains(s.bulkBodies[0], "id: flow-001") {
		t.Fatalf("existing flow must not be in the bulk body: %s", s.bulkBodies[0])
	}
}

func TestDeployBulk_OverrideSkipsSearch(t *testing.T) {
	s := &bulkDeployServer{existing: map[string][]string{"my.ns": {"flow-001"}}}
	dir := writeFlowDir(t, numberedFlows("my.ns", 3))
	results := deployDir(t, s, dir, "", "", false, true, false)

	requireSuccess(t, results, 3)
	if len(s.searches) != 0 {
		t.Fatalf("no search expected with --override, got %v", s.searches)
	}
}

func TestDeployBulk_OneSearchPerNamespace(t *testing.T) {
	s := &bulkDeployServer{}
	dir := writeFlowDir(t, map[string]string{
		"a.yaml": flowYAML("ns.one", "a"),
		"b.yaml": flowYAML("ns.one", "b"),
		"c.yaml": flowYAML("ns.two", "c"),
	})
	requireSuccess(t, deployDir(t, s, dir, "", "", false, false, false), 3)
	if len(s.searches) != 2 {
		t.Fatalf("expected 2 searches, got %v", s.searches)
	}
}

func TestDeployBulk_ValidationFailureIsPerFile(t *testing.T) {
	s := &bulkDeployServer{violations: `[{"index":1,"constraints":"bad task"}]`}
	dir := writeFlowDir(t, numberedFlows("my.ns", 3))
	results := deployDir(t, s, dir, "", "", false, false, false)

	if results[1].Success || !strings.Contains(results[1].Error, "bad task") {
		t.Fatalf("expected validation error on flow-001, got %+v", results[1])
	}
	if !results[0].Success || !results[2].Success {
		t.Fatalf("others must still deploy: %+v", results)
	}
	if strings.Contains(s.bulkBodies[0], "id: flow-001") {
		t.Fatal("invalid flow must not be sent to the bulk endpoint")
	}
}

func TestDeployBulk_WarningsDoNotFail(t *testing.T) {
	s := &bulkDeployServer{violations: `[{"index":0,"warnings":["careful"],"deprecationPaths":["tasks[0]"],"outdated":true}]`}
	dir := writeFlowDir(t, numberedFlows("my.ns", 2))
	requireSuccess(t, deployDir(t, s, dir, "", "", false, false, false), 2)
}

func TestDeployBulk_FailFastWritesNothing(t *testing.T) {
	s := &bulkDeployServer{violations: `[{"index":1,"constraints":"bad task"}]`}
	dir := writeFlowDir(t, numberedFlows("my.ns", 3))
	results := deployDir(t, s, dir, "", "", false, false, true)

	if len(s.bulkBodies) != 0 || len(s.perFlow) != 0 {
		t.Fatalf("fail-fast must write nothing, got bulk=%d perFlow=%v", len(s.bulkBodies), s.perFlow)
	}
	if len(results) != 2 || results[1].Success {
		t.Fatalf("expected results cut after the first failure, got %+v", results)
	}
}

func TestDeployBulk_FailFastLocalAndExistenceFailures(t *testing.T) {
	s := &bulkDeployServer{}
	files := numberedFlows("my.ns", 2)
	files["a-broken.yaml"] = "id: x\n"
	dir := writeFlowDir(t, files)
	results := deployDir(t, s, dir, "", "", false, false, true)
	if len(s.validateBodies) != 0 || len(s.bulkBodies) != 0 {
		t.Fatal("a local failure must stop before any request")
	}
	if len(results) != 1 || results[0].Success {
		t.Fatalf("unexpected results: %+v", results)
	}

	s2 := &bulkDeployServer{existing: map[string][]string{"my.ns": {"flow-000"}}}
	results = deployDir(t, s2, writeFlowDir(t, numberedFlows("my.ns", 2)), "", "", false, false, true)
	if len(s2.bulkBodies) != 0 {
		t.Fatal("an existence failure must stop before any write")
	}
	if len(results) != 1 || results[0].Success {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestDeployBulk_FailedChunkFallsBackPerFlow(t *testing.T) {
	s := &bulkDeployServer{
		failBulk:    func(int) bool { return true },
		failPerFlow: map[string]int{"flow-001": http.StatusForbidden},
	}
	dir := writeFlowDir(t, numberedFlows("my.ns", 3))
	results := deployDir(t, s, dir, "", "", false, false, false)

	if !results[0].Success || !results[2].Success || results[0].Revision != 7 {
		t.Fatalf("fallback should deploy flow-000 and flow-002: %+v", results)
	}
	if results[1].Success || !strings.Contains(results[1].Error, "denied") {
		t.Fatalf("flow-001 must carry its own error, got %+v", results[1])
	}
	if len(s.searches) != 1 {
		t.Fatalf("the fallback must not re-run the existence check, searches: %v", s.searches)
	}
}

func TestDeployBulk_FailFastStopsAfterFailedChunkWithoutFallback(t *testing.T) {
	s := &bulkDeployServer{failBulk: func(int) bool { return true }}
	dir := writeFlowDir(t, numberedFlows("my.ns", 150))
	results := deployDir(t, s, dir, "", "", false, true, true)

	if len(s.bulkBodies) != 1 {
		t.Fatalf("expected a single bulk attempt, got %d", len(s.bulkBodies))
	}
	if len(s.perFlow) != 0 {
		t.Fatalf("fail-fast must not fall back, got %v", s.perFlow)
	}
	if len(results) != 100 || results[0].Success || !strings.Contains(results[0].Error, "boom") {
		t.Fatalf("expected the 100 files of the failed chunk reported, got %d: %+v", len(results), results[0])
	}
}

func TestDeployBulk_MultiDocumentAndDuplicatesRejected(t *testing.T) {
	s := &bulkDeployServer{}
	dir := writeFlowDir(t, map[string]string{
		"a.yaml":       flowYAML("my.ns", "a"),
		"b-multi.yaml": flowYAML("my.ns", "b") + "---\n" + flowYAML("my.ns", "c"),
		"c-dup.yaml":   flowYAML("my.ns", "a"),
	})
	results := deployDir(t, s, dir, "", "", false, false, false)

	if !results[0].Success {
		t.Fatalf("first definition must deploy: %+v", results[0])
	}
	if results[1].Success || !strings.Contains(results[1].Error, "one flow") {
		t.Fatalf("multi-document file must be rejected: %+v", results[1])
	}
	if results[2].Success || !strings.Contains(results[2].Error, "already defined") {
		t.Fatalf("duplicate must be rejected: %+v", results[2])
	}
	if got := strings.Count(s.bulkBodies[0], "id: a\n"); got != 1 {
		t.Fatalf("flow a must be sent once, got %d", got)
	}
}

func TestDeployBulk_NamespaceOptionsAndTriggersAppliedToBody(t *testing.T) {
	s := &bulkDeployServer{}
	dir := writeFlowDir(t, numberedFlows("my.ns", 2))
	results := deployDir(t, s, dir, "", "staging.pr1", true, true, false)

	requireSuccess(t, results, 2)
	if results[0].Namespace != "staging.pr1.my.ns" {
		t.Fatalf("unexpected namespace %q", results[0].Namespace)
	}
	body := s.bulkBodies[0]
	if !strings.Contains(body, "namespace: staging.pr1.my.ns") || !strings.Contains(body, "disabled: true") {
		t.Fatalf("prefix and disabled triggers must be in the body: %s", body)
	}

	s2 := &bulkDeployServer{}
	results = deployDir(t, s2, writeFlowDir(t, numberedFlows("my.ns", 2)), "forced.ns", "", false, true, false)
	requireSuccess(t, results, 2)
	if !strings.Contains(s2.bulkBodies[0], "namespace: forced.ns") {
		t.Fatalf("namespace override must be in the body: %s", s2.bulkBodies[0])
	}
}

func TestDeployBulk_ResponseMissingFlowFallsBack(t *testing.T) {
	s := &bulkDeployServer{bulkResponse: func(string) string { return `[{"id":"flow-000","namespace":"my.ns","revision":4}]` }}
	dir := writeFlowDir(t, numberedFlows("my.ns", 2))
	results := deployDir(t, s, dir, "", "", false, true, false)

	requireSuccess(t, results, 2)
	if results[0].Revision != 4 || results[1].Revision != 7 {
		t.Fatalf("unexpected revisions: %+v", results)
	}
}

func TestRunFlowsDeploy_RoutesByPathKind(t *testing.T) {
	// A directory with a single file keeps the per-flow path.
	s := &bulkDeployServer{}
	server := s.start(t)
	dir := writeFlowDir(t, numberedFlows("my.ns", 1))
	renderer, err := NewRenderer(OutputJSON, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if err := runFlowsDeploy(newTestClient(t, server.URL), dir, true, "", "", false, false, true, renderer); err != nil {
		t.Fatal(err)
	}
	if len(s.bulkBodies) != 0 || len(s.validateBodies) != 0 {
		t.Fatal("a single-file directory must not use the bulk path")
	}

	// A directory of two goes through it.
	s2 := &bulkDeployServer{}
	server2 := s2.start(t)
	dir2 := writeFlowDir(t, numberedFlows("my.ns", 2))
	if err := runFlowsDeploy(newTestClient(t, server2.URL), dir2, true, "", "", false, false, true, renderer); err != nil {
		t.Fatal(err)
	}
	if len(s2.bulkBodies) != 1 || len(s2.perFlow) != 0 {
		t.Fatalf("expected one bulk request, got bulk=%d perFlow=%v", len(s2.bulkBodies), s2.perFlow)
	}
}
