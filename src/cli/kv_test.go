package cli

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	kestra "github.com/kestra-io/client-sdk/go-sdk/kestra_api_client"
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

func TestExtractKVTypedValue(t *testing.T) {
	resp := kestra.NewKVControllerKvDetail()
	resp.SetType(kestra.KVTYPE_STRING)
	resp.SetValue(map[string]any{"value": "hello"})

	typed := extractKVTypedValue(resp)
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

func TestExtractKVTypedValue_JSONObject(t *testing.T) {
	resp := kestra.NewKVControllerKvDetail()
	resp.SetType(kestra.KVTYPE_JSON)
	resp.SetValue(map[string]any{"feature": true, "count": 2})

	typed := extractKVTypedValue(resp)
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
	if valueMap["count"] != 2 {
		t.Fatalf("expected count=2, got %v", valueMap["count"])
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
	if valueMap["a"] != 1.0 {
		t.Fatalf("expected parsed value 1.0, got %v", valueMap["a"])
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
