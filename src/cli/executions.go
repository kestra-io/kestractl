package cli

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	apiclient "github.com/kestra-io/kestra-cli/src/api_client"
	"github.com/spf13/cobra"
)

func newExecutionsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "executions",
		Short: "Manage executions",
	}

	cmd.AddCommand(newExecutionsKillCommand())
	cmd.AddCommand(newExecutionsRunCommand())
	cmd.AddCommand(newExecutionsGetCommand())

	return cmd
}

func newExecutionsKillCommand() *cobra.Command {
	var namespace string
	var flowID string
	var tenant string
	var host string
	var token string
	var output string

	cmd := &cobra.Command{
		Use:   "kill-running",
		Short: "Kill executions in RUNNING state.",
		RunE: func(cmd *cobra.Command, args []string) error {
			output = strings.ToLower(output)
			if output != "table" && output != "json" {
				return errors.New("output must be 'table' or 'json'")
			}

			if flowID != "" && namespace == "" {
				return errors.New("--namespace is required when --flow-id is provided")
			}

			client := newKestraClient()
			context := temporaryContext(host, tenant, token)
			api := apiclient.NewExecutionsAPI(client)

			result, err := api.KillByQuery([]string{"RUNNING"}, namespace, flowID, tenant, context)
			if err != nil {
				return err
			}

			if output == "json" {
				return printJSON(result)
			}

			fmt.Println("Kill request sent successfully!")
			if namespace != "" || flowID != "" {
				filters := []string{}
				if namespace != "" {
					filters = append(filters, fmt.Sprintf("namespace: %s", namespace))
				}
				if flowID != "" {
					filters = append(filters, fmt.Sprintf("flow ID: %s", flowID))
				}
				fmt.Printf("Filters: %s\n", strings.Join(filters, ", "))
			} else {
				fmt.Println("Filters: none (all running executions)")
			}
			fmt.Println("State: RUNNING")

			if count, ok := result["count"]; ok {
				fmt.Printf("Executions killed: %v\n", count)
			} else if message, ok := result["message"]; ok {
				fmt.Printf("Message: %v\n", message)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Filter by namespace")
	cmd.Flags().StringVarP(&flowID, "flow-id", "f", "", "Filter by flow ID (requires --namespace)")
	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant name")
	cmd.Flags().StringVar(&host, "host", "", "Kestra host URL")
	cmd.Flags().StringVarP(&token, "token", "t", "", "API token")
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format (table or json)")

	return cmd
}

func newExecutionsRunCommand() *cobra.Command {
	var tenant string
	var host string
	var token string
	var wait bool
	var output string

	cmd := &cobra.Command{
		Use:   "run <namespace> <flow_id>",
		Short: "Trigger a flow execution.",
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
			api := apiclient.NewExecutionsAPI(client)

			if wait {
				fmt.Printf("Triggering execution of flow '%s' in namespace '%s'...\n", flowID, namespace)
				fmt.Println("Waiting for execution to complete...")
			}

			execution, err := api.TriggerExecution(namespace, flowID, wait, nil, tenant, context)
			if err != nil {
				return err
			}

			if output == "json" {
				return printJSON(execution)
			}

			fmt.Println("Execution triggered successfully!")
			fmt.Println()
			fmt.Printf("Execution ID: %s\n", stringify(execution["id"]))
			fmt.Printf("Flow: %s\n", stringify(execution["flowId"]))
			fmt.Printf("Namespace: %s\n", stringify(execution["namespace"]))
			printExecutionState(execution, wait)

			if url, ok := execution["url"]; ok {
				fmt.Printf("URL: %v\n", url)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant name")
	cmd.Flags().StringVar(&host, "host", "", "Kestra host URL")
	cmd.Flags().StringVarP(&token, "token", "t", "", "API token")
	cmd.Flags().BoolVarP(&wait, "wait", "w", false, "Wait for execution to complete")
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format (table or json)")

	return cmd
}

func newExecutionsGetCommand() *cobra.Command {
	var tenant string
	var host string
	var token string
	var output string

	cmd := &cobra.Command{
		Use:   "get <execution_id>",
		Short: "Get execution details.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			executionID := args[0]
			output = strings.ToLower(output)
			if output != "table" && output != "json" {
				return errors.New("output must be 'table' or 'json'")
			}

			client := newKestraClient()
			context := temporaryContext(host, tenant, token)
			api := apiclient.NewExecutionsAPI(client)

			execution, err := api.GetExecution(executionID, tenant, context)
			if err != nil {
				return err
			}

			if output == "json" {
				return printJSON(execution)
			}

			fmt.Println("Execution Details")
			fmt.Println()
			fmt.Printf("Execution ID: %s\n", stringify(execution["id"]))
			fmt.Printf("Flow: %s\n", stringify(execution["flowId"]))
			fmt.Printf("Namespace: %s\n", stringify(execution["namespace"]))
			fmt.Printf("Flow Revision: %s\n", stringify(execution["flowRevision"]))
			printExecutionState(execution, true)

			if labels, ok := execution["labels"].([]any); ok && len(labels) > 0 {
				fmt.Println("Labels:")
				for _, raw := range labels {
					if label, ok := raw.(map[string]any); ok {
						fmt.Printf("  - %s: %s\n", stringify(label["key"]), stringify(label["value"]))
					}
				}
			}

			if urlValue, ok := execution["url"]; ok {
				fmt.Printf("URL: %v\n", urlValue)
			} else {
				resolvedCtx, err := client.ResolveContext(context)
				if err == nil {
					tenantValue := tenant
					if tenantValue == "" && resolvedCtx != nil {
						tenantValue = resolvedCtx.Tenant
					}
					hostValue := ""
					if resolvedCtx != nil {
						hostValue = strings.TrimRight(resolvedCtx.Host, "/")
					}
					ns := stringify(execution["namespace"])
					flowID := stringify(execution["flowId"])
					id := stringify(execution["id"])
					if hostValue != "" && tenantValue != "" && ns != "" && flowID != "" && id != "" {
						url := fmt.Sprintf("%s/ui/%s/executions/%s/%s/%s", hostValue, tenantValue, ns, flowID, id)
						fmt.Printf("URL: %s\n", url)
					}
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant name")
	cmd.Flags().StringVar(&host, "host", "", "Kestra host URL")
	cmd.Flags().StringVarP(&token, "token", "t", "", "API token")
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format (table or json)")

	return cmd
}

func printExecutionState(execution map[string]any, includeTiming bool) {
	stateValue, ok := execution["state"].(map[string]any)
	if !ok {
		fmt.Println("State: unknown")
		return
	}

	if current, ok := stateValue["current"]; ok {
		fmt.Printf("State: %v\n", current)
	}

	if startDate, ok := stateValue["startDate"]; ok {
		fmt.Printf("Started: %v\n", startDate)
	} else if startDate, ok := execution["startDate"]; ok {
		fmt.Printf("Started: %v\n", startDate)
	}

	if includeTiming {
		if endDate, ok := stateValue["endDate"]; ok {
			fmt.Printf("Ended: %v\n", endDate)
		} else if endDate, ok := execution["endDate"]; ok {
			fmt.Printf("Ended: %v\n", endDate)
		}

		if duration, ok := stateValue["duration"]; ok {
			fmt.Printf("Duration: %s\n", formatDuration(duration))
		}
	}
}

func formatDuration(raw any) string {
	switch v := raw.(type) {
	case string:
		if strings.HasPrefix(v, "PT") {
			if dur, err := parseISO8601Duration(v); err == nil {
				return dur
			}
		}
		return v
	case float64:
		return fmt.Sprintf("%.2fs", v/1000)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func parseISO8601Duration(value string) (string, error) {
	// Handle simple PTxS format where x is seconds (possibly fractional).
	if !strings.HasPrefix(value, "PT") || !strings.HasSuffix(value, "S") {
		return "", fmt.Errorf("unsupported duration format")
	}

	seconds := strings.TrimSuffix(strings.TrimPrefix(value, "PT"), "S")
	if seconds == "" {
		return "", fmt.Errorf("invalid duration")
	}

	dur, err := time.ParseDuration(seconds + "s")
	if err != nil {
		return "", err
	}

	rounded := math.Round(dur.Seconds()*100) / 100
	return fmt.Sprintf("%.2fs", rounded), nil
}
