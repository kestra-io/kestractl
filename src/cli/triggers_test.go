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
