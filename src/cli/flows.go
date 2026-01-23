package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	kestra "github.com/kestra-io/client-sdk/go-sdk/kestra_api_client"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type flowsService interface {
	ListFlows(ctx context.Context, namespace, tenant string) ([]map[string]any, error)
	GetFlow(ctx context.Context, namespace, flowID, tenant string) (map[string]any, error)
	CreateFlow(ctx context.Context, yamlContent, tenant string, override bool) (map[string]any, error)
}

// sdkFlowsService implements flowsService using the Kestra SDK
type sdkFlowsService struct {
	client  *kestra.APIClient
	authCtx context.Context
}

func (s *sdkFlowsService) ListFlows(ctx context.Context, namespace, tenant string) ([]map[string]any, error) {
	flows, _, err := s.client.FlowsAPI.ListFlowsByNamespace(s.authCtx, namespace, tenant).Execute()
	if err != nil {
		return nil, formatSDKError(err)
	}

	result := make([]map[string]any, len(flows))
	for i, flow := range flows {
		result[i] = map[string]any{
			"id":          flow.GetId(),
			"namespace":   flow.GetNamespace(),
			"description": flow.GetDescription(),
			"revision":    flow.GetRevision(),
		}
	}
	return result, nil
}

func (s *sdkFlowsService) GetFlow(ctx context.Context, namespace, flowID, tenant string) (map[string]any, error) {
	flow, _, err := s.client.FlowsAPI.Flow(s.authCtx, namespace, flowID, tenant).
		Source(true).
		AllowDeleted(false).
		Execute()
	if err != nil {
		return nil, formatSDKError(err)
	}
	result := map[string]any{
		"id":        flow.GetId(),
		"namespace": flow.GetNamespace(),
		"revision":  flow.GetRevision(),
	}
	return result, nil
}

func (s *sdkFlowsService) CreateFlow(ctx context.Context, yamlContent, tenant string, override bool) (map[string]any, error) {
	namespace, flowID, err := parseFlowYAML(yamlContent)
	if err != nil {
		return nil, err
	}

	// Check if flow exists
	exists := false
	_, resp, err := s.client.FlowsAPI.Flow(s.authCtx, namespace, flowID, tenant).
		Source(false).
		AllowDeleted(false).
		Execute()
	if err == nil {
		exists = true
	} else if resp != nil && resp.StatusCode != 404 {
		return nil, formatSDKError(err)
	}

	if exists && !override {
		return nil, fmt.Errorf("flow '%s' already exists in namespace '%s'; use --override to update", flowID, namespace)
	}

	var result map[string]any
	if exists && override {
		// Update existing flow
		updateResp, _, err := s.client.FlowsAPI.UpdateFlow(s.authCtx, flowID, namespace, tenant).
			Body(yamlContent).
			Execute()
		if err != nil {
			return nil, formatSDKError(err)
		}
		// Convert response to map
		if updateResp != nil {
			result = map[string]any{
				"id":        updateResp.GetId(),
				"namespace": updateResp.GetNamespace(),
				"revision":  updateResp.GetRevision(),
			}
		}
	} else {
		// Create new flow
		flowResp, _, err := s.client.FlowsAPI.CreateFlow(s.authCtx, tenant).
			Body(yamlContent).
			Execute()
		if err != nil {
			return nil, formatSDKError(err)
		}
		result = map[string]any{
			"id":        flowResp.GetId(),
			"namespace": flowResp.GetNamespace(),
			"revision":  flowResp.GetRevision(),
		}
	}

	return result, nil
}

// parseFlowYAML extracts namespace and flow ID from YAML content
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

// formatSDKError formats SDK errors for display
func formatSDKError(err error) error {
	if sdkErr, ok := err.(*kestra.GenericOpenAPIError); ok {
		return fmt.Errorf("API error: %s - %s", sdkErr.Error(), string(sdkErr.Body()))
	}
	return err
}

func newFlowsCommand() *cobra.Command {
	factory := newSDKClientFactory()
	client, authCtx, err := factory.createClient()
	if err != nil {
		// Return command that will error on execution
		return newFlowsCommandWithService(nil)
	}
	service := &sdkFlowsService{client: client, authCtx: authCtx}
	return newFlowsCommandWithService(service)
}

func newFlowsCommandWithService(service flowsService) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "flows",
		Short: "Manage flows",
	}

	cmd.AddCommand(newFlowsListCommand(service))
	cmd.AddCommand(newFlowsGetCommand(service))
	cmd.AddCommand(newFlowsDeployCommand(service))

	return cmd
}

func newFlowsListCommand(service flowsService) *cobra.Command {
	cmd := &cobra.Command{
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
			namespace := args[0]

			if err := validateOutputFormat(); err != nil {
				return err
			}

			if service == nil {
				return errors.New("flows service not configured")
			}

			authCtx := temporaryContext()
			tenant := resolveTenant(authCtx)

			flows, err := service.ListFlows(context.Background(), namespace, tenant)
			if err != nil {
				return err
			}

			if globalFlags.Output == "json" {
				return printJSON(flows)
			}

			w := tabWriter()
			fmt.Fprintln(w, "ID\tNamespace\tDescription\tRevision")
			for _, flow := range flows {
				id := stringify(flow["id"])
				ns := stringify(flow["namespace"])
				desc := stringify(flow["description"])
				if desc == "" {
					desc = "-"
				}
				rev := stringify(flow["revision"])
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", id, ns, desc, rev)
			}
			w.Flush()

			return nil
		},
	}

	return cmd
}

func newFlowsGetCommand(service flowsService) *cobra.Command {
	cmd := &cobra.Command{
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
			namespace := args[0]
			flowID := args[1]

			if err := validateOutputFormat(); err != nil {
				return err
			}

			if service == nil {
				return errors.New("flows service not configured")
			}

			authCtx := temporaryContext()
			tenant := resolveTenant(authCtx)

			flow, err := service.GetFlow(context.Background(), namespace, flowID, tenant)
			if err != nil {
				return err
			}

			if globalFlags.Output == "json" {
				return printJSON(flow)
			}

			w := tabWriter()
			fmt.Fprintln(w, "Property\tValue")

			keys := make([]string, 0, len(flow))
			for key := range flow {
				keys = append(keys, key)
			}
			sort.Strings(keys)

			for _, key := range keys {
				val := toPrettyString(flow[key])
				val = strings.ReplaceAll(val, "\n", "\\n")
				fmt.Fprintf(w, "%s\t%s\n", key, val)
			}
			w.Flush()

			return nil
		},
	}

	return cmd
}

func newFlowsDeployCommand(service flowsService) *cobra.Command {
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
			filepath := args[0]

			if err := validateOutputFormat(); err != nil {
				return err
			}

			content, err := os.ReadFile(filepath)
			if err != nil {
				return fmt.Errorf("failed to read file '%s': %w", filepath, err)
			}

			if service == nil {
				return errors.New("flows service not configured")
			}

			authCtx := temporaryContext()
			tenant := resolveTenant(authCtx)

			flow, err := service.CreateFlow(context.Background(), string(content), tenant, override)
			if err != nil {
				return err
			}

			if globalFlags.Output == "json" {
				return printJSON(flow)
			}

			fmt.Println("Flow deployed successfully!")
			fmt.Printf("Flow ID: %s\n", stringify(flow["id"]))
			fmt.Printf("Namespace: %s\n", stringify(flow["namespace"]))
			fmt.Printf("Revision: %s\n", stringify(flow["revision"]))

			return nil
		},
	}

	cmd.Flags().BoolVar(&override, "override", false, "Override the flow if it already exists")

	return cmd
}
