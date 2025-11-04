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
	var host string
	var tenant string
	var token string
	var output string

	cmd := &cobra.Command{
		Use:   "list <namespace>",
		Short: "List flows in a namespace.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace := args[0]
			output = strings.ToLower(output)
			if output != "table" && output != "json" {
				return errors.New("output must be 'table' or 'json'")
			}

			client := newKestraClient()
			context := temporaryContext(host, tenant, token)
			api := apiclient.NewFlowsAPI(client)

			flows, err := api.ListFlows(namespace, tenant, context)
			if err != nil {
				return err
			}

			if output == "json" {
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

	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant name")
	cmd.Flags().StringVar(&host, "host", "", "Kestra host URL")
	cmd.Flags().StringVarP(&token, "token", "t", "", "API token")
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format (table or json)")

	return cmd
}

func newFlowsGetCommand() *cobra.Command {
	var host string
	var tenant string
	var token string
	var output string

	cmd := &cobra.Command{
		Use:   "get <namespace> <flow_id>",
		Short: "Get a specific flow.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace := args[0]
			flowID := args[1]
			output = strings.ToLower(output)
			if output != "table" && output != "json" {
				return errors.New("output must be 'table' or 'json'")
			}

			client := newKestraClient()
			context := temporaryContext(host, tenant, token)
			api := apiclient.NewFlowsAPI(client)

			flow, err := api.GetFlow(namespace, flowID, tenant, context)
			if err != nil {
				return err
			}

			if output == "json" {
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

	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant name")
	cmd.Flags().StringVar(&host, "host", "", "Kestra host URL")
	cmd.Flags().StringVarP(&token, "token", "t", "", "API token")
	cmd.Flags().StringVarP(&output, "output", "o", "json", "Output format (table or json)")

	return cmd
}

func newFlowsDeployCommand() *cobra.Command {
	var host string
	var tenant string
	var token string
	var override bool
	var output string

	cmd := &cobra.Command{
		Use:   "deploy <filepath>",
		Short: "Deploy a flow from a YAML file.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filepath := args[0]
			output = strings.ToLower(output)
			if output != "table" && output != "json" {
				return errors.New("output must be 'table' or 'json'")
			}

			content, err := os.ReadFile(filepath)
			if err != nil {
				return fmt.Errorf("failed to read file '%s': %w", filepath, err)
			}

			client := newKestraClient()
			context := temporaryContext(host, tenant, token)
			api := apiclient.NewFlowsAPI(client)

			flow, err := api.CreateFlow(string(content), tenant, context, override)
			if err != nil {
				return err
			}

			if output == "json" {
				return printJSON(flow)
			}

			fmt.Println("Flow deployed successfully!")
			fmt.Printf("Flow ID: %s\n", stringify(flow["id"]))
			fmt.Printf("Namespace: %s\n", stringify(flow["namespace"]))
			fmt.Printf("Revision: %s\n", stringify(flow["revision"]))

			return nil
		},
	}

	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant name")
	cmd.Flags().StringVar(&host, "host", "", "Kestra host URL")
	cmd.Flags().StringVarP(&token, "token", "t", "", "API token")
	cmd.Flags().BoolVar(&override, "override", false, "Override the flow if it already exists")
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format (table or json)")

	return cmd
}
