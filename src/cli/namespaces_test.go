package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestNamespacesListCommand_Help(t *testing.T) {
	cmd := newNamespacesListCommand()
	output, err := executeCommand(cmd, "--help")
	if err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}

	if !strings.Contains(output, "List all namespaces") {
		t.Fatalf("expected help text, got: %s", output)
	}
	if !strings.Contains(output, "--query") {
		t.Fatalf("expected query flag in help, got: %s", output)
	}
}

func TestNamespacesListCommand_QueryFlag(t *testing.T) {
	cmd := newNamespacesListCommand()

	// Verify the flag exists and has the right short form
	queryFlag := cmd.Flags().Lookup("query")
	if queryFlag == nil {
		t.Fatal("expected --query flag")
	}
	if queryFlag.Shorthand != "q" {
		t.Fatalf("expected -q shorthand, got %s", queryFlag.Shorthand)
	}
}

func TestNamespacesCommand_Structure(t *testing.T) {
	cmd := newNamespacesCommand()

	if cmd.Use != "namespaces" {
		t.Fatalf("expected use 'namespaces', got '%s'", cmd.Use)
	}

	// Should have at least the list subcommand
	subcommands := cmd.Commands()
	if len(subcommands) == 0 {
		t.Fatal("expected subcommands")
	}

	var hasListCmd bool
	for _, sub := range subcommands {
		if sub.Use == "list" {
			hasListCmd = true
			break
		}
	}
	if !hasListCmd {
		t.Fatal("expected 'list' subcommand")
	}
}

func TestNamespacesGetCommand_NoArgs(t *testing.T) {
	cmd := newNamespacesGetCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestNamespacesGetCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newNamespacesGetCommand()
	_, err := executeCommand(cmd, "my.namespace")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestNamespacesCreateCommand_NoArgs(t *testing.T) {
	cmd := newNamespacesCreateCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestNamespacesCreateCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newNamespacesCreateCommand()
	_, err := executeCommand(cmd, "my.namespace")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestNamespacesDeleteCommand_NoArgs(t *testing.T) {
	cmd := newNamespacesDeleteCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestNamespacesDeleteCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newNamespacesDeleteCommand()
	_, err := executeCommand(cmd, "my.namespace")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestNamespacesUpdateCommand_NoArgs(t *testing.T) {
	cmd := newNamespacesUpdateCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestNamespacesUpdateCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newNamespacesUpdateCommand()
	_, err := executeCommand(cmd, "my.namespace")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestNamespacesInheritedSecretsCommand_NoArgs(t *testing.T) {
	cmd := newNamespacesInheritedSecretsCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestNamespacesInheritedSecretsCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newNamespacesInheritedSecretsCommand()
	_, err := executeCommand(cmd, "my.namespace")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestNamespacesInheritedVariablesCommand_NoArgs(t *testing.T) {
	cmd := newNamespacesInheritedVariablesCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestNamespacesInheritedVariablesCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newNamespacesInheritedVariablesCommand()
	_, err := executeCommand(cmd, "my.namespace")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestNamespacesAutocompleteCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newNamespacesAutocompleteCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestNamespacesSearchCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newNamespacesSearchCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestParseVariableFlags_NoneProvided(t *testing.T) {
	variables, err := parseVariableFlags(nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if variables != nil {
		t.Fatalf("expected nil variables, got: %v", variables)
	}
}

func TestParseVariableFlags_Pairs(t *testing.T) {
	variables, err := parseVariableFlags([]string{"env=prod", "region=eu"}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if variables["env"] != "prod" || variables["region"] != "eu" {
		t.Fatalf("unexpected variables: %v", variables)
	}
}

func TestParseVariableFlags_InvalidPair(t *testing.T) {
	_, err := parseVariableFlags([]string{"invalid"}, "")
	if err == nil {
		t.Fatal("expected error for invalid key=value pair")
	}
	if !strings.Contains(err.Error(), "expected format key=value") {
		t.Fatalf("expected format error, got: %v", err)
	}
}

func TestParseVariableFlags_File(t *testing.T) {
	f, err := os.CreateTemp("", "variables-*.yml")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("env: staging\nnested:\n  key: value\n")
	f.Close()
	defer os.Remove(f.Name())

	variables, err := parseVariableFlags(nil, f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if variables["env"] != "staging" {
		t.Fatalf("unexpected variables: %v", variables)
	}
	nested, ok := variables["nested"].(map[string]interface{})
	if !ok || nested["key"] != "value" {
		t.Fatalf("expected nested map, got: %v", variables["nested"])
	}
}

func TestParseVariableFlags_PairOverridesFile(t *testing.T) {
	f, err := os.CreateTemp("", "variables-*.yml")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("env: staging\n")
	f.Close()
	defer os.Remove(f.Name())

	variables, err := parseVariableFlags([]string{"env=prod"}, f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if variables["env"] != "prod" {
		t.Fatalf("expected --variable to override file value, got: %v", variables)
	}
}

func TestNamespacesUpdateCommand_InvalidVariable(t *testing.T) {
	cmd := newNamespacesUpdateCommand()
	_, err := executeCommand(cmd, "my.namespace", "--variable", "invalid")
	if err == nil {
		t.Fatal("expected error for invalid --variable")
	}
	if !strings.Contains(err.Error(), "expected format key=value") {
		t.Fatalf("expected format error, got: %v", err)
	}
}

func TestRunNamespacesUpdate_SetsVariables(t *testing.T) {
	var gotBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"my.namespace","deleted":false,"variables":{"env":"prod"}}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runNamespacesUpdate(newTestClient(t, server.URL), "my.namespace", "", false, map[string]interface{}{"env": "prod"}, nil, nil, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runNamespacesUpdate error: %v", err)
	}

	vars, ok := gotBody["variables"].(map[string]interface{})
	if !ok || vars["env"] != "prod" {
		t.Fatalf("expected variables in request body, got: %v", gotBody)
	}
}

func TestRunNamespacesUpdate_DescriptionOnlyPreservesVariables(t *testing.T) {
	var gotBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"id":"my.namespace","deleted":false,"description":"old","variables":{"env":"prod","region":"eu"}}`))
		case http.MethodPut:
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_, _ = w.Write([]byte(`{"id":"my.namespace","deleted":false,"description":"touched","variables":{"env":"prod","region":"eu"}}`))
		}
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runNamespacesUpdate(newTestClient(t, server.URL), "my.namespace", "touched", true, nil, nil, nil, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runNamespacesUpdate error: %v", err)
	}

	if gotBody["description"] != "touched" {
		t.Fatalf("expected description to be updated, got: %v", gotBody)
	}
	vars, ok := gotBody["variables"].(map[string]interface{})
	if !ok || vars["env"] != "prod" || vars["region"] != "eu" {
		t.Fatalf("expected pre-existing variables to be preserved, got: %v", gotBody)
	}
}

func TestRunNamespacesUpdate_VariablesOnlyPreservesDescription(t *testing.T) {
	var gotBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"id":"my.namespace","deleted":false,"description":"keep me","variables":{"env":"prod"}}`))
		case http.MethodPut:
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_, _ = w.Write([]byte(`{"id":"my.namespace","deleted":false,"description":"keep me","variables":{"env":"staging"}}`))
		}
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runNamespacesUpdate(newTestClient(t, server.URL), "my.namespace", "", false, map[string]interface{}{"env": "staging"}, nil, nil, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runNamespacesUpdate error: %v", err)
	}

	if gotBody["description"] != "keep me" {
		t.Fatalf("expected pre-existing description to be preserved, got: %v", gotBody)
	}
	vars, ok := gotBody["variables"].(map[string]interface{})
	if !ok || vars["env"] != "staging" {
		t.Fatalf("expected variables to be updated, got: %v", gotBody)
	}
}

func TestRunNamespacesUpdate_GetError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runNamespacesUpdate(newTestClient(t, server.URL), "my.namespace", "new desc", true, nil, nil, nil, newTableRenderer(&buf))
	if err == nil {
		t.Fatal("expected error when fetching the current namespace fails")
	}
}

func TestRunNamespacesCreate_SetsVariables(t *testing.T) {
	var gotBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"my.namespace","deleted":false,"variables":{"env":"prod"}}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runNamespacesCreate(newTestClient(t, server.URL), "my.namespace", "", map[string]interface{}{"env": "prod"}, nil, nil, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runNamespacesCreate error: %v", err)
	}

	vars, ok := gotBody["variables"].(map[string]interface{})
	if !ok || vars["env"] != "prod" {
		t.Fatalf("expected variables in request body, got: %v", gotBody)
	}
}

func TestRunNamespacesGet_ShowsVariables(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"my.namespace","deleted":false,"variables":{"env":"prod"}}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runNamespacesGet(newTestClient(t, server.URL), "my.namespace", newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runNamespacesGet error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "VARIABLES") || !strings.Contains(out, "env") || !strings.Contains(out, "prod") {
		t.Fatalf("expected variables in output, got:\n%s", out)
	}
}

func TestRunNamespacesCreate_SendsConcurrencyAndQuotas(t *testing.T) {
	var gotBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"my.namespace","deleted":false,"concurrency":{"limit":10,"behavior":"QUEUE"},"quotas":[{"duration":"PT1H","limit":100,"behavior":"FAIL"}]}`))
	}))
	t.Cleanup(server.Close)

	concurrency, err := parseConcurrencyFlags(10, true, "QUEUE", true)
	if err != nil {
		t.Fatal(err)
	}
	quotas, err := parseQuotaFlags([]string{"duration=PT1H,limit=100,behavior=FAIL"})
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	err = runNamespacesCreate(newTestClient(t, server.URL), "my.namespace", "", nil, concurrency, quotas, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runNamespacesCreate error: %v", err)
	}

	sentConcurrency, ok := gotBody["concurrency"].(map[string]interface{})
	if !ok || sentConcurrency["limit"] != float64(10) || sentConcurrency["behavior"] != "QUEUE" {
		t.Fatalf("expected concurrency in request body, got: %v", gotBody)
	}
	sentQuotas, ok := gotBody["quotas"].([]interface{})
	if !ok || len(sentQuotas) != 1 {
		t.Fatalf("expected quotas in request body, got: %v", gotBody)
	}
	out := buf.String()
	if !strings.Contains(out, "CONCURRENCY") || !strings.Contains(out, "QUOTAS") {
		t.Fatalf("expected concurrency and quotas echoed in create output, got:\n%s", out)
	}
}

func TestRunNamespacesUpdate_ConcurrencyOnlyPreservesVariables(t *testing.T) {
	var gotBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"id":"my.namespace","deleted":false,"variables":{"env":"prod"},"quotas":[{"duration":"PT1H","limit":100,"behavior":"FAIL"}]}`))
		case http.MethodPut:
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_, _ = w.Write([]byte(`{"id":"my.namespace","deleted":false,"variables":{"env":"prod"}}`))
		}
	}))
	t.Cleanup(server.Close)

	concurrency, err := parseConcurrencyFlags(20, true, "FAIL", true)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	err = runNamespacesUpdate(newTestClient(t, server.URL), "my.namespace", "", false, nil, concurrency, nil, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runNamespacesUpdate error: %v", err)
	}

	sentConcurrency, ok := gotBody["concurrency"].(map[string]interface{})
	if !ok || sentConcurrency["limit"] != float64(20) || sentConcurrency["behavior"] != "FAIL" {
		t.Fatalf("expected concurrency in request body, got: %v", gotBody)
	}
	vars, ok := gotBody["variables"].(map[string]interface{})
	if !ok || vars["env"] != "prod" {
		t.Fatalf("expected pre-existing variables to be preserved, got: %v", gotBody)
	}
	sentQuotas, ok := gotBody["quotas"].([]interface{})
	if !ok || len(sentQuotas) != 1 {
		t.Fatalf("expected pre-existing quotas to be preserved, got: %v", gotBody)
	}
}

func TestNamespacesCreateCommand_InvalidQuota(t *testing.T) {
	cmd := newNamespacesCreateCommand()
	_, err := executeCommand(cmd, "my.namespace", "--quota", "duration=PT1H")
	if err == nil {
		t.Fatal("expected error for incomplete --quota")
	}
	if !strings.Contains(err.Error(), "duration, limit and behavior are all required") {
		t.Fatalf("expected missing-key error, got: %v", err)
	}
}

// --- large integers in namespace variables (follow-up to #121) -----------

const nsBigVariablesBody = `{"id":"ns","deleted":false,"variables":{"bigint":1725450000123456789,"nested":{"nanos":9007199254740993},"small":42}}`

// namespaceServer answers both the SDK's namespace call and the raw read with
// the same body, so a test exercises whichever path the code takes.
func namespaceServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

func TestNamespacesGet_PreservesLargeIntegers(t *testing.T) {
	server := namespaceServer(t, nsBigVariablesBody)

	var asJSON bytes.Buffer
	if err := runNamespacesGet(newTestClient(t, server.URL), "ns", newJSONRenderer(&asJSON)); err != nil {
		t.Fatalf("runNamespacesGet error: %v", err)
	}
	for _, digits := range []string{"1725450000123456789", "9007199254740993"} {
		if !strings.Contains(asJSON.String(), digits) {
			t.Fatalf("json output lost precision, want %s in:\n%s", digits, asJSON.String())
		}
		if strings.Contains(asJSON.String(), `"`+digits+`"`) {
			t.Fatalf("expected a bare JSON number, got:\n%s", asJSON.String())
		}
	}

	var table bytes.Buffer
	if err := runNamespacesGet(newTestClient(t, server.URL), "ns", newTableRenderer(&table)); err != nil {
		t.Fatalf("runNamespacesGet error: %v", err)
	}
	// The table used to print float64s through %v: "1.7254500001234568e+18"
	// for a scalar and "map[nanos:9.007199254740992e+15]" for a nested object.
	if !strings.Contains(table.String(), "1725450000123456789") {
		t.Fatalf("table output lost precision:\n%s", table.String())
	}
	if strings.Contains(table.String(), "e+18") || strings.Contains(table.String(), "map[") {
		t.Fatalf("expected JSON rendering rather than %%v, got:\n%s", table.String())
	}
	if !strings.Contains(table.String(), "9007199254740993") {
		t.Fatalf("nested table value lost precision:\n%s", table.String())
	}
}

func TestNamespacesCreate_EchoPreservesLargeIntegers(t *testing.T) {
	server := namespaceServer(t, nsBigVariablesBody)

	variables := map[string]any{"bigint": int64(1725450000123456789)}
	var buf bytes.Buffer
	if err := runNamespacesCreate(newTestClient(t, server.URL), "ns", "", variables, nil, nil, newJSONRenderer(&buf)); err != nil {
		t.Fatalf("runNamespacesCreate error: %v", err)
	}
	if !strings.Contains(buf.String(), "1725450000123456789") {
		t.Fatalf("create echo lost precision:\n%s", buf.String())
	}
}

func TestNamespacesUpdate_EchoPreservesLargeIntegers(t *testing.T) {
	server := namespaceServer(t, nsBigVariablesBody)

	variables := map[string]any{"bigint": int64(1725450000123456789)}
	var buf bytes.Buffer
	if err := runNamespacesUpdate(newTestClient(t, server.URL), "ns", "", false, variables, nil, nil, newJSONRenderer(&buf)); err != nil {
		t.Fatalf("runNamespacesUpdate error: %v", err)
	}
	if !strings.Contains(buf.String(), "1725450000123456789") {
		t.Fatalf("update echo lost precision:\n%s", buf.String())
	}
}

// TestNamespacesGet_RawReadFailureFallsBack keeps the command working when the
// extra raw read fails: the SDK's (lossy) variables are better than an error.
func TestNamespacesGet_RawReadFailureFallsBack(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls > 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"ns","deleted":false,"variables":{"env":"prod"}}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	if err := runNamespacesGet(newTestClient(t, server.URL), "ns", newJSONRenderer(&buf)); err != nil {
		t.Fatalf("runNamespacesGet error: %v", err)
	}
	if !strings.Contains(buf.String(), "prod") {
		t.Fatalf("expected the SDK variables as a fallback, got:\n%s", buf.String())
	}
}

// TestNamespacesGet_NoVariablesSkipsRawRead keeps the common case at a single
// request: an empty variables map has no precision to preserve.
func TestNamespacesGet_NoVariablesSkipsRawRead(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"ns","deleted":false}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	if err := runNamespacesGet(newTestClient(t, server.URL), "ns", newJSONRenderer(&buf)); err != nil {
		t.Fatalf("runNamespacesGet error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected exactly one request, got %d", calls)
	}
}

// TestNamespacesGet_VariablesTriggerRawRead is the counterpart: with variables
// present, the raw read must happen so their digits survive.
func TestNamespacesGet_VariablesTriggerRawRead(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(nsBigVariablesBody))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	if err := runNamespacesGet(newTestClient(t, server.URL), "ns", newJSONRenderer(&buf)); err != nil {
		t.Fatalf("runNamespacesGet error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected the raw read to happen, got %d request(s)", calls)
	}
	if !strings.Contains(buf.String(), "1725450000123456789") {
		t.Fatalf("expected exact digits:\n%s", buf.String())
	}
}
