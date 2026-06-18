package cli

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"unsafe"

	kestra "github.com/kestra-io/client-sdk/go-sdk/kestra_api_client"
)

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
