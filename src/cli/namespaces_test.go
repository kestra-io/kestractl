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

func TestNamespacesInheritedPluginDefaultsCommand_NoArgs(t *testing.T) {
	cmd := newNamespacesInheritedPluginDefaultsCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestNamespacesInheritedPluginDefaultsCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newNamespacesInheritedPluginDefaultsCommand()
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

func TestNamespacesExportPluginDefaultsCommand_NoArgs(t *testing.T) {
	cmd := newNamespacesExportPluginDefaultsCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestNamespacesExportPluginDefaultsCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newNamespacesExportPluginDefaultsCommand()
	_, err := executeCommand(cmd, "my.namespace")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestRunNamespacesExportPluginDefaults_ToStdout(t *testing.T) {
	yamlContent := []byte("- type: io.kestra.plugin.core.log.Log\n  values: {}\n")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(yamlContent)
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	cmd := newNamespacesExportPluginDefaultsCommand()
	cmd.SetOut(&buf)

	err := runNamespacesExportPluginDefaults(newTestClient(t, server.URL), "my.namespace", "", cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "io.kestra.plugin.core.log.Log") {
		t.Errorf("expected YAML in output, got:\n%s", buf.String())
	}
}

func TestRunNamespacesExportPluginDefaults_ToFile(t *testing.T) {
	yamlContent := []byte("- type: io.kestra.plugin.core.log.Log\n")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(yamlContent)
	}))
	t.Cleanup(server.Close)

	f, err := os.CreateTemp("", "plugin-defaults-*.yml")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	defer os.Remove(f.Name())

	var buf bytes.Buffer
	cmd := newNamespacesExportPluginDefaultsCommand()
	cmd.SetOut(&buf)

	err = runNamespacesExportPluginDefaults(newTestClient(t, server.URL), "my.namespace", f.Name(), cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(yamlContent) {
		t.Errorf("file content mismatch: got %q, want %q", data, yamlContent)
	}
	if !strings.Contains(buf.String(), "exported") {
		t.Errorf("expected confirmation message, got:\n%s", buf.String())
	}
}

func TestNamespacesImportPluginDefaultsCommand_MissingFile(t *testing.T) {
	cmd := newNamespacesImportPluginDefaultsCommand()
	_, err := executeCommand(cmd, "my.namespace")
	if err == nil {
		t.Fatal("expected error when --file not provided")
	}
	if !strings.Contains(err.Error(), "--file") {
		t.Fatalf("expected --file error, got: %v", err)
	}
}

func TestNamespacesImportPluginDefaultsCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	f, err := os.CreateTemp("", "plugin-defaults-*.yml")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("- type: io.kestra.plugin.core.log.Log\n")
	f.Close()
	defer os.Remove(f.Name())

	cmd := newNamespacesImportPluginDefaultsCommand()
	_, err = executeCommand(cmd, "my.namespace", "--file", f.Name())
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
	err := runNamespacesUpdate(newTestClient(t, server.URL), "my.namespace", "", false, map[string]interface{}{"env": "prod"}, newTableRenderer(&buf))
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
	err := runNamespacesUpdate(newTestClient(t, server.URL), "my.namespace", "touched", true, nil, newTableRenderer(&buf))
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
	err := runNamespacesUpdate(newTestClient(t, server.URL), "my.namespace", "", false, map[string]interface{}{"env": "staging"}, newTableRenderer(&buf))
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
	err := runNamespacesUpdate(newTestClient(t, server.URL), "my.namespace", "new desc", true, nil, newTableRenderer(&buf))
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
	err := runNamespacesCreate(newTestClient(t, server.URL), "my.namespace", "", map[string]interface{}{"env": "prod"}, newTableRenderer(&buf))
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
