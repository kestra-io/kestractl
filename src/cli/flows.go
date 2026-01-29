package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	kestra "github.com/kestra-io/client-sdk/go-sdk/kestra_api_client"
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

// ValidateResult holds the result of validating a single flow file.
type ValidateResult struct {
	FilePath         string   `json:"file_path"`
	FlowID           string   `json:"flow_id,omitempty"`
	Namespace        string   `json:"namespace,omitempty"`
	Constraints      []string `json:"constraints,omitempty"`
	Warnings         []string `json:"warnings,omitempty"`
	Infos            []string `json:"infos,omitempty"`
	DeprecationPaths []string `json:"deprecation_paths,omitempty"`
	Outdated         bool     `json:"outdated,omitempty"`
	Success          bool     `json:"success"`
}

func newFlowsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "flows",
		Short: "Manage flows",
	}

	cmd.AddCommand(newFlowsListCommand())
	cmd.AddCommand(newFlowsGetCommand())
	cmd.AddCommand(newFlowsDeployCommand())
	cmd.AddCommand(newFlowsValidateCommand())

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

func newFlowsValidateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "validate <path>",
		Short:        "Validate flows from a YAML file or directory.",
		SilenceUsage: true,
		Long: `Validate flow definitions from a local YAML file or directory.

When a directory is provided, all .yaml and .yml files are validated recursively.
Hidden files and directories (starting with .) are skipped.

Validation fails if any flow has constraint violations.
Warnings, infos, deprecations, and outdated flags are reported but do not fail validation.`,
		Example: `  # Validate a single flow
  kestra flows validate flow.yaml

  # Validate all flows in a directory (recursive)
  kestra flows validate ./flows/

  # Validate with JSON output
  kestra flows validate ./flows/ --output json`,
		Aliases: []string{"check"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutputFormat(); err != nil {
				return err
			}

			client, err := NewClient()
			if err != nil {
				return err
			}

			return runFlowsValidate(client, args[0])
		},
	}

	return cmd
}

func runFlowsValidate(client *Client, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to access path '%s': %w", path, err)
	}

	var files []string
	if info.IsDir() {
		files, err = collectFlowFiles(path)
		if err != nil {
			return fmt.Errorf("failed to scan directory '%s': %w", path, err)
		}
		if len(files) == 0 {
			return fmt.Errorf("no .yaml or .yml files found in directory '%s'", path)
		}
	} else {
		files = []string{path}
	}

	contents := make([]string, 0, len(files))
	for _, file := range files {
		data, readErr := os.ReadFile(file)
		if readErr != nil {
			return fmt.Errorf("failed to read file '%s': %w", file, readErr)
		}
		contents = append(contents, string(data))
	}

	body := strings.Join(contents, "\n---\n")

	violations, _, err := client.API.FlowsAPI.ValidateFlows(client.Ctx, client.Tenant).
		Body(body).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	return formatValidateResults(violations, files)
}

func formatValidateResults(violations []kestra.ValidateConstraintViolation, files []string) error {
	results := make([]ValidateResult, len(files))
	for i, file := range files {
		results[i] = ValidateResult{
			FilePath: file,
			Success:  true,
		}
	}

	unknownResults := make([]ValidateResult, 0)

	for _, violation := range violations {
		index := int(violation.GetIndex())
		var target *ValidateResult
		if index >= 0 && index < len(results) {
			target = &results[index]
		} else {
			unknownResults = append(unknownResults, ValidateResult{
				FilePath: fmt.Sprintf("<index %d>", index),
				Success:  true,
			})
			target = &unknownResults[len(unknownResults)-1]
		}

		if flowID := violation.GetFlow(); strings.TrimSpace(flowID) != "" {
			target.FlowID = flowID
		}
		if namespace := violation.GetNamespace(); strings.TrimSpace(namespace) != "" {
			target.Namespace = namespace
		}
		if constraint := strings.TrimSpace(violation.GetConstraints()); constraint != "" {
			constraint = strings.ReplaceAll(constraint, "\n", "; ")
			target.Constraints = appendUniqueString(target.Constraints, constraint)
		}
		if warnings := violation.GetWarnings(); len(warnings) > 0 {
			for _, warning := range warnings {
				warning = strings.ReplaceAll(strings.TrimSpace(warning), "\n", "; ")
				if warning != "" {
					target.Warnings = appendUniqueString(target.Warnings, warning)
				}
			}
		}
		if infos := violation.GetInfos(); len(infos) > 0 {
			for _, info := range infos {
				info = strings.ReplaceAll(strings.TrimSpace(info), "\n", "; ")
				if info != "" {
					target.Infos = appendUniqueString(target.Infos, info)
				}
			}
		}
		if deps := violation.GetDeprecationPaths(); len(deps) > 0 {
			for _, dep := range deps {
				dep = strings.TrimSpace(dep)
				if dep != "" {
					target.DeprecationPaths = appendUniqueString(target.DeprecationPaths, dep)
				}
			}
		}
		if violation.GetOutdated() {
			target.Outdated = true
		}
	}

	allResults := results
	if len(unknownResults) > 0 {
		allResults = append(allResults, unknownResults...)
	}

	failed := 0
	for i := range allResults {
		if len(allResults[i].Constraints) > 0 {
			allResults[i].Success = false
			failed++
		} else {
			allResults[i].Success = true
		}
	}

	if globalFlags.Output == "json" {
		return printJSON(allResults)
	}

	w := tabWriter()
	fmt.Fprintln(w, "FILE\tSTATUS\tCONSTRAINTS\tWARNINGS\tINFOS\tOUTDATED\tDEPRECATIONS")
	for _, result := range allResults {
		status := "OK"
		constraints := "-"
		if len(result.Constraints) > 0 {
			status = "FAILED"
			constraints = strings.Join(result.Constraints, "; ")
		}
		warnings := "-"
		if len(result.Warnings) > 0 {
			warnings = fmt.Sprintf("%d", len(result.Warnings))
		}
		infos := "-"
		if len(result.Infos) > 0 {
			infos = fmt.Sprintf("%d", len(result.Infos))
		}
		outdated := "false"
		if result.Outdated {
			outdated = "true"
		}
		deprecations := "-"
		if len(result.DeprecationPaths) > 0 {
			deprecations = fmt.Sprintf("%d", len(result.DeprecationPaths))
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			result.FilePath,
			status,
			constraints,
			warnings,
			infos,
			outdated,
			deprecations,
		)
	}
	w.Flush()

	valid := len(allResults) - failed
	fmt.Printf("\n%d flow(s) valid, %d failed\n", valid, failed)
	if failed > 0 {
		return fmt.Errorf("validation failed for %d flow(s)", failed)
	}
	return nil
}

func appendUniqueString(list []string, value string) []string {
	for _, existing := range list {
		if existing == value {
			return list
		}
	}
	return append(list, value)
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
		updateResp, _, err := client.API.FlowsAPI.UpdateFlow(client.Ctx, namespace, flowID, client.Tenant).
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
