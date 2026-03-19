package cli

import (
	"encoding/json"
	"reflect"
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
