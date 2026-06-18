package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	kestra "github.com/kestra-io/client-sdk/go-sdk/kestra_api_client"
	"github.com/spf13/viper"
)

// Client wraps the Kestra SDK with authentication and tenant info.
//
// API is the generated-style client (request builders + Execute). Kestra is the
// SDK's hand-written client, which exposes endpoints the generated surface does
// not (e.g. invitations, bindings). Both are built from the same resolved
// config, so they share host, auth, and headers.
type Client struct {
	API    *kestra.APIClient
	Kestra *kestra.KestraClient
	Ctx    context.Context
	Tenant string
}

// newClientFunc is the client factory function. Override in tests.
var newClientFunc = newClientDefault

// NewClient creates a client by resolving config from flags > env > config file.
func NewClient() (*Client, error) {
	return newClientFunc()
}

// newClientDefault is the default client creation logic.
func newClientDefault() (*Client, error) {
	host, tenant, token, username, password, err := resolveConfig()
	isVerbose := viper.GetBool("verbose")

	if err != nil {
		return nil, err
	}

	// Parse host URL
	hostURL, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("invalid host URL: %w", err)
	}

	scheme := hostURL.Scheme
	if scheme == "" {
		scheme = "http"
	}

	hostPart := hostURL.Host
	if hostPart == "" {
		hostPart = hostURL.Path
	}

	// Create SDK configuration
	cfg := kestra.NewConfiguration()
	cfg.Scheme = scheme
	cfg.Host = hostPart
	cfg.Servers = kestra.ServerConfigurations{
		{URL: host},
	}
	cfg.Debug = isVerbose

	parsed, err := parseHeaders(globalFlags.Headers)
	if err != nil {
		return nil, err
	}
	for k, v := range parsed {
		cfg.AddDefaultHeader(k, v)
	}

	client := kestra.NewAPIClient(cfg)

	// The hand-written client shares the same resolved host, headers, and debug
	// setting; auth is appended per-branch below.
	opts := []kestra.ClientOption{kestra.WithDebug(isVerbose)}
	if len(parsed) > 0 {
		opts = append(opts, kestra.WithHeaders(parsed))
	}

	ctx := context.Background()
	if token != "" {
		ctx = context.WithValue(ctx, kestra.ContextAccessToken, token)
		opts = append(opts, kestra.WithTokenAuth(token))
	} else if username != "" && password != "" {
		ctx = context.WithValue(ctx, kestra.ContextBasicAuth, kestra.BasicAuth{
			UserName: username,
			Password: password,
		})
		opts = append(opts, kestra.WithBasicAuth(username, password))
	} else {
		return nil, fmt.Errorf("could not init client without any auth, at least token or username+password is required")
	}

	return &Client{
		API:    client,
		Kestra: kestra.NewClient(host, opts...),
		Ctx:    ctx,
		Tenant: tenant,
	}, nil
}

// resolveConfig returns (host, tenant, token, error) using Viper for precedence: flags > env > config file.
func resolveConfig() (string, string, string, string, string, error) {
	// Viper handles precedence automatically: flags > env > config > defaults
	host := strings.TrimSpace(viper.GetString(FlagHost))
	tenant := viper.GetString(FlagTenant)
	token := viper.GetString(FlagToken)
	username := viper.GetString(FlagUsername)
	password := viper.GetString(FlagPassword)

	// Set defaults if not provided
	if host == "" {
		host = "http://localhost:8080"
	}
	if tenant == "" {
		tenant = "main"
	}

	host = normalizeHost(host)

	return host, tenant, token, username, password, nil
}

// parseHeaders parses a slice of "Key:Value" strings into a map.
func parseHeaders(headers []string) (map[string]string, error) {
	result := make(map[string]string, len(headers))
	for _, h := range headers {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			return nil, fmt.Errorf("invalid header format %q: expected 'Key:Value'", h)
		}
		result[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return result, nil
}

func normalizeHost(host string) string {
	trimmed := strings.TrimSpace(host)
	if trimmed == "" {
		return trimmed
	}

	normalized := strings.TrimRight(trimmed, "/")
	if normalized == "" {
		return trimmed
	}

	return normalized
}

// formatSDKError extracts a user-friendly message from SDK errors. It handles
// both SDK error types: GenericOpenAPIError from the generated client and
// ApiError from the hand-written client.
func formatSDKError(err error) error {
	var sdkErr *kestra.GenericOpenAPIError
	if errors.As(err, &sdkErr) {
		return formatErrorBody(sdkErr.Body(), sdkErr.Error())
	}
	var apiErr *kestra.ApiError
	if errors.As(err, &apiErr) {
		// Don't use apiErr.Error() as the fallback message: it already embeds
		// an "API error <code>:" prefix and the raw body, which would leak
		// back into every formatErrorBody branch (double prefix, full HTML
		// pages in the hint). Build a short status-based message instead.
		errMsg := apiErr.Message
		if errMsg == "" {
			errMsg = fmt.Sprintf("status %d", apiErr.StatusCode)
		}
		return formatErrorBody(apiErr.Body, errMsg)
	}
	// The hand-written client decodes successful (2xx/3xx) responses as JSON.
	// When the server answers with HTML instead — typically a login page after
	// an auth redirect — json.Unmarshal fails with a syntax error like
	// "invalid character '<'". Surface a clear message rather than the raw
	// parser error.
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return fmt.Errorf("API request failed: received a non-JSON response (likely an HTML page). Check your authentication and permissions for this operation")
	}
	return err
}

// formatErrorBody builds a user-friendly error from a raw API response body,
// falling back to errMsg when the body carries no usable message.
func formatErrorBody(body []byte, errMsg string) error {
	// Try to parse as JSON first
	if len(body) > 0 {
		var jsonErr map[string]any
		if json.Unmarshal(body, &jsonErr) == nil {
			// Extract message from JSON response
			if msg, ok := jsonErr["message"].(string); ok && msg != "" {
				return fmt.Errorf("API error: %s", msg)
			}
			if msg, ok := jsonErr["error"].(string); ok && msg != "" {
				return fmt.Errorf("API error: %s", msg)
			}
		}
	}

	// Check if response is HTML (common for 404s, auth errors, etc.)
	isHTML := len(body) > 0 && body[0] == '<'
	if isHTML {
		// Extract HTTP status from error message if available
		if errMsg != "" {
			return fmt.Errorf("API request failed: %s (received HTML response instead of JSON)", errMsg)
		}
		return fmt.Errorf("API request failed: received HTML response instead of JSON. Check your host URL and authentication")
	}

	// For non-HTML/non-JSON responses, show the error message
	if errMsg != "" {
		return fmt.Errorf("API error: %s", errMsg)
	}

	return fmt.Errorf("API error: unknown error")
}

// tryParseExecutionFromError handles known SDK type mismatch bugs.
// Returns nil if the error is a real error and should be propagated.
func tryParseExecutionFromError(err error) map[string]any {
	sdkErr, ok := err.(*kestra.GenericOpenAPIError)
	if !ok {
		return nil
	}

	body := sdkErr.Body()
	if len(body) == 0 {
		return nil
	}

	var rawResp map[string]any
	if json.Unmarshal(body, &rawResp) != nil {
		return nil
	}

	// Only treat as success if we have an ID (indicates the request actually succeeded)
	if _, hasID := rawResp["id"]; !hasID {
		return nil
	}

	return rawResp
}

type namespaceListItem struct {
	ID      string
	Deleted bool
}

// tryParseNamespaceListFromError handles known SDK type mismatch bugs.
// Returns nil if the error is a real error and should be propagated.
func tryParseNamespaceListFromError(err error) []namespaceListItem {
	sdkErr, ok := err.(*kestra.GenericOpenAPIError)
	if !ok {
		return nil
	}

	body := sdkErr.Body()
	if len(body) == 0 {
		return nil
	}

	var rawResp map[string]any
	if json.Unmarshal(body, &rawResp) != nil {
		return nil
	}

	rawResults, ok := rawResp["results"].([]any)
	if !ok {
		return nil
	}

	items := make([]namespaceListItem, 0, len(rawResults))
	for _, raw := range rawResults {
		rawMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		id, ok := rawMap["id"].(string)
		if !ok || strings.TrimSpace(id) == "" {
			continue
		}

		deleted := false
		if rawDeleted, ok := rawMap["deleted"].(bool); ok {
			deleted = rawDeleted
		}

		items = append(items, namespaceListItem{ID: id, Deleted: deleted})
	}

	if len(items) == 0 {
		return nil
	}

	return items
}
