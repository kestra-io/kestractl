package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// DeployResult holds the result of deploying a single flow.
type DeployResult struct {
	FilePath  string `json:"file_path"`
	FlowID    string `json:"flow_id"`
	Namespace string `json:"namespace"`
	Revision  int32  `json:"revision,omitempty"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
}

func newFlowsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "flows",
		Short: "Manage flows",
	}

	cmd.AddCommand(newFlowsListCommand())
	cmd.AddCommand(newFlowsGetCommand())
	cmd.AddCommand(newFlowsDeployCommand())

	return cmd
}

func newFlowsListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list <namespace>",
		Short: "List flows in a namespace.",
		Long: `List all flows in the specified namespace.

Returns a table showing flow ID, namespace, description, and revision number.`,
		Example: `  # List all flows in a namespace
  kestra flows list my.namespace

  # List flows with JSON output
  kestra flows list my.namespace --output json`,
		Aliases: []string{"ls"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutputFormat(); err != nil {
				return err
			}

			client, err := NewClient()
			if err != nil {
				return err
			}

			return runFlowsList(client, args[0])
		},
	}
}

func runFlowsList(client *Client, namespace string) error {
	flows, _, err := client.API.FlowsAPI.ListFlowsByNamespace(client.Ctx, namespace, client.Tenant).Execute()
	if err != nil {
		return formatSDKError(err)
	}

	if globalFlags.Output == "json" {
		result := make([]map[string]any, len(flows))
		for i, flow := range flows {
			result[i] = map[string]any{
				"id":          flow.GetId(),
				"namespace":   flow.GetNamespace(),
				"description": flow.GetDescription(),
				"revision":    flow.GetRevision(),
			}
		}
		return printJSON(result)
	}

	w := tabWriter()
	fmt.Fprintln(w, "ID\tNamespace\tDescription\tRevision")
	for _, flow := range flows {
		desc := flow.GetDescription()
		if desc == "" {
			desc = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\n",
			flow.GetId(),
			flow.GetNamespace(),
			desc,
			flow.GetRevision())
	}
	w.Flush()

	return nil
}

func newFlowsGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "get <namespace> <flow_id>",
		Short: "Get a specific flow.",
		Long: `Retrieve the full definition of a specific flow.

The default output format is JSON as it preserves the complete flow definition.`,
		Example: `  # Get a flow as JSON
  kestra flows get my.namespace my-flow

  # Get a flow as a table
  kestra flows get my.namespace my-flow --output table`,
		Aliases: []string{"show", "describe"},
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutputFormat(); err != nil {
				return err
			}

			client, err := NewClient()
			if err != nil {
				return err
			}

			return runFlowsGet(client, args[0], args[1])
		},
	}
}

func runFlowsGet(client *Client, namespace, flowID string) error {
	flow, _, err := client.API.FlowsAPI.Flow(client.Ctx, namespace, flowID, client.Tenant).
		Source(true).
		AllowDeleted(false).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	result := map[string]any{
		"id":        flow.GetId(),
		"namespace": flow.GetNamespace(),
		"revision":  flow.GetRevision(),
	}

	if globalFlags.Output == "json" {
		return printJSON(result)
	}

	w := tabWriter()
	fmt.Fprintln(w, "Property\tValue")

	keys := make([]string, 0, len(result))
	for key := range result {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		val := toPrettyString(result[key])
		val = strings.ReplaceAll(val, "\n", "\\n")
		fmt.Fprintf(w, "%s\t%s\n", key, val)
	}
	w.Flush()

	return nil
}

func newFlowsDeployCommand() *cobra.Command {
	var (
		override          bool
		namespaceOverride string
		failFast          bool
	)

	cmd := &cobra.Command{
		Use:          "deploy <path>",
		Short:        "Deploy flows from a YAML file or directory.",
		SilenceUsage: true,
		Long: `Deploy flow definitions from a local YAML file or directory to Kestra.

When a directory is provided, all .yaml and .yml files are deployed recursively.
Hidden files and directories (starting with .) are skipped.

By default, the deployment will fail if a flow already exists.
Use --override to update existing flows.

By default, when deploying multiple flows, all files are processed even if some fail.
Use --fail-fast to stop on the first error.`,
		Example: `  # Deploy a single flow
  kestra flows deploy flow.yaml

  # Deploy all flows in a directory (recursive)
  kestra flows deploy ./flows/

  # Deploy with namespace override (all flows go to specified namespace)
  kestra flows deploy ./flows/ --namespace prod.namespace

  # Stop on first error (fail-fast)
  kestra flows deploy ./flows/ --fail-fast

  # Override existing flows
  kestra flows deploy ./flows/ --override

  # Combine flags
  kestra flows deploy ./flows/ --namespace prod --override --fail-fast

  # Deploy with JSON output
  kestra flows deploy flow.yaml --output json`,
		Aliases: []string{"create", "apply"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutputFormat(); err != nil {
				return err
			}

			client, err := NewClient()
			if err != nil {
				return err
			}

			return runFlowsDeploy(client, args[0], override, namespaceOverride, failFast)
		},
	}

	cmd.Flags().BoolVar(&override, "override", false, "Override the flow if it already exists")
	cmd.Flags().StringVar(&namespaceOverride, "namespace", "", "Override the namespace for all deployed flows")
	cmd.Flags().BoolVar(&failFast, "fail-fast", false, "Stop on the first deployment error")

	return cmd
}

func runFlowsDeploy(client *Client, path string, override bool, namespaceOverride string, failFast bool) error {
	// Check if path is a file or directory
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to access path '%s': %w", path, err)
	}

	var files []string
	if info.IsDir() {
		// Collect all YAML files recursively
		files, err = collectFlowFiles(path)
		if err != nil {
			return fmt.Errorf("failed to scan directory '%s': %w", path, err)
		}
		if len(files) == 0 {
			return fmt.Errorf("no .yaml or .yml files found in directory '%s'", path)
		}
	} else {
		// Single file
		files = []string{path}
	}

	// Deploy all flows
	results := make([]DeployResult, 0, len(files))
	for _, file := range files {
		result := deployFlow(client, file, namespaceOverride, override)
		results = append(results, result)

		if !result.Success && failFast {
			// Stop on first error when fail-fast is enabled
			break
		}
	}

	// Output results
	return formatDeployResults(results, len(files) == 1)
}

// collectFlowFiles recursively collects all .yaml and .yml files from a directory.
// Hidden files and directories (starting with .) are skipped.
func collectFlowFiles(rootPath string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Skip hidden files/dirs (but not the root path itself if it starts with .)
		if strings.HasPrefix(d.Name(), ".") && path != rootPath {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		// Collect .yaml and .yml files
		if !d.IsDir() {
			ext := strings.ToLower(filepath.Ext(path))
			if ext == ".yaml" || ext == ".yml" {
				files = append(files, path)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files) // Deterministic order
	return files, nil
}

// deployFlow deploys a single flow file and returns the result.
func deployFlow(client *Client, filePath string, namespaceOverride string, override bool) DeployResult {
	result := DeployResult{
		FilePath: filePath,
		Success:  false,
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		result.Error = fmt.Sprintf("failed to read file: %v", err)
		return result
	}

	yamlContent := string(content)
	namespace, flowID, err := parseFlowYAML(yamlContent)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	result.FlowID = flowID

	// Apply namespace override if provided
	if namespaceOverride != "" {
		// Modify YAML content to use the overridden namespace
		yamlContent, err = replaceNamespaceInYAML(yamlContent, namespaceOverride)
		if err != nil {
			result.Error = fmt.Sprintf("failed to override namespace: %v", err)
			return result
		}
		namespace = namespaceOverride
	}
	result.Namespace = namespace

	// Check if flow exists
	exists := false
	_, resp, checkErr := client.API.FlowsAPI.Flow(client.Ctx, namespace, flowID, client.Tenant).
		Source(false).
		AllowDeleted(false).
		Execute()
	if checkErr == nil {
		exists = true
	} else if resp != nil && resp.StatusCode != 404 {
		result.Error = formatSDKError(checkErr).Error()
		return result
	}

	if exists && !override {
		result.Error = fmt.Sprintf("flow '%s' already exists in namespace '%s'; use --override to update", flowID, namespace)
		return result
	}

	if exists && override {
		// Update existing flow
		updateResp, _, err := client.API.FlowsAPI.UpdateFlow(client.Ctx, flowID, namespace, client.Tenant).
			Body(yamlContent).
			Execute()
		if err != nil {
			result.Error = formatSDKError(err).Error()
			return result
		}
		result.Revision = updateResp.GetRevision()
	} else {
		// Create new flow
		flowResp, _, err := client.API.FlowsAPI.CreateFlow(client.Ctx, client.Tenant).
			Body(yamlContent).
			Execute()
		if err != nil {
			result.Error = formatSDKError(err).Error()
			return result
		}
		result.Revision = flowResp.GetRevision()
	}

	result.Success = true
	return result
}

// formatDeployResults outputs the deployment results in the appropriate format.
func formatDeployResults(results []DeployResult, singleFile bool) error {
	// Count successes and failures
	var successCount, failCount int
	for _, r := range results {
		if r.Success {
			successCount++
		} else {
			failCount++
		}
	}

	if globalFlags.Output == "json" {
		return printJSON(results)
	}

	// Table output
	w := tabWriter()
	fmt.Fprintln(w, "FILE\tFLOW ID\tNAMESPACE\tSTATUS\tERROR")
	for _, r := range results {
		status := "OK"
		errMsg := "-"
		if !r.Success {
			status = "FAILED"
			errMsg = r.Error
		}
		flowID := r.FlowID
		if flowID == "" {
			flowID = "-"
		}
		namespace := r.Namespace
		if namespace == "" {
			namespace = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", r.FilePath, flowID, namespace, status, errMsg)
	}
	w.Flush()

	// Print summary
	fmt.Printf("\n%d flow(s) deployed successfully, %d failed\n", successCount, failCount)

	if failCount > 0 {
		return fmt.Errorf("deployment completed with %d error(s)", failCount)
	}
	return nil
}

// parseFlowYAML extracts namespace and flow ID from YAML content.
func parseFlowYAML(content string) (string, string, error) {
	var payload map[string]any
	if err := yaml.Unmarshal([]byte(content), &payload); err != nil {
		return "", "", fmt.Errorf("invalid YAML content: %w", err)
	}

	rawNamespace, ok := payload["namespace"]
	if !ok {
		return "", "", errors.New("flow YAML must contain a 'namespace' field")
	}
	namespace, ok := rawNamespace.(string)
	if !ok || strings.TrimSpace(namespace) == "" {
		return "", "", errors.New("flow YAML must contain a valid 'namespace' field")
	}

	rawID, ok := payload["id"]
	if !ok {
		return "", "", errors.New("flow YAML must contain an 'id' field")
	}
	flowID, ok := rawID.(string)
	if !ok || strings.TrimSpace(flowID) == "" {
		return "", "", errors.New("flow YAML must contain a valid 'id' field")
	}

	return namespace, flowID, nil
}

// replaceNamespaceInYAML modifies the namespace field in YAML content.
func replaceNamespaceInYAML(content string, newNamespace string) (string, error) {
	var payload map[string]any
	if err := yaml.Unmarshal([]byte(content), &payload); err != nil {
		return "", fmt.Errorf("invalid YAML content: %w", err)
	}

	payload["namespace"] = newNamespace

	modified, err := yaml.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal modified YAML: %w", err)
	}

	return string(modified), nil
}
