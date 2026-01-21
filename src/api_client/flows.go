package apiclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseFlowYAML extracts namespace and flow ID from YAML content.
func ParseFlowYAML(content string) (string, string, error) {
	var payload map[string]any
	if err := yaml.Unmarshal([]byte(content), &payload); err != nil {
		return "", "", fmt.Errorf("invalid YAML content: %w", err)
	}

	rawNamespace, ok := payload["namespace"]
	if !ok {
		return "", "", errors.New("flow YAML must contain a 'namespace' field")
	}
	namespace, ok := rawNamespace.(string)
	if !ok || strings.TrimSpace(namespace) == "" {
		return "", "", errors.New("flow YAML must contain a valid 'namespace' field")
	}

	rawID, ok := payload["id"]
	if !ok {
		return "", "", errors.New("flow YAML must contain an 'id' field")
	}
	flowID, ok := rawID.(string)
	if !ok || strings.TrimSpace(flowID) == "" {
		return "", "", errors.New("flow YAML must contain a valid 'id' field")
	}

	return namespace, flowID, nil
}

// FlowsAPI exposes Kestra flow operations.
type FlowsAPI struct {
	client *KestraClient
}

// NewFlowsAPI returns a flows API helper.
func NewFlowsAPI(client *KestraClient) *FlowsAPI {
	return &FlowsAPI{client: client}
}

// ListFlows returns the flows available in a namespace.
func (f *FlowsAPI) ListFlows(namespace, tenant string, ctx *AuthContext) ([]map[string]any, error) {
	resolvedCtx, err := f.client.ResolveContext(ctx)
	if err != nil {
		return nil, err
	}
	resolvedTenant := f.client.ResolveTenant(resolvedCtx, tenant)

	endpoint := fmt.Sprintf("/api/v1/%s/flows/%s", resolvedTenant, namespace)
	body, err := f.client.Do(httpMethodGet, endpoint, RequestOptions{Context: resolvedCtx})
	if err != nil {
		return nil, err
	}

	var flows []map[string]any
	if err := json.Unmarshal(body, &flows); err != nil {
		return nil, err
	}

	return flows, nil
}

// GetFlow retrieves a flow definition.
func (f *FlowsAPI) GetFlow(namespace, flowID, tenant string, ctx *AuthContext) (map[string]any, error) {
	resolvedCtx, err := f.client.ResolveContext(ctx)
	if err != nil {
		return nil, err
	}
	resolvedTenant := f.client.ResolveTenant(resolvedCtx, tenant)

	endpoint := fmt.Sprintf("/api/v1/%s/flows/%s/%s", resolvedTenant, namespace, flowID)
	body, err := f.client.Do(httpMethodGet, endpoint, RequestOptions{Context: resolvedCtx})
	if err != nil {
		return nil, err
	}

	var flow map[string]any
	if err := json.Unmarshal(body, &flow); err != nil {
		return nil, err
	}

	return flow, nil
}

// FlowExists returns true when the flow already exists.
func (f *FlowsAPI) FlowExists(namespace, flowID, tenant string, ctx *AuthContext) (bool, error) {
	_, err := f.GetFlow(namespace, flowID, tenant, ctx)
	if err != nil {
		if strings.Contains(err.Error(), "status 404") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// CreateFlow creates or updates a flow based on YAML content.
func (f *FlowsAPI) CreateFlow(yamlContent string, tenant string, ctx *AuthContext, override bool) (map[string]any, error) {
	resolvedCtx, err := f.client.ResolveContext(ctx)
	if err != nil {
		return nil, err
	}
	resolvedTenant := f.client.ResolveTenant(resolvedCtx, tenant)

	namespace, flowID, err := ParseFlowYAML(yamlContent)
	if err != nil {
		return nil, err
	}

	exists, err := f.FlowExists(namespace, flowID, resolvedTenant, resolvedCtx)
	if err != nil && !strings.Contains(err.Error(), "status 404") {
		return nil, err
	}

	if exists && !override {
		return nil, fmt.Errorf("flow '%s' already exists in namespace '%s'; use --override to update", flowID, namespace)
	}

	contentReader := strings.NewReader(yamlContent)

	var endpoint string
	method := httpMethodPost
	if exists && override {
		endpoint = fmt.Sprintf("/api/v1/%s/flows/%s/%s", resolvedTenant, namespace, flowID)
		method = httpMethodPut
	} else {
		endpoint = fmt.Sprintf("/api/v1/%s/flows", resolvedTenant)
	}

	body, err := f.client.Do(method, endpoint, RequestOptions{
		Context:     resolvedCtx,
		Body:        contentReader,
		ContentType: "application/x-yaml",
	})
	if err != nil {
		return nil, err
	}

	var flow map[string]any
	if err := json.Unmarshal(body, &flow); err != nil {
		return nil, err
	}
	return flow, nil
}
