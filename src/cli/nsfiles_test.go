package cli

import (
	"strings"
	"testing"
)

func TestNamespaceFilesCommand_Structure(t *testing.T) {
	cmd := newNamespaceFilesCommand()
	if cmd.Use != "nsfiles" {
		t.Fatalf("expected Use to be 'nsfiles', got '%s'", cmd.Use)
	}

	subcommands := map[string]bool{}
	for _, sub := range cmd.Commands() {
		subcommands[sub.Name()] = true
	}

	if !subcommands["list"] {
		t.Fatal("expected 'list' subcommand to be registered")
	}
	if !subcommands["get"] {
		t.Fatal("expected 'get' subcommand to be registered")
	}
}

func TestNamespaceFilesListCommand_Flags(t *testing.T) {
	cmd := newNamespaceFilesListCommand()

	flags := []string{"path", "recursive"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Fatalf("expected flag --%s to exist", flag)
		}
	}
}

func TestNamespaceFilesGetCommand_Flags(t *testing.T) {
	cmd := newNamespaceFilesGetCommand()

	flags := []string{"path", "revision"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Fatalf("expected flag --%s to exist", flag)
		}
	}
}

func TestNamespaceFilesListCommand_Help(t *testing.T) {
	cmd := newNamespaceFilesListCommand()
	output, err := executeCommand(cmd, "--help")
	if err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}

	expected := []string{
		"list <namespace>",
		"--path",
		"--recursive",
	}
	for _, s := range expected {
		if !strings.Contains(output, s) {
			t.Fatalf("expected help to contain '%s', got: %s", s, output)
		}
	}
}

func TestNamespaceFilesGetCommand_Help(t *testing.T) {
	cmd := newNamespaceFilesGetCommand()
	output, err := executeCommand(cmd, "--help")
	if err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}

	expected := []string{
		"get <namespace>",
		"--path",
		"--revision",
	}
	for _, s := range expected {
		if !strings.Contains(output, s) {
			t.Fatalf("expected help to contain '%s', got: %s", s, output)
		}
	}
}
