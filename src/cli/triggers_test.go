package cli

import (
	"errors"
	"strings"
	"testing"
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
	_, err := executeCommand(cmd)
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
	_, err := executeCommand(cmd)
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
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}
