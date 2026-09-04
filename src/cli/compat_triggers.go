package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	kestra "github.com/kestra-io/client-sdk/go-sdk/v2/kestra_api_client"
)

// Trigger responses across Kestra versions (issue #118).
//
// The two servers answer the trigger endpoints with different shapes, and the
// SDK models both — in different clients:
//
//	                    Kestra 1.x                        Kestra 2.0
//	/triggers/search    {abstractTrigger, triggerContext} {trigger, state}
//	                    generated TriggersAPI             hand-written Triggers()
//	single trigger      Trigger (date, nextExecutionDate) ApiTriggerState
//	                                                      (updatedAt,
//	                                                       nextEvaluationDate)
//
// The generated surface — which kestractl used for every trigger command — is
// still on the 1.x models, so a 2.0 body decodes into empty fields (`triggers
// list` rendered blank rows) or is rejected outright over a required field the
// 2.0 server never sends ("no value given for required property date", on
// search-for-flow, unlock and the single backfill ops).
//
// The fix is to read each shape with the client that models it, and to funnel
// both into the era-agnostic values below so the commands themselves stay free
// of version branching. An unknown era (a develop build, or an unreadable
// /configs) takes the 2.0 path: that is the server this binary targets.
//
// Reshaping the 1.x body in compatTransport instead would put the mapping in
// one place, but it cannot express this: `updatedAt` is required by the 2.0
// decoder and a 1.x server omits it entirely from some responses (POST
// .../restart returns a state with no date at all), so the transport would have
// to invent a timestamp. Dispatching on the era keeps every field a server
// actually sent.

// triggerListRow is one `triggers list` row, read from whichever shape the
// server sent.
type triggerListRow struct {
	Namespace         string
	FlowID            string
	TriggerID         string
	Type              string
	Disabled          bool
	NextExecutionDate *time.Time
}

// triggerRef identifies a single trigger and its disabled flag: what the
// per-flow search, unlock, and the single backfill ops render.
type triggerRef struct {
	Namespace string
	FlowID    string
	TriggerID string
	Disabled  bool
}

// fetchTriggerRows lists triggers, reading the 2.0 {trigger, state} shape or the
// 1.x {abstractTrigger, triggerContext} one, and returns the rows plus the total.
func fetchTriggerRows(client *Client, page, size int32) ([]triggerListRow, int64, error) {
	if client.isLegacyServer() {
		body, err := legacyTriggerRequest(client, http.MethodGet,
			legacyTriggerPath(client, "search"),
			url.Values{"page": {strconv.Itoa(int(page))}, "size": {strconv.Itoa(int(size))}}, nil)
		if err != nil {
			return nil, 0, err
		}

		results, total := jsonResults(body)
		rows := make([]triggerListRow, 0, len(results))
		for _, item := range results {
			ctx := jsonObject(item, "triggerContext")
			rows = append(rows, triggerListRow{
				Namespace:         jsonString(ctx, "namespace"),
				FlowID:            jsonString(ctx, "flowId"),
				TriggerID:         jsonString(ctx, "triggerId"),
				Type:              jsonString(jsonObject(item, "abstractTrigger"), "type"),
				Disabled:          jsonBool(ctx, "disabled"),
				NextExecutionDate: jsonTime(ctx, "nextExecutionDate"),
			})
		}
		return rows, total, nil
	}

	p, s := int(page), int(size)
	resp, err := client.Kestra.Triggers().SearchTriggers(client.Ctx, client.Tenant, &p, &s, nil, nil, nil)
	if err != nil {
		return nil, 0, formatSDKError(err)
	}
	if resp == nil {
		return nil, 0, nil
	}

	rows := make([]triggerListRow, 0, len(resp.GetResults()))
	for _, t := range resp.GetResults() {
		state := t.GetState()
		trigger := t.GetTrigger()
		row := triggerListRow{
			Namespace: state.GetNamespace(),
			FlowID:    state.GetFlowId(),
			TriggerID: state.GetTriggerId(),
			Type:      trigger.GetType(),
			Disabled:  state.GetDisabled(),
		}
		// 2.0 renamed nextExecutionDate to nextEvaluationDate.
		if next, ok := state.GetNextEvaluationDateOk(); ok && next != nil && !next.IsZero() {
			row.NextExecutionDate = next
		}
		rows = append(rows, row)
	}
	return rows, resp.GetTotal(), nil
}

// fetchFlowTriggerRefs lists the triggers of one flow, on either era.
func fetchFlowTriggerRefs(client *Client, namespace, flowID string, page, size int32, query string) ([]triggerRef, int64, error) {
	if client.isLegacyServer() {
		params := url.Values{"page": {strconv.Itoa(int(page))}, "size": {strconv.Itoa(int(size))}}
		if query != "" {
			params.Set("q", query)
		}
		body, err := legacyTriggerRequest(client, http.MethodGet,
			legacyTriggerPath(client, namespace, flowID), params, nil)
		if err != nil {
			return nil, 0, err
		}

		results, total := jsonResults(body)
		refs := make([]triggerRef, 0, len(results))
		for _, item := range results {
			refs = append(refs, jsonTriggerRef(item))
		}
		return refs, total, nil
	}

	p, s := int(page), int(size)
	var q *string
	if query != "" {
		q = &query
	}
	resp, err := client.Kestra.Triggers().SearchTriggersForFlow(client.Ctx, client.Tenant, namespace, flowID, &p, &s, q, nil)
	if err != nil {
		return nil, 0, formatSDKError(err)
	}
	if resp == nil {
		return nil, 0, nil
	}

	refs := make([]triggerRef, 0, len(resp.GetResults()))
	for _, state := range resp.GetResults() {
		refs = append(refs, triggerRef{
			Namespace: state.GetNamespace(),
			FlowID:    state.GetFlowId(),
			TriggerID: state.GetTriggerId(),
			Disabled:  state.GetDisabled(),
		})
	}
	return refs, resp.GetTotal(), nil
}

// unlockTriggerRef unlocks a trigger and returns what the server reported back,
// or nil when the response carried no body.
func unlockTriggerRef(client *Client, namespace, flowID, triggerID string) (*triggerRef, error) {
	if client.isLegacyServer() {
		body, err := legacyTriggerRequest(client, http.MethodPost,
			legacyTriggerPath(client, namespace, flowID, triggerID, "unlock"), nil, nil)
		if err != nil {
			return nil, err
		}
		if body == nil {
			return nil, nil
		}
		ref := jsonTriggerRef(body)
		return &ref, nil
	}

	state, err := client.Kestra.Triggers().UnlockTrigger(client.Ctx, client.Tenant, namespace, flowID, triggerID)
	if err != nil {
		return nil, formatSDKError(err)
	}
	return triggerStateRef(state), nil
}

// backfillTriggerRef runs one of the single-trigger backfill operations —
// "pause", "unpause" or "delete" — and returns what the server reported back.
func backfillTriggerRef(client *Client, namespace, flowID, triggerID, op string) (*triggerRef, error) {
	if client.isLegacyServer() {
		// A 1.x server takes the whole Trigger as the request body, `date`
		// included — that is what the generated client used to send.
		payload := map[string]any{
			"namespace": namespace,
			"flowId":    flowID,
			"triggerId": triggerID,
			"date":      time.Now().UTC().Format(time.RFC3339),
		}

		method, action := http.MethodPut, op
		if op != "pause" && op != "unpause" {
			method, action = http.MethodPost, "delete"
		}
		body, err := legacyTriggerRequest(client, method,
			legacyTriggerPath(client, "backfill", action), nil, payload)
		if err != nil {
			return nil, err
		}
		if body == nil {
			return nil, nil
		}
		ref := jsonTriggerRef(body)
		return &ref, nil
	}

	// 2.0 takes just the identifying triple.
	id := kestra.NewTriggerControllerApiTriggerId()
	id.SetNamespace(namespace)
	id.SetFlowId(flowID)
	id.SetTriggerId(triggerID)

	var state *kestra.ApiTriggerState
	var err error
	switch op {
	case "pause":
		state, err = client.Kestra.Triggers().PauseBackfill(client.Ctx, client.Tenant, *id)
	case "unpause":
		state, err = client.Kestra.Triggers().UnpauseBackfill(client.Ctx, client.Tenant, *id)
	default: // delete
		state, err = client.Kestra.Triggers().DeleteBackfill(client.Ctx, client.Tenant, *id)
	}
	if err != nil {
		return nil, formatSDKError(err)
	}
	return triggerStateRef(state), nil
}

// --- the Kestra 1.x path ---
//
// The generated models are not used on this side either, even though they are
// the ones written for 1.x: they mark `date` required, and a 1.3 server omits it
// from a trigger whose state was reset (POST .../restart, and any trigger that
// has not been evaluated yet), which makes the decoder reject the whole body
// with "no value given for required property date". These endpoints are read
// from the raw response instead, taking only the fields the commands render, so
// a missing optional field renders empty rather than failing the command.
//
// The request is built on the SDK's own configuration — host, default headers,
// context auth, and the shared HTTP client — so it still goes through
// compatTransport and the --verbose dump.

// legacyTriggerPath assembles /api/v1/{tenant}/triggers/{segments...}.
func legacyTriggerPath(client *Client, segments ...string) string {
	path := "/api/v1/" + url.PathEscape(client.Tenant) + "/triggers"
	for _, segment := range segments {
		path += "/" + url.PathEscape(segment)
	}
	return path
}

// legacyTriggerRequest issues one request against a Kestra 1.x trigger endpoint
// and returns its decoded JSON object, or nil when the response has no body.
func legacyTriggerRequest(client *Client, method, path string, query url.Values, payload any) (map[string]any, error) {
	cfg := client.API.GetConfig()

	base := ""
	if len(cfg.Servers) > 0 {
		base = cfg.Servers[0].URL
	}
	if base == "" {
		base = cfg.Scheme + "://" + cfg.Host
	}
	endpoint := strings.TrimRight(base, "/") + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	var reader io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(client.Ctx, method, endpoint, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for header, value := range cfg.DefaultHeader {
		req.Header.Set(header, value)
	}
	// Reuse the auth the SDK stored on the context.
	if auth, ok := client.Ctx.Value(kestra.ContextBasicAuth).(kestra.BasicAuth); ok {
		req.SetBasicAuth(auth.UserName, auth.Password)
	}
	if token, ok := client.Ctx.Value(kestra.ContextAccessToken).(string); ok {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// formatErrorBody renders the same message the SDK errors get.
		return nil, formatErrorBody(raw, fmt.Sprintf("status %d", resp.StatusCode))
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, formatErrorBody(raw, "the trigger endpoint returned an unexpected body")
	}
	return decoded, nil
}

// jsonResults reads the {"results":[...],"total":N} envelope both servers use
// for a paged response.
func jsonResults(body map[string]any) ([]map[string]any, int64) {
	raw, _ := body["results"].([]any)
	results := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if object, ok := item.(map[string]any); ok {
			results = append(results, object)
		}
	}

	var total int64
	switch value := body["total"].(type) {
	case float64:
		total = int64(value)
	case json.Number:
		total, _ = value.Int64()
	}
	return results, total
}

func jsonObject(body map[string]any, key string) map[string]any {
	object, _ := body[key].(map[string]any)
	return object
}

func jsonString(body map[string]any, key string) string {
	value, _ := body[key].(string)
	return value
}

func jsonBool(body map[string]any, key string) bool {
	value, _ := body[key].(bool)
	return value
}

// jsonTime reads an RFC 3339 timestamp, and returns nil when the field is
// absent, empty, unparseable or the zero instant.
func jsonTime(body map[string]any, key string) *time.Time {
	value, _ := body[key].(string)
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil || parsed.IsZero() {
		return nil
	}
	return &parsed
}

// jsonTriggerRef reads the identifying triple out of either server's
// single-trigger body: 1.x Trigger and 2.0 ApiTriggerState agree on these
// field names.
func jsonTriggerRef(body map[string]any) triggerRef {
	return triggerRef{
		Namespace: jsonString(body, "namespace"),
		FlowID:    jsonString(body, "flowId"),
		TriggerID: jsonString(body, "triggerId"),
		Disabled:  jsonBool(body, "disabled"),
	}
}

func triggerStateRef(state *kestra.ApiTriggerState) *triggerRef {
	if state == nil {
		return nil
	}
	return &triggerRef{
		Namespace: state.GetNamespace(),
		FlowID:    state.GetFlowId(),
		TriggerID: state.GetTriggerId(),
		Disabled:  state.GetDisabled(),
	}
}
