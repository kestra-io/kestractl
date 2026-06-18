package cli

import (
	"errors"
	"strings"
	"testing"
)

func TestServerLicenseCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newServerLicenseCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestServerCommand_Structure(t *testing.T) {
	cmd := newServerCommand()
	if cmd.Use != "server" {
		t.Fatalf("expected use 'server', got '%s'", cmd.Use)
	}
	var hasLicenseCmd bool
	for _, sub := range cmd.Commands() {
		if sub.Use == "license" {
			hasLicenseCmd = true
			break
		}
	}
	if !hasLicenseCmd {
		t.Fatal("expected 'license' subcommand")
	}
}
