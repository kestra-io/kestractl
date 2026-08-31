package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	kestra "github.com/kestra-io/client-sdk/go-sdk/v2/kestra_api_client"
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
	// Test that the command allows 0 arguments
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newFlowsListCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
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

func TestFlowsValidateCommand_NoArgs(t *testing.T) {
	// Test that the command requires exactly 1 argument
	cmd := newFlowsValidateCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestFlowsValidateCommand_FileNotFound(t *testing.T) {
	// Override client factory to avoid config errors
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return &Client{Tenant: "main"}, nil
	}
	defer func() { newClientFunc = original }()

	cmd := newFlowsValidateCommand()
	_, err := executeCommand(cmd, "/nonexistent/path/flow.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
	if !strings.Contains(err.Error(), "failed to access path") {
		t.Fatalf("expected path access error, got: %v", err)
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
	if !strings.Contains(err.Error(), "failed to access path") {
		t.Fatalf("expected path access error, got: %v", err)
	}
}

func TestFlowsDependenciesCommand_NoArgs(t *testing.T) {
	cmd := newFlowsDependenciesCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 2 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestFlowsDependenciesCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newFlowsDependenciesCommand()
	_, err := executeCommand(cmd, "my.ns", "my-flow")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestFlowsImportCommand_NoArgs(t *testing.T) {
	cmd := newFlowsImportCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestFlowsImportCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newFlowsImportCommand()
	_, err := executeCommand(cmd, "flows.zip")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestFlowsImportCommand_MissingFile(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return &Client{Tenant: "main"}, nil
	}
	defer func() { newClientFunc = original }()

	cmd := newFlowsImportCommand()
	_, err := executeCommand(cmd, "/nonexistent/path/flows.zip")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "failed to open") {
		t.Fatalf("expected open error, got: %v", err)
	}
}

func TestFlowsExportCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newFlowsExportCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestBuildFlowExportFilters(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if got := buildFlowExportFilters("", ""); len(got) != 0 {
			t.Fatalf("expected no filters, got %d", len(got))
		}
	})

	t.Run("namespace and query", func(t *testing.T) {
		filters := buildFlowExportFilters("my.ns", "boom")
		if len(filters) != 2 {
			t.Fatalf("expected 2 filters, got %d", len(filters))
		}
		got := map[kestra.QueryFilterField]any{}
		for _, f := range filters {
			if f.GetOperation() != kestra.QUERYFILTEROP_EQUALS {
				t.Errorf("expected EQUALS op, got %v", f.GetOperation())
			}
			got[f.GetField()] = f.Value
		}
		if got[kestra.QUERYFILTERFIELD_NAMESPACE] != "my.ns" {
			t.Errorf("namespace: got %v", got[kestra.QUERYFILTERFIELD_NAMESPACE])
		}
		if got[kestra.QUERYFILTERFIELD_QUERY] != "boom" {
			t.Errorf("query: got %v", got[kestra.QUERYFILTERFIELD_QUERY])
		}
	})
}

func TestFlowsEnableCommand_NotEnoughArgs(t *testing.T) {
	cmd := newFlowsEnableCommand()
	_, err := executeCommand(cmd, "my.ns")
	if err == nil {
		t.Fatal("expected error when fewer than 2 args provided")
	}
	if !strings.Contains(err.Error(), "requires at least 2 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestFlowsEnableCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newFlowsEnableCommand()
	_, err := executeCommand(cmd, "my.ns", "my-flow")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestFlowsDisableCommand_NotEnoughArgs(t *testing.T) {
	cmd := newFlowsDisableCommand()
	_, err := executeCommand(cmd, "my.ns")
	if err == nil {
		t.Fatal("expected error when fewer than 2 args provided")
	}
	if !strings.Contains(err.Error(), "requires at least 2 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestFlowsDisableCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newFlowsDisableCommand()
	_, err := executeCommand(cmd, "my.ns", "my-flow")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestFlowsNamespacesCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newFlowsNamespacesCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestFlowsRevisionsCommand_NoArgs(t *testing.T) {
	cmd := newFlowsRevisionsCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 2 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestFlowsRevisionsCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newFlowsRevisionsCommand()
	_, err := executeCommand(cmd, "my.ns", "my-flow")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestFlowsDeleteCommand_NoArgs(t *testing.T) {
	cmd := newFlowsDeleteCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 2 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestFlowsDeleteCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newFlowsDeleteCommand()
	_, err := executeCommand(cmd, "my.ns", "my-flow", "--yes")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestFlowsDeleteBulkCommand_NoArgs(t *testing.T) {
	cmd := newFlowsDeleteBulkCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
}

func TestFlowsDeleteBulkCommand_InvalidFormat(t *testing.T) {
	cmd := newFlowsDeleteBulkCommand()
	_, err := executeCommand(cmd, "invalid-no-slash")
	if err == nil {
		t.Fatal("expected error for invalid flow id format")
	}
	if !strings.Contains(err.Error(), "namespace") {
		t.Fatalf("expected format error, got: %v", err)
	}
}

func TestFlowsDeleteBulkCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newFlowsDeleteBulkCommand()
	_, err := executeCommand(cmd, "my.ns/flow1", "my.ns/flow2")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestFlowsDisableBulkCommand_NoArgs(t *testing.T) {
	cmd := newFlowsDisableBulkCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
}

func TestFlowsEnableBulkCommand_NoArgs(t *testing.T) {
	cmd := newFlowsEnableBulkCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
}

func TestParseFlowIds(t *testing.T) {
	ids, err := parseFlowIds([]string{"my.ns/my-flow", "other.ns/other-flow"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 ids, got %d", len(ids))
	}
	if ids[0].GetNamespace() != "my.ns" || ids[0].GetId() != "my-flow" {
		t.Fatalf("unexpected first id: %+v", ids[0])
	}

	_, err = parseFlowIds([]string{"invalid"})
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
}

func TestFlowsSearchCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newFlowsSearchCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestFlowsDeleteCommand_CancelOnDecline(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return &Client{Tenant: "main"}, nil
	}
	defer func() { newClientFunc = original }()

	cmd := newFlowsDeleteCommand()
	cmd.SetIn(strings.NewReader("n\n"))
	out, err := executeCommand(cmd, "my.ns", "my-flow")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Cancelled") {
		t.Fatalf("expected cancellation output, got: %s", out)
	}
}

func TestCollectFlowFiles(t *testing.T) {
	testDir := "testdata/deploy_folder_test"

	files, err := collectFlowFiles(testDir, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should find flow1.yaml, flow2.yaml, flow-invalid.yaml, and subdir/flow3.yaml
	// Should NOT find .hidden.yaml or invalid.txt
	expectedCount := 4
	if len(files) != expectedCount {
		t.Fatalf("expected %d files, got %d: %v", expectedCount, len(files), files)
	}

	// Check that hidden file is not included
	for _, f := range files {
		if strings.Contains(f, ".hidden") {
			t.Fatalf("hidden file should not be included: %s", f)
		}
		if strings.HasSuffix(f, ".txt") {
			t.Fatalf("txt file should not be included: %s", f)
		}
	}
}

func TestCollectFlowFiles_NonRecursive(t *testing.T) {
	testDir := "testdata/deploy_folder_test"

	files, err := collectFlowFiles(testDir, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Non-recursive should find only top-level flow1.yaml, flow2.yaml, flow-invalid.yaml
	// Should NOT find subdir/flow3.yaml, .hidden.yaml, or invalid.txt
	expectedCount := 3
	if len(files) != expectedCount {
		t.Fatalf("expected %d files, got %d: %v", expectedCount, len(files), files)
	}

	for _, f := range files {
		if strings.Contains(f, "subdir") {
			t.Fatalf("file in subdirectory should not be included when recursive=false: %s", f)
		}
	}
}

func TestCollectFlowFiles_NestedDirs(t *testing.T) {
	testDir := "testdata/deploy_folder_test"

	files, err := collectFlowFiles(testDir, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check that nested file is found
	foundNested := false
	for _, f := range files {
		if strings.Contains(f, "subdir") && strings.Contains(f, "flow3.yaml") {
			foundNested = true
			break
		}
	}
	if !foundNested {
		t.Fatal("nested file subdir/flow3.yaml not found")
	}
}

func TestCollectFlowFiles_MixedExtensions(t *testing.T) {
	testDir := "testdata/deploy_folder_test"

	files, err := collectFlowFiles(testDir, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All collected files should be .yaml or .yml
	for _, f := range files {
		ext := strings.ToLower(filepath.Ext(f))
		if ext != ".yaml" && ext != ".yml" {
			t.Fatalf("unexpected file extension: %s", f)
		}
	}
}

func TestCollectFlowFiles_HiddenFiles(t *testing.T) {
	testDir := "testdata/deploy_folder_test"

	files, err := collectFlowFiles(testDir, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No hidden files should be included
	for _, f := range files {
		base := filepath.Base(f)
		if strings.HasPrefix(base, ".") {
			t.Fatalf("hidden file should not be included: %s", f)
		}
	}
}

func TestCollectFlowFiles_SortedOrder(t *testing.T) {
	testDir := "testdata/deploy_folder_test"

	files, err := collectFlowFiles(testDir, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Files should be sorted
	for i := 1; i < len(files); i++ {
		if files[i-1] > files[i] {
			t.Fatalf("files not sorted: %s > %s", files[i-1], files[i])
		}
	}
}

func TestCollectFlowFiles_NonexistentDir(t *testing.T) {
	_, err := collectFlowFiles("/nonexistent/directory", true)
	if err == nil {
		t.Fatal("expected error for nonexistent directory")
	}
}

func TestCollectFlowFiles_EmptyDir(t *testing.T) {
	// Create a temporary empty directory
	tmpDir, err := os.MkdirTemp("", "empty_test_dir")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	files, err := collectFlowFiles(tmpDir, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected 0 files, got %d", len(files))
	}
}

func TestFlowsDeployCommand_DirectoryNotFound(t *testing.T) {
	// Override client factory to avoid config errors
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return &Client{Tenant: "main"}, nil
	}
	defer func() { newClientFunc = original }()

	cmd := newFlowsDeployCommand()
	_, err := executeCommand(cmd, "/nonexistent/directory/")
	if err == nil {
		t.Fatal("expected error for nonexistent directory")
	}
	if !strings.Contains(err.Error(), "failed to access path") {
		t.Fatalf("expected path access error, got: %v", err)
	}
}

func TestFlowsDeployCommand_Flags(t *testing.T) {
	cmd := newFlowsDeployCommand()

	// Check that all expected flags exist
	flags := []string{"override", "namespace", "fail-fast", "recursive"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Fatalf("expected flag --%s to exist", flag)
		}
	}
}

func TestFlowsDeployCommand_Help(t *testing.T) {
	cmd := newFlowsDeployCommand()
	output, err := executeCommand(cmd, "--help")
	if err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}

	// Check for key elements in help text
	expectedStrings := []string{
		"deploy <path>",
		"--override",
		"--namespace",
		"--fail-fast",
		"--recursive",
		"directory",
		"recursive",
	}
	for _, s := range expectedStrings {
		if !strings.Contains(output, s) {
			t.Fatalf("expected help to contain '%s', got: %s", s, output)
		}
	}
}

func TestFlowsValidateCommand_Help(t *testing.T) {
	cmd := newFlowsValidateCommand()
	output, err := executeCommand(cmd, "--help")
	if err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}

	// Check for key elements in help text
	expectedStrings := []string{
		"validate <path>",
		"--recursive",
		"directory",
		"recursive",
		"json",
	}
	for _, s := range expectedStrings {
		if !strings.Contains(output, s) {
			t.Fatalf("expected help to contain '%s', got: %s", s, output)
		}
	}
}

func TestDeployResult_Structure(t *testing.T) {
	// Test that DeployResult has the expected fields
	result := DeployResult{
		FilePath:  "test/flow.yaml",
		FlowID:    "test-flow",
		Namespace: "test.namespace",
		Revision:  1,
		Success:   true,
		Error:     "",
	}

	if result.FilePath != "test/flow.yaml" {
		t.Fatalf("expected FilePath 'test/flow.yaml', got '%s'", result.FilePath)
	}
	if result.FlowID != "test-flow" {
		t.Fatalf("expected FlowID 'test-flow', got '%s'", result.FlowID)
	}
	if result.Namespace != "test.namespace" {
		t.Fatalf("expected Namespace 'test.namespace', got '%s'", result.Namespace)
	}
	if result.Revision != 1 {
		t.Fatalf("expected Revision 1, got %d", result.Revision)
	}
	if !result.Success {
		t.Fatal("expected Success to be true")
	}
}

func TestReplaceNamespaceInYAML(t *testing.T) {
	tests := []struct {
		name         string
		yaml         string
		newNamespace string
		wantErr      bool
	}{
		{
			name:         "replace namespace",
			yaml:         "id: my-flow\nnamespace: old.namespace\n",
			newNamespace: "new.namespace",
			wantErr:      false,
		},
		{
			name:         "replace with complex yaml",
			yaml:         "id: my-flow\nnamespace: old.namespace\ndescription: A test\ntasks:\n  - id: task1\n",
			newNamespace: "prod.namespace",
			wantErr:      false,
		},
		{
			name:         "invalid yaml",
			yaml:         "not: valid: yaml: [",
			newNamespace: "new.namespace",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := replaceNamespaceInYAML(tt.yaml, tt.newNamespace)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Parse the result and verify namespace was replaced
			ns, _, err := parseFlowYAML(result)
			if err != nil {
				t.Fatalf("failed to parse result: %v", err)
			}
			if ns != tt.newNamespace {
				t.Fatalf("expected namespace '%s', got '%s'", tt.newNamespace, ns)
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

func TestFlowsBulkUpdateCommand_MissingFile(t *testing.T) {
	cmd := newFlowsBulkUpdateCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when --file not provided")
	}
	if !strings.Contains(err.Error(), "--file") {
		t.Fatalf("expected --file error, got: %v", err)
	}
}

func TestFlowsBulkUpdateCommand_ClientError(t *testing.T) {
	f, err := os.CreateTemp("", "flows-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString("id: test\nnamespace: my.ns\n")
	f.Close()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newFlowsBulkUpdateCommand()
	_, err = executeCommand(cmd, "--file", f.Name())
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestFlowsGenerateGraphFromSourceCommand_MissingFile(t *testing.T) {
	cmd := newFlowsGenerateGraphFromSourceCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no --file provided")
	}
	if !strings.Contains(err.Error(), "--file") {
		t.Fatalf("expected --file error, got: %v", err)
	}
}

func TestFlowsTaskCommand_NoArgs(t *testing.T) {
	cmd := newFlowsTaskCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 3 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestFlowsGraphCommand_NoArgs(t *testing.T) {
	cmd := newFlowsGenerateGraphCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 2 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestRunFlowsGenerateGraph(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/flows/my.namespace/my-flow/graph") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodes":[{"uid":"n1","type":"io.kestra.core.tasks.flows.Sequential"}],"edges":[{"source":"n1","target":"n2"}]}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runFlowsGenerateGraph(newTestClient(t, server.URL), "my.namespace", "my-flow", nil, nil, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runFlowsGenerateGraph error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"n1", "Sequential", "Edges: 1"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunFlowsGenerateGraph_Revision(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("revision"); got != "3" {
			t.Errorf("expected revision=3, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodes":[],"edges":[]}`))
	}))
	t.Cleanup(server.Close)

	rev := 3
	var buf bytes.Buffer
	err := runFlowsGenerateGraph(newTestClient(t, server.URL), "my.namespace", "my-flow", &rev, nil, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runFlowsGenerateGraph error: %v", err)
	}
}

func TestFlowsTaskCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newFlowsTaskCommand()
	_, err := executeCommand(cmd, "my.ns", "my-flow", "my-task")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestFlowsExportByIdsCommand_NoArgs(t *testing.T) {
	cmd := newFlowsExportByIdsCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
}

func TestFlowsExportByIdsCommand_InvalidFormat(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	cmd := newFlowsExportByIdsCommand()
	_, err := executeCommand(cmd, "bad-format")
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
	if !strings.Contains(err.Error(), "namespace") {
		t.Fatalf("expected namespace error, got: %v", err)
	}
}

func TestFlowsExportByIdsCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newFlowsExportByIdsCommand()
	_, err := executeCommand(cmd, "my.ns/flow1", "--output-file", "/dev/null")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestFlowsExportByQueryCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newFlowsExportByQueryCommand()
	_, err := executeCommand(cmd, "--output-file", "/dev/null")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestFlowsDeleteByQueryCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newFlowsDeleteByQueryCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestFlowsDisableByQueryCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newFlowsDisableByQueryCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestFlowsEnableByQueryCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newFlowsEnableByQueryCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestFlowsSearchBySourceCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newFlowsSearchBySourceCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestFlowsConcurrencyLimitsCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newFlowsConcurrencyLimitsCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestFlowsDeleteRevisionsCommand_NoArgs(t *testing.T) {
	cmd := newFlowsDeleteRevisionsCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 2 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestFlowsDeleteRevisionsCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newFlowsDeleteRevisionsCommand()
	_, err := executeCommand(cmd, "my.namespace", "my-flow")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestFlowsListByNamespaceCommand_NoArgs(t *testing.T) {
	cmd := newFlowsListByNamespaceCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestFlowsListByNamespaceCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newFlowsListByNamespaceCommand()
	_, err := executeCommand(cmd, "my.namespace")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestFlowsListDeprecatedCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newFlowsListDeprecatedCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestFlowsExpressionsCommand_MissingFile(t *testing.T) {
	cmd := newFlowsExpressionsCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when --file not provided")
	}
	if !strings.Contains(err.Error(), "--file") {
		t.Fatalf("expected --file error, got: %v", err)
	}
}

func TestFlowsExpressionsCommand_ClientError(t *testing.T) {
	f, err := os.CreateTemp("", "flow-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString("id: my-flow\nnamespace: my.ns\n")
	f.Close()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newFlowsExpressionsCommand()
	_, err = executeCommand(cmd, "--file", f.Name())
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestFlowsNamespaceDependenciesCommand_NoArgs(t *testing.T) {
	cmd := newFlowsNamespaceDependenciesCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestFlowsNamespaceDependenciesCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newFlowsNamespaceDependenciesCommand()
	_, err := executeCommand(cmd, "my.namespace")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestRunFlowsNamespaceDependencies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodes":[{"uid":"n1","id":"flow-a","namespace":"my.namespace"}],"edges":[{"source":"n1","target":"n2","relation":"FLOW_TASK"}]}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runFlowsNamespaceDependencies(newTestClient(t, server.URL), "my.namespace", false, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runFlowsNamespaceDependencies error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"n1", "n2", "FLOW_TASK", "1 node(s), 1 dependency edge(s)"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
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

func TestFlowsValidateTaskCommand_NoFile(t *testing.T) {
	cmd := newFlowsValidateTaskCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when --file is missing")
	}
	if !strings.Contains(err.Error(), "--file is required") {
		t.Fatalf("expected --file error, got: %v", err)
	}
}

func TestRunFlowsValidateTask_Valid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"index":0,"constraints":"","warnings":[]}`))
	}))
	t.Cleanup(server.Close)

	f, err := os.CreateTemp("", "task-*.yml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	_, _ = f.WriteString("type: io.kestra.plugin.core.log.Log\nmessage: hello")
	f.Close()

	var buf bytes.Buffer
	err = runFlowsValidateTask(newTestClient(t, server.URL), f.Name(), "tasks", newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runFlowsValidateTask error: %v", err)
	}
	if !strings.Contains(buf.String(), "passed") {
		t.Errorf("expected 'passed' in output, got:\n%s", buf.String())
	}
}

func TestRunFlowsValidateTask_Invalid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"index":0,"constraints":"type is required","warnings":[]}`))
	}))
	t.Cleanup(server.Close)

	f, err := os.CreateTemp("", "task-*.yml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	_, _ = f.WriteString("message: hello")
	f.Close()

	var buf bytes.Buffer
	err = runFlowsValidateTask(newTestClient(t, server.URL), f.Name(), "tasks", newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runFlowsValidateTask error: %v", err)
	}
	if !strings.Contains(buf.String(), "type is required") {
		t.Errorf("expected constraint message in output, got:\n%s", buf.String())
	}
}

func TestFlowsValidateTriggerCommand_NoFile(t *testing.T) {
	cmd := newFlowsValidateTriggerCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when --file is missing")
	}
	if !strings.Contains(err.Error(), "--file is required") {
		t.Fatalf("expected --file error, got: %v", err)
	}
}

func TestRunFlowsValidateTrigger_Valid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"index":0,"constraints":"","warnings":["deprecated field"]}`))
	}))
	t.Cleanup(server.Close)

	f, err := os.CreateTemp("", "trigger-*.yml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	_, _ = f.WriteString("type: io.kestra.plugin.core.trigger.Schedule\ncron: \"0 * * * *\"")
	f.Close()

	var buf bytes.Buffer
	err = runFlowsValidateTrigger(newTestClient(t, server.URL), f.Name(), newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runFlowsValidateTrigger error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "passed") {
		t.Errorf("expected 'passed' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "deprecated field") {
		t.Errorf("expected warning in output, got:\n%s", out)
	}
}

func TestFlowsUpdateConcurrencyCommand_NoArgs(t *testing.T) {
	cmd := newFlowsUpdateConcurrencyCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 2 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestRunFlowsUpdateConcurrency(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tenantId":"default","namespace":"prod","flowId":"my-flow","running":5}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runFlowsUpdateConcurrency(newTestClient(t, server.URL), "prod", "my-flow", 5, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runFlowsUpdateConcurrency error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"prod", "my-flow", "5", "updated"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestFlowsNamespaceSyncCommand_NoArgs(t *testing.T) {
	cmd := newFlowsNamespaceSyncCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 2 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestRunFlowsNamespaceSync(t *testing.T) {
	var gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"f1","namespace":"prod","flowId":"flow-one"}]`))
	}))
	t.Cleanup(server.Close)

	tmpFile, err := os.CreateTemp("", "flows-*.yaml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	_, _ = tmpFile.WriteString("id: flow-one\nnamespace: prod\n")
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	var buf bytes.Buffer
	err = runFlowsNamespaceSync(newTestClient(t, server.URL), "prod", tmpFile.Name(), false, false, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runFlowsNamespaceSync error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if !strings.Contains(buf.String(), "prod") {
		t.Errorf("expected namespace in output, got:\n%s", buf.String())
	}
}

// --- Array-format labels regression tests (kestra-io/kestractl#83) -----------
//
// Kestra returns flow labels as a JSON array ([{"key":..,"value":..}]) which the
// generated client cannot decode into its object-typed field, failing every
// flows get/list/deploy against a namespace that has labelled flows. These tests
// drive the real SDK against a stub server returning array-format labels and
// assert the commands recover from the raw response body.

// A complete, otherwise-valid flow that fails to decode ONLY because of its
// array-format labels — faithfully reproducing #83 (see the assertion in
// TestConfirmArrayLabelsDecodeError).
const flowWithArrayLabelsJSON = `{"id":"my-flow","namespace":"my.namespace","revision":2,"description":"labelled flow","disabled":false,"draft":false,"deleted":false,"tasks":[],"source":"id: my-flow\nnamespace: my.namespace\nlabels:\n  - key: type\n    value: data_extraction\n","labels":[{"key":"type","value":"data_extraction"},{"key":"version","value":"v2"}]}`

func TestRunFlowsGet_ArrayLabels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/flows/my.namespace/my-flow") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(flowWithArrayLabelsJSON))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runFlowsGet(newTestClient(t, server.URL), "my.namespace", "my-flow", newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runFlowsGet error: %v", err)
	}
	if !strings.Contains(buf.String(), "data_extraction") {
		t.Errorf("expected flow source in output, got:\n%s", buf.String())
	}
}

func TestRunFlowsGet_ArrayLabels_JSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(flowWithArrayLabelsJSON))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	if err := runFlowsGet(newTestClient(t, server.URL), "my.namespace", "my-flow", newJSONRenderer(&buf)); err != nil {
		t.Fatalf("runFlowsGet error: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if got["id"] != "my-flow" {
		t.Errorf("expected id my-flow, got %v", got["id"])
	}
	if got["revision"] != float64(2) {
		t.Errorf("expected revision 2, got %v", got["revision"])
	}
	if !strings.Contains(got["source"].(string), "data_extraction") {
		t.Errorf("expected recovered source, got %v", got["source"])
	}
}

func TestRunFlowsList_Namespace_ArrayLabels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/flows/my.namespace") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[" + flowWithArrayLabelsJSON + `,{"id":"second","namespace":"my.namespace","revision":1,"disabled":false,"deleted":false,"tasks":[],"labels":[{"key":"team","value":"data"}]}]`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	if err := runFlowsList(newTestClient(t, server.URL), "my.namespace", newTableRenderer(&buf)); err != nil {
		t.Fatalf("runFlowsList error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"my-flow", "second", "labelled flow"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestListAllFlows_Search_ArrayLabels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/flows/search") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[` + flowWithArrayLabelsJSON + `],"total":1}`))
	}))
	t.Cleanup(server.Close)

	// namespace "" routes through listAllFlows -> SearchFlows.
	var buf bytes.Buffer
	if err := runFlowsList(newTestClient(t, server.URL), "", newTableRenderer(&buf)); err != nil {
		t.Fatalf("runFlowsList (all) error: %v", err)
	}
	if !strings.Contains(buf.String(), "my-flow") {
		t.Errorf("expected flow in output, got:\n%s", buf.String())
	}
}

func TestDeployFlow_OverrideWithArrayLabels(t *testing.T) {
	var putCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			// Exists-check: 200 with array-format labels -> decode fails but flow exists.
			_, _ = w.Write([]byte(flowWithArrayLabelsJSON))
		case http.MethodPut:
			putCalled = true
			// Update response also carries the array-format labels.
			_, _ = w.Write([]byte(`{"id":"my-flow","namespace":"my.namespace","revision":3,"disabled":false,"deleted":false,"tasks":[],"labels":[{"key":"type","value":"data_extraction"}]}`))
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	}))
	t.Cleanup(server.Close)

	tmpFile, err := os.CreateTemp(t.TempDir(), "flow-*.yaml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	_, _ = tmpFile.WriteString("id: my-flow\nnamespace: my.namespace\nlabels:\n  - key: type\n    value: data_extraction\ntasks:\n  - id: t1\n    type: io.kestra.plugin.core.log.Log\n    message: hi\n")
	tmpFile.Close()

	result := deployFlow(newTestClient(t, server.URL), tmpFile.Name(), "", true)
	if !result.Success {
		t.Fatalf("expected deploy success, got error: %s", result.Error)
	}
	if !putCalled {
		t.Error("expected the update PUT to be issued")
	}
	if result.Revision != 3 {
		t.Errorf("expected revision 3, got %d", result.Revision)
	}
}

func TestDeployFlow_CreateWithArrayLabels(t *testing.T) {
	var postCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			// Exists-check: flow does not exist.
			w.WriteHeader(http.StatusNotFound)
		case http.MethodPost:
			postCalled = true
			_, _ = w.Write([]byte(`{"id":"my-flow","namespace":"my.namespace","revision":1,"disabled":false,"deleted":false,"tasks":[],"labels":[{"key":"type","value":"data_extraction"}]}`))
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	}))
	t.Cleanup(server.Close)

	tmpFile, err := os.CreateTemp(t.TempDir(), "flow-*.yaml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	_, _ = tmpFile.WriteString("id: my-flow\nnamespace: my.namespace\nlabels:\n  - key: type\n    value: data_extraction\ntasks:\n  - id: t1\n    type: io.kestra.plugin.core.log.Log\n    message: hi\n")
	tmpFile.Close()

	result := deployFlow(newTestClient(t, server.URL), tmpFile.Name(), "", false)
	if !result.Success {
		t.Fatalf("expected deploy success, got error: %s", result.Error)
	}
	if !postCalled {
		t.Error("expected the create POST to be issued")
	}
	if result.Revision != 1 {
		t.Errorf("expected revision 1, got %d", result.Revision)
	}
}

func TestTryParseFlowFromError(t *testing.T) {
	t.Run("recovers flow from labelled body", func(t *testing.T) {
		sdkErr := &kestra.GenericOpenAPIError{}
		setGenericOpenAPIErrorBody(sdkErr, []byte(flowWithArrayLabelsJSON))
		f, ok := tryParseFlowFromError(sdkErr)
		if !ok {
			t.Fatal("expected recovery to succeed")
		}
		if f.ID != "my-flow" || f.Namespace != "my.namespace" || f.Revision != 2 {
			t.Errorf("unexpected parsed flow: %+v", f)
		}
		if !strings.Contains(f.Source, "data_extraction") {
			t.Errorf("expected source recovered, got %q", f.Source)
		}
	})

	t.Run("propagates genuine error body without id", func(t *testing.T) {
		sdkErr := &kestra.GenericOpenAPIError{}
		setGenericOpenAPIErrorBody(sdkErr, []byte(`{"message":"boom"}`))
		if _, ok := tryParseFlowFromError(sdkErr); ok {
			t.Fatal("expected recovery to fail for an error body without an id")
		}
	})

	t.Run("ignores non-SDK errors", func(t *testing.T) {
		if _, ok := tryParseFlowFromError(errors.New("plain error")); ok {
			t.Fatal("expected recovery to fail for a non-SDK error")
		}
	})
}

func TestTryParseFlowListFromError(t *testing.T) {
	t.Run("recovers array body", func(t *testing.T) {
		sdkErr := &kestra.GenericOpenAPIError{}
		setGenericOpenAPIErrorBody(sdkErr, []byte("["+flowWithArrayLabelsJSON+"]"))
		flows, ok := tryParseFlowListFromError(sdkErr)
		if !ok || len(flows) != 1 || flows[0].ID != "my-flow" {
			t.Fatalf("unexpected recovery: ok=%v flows=%+v", ok, flows)
		}
	})

	t.Run("rejects non-array body", func(t *testing.T) {
		sdkErr := &kestra.GenericOpenAPIError{}
		setGenericOpenAPIErrorBody(sdkErr, []byte(`{"message":"boom"}`))
		if _, ok := tryParseFlowListFromError(sdkErr); ok {
			t.Fatal("expected recovery to fail for a non-array body")
		}
	})
}

func TestTryParseFlowSearchFromError(t *testing.T) {
	t.Run("recovers paged body", func(t *testing.T) {
		sdkErr := &kestra.GenericOpenAPIError{}
		setGenericOpenAPIErrorBody(sdkErr, []byte(`{"results":[`+flowWithArrayLabelsJSON+`],"total":7}`))
		flows, total, ok := tryParseFlowSearchFromError(sdkErr)
		if !ok || total != 7 || len(flows) != 1 || flows[0].ID != "my-flow" {
			t.Fatalf("unexpected recovery: ok=%v total=%d flows=%+v", ok, total, flows)
		}
	})

	t.Run("rejects body without results", func(t *testing.T) {
		sdkErr := &kestra.GenericOpenAPIError{}
		setGenericOpenAPIErrorBody(sdkErr, []byte(`{"message":"boom"}`))
		if _, _, ok := tryParseFlowSearchFromError(sdkErr); ok {
			t.Fatal("expected recovery to fail for a body without results")
		}
	})
}

// TestConfirmArrayLabelsDecodeError used to guard fixture fidelity for #83: the
// generated client failed to decode array-format labels, which is what the
// tryParseFlow* fallbacks exist to recover from. go-sdk v2 fixed this (Labels
// is now a MapObjectObject that accepts both array and map form), so the
// fixture now decodes cleanly. The tryParseFlow* fallbacks are kept as a
// defensive fallback for other SDK type-mismatch-on-error cases (see
// formatSDKError / AGENTS.md), but are no longer exercised by this fixture.
func TestConfirmArrayLabelsDecodeError(t *testing.T) {
	var fws kestra.FlowWithSource
	err := json.Unmarshal([]byte(flowWithArrayLabelsJSON), &fws)
	if err != nil {
		t.Fatalf("expected array-format labels to decode cleanly on go-sdk v2, got: %v", err)
	}
}
