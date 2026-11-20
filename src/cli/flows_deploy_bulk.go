package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
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
	finish := func() []DeployResult { return results[:processed] }

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

	// Stage 5 and 6: bulk write, falling back flow by flow per failed chunk.
	for start := 0; start < len(items); start += validateBatchSize {
		chunk := items[start:min(start+validateBatchSize, len(items))]
		revisions, err := postFlowsBulk(client, chunk)
		if err != nil {
			if failFast {
				for _, item := range chunk {
					results[item.index].Error = err.Error()
				}
				// Report everything up to the last file of the failed chunk.
				processed = chunk[len(chunk)-1].index + 1
				return finish()
			}
			// "Already exists" was decided in stage 4 and stands, so the
			// fallback upserts. Flows the failed chunk already saved are re-sent
			// with the same source, which returns their existing revision.
			for _, item := range chunk {
				results[item.index] = deployPreparedFlow(client, results[item.index], item.source, true)
			}
			continue
		}
		for _, item := range chunk {
			key := flowKey(results[item.index].Namespace, results[item.index].FlowID)
			revision, ok := revisions[key]
			if !ok {
				// Saved as far as the server says, but not echoed back: ask again.
				results[item.index] = deployPreparedFlow(client, results[item.index], item.source, true)
				continue
			}
			results[item.index].Revision = revision
			results[item.index].Success = true
		}
	}

	return finish()
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

// postFlowsBulk upserts one chunk through POST /flows/bulk and returns the
// revision of each flow keyed by namespace/id.
//
// delete is always sent explicitly because the server defaults it to true, and
// no namespace is sent so flows keep their own. The body must be
// application/yaml: Kestra 1.x rejects application/x-yaml here. The response is
// decoded by hand to avoid the generated client's label decode failure (#83).
func postFlowsBulk(client *Client, chunk []bulkDeployItem) (map[string]int32, error) {
	documents := make([]string, len(chunk))
	for i, item := range chunk {
		documents[i] = trimDocumentMarkers(item.source)
	}

	query := url.Values{}
	query.Set("delete", "false")
	path := client.tenantAPIPath(query, "flows", "bulk")

	body, status, err := client.doRawRequestPathBody(http.MethodPost, path, strings.NewReader(strings.Join(documents, "\n---\n")), "application/yaml")
	if err != nil {
		return nil, formatSDKError(err)
	}
	if status < 200 || status >= 300 {
		return nil, formatErrorBody(body, fmt.Sprintf("status %d", status))
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
