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
	cmd.AddCommand(newExecutionsForceRunCommand())
	cmd.AddCommand(newExecutionsUnqueueCommand())
	cmd.AddCommand(newExecutionsDeleteCommand())
	cmd.AddCommand(newExecutionsReplayCommand())
	cmd.AddCommand(newExecutionsSetLabelsCommand())
	cmd.AddCommand(newExecutionsFlowGraphCommand())
	cmd.AddCommand(newExecutionsLatestCommand())
	cmd.AddCommand(newExecutionsChangeStatusCommand())
	cmd.AddCommand(newExecutionsSearchByFlowCommand())
	cmd.AddCommand(newExecutionsUpdateTaskRunCommand())
	cmd.AddCommand(newExecutionsBulkKillCommand())
	cmd.AddCommand(newExecutionsBulkDeleteCommand())
	cmd.AddCommand(newExecutionsBulkRestartCommand())
	cmd.AddCommand(newExecutionsBulkReplayCommand())
	cmd.AddCommand(newExecutionsBulkPauseCommand())
	cmd.AddCommand(newExecutionsBulkResumeCommand())
	cmd.AddCommand(newExecutionsBulkForceRunCommand())
	cmd.AddCommand(newExecutionsBulkSetLabelsCommand())
	cmd.AddCommand(newExecutionsBulkUnqueueCommand())

	return cmd
}

func newExecutionsChangeStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "change-status <execution_id> <status>",
		Short: "Change the status of an execution.",
		Long:  `Force the status of an execution to a new state (e.g. SUCCESS, FAILED, KILLED).`,
		Example: `  # Mark an execution as a success
  kestractl executions change-status 2TLGqHrXC9k8BczKJe5djX SUCCESS`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := NewClient()
			if err != nil {
				return err
			}

			return runExecutionsChangeStatus(client, args[0], args[1], renderer)
		},
	}

	return cmd
}

func runExecutionsChangeStatus(client *Client, executionID, status string, renderer *Renderer) error {
	exec, _, err := client.API.ExecutionsAPI.UpdateExecutionStatus(client.Ctx, executionID, client.Tenant).
		Status(kestra.StateType(strings.ToUpper(status))).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}
	if exec == nil {
		exec = kestra.NewExecutionWithDefaults()
	}

	state := exec.GetState()
	result := map[string]any{
		"id":     exec.GetId(),
		"status": state.GetCurrent(),
	}
	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Execution '%s' status changed to %s\n", exec.GetId(), state.GetCurrent())
		return nil
	})
}

// parseFlowRefs converts "namespace:flowId" strings into flow filters for the
// latest-executions endpoint.
func parseFlowRefs(refs []string) ([]kestra.ExecutionRepositoryInterfaceFlowFilter, error) {
	filters := make([]kestra.ExecutionRepositoryInterfaceFlowFilter, 0, len(refs))
	for _, r := range refs {
		ns, id, ok := strings.Cut(r, ":")
		if !ok || ns == "" || id == "" {
			return nil, fmt.Errorf("invalid flow reference %q: expected format namespace:flowId", r)
		}
		filters = append(filters, *kestra.NewExecutionRepositoryInterfaceFlowFilter(ns, id))
	}
	return filters, nil
}

func newExecutionsLatestCommand() *cobra.Command {
	var flows []string

	cmd := &cobra.Command{
		Use:   "latest",
		Short: "Show the latest execution for each given flow.",
		Long:  `Show the latest execution for each requested flow (namespace:flowId).`,
		Example: `  # Latest execution of two flows
  kestractl executions latest --flow company.team:daily --flow company.team:hourly`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			if len(flows) == 0 {
				return fmt.Errorf("at least one --flow namespace:flowId is required")
			}
			filters, err := parseFlowRefs(flows)
			if err != nil {
				return err
			}
			client, err := NewClient()
			if err != nil {
				return err
			}

			return runExecutionsLatest(client, filters, renderer)
		},
	}

	cmd.Flags().StringArrayVar(&flows, "flow", nil, "Flow reference as namespace:flowId (repeatable)")

	return cmd
}

func runExecutionsLatest(client *Client, filters []kestra.ExecutionRepositoryInterfaceFlowFilter, renderer *Renderer) error {
	results, _, err := client.API.ExecutionsAPI.LatestExecutions(client.Ctx, client.Tenant).
		ExecutionRepositoryInterfaceFlowFilter(filters).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	if renderer.IsJSON() {
		list := make([]map[string]any, len(results))
		for i, r := range results {
			list[i] = map[string]any{
				"id":        r.GetId(),
				"namespace": r.GetNamespace(),
				"flowId":    r.GetFlowId(),
				"status":    string(r.GetStatus()),
				"startDate": r.GetStartDate().Format(time.RFC3339),
			}
		}
		return renderer.RenderJSON(list)
	}

	return renderer.Render(results, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "NAMESPACE\tFLOW\tEXECUTION\tSTATUS\tSTARTED")
		for _, r := range results {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				r.GetNamespace(), r.GetFlowId(), r.GetId(),
				string(r.GetStatus()), r.GetStartDate().Format(time.RFC3339))
		}
		return nil
	})
}

func newExecutionsFlowGraphCommand() *cobra.Command {
	var subflows []string

	cmd := &cobra.Command{
		Use:   "flow-graph <execution_id>",
		Short: "Show the topology graph of an execution.",
		Long:  `Show the nodes and edges of the flow graph for a given execution.`,
		Example: `  # Print the execution graph
  kestractl executions flow-graph 2TLGqHrXC9k8BczKJe5djX

  # Expand specific subflows
  kestractl executions flow-graph 2TLGqHrXC9k8BczKJe5djX --subflow my.ns.subflow`,
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

			return runExecutionsFlowGraph(client, args[0], subflows, renderer)
		},
	}

	cmd.Flags().StringArrayVar(&subflows, "subflow", nil, "Subflow to expand (repeatable)")

	return cmd
}

func runExecutionsFlowGraph(client *Client, executionID string, subflows []string, renderer *Renderer) error {
	req := client.API.ExecutionsAPI.ExecutionFlowGraph(client.Ctx, executionID, client.Tenant)
	if len(subflows) > 0 {
		req = req.Subflows(subflows)
	}
	graph, _, err := req.Execute()
	if err != nil {
		return formatSDKError(err)
	}
	if graph == nil {
		graph = kestra.NewFlowGraphWithDefaults()
	}

	nodes := graph.GetNodes()
	edges := graph.GetEdges()

	if renderer.IsJSON() {
		nodeList := make([]map[string]any, len(nodes))
		for i, n := range nodes {
			nodeList[i] = map[string]any{
				"uid":  n.GetUid(),
				"type": n.GetType(),
			}
		}
		edgeList := make([]map[string]any, len(edges))
		for i, e := range edges {
			edgeList[i] = map[string]any{
				"source":   e.GetSource(),
				"target":   e.GetTarget(),
				"relation": edgeRelationValue(e),
			}
		}
		return renderer.RenderJSON(map[string]any{
			"nodes":     nodeList,
			"edges":     edgeList,
			"flowables": graph.GetFlowables(),
		})
	}

	return renderer.Render(edges, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "SOURCE\tTARGET\tRELATION")
		for _, e := range edges {
			fmt.Fprintf(w, "%s\t%s\t%s\n", e.GetSource(), e.GetTarget(), edgeRelationValue(e))
		}
		fmt.Fprintf(w, "\n%d node(s), %d edge(s)\n", len(nodes), len(edges))
		return nil
	})
}

// edgeRelationValue extracts the relation value of a flow graph edge, if any.
func edgeRelationValue(e kestra.FlowGraphEdge) string {
	rel := e.GetRelation()
	if rel.Value != nil {
		return *rel.Value
	}
	return ""
}

// parseLabels converts "key=value" strings into Kestra Label values.
func parseLabels(pairs []string) ([]kestra.Label, error) {
	labels := make([]kestra.Label, 0, len(pairs))
	for _, p := range pairs {
		key, value, ok := strings.Cut(p, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("invalid label %q: expected format key=value", p)
		}
		labels = append(labels, *kestra.NewLabel(key, value))
	}
	return labels, nil
}

func newExecutionsSetLabelsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set-labels <execution_id> <key=value> [key=value...]",
		Short: "Set labels on a terminated execution.",
		Long:  `Set (add or overwrite) labels on a terminated execution.`,
		Example: `  # Add two labels to a terminated execution
  kestractl executions set-labels 2TLGqHrXC9k8BczKJe5djX env=prod team=data`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			labels, err := parseLabels(args[1:])
			if err != nil {
				return err
			}
			client, err := NewClient()
			if err != nil {
				return err
			}

			return runExecutionsSetLabels(client, args[0], labels, renderer)
		},
	}

	return cmd
}

func runExecutionsSetLabels(client *Client, executionID string, labels []kestra.Label, renderer *Renderer) error {
	_, _, err := client.API.ExecutionsAPI.SetLabelsOnTerminatedExecution(client.Ctx, executionID, client.Tenant).
		Label(labels).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	result := map[string]any{
		"id":     executionID,
		"labels": len(labels),
	}
	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Set %d label(s) on execution '%s'\n", len(labels), executionID)
		return nil
	})
}

func newExecutionsReplayCommand() *cobra.Command {
	var (
		taskRunID string
		revision  int32
	)

	cmd := &cobra.Command{
		Use:   "replay <execution_id>",
		Short: "Replay an execution as a new execution.",
		Long: `Replay an execution, creating a new execution that reuses the original's
inputs and labels.

Use --task-run-id to replay from a specific task run, and --revision to
replay against a specific flow revision.`,
		Example: `  # Replay an execution
	  kestractl executions replay 2TLGqHrXC9k8BczKJe5djX

	  # Replay from a specific task run
	  kestractl executions replay 2TLGqHrXC9k8BczKJe5djX --task-run-id 5Abc...`,
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

			return runExecutionsReplay(client, args[0], taskRunID, cmd.Flags().Changed("revision"), revision, renderer)
		},
	}

	cmd.Flags().StringVar(&taskRunID, "task-run-id", "", "Replay from this task run ID")
	cmd.Flags().Int32Var(&revision, "revision", 0, "Flow revision to replay against")

	return cmd
}

func runExecutionsReplay(client *Client, executionID, taskRunID string, hasRevision bool, revision int32, renderer *Renderer) error {
	req := client.API.ExecutionsAPI.ReplayExecution(client.Ctx, executionID, client.Tenant)
	if taskRunID != "" {
		req = req.TaskRunId(taskRunID)
	}
	if hasRevision {
		req = req.Revision(revision)
	}

	exec, _, err := req.Execute()
	if err != nil {
		return formatSDKError(err)
	}
	if exec == nil {
		return fmt.Errorf("replay returned no execution")
	}

	return renderExecutionResult(renderer, exec, fmt.Sprintf("Execution '%s' replayed as '%s'", executionID, exec.GetId()))
}

func newExecutionsDeleteCommand() *cobra.Command {
	var (
		deleteLogs    bool
		deleteMetrics bool
		deleteStorage bool
	)

	cmd := &cobra.Command{
		Use:   "delete <execution_id>",
		Short: "Delete an execution.",
		Long: `Delete an execution.

By default only the execution record is removed. Use --delete-logs,
--delete-metrics, and --delete-storage to also purge the associated logs,
metrics, and internal storage files.`,
		Example: `  # Delete an execution
	  kestractl executions delete 2TLGqHrXC9k8BczKJe5djX

	  # Delete an execution along with its logs and storage
	  kestractl executions delete 2TLGqHrXC9k8BczKJe5djX --delete-logs --delete-storage`,
		Aliases: []string{"rm", "del"},
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

			return runExecutionsDelete(client, args[0], deleteLogs, deleteMetrics, deleteStorage, renderer)
		},
	}

	cmd.Flags().BoolVar(&deleteLogs, "delete-logs", false, "Also delete the execution's logs")
	cmd.Flags().BoolVar(&deleteMetrics, "delete-metrics", false, "Also delete the execution's metrics")
	cmd.Flags().BoolVar(&deleteStorage, "delete-storage", false, "Also delete the execution's internal storage files")

	return cmd
}

func runExecutionsDelete(client *Client, executionID string, deleteLogs, deleteMetrics, deleteStorage bool, renderer *Renderer) error {
	_, err := client.API.ExecutionsAPI.DeleteExecution(client.Ctx, executionID, client.Tenant).
		DeleteLogs(deleteLogs).
		DeleteMetrics(deleteMetrics).
		DeleteStorage(deleteStorage).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	result := map[string]any{
		"id":     executionID,
		"status": "deleted",
	}
	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Execution '%s' deleted\n", executionID)
		return nil
	})
}

func newExecutionsUnqueueCommand() *cobra.Command {
	var state string

	cmd := &cobra.Command{
		Use:   "unqueue <execution_id>",
		Short: "Unqueue a queued execution.",
		Long: `Unqueue an execution that is waiting in the queue.

Use --state to set the state the execution transitions to (defaults to the
server's behavior, typically RUNNING).`,
		Example: `  # Unqueue an execution
	  kestractl executions unqueue 2TLGqHrXC9k8BczKJe5djX

	  # Unqueue and mark it as CANCELLED
	  kestractl executions unqueue 2TLGqHrXC9k8BczKJe5djX --state CANCELLED`,
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

			return runExecutionsUnqueue(client, args[0], state, renderer)
		},
	}

	cmd.Flags().StringVar(&state, "state", "", "Target state for the unqueued execution (e.g. RUNNING, CANCELLED)")

	return cmd
}

func runExecutionsUnqueue(client *Client, executionID, state string, renderer *Renderer) error {
	req := client.API.ExecutionsAPI.UnqueueExecution(client.Ctx, executionID, client.Tenant)
	if state != "" {
		req = req.State(kestra.StateType(strings.ToUpper(state)))
	}

	exec, _, err := req.Execute()
	if err != nil {
		return formatSDKError(err)
	}
	if exec == nil {
		return fmt.Errorf("unqueue returned no execution")
	}

	return renderExecutionResult(renderer, exec, fmt.Sprintf("Execution '%s' unqueued", executionID))
}

func newExecutionsForceRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "force-run <execution_id>",
		Short: "Force-run a queued or paused execution.",
		Long:  `Force an execution to run immediately, bypassing queue or concurrency limits.`,
		Example: `  # Force-run an execution
	  kestractl executions force-run 2TLGqHrXC9k8BczKJe5djX`,
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

			return runExecutionsForceRun(client, args[0], renderer)
		},
	}

	return cmd
}

func runExecutionsForceRun(client *Client, executionID string, renderer *Renderer) error {
	exec, _, err := client.API.ExecutionsAPI.ForceRunExecution(client.Ctx, executionID, client.Tenant).Execute()
	if err != nil {
		return formatSDKError(err)
	}
	if exec == nil {
		return fmt.Errorf("force-run returned no execution")
	}

	return renderExecutionResult(renderer, exec, fmt.Sprintf("Execution '%s' force-run requested", executionID))
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

func newExecutionsUpdateTaskRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-taskrun <execution_id> <taskrun_id> <state>",
		Short: "Change the state of a task run within an execution.",
		Example: `  kestractl executions update-taskrun exec-123 taskrun-456 SUCCESS
  kestractl executions update-taskrun exec-123 taskrun-456 FAILED --output json`,
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runExecutionsUpdateTaskRun(client, args[0], args[1], args[2], renderer)
		},
	}
	return cmd
}

func runExecutionsUpdateTaskRun(client *Client, executionID, taskRunID, state string, renderer *Renderer) error {
	req := kestra.NewExecutionControllerStateRequest(taskRunID, kestra.StateType(strings.ToUpper(state)))
	exec, _, err := client.API.ExecutionsAPI.
		UpdateTaskRunState(client.Ctx, executionID, client.Tenant).
		ExecutionControllerStateRequest(*req).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}
	if exec == nil {
		exec = kestra.NewExecutionWithDefaults()
	}

	result := map[string]any{
		"id":        exec.GetId(),
		"namespace": exec.GetNamespace(),
		"flowId":    exec.GetFlowId(),
		"state":     func() string { s := exec.GetState(); return string(s.GetCurrent()) }(),
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "ID\t%s\n", stringify(result["id"]))
		fmt.Fprintf(w, "NAMESPACE\t%s\n", stringify(result["namespace"]))
		fmt.Fprintf(w, "FLOW\t%s\n", stringify(result["flowId"]))
		fmt.Fprintf(w, "STATE\t%s\n", stringify(result["state"]))
		return nil
	})
}

func newExecutionsSearchByFlowCommand() *cobra.Command {
	var namespace, flowID string
	var page, size int32

	cmd := &cobra.Command{
		Use:   "search-by-flow",
		Short: "Search executions for a specific flow.",
		Example: `  kestractl executions search-by-flow --namespace my.ns --flow-id my-flow
  kestractl executions search-by-flow --namespace my.ns --flow-id my-flow --page 2 --size 20`,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runExecutionsSearchByFlow(client, namespace, flowID, page, size, renderer)
		},
	}

	cmd.Flags().StringVar(&namespace, "namespace", "", "Namespace of the flow (required)")
	cmd.Flags().StringVar(&flowID, "flow-id", "", "Flow ID (required)")
	cmd.Flags().Int32Var(&page, "page", 1, "Page number")
	cmd.Flags().Int32Var(&size, "size", 50, "Page size")
	_ = cmd.MarkFlagRequired("namespace")
	_ = cmd.MarkFlagRequired("flow-id")
	return cmd
}

func runExecutionsSearchByFlow(client *Client, namespace, flowID string, page, size int32, renderer *Renderer) error {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 50
	}

	resp, _, err := client.API.ExecutionsAPI.
		SearchExecutionsByFlowId(client.Ctx, client.Tenant).
		Namespace(namespace).
		FlowId(flowID).
		Page(page).
		Size(size).
		Execute()
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

func newExecutionsBulkCommand(use, short, op string) *cobra.Command {
	return &cobra.Command{
		Use:     fmt.Sprintf("%s <execution_id>...", use),
		Short:   short,
		Example: fmt.Sprintf("  kestractl executions %s id1 id2 id3", use),
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runExecutionsBulkOp(client, args, op, renderer)
		},
	}
}

func newExecutionsBulkKillCommand() *cobra.Command {
	return newExecutionsBulkCommand("kill-bulk", "Kill multiple executions.", "kill")
}

func newExecutionsBulkDeleteCommand() *cobra.Command {
	return newExecutionsBulkCommand("delete-bulk", "Delete multiple executions.", "delete")
}

func newExecutionsBulkRestartCommand() *cobra.Command {
	return newExecutionsBulkCommand("restart-bulk", "Restart multiple executions.", "restart")
}

func newExecutionsBulkReplayCommand() *cobra.Command {
	return newExecutionsBulkCommand("replay-bulk", "Replay multiple executions.", "replay")
}

func newExecutionsBulkPauseCommand() *cobra.Command {
	return newExecutionsBulkCommand("pause-bulk", "Pause multiple executions.", "pause")
}

func newExecutionsBulkResumeCommand() *cobra.Command {
	return newExecutionsBulkCommand("resume-bulk", "Resume multiple executions.", "resume")
}

func newExecutionsBulkForceRunCommand() *cobra.Command {
	return newExecutionsBulkCommand("force-run-bulk", "Force-run multiple executions.", "force-run")
}

func runExecutionsBulkOp(client *Client, ids []string, op string, renderer *Renderer) error {
	var resp *kestra.BulkResponse
	var err error

	switch op {
	case "kill":
		resp, _, err = client.API.ExecutionsAPI.
			KillExecutionsByIds(client.Ctx, client.Tenant).
			RequestBody(ids).Execute()
	case "delete":
		resp, _, err = client.API.ExecutionsAPI.
			DeleteExecutionsByIds(client.Ctx, client.Tenant).
			RequestBody(ids).Execute()
	case "restart":
		resp, _, err = client.API.ExecutionsAPI.
			RestartExecutionsByIds(client.Ctx, client.Tenant).
			RequestBody(ids).Execute()
	case "replay":
		resp, _, err = client.API.ExecutionsAPI.
			ReplayExecutionsByIds(client.Ctx, client.Tenant).
			RequestBody(ids).Execute()
	case "pause":
		resp, _, err = client.API.ExecutionsAPI.
			PauseExecutionsByIds(client.Ctx, client.Tenant).
			RequestBody(ids).Execute()
	case "resume":
		resp, _, err = client.API.ExecutionsAPI.
			ResumeExecutionsByIds(client.Ctx, client.Tenant).
			RequestBody(ids).Execute()
	default: // force-run
		resp, _, err = client.API.ExecutionsAPI.
			ForceRunByIds(client.Ctx, client.Tenant).
			RequestBody(ids).Execute()
	}

	if err != nil {
		return formatSDKError(err)
	}

	count := int32(0)
	if resp != nil {
		count = resp.GetCount()
	}

	result := map[string]any{
		"operation": op,
		"count":     count,
		"ids":       ids,
	}
	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Bulk %s: %d execution(s) affected.\n", op, count)
		return nil
	})
}

func newExecutionsBulkSetLabelsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set-labels-bulk <key=value> [key=value...] --ids <id1> [--ids <id2>...]",
		Short: "Set labels on multiple terminated executions.",
		Example: `  kestractl executions set-labels-bulk env=prod team=data --ids id1 --ids id2
  kestractl executions set-labels-bulk env=prod --ids id1 id2 id3`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			ids, _ := cmd.Flags().GetStringArray("ids")
			if len(ids) == 0 {
				return fmt.Errorf("at least one --ids value is required")
			}
			labels, err := parseLabels(args)
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runExecutionsBulkSetLabels(client, ids, labels, renderer)
		},
	}
	cmd.Flags().StringArray("ids", nil, "Execution IDs to label (repeatable)")
	return cmd
}

func runExecutionsBulkSetLabels(client *Client, ids []string, labels []kestra.Label, renderer *Renderer) error {
	req := kestra.NewExecutionControllerSetLabelsByIdsRequest(ids, labels)
	resp, _, err := client.API.ExecutionsAPI.
		SetLabelsOnTerminatedExecutionsByIds(client.Ctx, client.Tenant).
		ExecutionControllerSetLabelsByIdsRequest(*req).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	count := int32(0)
	if resp != nil {
		count = resp.GetCount()
	}

	result := map[string]any{"count": count, "ids": ids, "labels": len(labels)}
	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Set %d label(s) on %d execution(s).\n", len(labels), count)
		return nil
	})
}

func newExecutionsBulkUnqueueCommand() *cobra.Command {
	var state string

	cmd := &cobra.Command{
		Use:   "unqueue-bulk <execution_id>...",
		Short: "Unqueue multiple executions.",
		Example: `  kestractl executions unqueue-bulk id1 id2 id3
  kestractl executions unqueue-bulk id1 id2 --state RUNNING`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runExecutionsBulkUnqueue(client, args, state, renderer)
		},
	}

	cmd.Flags().StringVar(&state, "state", "", "Target state for unqueued executions")
	return cmd
}

func runExecutionsBulkUnqueue(client *Client, ids []string, state string, renderer *Renderer) error {
	req := client.API.ExecutionsAPI.
		UnqueueExecutionsByIds(client.Ctx, client.Tenant).
		RequestBody(ids)
	if state != "" {
		req = req.State(kestra.StateType(strings.ToUpper(state)))
	}

	resp, _, err := req.Execute()
	if err != nil {
		return formatSDKError(err)
	}

	count := int32(0)
	if resp != nil {
		count = resp.GetCount()
	}

	result := map[string]any{"count": count, "ids": ids}
	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Unqueued %d execution(s).\n", count)
		return nil
	})
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
