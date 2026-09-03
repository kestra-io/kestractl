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
