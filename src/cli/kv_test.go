package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	kestra "github.com/kestra-io/client-sdk/go-sdk/v2/kestra_api_client"
	"github.com/spf13/cobra"
)

func TestKVCommand_Structure(t *testing.T) {
	cmd := newKVCommand()

	if cmd.Use != "kv" {
		t.Fatalf("expected use 'kv', got '%s'", cmd.Use)
	}

	expected := map[string]bool{
		"list":   false,
		"set":    false,
		"update": false,
		"get":    false,
		"delete": false,
	}

	for _, sub := range cmd.Commands() {
		if _, ok := expected[sub.Name()]; ok {
			expected[sub.Name()] = true
		}
	}

	for name, found := range expected {
		if !found {
			t.Fatalf("expected '%s' subcommand", name)
		}
	}
}

func TestKVListCommand_AcceptsOptionalNamespace(t *testing.T) {
	cmd := newKVListCommand()

	if cmd.Args == nil {
		t.Fatal("expected args validation")
	}

	// 0 args should be accepted by the arg validator
	if err := cobra.MaximumNArgs(1)(cmd, []string{}); err != nil {
		t.Fatalf("expected 0 args to be valid: %v", err)
	}

	// 1 arg should be accepted
	if err := cobra.MaximumNArgs(1)(cmd, []string{"my.namespace"}); err != nil {
		t.Fatalf("expected 1 arg to be valid: %v", err)
	}

	// 2 args should fail
	if err := cobra.MaximumNArgs(1)(cmd, []string{"a", "b"}); err == nil {
		t.Fatal("expected 2 args to be rejected")
	}
}

func TestKVSetCommand_NoArgs(t *testing.T) {
	cmd := newKVSetCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 4 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestKVGetCommand_NoArgs(t *testing.T) {
	cmd := newKVGetCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 2 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestKVDeleteCommand_NoArgs(t *testing.T) {
	cmd := newKVDeleteCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 2 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestKVSetCommand_MissingTypeArg(t *testing.T) {
	cmd := newKVSetCommand()
	_, err := executeCommand(cmd, "my.ns", "my-key", "my-value")
	if err == nil {
		t.Fatal("expected missing type error")
	}
	if !strings.Contains(err.Error(), "accepts 4 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestParseKVType(t *testing.T) {
	kvType, err := parseKVType("string")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if kvType != kestra.KVTYPE_STRING {
		t.Fatalf("expected STRING, got %s", kvType)
	}

	_, err = parseKVType("invalid")
	if err == nil {
		t.Fatal("expected invalid type error")
	}
}

func TestFormatKVRequestBody(t *testing.T) {
	tests := []struct {
		name      string
		kvType    kestra.KVType
		rawValue  string
		wantBody  string
		wantError bool
	}{
		{name: "string", kvType: kestra.KVTYPE_STRING, rawValue: "hello", wantBody: `"hello"`},
		{name: "number", kvType: kestra.KVTYPE_NUMBER, rawValue: "42", wantBody: "42"},
		{name: "boolean", kvType: kestra.KVTYPE_BOOLEAN, rawValue: "TRUE", wantBody: "true"},
		{name: "json", kvType: kestra.KVTYPE_JSON, rawValue: `{"a":1}`, wantBody: `{"a":1}`},
		{name: "date", kvType: kestra.KVTYPE_DATE, rawValue: "2025-10-13", wantBody: "2025-10-13"},
		{name: "datetime", kvType: kestra.KVTYPE_DATETIME, rawValue: "2025-10-14T18:02:08Z", wantBody: "2025-10-14T18:02:08Z"},
		{name: "duration", kvType: kestra.KVTYPE_DURATION, rawValue: "PT15M", wantBody: "PT15M"},
		{name: "invalid number", kvType: kestra.KVTYPE_NUMBER, rawValue: "abc", wantError: true},
		{name: "invalid bool", kvType: kestra.KVTYPE_BOOLEAN, rawValue: "yes", wantError: true},
		{name: "invalid json", kvType: kestra.KVTYPE_JSON, rawValue: "{", wantError: true},
		{name: "invalid date", kvType: kestra.KVTYPE_DATE, rawValue: "2025/10/13", wantError: true},
		{name: "invalid datetime", kvType: kestra.KVTYPE_DATETIME, rawValue: "2025-10-14", wantError: true},
		{name: "invalid duration", kvType: kestra.KVTYPE_DURATION, rawValue: "15m", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := formatKVRequestBody(tt.kvType, tt.rawValue)
			if tt.wantError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if body != tt.wantBody {
				t.Fatalf("expected body %q, got %q", tt.wantBody, body)
			}
		})
	}
}

func TestParseKVTypedValueBody(t *testing.T) {
	typed := parseKVTypedValueBody([]byte(`{"type":"STRING","value":"hello","revision":1}`))
	if typed == nil {
		t.Fatal("expected typed value, got nil")
	}
	if typed.Type != "STRING" {
		t.Fatalf("expected STRING, got %s", typed.Type)
	}
	if typed.Value != "hello" {
		t.Fatalf("expected hello, got %v", typed.Value)
	}
}

func TestParseKVTypedValueBody_JSONObject(t *testing.T) {
	typed := parseKVTypedValueBody([]byte(`{"type":"JSON","value":{"feature":true,"count":2}}`))
	if typed == nil {
		t.Fatal("expected typed value, got nil")
	}
	if typed.Type != "JSON" {
		t.Fatalf("expected JSON, got %s", typed.Type)
	}

	valueMap, ok := typed.Value.(map[string]any)
	if !ok {
		t.Fatalf("expected map value, got %T", typed.Value)
	}
	if valueMap["feature"] != true {
		t.Fatalf("expected feature=true, got %v", valueMap["feature"])
	}
	if got := toPrettyString(valueMap["count"]); got != "2" {
		t.Fatalf("expected count=2, got %s", got)
	}
}

func TestParseKVTypedValueBody_RejectsProblemDocumentAndEmptyBody(t *testing.T) {
	if got := parseKVTypedValueBody(nil); got != nil {
		t.Fatalf("expected nil for an empty body, got %#v", got)
	}
	problem := []byte(`{"type":"https://kestra.io/docs/api-reference/problems/not-found","title":"Resource not found","status":404,"detail":"No value found"}`)
	if got := parseKVTypedValueBody(problem); got != nil {
		t.Fatalf("expected a problem document to be rejected, got %#v", got)
	}
}

func TestTryParseKVTypedValueFromError(t *testing.T) {
	payload := map[string]any{"type": "JSON", "value": map[string]any{"a": 1.0}}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	sdkErr := &kestra.GenericOpenAPIError{}
	setGenericOpenAPIErrorBody(sdkErr, body)

	typed := tryParseKVTypedValueFromError(sdkErr)
	if typed == nil {
		t.Fatal("expected parsed typed value, got nil")
	}
	if typed.Type != "JSON" {
		t.Fatalf("expected JSON type, got %s", typed.Type)
	}

	valueMap, ok := typed.Value.(map[string]any)
	if !ok {
		t.Fatalf("expected map value, got %T", typed.Value)
	}
	if got := toPrettyString(valueMap["a"]); got != "1" {
		t.Fatalf("expected parsed value 1, got %s", got)
	}
}

func TestKVDeleteAllCommand_NoArgs(t *testing.T) {
	cmd := newKVDeleteAllCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "requires at least 2 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestKVDeleteAllCommand_OnlyNamespace(t *testing.T) {
	cmd := newKVDeleteAllCommand()
	_, err := executeCommand(cmd, "my.namespace")
	if err == nil {
		t.Fatal("expected error when only namespace provided")
	}
	if !strings.Contains(err.Error(), "requires at least 2 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestKVDeleteAllCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newKVDeleteAllCommand()
	_, err := executeCommand(cmd, "my.namespace", "key1", "key2")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestKVSetCommand_HasTTLFlag(t *testing.T) {
	cmd := newKVSetCommand()
	if cmd.Flags().Lookup("ttl") == nil {
		t.Fatal("expected --ttl flag on kv set")
	}
}

func TestKVUpdateCommand_HasTTLFlag(t *testing.T) {
	cmd := newKVUpdateCommand()
	if cmd.Flags().Lookup("ttl") == nil {
		t.Fatal("expected --ttl flag on kv update")
	}
}

func TestKVWriteCommand_TTLPassedToAPI(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runKVWrite(newTestClient(t, server.URL), "ns", "mykey", "hello", "STRING", "PT1H", false, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runKVWrite error: %v", err)
	}
	if !strings.Contains(gotQuery, "ttl=PT1H") {
		t.Errorf("expected ttl query param, got %q", gotQuery)
	}
	if !strings.Contains(buf.String(), "TTL: PT1H") {
		t.Errorf("expected TTL in output, got:\n%s", buf.String())
	}
}

func TestKVWriteCommand_NoTTL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "ttl") {
			http.Error(w, "unexpected ttl param", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runKVWrite(newTestClient(t, server.URL), "ns", "mykey", "42", "NUMBER", "", false, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runKVWrite error: %v", err)
	}
	if strings.Contains(buf.String(), "TTL:") {
		t.Errorf("expected no TTL in output when not set, got:\n%s", buf.String())
	}
}

func TestKVListInheritedCommand_NoArgs(t *testing.T) {
	cmd := newKVListInheritedCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestKVListInheritedCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newKVListInheritedCommand()
	_, err := executeCommand(cmd, "my.namespace")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestTryParseKVTypedValueFromError_RejectsProblemDocument(t *testing.T) {
	// Kestra 2.0 answers a missing key with an RFC 7807 problem document. Its
	// "type" is a problem-category URL, not a KV value type: reading it as one
	// turned a 404 into a rendered record and a zero exit code.
	body := []byte(`{"type":"https://kestra.io/docs/api-reference/problems/not-found","title":"Resource not found","status":404,"detail":"No value found for key 'k' in namespace 'system'"}`)
	err := &kestra.GenericOpenAPIError{}
	setGenericOpenAPIErrorBody(err, body)

	if got := tryParseKVTypedValueFromError(err); got != nil {
		t.Fatalf("expected a problem document to be rejected, got %#v", got)
	}
}

func TestTryParseKVTypedValueFromError_StillRecoversRealValue(t *testing.T) {
	body := []byte(`{"type":"STRING","value":"hello"}`)
	err := &kestra.GenericOpenAPIError{}
	setGenericOpenAPIErrorBody(err, body)

	got := tryParseKVTypedValueFromError(err)
	if got == nil || got.Type != "STRING" || got.Value != "hello" {
		t.Fatalf("expected the KV payload to still be recovered, got %#v", got)
	}
}

// --- #121: integer precision above 2^53 ---------------------------------

const (
	bigNumber   = "9007199254740993"
	bigNanosObj = `{"nanos":1725450000123456789}`
)

// TestFormatKVRequestBody_PreservesLargeIntegers pins the write path: the body
// PUT to Kestra must carry the digits the user typed, not a float64 round-trip.
func TestFormatKVRequestBody_PreservesLargeIntegers(t *testing.T) {
	tests := []struct {
		name     string
		kvType   kestra.KVType
		rawValue string
		wantBody string
	}{
		{name: "number above 2^53", kvType: kestra.KVTYPE_NUMBER, rawValue: bigNumber, wantBody: bigNumber},
		{name: "number with surrounding space", kvType: kestra.KVTYPE_NUMBER, rawValue: " " + bigNumber + " ", wantBody: bigNumber},
		{name: "json with epoch nanos", kvType: kestra.KVTYPE_JSON, rawValue: bigNanosObj, wantBody: bigNanosObj},
		{name: "json array of big ints", kvType: kestra.KVTYPE_JSON, rawValue: `[9007199254740993,9007199254740995]`, wantBody: `[9007199254740993,9007199254740995]`},
		{name: "json nested big int", kvType: kestra.KVTYPE_JSON, rawValue: `{"a":{"b":[1725450000123456789]}}`, wantBody: `{"a":{"b":[1725450000123456789]}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := formatKVRequestBody(tt.kvType, tt.rawValue)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if body != tt.wantBody {
				t.Fatalf("expected body %q, got %q", tt.wantBody, body)
			}
		})
	}
}

// TestFormatKVRequestBody_JSONIsCompacted keeps the pre-#121 behaviour of
// normalising whitespace, which the old Unmarshal/Marshal round-trip provided.
func TestFormatKVRequestBody_JSONIsCompacted(t *testing.T) {
	body, err := formatKVRequestBody(kestra.KVTYPE_JSON, "{ \"a\" : 1,\n\"b\": [ 2, 3 ] }")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body != `{"a":1,"b":[2,3]}` {
		t.Fatalf("expected compacted JSON, got %q", body)
	}
}

func TestFormatKVRequestBody_RejectsNonNumericNumbers(t *testing.T) {
	for _, raw := range []string{"abc", "true", `"5"`, "null", "[1]", "{}", "1 2", ""} {
		if body, err := formatKVRequestBody(kestra.KVTYPE_NUMBER, raw); err == nil {
			t.Fatalf("expected NUMBER %q to be rejected, got body %q", raw, body)
		}
	}
}

func TestFormatKVRequestBody_RejectsTrailingJSONGarbage(t *testing.T) {
	if body, err := formatKVRequestBody(kestra.KVTYPE_JSON, `{"a":1} {"b":2}`); err == nil {
		t.Fatalf("expected trailing garbage to be rejected, got body %q", body)
	}
}

// TestParseKVDisplayValue_PreservesLargeIntegers covers the echo printed by
// kv set / kv update, asserting on rendered digits rather than on Go types.
func TestParseKVDisplayValue_PreservesLargeIntegers(t *testing.T) {
	tests := []struct {
		name     string
		kvType   kestra.KVType
		rawValue string
		want     string
	}{
		{name: "number", kvType: kestra.KVTYPE_NUMBER, rawValue: bigNumber, want: bigNumber},
		{name: "small number", kvType: kestra.KVTYPE_NUMBER, rawValue: "42", want: "42"},
		{name: "float number", kvType: kestra.KVTYPE_NUMBER, rawValue: "1.5", want: "1.5"},
		{name: "boolean", kvType: kestra.KVTYPE_BOOLEAN, rawValue: "true", want: "true"},
		{name: "string", kvType: kestra.KVTYPE_STRING, rawValue: "hello", want: "hello"},
		{name: "date", kvType: kestra.KVTYPE_DATE, rawValue: "2025-10-13", want: "2025-10-13"},
		{name: "datetime", kvType: kestra.KVTYPE_DATETIME, rawValue: "2025-10-14T18:02:08Z", want: "2025-10-14T18:02:08Z"},
		{name: "duration", kvType: kestra.KVTYPE_DURATION, rawValue: "PT15M", want: "PT15M"},
		{name: "json nanos", kvType: kestra.KVTYPE_JSON, rawValue: bigNanosObj, want: "1725450000123456789"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toPrettyString(parseKVDisplayValue(tt.kvType, tt.rawValue))
			if !strings.Contains(got, tt.want) {
				t.Fatalf("expected rendered value to contain %q, got %q", tt.want, got)
			}
		})
	}
}

func TestTryParseKVTypedValueFromError_PreservesLargeIntegers(t *testing.T) {
	err := &kestra.GenericOpenAPIError{}
	setGenericOpenAPIErrorBody(err, []byte(`{"type":"NUMBER","value":`+bigNumber+`,"revision":1}`))

	got := tryParseKVTypedValueFromError(err)
	if got == nil {
		t.Fatal("expected the KV payload to be recovered")
	}
	if rendered := toPrettyString(got.Value); rendered != bigNumber {
		t.Fatalf("expected %s, got %s", bigNumber, rendered)
	}
}

// kvGetServer answers the KV detail endpoint with a canned raw body.
func kvGetServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/namespaces/ns/kv/k") {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

func TestKVGet_PreservesLargeIntegers(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "number", body: `{"type":"NUMBER","value":` + bigNumber + `,"revision":1}`, want: bigNumber},
		{name: "json object", body: `{"type":"JSON","value":` + bigNanosObj + `,"revision":1}`, want: "1725450000123456789"},
		{name: "json array", body: `{"type":"JSON","value":[9007199254740993],"revision":1}`, want: "9007199254740993"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := kvGetServer(t, tt.body)

			var table bytes.Buffer
			if err := runKVGet(newTestClient(t, server.URL), "ns", "k", newTableRenderer(&table)); err != nil {
				t.Fatalf("runKVGet error: %v", err)
			}
			if !strings.Contains(table.String(), tt.want) {
				t.Fatalf("table output lost precision, want %s in:\n%s", tt.want, table.String())
			}

			var asJSON bytes.Buffer
			if err := runKVGet(newTestClient(t, server.URL), "ns", "k", newJSONRenderer(&asJSON)); err != nil {
				t.Fatalf("runKVGet error: %v", err)
			}
			if !strings.Contains(asJSON.String(), tt.want) {
				t.Fatalf("json output lost precision, want %s in:\n%s", tt.want, asJSON.String())
			}
			// The value must stay a bare JSON number, never a quoted string.
			if strings.Contains(asJSON.String(), `"`+tt.want+`"`) {
				t.Fatalf("expected a bare JSON number, got:\n%s", asJSON.String())
			}
		})
	}
}

// TestKVGet_OtherTypes keeps the non-numeric types rendering as before.
func TestKVGet_OtherTypes(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "string", body: `{"type":"STRING","value":"hello"}`, want: "Value: hello"},
		{name: "boolean", body: `{"type":"BOOLEAN","value":true}`, want: "Value: true"},
		{name: "datetime", body: `{"type":"DATETIME","value":"2025-10-14T18:02:08Z"}`, want: "Value: 2025-10-14T18:02:08Z"},
		{name: "duration", body: `{"type":"DURATION","value":"PT15M"}`, want: "Value: PT15M"},
		{name: "nested json", body: `{"type":"JSON","value":{"a":{"b":true}}}`, want: `"b": true`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := kvGetServer(t, tt.body)
			var buf bytes.Buffer
			if err := runKVGet(newTestClient(t, server.URL), "ns", "k", newTableRenderer(&buf)); err != nil {
				t.Fatalf("runKVGet error: %v", err)
			}
			if !strings.Contains(buf.String(), tt.want) {
				t.Fatalf("expected %q in:\n%s", tt.want, buf.String())
			}
		})
	}
}

func TestKVGet_NotFoundStillErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"type":"https://kestra.io/docs/api-reference/problems/not-found","title":"Resource not found","status":404,"detail":"No value found for key 'k' in namespace 'ns'"}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runKVGet(newTestClient(t, server.URL), "ns", "k", newTableRenderer(&buf))
	if err == nil {
		t.Fatal("expected a not-found error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected a not-found message, got: %v", err)
	}
}

// TestKVWrite_SendsExactDigits is the data-loss half of #121: the value stored
// server-side must be the digits the user typed.
func TestKVWrite_SendsExactDigits(t *testing.T) {
	tests := []struct {
		name     string
		kvType   string
		rawValue string
		wantBody string
	}{
		{name: "number", kvType: "NUMBER", rawValue: bigNumber, wantBody: bigNumber},
		{name: "json", kvType: "JSON", rawValue: bigNanosObj, wantBody: bigNanosObj},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotBody string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw, _ := io.ReadAll(r.Body)
				gotBody = string(raw)
				w.WriteHeader(http.StatusNoContent)
			}))
			t.Cleanup(server.Close)

			var buf bytes.Buffer
			if err := runKVWrite(newTestClient(t, server.URL), "ns", "k", tt.rawValue, tt.kvType, "", false, newTableRenderer(&buf)); err != nil {
				t.Fatalf("runKVWrite error: %v", err)
			}
			if gotBody != tt.wantBody {
				t.Fatalf("expected request body %q, got %q", tt.wantBody, gotBody)
			}
			for _, digits := range strings.FieldsFunc(tt.wantBody, func(r rune) bool { return r < '0' || r > '9' }) {
				if !strings.Contains(buf.String(), digits) {
					t.Fatalf("expected the echo to show exact digits %s, got:\n%s", digits, buf.String())
				}
			}
		})
	}
}
