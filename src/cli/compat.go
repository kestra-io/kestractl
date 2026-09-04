package cli

import (
	"bytes"
	"context"
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
// best-effort basis. Four things make that work:
//
//   - compatTransport fills in response fields the 2.0 SDK marks as required
//     but a 1.x server never emits (a 1.x flow has no `draft`, a 1.x execution
//     summary has no `tenantId`). Without it the generated decoder rejects the
//     whole body with "no value given for required property draft".
//   - requireKestra2 refuses features that do not exist server-side before 2.0,
//     instead of letting the server silently ignore the field (a 1.x server
//     drops an unknown `instanceOwner` or `quotas` from the request body and
//     returns 200).
//   - compatTransport mirrors a bulk response's item count across the two
//     spellings the servers use — `count` on 1.x, `totalItems` on 2.0 — so
//     whichever one the SDK models, the other server's number is not dropped at
//     decode, leaving every bulk command reporting 0.
//   - compatTransport also normalises the single-execution action paths, which
//     Kestra 2.0 moved under /executions/{id}/actions/... The v2 SDK is split
//     on this: its generated endpoints still emit the 1.x paths (kill, replay,
//     restart, force-run, pause, resume, unqueue, change-status, labels,
//     replay-with-inputs, state) while its hand-written ones already emit the
//     2.0 paths (eval, and the same actions again). Neither shape works on both
//     servers, so the transport rewrites whichever the SDK produced into the one
//     the server actually routes. When the version is unknown it sends the 2.0
//     shape and falls back to the 1.x one on a route miss, rather than picking
//     one and failing every action if the guess was wrong.

// serverEra is what the transport knows about the server it is talking to.
//
// The distinction between eraCurrent and eraUnknown matters only for the action
// path rewrite: adding a response field is harmless when the version is a guess,
// but sending a request to the wrong path is not, so an unknown server gets the
// 2.0 shape with the 1.x shape as a fallback rather than a single guess.
type serverEra int

const (
	eraUnknown serverEra = iota // unreachable or unparseable version (develop builds)
	eraLegacy                   // Kestra 1.x
	eraCurrent                  // Kestra 2.0 or later
)

// noRewriteKey marks a context whose request must keep the path the SDK built.
//
// It exists for POST /executions/{namespace}/{flowId}, which creates an
// execution and is indistinguishable from a 1.x action call by path shape
// alone: a flow named `pause` in a namespace named `productionanalytics` reads
// exactly like `pause` on execution `productionanalytics`. Rather than tighten
// the id heuristic — which only moves the collision to a longer namespace — the
// one call site that builds that path opts out explicitly.
type noRewriteKey struct{}

// withoutExecutionActionRewrite returns a context whose requests keep the path
// the SDK built, exempt from the action rewrite.
func withoutExecutionActionRewrite(ctx context.Context) context.Context {
	return context.WithValue(ctx, noRewriteKey{}, true)
}

func rewriteExempt(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	exempt, _ := ctx.Value(noRewriteKey{}).(bool)
	return exempt
}

// newCompatHTTPClient returns the HTTP client shared by both SDK clients, so the
// response shim applies to generated and hand-written endpoints alike.
//
// era reports what is known about the server version; it is wired to
// Client.ServerVersion once the client exists, and the shims stay inert until
// then.
func newCompatHTTPClient() (*http.Client, *compatTransport) {
	transport := &compatTransport{base: http.DefaultTransport, era: func() serverEra { return eraUnknown }}
	return &http.Client{Transport: transport}, transport
}

type compatTransport struct {
	base http.RoundTripper
	era  func() serverEra
}

// compatMarkers are the keys a body must contain for fillCompatDefaults to have
// anything to do. Checking them before anything else keeps the shim off the hot
// path: most responses are returned without being decoded at all.
var compatMarkers = [][]byte{[]byte(`"tasks"`), []byte(`"flowRevision"`)}

func (t *compatTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req, fallback := t.routeExecutionAction(req)

	resp, err := t.base.RoundTrip(req)
	if err == nil && fallback != "" && isRouteMiss(resp) {
		// The version is unknown, so the shape was a guess. A 403/404 here means
		// the route did not match and nothing happened server-side, which makes
		// the other shape safe to try. If it misses too, the first response is
		// kept: its error describes the 2.x server this binary targets.
		if retry, ok := cloneRequestWithPath(req, fallback); ok {
			if second, secondErr := t.base.RoundTrip(retry); secondErr == nil {
				if isRouteMiss(second) {
					drainAndClose(second)
				} else {
					drainAndClose(resp)
					req, resp = retry, second
				}
			}
		}
	}
	if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 || !isJSONResponse(resp) {
		return resp, err
	}
	tenant, shimmed := shimmedEndpoint(req.URL.Path)
	bulk := isBulkEndpoint(req.URL.Path)
	if !shimmed && !bulk {
		return resp, nil
	}

	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, err
	}

	// The version probe runs through this same transport, so check the cheap
	// marker scan first and only then ask which server we are talking to.
	if shimmed && containsAny(body, compatMarkers) && t.era() == eraLegacy {
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

	if bulk {
		if patched, ok := mirrorBulkCount(body); ok {
			body = patched
			resp.Header.Del("Content-Length")
		}
	}

	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	return resp, nil
}

// isBulkEndpoint reports whether a path is one of the /by-ids or /by-query bulk
// endpoints, on any resource.
func isBulkEndpoint(path string) bool {
	return strings.HasSuffix(path, "/by-ids") || strings.HasSuffix(path, "/by-query")
}

// mirrorBulkCount makes a bulk response carry both spellings of its item
// count, and reports whether it changed anything.
//
// Kestra 2.0 answers a bulk request with the async-operation shape
// {"operationId":..., "totalItems":N}; 1.x answers {"count":N}. Whichever of
// the two the SDK models, the other server's spelling is dropped at decode and
// the command reports 0 executions affected even though the operation was
// accepted. Filling in the missing key covers both directions, so this keeps
// working across an SDK bump that retypes these endpoints (see
// kestra-io/client-sdk#409) without needing to know which shape the SDK is on.
//
// The decision is made on the decoded top-level object, not a substring scan of
// the body: a nested `count` elsewhere in the payload would otherwise read as
// "already present" and skip the mirroring this exists to do.
func mirrorBulkCount(body []byte) ([]byte, bool) {
	// UseNumber keeps the count verbatim rather than routing it through float64.
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var payload map[string]json.RawMessage
	if dec.Decode(&payload) != nil {
		return body, false
	}

	_, hasCount := payload["count"]
	_, hasTotal := payload["totalItems"]
	if hasCount == hasTotal {
		return body, false
	}

	if hasTotal {
		payload["count"] = payload["totalItems"]
	} else {
		payload["totalItems"] = payload["count"]
	}

	patched, err := json.Marshal(payload)
	if err != nil {
		return body, false
	}
	return patched, true
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

// executionActions are the single-execution endpoints Kestra 2.0 moved under
// /executions/{executionId}/actions/. Everything else below an execution
// (graph, flow, file, file/metas, follow, follow-dependencies) kept its path in
// 2.0 and is deliberately absent, as are the bulk `<action>/by-ids` and
// `<action>/by-query` variants, which never moved.
var executionActions = map[string]bool{
	"change-status":      true,
	"eval":               true,
	"force-run":          true,
	"kill":               true,
	"labels":             true,
	"pause":              true,
	"replay":             true,
	"replay-with-inputs": true,
	"restart":            true,
	"resume":             true,
	"state":              true,
	"unqueue":            true,
}

// routeExecutionAction returns the request to send and, when the server version
// is unknown, the alternate action path to fall back to on a route miss.
//
// A known server gets a single shape and no fallback: bare
// /executions/{id}/{action} on 1.x, /executions/{id}/actions/{action} on 2.0 and
// later. An unknown server gets the 2.0 shape — this binary targets 2.x — with
// the 1.x shape as the fallback, so a 1.x develop build keeps working instead of
// silently 404ing on every action.
//
// The version probe is only consulted once the path is known to be an action
// call, so requests that cannot be rewritten never pay for it (and the probe
// itself, on /api/v1/configs, cannot recurse into it).
func (t *compatTransport) routeExecutionAction(req *http.Request) (*http.Request, string) {
	if rewriteExempt(req.Context()) {
		return req, ""
	}
	prefix, action, ok := splitExecutionAction(req.URL.Path)
	if !ok {
		return req, ""
	}

	modern := prefix + "/actions/" + action
	legacy := prefix + "/" + action

	target, fallback := modern, ""
	switch t.era() {
	case eraLegacy:
		target = legacy
	case eraUnknown:
		fallback = legacy
	}

	if target != req.URL.Path {
		if rewritten, ok := cloneRequestWithPath(req, target); ok {
			req = rewritten
		} else {
			// The body cannot be replayed, so leave the path alone rather than
			// send a rewritten request whose body may already be consumed.
			return req, ""
		}
	}
	return req, fallback
}

// cloneRequestWithPath copies a request with a new path, and reports whether it
// could. RoundTrip must not mutate the request it is given; Clone deep-copies
// the URL, so writing Path here cannot be observed by the caller.
//
// A request carrying a body needs GetBody to be replayable — Clone shares the
// original reader, which the first attempt consumes.
func cloneRequestWithPath(req *http.Request, path string) (*http.Request, bool) {
	out := req.Clone(req.Context())
	if req.Body != nil && req.Body != http.NoBody {
		if req.GetBody == nil {
			return nil, false
		}
		body, err := req.GetBody()
		if err != nil {
			return nil, false
		}
		out.Body = body
	}
	// RawPath is cleared because every segment assembled here is unescaped.
	out.URL.Path = path
	out.URL.RawPath = ""
	return out, true
}

// isRouteMiss reports whether a response looks like the path did not match a
// route. Kestra answers an unmatched path under /executions/{id}/ with 404, and
// with 403 where a catch-all route matched but denied access.
func isRouteMiss(resp *http.Response) bool {
	return resp != nil && (resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden)
}

func drainAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

// splitExecutionAction parses /api/v1/{tenant}/executions/{executionId}/[actions/]{action}
// into the path up to and including the execution id, plus the action name.
//
// The execution id is required to look like one (Kestra ids are 20+ character
// alphanumeric FriendlyIds) because POST /executions/{namespace}/{flowId}
// creates an execution and would otherwise collide: a flow named `kill` in a
// namespace named `kill` reads as an action call on that shape alone.
func splitExecutionAction(path string) (prefix, action string, ok bool) {
	rest, found := strings.CutPrefix(path, "/api/v1/")
	if !found {
		return "", "", false
	}

	parts := strings.Split(rest, "/")
	if len(parts) < 4 || parts[1] != "executions" || !looksLikeExecutionID(parts[2]) {
		return "", "", false
	}

	switch {
	case len(parts) == 4:
		action = parts[3]
	case len(parts) == 5 && parts[3] == "actions":
		action = parts[4]
	default:
		return "", "", false
	}
	if !executionActions[action] {
		return "", "", false
	}

	return "/api/v1/" + strings.Join(parts[:3], "/"), action, true
}

// looksLikeExecutionID reports whether a path segment has the shape of a Kestra
// execution id: at least 16 alphanumeric characters and nothing else. A tenant
// or namespace segment that long without a dot, hyphen or underscore does not
// occur in practice.
func looksLikeExecutionID(segment string) bool {
	if len(segment) < 16 {
		return false
	}
	for _, r := range segment {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		default:
			return false
		}
	}
	return true
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

// serverEra reports what is known about the server's major version. An
// unreachable or unparseable version (develop builds, snapshots) is eraUnknown
// rather than being assumed current, so the action path rewrite can fall back
// instead of committing to a guess.
func (c *Client) serverEra() serverEra {
	version, err := c.ServerVersion()
	if err != nil {
		return eraUnknown
	}
	core, _ := splitVersion(version)
	if core == [3]int{} {
		return eraUnknown
	}
	if core[0] < 2 {
		return eraLegacy
	}
	return eraCurrent
}

// isLegacyServer reports whether the server is known to run Kestra 1.x. An
// unreachable or unparseable version counts as not-legacy: the response shims
// and version guards exist for 1.x servers, not for unknown ones.
func (c *Client) isLegacyServer() bool {
	return c.serverEra() == eraLegacy
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
