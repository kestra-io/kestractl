package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"

	kestra "github.com/kestra-io/client-sdk/go-sdk/kestra_api_client"
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

// resolveConfig returns (host, tenant, token, error) with precedence: flags > env > config file.
func resolveConfig() (string, string, string, error) {
	// Priority 1: CLI flags
	if globalFlags.Host != "" || globalFlags.Token != "" {
		host := globalFlags.Host
		if host == "" {
			host = "http://localhost:8080"
		}
		tenant := globalFlags.Tenant
		if tenant == "" {
			tenant = "main"
		}
		return host, tenant, globalFlags.Token, nil
	}

	// Priority 2: Environment variables
	if os.Getenv("KESTRA_HOST") != "" || os.Getenv("KESTRA_TOKEN") != "" {
		host := os.Getenv("KESTRA_HOST")
		if host == "" {
			host = "http://localhost:8080"
		}
		tenant := os.Getenv("KESTRA_TENANT")
		if tenant == "" {
			tenant = "main"
		}
		return host, tenant, os.Getenv("KESTRA_TOKEN"), nil
	}

	// Priority 3: Config file
	mgr := NewAuthManager("")
	authCtx, err := mgr.GetContext("")
	if err != nil {
		return "", "", "", err
	}

	return authCtx.Host, authCtx.Tenant, authCtx.Token, nil
}

// formatSDKError extracts a user-friendly message from SDK errors.
func formatSDKError(err error) error {
	if sdkErr, ok := err.(*kestra.GenericOpenAPIError); ok {
		body := string(sdkErr.Body())
		if body != "" {
			return fmt.Errorf("API error: %s", body)
		}
		return fmt.Errorf("API error: %s", sdkErr.Error())
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
