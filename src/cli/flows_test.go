package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	kestra "github.com/kestra-io/client-sdk/go-sdk/kestra_api_client"
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
