package cli

import (
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
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

	nsPosts     []string // "namespace delete=..| content-type" of namespace-endpoint requests
	nsBodies    []string
	failNS      func(ns string) bool // true -> answer 403
	bulkStatus  int                  // status of a failing bulk request (default 500)
	bulkMessage string               // body of a failing bulk request

	searchPageSize int // when set, search answers at most this many results per page
	searchCalls    int
	failSearch     func(ns string) bool // true -> answer 500
	validateStatus int                  // when set, /validate answers this status

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
			if s.validateStatus != 0 {
				w.WriteHeader(s.validateStatus)
				_, _ = w.Write([]byte(`{"message":"validate down"}`))
				return
			}
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
			s.searchCalls++
			if s.failSearch != nil && s.failSearch(ns) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"message":"search down"}`))
				return
			}
			results := []string{}
			ids := s.existing[ns]
			total := len(ids)
			if s.searchPageSize > 0 {
				page, _ := strconv.Atoi(r.URL.Query().Get("page"))
				lo := min((page-1)*s.searchPageSize, total)
				ids = ids[lo:min(lo+s.searchPageSize, total)]
			}
			for _, id := range ids {
				results = append(results, fmt.Sprintf(`{"id":%q,"namespace":%q,"revision":1,"labels":[{"key":"a","value":"b"}]}`, id, ns))
			}
			_, _ = fmt.Fprintf(w, `{"results":[%s],"total":%d}`, strings.Join(results, ","), total)
		case strings.HasSuffix(path, "/flows/bulk"):
			s.bulkBodies = append(s.bulkBodies, string(body))
			s.bulkQueries = append(s.bulkQueries, r.URL.Query())
			// Like Kestra 1.x: a repeated flow id fails the whole request, even
			// across namespaces.
			seenIDs := map[string]bool{}
			for _, doc := range strings.Split(string(body), "\n---\n") {
				_, id := docIdentity(doc)
				if seenIDs[id] {
					w.WriteHeader(http.StatusUnprocessableEntity)
					_, _ = w.Write([]byte(`{"message":"Duplicate flow id"}`))
					return
				}
				seenIDs[id] = true
			}
			s.bulkTypes = append(s.bulkTypes, r.Header.Get("Content-Type"))
			if s.failBulk != nil && s.failBulk(len(s.bulkBodies)) {
				status, msg := s.bulkStatus, s.bulkMessage
				if status == 0 {
					status, msg = http.StatusInternalServerError, `{"message":"boom"}`
				}
				w.WriteHeader(status)
				_, _ = w.Write([]byte(msg))
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
		case r.Method == http.MethodPost && strings.Contains(path, "/flows/") && len(strings.Split(strings.SplitN(path, "/flows/", 2)[1], "/")) == 1:
			ns := strings.SplitN(path, "/flows/", 2)[1]
			ct := r.Header.Get("Content-Type")
			s.nsPosts = append(s.nsPosts, ns+" delete="+r.URL.Query().Get("delete")+" override="+r.URL.Query().Get("override")+" "+strings.SplitN(ct, ";", 2)[0])
			var joined []string
			if _, params, err := mime.ParseMediaType(ct); err == nil {
				mr := multipart.NewReader(strings.NewReader(string(body)), params["boundary"])
				for {
					p, err := mr.NextPart()
					if err != nil {
						break
					}
					b, _ := io.ReadAll(p)
					joined = append(joined, string(b))
				}
			}
			body = []byte(strings.Join(joined, "\n---\n"))
			s.nsBodies = append(s.nsBodies, string(body))
			if s.failNS != nil && s.failNS(ns) {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"message":"Forbidden"}`))
				return
			}
			out := []string{}
			for _, doc := range strings.Split(string(body), "\n---\n") {
				dns, id := docIdentity(doc)
				out = append(out, fmt.Sprintf(`{"id":%q,"namespace":%q,"revision":5}`, id, dns))
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
	if len(results) != 100 || results[0].Success || !strings.Contains(results[0].Error, "outcome unknown") {
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

func TestDeployBulk_ForbiddenBulkSwitchesToNamespaceEndpoint(t *testing.T) {
	s := &bulkDeployServer{bulkStatus: http.StatusForbidden, bulkMessage: `{"message":"Forbidden"}`, failBulk: func(int) bool { return true }}
	files := numberedFlows("ns.one", 120)
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("other-%d", i)
		files[id+".yaml"] = flowYAML("ns.two", id)
	}
	dir := writeFlowDir(t, files)
	results := deployDir(t, s, dir, "", "", false, true, false)

	requireSuccess(t, results, 125)
	if len(s.bulkBodies) != 1 {
		t.Fatalf("bulk must be tried once, then skipped for later chunks; got %d", len(s.bulkBodies))
	}
	if len(s.perFlow) != 0 {
		t.Fatalf("no per-flow requests expected, got %d", len(s.perFlow))
	}
	// 125 flows = chunks of 100 + 25; grouped by namespace per chunk.
	for _, p := range s.nsPosts {
		if !strings.Contains(p, "delete=false override=false multipart/form-data") {
			t.Fatalf("namespace request must send delete=false, override=false, multipart: %s", p)
		}
	}
	if len(s.nsPosts) < 3 {
		t.Fatalf("expected one request per namespace and chunk, got %v", s.nsPosts)
	}
	if results[0].Revision != 5 {
		t.Fatalf("revision must come from the namespace response, got %d", results[0].Revision)
	}
	for _, b := range s.nsBodies {
		if strings.Contains(b, "namespace: ns.one") && strings.Contains(b, "namespace: ns.two") {
			t.Fatal("a namespace request must only hold flows of that namespace")
		}
	}
}

func TestDeployBulk_ServerErrorDoesNotSwitchEndpoint(t *testing.T) {
	s := &bulkDeployServer{failBulk: func(int) bool { return true }}
	results := deployDir(t, s, writeFlowDir(t, numberedFlows("my.ns", 3)), "", "", false, true, false)
	requireSuccess(t, results, 3)
	if len(s.nsPosts) != 0 || len(s.perFlow) == 0 {
		t.Fatalf("a 500 must go per flow, got ns=%v perFlow=%v", s.nsPosts, s.perFlow)
	}
}

func TestDeployBulk_NamespaceEndpointFailureFallsBackPerFlow(t *testing.T) {
	s := &bulkDeployServer{
		bulkStatus: http.StatusForbidden, bulkMessage: `{"message":"Forbidden"}`, failBulk: func(int) bool { return true },
		failNS:      func(string) bool { return true },
		failPerFlow: map[string]int{"flow-001": http.StatusForbidden},
	}
	results := deployDir(t, s, writeFlowDir(t, numberedFlows("my.ns", 3)), "", "", false, true, false)
	if !results[0].Success || !results[2].Success || results[1].Success || !strings.Contains(results[1].Error, "denied") {
		t.Fatalf("per-flow errors expected: %+v", results)
	}
}

func TestDeployBulk_FailFastNamespaceEndpointFailureAttributedToItsNamespace(t *testing.T) {
	s := &bulkDeployServer{
		bulkStatus: http.StatusForbidden, bulkMessage: `{"message":"Forbidden"}`, failBulk: func(int) bool { return true },
		failNS: func(ns string) bool { return ns == "qa.forbidden" },
	}
	dir := writeFlowDir(t, map[string]string{
		"a.yaml": flowYAML("qa.allowed", "a"),
		"b.yaml": flowYAML("qa.forbidden", "b"),
		"c.yaml": flowYAML("qa.zlast", "c"),
	})
	results := deployDir(t, s, dir, "", "", false, true, true)

	if len(s.perFlow) != 0 {
		t.Fatal("fail-fast must not fall back per flow")
	}
	if !results[0].Success {
		t.Fatalf("qa.allowed was written before the failure: %+v", results[0])
	}
	if results[1].Success || !strings.Contains(results[1].Error, "Forbidden") {
		t.Fatalf("qa.forbidden must carry the error: %+v", results[1])
	}
	if results[2].Success || strings.Contains(results[2].Error, "Forbidden") || !strings.Contains(results[2].Error, "skipped") {
		t.Fatalf("unattempted file must be reported as skipped: %+v", results[2])
	}
}

func TestDeployBulk_FailFastBulkErrorOnlyOnFilesItNames(t *testing.T) {
	s := &bulkDeployServer{
		bulkStatus: http.StatusBadRequest, bulkMessage: `{"message":"namespace 'qa.forbidden' is not allowed"}`,
		failBulk: func(int) bool { return true },
	}
	dir := writeFlowDir(t, map[string]string{
		"a.yaml": flowYAML("qa.allowed", "a"),
		"b.yaml": flowYAML("qa.forbidden", "b"),
	})
	results := deployDir(t, s, dir, "", "", false, true, true)
	if strings.Contains(results[0].Error, "qa.forbidden") || !strings.Contains(results[0].Error, "not deployed") {
		t.Fatalf("qa.allowed must not inherit the message: %+v", results[0])
	}
	if !strings.Contains(results[1].Error, "qa.forbidden") {
		t.Fatalf("qa.forbidden must carry the message: %+v", results[1])
	}
}

func TestDeployBulk_FailFastValidationLeavesValidFilesMarkedSkipped(t *testing.T) {
	s := &bulkDeployServer{violations: `[{"index":2,"constraints":"bad"}]`}
	results := deployDir(t, s, writeFlowDir(t, numberedFlows("my.ns", 3)), "", "", false, false, true)
	if len(results) != 3 {
		t.Fatalf("got %+v", results)
	}
	for _, r := range results[:2] {
		if r.Success || !strings.Contains(r.Error, "skipped (--fail-fast)") {
			t.Fatalf("valid but unwritten file must say why: %+v", r)
		}
	}
}

func TestPrepareFlowDeploy_SourceIsNormalized(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.yaml")
	if err := os.WriteFile(path, []byte("---\n"+flowYAML("my.ns", "f")+"\n\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, src := prepareFlowDeploy(path, "", "", false, false)
	if src != strings.TrimSpace(flowYAML("my.ns", "f")) {
		t.Fatalf("source must match what the bulk endpoint stores, got %q", src)
	}
}

func TestDeployFlow_SendsTrimmedSource(t *testing.T) {
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		b, _ := io.ReadAll(r.Body)
		got = string(b)
		_, _ = w.Write([]byte(`{"id":"f","namespace":"my.ns","revision":1,"tasks":[]}`))
	}))
	t.Cleanup(server.Close)
	path := filepath.Join(t.TempDir(), "f.yaml")
	if err := os.WriteFile(path, []byte(flowYAML("my.ns", "f")), 0o600); err != nil {
		t.Fatal(err)
	}
	if r := deployFlow(newTestClient(t, server.URL), path, "", "", false, false); !r.Success {
		t.Fatal(r.Error)
	}
	if strings.HasSuffix(got, "\n") {
		t.Fatalf("per-flow body must not keep the trailing newline: %q", got)
	}
}

func TestListFlowIDsInNamespace_FetchesEveryPage(t *testing.T) {
	s := &bulkDeployServer{searchPageSize: 2, existing: map[string][]string{"my.ns": {"a", "b", "c"}}}
	server := s.start(t)
	ids, err := listFlowIDsInNamespace(newTestClient(t, server.URL), "my.ns")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 3 || !ids["c"] {
		t.Fatalf("expected all 3 ids across pages, got %v", ids)
	}
	if s.searchCalls != 2 {
		t.Fatalf("expected 2 search pages, got %d", s.searchCalls)
	}
}

func TestDeployBulk_ExistenceSearchFailureIsPerNamespace(t *testing.T) {
	s := &bulkDeployServer{failSearch: func(ns string) bool { return ns == "ns.bad" }}
	dir := writeFlowDir(t, map[string]string{
		"a.yaml": flowYAML("ns.bad", "a"),
		"b.yaml": flowYAML("ns.bad", "b"),
		"c.yaml": flowYAML("ns.good", "c"),
	})
	results := deployDir(t, s, dir, "", "", false, false, false)
	for _, r := range results[:2] {
		if r.Success || !strings.Contains(r.Error, "failed to check whether flow") {
			t.Fatalf("expected an existence-check failure, got %+v", r)
		}
	}
	if !results[2].Success {
		t.Fatalf("the other namespace must deploy: %+v", results[2])
	}
	if s.searchCalls != 2 {
		t.Fatalf("a failing namespace must be searched once, got %d searches", s.searchCalls)
	}
	if strings.Contains(s.bulkBodies[0], "id: a") {
		t.Fatal("unchecked flows must not be written")
	}
}

func TestDeployBulk_ValidateFailureFallsBackPerFlow(t *testing.T) {
	s := &bulkDeployServer{validateStatus: http.StatusInternalServerError}
	results := deployDir(t, s, writeFlowDir(t, numberedFlows("my.ns", 3)), "", "", false, false, false)
	requireSuccess(t, results, 3)
	if len(s.bulkBodies) != 0 || len(s.perFlow) == 0 {
		t.Fatalf("expected per-flow deploys only, got bulk=%d perFlow=%v", len(s.bulkBodies), s.perFlow)
	}
}

func TestDeployBulk_FailFastNamespaceMatchedAsWholeToken(t *testing.T) {
	s := &bulkDeployServer{
		bulkStatus: http.StatusBadRequest, bulkMessage: `{"message":"namespace 'qa.forbidden' is not allowed"}`,
		failBulk: func(int) bool { return true },
	}
	dir := writeFlowDir(t, map[string]string{
		"a.yaml": flowYAML("qa", "a"),
		"b.yaml": flowYAML("qa.forbidden", "b"),
	})
	results := deployDir(t, s, dir, "", "", false, true, true)
	if strings.Contains(results[0].Error, "qa.forbidden") || !strings.Contains(results[0].Error, "rejected") {
		t.Fatalf("namespace qa must not inherit the qa.forbidden message: %+v", results[0])
	}
	if !strings.Contains(results[1].Error, "qa.forbidden") {
		t.Fatalf("qa.forbidden must carry the message: %+v", results[1])
	}
}

func TestDeployBulk_FailFastUnreadableResponseIsOutcomeUnknown(t *testing.T) {
	s := &bulkDeployServer{bulkResponse: func(string) string { return "not json" }}
	results := deployDir(t, s, writeFlowDir(t, numberedFlows("my.ns", 2)), "", "", false, true, true)
	for _, r := range results {
		if r.Success || !strings.Contains(r.Error, "outcome unknown") || strings.Contains(r.Error, "rejected") {
			t.Fatalf("expected an unknown outcome, got %+v", r)
		}
	}
	if len(s.perFlow) != 0 {
		t.Fatal("fail-fast must not retry per flow")
	}
}

func TestDeployBulk_FailFastFlowMissingFromResponseIsNotRetried(t *testing.T) {
	s := &bulkDeployServer{bulkResponse: func(string) string { return `[{"id":"flow-000","namespace":"my.ns","revision":4}]` }}
	results := deployDir(t, s, writeFlowDir(t, numberedFlows("my.ns", 2)), "", "", false, true, true)
	if !results[0].Success || results[1].Success || !strings.Contains(results[1].Error, "outcome unknown") {
		t.Fatalf("unexpected results: %+v", results)
	}
	if len(s.perFlow) != 0 {
		t.Fatalf("fail-fast must not re-send per flow, got %v", s.perFlow)
	}
}

func TestDeployBulk_FailFastMissingFlowStillRecordsTheOthersInTheResponse(t *testing.T) {
	s := &bulkDeployServer{bulkResponse: func(string) string {
		return `[{"id":"flow-001","namespace":"my.ns","revision":4},{"id":"flow-002","namespace":"my.ns","revision":6}]`
	}}
	results := deployDir(t, s, writeFlowDir(t, numberedFlows("my.ns", 3)), "", "", false, true, true)
	if len(results) != 3 {
		t.Fatalf("got %+v", results)
	}
	if results[0].Success || !strings.Contains(results[0].Error, "outcome unknown") {
		t.Fatalf("the missing flow must be unknown: %+v", results[0])
	}
	if !results[1].Success || results[1].Revision != 4 || !results[2].Success || results[2].Revision != 6 {
		t.Fatalf("flows present in the response must be recorded: %+v", results)
	}
	if len(s.perFlow) != 0 {
		t.Fatal("fail-fast must not re-send per flow")
	}
}

func TestDeployBulk_SameFlowIDAcrossNamespacesUsesSeparateRequests(t *testing.T) {
	s := &bulkDeployServer{}
	files := map[string]string{
		"a.yaml": flowYAML("ns.a", "hello"),
		"b.yaml": flowYAML("ns.b", "hello"),
		"c.yaml": flowYAML("ns.c", "hello"),
	}
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("other-%d", i)
		files[id+".yaml"] = flowYAML("ns.a", id)
	}
	results := deployDir(t, s, writeFlowDir(t, files), "", "", false, true, false)

	requireSuccess(t, results, 8)
	if len(s.bulkBodies) != 3 {
		t.Fatalf("hello x3 needs exactly 3 bulk requests, got %d", len(s.bulkBodies))
	}
	if len(s.perFlow) != 0 || len(s.nsPosts) != 0 {
		t.Fatalf("no fallback expected: perFlow=%v ns=%v", s.perFlow, s.nsPosts)
	}
}

func TestBuildBulkChunks_CapsSizeAndKeepsIDsUnique(t *testing.T) {
	results := make([]DeployResult, 0, 205)
	items := make([]bulkDeployItem, 0, 205)
	for i := 0; i < 205; i++ {
		results = append(results, DeployResult{FlowID: fmt.Sprintf("f%d", i%150)})
		items = append(items, bulkDeployItem{index: i})
	}
	chunks := buildBulkChunks(results, items)
	total := 0
	for _, c := range chunks {
		if len(c) > validateBatchSize {
			t.Fatalf("chunk of %d exceeds the cap", len(c))
		}
		seen := map[string]bool{}
		for _, it := range c {
			id := results[it.index].FlowID
			if seen[id] {
				t.Fatalf("id %s repeated in a chunk", id)
			}
			seen[id] = true
		}
		total += len(c)
	}
	if total != 205 || len(chunks) != 3 {
		t.Fatalf("expected 205 flows in 3 chunks, got %d in %d", total, len(chunks))
	}
}

func TestDeployBulk_SourceWithSeparatorStaysOutOfBulkBody(t *testing.T) {
	s := &bulkDeployServer{}
	files := numberedFlows("my.ns", 2)
	files["noted.yaml"] = flowYAML("my.ns", "noted") + "# --- triggers ---\n"
	results := deployDir(t, s, writeFlowDir(t, files), "", "", false, true, false)

	requireSuccess(t, results, 3)
	for _, b := range s.bulkBodies {
		if strings.Contains(b, "--- triggers ---") || strings.Contains(b, "id: noted") {
			t.Fatalf("a source holding --- must not be in a bulk body: %s", b)
		}
	}
	if len(s.bulkBodies) != 1 || len(s.nsPosts) != 1 || !strings.Contains(s.nsBodies[0], "id: noted") {
		t.Fatalf("expected 1 bulk + 1 namespace request, got bulk=%d ns=%v", len(s.bulkBodies), s.nsPosts)
	}
	if len(s.perFlow) != 0 {
		t.Fatalf("no per-flow requests expected, got %v", s.perFlow)
	}
}
