package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	kestra "github.com/kestra-io/client-sdk/go-sdk/kestra_api_client"
	"github.com/spf13/viper"
)

// Client wraps the Kestra SDK with authentication and tenant info.
type Client struct {
	API    *kestra.APIClient
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
	host, tenant, token, err := resolveConfig()
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

	client := kestra.NewAPIClient(cfg)
	ctx := context.WithValue(context.Background(), kestra.ContextAccessToken, token)

	return &Client{
		API:    client,
		Ctx:    ctx,
		Tenant: tenant,
	}, nil
}

// resolveConfig returns (host, tenant, token, error) using Viper for precedence: flags > env > config file.
func resolveConfig() (string, string, string, error) {
	// Viper handles precedence automatically: flags > env > config > defaults
	host := viper.GetString("host")
	tenant := viper.GetString("tenant")
	token := viper.GetString("token")

	// Set defaults if not provided
	if host == "" {
		host = "http://localhost:8080"
	}
	if tenant == "" {
		tenant = "main"
	}

	return host, tenant, token, nil
}

// formatSDKError extracts a user-friendly message from SDK errors.
func formatSDKError(err error) error {
	if sdkErr, ok := err.(*kestra.GenericOpenAPIError); ok {
		body := sdkErr.Body()
		
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
		
		bodyStr := string(body)
		
		// Check if response is HTML (common for 404s, auth errors, etc.)
		isHTML := len(bodyStr) > 0 && bodyStr[0] == '<'
		if isHTML {
			// Extract HTTP status from error message if available
			errMsg := sdkErr.Error()
			if errMsg != "" {
				return fmt.Errorf("API request failed: %s (received HTML response instead of JSON)", errMsg)
			}
			return fmt.Errorf("API request failed: received HTML response instead of JSON. Check your host URL and authentication")
		}
		
		// For non-HTML/non-JSON responses, show the error message
		if errMsg := sdkErr.Error(); errMsg != "" {
			return fmt.Errorf("API error: %s", errMsg)
		}
		
		return fmt.Errorf("API error: unknown error")
	}
	return err
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
