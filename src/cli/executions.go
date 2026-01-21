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

type executionsService interface {
	KillByQuery(state []string, namespace, flowID, tenant string, ctx *apiclient.AuthContext) (map[string]any, error)
	TriggerExecution(namespace, flowID string, wait bool, inputs map[string]any, tenant string, ctx *apiclient.AuthContext) (map[string]any, error)
	GetExecution(executionID, tenant string, ctx *apiclient.AuthContext) (map[string]any, error)
}

func newExecutionsCommand() *cobra.Command {
	service := apiclient.NewExecutionsAPI(newKestraClient())
	return newExecutionsCommandWithService(service)
}

func newExecutionsCommandWithService(service executionsService) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "executions",
		Short: "Manage executions",
	}

	cmd.AddCommand(newExecutionsKillCommand(service))
	cmd.AddCommand(newExecutionsRunCommand(service))
	cmd.AddCommand(newExecutionsGetCommand(service))

	return cmd
}

func newExecutionsKillCommand(service executionsService) *cobra.Command {
	var namespace string
	var flowID string

	cmd := &cobra.Command{
		Use:   "kill-running",
		Short: "Kill executions in RUNNING state.",
		Long: `Kill all running executions, optionally filtered by namespace and flow ID.

This command sends a kill request to all executions currently in RUNNING state.
Use the --namespace and --flow-id flags to target specific executions.`,
		Example: `  # Kill all running executions
  kestra executions kill-running

  # Kill running executions in a specific namespace
  kestra executions kill-running --namespace my.namespace

  # Kill running executions for a specific flow
  kestra executions kill-running --namespace my.namespace --flow-id my-flow`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutputFormat(); err != nil {
				return err
			}

			if flowID != "" && namespace == "" {
				return errors.New("--namespace is required when --flow-id is provided")
			}

			context := temporaryContext()
			if service == nil {
				return errors.New("executions service not configured")
			}

			result, err := service.KillByQuery([]string{"RUNNING"}, namespace, flowID, globalFlags.Tenant, context)
			if err != nil {
				return err
			}

			if globalFlags.Output == "json" {
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

	return cmd
}

func newExecutionsRunCommand(service executionsService) *cobra.Command {
	var wait bool

	cmd := &cobra.Command{
		Use:   "run <namespace> <flow_id>",
		Short: "Trigger a flow execution.",
		Long: `Trigger a flow execution in the specified namespace.

The command returns immediately by default. Use --wait to poll until
the execution completes (SUCCESS, FAILED, or other terminal state).`,
		Example: `  # Trigger a flow
  kestra executions run my.namespace my-flow

  # Trigger and wait for completion
  kestra executions run my.namespace my-flow --wait

  # Get JSON output
  kestra executions run my.namespace my-flow --output json`,
		Aliases: []string{"trigger", "execute"},
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace := args[0]
			flowID := args[1]

			if err := validateOutputFormat(); err != nil {
				return err
			}

			context := temporaryContext()
			if service == nil {
				return errors.New("executions service not configured")
			}

			if wait {
				fmt.Printf("Triggering execution of flow '%s' in namespace '%s'...\n", flowID, namespace)
				fmt.Println("Waiting for execution to complete...")
			}

			execution, err := service.TriggerExecution(namespace, flowID, wait, nil, globalFlags.Tenant, context)
			if err != nil {
				return err
			}

			if globalFlags.Output == "json" {
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

	cmd.Flags().BoolVarP(&wait, "wait", "w", false, "Wait for execution to complete")

	return cmd
}

func newExecutionsGetCommand(service executionsService) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <execution_id>",
		Short: "Get execution details.",
		Long: `Retrieve detailed information about a specific execution.

The command displays execution status, timing, labels, and other metadata.`,
		Example: `  # Get execution details
  kestra executions get 2TLGqHrXC9k8BczKJe5djX

  # Get execution details as JSON
  kestra executions get 2TLGqHrXC9k8BczKJe5djX --output json`,
		Aliases: []string{"show", "describe"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			executionID := args[0]

			if err := validateOutputFormat(); err != nil {
				return err
			}

			context := temporaryContext()
			if service == nil {
				return errors.New("executions service not configured")
			}

			execution, err := service.GetExecution(executionID, globalFlags.Tenant, context)
			if err != nil {
				return err
			}

			if globalFlags.Output == "json" {
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
				client := newKestraClient()
				resolvedCtx, err := client.ResolveContext(context)
				if err == nil {
					tenantValue := globalFlags.Tenant
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
