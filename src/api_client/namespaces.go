package apiclient

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// NamespacesAPI wraps namespace endpoints.
type NamespacesAPI struct {
	client *KestraClient
}

// NewNamespacesAPI creates a namespace helper.
func NewNamespacesAPI(client *KestraClient) *NamespacesAPI {
	return &NamespacesAPI{client: client}
}

// ListNamespaces returns namespaces optionally filtered by query.
func (n *NamespacesAPI) ListNamespaces(tenant string, ctx *AuthContext, query string, page, size int) ([]any, error) {
	resolvedCtx, err := n.client.ResolveContext(ctx)
	if err != nil {
		return nil, err
	}
	resolvedTenant := n.client.ResolveTenant(resolvedCtx, tenant)

	params := url.Values{}
	if page > 0 {
		params.Set("page", fmt.Sprintf("%d", page))
	}
	if size > 0 {
		params.Set("size", fmt.Sprintf("%d", size))
	}
	if query != "" {
		params.Set("q", query)
	}

	endpoint := fmt.Sprintf("/api/v1/%s/namespaces/search", resolvedTenant)
	body, err := n.client.Do(httpMethodGet, endpoint, RequestOptions{
		Context: resolvedCtx,
		Params:  params,
	})
	if err != nil {
		return nil, err
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	rawResults, ok := payload["results"]
	if !ok {
		return []any{}, nil
	}

	results, ok := rawResults.([]any)
	if !ok {
		return []any{}, nil
	}

	return results, nil
}
