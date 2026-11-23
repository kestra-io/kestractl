package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"

	kestra "github.com/kestra-io/client-sdk/go-sdk/v2/kestra_api_client"
)

// bulkDeployItem is one file of a directory deploy that survived local
// preparation.
type bulkDeployItem struct {
	index  int // position in the file list, and in the results
	source string
}

func flowKey(namespace, flowID string) string { return namespace + "/" + flowID }

// deployFlowsBulk deploys the flows of a directory in a handful of requests
// instead of two per flow: batch validation, one existence search per
// namespace (without --override) and chunked POSTs to /flows/bulk.
//
// The bulk endpoint is not transactional, so nothing is sent until every flow
// has been validated, and a failed chunk is retried flow by flow to give each
// file its own error. With failFast, nothing is written when a local,
// validation or existence check fails, and the bulk phase stops after the
// first failed chunk without falling back.
//
// It returns one result per processed file, in file order. With failFast, files
// after the first failure are not reported, as in the per-flow path.
func deployFlowsBulk(client *Client, files []string, namespaceOverride, namespacePrefix string, disableTriggers, override, failFast bool) []DeployResult {
	results := make([]DeployResult, len(files))
	processed := len(files) // results[:processed] are reported

	// cutAtFirstFailure applies the fail-fast cut after a pre-write stage: report
	// up to and including the first failed file and write nothing.
	cutAtFirstFailure := func() bool {
		if !failFast {
			return false
		}
		for i := 0; i < processed; i++ {
			if results[i].Error != "" {
				processed = i + 1
				return true
			}
		}
		return false
	}
	// finish reports the processed files. A file that is neither deployed nor
	// failed was never attempted (fail-fast stopped the run before its write),
	// so say so rather than show a failure with no reason.
	finish := func() []DeployResult {
		for i := 0; i < processed; i++ {
			if !results[i].Success && results[i].Error == "" {
				results[i].Error = "not deployed: skipped (--fail-fast) because another flow failed"
			}
		}
		return results[:processed]
	}

	// Stage 1 and 2: local preparation and local guards.
	items := make([]bulkDeployItem, 0, len(files))
	seen := make(map[string]string, len(files))
	for i, file := range files {
		result, source := prepareFlowDeploy(file, namespaceOverride, namespacePrefix, disableTriggers, true)
		results[i] = result
		if result.Error != "" {
			continue
		}
		key := flowKey(result.Namespace, result.FlowID)
		if first, dup := seen[key]; dup {
			results[i].Error = fmt.Sprintf("flow '%s' in namespace '%s' is already defined by '%s' in this directory", result.FlowID, result.Namespace, first)
			continue
		}
		seen[key] = file
		items = append(items, bulkDeployItem{index: i, source: source})
	}
	if cutAtFirstFailure() {
		return finish()
	}

	// Stage 3: validate everything before writing anything.
	if len(items) > 0 {
		sources := make([]string, len(items))
		for i, item := range items {
			sources[i] = item.source
		}
		violations, err := validateSourcesInBatches(client, sources, validateBatchSize)
		if err != nil {
			// The validate endpoint itself is unusable (permissions, outage...):
			// the save path validates too, so deploy flow by flow as before.
			return deployItemsSequentially(client, results, items, override, failFast)
		}
		seeds := make([]ValidateResult, len(items))
		validated, _ := buildValidateResults(violations, seeds)
		kept := items[:0]
		for i, item := range items {
			if i < len(validated) && len(validated[i].Constraints) > 0 {
				results[item.index].Error = "invalid flow: " + strings.Join(validated[i].Constraints, "; ")
				continue
			}
			kept = append(kept, item)
		}
		items = kept
		if cutAtFirstFailure() {
			return finish()
		}
	}

	// Stage 4: existence check, one search per namespace. It is best-effort: a
	// flow created by someone else between this search and the write is updated
	// instead of rejected, as the GET-then-POST of the per-flow path already allows.
	if !override && len(items) > 0 {
		existing := make(map[string]map[string]bool)
		kept := items[:0]
		for _, item := range items {
			namespace, flowID := results[item.index].Namespace, results[item.index].FlowID
			ids, checked := existing[namespace]
			if !checked {
				var err error
				ids, err = listFlowIDsInNamespace(client, namespace)
				if err != nil {
					// Remember the failure so the namespace is searched once.
					results[item.index].Error = fmt.Sprintf("failed to check whether flow '%s' exists in namespace '%s': %v", flowID, namespace, err)
					existing[namespace] = nil
					continue
				}
				existing[namespace] = ids
			}
			switch {
			case ids == nil:
				results[item.index].Error = fmt.Sprintf("failed to check whether flow '%s' exists in namespace '%s': the namespace search failed", flowID, namespace)
			case ids[flowID]:
				results[item.index].Error = fmt.Sprintf("flow '%s' already exists in namespace '%s'; use --override to update", flowID, namespace)
			default:
				kept = append(kept, item)
			}
		}
		items = kept
		if cutAtFirstFailure() {
			return finish()
		}
	}

	// Stage 5 and 6: write, falling back per failed request.
	//
	// maxAttempted is the last file index a write was tried for; under
	// --fail-fast, files after it are reported as skipped.
	maxAttempted := -1

	// settle records the outcome of one write request and reports whether a
	// fail-fast run must stop. perNamespace tells whether the request held a
	// single namespace, which decides how a failure is attributed.
	settle := func(unit []bulkDeployItem, revisions map[string]int32, err error, perNamespace bool) bool {
		for _, item := range unit {
			maxAttempted = max(maxAttempted, item.index)
		}
		if err == nil {
			// Process the whole response before stopping: flows after a
			// missing one were saved too, and must be reported as such.
			missing := false
			for _, item := range unit {
				revision, ok := revisions[flowKey(results[item.index].Namespace, results[item.index].FlowID)]
				switch {
				case ok:
					results[item.index].Revision = revision
					results[item.index].Success = true
				case failFast:
					// No per-flow retry under --fail-fast: report the unknown outcome.
					results[item.index].Error = "outcome unknown: the server did not return this flow in its response, so it may or may not be deployed (--fail-fast)"
					missing = true
				default:
					// Saved as far as the server says, but not echoed back: ask again.
					results[item.index] = deployPreparedFlow(client, results[item.index], item.source, true)
				}
			}
			if missing {
				processed = maxAttempted + 1
			}
			return missing
		}
		if failFast {
			for _, item := range unit {
				results[item.index].Error = attributeWriteError(results[item.index].Namespace, err, perNamespace)
			}
			processed = maxAttempted + 1
			return true
		}
		// "Already exists" was decided in stage 4 and stands, so the fallback
		// upserts. Flows the failed request already saved are re-sent with the
		// same source, which returns their existing revision.
		for _, item := range unit {
			results[item.index] = deployPreparedFlow(client, results[item.index], item.source, true)
		}
		return false
	}

	// A source holding "---" anywhere must stay out of a /flows/bulk body:
	// Kestra 1.x up to 1.3.48 splits that body on a raw "---", which would cut
	// a "# --- notes ---" comment or an "a---b" value into fragments. The
	// multipart namespace route does not split part content (checked on 1.0.24,
	// 1.2.1 and 1.3.39), so those flows go through it.
	var bulkItems, separatorItems []bulkDeployItem
	for _, item := range items {
		if strings.Contains(item.source, "---") {
			separatorItems = append(separatorItems, item)
		} else {
			bulkItems = append(bulkItems, item)
		}
	}

	// When the server refuses /flows/bulk itself (see bulkEndpointUnusable) the
	// remaining writes go through POST /flows/{namespace} instead.
	useNamespaceEndpoint := false
	for _, chunk := range buildBulkChunks(results, bulkItems) {
		if !useNamespaceEndpoint {
			revisions, err := postFlowsBulk(client, chunk)
			if err == nil || !bulkEndpointUnusable(err) {
				if settle(chunk, revisions, err, false) {
					return finish()
				}
				continue
			}
			useNamespaceEndpoint = true
		}
		// Files of this chunk not yet written are reported as skipped on stop.
		for _, item := range chunk {
			maxAttempted = max(maxAttempted, item.index)
		}
		for _, unit := range groupByNamespace(results, chunk) {
			revisions, err := postFlowsNamespace(client, results[unit[0].index].Namespace, unit)
			if settle(unit, revisions, err, true) {
				return finish()
			}
		}
	}

	for _, group := range groupByNamespace(results, separatorItems) {
		for start := 0; start < len(group); start += validateBatchSize {
			unit := group[start:min(start+validateBatchSize, len(group))]
			revisions, err := postFlowsNamespace(client, results[unit[0].index].Namespace, unit)
			if settle(unit, revisions, err, true) {
				return finish()
			}
		}
	}

	return finish()
}

// buildBulkChunks packs items into requests of at most validateBatchSize flows
// in which no flow ID appears twice. Kestra 1.0 up to at least 2.0.11 rejects a
// bulk body that repeats an ID even across namespaces, so a/hello and b/hello
// must travel in different requests. First fit keeps the request count at the
// minimum unless IDs repeat.
func buildBulkChunks(results []DeployResult, items []bulkDeployItem) [][]bulkDeployItem {
	var chunks [][]bulkDeployItem
	var ids []map[string]bool
	for _, item := range items {
		id := results[item.index].FlowID
		placed := false
		for i := range chunks {
			if len(chunks[i]) < validateBatchSize && !ids[i][id] {
				chunks[i] = append(chunks[i], item)
				ids[i][id] = true
				placed = true
				break
			}
		}
		if !placed {
			chunks = append(chunks, []bulkDeployItem{item})
			ids = append(ids, map[string]bool{id: true})
		}
	}
	return chunks
}

// attributeWriteError builds the per-file error of a failed write request under
// --fail-fast. A namespace request fails for exactly its own namespace. A bulk
// request spans several namespaces and the server does not say which flow it
// refused, so its message is kept only for files whose namespace it names (as a
// whole quoted token, so 'qa' does not match 'qa.forbidden'). Other files are
// told the batch was rejected when the server answered 4xx; for anything else
// (transport error, unreadable 2xx body, 5xx) the outcome is unknown, because
// the endpoint is not transactional and part of the batch may be saved.
func attributeWriteError(namespace string, err error, perNamespace bool) string {
	if perNamespace || namesNamespace(err.Error(), namespace) {
		return err.Error()
	}
	var reqErr *bulkRequestError
	if errors.As(err, &reqErr) && reqErr.status >= 400 && reqErr.status < 500 {
		return "not deployed: the bulk request holding this flow was rejected by the server (--fail-fast)"
	}
	return "outcome unknown: the bulk request holding this flow failed or its response could not be read, so it may be partly deployed (--fail-fast)"
}

// namesNamespace reports whether message quotes namespace as a whole token, the
// way the server writes it ('ns' or "ns").
func namesNamespace(message, namespace string) bool {
	return strings.Contains(message, "'"+namespace+"'") || strings.Contains(message, `"`+namespace+`"`)
}

// groupByNamespace splits a chunk into one unit per final namespace, in order of
// first appearance.
func groupByNamespace(results []DeployResult, chunk []bulkDeployItem) [][]bulkDeployItem {
	var order []string
	byNamespace := make(map[string][]bulkDeployItem)
	for _, item := range chunk {
		ns := results[item.index].Namespace
		if _, ok := byNamespace[ns]; !ok {
			order = append(order, ns)
		}
		byNamespace[ns] = append(byNamespace[ns], item)
	}
	units := make([][]bulkDeployItem, 0, len(order))
	for _, ns := range order {
		units = append(units, byNamespace[ns])
	}
	return units
}

// deployItemsSequentially deploys prepared items with the per-flow path. Under
// failFast no earlier file can have failed (the caller stops at the first
// pre-write failure), so cutting after the first failed item is enough.
func deployItemsSequentially(client *Client, results []DeployResult, items []bulkDeployItem, override, failFast bool) []DeployResult {
	for _, item := range items {
		results[item.index] = deployPreparedFlow(client, results[item.index], item.source, override)
		if failFast && !results[item.index].Success {
			return results[:item.index+1]
		}
	}
	return results
}

// bulkRequestError is a non-2xx answer to a flow write request.
type bulkRequestError struct {
	status int
	err    error
}

func (e *bulkRequestError) Error() string { return e.err.Error() }
func (e *bulkRequestError) Unwrap() error { return e.err }

// bulkEndpointUnusable reports whether a refused /flows/bulk request means the
// endpoint itself cannot be used, as opposed to a problem with these flows.
//
//   - 404 / 405 / 415: the route or the content type is not served.
//   - 403: Kestra EE 1.3 answers a bare 403 to every /flows/bulk call, even for
//     an admin. A genuine permission denial also lands here, but the namespace
//     endpoint needs at least the same permissions, so it fails the same way
//     and the per-flow fallback then reports each file's own error.
//
// 400/413/5xx and transport errors point at the content or the server, not the
// route, and go straight to the per-flow fallback.
func bulkEndpointUnusable(err error) bool {
	var reqErr *bulkRequestError
	if !errors.As(err, &reqErr) {
		return false
	}
	switch reqErr.status {
	case http.StatusForbidden, http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusUnsupportedMediaType:
		return true
	}
	return false
}

// postFlowsBulk upserts one chunk through POST /flows/bulk and returns the
// revision of each flow keyed by namespace/id.
//
// delete is always sent explicitly because the server defaults it to true, and
// no namespace is sent so flows keep their own. The body must be
// application/yaml: Kestra 1.x rejects application/x-yaml here.
func postFlowsBulk(client *Client, chunk []bulkDeployItem) (map[string]int32, error) {
	query := url.Values{}
	query.Set("delete", "false")
	return postFlowsBody(client, client.tenantAPIPath(query, "flows", "bulk"), strings.NewReader(joinFlowDocuments(chunk)), "application/yaml")
}

// postFlowsNamespace upserts flows that all belong to namespace through the
// multipart variant of POST /flows/{namespace}, the request `flows
// namespace-sync` sends. It is the fallback for servers that refuse /flows/bulk.
//
// Read from the Kestra 1.3 sources: the endpoint upserts every flow (findById,
// then update or create), so it matches the existence decision already made
// here; its `override` parameter rewrites each flow's namespace, so it is sent
// as false; delete must be explicit because the server default is true. Flows
// must sit exactly in the path namespace, hence one request per namespace. The
// server trims each uploaded file, like the bulk endpoint. The YAML variant of
// the same route is not used: Kestra EE 1.3 checks CREATE, UPDATE and DELETE on
// the namespace there, and refuses the request with a bare 403, as it does for
// /flows/bulk. The route is deprecated on 2.x, which is why it is only used
// once /flows/bulk has been refused.
func postFlowsNamespace(client *Client, namespace string, chunk []bulkDeployItem) (map[string]int32, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for i, item := range chunk {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", multipart.FileContentDisposition("flows", fmt.Sprintf("flow-%d.yaml", i)))
		header.Set("Content-Type", "application/x-yaml")
		part, err := mw.CreatePart(header)
		if err != nil {
			return nil, err
		}
		if _, err := part.Write([]byte(item.source)); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}

	query := url.Values{}
	query.Set("delete", "false")
	query.Set("override", "false")
	return postFlowsBody(client, client.tenantAPIPath(query, "flows", namespace), &buf, mw.FormDataContentType())
}

// joinFlowDocuments builds the multi-document body of /flows/bulk. The server
// splits it exactly like /validate and trims each document, which is why sources
// are normalised by normalizeFlowSource before they get here.
func joinFlowDocuments(chunk []bulkDeployItem) string {
	documents := make([]string, len(chunk))
	for i, item := range chunk {
		documents[i] = trimDocumentMarkers(item.source)
	}
	return strings.Join(documents, "\n---\n")
}

// postFlowsBody sends a flow write request and decodes the revisions. The
// response is decoded by hand to avoid the generated client's label decode
// failure (#83).
func postFlowsBody(client *Client, path string, reqBody io.Reader, contentType string) (map[string]int32, error) {
	body, status, err := client.doRawRequestPathBody(http.MethodPost, path, reqBody, contentType)
	if err != nil {
		return nil, formatSDKError(err)
	}
	if status < 200 || status >= 300 {
		return nil, &bulkRequestError{status: status, err: formatErrorBody(body, fmt.Sprintf("status %d", status))}
	}

	var flows []struct {
		ID        string `json:"id"`
		Namespace string `json:"namespace"`
		Revision  int32  `json:"revision"`
	}
	if err := json.Unmarshal(body, &flows); err != nil {
		return nil, fmt.Errorf("unexpected bulk response: %w", err)
	}
	revisions := make(map[string]int32, len(flows))
	for _, f := range flows {
		revisions[flowKey(f.Namespace, f.ID)] = f.Revision
	}
	return revisions, nil
}

// normalizeFlowSource returns the source as the bulk and namespace endpoints
// store it: document markers and surrounding whitespace removed (the server
// trims each document, like Java's String.trim, i.e. every char <= U+0020).
// Sending the same text on the per-flow path keeps a repeated deploy of an
// unchanged file on the same revision, whichever path carries it.
func normalizeFlowSource(source string) string {
	return strings.TrimFunc(trimDocumentMarkers(source), func(r rune) bool { return r <= ' ' })
}

// listFlowIDsInNamespace pages through the flows of one namespace and returns
// their IDs as a set. The filter is an exact match, so child namespaces are not
// included.
func listFlowIDsInNamespace(client *Client, namespace string) (map[string]bool, error) {
	const pageSize int32 = 1000
	filters := []kestra.QueryFilter{equalsFilter(kestra.QUERYFILTERFIELD_NAMESPACE, namespace)}
	ids := make(map[string]bool)
	page := int32(1)
	seen := 0

	for {
		resp, _, err := client.API.FlowsAPI.SearchFlows(client.Ctx, client.Tenant).
			Page(page).
			Size(pageSize).
			Filters(filters).
			Execute()

		var batch []parsedFlow
		var total int64
		if err != nil {
			// Array-format labels (#83): recover the flows from the raw body.
			parsed, parsedTotal, ok := tryParseFlowSearchFromError(err)
			if !ok {
				return nil, formatSDKError(err)
			}
			batch, total = parsed, parsedTotal
		} else {
			for _, f := range resp.GetResults() {
				batch = append(batch, parsedFlowFromFlow(f))
			}
			total = resp.GetTotal()
		}

		for _, f := range batch {
			ids[f.ID] = true
		}
		seen += len(batch)
		if len(batch) == 0 || int64(seen) >= total {
			break
		}
		page++
	}

	return ids, nil
}
