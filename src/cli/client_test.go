package cli

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unsafe"

	kestra "github.com/kestra-io/client-sdk/go-sdk/v2/kestra_api_client"
	"github.com/spf13/viper"
)

// doRawRequest is the shared path for the endpoints the SDK cannot express
// (the path-less webhook) or mistypes (namespace inherited variables), so its
// URL building, auth and error handling are covered here rather than only
// transitively through those two callers.
func TestClientDoRawRequest(t *testing.T) {
	t.Run("builds the tenant path and returns the body", func(t *testing.T) {
		var gotPath, gotMethod, gotAccept string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath, gotMethod, gotAccept = r.URL.Path, r.Method, r.Header.Get("Accept")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))
		t.Cleanup(server.Close)

		body, err := newTestClient(t, server.URL).doRawRequest(http.MethodGet, "namespaces", "my.namespace", "inherited-variables")
		if err != nil {
			t.Fatalf("doRawRequest error: %v", err)
		}
		if string(body) != `{"ok":true}` {
			t.Fatalf("unexpected body: %s", body)
		}
		if gotPath != "/api/v1/main/namespaces/my.namespace/inherited-variables" {
			t.Fatalf("unexpected path: %s", gotPath)
		}
		if gotMethod != http.MethodGet || gotAccept != "application/json" {
			t.Fatalf("unexpected method/accept: %s %s", gotMethod, gotAccept)
		}
	})

	t.Run("escapes path segments", func(t *testing.T) {
		var gotRawPath string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotRawPath = r.URL.EscapedPath()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		}))
		t.Cleanup(server.Close)

		if _, err := newTestClient(t, server.URL).doRawRequest(http.MethodGet, "namespaces", "a b/c"); err != nil {
			t.Fatalf("doRawRequest error: %v", err)
		}
		// A segment containing a slash must not add a path element.
		if gotRawPath != "/api/v1/main/namespaces/a%20b%2Fc" {
			t.Fatalf("unexpected escaped path: %s", gotRawPath)
		}
	})

	t.Run("sends the token from the context", func(t *testing.T) {
		var gotAuth string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		}))
		t.Cleanup(server.Close)

		client := newTestClient(t, server.URL)
		client.Ctx = context.WithValue(client.Ctx, kestra.ContextAccessToken, "s3cret-token")

		if _, err := client.doRawRequest(http.MethodGet, "namespaces"); err != nil {
			t.Fatalf("doRawRequest error: %v", err)
		}
		if gotAuth != "Bearer s3cret-token" {
			t.Fatalf("unexpected Authorization header: %q", gotAuth)
		}
	})

	t.Run("sends basic auth from the context", func(t *testing.T) {
		var gotUser string
		var gotOK bool
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotUser, _, gotOK = r.BasicAuth()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		}))
		t.Cleanup(server.Close)

		client := newTestClient(t, server.URL)
		client.Ctx = context.WithValue(client.Ctx, kestra.ContextBasicAuth, kestra.BasicAuth{UserName: "root@root.com", Password: "pw"})

		if _, err := client.doRawRequest(http.MethodGet, "namespaces"); err != nil {
			t.Fatalf("doRawRequest error: %v", err)
		}
		if !gotOK || gotUser != "root@root.com" {
			t.Fatalf("expected basic auth to be sent, got user %q (ok=%v)", gotUser, gotOK)
		}
	})

	t.Run("formats the error body on 404", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"namespace not found"}`))
		}))
		t.Cleanup(server.Close)

		_, err := newTestClient(t, server.URL).doRawRequest(http.MethodGet, "namespaces", "missing")
		if err == nil {
			t.Fatal("expected an error for a 404 response")
		}
		if !strings.Contains(err.Error(), "namespace not found") {
			t.Fatalf("expected the server message in the error, got: %v", err)
		}
	})

	t.Run("falls back to the status when the body carries no message", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`boom`))
		}))
		t.Cleanup(server.Close)

		_, err := newTestClient(t, server.URL).doRawRequest(http.MethodGet, "namespaces")
		if err == nil {
			t.Fatal("expected an error for a 500 response")
		}
		if !strings.Contains(err.Error(), "status 500") {
			t.Fatalf("expected the status in the error, got: %v", err)
		}
	})
}

func setGenericOpenAPIErrorBody(err *kestra.GenericOpenAPIError, body []byte) {
	field := reflect.ValueOf(err).Elem().FieldByName("body")
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().SetBytes(body)
}

func TestTryParseNamespaceListFromError(t *testing.T) {
	payload := map[string]any{
		"results": []any{
			map[string]any{
				"id":        "team.alpha",
				"deleted":   false,
				"variables": "{}",
			},
			map[string]any{
				"id":      "team.beta",
				"deleted": true,
				"variables": map[string]any{
					"foo": map[string]any{"bar": "baz"},
				},
			},
		},
		"total": 2,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	sdkErr := &kestra.GenericOpenAPIError{}
	setGenericOpenAPIErrorBody(sdkErr, body)

	items := tryParseNamespaceListFromError(sdkErr)
	if items == nil {
		t.Fatal("expected fallback items, got nil")
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	if items[0].ID != "team.alpha" || items[0].Deleted {
		t.Fatalf("unexpected first item: %+v", items[0])
	}

	if items[1].ID != "team.beta" || !items[1].Deleted {
		t.Fatalf("unexpected second item: %+v", items[1])
	}
}

func TestFormatSDKError_ApiError(t *testing.T) {
	t.Run("json message body", func(t *testing.T) {
		err := formatSDKError(&kestra.ApiError{
			StatusCode: 404,
			Body:       []byte(`{"message":"role not found"}`),
		})
		if err == nil || err.Error() != "API error: role not found" {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("html body", func(t *testing.T) {
		err := formatSDKError(&kestra.ApiError{
			StatusCode: 404,
			Body:       []byte("<html><body>Not Found</body></html>"),
			Message:    "Not Found",
		})
		if err == nil || !strings.Contains(err.Error(), "HTML response") {
			t.Fatalf("expected HTML hint, got: %v", err)
		}
	})

	t.Run("falls back to message", func(t *testing.T) {
		err := formatSDKError(&kestra.ApiError{StatusCode: 500, Message: "boom"})
		if err == nil || !strings.Contains(err.Error(), "boom") {
			t.Fatalf("expected fallback message, got: %v", err)
		}
	})

	t.Run("plain text body has no double prefix", func(t *testing.T) {
		err := formatSDKError(&kestra.ApiError{
			StatusCode: 403,
			Body:       []byte("Forbidden"),
		})
		if err == nil || err.Error() != "API error: status 403" {
			t.Fatalf("expected 'API error: status 403', got: %v", err)
		}
	})

	t.Run("empty body and message uses status code", func(t *testing.T) {
		err := formatSDKError(&kestra.ApiError{StatusCode: 502})
		if err == nil || err.Error() != "API error: status 502" {
			t.Fatalf("expected 'API error: status 502', got: %v", err)
		}
	})

	t.Run("html body does not leak the page into the hint", func(t *testing.T) {
		err := formatSDKError(&kestra.ApiError{
			StatusCode: 401,
			Body:       []byte("<html><body>big page</body></html>"),
		})
		if err == nil || strings.Contains(err.Error(), "big page") {
			t.Fatalf("expected HTML body to be omitted, got: %v", err)
		}
		if !strings.Contains(err.Error(), "HTML response") {
			t.Fatalf("expected HTML hint, got: %v", err)
		}
	})

	t.Run("json syntax error from html response", func(t *testing.T) {
		// Mimic the hand-written client decoding an HTML page as JSON.
		var htmlAsJSON map[string]any
		jsonErr := json.Unmarshal([]byte("<html><body>login</body></html>"), &htmlAsJSON)
		if jsonErr == nil {
			t.Fatal("expected json unmarshal to fail")
		}
		err := formatSDKError(jsonErr)
		if err == nil || !strings.Contains(err.Error(), "non-JSON response") {
			t.Fatalf("expected non-JSON hint, got: %v", err)
		}
	})

	t.Run("unrelated error passes through", func(t *testing.T) {
		orig := errors.New("plain")
		if got := formatSDKError(orig); got != orig {
			t.Fatalf("expected passthrough, got: %v", got)
		}
	})
}

func TestParseHeaders(t *testing.T) {
	t.Run("custom header", func(t *testing.T) {
		got, err := parseHeaders([]string{"X-Custom-Header:my-value"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["X-Custom-Header"] != "my-value" {
			t.Fatalf("expected 'my-value', got %q", got["X-Custom-Header"])
		}
	})

	t.Run("cookie header", func(t *testing.T) {
		got, err := parseHeaders([]string{"Cookie:session=abc123; user=john"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["Cookie"] != "session=abc123; user=john" {
			t.Fatalf("expected 'session=abc123; user=john', got %q", got["Cookie"])
		}
	})
}

func TestNormalizeHost(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "no trailing slash",
			in:   "http://localhost:8080",
			want: "http://localhost:8080",
		},
		{
			name: "single trailing slash",
			in:   "http://localhost:8080/",
			want: "http://localhost:8080",
		},
		{
			name: "multiple trailing slashes",
			in:   "http://localhost:8080///",
			want: "http://localhost:8080",
		},
		{
			name: "path trailing slash",
			in:   "https://example.com/api/",
			want: "https://example.com/api",
		},
		{
			name: "whitespace",
			in:   "  https://example.com/api/  ",
			want: "https://example.com/api",
		},
		{
			name: "empty",
			in:   "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeHost(tt.in)
			if got != tt.want {
				t.Fatalf("expected '%s', got '%s'", tt.want, got)
			}
		})
	}
}

func TestTryParseActionsFromError(t *testing.T) {
	t.Run("parses raw string array", func(t *testing.T) {
		sdkErr := &kestra.GenericOpenAPIError{}
		setGenericOpenAPIErrorBody(sdkErr, []byte(`["READ","CREATE","UPDATE","DELETE"]`))
		actions := tryParseActionsFromError(sdkErr)
		if len(actions) != 4 || actions[0] != "READ" || actions[3] != "DELETE" {
			t.Fatalf("unexpected actions: %v", actions)
		}
	})

	t.Run("nil for non-array body", func(t *testing.T) {
		sdkErr := &kestra.GenericOpenAPIError{}
		setGenericOpenAPIErrorBody(sdkErr, []byte(`{"message":"boom"}`))
		if actions := tryParseActionsFromError(sdkErr); actions != nil {
			t.Fatalf("expected nil, got: %v", actions)
		}
	})

	t.Run("nil for unrelated error", func(t *testing.T) {
		if actions := tryParseActionsFromError(errors.New("plain")); actions != nil {
			t.Fatalf("expected nil, got: %v", actions)
		}
	})
}

// --- auth-method precedence (issue #120) ---

// writeAuthConfig writes a config.yaml with a single default context and
// returns its path.
func writeAuthConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// initConfigForTest parses args on a fresh root command and runs the real
// initializeConfig, so tests exercise the whole flags > env > config chain.
func initConfigForTest(t *testing.T, args ...string) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	root := NewRootCommand()
	if err := root.ParseFlags(args); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	if err := initializeConfig(root); err != nil {
		t.Fatalf("initializeConfig: %v", err)
	}
}

const tokenContextConfig = `default_context: tokenctx
contexts:
  tokenctx:
    host: http://example.invalid
    tenant: othertenant
    auth_method: token
    token: config-token
`

const basicContextConfig = `default_context: basicctx
contexts:
  basicctx:
    host: http://example.invalid
    tenant: othertenant
    auth_method: basic
    username: config-user
    password: config-pass
`

func TestResolveConfigAuthPrecedence(t *testing.T) {
	tests := []struct {
		name     string
		config   string
		env      map[string]string
		args     []string
		want     resolvedAuth
		errorHas string
	}{
		{
			name:   "flag basic auth beats config file token",
			config: tokenContextConfig,
			args:   []string{"--host", "http://localhost:9801", "--username", "u", "--password", "p"},
			want:   resolvedAuth{Method: authMethodBasic, Username: "u", Password: "p"},
		},
		{
			name: "flag basic auth with no config file",
			args: []string{"--host", "http://localhost:9801", "--username", "u", "--password", "p"},
			want: resolvedAuth{Method: authMethodBasic, Username: "u", Password: "p"},
		},
		{
			name: "flag basic auth beats env token",
			env:  map[string]string{"KESTRACTL_TOKEN": "env-token"},
			args: []string{"--username", "u", "--password", "p"},
			want: resolvedAuth{Method: authMethodBasic, Username: "u", Password: "p"},
		},
		{
			name:   "env basic auth beats config file token",
			config: tokenContextConfig,
			env:    map[string]string{"KESTRACTL_USERNAME": "envuser", "KESTRACTL_PASSWORD": "envpass"},
			want:   resolvedAuth{Method: authMethodBasic, Username: "envuser", Password: "envpass"},
		},
		{
			name:   "flag token beats config file basic auth",
			config: basicContextConfig,
			args:   []string{"--token", "flag-token"},
			want:   resolvedAuth{Method: authMethodToken, Token: "flag-token"},
		},
		{
			name:   "flag basic auth overrides config file basic auth values",
			config: basicContextConfig,
			args:   []string{"--username", "u", "--password", "p"},
			want:   resolvedAuth{Method: authMethodBasic, Username: "u", Password: "p"},
		},
		{
			name:   "config file token used when nothing else is set",
			config: tokenContextConfig,
			want:   resolvedAuth{Method: authMethodToken, Token: "config-token"},
		},
		{
			name:   "config file basic auth used when nothing else is set",
			config: basicContextConfig,
			want:   resolvedAuth{Method: authMethodBasic, Username: "config-user", Password: "config-pass"},
		},
		{
			name: "token wins over basic auth within the same source",
			args: []string{"--token", "flag-token", "--username", "u", "--password", "p"},
			want: resolvedAuth{Method: authMethodToken, Token: "flag-token"},
		},
		{
			name:   "values for the chosen method still merge across sources",
			config: basicContextConfig,
			args:   []string{"--password", "flagpass"},
			want:   resolvedAuth{Method: authMethodBasic, Username: "config-user", Password: "flagpass"},
		},
		{
			name:     "incomplete basic auth from flags is a hard error, not a token fallback",
			config:   tokenContextConfig,
			args:     []string{"--username", "u"},
			errorHas: "password",
		},
		{
			name:     "incomplete basic auth from the config file is a hard error",
			config:   "default_context: c\ncontexts:\n  c:\n    username: only-user\n",
			errorHas: "password",
		},
		{
			name:     "basic auth with only a password names the missing username",
			args:     []string{"--password", "p"},
			errorHas: "username",
		},
		{
			// Documents the behaviour of an explicitly empty --token: it carries
			// no auth information, so lower-precedence sources still apply.
			name:   "empty token flag does not select the token method",
			config: basicContextConfig,
			args:   []string{"--token", ""},
			want:   resolvedAuth{Method: authMethodBasic, Username: "config-user", Password: "config-pass"},
		},
		{
			name: "no auth configured anywhere",
			want: resolvedAuth{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			args := tt.args
			if tt.config != "" {
				args = append([]string{"--config", writeAuthConfig(t, tt.config)}, args...)
			} else {
				// Never let the developer's real ~/.kestractl/config.yaml leak in.
				args = append([]string{"--config", writeAuthConfig(t, "")}, args...)
			}
			initConfigForTest(t, args...)

			_, _, auth, err := resolveConfig()
			if tt.errorHas != "" {
				if err == nil {
					t.Fatalf("expected an error mentioning %q, got auth %+v", tt.errorHas, auth)
				}
				if !strings.Contains(err.Error(), tt.errorHas) {
					t.Fatalf("error %q does not mention %q", err.Error(), tt.errorHas)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if auth != tt.want {
				t.Fatalf("auth = %+v, want %+v", auth, tt.want)
			}
		})
	}
}

// TestIsProblemDocument_StatusNumberForms covers both decoders: a plain
// json.Unmarshal yields float64, while UseNumber (used by the KV paths to keep
// integers above 2^53 exact) yields json.Number. Either must still be
// recognised, or a 404 would render as a record with a zero exit code.
func TestIsProblemDocument_StatusNumberForms(t *testing.T) {
	tests := []struct {
		name string
		doc  map[string]any
		want bool
	}{
		{name: "float64 status", doc: map[string]any{"status": float64(404), "title": "Resource not found"}, want: true},
		{name: "json.Number status", doc: map[string]any{"status": json.Number("404"), "title": "Resource not found"}, want: true},
		{name: "json.Number status with detail only", doc: map[string]any{"status": json.Number("404"), "detail": "No value found"}, want: true},
		{name: "json.Number status without title or detail", doc: map[string]any{"status": json.Number("404")}, want: false},
		{name: "string status", doc: map[string]any{"status": "404", "title": "Resource not found"}, want: false},
		{name: "no status", doc: map[string]any{"title": "Resource not found"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isProblemDocument(tt.doc); got != tt.want {
				t.Fatalf("isProblemDocument = %v, want %v", got, tt.want)
			}
			if tt.want && problemMessage(tt.doc) == "" {
				t.Fatal("expected a rendered problem message")
			}
		})
	}
}
