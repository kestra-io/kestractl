package cli

import (
	"regexp"
	"strings"
	"testing"
)

func TestWorkersCommand_Structure(t *testing.T) {
	cmd := newWorkersCommand()

	if cmd.Use != "workers" {
		t.Fatalf("expected use 'workers', got '%s'", cmd.Use)
	}

	found := false
	for _, sub := range cmd.Commands() {
		if sub.Name() == "registration-tokens" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected 'registration-tokens' subcommand")
	}
}

func TestRegistrationTokensCommand_Structure(t *testing.T) {
	cmd := newRegistrationTokensCommand()

	if cmd.Use != "registration-tokens" {
		t.Fatalf("expected use 'registration-tokens', got '%s'", cmd.Use)
	}

	found := false
	for _, sub := range cmd.Commands() {
		if sub.Name() == "generate" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected 'generate' subcommand")
	}
}

func TestGenerateRegistrationToken_Format(t *testing.T) {
	token, err := generateRegistrationToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parts := strings.Split(token, "_")
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts separated by '_', got %d: %s", len(parts), token)
	}

	if parts[0] != "kwreg" {
		t.Fatalf("expected prefix 'kwreg', got '%s'", parts[0])
	}

	if len(parts[1]) != 52 {
		t.Fatalf("expected payload of 52 chars, got %d: %s", len(parts[1]), parts[1])
	}

	payloadRe := regexp.MustCompile(`^[a-z2-7]+$`)
	if !payloadRe.MatchString(parts[1]) {
		t.Fatalf("payload contains invalid chars: %s", parts[1])
	}

	if len(parts[2]) != 6 {
		t.Fatalf("expected checksum of 6 chars, got %d: %s", len(parts[2]), parts[2])
	}

	checksumRe := regexp.MustCompile(`^[0-9a-f]+$`)
	if !checksumRe.MatchString(parts[2]) {
		t.Fatalf("checksum contains invalid chars: %s", parts[2])
	}
}

func TestGenerateRegistrationToken_Uniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		token, err := generateRegistrationToken()
		if err != nil {
			t.Fatalf("unexpected error on iteration %d: %v", i, err)
		}
		if seen[token] {
			t.Fatalf("duplicate token on iteration %d: %s", i, token)
		}
		seen[token] = true
	}
}

func TestRegistrationTokensGenerate_ExecuteCommand(t *testing.T) {
	root := newWorkersCommand()
	output, err := executeCommand(root, "registration-tokens", "generate")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	token := strings.TrimSpace(output)
	parts := strings.Split(token, "_")
	if len(parts) != 3 || parts[0] != "kwreg" {
		t.Fatalf("output is not a valid token: %q", token)
	}
}
