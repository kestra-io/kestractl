package cli

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestTriggersListCommand_Help(t *testing.T) {
	cmd := newTriggersListCommand()
	output, err := executeCommand(cmd, "--help")
	if err != nil {
		t.Fatalf("executeCommand returned error: %v", err)
	}
	if !strings.Contains(output, "List triggers") {
		t.Fatalf("expected help text, got: %s", output)
	}
}

func TestTriggersListCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newTriggersListCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestTriggersDeleteCommand_NoArgs(t *testing.T) {
	cmd := newTriggersDeleteCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 3 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestTriggersDeleteCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newTriggersDeleteCommand()
	_, err := executeCommand(cmd, "my.ns", "my-flow", "my-trigger")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestTriggersUnlockCommand_NoArgs(t *testing.T) {
	cmd := newTriggersUnlockCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 3 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestTriggersUnlockCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newTriggersUnlockCommand()
	_, err := executeCommand(cmd, "my.ns", "my-flow", "my-trigger")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestTriggersRestartCommand_NoArgs(t *testing.T) {
	cmd := newTriggersRestartCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 3 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestTriggersRestartCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newTriggersRestartCommand()
	_, err := executeCommand(cmd, "my.ns", "my-flow", "my-trigger")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestTriggersUpdateCommand_NoArgs(t *testing.T) {
	cmd := newTriggersUpdateCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 3 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestTriggersUpdateCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newTriggersUpdateCommand()
	_, err := executeCommand(cmd, "my.ns", "my-flow", "my-trigger")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestTriggersBackfillPauseCommand_NoArgs(t *testing.T) {
	cmd := newTriggersBackfillPauseCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 3 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestTriggersBackfillPauseCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newTriggersBackfillPauseCommand()
	_, err := executeCommand(cmd, "my.ns", "my-flow", "my-trigger")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestTriggersBackfillUnpauseCommand_NoArgs(t *testing.T) {
	cmd := newTriggersBackfillUnpauseCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 3 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestTriggersBackfillDeleteCommand_NoArgs(t *testing.T) {
	cmd := newTriggersBackfillDeleteCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 3 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestTriggersSearchForFlowCommand_NoArgs(t *testing.T) {
	cmd := newTriggersSearchForFlowCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 2 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestTriggersSearchForFlowCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newTriggersSearchForFlowCommand()
	_, err := executeCommand(cmd, "my.namespace", "my-flow")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestTriggersCommand_Structure(t *testing.T) {
	cmd := newTriggersCommand()
	if cmd.Use != "triggers" {
		t.Fatalf("expected use 'triggers', got '%s'", cmd.Use)
	}
	var hasListCmd bool
	for _, sub := range cmd.Commands() {
		if sub.Use == "list" {
			hasListCmd = true
			break
		}
	}
	if !hasListCmd {
		t.Fatal("expected 'list' subcommand")
	}
}

func TestParseTriggerIds_Valid(t *testing.T) {
	triggers, err := parseTriggerIds([]string{"my.ns/my-flow/my-trigger"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(triggers) != 1 {
		t.Fatalf("expected 1 trigger, got %d", len(triggers))
	}
	if triggers[0].GetNamespace() != "my.ns" {
		t.Errorf("expected namespace my.ns, got %s", triggers[0].GetNamespace())
	}
}

func TestParseTriggerIds_Invalid(t *testing.T) {
	_, err := parseTriggerIds([]string{"bad-format"})
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
}

func TestTriggersDeleteByIdsCommand_NoArgs(t *testing.T) {
	cmd := newTriggersDeleteByIdsCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
}

func TestTriggersDeleteByIdsCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newTriggersDeleteByIdsCommand()
	_, err := executeCommand(cmd, "my.ns/my-flow/my-trigger")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestTriggersUnlockByIdsCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newTriggersUnlockByIdsCommand()
	_, err := executeCommand(cmd, "my.ns/my-flow/my-trigger")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestTriggersDisableByIdsCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newTriggersDisableByIdsCommand()
	_, err := executeCommand(cmd, "my.ns/my-flow/my-trigger")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestTriggersEnableByIdsCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newTriggersEnableByIdsCommand()
	_, err := executeCommand(cmd, "my.ns/my-flow/my-trigger")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestTriggersDisableByQueryCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newTriggersDisableByQueryCommand()
	_, err := executeCommand(cmd, "--namespace", "qa.test")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestTriggersDeleteByQueryCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newTriggersDeleteByQueryCommand()
	_, err := executeCommand(cmd, "--namespace", "qa.test")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestTriggersDisableCommand_NoArgs(t *testing.T) {
	cmd := newTriggersDisableCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 3 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestTriggersDisableCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newTriggersDisableCommand()
	_, err := executeCommand(cmd, "my.ns", "my-flow", "my-trigger")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestTriggersEnableCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newTriggersEnableCommand()
	_, err := executeCommand(cmd, "my.ns", "my-flow", "my-trigger")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestTriggersCreateBackfillCommand_MissingFlags(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	cmd := newTriggersCreateBackfillCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when required flags missing")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Fatalf("expected required-flags error, got: %v", err)
	}
}

func TestTriggersCreateBackfillCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newTriggersCreateBackfillCommand()
	_, err := executeCommand(cmd, "--namespace", "my.ns", "--flow-id", "my-flow", "--trigger-id", "my-trigger")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestTriggersPauseBackfillByIdsCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newTriggersPauseBackfillByIdsCommand()
	_, err := executeCommand(cmd, "my.ns/my-flow/my-trigger")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestTriggersUnpauseBackfillByIdsCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newTriggersUnpauseBackfillByIdsCommand()
	_, err := executeCommand(cmd, "my.ns/my-flow/my-trigger")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestTriggersDeleteBackfillByIdsCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newTriggersDeleteBackfillByIdsCommand()
	_, err := executeCommand(cmd, "my.ns/my-flow/my-trigger")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestTriggersPauseBackfillByQueryCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newTriggersPauseBackfillByQueryCommand()
	_, err := executeCommand(cmd, "--namespace", "my.ns")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestTriggersUnpauseBackfillByQueryCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newTriggersUnpauseBackfillByQueryCommand()
	_, err := executeCommand(cmd, "--namespace", "my.ns")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestTriggersDeleteBackfillByQueryCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newTriggersDeleteBackfillByQueryCommand()
	_, err := executeCommand(cmd, "--namespace", "my.ns")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestTriggersBackfillByQueryCommand_NoFilter(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	for _, tc := range []struct {
		name string
		cmd  *cobra.Command
	}{
		{"pause", newTriggersPauseBackfillByQueryCommand()},
		{"unpause", newTriggersUnpauseBackfillByQueryCommand()},
		{"delete", newTriggersDeleteBackfillByQueryCommand()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := executeCommand(tc.cmd)
			if err == nil {
				t.Fatal("expected error when no selection filter is given")
			}
			if !strings.Contains(err.Error(), "selection filter is required") {
				t.Fatalf("expected selection-filter error, got: %v", err)
			}
		})
	}
}

func TestRunTriggersBackfillBulkByQuery_SendsFilters(t *testing.T) {
	for _, tc := range []struct {
		op   string
		path string
	}{
		{"pause", "/api/v1/main/triggers/backfill/pause/by-query"},
		{"unpause", "/api/v1/main/triggers/backfill/unpause/by-query"},
		{"delete", "/api/v1/main/triggers/backfill/delete/by-query"},
	} {
		t.Run(tc.op, func(t *testing.T) {
			var gotPath, gotQuery string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				gotQuery = r.URL.RawQuery
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"operationId":"op-123","totalItems":4}`))
			}))
			defer server.Close()

			filters, err := parseQueryFilters("my.ns", nil)
			if err != nil {
				t.Fatalf("parseQueryFilters: %v", err)
			}

			var buf bytes.Buffer
			renderer := newTableRenderer(&buf)
			if err := runTriggersBackfillBulkByQuery(newTestClient(t, server.URL), tc.op, filters, renderer); err != nil {
				t.Fatalf("runTriggersBackfillBulkByQuery: %v", err)
			}

			if gotPath != tc.path {
				t.Fatalf("expected path %q, got %q", tc.path, gotPath)
			}
			// The namespace selection must reach the server as a filter param.
			if !strings.Contains(gotQuery, "filters%5Bnamespace%5D%5BEQUALS%5D=my.ns") {
				t.Fatalf("expected namespace filter in query, got %q", gotQuery)
			}
			if !strings.Contains(buf.String(), "4 trigger(s) scheduled") {
				t.Fatalf("expected summary in output, got: %s", buf.String())
			}
		})
	}
}

func TestRunTriggersExportCSV_Stdout(t *testing.T) {
	csvData := "namespace,flowId,triggerId,type\nprod,myflow,schedule,Schedule"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte(csvData))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runTriggersExportCSV(newTestClient(t, server.URL), "", &buf)
	if err != nil {
		t.Fatalf("runTriggersExportCSV error: %v", err)
	}
	if !strings.Contains(buf.String(), "namespace") {
		t.Errorf("expected CSV headers in output, got:\n%s", buf.String())
	}
}

func TestRunTriggersExportCSV_ToFile(t *testing.T) {
	csvData := "namespace,flowId,triggerId,type\nprod,myflow,schedule,Schedule"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte(csvData))
	}))
	t.Cleanup(server.Close)

	tmpFile, err := os.CreateTemp("", "triggers-*.csv")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	var buf bytes.Buffer
	err = runTriggersExportCSV(newTestClient(t, server.URL), tmpFile.Name(), &buf)
	if err != nil {
		t.Fatalf("runTriggersExportCSV error: %v", err)
	}
	if !strings.Contains(buf.String(), "exported") {
		t.Errorf("expected export message, got:\n%s", buf.String())
	}
	written, _ := os.ReadFile(tmpFile.Name())
	if !strings.Contains(string(written), "namespace") {
		t.Errorf("expected CSV content in file, got:\n%s", written)
	}
}

func TestTriggersExportCSVCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newTriggersExportCSVCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}
