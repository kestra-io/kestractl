package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	kestra "github.com/kestra-io/client-sdk/go-sdk"
	"github.com/spf13/cobra"
)

type executionsService interface {
	KillByQuery(ctx context.Context, state []string, namespace, flowID, tenant string) (map[string]any, error)
	TriggerExecution(ctx context.Context, namespace, flowID string, wait bool, inputs map[string]any, tenant string) (map[string]any, error)
	GetExecution(ctx context.Context, executionID, tenant string) (map[string]any, error)
}

// sdkExecutionsService implements executionsService using the Kestra SDK
type sdkExecutionsService struct {
	client  *kestra.APIClient
	authCtx context.Context
}

func (s *sdkExecutionsService) KillByQuery(ctx context.Context, state []string, namespace, flowID, tenant string) (map[string]any, error) {
	// Convert state strings to StateType
	stateTypes := make([]kestra.StateType, len(state))
	for i, st := range state {
		stateTypes[i] = kestra.StateType(st)
	}

	req := s.client.ExecutionsAPI.KillExecutionsByQuery(s.authCtx, tenant).
		DeleteExecutionsByQueryRequest(kestra.DeleteExecutionsByQueryRequest{}).
		State(stateTypes)

	if namespace != "" {
		req = req.Namespace(namespace)
	}
	if flowID != "" {
		req = req.FlowId(flowID)
	}

	result, _, err := req.Execute()
	if err != nil {
		return nil, formatSDKError(err)
	}

	return result, nil
}

func (s *sdkExecutionsService) TriggerExecution(ctx context.Context, namespace, flowID string, wait bool, inputs map[string]any, tenant string) (map[string]any, error) {
	resp, _, err := s.client.ExecutionsAPI.CreateExecution(s.authCtx, namespace, flowID, tenant).
		Wait(wait).
		Execute()
	if err != nil {
		// The SDK has a type mismatch issue with the inputs field.
		// If we get a JSON parsing error but the response body contains valid data,
		// try to extract the relevant fields from the raw body.
		if sdkErr, ok := err.(*kestra.GenericOpenAPIError); ok {
			body := sdkErr.Body()
			if len(body) > 0 {
				var rawResp map[string]any
				if jsonErr := json.Unmarshal(body, &rawResp); jsonErr == nil {
					// Successfully parsed the raw response
					result := map[string]any{}
					if id, ok := rawResp["id"]; ok {
						result["id"] = id
					}
					if flowId, ok := rawResp["flowId"]; ok {
						result["flowId"] = flowId
					}
					if ns, ok := rawResp["namespace"]; ok {
						result["namespace"] = ns
					}
					if state, ok := rawResp["state"].(map[string]any); ok {
						result["state"] = state
					}
					if url, ok := rawResp["url"]; ok {
						result["url"] = url
					}
					// If we got meaningful data, return it
					if _, hasId := result["id"]; hasId {
						return result, nil
					}
				}
			}
		}
		return nil, formatSDKError(err)
	}

	// Convert response to map
	result := map[string]any{
		"id":        resp.GetId(),
		"flowId":    resp.GetFlowId(),
		"namespace": resp.GetNamespace(),
	}

	state := resp.State
	stateMap := map[string]any{
		"current": state.GetCurrent(),
	}
	if !state.GetStartDate().IsZero() {
		stateMap["startDate"] = state.GetStartDate().Format(time.RFC3339)
	}
	if !state.GetEndDate().IsZero() {
		stateMap["endDate"] = state.GetEndDate().Format(time.RFC3339)
	}
	result["state"] = stateMap

	return result, nil
}

func (s *sdkExecutionsService) GetExecution(ctx context.Context, executionID, tenant string) (map[string]any, error) {
	resp, _, err := s.client.ExecutionsAPI.GetExecution(s.authCtx, executionID, tenant).Execute()
	if err != nil {
		// The SDK has type mismatch issues (e.g., SUBMITTED state not in enum).
		// Try to parse the raw response on error.
		if sdkErr, ok := err.(*kestra.GenericOpenAPIError); ok {
			body := sdkErr.Body()
			if len(body) > 0 {
				var rawResp map[string]any
				if jsonErr := json.Unmarshal(body, &rawResp); jsonErr == nil {
					result := map[string]any{}
					if id, ok := rawResp["id"]; ok {
						result["id"] = id
					}
					if flowId, ok := rawResp["flowId"]; ok {
						result["flowId"] = flowId
					}
					if ns, ok := rawResp["namespace"]; ok {
						result["namespace"] = ns
					}
					if flowRev, ok := rawResp["flowRevision"]; ok {
						result["flowRevision"] = flowRev
					}
					if state, ok := rawResp["state"].(map[string]any); ok {
						result["state"] = state
					}
					if labels, ok := rawResp["labels"].([]any); ok {
						result["labels"] = labels
					}
					// If we got meaningful data, return it
					if _, hasId := result["id"]; hasId {
						return result, nil
					}
				}
			}
		}
		return nil, formatSDKError(err)
	}

	result := map[string]any{
		"id":           resp.GetId(),
		"flowId":       resp.GetFlowId(),
		"namespace":    resp.GetNamespace(),
		"flowRevision": resp.GetFlowRevision(),
	}

	state := resp.State
	stateMap := map[string]any{
		"current": state.GetCurrent(),
	}
	if !state.GetStartDate().IsZero() {
		stateMap["startDate"] = state.GetStartDate().Format(time.RFC3339)
	}
	if !state.GetEndDate().IsZero() {
		stateMap["endDate"] = state.GetEndDate().Format(time.RFC3339)
	}
	if state.Duration != nil {
		stateMap["duration"] = *state.Duration
	}
	result["state"] = stateMap

	if labels := resp.GetLabels(); len(labels) > 0 {
		labelsSlice := make([]any, len(labels))
		for i, label := range labels {
			labelsSlice[i] = map[string]any{
				"key":   label.GetKey(),
				"value": label.GetValue(),
			}
		}
		result["labels"] = labelsSlice
	}

	return result, nil
}

func newExecutionsCommand() *cobra.Command {
	factory := newSDKClientFactory()
	client, authCtx, err := factory.createClient()
	if err != nil {
		return newExecutionsCommandWithService(nil)
	}
	service := &sdkExecutionsService{client: client, authCtx: authCtx}
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

			if service == nil {
				return errors.New("executions service not configured")
			}

			authCtx := temporaryContext()
			tenant := resolveTenant(authCtx)

			result, err := service.KillByQuery(context.Background(), []string{"RUNNING"}, namespace, flowID, tenant)
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

			if service == nil {
				return errors.New("executions service not configured")
			}

			authCtx := temporaryContext()
			tenant := resolveTenant(authCtx)

			if wait {
				fmt.Printf("Triggering execution of flow '%s' in namespace '%s'...\n", flowID, namespace)
				fmt.Println("Waiting for execution to complete...")
			}

			execution, err := service.TriggerExecution(context.Background(), namespace, flowID, wait, nil, tenant)
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

			if service == nil {
				return errors.New("executions service not configured")
			}

			authCtx := temporaryContext()
			tenant := resolveTenant(authCtx)

			execution, err := service.GetExecution(context.Background(), executionID, tenant)
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

			// Build URL from context if not in response
			if _, ok := execution["url"]; !ok {
				if authCtx != nil {
					hostValue := strings.TrimRight(authCtx.Host, "/")
					ns := stringify(execution["namespace"])
					flowID := stringify(execution["flowId"])
					id := stringify(execution["id"])
					if hostValue != "" && tenant != "" && ns != "" && flowID != "" && id != "" {
						url := fmt.Sprintf("%s/ui/%s/executions/%s/%s/%s", hostValue, tenant, ns, flowID, id)
						fmt.Printf("URL: %s\n", url)
					}
				}
			} else {
				fmt.Printf("URL: %v\n", execution["url"])
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
