package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	kestra "github.com/kestra-io/client-sdk/go-sdk/v2/kestra_api_client"
)

func TestDecodeJSONPreservingNumbers(t *testing.T) {
	var got map[string]any
	if err := decodeJSONPreservingNumbers([]byte(`{"nanos":1725450000123456789,"nested":{"big":9007199254740993},"list":[9007199254740995]}`), &got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := got["nanos"].(json.Number); !ok {
		t.Fatalf("expected a json.Number, got %T", got["nanos"])
	}
	rendered := toPrettyString(got)
	for _, digits := range []string{"1725450000123456789", "9007199254740993", "9007199254740995"} {
		if !strings.Contains(rendered, digits) {
			t.Fatalf("expected %s in:\n%s", digits, rendered)
		}
	}
}

func TestDecodeJSONPreservingNumbers_Errors(t *testing.T) {
	var out any
	if err := decodeJSONPreservingNumbers([]byte(`{"a":1} {"b":2}`), &out); err == nil {
		t.Fatal("expected trailing data to be rejected")
	}
	if err := decodeJSONPreservingNumbers([]byte(`{`), &out); err == nil {
		t.Fatal("expected malformed JSON to be rejected")
	}
	if err := decodeJSONPreservingNumbers(nil, &out); err == nil {
		t.Fatal("expected an empty body to be rejected")
	}
}

func TestRawGet(t *testing.T) {
	var gotPath, gotAccept, gotAuth, gotExtra string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAccept = r.Header.Get("Accept")
		gotAuth = r.Header.Get("Authorization")
		gotExtra = r.Header.Get("X-Extra")
		_, _ = w.Write([]byte(`{"value":9007199254740993}`))
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	client.API.GetConfig().DefaultHeader["X-Extra"] = "yes"
	client.Ctx = context.WithValue(client.Ctx, kestra.ContextAccessToken, "tok")

	body, err := rawGet(client, "/api/v1/main/namespaces/ns")
	if err != nil {
		t.Fatalf("rawGet error: %v", err)
	}
	if string(body) != `{"value":9007199254740993}` {
		t.Fatalf("unexpected body: %s", body)
	}
	if gotPath != "/api/v1/main/namespaces/ns" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
	if gotAccept != "application/json" {
		t.Fatalf("unexpected Accept: %s", gotAccept)
	}
	if gotAuth != "Bearer tok" {
		t.Fatalf("expected the context token to be reused, got %q", gotAuth)
	}
	if gotExtra != "yes" {
		t.Fatalf("expected the SDK default headers to be reused, got %q", gotExtra)
	}
}

func TestRawGet_ErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"type":"https://kestra.io/docs/api-reference/problems/not-found","title":"Resource not found","status":404,"detail":"No namespace found"}`))
	}))
	t.Cleanup(server.Close)

	_, err := rawGet(newTestClient(t, server.URL), "/api/v1/main/namespaces/ns")
	if err == nil {
		t.Fatal("expected an error for a 404")
	}
	if !strings.Contains(err.Error(), "No namespace found") {
		t.Fatalf("expected the problem detail in the error, got: %v", err)
	}
}
