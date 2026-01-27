package cli

import (
	"strings"
	"testing"
)

func TestExecutionsRunCommand_NoArgs(t *testing.T) {
	cmd := newExecutionsRunCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 2 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestExecutionsRunCommand_OneArg(t *testing.T) {
	cmd := newExecutionsRunCommand()
	_, err := executeCommand(cmd, "namespace")
	if err == nil {
		t.Fatal("expected error when only 1 arg provided")
	}
	if !strings.Contains(err.Error(), "accepts 2 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestExecutionsGetCommand_NoArgs(t *testing.T) {
	cmd := newExecutionsGetCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestExecutionsKillCommand_RequiresNamespaceWithFlowID(t *testing.T) {
	// Override client to avoid config errors
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return &Client{Tenant: "main"}, nil
	}
	defer func() { newClientFunc = original }()

	cmd := newExecutionsKillCommand()
	_, err := executeCommand(cmd, "--flow-id", "some-flow")
	if err == nil {
		t.Fatal("expected error when flow-id is provided without namespace")
	}
	if !strings.Contains(err.Error(), "--namespace is required") {
		t.Fatalf("expected namespace required error, got: %v", err)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		input    any
		expected string
	}{
		{input: "PT5S", expected: "5.00s"},
		{input: "PT5.123S", expected: "5.12s"},
		{input: "PT0.5S", expected: "0.50s"},
		{input: float64(5000), expected: "5.00s"},
		{input: float64(1234), expected: "1.23s"},
		{input: "not-iso", expected: "not-iso"},
		{input: "plain string", expected: "plain string"},
		{input: 42, expected: "42"},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := formatDuration(tt.input)
			if result != tt.expected {
				t.Errorf("formatDuration(%v) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseISO8601Duration(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		wantErr  bool
	}{
		{input: "PT5S", expected: "5.00s"},
		{input: "PT5.123S", expected: "5.12s"},
		{input: "PT0.5S", expected: "0.50s"},
		{input: "PT123S", expected: "123.00s"},
		{input: "PT0.001S", expected: "0.00s"},
		{input: "invalid", wantErr: true},
		{input: "PT", wantErr: true},
		{input: "PTS", wantErr: true},
		{input: "5S", wantErr: true},
		{input: "PT5M", wantErr: true}, // Minutes not supported
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := parseISO8601Duration(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseISO8601Duration(%s) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("parseISO8601Duration(%s) unexpected error: %v", tt.input, err)
				return
			}
			if result != tt.expected {
				t.Errorf("parseISO8601Duration(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestPrintExecutionState(t *testing.T) {
	// Test with valid state
	execution := map[string]any{
		"state": map[string]any{
			"current":   "SUCCESS",
			"startDate": "2024-01-15T10:00:00Z",
			"endDate":   "2024-01-15T10:00:05Z",
			"duration":  "PT5S",
		},
	}

	// This should not panic
	output, _ := captureStdout(func() error {
		printExecutionState(execution, true)
		return nil
	})

	if !strings.Contains(output, "State: SUCCESS") {
		t.Errorf("expected state output, got: %s", output)
	}
}

func TestPrintExecutionState_Unknown(t *testing.T) {
	// Test with missing state
	execution := map[string]any{}

	output, _ := captureStdout(func() error {
		printExecutionState(execution, false)
		return nil
	})

	if !strings.Contains(output, "State: unknown") {
		t.Errorf("expected 'unknown' state, got: %s", output)
	}
}
