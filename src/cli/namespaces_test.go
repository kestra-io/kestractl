package cli

import (
	"errors"
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
