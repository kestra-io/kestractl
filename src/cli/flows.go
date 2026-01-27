package cli

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

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
	var override bool

	cmd := &cobra.Command{
		Use:   "deploy <filepath>",
		Short: "Deploy a flow from a YAML file.",
		Long: `Deploy a flow definition from a local YAML file to Kestra.

By default, the deployment will fail if the flow already exists.
Use --override to update an existing flow.`,
		Example: `  # Deploy a new flow
  kestra flows deploy flow.yaml

  # Deploy and override existing flow
  kestra flows deploy flow.yaml --override

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

			return runFlowsDeploy(client, args[0], override)
		},
	}

	cmd.Flags().BoolVar(&override, "override", false, "Override the flow if it already exists")

	return cmd
}

func runFlowsDeploy(client *Client, filepath string, override bool) error {
	content, err := os.ReadFile(filepath)
	if err != nil {
		return fmt.Errorf("failed to read file '%s': %w", filepath, err)
	}

	yamlContent := string(content)
	namespace, flowID, err := parseFlowYAML(yamlContent)
	if err != nil {
		return err
	}

	// Check if flow exists
	exists := false
	_, resp, checkErr := client.API.FlowsAPI.Flow(client.Ctx, namespace, flowID, client.Tenant).
		Source(false).
		AllowDeleted(false).
		Execute()
	if checkErr == nil {
		exists = true
	} else if resp != nil && resp.StatusCode != 404 {
		return formatSDKError(checkErr)
	}

	if exists && !override {
		return fmt.Errorf("flow '%s' already exists in namespace '%s'; use --override to update", flowID, namespace)
	}

	var result map[string]any
	if exists && override {
		// Update existing flow
		updateResp, _, err := client.API.FlowsAPI.UpdateFlow(client.Ctx, flowID, namespace, client.Tenant).
			Body(yamlContent).
			Execute()
		if err != nil {
			return formatSDKError(err)
		}
		result = map[string]any{
			"id":        updateResp.GetId(),
			"namespace": updateResp.GetNamespace(),
			"revision":  updateResp.GetRevision(),
		}
	} else {
		// Create new flow
		flowResp, _, err := client.API.FlowsAPI.CreateFlow(client.Ctx, client.Tenant).
			Body(yamlContent).
			Execute()
		if err != nil {
			return formatSDKError(err)
		}
		result = map[string]any{
			"id":        flowResp.GetId(),
			"namespace": flowResp.GetNamespace(),
			"revision":  flowResp.GetRevision(),
		}
	}

	if globalFlags.Output == "json" {
		return printJSON(result)
	}

	fmt.Println("Flow deployed successfully!")
	fmt.Printf("Flow ID: %s\n", stringify(result["id"]))
	fmt.Printf("Namespace: %s\n", stringify(result["namespace"]))
	fmt.Printf("Revision: %s\n", stringify(result["revision"]))

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
