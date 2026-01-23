package cli

import (
	"context"
	"net/url"

	kestra "github.com/kestra-io/client-sdk/go-sdk/kestra_api_client"
)

// sdkClientFactory manages SDK client creation and authentication
type sdkClientFactory struct {
	authManager *AuthManager
}

// newSDKClientFactory creates a new factory instance
func newSDKClientFactory() *sdkClientFactory {
	return &sdkClientFactory{
		authManager: NewAuthManager(""),
	}
}

// createClient creates a new SDK client configured with the appropriate auth context
func (f *sdkClientFactory) createClient() (*kestra.APIClient, context.Context, error) {
	// Get auth context from flags or stored config
	authCtx := temporaryContext()
	if authCtx == nil {
		var err error
		authCtx, err = f.authManager.GetContext("")
		if err != nil {
			return nil, nil, err
		}
	}

	// Parse host URL to extract scheme
	hostURL, err := url.Parse(authCtx.Host)
	if err != nil {
		return nil, nil, err
	}

	scheme := hostURL.Scheme
	if scheme == "" {
		scheme = "http"
	}

	host := hostURL.Host
	if host == "" {
		host = hostURL.Path // Handle case where no scheme was provided
	}

	// Create SDK configuration
	cfg := kestra.NewConfiguration()
	cfg.Scheme = scheme
	cfg.Host = host
	cfg.Servers = kestra.ServerConfigurations{
		{
			URL: authCtx.Host,
		},
	}

	// Create the client
	client := kestra.NewAPIClient(cfg)

	// Create authenticated context
	ctx := f.createAuthContext(authCtx)

	return client, ctx, nil
}

// createAuthContext creates a context with the appropriate authentication
func (f *sdkClientFactory) createAuthContext(authCtx *AuthContext) context.Context {
	ctx := context.Background()

	if authCtx.AuthMethod == "token" && authCtx.Token != "" {
		ctx = context.WithValue(ctx, kestra.ContextAccessToken, authCtx.Token)
	} else if authCtx.AuthMethod == "username_password" && authCtx.Username != "" {
		ctx = context.WithValue(ctx, kestra.ContextBasicAuth, kestra.BasicAuth{
			UserName: authCtx.Username,
			Password: authCtx.Password,
		})
	}

	return ctx
}

// resolveTenant returns the tenant from global flags or from the auth context
func resolveTenant(authCtx *AuthContext) string {
	if globalFlags.Tenant != "" {
		return globalFlags.Tenant
	}
	if authCtx != nil && authCtx.Tenant != "" {
		return authCtx.Tenant
	}
	return "main"
}
