package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// flow13 is a flow as a Kestra 1.3 server returns it: no `draft`.
const flow13 = `{"id":"f","namespace":"ns","revision":1,"disabled":false,"deleted":false,"tasks":[],"source":"id: f"}`

// execution13 is an execution summary as a Kestra 1.3 server returns it: no `tenantId`.
const execution13 = `{"id":"e","originalId":"e","deleted":false,"metadata":{"attemptNumber":1,"originalCreatedDate":"2026-09-03T12:54:30Z"},"namespace":"ns","flowId":"f","flowRevision":1,"state":{"current":"SUCCESS","histories":[],"duration":"PT1S"}}`

func TestFillCompatDefaults(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		changed bool
	}{
		{
			name:    "flow without draft gets draft=false",
			in:      `{"id":"f","namespace":"ns","tasks":[]}`,
			want:    `{"draft":false,"id":"f","namespace":"ns","tasks":[]}`,
			changed: true,
		},
		{
			name:    "flow with draft is left alone",
			in:      `{"id":"f","namespace":"ns","tasks":[],"draft":true}`,
			want:    `{"draft":true,"id":"f","namespace":"ns","tasks":[]}`,
			changed: false,
		},
		{
			name:    "flows nested in a page and in a list",
			in:      `{"results":[{"id":"a","namespace":"ns","tasks":[]}],"total":1}`,
			want:    `{"results":[{"draft":false,"id":"a","namespace":"ns","tasks":[]}],"total":1}`,
			changed: true,
		},
		{
			name:    "execution without tenantId gets the request tenant",
			in:      `{"id":"e","flowId":"f","flowRevision":1,"state":{"current":"SUCCESS"}}`,
			want:    `{"flowId":"f","flowRevision":1,"id":"e","state":{"current":"SUCCESS"},"tenantId":"acme"}`,
			changed: true,
		},
		{
			name:    "namespace is not a flow",
			in:      `{"id":"ns","deleted":false}`,
			want:    `{"deleted":false,"id":"ns"}`,
			changed: false,
		},
		{
			name:    "trigger is not an execution",
			in:      `{"flowId":"f","namespace":"ns","triggerId":"t"}`,
			want:    `{"flowId":"f","namespace":"ns","triggerId":"t"}`,
			changed: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v any
			if err := json.Unmarshal([]byte(tt.in), &v); err != nil {
				t.Fatal(err)
			}
			if got := fillCompatDefaults(v, "acme"); got != tt.changed {
				t.Fatalf("changed = %v, want %v", got, tt.changed)
			}
			out, _ := json.Marshal(v)
			if string(out) != tt.want {
				t.Fatalf("got  %s\nwant %s", out, tt.want)
			}
		})
	}
}

func TestShimmedEndpoint(t *testing.T) {
	tests := []struct {
		path   string
		tenant string
		ok     bool
	}{
		{"/api/v1/acme/executions/search", "acme", true},
		{"/api/v1/main/flows/ns/f", "main", true},
		{"/api/v1/main/executions/flows/ns/f", "main", true},
		{"/api/v1/main/namespaces/ns/kv/key", "main", false},
		{"/api/v1/main/triggers/search", "main", false},
		{"/api/v1/configs", "configs", false},
		{"/api/v1/blueprints/community/flow/x", "blueprints", false},
		{"/health", "", false},
	}
	for _, tt := range tests {
		tenant, ok := shimmedEndpoint(tt.path)
		if tenant != tt.tenant || ok != tt.ok {
			t.Errorf("shimmedEndpoint(%q) = (%q, %v), want (%q, %v)", tt.path, tenant, ok, tt.tenant, tt.ok)
		}
	}
}

// versionHandler answers the /api/v1/configs probe with the given version and
// delegates everything else.
func versionHandler(version string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/configs" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"version":"` + version + `"}`))
			return
		}
		next(w, r)
	}
}

func TestCompatTransport_DecodesKestra13Flows(t *testing.T) {
	server := httptest.NewServer(versionHandler("1.3.35", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/revisions"):
			_, _ = w.Write([]byte("[" + flow13 + "]"))
		case strings.HasSuffix(r.URL.Path, "/flows/search"):
			_, _ = w.Write([]byte(`{"results":[` + flow13 + `],"total":1}`))
		default:
			_, _ = w.Write([]byte(flow13))
		}
	}))
	t.Cleanup(server.Close)
	client := newTestClient(t, server.URL)

	revisions, _, err := client.API.FlowsAPI.ListFlowRevisions(client.Ctx, "ns", "f", "main").Execute()
	if err != nil {
		t.Fatalf("revisions: %v", err)
	}
	if len(revisions) != 1 || revisions[0].GetId() != "f" || revisions[0].GetDraft() {
		t.Fatalf("unexpected revisions: %+v", revisions)
	}

	page, _, err := client.API.FlowsAPI.SearchFlows(client.Ctx, "main").Execute()
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if page.GetTotal() != 1 || len(page.GetResults()) != 1 {
		t.Fatalf("unexpected page: %+v", page)
	}

	flow, _, err := client.API.FlowsAPI.Flow(client.Ctx, "ns", "f", "main").Execute()
	if err != nil {
		t.Fatalf("flow: %v", err)
	}
	if flow.GetSource() != "id: f" {
		t.Fatalf("unexpected flow: %+v", flow)
	}
}

func TestCompatTransport_DecodesKestra13ExecutionPageWithRequestTenant(t *testing.T) {
	server := httptest.NewServer(versionHandler("1.3.35", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[` + execution13 + `],"total":1}`))
	}))
	t.Cleanup(server.Close)
	client := newTestClient(t, server.URL)

	// The client's own tenant is "main"; the executions belong to the tenant
	// in the request path.
	page, err := client.Kestra.Executions().SearchExecutions(client.Ctx, "acme", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("search executions: %v", err)
	}
	results := page.GetResults()
	if len(results) != 1 || results[0].GetTenantId() != "acme" {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestCompatTransport_PassesBodiesThroughUntouched(t *testing.T) {
	// Bodies come back byte-identical, large integers included, whenever the
	// shim has no business touching them: non-JSON, error statuses, endpoints
	// other than flows/executions (a KV value may look exactly like a flow),
	// and every response from a 2.x server.
	flowish := `{"id":"x","namespace":"y","tasks":[{"id":"t"}],"big":9007199254740993}`
	logs := `{"results":[{"executionId":"e","timestamp":1756789012345678901,"level":"INFO"}],"total":1}`
	sse := "data: {\"id\":\"e\",\"flowId\":\"f\",\"flowRevision\":1,\"state\":{}}\n\n"
	zip := "PK\x03\x04not-json"

	newServer := func(version string) *httptest.Server {
		server := httptest.NewServer(versionHandler(version, func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasSuffix(r.URL.Path, "/sse"):
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte(sse))
			case strings.HasSuffix(r.URL.Path, "/zip"):
				w.Header().Set("Content-Type", "application/zip")
				_, _ = w.Write([]byte(zip))
			case strings.HasSuffix(r.URL.Path, "/logs"):
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(logs))
			case strings.HasSuffix(r.URL.Path, "/missing"):
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(flowish))
			default:
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(flowish))
			}
		}))
		t.Cleanup(server.Close)
		return server
	}

	cases := []struct {
		name, version, path, want string
	}{
		{"1.x kv value shaped like a flow", "1.3.35", "/api/v1/main/namespaces/ns/kv/k", flowish},
		{"1.x logs page", "1.3.35", "/api/v1/main/logs", logs},
		{"1.x SSE on an executions endpoint", "1.3.35", "/api/v1/main/executions/e/sse", sse},
		{"1.x zip on a flows endpoint", "1.3.35", "/api/v1/main/flows/zip", zip},
		{"1.x error body on a flows endpoint", "1.3.35", "/api/v1/main/flows/missing", flowish},
		{"2.x flow endpoint", "2.0.0-rc13", "/api/v1/main/flows/ns/f", flowish},
		{"2.x executions endpoint", "2.0.0-rc13", "/api/v1/main/executions/search", flowish},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			server := newServer(tt.version)
			client := newTestClient(t, server.URL)
			resp, err := client.API.GetConfig().HTTPClient.Get(server.URL + tt.path)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			if string(body) != tt.want {
				t.Errorf("body altered\ngot  %q\nwant %q", body, tt.want)
			}
		})
	}
}

func TestCompatTransport_KeepsLargeIntegersWhenPatching(t *testing.T) {
	server := httptest.NewServer(versionHandler("1.3.35", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"f","namespace":"ns","tasks":[],"big":9007199254740993}`))
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	resp, err := client.API.GetConfig().HTTPClient.Get(server.URL + "/api/v1/main/flows/ns/f")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), `"big":9007199254740993`) || !strings.Contains(string(body), `"draft":false`) {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestRequireKestra2(t *testing.T) {
	tests := []struct {
		name    string
		version string
		status  int
		wantErr string
	}{
		{name: "1.3 is refused", version: "1.3.35", status: 200, wantErr: "quotas is only available on Kestra 2.0 or later (the server runs 1.3.35)"},
		{name: "2.0 rc is allowed", version: "2.0.0-rc13", status: 200},
		{name: "2.1 is allowed", version: "2.1.0", status: 200},
		{name: "develop build is not blocked", version: "develop-SNAPSHOT", status: 200},
		{name: "unreachable version endpoint is not blocked", status: 500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(`{"version":"` + tt.version + `"}`))
			}))
			t.Cleanup(server.Close)
			client := newTestClient(t, server.URL)

			err := requireKestra2(client, "quotas")
			if tt.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr != "" && (err == nil || err.Error() != tt.wantErr) {
				t.Fatalf("got %v, want %q", err, tt.wantErr)
			}

			// The version is fetched once per client, not once per guard.
			_ = requireKestra2(client, "quotas")
			if calls.Load() != 1 {
				t.Fatalf("configuration endpoint called %d times, want 1", calls.Load())
			}
		})
	}
}

// newKestra13Server answers the version probe as a 1.3 server and fails the
// test on any other request, proving a guarded command never reaches the API.
func newKestra13Server(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/configs" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"1.3.35"}`))
	}))
	t.Cleanup(server.Close)
	return server
}

func TestGuardedCommands_RefuseKestra2FeaturesOn13(t *testing.T) {
	concurrency, err := parseConcurrencyFlags(10, true, "QUEUE", true)
	if err != nil {
		t.Fatal(err)
	}
	superAdmin := userMutation{superAdmin: true, superAdminSet: true}

	tests := map[string]func(*Client) error{
		"tenants create --concurrency-limit": func(c *Client) error {
			return runTenantsCreate(c, "t", "t", concurrency, nil, newTableRenderer(io.Discard))
		},
		"namespaces create --concurrency-limit": func(c *Client) error {
			return runNamespacesCreate(c, "ns", "", nil, concurrency, nil, newTableRenderer(io.Discard))
		},
		"users create --superadmin": func(c *Client) error {
			return runUsersCreate(c, superAdmin, newTableRenderer(io.Discard))
		},
		"users set-super-admin": func(c *Client) error {
			return runUsersPatchSuperAdmin(c, "u1", true, newTableRenderer(io.Discard))
		},
		"invitations create --superadmin": func(c *Client) error {
			return runInvitationsCreate(c, "a@b.c", nil, nil, true, false, newTableRenderer(io.Discard))
		},
	}
	for name, run := range tests {
		t.Run(name, func(t *testing.T) {
			err := run(newTestClient(t, newKestra13Server(t).URL))
			if err == nil || !strings.Contains(err.Error(), "only available on Kestra 2.0 or later") {
				t.Fatalf("expected a Kestra 2.0 guard error, got %v", err)
			}
		})
	}
}

func TestSplitExecutionAction(t *testing.T) {
	const id = "2TLGqHrXC9k8BczKJe5djX"

	tests := []struct {
		name   string
		path   string
		prefix string
		action string
		ok     bool
	}{
		{
			name:   "1.x action path",
			path:   "/api/v1/main/executions/" + id + "/kill",
			prefix: "/api/v1/main/executions/" + id,
			action: "kill",
			ok:     true,
		},
		{
			name:   "2.0 action path",
			path:   "/api/v1/main/executions/" + id + "/actions/replay",
			prefix: "/api/v1/main/executions/" + id,
			action: "replay",
			ok:     true,
		},
		{
			name:   "hyphenated action",
			path:   "/api/v1/acme/executions/" + id + "/replay-with-inputs",
			prefix: "/api/v1/acme/executions/" + id,
			action: "replay-with-inputs",
			ok:     true,
		},
		// Subresources that did not move in 2.0 must not be rewritten.
		{name: "graph", path: "/api/v1/main/executions/" + id + "/graph"},
		{name: "flow", path: "/api/v1/main/executions/" + id + "/flow"},
		{name: "follow", path: "/api/v1/main/executions/" + id + "/follow"},
		{name: "file metas", path: "/api/v1/main/executions/" + id + "/file/metas"},
		{name: "the execution itself", path: "/api/v1/main/executions/" + id},
		// Bulk variants keep their 1.x shape on both servers.
		{name: "bulk by-ids", path: "/api/v1/main/executions/kill/by-ids"},
		{name: "bulk by-query", path: "/api/v1/main/executions/replay/by-query"},
		// POST /executions/{namespace}/{flowId} creates an execution: a flow
		// named after an action must not be mistaken for one.
		{name: "create execution for a flow named kill", path: "/api/v1/main/executions/qa.compat/kill"},
		{name: "create execution in a short namespace", path: "/api/v1/main/executions/qa/kill"},
		{name: "other resource", path: "/api/v1/main/flows/" + id + "/kill"},
		{name: "not an api path", path: "/executions/" + id + "/kill"},
		{name: "unknown action", path: "/api/v1/main/executions/" + id + "/teleport"},
		{name: "too deep", path: "/api/v1/main/executions/" + id + "/actions/kill/extra"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefix, action, ok := splitExecutionAction(tt.path)
			if ok != tt.ok || prefix != tt.prefix || action != tt.action {
				t.Fatalf("got (%q, %q, %v), want (%q, %q, %v)", prefix, action, ok, tt.prefix, tt.action, tt.ok)
			}
		})
	}
}

// TestCompatTransport_RoutesExecutionActionsPerServerVersion pins the wire path
// for both the SDK's generated executions endpoints (which emit 1.x paths) and
// its hand-written ones (which emit 2.0 /actions/ paths): each must come out as
// the shape the server in front of it actually routes.
func TestCompatTransport_RoutesExecutionActionsPerServerVersion(t *testing.T) {
	const id = "2TLGqHrXC9k8BczKJe5djX"

	tests := []struct {
		name    string
		version string
		call    func(*Client) error
		want    string
	}{
		{
			name:    "generated kill on 1.x keeps the bare path",
			version: "1.3.35",
			call: func(c *Client) error {
				_, _, err := c.API.ExecutionsAPI.KillExecution(c.Ctx, id, "main").Execute()
				return err
			},
			want: "/api/v1/main/executions/" + id + "/kill",
		},
		{
			name:    "generated kill on 2.0 moves under actions",
			version: "2.0.0-rc13",
			call: func(c *Client) error {
				_, _, err := c.API.ExecutionsAPI.KillExecution(c.Ctx, id, "main").Execute()
				return err
			},
			want: "/api/v1/main/executions/" + id + "/actions/kill",
		},
		{
			name:    "generated replay on 2.0 moves under actions",
			version: "2.0.0-rc13",
			call: func(c *Client) error {
				_, _, err := c.API.ExecutionsAPI.ReplayExecution(c.Ctx, id, "main").Execute()
				return err
			},
			want: "/api/v1/main/executions/" + id + "/actions/replay",
		},
		{
			name:    "hand-written eval on 1.x drops actions",
			version: "1.3.35",
			call: func(c *Client) error {
				_, err := c.Kestra.Executions().EvalExpression(c.Ctx, id, "main", "{{ flow.id }}")
				return err
			},
			want: "/api/v1/main/executions/" + id + "/eval",
		},
		{
			name:    "hand-written eval on 2.0 keeps actions",
			version: "2.0.0-rc13",
			call: func(c *Client) error {
				_, err := c.Kestra.Executions().EvalExpression(c.Ctx, id, "main", "{{ flow.id }}")
				return err
			},
			want: "/api/v1/main/executions/" + id + "/actions/eval",
		},
		{
			name:    "graph is not an action on 2.0",
			version: "2.0.0-rc13",
			call: func(c *Client) error {
				_, _, err := c.API.ExecutionsAPI.ExecutionFlowGraph(c.Ctx, id, "main").Execute()
				return err
			},
			want: "/api/v1/main/executions/" + id + "/graph",
		},
		{
			name:    "bulk kill keeps its path on 2.0",
			version: "2.0.0-rc13",
			call: func(c *Client) error {
				_, _, err := c.API.ExecutionsAPI.KillExecutionsByIds(c.Ctx, "main").RequestBody([]string{id}).Execute()
				return err
			},
			want: "/api/v1/main/executions/kill/by-ids",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			server := httptest.NewServer(versionHandler(tt.version, func(w http.ResponseWriter, r *http.Request) {
				got = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{}`))
			}))
			t.Cleanup(server.Close)

			// The response is deliberately empty, so a decode error is fine:
			// the assertion is on the path the server was asked for.
			_ = tt.call(newTestClient(t, server.URL))
			if got != tt.want {
				t.Fatalf("requested %q, want %q", got, tt.want)
			}
		})
	}
}

// TestCompatTransport_DoesNotMutateCallerRequest guards the net/http contract:
// RoundTrip must leave the request it was handed untouched.
func TestCompatTransport_DoesNotMutateCallerRequest(t *testing.T) {
	const path = "/api/v1/main/executions/2TLGqHrXC9k8BczKJe5djX/kill"

	server := httptest.NewServer(versionHandler("2.0.0-rc13", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	req, err := http.NewRequest(http.MethodDelete, server.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.API.GetConfig().HTTPClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	if req.URL.Path != path {
		t.Fatalf("caller request was mutated: %q", req.URL.Path)
	}
}

func TestMirrorBulkCount(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{
			name: "2.0 async operation response gains count",
			in:   `{"operationId":"2gtoTYnQy1KHopwSQAXE3y","totalItems":3}`,
			want: `{"count":3,"operationId":"2gtoTYnQy1KHopwSQAXE3y","totalItems":3}`,
			ok:   true,
		},
		{
			name: "1.x response gains totalItems",
			in:   `{"count":3}`,
			want: `{"count":3,"totalItems":3}`,
			ok:   true,
		},
		{
			name: "a response carrying both is untouched",
			in:   `{"count":1,"totalItems":3}`,
			want: `{"count":1,"totalItems":3}`,
		},
		{
			name: "neither key is untouched",
			in:   `{"operationId":"x"}`,
			want: `{"operationId":"x"}`,
		},
		{
			name: "a large count round-trips verbatim",
			in:   `{"totalItems":9007199254740993}`,
			want: `{"count":9007199254740993,"totalItems":9007199254740993}`,
			ok:   true,
		},
		{
			// A substring scan of the body would read the nested `count` as
			// "already present" and skip the mirroring.
			name: "a nested count does not mask a missing top-level one",
			in:   `{"operationId":"x","totalItems":3,"failures":[{"count":1}]}`,
			want: `{"count":3,"failures":[{"count":1}],"operationId":"x","totalItems":3}`,
			ok:   true,
		},
		{
			name: "non-object body is untouched",
			in:   `["totalItems"]`,
			want: `["totalItems"]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := mirrorBulkCount([]byte(tt.in))
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if string(got) != tt.want {
				t.Fatalf("got  %s\nwant %s", got, tt.want)
			}
		})
	}
}

// TestRunExecutionsBulkOp_CountsKestra20AsyncResponse covers the whole path: a
// 2.0 bulk endpoint answers {"operationId":..., "totalItems":N} and the command
// must report N, not the 0 the SDK's count-only model would decode.
func TestRunExecutionsBulkOp_CountsKestra20AsyncResponse(t *testing.T) {
	for _, tt := range []struct {
		version, body string
	}{
		{"2.0.0-rc13", `{"operationId":"2gtoTYnQy1KHopwSQAXE3y","totalItems":2}`},
		{"1.3.35", `{"count":2}`},
	} {
		t.Run(tt.version, func(t *testing.T) {
			server := httptest.NewServer(versionHandler(tt.version, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.body))
			}))
			t.Cleanup(server.Close)

			var out strings.Builder
			renderer, err := NewRenderer("table", &out)
			if err != nil {
				t.Fatal(err)
			}
			err = runExecutionsBulkOp(newTestClient(t, server.URL), []string{"a", "b"}, "kill", renderer)
			if err != nil {
				t.Fatalf("bulk kill: %v", err)
			}
			if !strings.Contains(out.String(), "2 execution(s) affected") {
				t.Fatalf("unexpected output: %q", out.String())
			}
		})
	}
}

// TestCompatTransport_UnknownVersionFallsBackToLegacyPath covers the server
// whose version cannot be determined (a develop build, or a /configs the user
// cannot read): the 2.0 shape is tried first because that is what this binary
// targets, and a route miss falls back to the 1.x shape rather than failing.
func TestCompatTransport_UnknownVersionFallsBackToLegacyPath(t *testing.T) {
	const id = "2TLGqHrXC9k8BczKJe5djX"

	tests := []struct {
		name        string
		configs     func(http.ResponseWriter)
		routes      map[string]int // path -> status
		wantOrder   []string
		wantSuccess bool
	}{
		{
			name:    "develop build serving only the 1.x path",
			configs: func(w http.ResponseWriter) { _, _ = w.Write([]byte(`{"version":"develop"}`)) },
			routes: map[string]int{
				"/api/v1/main/executions/" + id + "/actions/kill": http.StatusNotFound,
				"/api/v1/main/executions/" + id + "/kill":         http.StatusOK,
			},
			wantOrder: []string{
				"/api/v1/main/executions/" + id + "/actions/kill",
				"/api/v1/main/executions/" + id + "/kill",
			},
			wantSuccess: true,
		},
		{
			name: "unreadable configs, server serving only the 1.x path",
			configs: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"message":"Forbidden"}`))
			},
			routes: map[string]int{
				"/api/v1/main/executions/" + id + "/actions/kill": http.StatusForbidden,
				"/api/v1/main/executions/" + id + "/kill":         http.StatusOK,
			},
			wantOrder: []string{
				"/api/v1/main/executions/" + id + "/actions/kill",
				"/api/v1/main/executions/" + id + "/kill",
			},
			wantSuccess: true,
		},
		{
			name:    "2.0 path wins on the first try, no fallback request",
			configs: func(w http.ResponseWriter) { _, _ = w.Write([]byte(`{"version":"develop"}`)) },
			routes: map[string]int{
				"/api/v1/main/executions/" + id + "/actions/kill": http.StatusOK,
			},
			wantOrder:   []string{"/api/v1/main/executions/" + id + "/actions/kill"},
			wantSuccess: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seen []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/api/v1/configs" {
					tt.configs(w)
					return
				}
				seen = append(seen, r.URL.Path)
				status, ok := tt.routes[r.URL.Path]
				if !ok {
					status = http.StatusNotFound
				}
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{}`))
			}))
			t.Cleanup(server.Close)

			client := newTestClient(t, server.URL)
			_, resp, _ := client.API.ExecutionsAPI.KillExecution(client.Ctx, id, "main").Execute()

			if strings.Join(seen, " -> ") != strings.Join(tt.wantOrder, " -> ") {
				t.Fatalf("requests\ngot  %v\nwant %v", seen, tt.wantOrder)
			}
			if tt.wantSuccess && (resp == nil || resp.StatusCode != http.StatusOK) {
				t.Fatalf("expected the fallback to succeed, got %+v", resp)
			}
		})
	}
}

// TestCompatTransport_UnknownVersionKeepsFirstErrorWhenBothMiss pins which
// error survives when neither shape routes: the 2.0 one, since that is the
// server this binary targets.
func TestCompatTransport_UnknownVersionKeepsFirstErrorWhenBothMiss(t *testing.T) {
	const id = "2TLGqHrXC9k8BczKJe5djX"

	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/configs" {
			_, _ = w.Write([]byte(`{"version":"develop"}`))
			return
		}
		seen = append(seen, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"detail":"` + r.URL.Path + `"}`))
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	_, resp, _ := client.API.ExecutionsAPI.KillExecution(client.Ctx, id, "main").Execute()

	if len(seen) != 2 {
		t.Fatalf("expected both shapes to be tried, got %v", seen)
	}
	if resp == nil || !strings.HasSuffix(resp.Request.URL.Path, "/actions/kill") {
		t.Fatalf("expected the 2.0 response to be kept, got %+v", resp)
	}
}

// TestCompatTransport_KnownVersionDoesNotRetry keeps the fallback off the path
// for a server whose version is known: a 404 there is a real missing execution,
// not a route miss, and a second request would only muddy the error.
func TestCompatTransport_KnownVersionDoesNotRetry(t *testing.T) {
	const id = "2TLGqHrXC9k8BczKJe5djX"

	for _, version := range []string{"2.0.0-rc13", "1.3.35"} {
		t.Run(version, func(t *testing.T) {
			var seen []string
			server := httptest.NewServer(versionHandler(version, func(w http.ResponseWriter, r *http.Request) {
				seen = append(seen, r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"detail":"Execution not found"}`))
			}))
			t.Cleanup(server.Close)

			client := newTestClient(t, server.URL)
			_, _, _ = client.API.ExecutionsAPI.KillExecution(client.Ctx, id, "main").Execute()

			if len(seen) != 1 {
				t.Fatalf("expected exactly one request, got %v", seen)
			}
		})
	}
}

// TestCompatTransport_CreateExecutionIsNotRewritten guards the collision
// between POST /executions/{namespace}/{flowId} and a 1.x action call: a flow
// named after an action, in a namespace long enough to look like an execution
// id, must still be triggered rather than rewritten to an actions path.
func TestCompatTransport_CreateExecutionIsNotRewritten(t *testing.T) {
	var seen string
	server := httptest.NewServer(versionHandler("2.0.0-rc13", func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	// "productionanalytics" is 19 dot-free alphanumerics, so it passes
	// looksLikeExecutionID; "pause" is in the action set.
	ctx := withoutExecutionActionRewrite(client.Ctx)
	_, _, _ = client.API.ExecutionsAPI.CreateExecution(ctx, "productionanalytics", "pause", "main").Execute()

	if seen != "/api/v1/main/executions/productionanalytics/pause" {
		t.Fatalf("create-execution path was rewritten to %q", seen)
	}
}

// TestRunExecutionsByQueryOp_NormalizesOutputAcrossVersions pins the rendered
// output for the by-query commands: the same shape on both servers, and no
// field the server did not send (the compat transport mirrors a count key into
// the body, which must not reach the user's JSON).
func TestRunExecutionsByQueryOp_NormalizesOutputAcrossVersions(t *testing.T) {
	for _, tt := range []struct {
		version, body string
	}{
		{"2.0.0-rc13", `{"operationId":"2gtoTYnQy1KHopwSQAXE3y","totalItems":4}`},
		{"1.3.35", `{"count":4}`},
	} {
		t.Run(tt.version, func(t *testing.T) {
			server := httptest.NewServer(versionHandler(tt.version, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.body))
			}))
			t.Cleanup(server.Close)

			var out strings.Builder
			renderer, err := NewRenderer("json", &out)
			if err != nil {
				t.Fatal(err)
			}
			if err := runExecutionsByQueryOp(newTestClient(t, server.URL), "kill", nil, renderer); err != nil {
				t.Fatalf("by-query kill: %v", err)
			}

			var got map[string]any
			if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
				t.Fatalf("output is not JSON: %q", out.String())
			}
			if got["operation"] != "kill" || got["count"] != float64(4) {
				t.Fatalf("unexpected payload: %v", got)
			}
			// Neither the server's own spelling nor the mirrored one leaks.
			for _, leaked := range []string{"totalItems", "operationId"} {
				if _, ok := got[leaked]; ok {
					t.Errorf("%q leaked into rendered output: %v", leaked, got)
				}
			}
		})
	}
}
