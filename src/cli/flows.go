package cli

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	apiclient "github.com/kestra-io/kestra-cli/src/api_client"
	"github.com/spf13/cobra"
)

type flowsService interface {
	ListFlows(namespace, tenant string, ctx *apiclient.AuthContext) ([]map[string]any, error)
	GetFlow(namespace, flowID, tenant string, ctx *apiclient.AuthContext) (map[string]any, error)
	CreateFlow(yamlContent string, tenant string, ctx *apiclient.AuthContext, override bool) (map[string]any, error)
}

func newFlowsCommand() *cobra.Command {
	service := apiclient.NewFlowsAPI(newKestraClient())
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

			context := temporaryContext()
			if service == nil {
				return errors.New("flows service not configured")
			}

			flows, err := service.ListFlows(namespace, globalFlags.Tenant, context)
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

			context := temporaryContext()
			if service == nil {
				return errors.New("flows service not configured")
			}

			flow, err := service.GetFlow(namespace, flowID, globalFlags.Tenant, context)
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

			context := temporaryContext()
			if service == nil {
				return errors.New("flows service not configured")
			}

			flow, err := service.CreateFlow(string(content), globalFlags.Tenant, context, override)
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
