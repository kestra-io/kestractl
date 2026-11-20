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

	// Stage 4: existence check, one search per namespace.
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

	// Stage 5 and 6: bulk write, falling back per failed chunk.
	//
	// When the server refuses /flows/bulk itself (see bulkEndpointUnusable) the
	// remaining writes go through POST /flows/{namespace} instead.
	useNamespaceEndpoint := false
	for start := 0; start < len(items); start += validateBatchSize {
		chunk := items[start:min(start+validateBatchSize, len(items))]

		var units [][]bulkDeployItem
		var post func(unit []bulkDeployItem) (map[string]int32, error)
		var err error
		var revisions map[string]int32

		if !useNamespaceEndpoint {
			revisions, err = postFlowsBulk(client, chunk)
			if err != nil && bulkEndpointUnusable(err) {
				useNamespaceEndpoint = true
			}
		}

		if useNamespaceEndpoint {
			units = groupByNamespace(results, chunk)
			post = func(unit []bulkDeployItem) (map[string]int32, error) {
				return postFlowsNamespace(client, results[unit[0].index].Namespace, unit)
			}
		} else {
			units = [][]bulkDeployItem{chunk}
			post = func([]bulkDeployItem) (map[string]int32, error) { return revisions, err }
		}

		for _, unit := range units {
			unitRevisions, unitErr := post(unit)
			if unitErr == nil {
				for _, item := range unit {
					revision, ok := unitRevisions[flowKey(results[item.index].Namespace, results[item.index].FlowID)]
					if !ok {
						// Saved as far as the server says, but not echoed back: ask again.
						results[item.index] = deployPreparedFlow(client, results[item.index], item.source, true)
						continue
					}
					results[item.index].Revision = revision
					results[item.index].Success = true
				}
				continue
			}
			if failFast {
				for _, item := range unit {
					results[item.index].Error = attributeWriteError(results[item.index].Namespace, unitErr, useNamespaceEndpoint)
				}
				// Everything up to the end of this chunk has been attempted or
				// is reported as skipped.
				processed = chunk[len(chunk)-1].index + 1
				return finish()
			}
			// "Already exists" was decided in stage 4 and stands, so the
			// fallback upserts. Flows the failed request already saved are re-sent
			// with the same source, which returns their existing revision.
			for _, item := range unit {
				results[item.index] = deployPreparedFlow(client, results[item.index], item.source, true)
			}
		}
	}

	return finish()
}

// attributeWriteError builds the per-file error of a failed write request under
// --fail-fast. A namespace request fails for exactly its own namespace. A bulk
// request spans several namespaces and the server does not say which flow it
// refused, so its message is kept only for files whose namespace it names; the
// others are told the batch was rejected and they were not deployed.
func attributeWriteError(namespace string, err error, perNamespace bool) string {
	if perNamespace || strings.Contains(err.Error(), namespace) {
		return err.Error()
	}
	return "not deployed: the bulk request holding this flow was rejected by the server (--fail-fast)"
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
