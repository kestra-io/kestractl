package apiclient

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// ExecutionsAPI wraps execution related endpoints.
type ExecutionsAPI struct {
	client *KestraClient
}

// NewExecutionsAPI constructs an execution API helper.
func NewExecutionsAPI(client *KestraClient) *ExecutionsAPI {
	return &ExecutionsAPI{client: client}
}

// KillByQuery terminates executions matching the provided filters.
func (e *ExecutionsAPI) KillByQuery(state []string, namespace, flowID, tenant string, ctx *AuthContext) (map[string]any, error) {
	resolvedCtx, err := e.client.ResolveContext(ctx)
	if err != nil {
		return nil, err
	}
	resolvedTenant := e.client.ResolveTenant(resolvedCtx, tenant)

	params := url.Values{}
	for _, s := range state {
		params.Add("state", s)
	}
	if namespace != "" {
		params.Set("namespace", namespace)
	}
	if flowID != "" {
		params.Set("flowId", flowID)
	}

	endpoint := fmt.Sprintf("/api/v1/%s/executions/kill/by-query", resolvedTenant)
	body, err := e.client.Do(httpMethodDelete, endpoint, RequestOptions{
		Context: resolvedCtx,
		Params:  params,
	})
	if err != nil {
		return nil, err
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// TriggerExecution runs a workflow execution.
func (e *ExecutionsAPI) TriggerExecution(namespace, flowID string, wait bool, inputs map[string]any, tenant string, ctx *AuthContext) (map[string]any, error) {
	resolvedCtx, err := e.client.ResolveContext(ctx)
	if err != nil {
		return nil, err
	}
	resolvedTenant := e.client.ResolveTenant(resolvedCtx, tenant)

	params := url.Values{}
	if wait {
		params.Set("wait", "true")
	}

	endpoint := fmt.Sprintf("/api/v1/%s/executions/%s/%s", resolvedTenant, namespace, flowID)

	var reqOpts RequestOptions
	reqOpts.Context = resolvedCtx
	reqOpts.Params = params
	if len(inputs) > 0 {
		reqOpts.JSON = inputs
	}

	body, err := e.client.Do(httpMethodPost, endpoint, reqOpts)
	if err != nil {
		return nil, err
	}

	var execution map[string]any
	if err := json.Unmarshal(body, &execution); err != nil {
		return nil, err
	}

	return execution, nil
}

// GetExecution fetches execution details.
func (e *ExecutionsAPI) GetExecution(executionID, tenant string, ctx *AuthContext) (map[string]any, error) {
	resolvedCtx, err := e.client.ResolveContext(ctx)
	if err != nil {
		return nil, err
	}
	resolvedTenant := e.client.ResolveTenant(resolvedCtx, tenant)

	endpoint := fmt.Sprintf("/api/v1/%s/executions/%s", resolvedTenant, executionID)

	body, err := e.client.Do(httpMethodGet, endpoint, RequestOptions{Context: resolvedCtx})
	if err != nil {
		return nil, err
	}

	var execution map[string]any
	if err := json.Unmarshal(body, &execution); err != nil {
		return nil, err
	}

	return execution, nil
}
