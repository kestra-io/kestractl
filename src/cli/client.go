package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"

	kestra "github.com/kestra-io/client-sdk/go-sdk/v2/kestra_api_client"
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

	// Kestra version reported by the server, resolved lazily by ServerVersion.
	serverVersion     string
	serverVersionErr  error
	serverVersionOnce sync.Once
}

// newClientFunc is the client factory function. Override in tests.
var newClientFunc = newClientDefault

// NewClient creates a client by resolving config from flags > env > config file.
func NewClient() (*Client, error) {
	return newClientFunc()
}

// newClientDefault is the default client creation logic.
func newClientDefault() (*Client, error) {
	host, tenant, auth, err := resolveConfig()

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
	// The generated SDK debug logger prints the Authorization header verbatim,
	// so both SDK loggers stay off and compatTransport does the dumping with
	// uniform masking instead (issue #119).
	cfg.Debug = false

	// One HTTP client for both SDK clients, so the Kestra 1.x response shim
	// (see compat.go) applies to generated and hand-written endpoints alike.
	httpClient, compat := newCompatHTTPClient()
	cfg.HTTPClient = httpClient

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
	opts := []kestra.ClientOption{kestra.WithDebug(false), kestra.WithHTTPClient(httpClient)}
	if len(parsed) > 0 {
		opts = append(opts, kestra.WithHeaders(parsed))
	}

	ctx := context.Background()
	if auth.Token != "" {
		ctx = context.WithValue(ctx, kestra.ContextAccessToken, auth.Token)
		opts = append(opts, kestra.WithTokenAuth(auth.Token))
	} else if auth.Username != "" && auth.Password != "" {
		ctx = context.WithValue(ctx, kestra.ContextBasicAuth, kestra.BasicAuth{
			UserName: auth.Username,
			Password: auth.Password,
		})
		opts = append(opts, kestra.WithBasicAuth(auth.Username, auth.Password))
	} else {
		return nil, fmt.Errorf("could not init client without any auth, at least token or username+password is required")
	}

	c := &Client{
		API:    client,
		Kestra: kestra.NewClient(host, opts...),
		Ctx:    ctx,
		Tenant: tenant,
	}
	compat.era = c.serverEra
	return c, nil
}

// Auth methods reported by resolvedAuth.
const (
	authMethodToken = "token"
	authMethodBasic = "basic"
)

// resolvedAuth is the authentication kestractl will use for this invocation.
// Only the fields belonging to Method are populated; Method is empty when no
// source configured any authentication at all.
type resolvedAuth struct {
	Method   string
	Token    string
	Username string
	Password string
}

// resolveConfig returns (host, tenant, auth, error) using Viper for precedence: flags > env > config file.
func resolveConfig() (string, string, resolvedAuth, error) {
	// Viper handles precedence automatically: flags > env > config > defaults
	host := strings.TrimSpace(viper.GetString(FlagHost))
	tenant := viper.GetString(FlagTenant)

	// Set defaults if not provided
	if host == "" {
		host = "http://localhost:8080"
	}
	if tenant == "" {
		tenant = "main"
	}

	host = normalizeHost(host)

	auth, err := resolveAuth()
	if err != nil {
		return host, tenant, resolvedAuth{}, err
	}

	return host, tenant, auth, nil
}

// explicitAuthFlags records which auth flags the user set on the command line
// with a non-empty value. It is populated by initializeConfig and stays empty
// for callers that never parse flags.
var explicitAuthFlags map[string]bool

// authFieldFromFlag reports whether the named auth field was set by a flag.
func authFieldFromFlag(name string) bool { return explicitAuthFlags[name] }

// authFieldFromEnv reports whether the named auth field was set by a
// KESTRACTL_* environment variable.
func authFieldFromEnv(name string) bool {
	v, ok := os.LookupEnv(envPrefix + "_" + strings.ToUpper(name))
	return ok && strings.TrimSpace(v) != ""
}

// authFieldResolved reports whether the named auth field has any value at all.
// Used as the last source in the precedence chain, where a value can only come
// from the config file's default context.
func authFieldResolved(name string) bool { return viper.GetString(name) != "" }

// resolveAuth picks the authentication method and its values.
//
// The method is decided by the highest-precedence source (flags > env > config
// file) that sets any of token/username/password; a token wins over basic auth
// within the same source. Once the method is chosen, its individual values
// still resolve through the normal per-field Viper precedence, so a config-file
// username can pair with a --password flag. Fields belonging to the losing
// method are dropped rather than silently taking over the request (issue #120).
func resolveAuth() (resolvedAuth, error) {
	method := ""
	for _, sets := range []func(string) bool{authFieldFromFlag, authFieldFromEnv, authFieldResolved} {
		switch {
		case sets(FlagToken):
			method = authMethodToken
		case sets(FlagUsername) || sets(FlagPassword):
			method = authMethodBasic
		}
		if method != "" {
			break
		}
	}

	switch method {
	case authMethodToken:
		token := viper.GetString(FlagToken)
		if token == "" {
			return resolvedAuth{}, missingAuthFieldsError([]string{FlagToken})
		}
		return resolvedAuth{Method: authMethodToken, Token: token}, nil
	case authMethodBasic:
		username := viper.GetString(FlagUsername)
		password := viper.GetString(FlagPassword)
		var missing []string
		if username == "" {
			missing = append(missing, FlagUsername)
		}
		if password == "" {
			missing = append(missing, FlagPassword)
		}
		if len(missing) > 0 {
			return resolvedAuth{}, missingAuthFieldsError(missing)
		}
		return resolvedAuth{Method: authMethodBasic, Username: username, Password: password}, nil
	}

	return resolvedAuth{}, nil
}

// missingAuthFieldsError explains which auth fields are missing and where they
// can be set.
func missingAuthFieldsError(missing []string) error {
	hints := make([]string, 0, len(missing))
	for _, name := range missing {
		hints = append(hints, fmt.Sprintf("--%s, %s_%s, or %q in the config file's default context", name, envPrefix, strings.ToUpper(name), name))
	}
	return fmt.Errorf("incomplete authentication: missing %s (set %s)", strings.Join(missing, " and "), strings.Join(hints, "; "))
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
			if msg := problemMessage(jsonErr); msg != "" {
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

// Kestra 2.0 answers errors with an RFC 7807 problem document
// ({"type","title","status","detail"}) instead of the pre-2.0
// {"message": ...} shape. isProblemDocument recognises one so the
// tryParse*FromError fallbacks below don't mistake it for a payload: a
// problem document carries a "type" key, which those helpers would
// otherwise read as the resource's own type and report success on a 404.
func isProblemDocument(m map[string]any) bool {
	// "status" is a float64 after a plain json.Unmarshal and a json.Number when
	// the body was decoded with json.Decoder.UseNumber (as the KV paths do to
	// keep integers above 2^53 exact), so accept either.
	switch m["status"].(type) {
	case float64, json.Number:
	default:
		return false
	}
	if _, ok := m["title"].(string); ok {
		return true
	}
	_, ok := m["detail"].(string)
	return ok
}

// problemMessage renders an RFC 7807 problem document as a single line,
// preferring the title and detail together since the title alone is generic
// ("Resource not found") and the detail alone omits the category.
func problemMessage(m map[string]any) string {
	if !isProblemDocument(m) {
		return ""
	}
	title, _ := m["title"].(string)
	detail, _ := m["detail"].(string)
	switch {
	case title != "" && detail != "" && title != detail:
		return title + ": " + detail
	case detail != "":
		return detail
	default:
		return title
	}
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

	// UseNumber: this body is rendered as-is, user data included (a JSON input
	// with epoch nanos, a large ID), and plain json.Unmarshal would round every
	// integer above 2^53 through float64 (follow-up to #121).
	var rawResp map[string]any
	if decodeJSONPreservingNumbers(body, &rawResp) != nil {
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
