package apiclient

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// KestraClient performs authenticated HTTP requests.
type KestraClient struct {
	authManager *AuthManager
	httpClient  *http.Client
}

const (
	httpMethodGet    = "GET"
	httpMethodPost   = "POST"
	httpMethodPut    = "PUT"
	httpMethodDelete = "DELETE"
)

// NewKestraClient constructs a client with the supplied AuthManager.
func NewKestraClient(authManager *AuthManager) *KestraClient {
	if authManager == nil {
		authManager = NewAuthManager("")
	}

	return &KestraClient{
		authManager: authManager,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// RequestOptions encapsulates optional parameters for requests.
type RequestOptions struct {
	Context     *AuthContext
	Params      url.Values
	Headers     map[string]string
	Body        io.Reader
	JSON        any
	ContentType string
}

// Do issues an HTTP request to the Kestra API.
func (c *KestraClient) Do(method, endpoint string, opts RequestOptions) ([]byte, error) {
	ctx := opts.Context
	if ctx == nil {
		var err error
		ctx, err = c.authManager.GetContext("")
		if err != nil {
			return nil, err
		}
	}

	base := strings.TrimRight(ctx.Host, "/")
	if base == "" {
		return nil, fmt.Errorf("invalid host for context '%s'", ctx.Name)
	}

	path := strings.TrimLeft(endpoint, "/")
	fullURL := fmt.Sprintf("%s/%s", base, path)

	var body io.Reader
	if opts.JSON != nil {
		buf, err := json.Marshal(opts.JSON)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(buf)
		if opts.Headers == nil {
			opts.Headers = map[string]string{}
		}
		opts.Headers["Content-Type"] = "application/json"
	} else if opts.Body != nil {
		body = opts.Body
	}

	req, err := http.NewRequest(method, fullURL, body)
	if err != nil {
		return nil, err
	}

	if opts.Params != nil && len(opts.Params) > 0 {
		req.URL.RawQuery = opts.Params.Encode()
	}

	headers := map[string]string{}
	for k, v := range opts.Headers {
		headers[k] = v
	}

	if opts.ContentType != "" {
		headers["Content-Type"] = opts.ContentType
	}

	if ctx.AuthMethod == "token" && ctx.Token != "" {
		headers["Authorization"] = fmt.Sprintf("Bearer %s", ctx.Token)
	} else if ctx.AuthMethod == "username_password" && ctx.Username != "" {
		auth := ctx.Username + ":" + ctx.Password
		encoded := base64.StdEncoding.EncodeToString([]byte(auth))
		headers["Authorization"] = "Basic " + encoded
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	return respBody, nil
}

// AuthManager exposes the internal auth manager.
func (c *KestraClient) AuthManager() *AuthManager {
	return c.authManager
}

// ResolveContext returns the provided context or falls back to the default one.
func (c *KestraClient) ResolveContext(ctx *AuthContext) (*AuthContext, error) {
	if ctx != nil {
		return ctx, nil
	}
	return c.authManager.GetContext("")
}

// ResolveTenant determines the tenant to use for a request.
func (c *KestraClient) ResolveTenant(ctx *AuthContext, tenant string) string {
	if tenant != "" {
		return tenant
	}
	if ctx != nil && strings.TrimSpace(ctx.Tenant) != "" {
		return ctx.Tenant
	}
	return "main"
}
