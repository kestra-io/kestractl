package cli

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestParseFlowYAML(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		wantNS      string
		wantID      string
		wantErr     bool
		errContains string
	}{
		{
			name:   "valid flow",
			yaml:   "id: my-flow\nnamespace: test.namespace\n",
			wantNS: "test.namespace",
			wantID: "my-flow",
		},
		{
			name:   "valid flow with extra fields",
			yaml:   "id: my-flow\nnamespace: test.namespace\ndescription: A test flow\n",
			wantNS: "test.namespace",
			wantID: "my-flow",
		},
		{
			name:        "missing namespace",
			yaml:        "id: my-flow\n",
			wantErr:     true,
			errContains: "namespace",
		},
		{
			name:        "missing id",
			yaml:        "namespace: test.namespace\n",
			wantErr:     true,
			errContains: "id",
		},
		{
			name:        "empty namespace",
			yaml:        "id: my-flow\nnamespace: \n",
			wantErr:     true,
			errContains: "namespace",
		},
		{
			name:        "empty id",
			yaml:        "id: \nnamespace: test.namespace\n",
			wantErr:     true,
			errContains: "id",
		},
		{
			name:        "invalid yaml",
			yaml:        "not: valid: yaml: [",
			wantErr:     true,
			errContains: "invalid YAML",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns, id, err := parseFlowYAML(tt.yaml)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("expected error containing '%s', got: %v", tt.errContains, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ns != tt.wantNS {
				t.Fatalf("expected namespace '%s', got '%s'", tt.wantNS, ns)
			}
			if id != tt.wantID {
				t.Fatalf("expected id '%s', got '%s'", tt.wantID, id)
			}
		})
	}
}

func TestFlowsListCommand_NoArgs(t *testing.T) {
	// Test that the command requires exactly 1 argument
	cmd := newFlowsListCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestFlowsGetCommand_NoArgs(t *testing.T) {
	// Test that the command requires exactly 2 arguments
	cmd := newFlowsGetCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 2 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestFlowsDeployCommand_NoArgs(t *testing.T) {
	// Test that the command requires exactly 1 argument
	cmd := newFlowsDeployCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestFlowsDeployCommand_FileNotFound(t *testing.T) {
	// Override client factory to avoid config errors
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return &Client{Tenant: "main"}, nil
	}
	defer func() { newClientFunc = original }()

	cmd := newFlowsDeployCommand()
	_, err := executeCommand(cmd, "/nonexistent/path/flow.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
	if !strings.Contains(err.Error(), "failed to read file") {
		t.Fatalf("expected file read error, got: %v", err)
	}
}

func TestValidateOutputFormat(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{input: "table", wantErr: false},
		{input: "json", wantErr: false},
		{input: "TABLE", wantErr: false},
		{input: "JSON", wantErr: false},
		{input: "", wantErr: false}, // defaults to table
		{input: "xml", wantErr: true},
		{input: "yaml", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			globalFlags.Output = tt.input
			err := validateOutputFormat()
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func executeCommand(cmd *cobra.Command, args ...string) (string, error) {
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)

	stdout, err := captureStdout(func() error {
		return cmd.Execute()
	})
	return buf.String() + stdout, err
}

func captureStdout(fn func() error) (string, error) {
	originalStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	defer func() {
		os.Stdout = originalStdout
	}()

	os.Stdout = w

	var buf bytes.Buffer
	copyDone := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(&buf, r)
		r.Close()
		copyDone <- copyErr
	}()

	closed := false
	defer func() {
		if !closed {
			w.Close()
		}
	}()

	errFn := fn()
	if !closed {
		w.Close()
		closed = true
	}

	copyErr := <-copyDone

	if errFn != nil {
		return buf.String(), errFn
	}
	return buf.String(), copyErr
}
