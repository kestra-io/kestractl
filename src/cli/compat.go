package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
)

// Kestra 1.x compatibility.
//
// kestractl v2 targets Kestra 2.x, and stays usable against a 1.3 server on a
// best-effort basis. Two things make that work:
//
//   - compatTransport fills in response fields the 2.0 SDK marks as required
//     but a 1.x server never emits (a 1.x flow has no `draft`, a 1.x execution
//     summary has no `tenantId`). Without it the generated decoder rejects the
//     whole body with "no value given for required property draft".
//   - requireKestra2 refuses features that do not exist server-side before 2.0,
//     instead of letting the server silently ignore the field (a 1.x server
//     drops an unknown `instanceOwner` or `quotas` from the request body and
//     returns 200).

// newCompatHTTPClient returns the HTTP client shared by both SDK clients, so the
// response shim applies to generated and hand-written endpoints alike.
//
// legacyServer reports whether the server runs Kestra 1.x; it is wired to
// Client.ServerVersion once the client exists, and the shim stays inert until
// then and against 2.x servers.
func newCompatHTTPClient() (*http.Client, *compatTransport) {
	transport := &compatTransport{base: http.DefaultTransport, legacyServer: func() bool { return false }}
	return &http.Client{Transport: transport}, transport
}

type compatTransport struct {
	base         http.RoundTripper
	legacyServer func() bool
}

// compatMarkers are the keys a body must contain for fillCompatDefaults to have
// anything to do. Checking them before anything else keeps the shim off the hot
// path: most responses are returned without being decoded at all.
var compatMarkers = [][]byte{[]byte(`"tasks"`), []byte(`"flowRevision"`)}

func (t *compatTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 || !isJSONResponse(resp) {
		return resp, err
	}
	tenant, shimmed := shimmedEndpoint(req.URL.Path)
	if !shimmed {
		return resp, nil
	}

	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, err
	}

	// The version probe runs through this same transport, so check the cheap
	// marker scan first and only then ask which server we are talking to.
	if containsAny(body, compatMarkers) && t.legacyServer() {
		// UseNumber keeps integers verbatim: decoding into float64 would round
		// anything above 2^53 (epoch nanos, counters) on the way back out.
		dec := json.NewDecoder(bytes.NewReader(body))
		dec.UseNumber()
		var payload any
		if dec.Decode(&payload) == nil && fillCompatDefaults(payload, tenant) {
			if patched, err := json.Marshal(payload); err == nil {
				body = patched
				resp.Header.Del("Content-Length")
			}
		}
	}

	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	return resp, nil
}

func isJSONResponse(resp *http.Response) bool {
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		return false
	}
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}

func containsAny(body []byte, needles [][]byte) bool {
	for _, n := range needles {
		if bytes.Contains(body, n) {
			return true
		}
	}
	return false
}

// shimmedEndpoint reports whether a request path is one whose response can
// carry flows or executions — /api/v1/{tenant}/flows/... and
// /api/v1/{tenant}/executions/... — and returns that tenant. Restricting the
// shim to these keeps it away from bodies that merely look like a flow, such
// as a user's own JSON stored in the KV store. The tenant comes from the path
// rather than the client because flows usage-report walks several tenants.
func shimmedEndpoint(path string) (tenant string, ok bool) {
	rest := strings.TrimPrefix(path, "/api/v1/")
	if rest == path {
		return "", false
	}
	tenant, rest, _ = strings.Cut(rest, "/")
	resource, _, _ := strings.Cut(rest, "/")
	return tenant, resource == "flows" || resource == "executions"
}

// fillCompatDefaults walks a decoded JSON payload and adds the fields a 2.0
// decoder requires but a 1.x server omits. It reports whether anything changed.
//
// A flow is recognised by its id/namespace/tasks triple (Flow, FlowWithSource,
// FlowForExecution, in a list or a page). An execution by flowId/flowRevision/
// state (Execution, ApiExecution). A flow on a 1.x server is never a draft.
func fillCompatDefaults(v any, tenant string) bool {
	changed := false
	switch node := v.(type) {
	case map[string]any:
		if jsonHasKeys(node, "id", "namespace", "tasks") {
			changed = setJSONDefault(node, "draft", false) || changed
		}
		if jsonHasKeys(node, "flowId", "flowRevision", "state") {
			changed = setJSONDefault(node, "tenantId", tenant) || changed
		}
		for _, child := range node {
			changed = fillCompatDefaults(child, tenant) || changed
		}
	case []any:
		for _, child := range node {
			changed = fillCompatDefaults(child, tenant) || changed
		}
	}
	return changed
}

func jsonHasKeys(m map[string]any, keys ...string) bool {
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			return false
		}
	}
	return true
}

func setJSONDefault(m map[string]any, key string, value any) bool {
	if _, ok := m[key]; ok {
		return false
	}
	m[key] = value
	return true
}

// ServerVersion returns the Kestra version reported by the configuration
// endpoint. It is fetched once per process; a failure is cached too, since a
// CLI invocation has no later point at which a retry would help.
func (c *Client) ServerVersion() (string, error) {
	c.serverVersionOnce.Do(func() {
		c.serverVersion, c.serverVersionErr = fetchKestraVersion(c)
	})
	return c.serverVersion, c.serverVersionErr
}

// isLegacyServer reports whether the server is known to run Kestra 1.x. An
// unreachable or unparseable version (develop builds, snapshots) counts as
// current: the shims exist for 1.x servers, not for unknown ones.
func (c *Client) isLegacyServer() bool {
	version, err := c.ServerVersion()
	if err != nil {
		return false
	}
	core, _ := splitVersion(version)
	return core != [3]int{} && core[0] < 2
}

// requireKestra2 fails when the server is known to run Kestra 1.x, naming the
// feature that needs 2.0. It does not block unknown servers — a real auth or
// network problem surfaces on the very next call anyway; it exists to turn a
// silent no-op on 1.x into an error.
func requireKestra2(client *Client, feature string) error {
	if !client.isLegacyServer() {
		return nil
	}
	version, _ := client.ServerVersion()
	return fmt.Errorf("%s is only available on Kestra 2.0 or later (the server runs %s)", feature, version)
}
