package cli

import (
	"fmt"
	"io"
	"math"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

func newExecutionsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "executions",
		Short: "Manage executions (start, list, cancel, delete)",
	}

	cmd.AddCommand(newExecutionsRunCommand())
	cmd.AddCommand(newExecutionsGetCommand())

	return cmd
}

func newExecutionsRunCommand() *cobra.Command {
	var wait bool

	cmd := &cobra.Command{
		Use:   "run <namespace> <flow_id>",
		Short: "Trigger a flow execution.",
		Long: `Trigger a flow execution in the specified namespace.

The command returns immediately by default. Use --wait to poll until
the execution completes (SUCCESS, FAILED, or other terminal state).`,
		Example: `  # Trigger a flow
	  kestractl executions run my.namespace my-flow

	  # Trigger and wait for completion
	  kestractl executions run my.namespace my-flow --wait

	  # Get JSON output
	  kestractl executions run my.namespace my-flow --output json`,
		Aliases: []string{"trigger", "execute"},
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := NewClient()
			if err != nil {
				return err
			}

			return runExecutionsRun(client, args[0], args[1], wait, renderer)
		},
	}

	cmd.Flags().BoolVarP(&wait, "wait", "w", false, "Wait for execution to complete")

	return cmd
}

func runExecutionsRun(client *Client, namespace, flowID string, wait bool, renderer *Renderer) error {
	if wait {
		fmt.Fprintf(renderer.Writer(), "Triggering execution of flow '%s' in namespace '%s'...\n", flowID, namespace)
		fmt.Fprintln(renderer.Writer(), "Waiting for execution to complete...")
	}

	resp, _, err := client.API.ExecutionsAPI.CreateExecution(client.Ctx, namespace, flowID, client.Tenant).
		Wait(wait).
		Execute()

	// Handle SDK type mismatch bugs
	var execution map[string]any
	if err != nil {
		execution = tryParseExecutionFromError(err)
		if execution == nil {
			return formatSDKError(err)
		}
	} else {
		execution = map[string]any{
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
		execution["state"] = stateMap
	}

	return renderer.Render(execution, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "Execution triggered successfully!")
		fmt.Fprintln(w)
		fmt.Fprintf(w, "Execution ID: %s\n", stringify(execution["id"]))
		fmt.Fprintf(w, "Flow: %s\n", stringify(execution["flowId"]))
		fmt.Fprintf(w, "Namespace: %s\n", stringify(execution["namespace"]))
		printExecutionState(w, execution, wait)

		if url, ok := execution["url"]; ok {
			fmt.Fprintf(w, "URL: %v\n", url)
		}
		return nil
	})
}

func newExecutionsGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "get <execution_id>",
		Short: "Get execution details.",
		Long: `Retrieve detailed information about a specific execution.

The command displays execution status, timing, labels, and other metadata.`,
		Example: `  # Get execution details
	  kestractl executions get 2TLGqHrXC9k8BczKJe5djX

	  # Get execution details as JSON
	  kestractl executions get 2TLGqHrXC9k8BczKJe5djX --output json`,
		Aliases: []string{"show", "describe"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := NewClient()
			if err != nil {
				return err
			}

			return runExecutionsGet(client, args[0], renderer)
		},
	}
}

func runExecutionsGet(client *Client, executionID string, renderer *Renderer) error {
	resp, _, err := client.API.ExecutionsAPI.Execution(client.Ctx, executionID, client.Tenant).Execute()

	// Handle SDK type mismatch bugs
	var execution map[string]any
	if err != nil {
		execution = tryParseExecutionFromError(err)
		if execution == nil {
			return formatSDKError(err)
		}
	} else {
		execution = map[string]any{
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
		stateMap["duration"] = state.GetDuration()
		execution["state"] = stateMap

		if labels := resp.GetLabels(); len(labels) > 0 {
			labelsSlice := make([]any, len(labels))
			for i, label := range labels {
				labelsSlice[i] = map[string]any{
					"key":   label.GetKey(),
					"value": label.GetValue(),
				}
			}
			execution["labels"] = labelsSlice
		}
	}

	return renderer.Render(execution, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "Execution Details")
		fmt.Fprintln(w)
		fmt.Fprintf(w, "Execution ID: %s\n", stringify(execution["id"]))
		fmt.Fprintf(w, "Flow: %s\n", stringify(execution["flowId"]))
		fmt.Fprintf(w, "Namespace: %s\n", stringify(execution["namespace"]))
		fmt.Fprintf(w, "Flow Revision: %s\n", stringify(execution["flowRevision"]))
		printExecutionState(w, execution, true)

		if labels, ok := execution["labels"].([]any); ok && len(labels) > 0 {
			fmt.Fprintln(w, "Labels:")
			for _, raw := range labels {
				if label, ok := raw.(map[string]any); ok {
					fmt.Fprintf(w, "  - %s: %s\n", stringify(label["key"]), stringify(label["value"]))
				}
			}
		}
		return nil
	})
}

func printExecutionState(w io.Writer, execution map[string]any, includeTiming bool) {
	stateValue, ok := execution["state"].(map[string]any)
	if !ok {
		fmt.Fprintln(w, "State: unknown")
		return
	}

	if current, ok := stateValue["current"]; ok {
		fmt.Fprintf(w, "State: %v\n", current)
	}

	if startDate, ok := stateValue["startDate"]; ok {
		fmt.Fprintf(w, "Started: %v\n", startDate)
	} else if startDate, ok := execution["startDate"]; ok {
		fmt.Fprintf(w, "Started: %v\n", startDate)
	}

	if includeTiming {
		if endDate, ok := stateValue["endDate"]; ok {
			fmt.Fprintf(w, "Ended: %v\n", endDate)
		} else if endDate, ok := execution["endDate"]; ok {
			fmt.Fprintf(w, "Ended: %v\n", endDate)
		}

		if duration, ok := stateValue["duration"]; ok {
			fmt.Fprintf(w, "Duration: %s\n", formatDuration(duration))
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
