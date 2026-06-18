package cli

import (
	"fmt"
	"io"
	"math"
	"strings"
	"text/tabwriter"
	"time"

	kestra "github.com/kestra-io/client-sdk/go-sdk/kestra_api_client"
	"github.com/spf13/cobra"
)

func newExecutionsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "executions",
		Short: "Manage executions (start, list, cancel, delete)",
	}

	cmd.AddCommand(newExecutionsRunCommand())
	cmd.AddCommand(newExecutionsGetCommand())
	cmd.AddCommand(newExecutionsListCommand())
	cmd.AddCommand(newExecutionsKillCommand())
	cmd.AddCommand(newExecutionsRestartCommand())
	cmd.AddCommand(newExecutionsResumeCommand())
	cmd.AddCommand(newExecutionsPauseCommand())

	return cmd
}

func newExecutionsPauseCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pause <execution_id>",
		Short: "Pause a running execution.",
		Long:  `Pause a running execution. Paused executions can later be resumed.`,
		Example: `  # Pause a running execution
	  kestractl executions pause 2TLGqHrXC9k8BczKJe5djX`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := NewClient()
			if err != nil {
				return err
			}

			return runExecutionsPause(client, args[0], renderer)
		},
	}

	return cmd
}

func runExecutionsPause(client *Client, executionID string, renderer *Renderer) error {
	_, err := client.API.ExecutionsAPI.PauseExecution(client.Ctx, executionID, client.Tenant).Execute()
	if err != nil {
		return formatSDKError(err)
	}

	result := map[string]any{
		"id":     executionID,
		"status": "paused",
	}
	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Execution '%s' paused\n", executionID)
		return nil
	})
}

func newExecutionsResumeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resume <execution_id>",
		Short: "Resume a paused execution.",
		Long:  `Resume an execution that is currently paused, continuing its tasks.`,
		Example: `  # Resume a paused execution
	  kestractl executions resume 2TLGqHrXC9k8BczKJe5djX`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := NewClient()
			if err != nil {
				return err
			}

			return runExecutionsResume(client, args[0], renderer)
		},
	}

	return cmd
}

func runExecutionsResume(client *Client, executionID string, renderer *Renderer) error {
	_, _, err := client.API.ExecutionsAPI.ResumeExecution(client.Ctx, executionID, client.Tenant).Execute()
	if err != nil {
		return formatSDKError(err)
	}

	result := map[string]any{
		"id":     executionID,
		"status": "resumed",
	}
	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Execution '%s' resumed\n", executionID)
		return nil
	})
}

// executionToMap converts an SDK Execution into the map shape used for both
// JSON output and table rendering across the execution action commands.
func executionToMap(exec *kestra.Execution) map[string]any {
	result := map[string]any{
		"id":           exec.GetId(),
		"flowId":       exec.GetFlowId(),
		"namespace":    exec.GetNamespace(),
		"flowRevision": exec.GetFlowRevision(),
	}
	st := exec.GetState()
	stateMap := map[string]any{"current": st.GetCurrent()}
	if !st.GetStartDate().IsZero() {
		stateMap["startDate"] = st.GetStartDate().Format(time.RFC3339)
	}
	stateMap["duration"] = st.GetDuration()
	result["state"] = stateMap
	return result
}

// renderExecutionResult renders a single execution returned by an action
// command (restart, resume, force-run, ...) with a leading status headline.
func renderExecutionResult(renderer *Renderer, exec *kestra.Execution, headline string) error {
	result := executionToMap(exec)
	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, headline)
		fmt.Fprintf(w, "Execution ID: %s\n", stringify(result["id"]))
		fmt.Fprintf(w, "Flow: %s\n", stringify(result["flowId"]))
		fmt.Fprintf(w, "Namespace: %s\n", stringify(result["namespace"]))
		printExecutionState(w, result, true)
		return nil
	})
}

func newExecutionsRestartCommand() *cobra.Command {
	var revision int32

	cmd := &cobra.Command{
		Use:   "restart <execution_id>",
		Short: "Restart a failed or terminated execution.",
		Long: `Restart an execution from its failed or terminated tasks.

By default the execution is restarted on the same flow revision. Use
--revision to restart against a specific flow revision.`,
		Example: `  # Restart an execution
	  kestractl executions restart 2TLGqHrXC9k8BczKJe5djX

	  # Restart against a specific flow revision
	  kestractl executions restart 2TLGqHrXC9k8BczKJe5djX --revision 3`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := NewClient()
			if err != nil {
				return err
			}

			return runExecutionsRestart(client, args[0], cmd.Flags().Changed("revision"), revision, renderer)
		},
	}

	cmd.Flags().Int32Var(&revision, "revision", 0, "Flow revision to restart against")

	return cmd
}

func runExecutionsRestart(client *Client, executionID string, hasRevision bool, revision int32, renderer *Renderer) error {
	req := client.API.ExecutionsAPI.RestartExecution(client.Ctx, executionID, client.Tenant)
	if hasRevision {
		req = req.Revision(revision)
	}

	exec, _, err := req.Execute()
	if err != nil {
		return formatSDKError(err)
	}
	if exec == nil {
		return fmt.Errorf("restart returned no execution")
	}

	return renderExecutionResult(renderer, exec, fmt.Sprintf("Execution '%s' restarted", executionID))
}

func newExecutionsKillCommand() *cobra.Command {
	var cascade bool

	cmd := &cobra.Command{
		Use:   "kill <execution_id>",
		Short: "Kill a running execution.",
		Long: `Kill a running execution, stopping all of its tasks.

By default, killing an execution also kills its subflow executions. Pass
--cascade=false to kill only the parent execution.`,
		Example: `  # Kill an execution
	  kestractl executions kill 2TLGqHrXC9k8BczKJe5djX

	  # Kill only the parent execution, leaving subflows running
	  kestractl executions kill 2TLGqHrXC9k8BczKJe5djX --cascade=false`,
		Aliases: []string{"cancel"},
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

			return runExecutionsKill(client, args[0], cascade, renderer)
		},
	}

	cmd.Flags().BoolVar(&cascade, "cascade", true, "Also kill subflow executions")

	return cmd
}

func runExecutionsKill(client *Client, executionID string, cascade bool, renderer *Renderer) error {
	_, _, err := client.API.ExecutionsAPI.KillExecution(client.Ctx, executionID, client.Tenant).
		IsOnKillCascade(cascade).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	result := map[string]any{
		"id":      executionID,
		"status":  "kill requested",
		"cascade": cascade,
	}
	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Kill requested for execution '%s'\n", executionID)
		return nil
	})
}

func newExecutionsListCommand() *cobra.Command {
	var (
		namespace string
		flowID    string
		state     string
		page      int32
		size      int32
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List executions.",
		Long: `Search and list executions, optionally filtered by namespace, flow, or state.

Results are paginated. Use --page and --size to navigate larger result sets.`,
		Example: `  # List the most recent executions
	  kestractl executions list

	  # List executions for a namespace
	  kestractl executions list --namespace my.namespace

	  # List failed executions for a specific flow
	  kestractl executions list --namespace my.namespace --flow-id my-flow --state FAILED

	  # Paginate
	  kestractl executions list --page 2 --size 100

	  # JSON output
	  kestractl executions list --output json`,
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := NewClient()
			if err != nil {
				return err
			}

			return runExecutionsList(client, namespace, flowID, state, page, size, renderer)
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Filter by namespace")
	cmd.Flags().StringVar(&flowID, "flow-id", "", "Filter by flow ID")
	cmd.Flags().StringVar(&state, "state", "", "Filter by execution state (e.g. SUCCESS, FAILED, RUNNING)")
	cmd.Flags().Int32Var(&page, "page", 1, "Page number (1-based)")
	cmd.Flags().Int32Var(&size, "size", 50, "Number of executions per page")

	return cmd
}

// equalsFilter builds a QueryFilter matching field == value. The Value field is
// assigned directly because the generated SetValue setter has a mismatched
// (map-only) signature.
func equalsFilter(field kestra.QueryFilterField, value string) kestra.QueryFilter {
	f := kestra.NewQueryFilter()
	f.SetField(field)
	f.SetOperation(kestra.QUERYFILTEROP_EQUALS)
	f.Value = value
	return *f
}

// buildExecutionFilters assembles the QueryFilter list for an executions search
// from the optional namespace, flow ID, and state selectors.
func buildExecutionFilters(namespace, flowID, state string) []kestra.QueryFilter {
	filters := make([]kestra.QueryFilter, 0, 3)
	if namespace != "" {
		filters = append(filters, equalsFilter(kestra.QUERYFILTERFIELD_NAMESPACE, namespace))
	}
	if flowID != "" {
		filters = append(filters, equalsFilter(kestra.QUERYFILTERFIELD_FLOW_ID, flowID))
	}
	if state != "" {
		filters = append(filters, equalsFilter(kestra.QUERYFILTERFIELD_STATE, strings.ToUpper(state)))
	}
	return filters
}

func runExecutionsList(client *Client, namespace, flowID, state string, page, size int32, renderer *Renderer) error {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 50
	}

	req := client.API.ExecutionsAPI.SearchExecutions(client.Ctx, client.Tenant).
		Page(page).
		Size(size)
	if filters := buildExecutionFilters(namespace, flowID, state); len(filters) > 0 {
		req = req.Filters(filters)
	}

	resp, _, err := req.Execute()
	if err != nil {
		return formatSDKError(err)
	}

	executions := resp.GetResults()
	result := make([]map[string]any, len(executions))
	for i, exec := range executions {
		row := map[string]any{
			"id":        exec.GetId(),
			"namespace": exec.GetNamespace(),
			"flowId":    exec.GetFlowId(),
		}
		st := exec.GetState()
		row["state"] = st.GetCurrent()
		if !st.GetStartDate().IsZero() {
			row["startDate"] = st.GetStartDate().Format(time.RFC3339)
		}
		row["duration"] = st.GetDuration()
		result[i] = row
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ID\tNAMESPACE\tFLOW\tSTATE\tSTARTED\tDURATION")
		for _, row := range result {
			started := "-"
			if s, ok := row["startDate"].(string); ok && s != "" {
				started = s
			}
			duration := "-"
			if d, ok := row["duration"]; ok {
				if ds := formatDuration(d); ds != "" {
					duration = ds
				}
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				stringify(row["id"]),
				stringify(row["namespace"]),
				stringify(row["flowId"]),
				stringify(row["state"]),
				started,
				duration,
			)
		}
		fmt.Fprintf(w, "\nShowing %d execution(s) (page %d, total %d)\n", len(result), page, resp.GetTotal())
		return nil
	})
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
