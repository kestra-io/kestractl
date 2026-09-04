package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	kestra "github.com/kestra-io/client-sdk/go-sdk/v2/kestra_api_client"
	"github.com/spf13/cobra"
)

func newLogsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Inspect execution logs (list)",
		Long:  `View logs produced by executions.`,
	}

	cmd.AddCommand(newLogsListCommand())
	cmd.AddCommand(newLogsSearchCommand())
	cmd.AddCommand(newLogsDownloadCommand())
	cmd.AddCommand(newLogsDeleteCommand())
	cmd.AddCommand(newLogsDeleteFlowCommand())

	return cmd
}

func newLogsDownloadCommand() *cobra.Command {
	var (
		minLevel   string
		taskRunID  string
		taskID     string
		attempt    int
		outputFile string
	)

	cmd := &cobra.Command{
		Use:   "download <execution_id>",
		Short: "Download logs for an execution as a plain-text file.",
		Long: `Download the log entries produced by an execution as plain text.

By default the logs are streamed to stdout. Use --output-file to write them
to a file instead. The --min-level / --task-id / --task-run-id / --attempt
filters narrow the downloaded logs the same way as 'logs list'.`,
		Example: `  # Stream logs to stdout
	  kestractl logs download 2TLGqHrXC9k8BczKJe5djX

	  # Save logs to a file
	  kestractl logs download 2TLGqHrXC9k8BczKJe5djX --output-file run.log

	  # Download only errors
	  kestractl logs download 2TLGqHrXC9k8BczKJe5djX --min-level ERROR`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := NewClient()
			if err != nil {
				return err
			}

			opts := logFilterOptions(minLevel, taskRunID, taskID, cmd.Flags().Changed("attempt"), attempt)
			return runLogsDownload(client, args[0], opts, outputFile, cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVar(&minLevel, "min-level", "", "Minimum log level to download (e.g. TRACE, DEBUG, INFO, WARN, ERROR)")
	cmd.Flags().StringVar(&taskRunID, "task-run-id", "", "Download logs for this task run ID")
	cmd.Flags().StringVar(&taskID, "task-id", "", "Download logs for this task ID")
	cmd.Flags().IntVar(&attempt, "attempt", 0, "Download logs for this attempt number")
	cmd.Flags().StringVarP(&outputFile, "output-file", "f", "", "Write logs to this file instead of stdout")

	return cmd
}

func runLogsDownload(client *Client, executionID string, opts logFilter, outputFile string, out io.Writer) error {
	file, err := client.Kestra.Logs().DownloadLogsFromExecution(
		client.Ctx, executionID, client.Tenant, opts.minLevel, opts.taskRunID, opts.taskID, opts.attempt)
	if err != nil {
		return formatSDKError(err)
	}
	defer cleanupNamespaceTempFile(file)

	dst := out
	if outputFile != "" {
		f, err := os.Create(outputFile)
		if err != nil {
			return fmt.Errorf("failed to create output file %q: %w", outputFile, err)
		}
		defer f.Close()
		dst = f
	}

	if _, err := io.Copy(dst, file); err != nil {
		return fmt.Errorf("failed to write logs: %w", err)
	}
	if outputFile != "" {
		fmt.Fprintf(out, "Logs for execution '%s' written to %s\n", executionID, outputFile)
	}
	return nil
}

func newLogsDeleteFlowCommand() *cobra.Command {
	var triggerID string

	cmd := &cobra.Command{
		Use:   "delete-flow <namespace> <flow_id>",
		Short: "Delete all logs for a flow.",
		Long: `Delete every log entry produced by a flow across all of its executions.

Use --trigger-id to restrict deletion to logs produced by a specific
trigger of the flow.`,
		Example: `  # Delete all logs for a flow
	  kestractl logs delete-flow my.namespace my-flow

	  # Delete only logs produced by a specific trigger
	  kestractl logs delete-flow my.namespace my-flow --trigger-id my-trigger`,
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

			var triggerFilter *string
			if triggerID != "" {
				triggerFilter = &triggerID
			}
			return runLogsDeleteFlow(client, args[0], args[1], triggerFilter, renderer)
		},
	}

	cmd.Flags().StringVar(&triggerID, "trigger-id", "", "Only delete logs produced by this trigger ID")

	return cmd
}

func runLogsDeleteFlow(client *Client, namespace, flowID string, triggerID *string, renderer *Renderer) error {
	err := client.Kestra.Logs().DeleteLogsFromFlow(client.Ctx, namespace, flowID, client.Tenant, triggerID)
	if err != nil {
		return formatSDKError(err)
	}

	return renderStatus(renderer, fmt.Sprintf("Logs for flow '%s' in namespace '%s' deleted.", flowID, namespace),
		map[string]any{"namespace": namespace, "flowId": flowID, "status": "deleted"})
}

func newLogsDeleteCommand() *cobra.Command {
	var (
		minLevel  string
		taskRunID string
		taskID    string
		attempt   int
	)

	cmd := &cobra.Command{
		Use:   "delete <execution_id>",
		Short: "Delete logs for an execution.",
		Long: `Delete the log entries produced by an execution.

Use --min-level / --task-id / --task-run-id / --attempt to delete only a
subset of the execution's logs.`,
		Example: `  # Delete all logs for an execution
	  kestractl logs delete 2TLGqHrXC9k8BczKJe5djX

	  # Delete only logs for a single task
	  kestractl logs delete 2TLGqHrXC9k8BczKJe5djX --task-id my-task`,
		Aliases: []string{"rm"},
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

			opts := logFilterOptions(minLevel, taskRunID, taskID, cmd.Flags().Changed("attempt"), attempt)
			return runLogsDelete(client, args[0], opts, renderer)
		},
	}

	cmd.Flags().StringVar(&minLevel, "min-level", "", "Only delete logs at or above this level")
	cmd.Flags().StringVar(&taskRunID, "task-run-id", "", "Only delete logs for this task run ID")
	cmd.Flags().StringVar(&taskID, "task-id", "", "Only delete logs for this task ID")
	cmd.Flags().IntVar(&attempt, "attempt", 0, "Only delete logs for this attempt number")

	return cmd
}

func runLogsDelete(client *Client, executionID string, opts logFilter, renderer *Renderer) error {
	err := client.Kestra.Logs().DeleteLogsFromExecution(
		client.Ctx, executionID, client.Tenant, opts.minLevel, opts.taskRunID, opts.taskID, opts.attempt)
	if err != nil {
		return formatSDKError(err)
	}

	return renderStatus(renderer, fmt.Sprintf("Logs for execution '%s' deleted.", executionID),
		map[string]any{"executionId": executionID, "status": "deleted"})
}

func newLogsSearchCommand() *cobra.Command {
	var (
		query     string
		namespace string
		flowID    string
		triggerID string
		minLevel  string
		page      int32
		size      int32
		sort      []string
	)

	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search logs across executions.",
		Long: `Search log entries across the tenant, optionally filtered by free-text
query, namespace, flow, trigger, or minimum level.

Results are paginated. Use --page and --size to navigate larger result sets.`,
		Example: `  # Search for an error message
	  kestractl logs search --query "connection refused" --min-level ERROR

	  # Search logs for a flow
	  kestractl logs search --namespace my.namespace --flow-id my-flow

	  # JSON output
	  kestractl logs search --namespace my.namespace --output json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := NewClient()
			if err != nil {
				return err
			}

			filters := buildLogSearchFilters(client, query, namespace, flowID, triggerID, minLevel)
			return runLogsSearch(client, filters, page, size, sort, renderer)
		},
	}

	cmd.Flags().StringVarP(&query, "query", "q", "", "Free-text search query")
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Filter by namespace")
	cmd.Flags().StringVar(&flowID, "flow-id", "", "Filter by flow ID")
	cmd.Flags().StringVar(&triggerID, "trigger-id", "", "Filter by trigger ID")
	cmd.Flags().StringVar(&minLevel, "min-level", "", "Minimum log level (e.g. TRACE, DEBUG, INFO, WARN, ERROR)")
	cmd.Flags().Int32Var(&page, "page", 1, "Page number (1-based)")
	cmd.Flags().Int32Var(&size, "size", 50, "Number of log entries per page")
	cmd.Flags().StringArrayVar(&sort, "sort", nil, "Sort expression (e.g. 'timestamp:desc', repeatable)")

	return cmd
}

// buildLogSearchFilters assembles the SearchFilters for a log search from the
// optional query, namespace, flow, trigger, and level selectors. Every field is
// matched with EQUALS except the level, whose accepted operation differs by
// server era (see minLevelSearchOp).
func buildLogSearchFilters(client *Client, query, namespace, flowID, triggerID, minLevel string) []kestra.SearchFilter {
	filters := make([]kestra.SearchFilter, 0, 5)
	add := func(field kestra.SearchFilterField, op kestra.SearchFilterOp, value string) {
		if value != "" {
			filters = append(filters, kestra.SearchFilter{
				Field:     field,
				Operation: op,
				Value:     value,
			})
		}
	}
	add(kestra.FilterQuery, kestra.OpEquals, query)
	add(kestra.FilterNamespace, kestra.OpEquals, namespace)
	add(kestra.FilterFlowId, kestra.OpEquals, flowID)
	add(kestra.FilterTriggerId, kestra.OpEquals, triggerID)
	add(kestra.FilterMinLevel, minLevelSearchOp(client), strings.ToUpper(minLevel))
	return filters
}

// minLevelSearchOp returns the operation the log-search endpoint accepts on the
// level field. The two server lines disagree, and each rejects the other's
// operation with a 400, so this cannot be a single value:
//
//   - Kestra 2.0 exposes the field as LEVEL and accepts only
//     GREATER_THAN_OR_EQUAL_TO / LESS_THAN_OR_EQUAL_TO / IN / NOT_IN.
//   - Kestra 1.x exposes it as MIN_LEVEL — already a minimum server-side — and
//     accepts only EQUALS / NOT_EQUALS.
//
// An unknown server (develop build, unreachable /configs) gets the 2.0
// operation, matching the rest of the compat layer's default.
func minLevelSearchOp(client *Client) kestra.SearchFilterOp {
	if client != nil && client.isLegacyServer() {
		return kestra.OpEquals
	}
	return kestra.OpGreaterThanOrEqualTo
}

func runLogsSearch(client *Client, filters []kestra.SearchFilter, page, size int32, sort []string, renderer *Renderer) error {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 50
	}
	pageParam, sizeParam := int(page), int(size)

	resp, err := client.Kestra.Logs().SearchLogs(client.Ctx, client.Tenant, &pageParam, &sizeParam, sort, filters)
	if err != nil {
		return formatSDKError(err)
	}
	if resp == nil {
		resp = &kestra.PagedResultsLogEntry{}
	}

	logs := resp.GetResults()
	result := make([]map[string]any, len(logs))
	for i, entry := range logs {
		result[i] = map[string]any{
			"timestamp": entry.GetTimestamp().Format(time.RFC3339),
			"level":     string(entry.GetLevel()),
			"namespace": entry.GetNamespace(),
			"flowId":    entry.GetFlowId(),
			"taskId":    entry.GetTaskId(),
			"message":   entry.GetMessage(),
		}
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "TIMESTAMP\tLEVEL\tNAMESPACE\tFLOW\tTASK\tMESSAGE")
		for _, entry := range logs {
			task := entry.GetTaskId()
			if task == "" {
				task = "-"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				entry.GetTimestamp().Format(time.RFC3339),
				stringify(entry.GetLevel()),
				entry.GetNamespace(),
				entry.GetFlowId(),
				task,
				entry.GetMessage(),
			)
		}
		fmt.Fprintf(w, "\nShowing %d log entry(ies) (page %d, total %d)\n", len(logs), page, resp.Total)
		return nil
	})
}

func newLogsListCommand() *cobra.Command {
	var (
		minLevel  string
		taskRunID string
		taskID    string
		attempt   int
	)

	cmd := &cobra.Command{
		Use:   "list <execution_id>",
		Short: "List logs for an execution.",
		Long: `List the log entries produced by an execution.

Use --min-level to filter by severity (e.g. INFO, WARN, ERROR), and
--task-id / --task-run-id / --attempt to narrow the logs to a single task.`,
		Example: `  # List all logs for an execution
	  kestractl logs list 2TLGqHrXC9k8BczKJe5djX

	  # List only warnings and errors
	  kestractl logs list 2TLGqHrXC9k8BczKJe5djX --min-level WARN

	  # List logs for a single task
	  kestractl logs list 2TLGqHrXC9k8BczKJe5djX --task-id my-task`,
		Aliases: []string{"ls"},
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

			opts := logFilterOptions(minLevel, taskRunID, taskID, cmd.Flags().Changed("attempt"), attempt)
			return runLogsList(client, args[0], opts, renderer)
		},
	}

	cmd.Flags().StringVar(&minLevel, "min-level", "", "Minimum log level to return (e.g. TRACE, DEBUG, INFO, WARN, ERROR)")
	cmd.Flags().StringVar(&taskRunID, "task-run-id", "", "Filter logs by task run ID")
	cmd.Flags().StringVar(&taskID, "task-id", "", "Filter logs by task ID")
	cmd.Flags().IntVar(&attempt, "attempt", 0, "Filter logs by attempt number")

	return cmd
}

// logFilter holds the optional selectors shared by the execution log endpoints.
type logFilter struct {
	minLevel  *string
	taskRunID *string
	taskID    *string
	attempt   *int
}

// logFilterOptions builds a logFilter from raw flag values, leaving unset
// selectors nil so they are omitted from the request.
func logFilterOptions(minLevel, taskRunID, taskID string, hasAttempt bool, attempt int) logFilter {
	f := logFilter{}
	if minLevel != "" {
		f.minLevel = &minLevel
	}
	if taskRunID != "" {
		f.taskRunID = &taskRunID
	}
	if taskID != "" {
		f.taskID = &taskID
	}
	if hasAttempt {
		f.attempt = &attempt
	}
	return f
}

func runLogsList(client *Client, executionID string, opts logFilter, renderer *Renderer) error {
	logs, err := client.Kestra.Logs().ListLogsFromExecution(
		client.Ctx, executionID, client.Tenant, opts.minLevel, opts.taskRunID, opts.taskID, opts.attempt)
	if err != nil {
		return formatSDKError(err)
	}
	if logs == nil {
		logs = []kestra.LogEntry{}
	}

	result := make([]map[string]any, len(logs))
	for i, entry := range logs {
		result[i] = map[string]any{
			"timestamp": entry.GetTimestamp().Format(time.RFC3339),
			"level":     string(entry.GetLevel()),
			"taskId":    entry.GetTaskId(),
			"message":   entry.GetMessage(),
		}
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "TIMESTAMP\tLEVEL\tTASK\tMESSAGE")
		for _, entry := range logs {
			task := entry.GetTaskId()
			if task == "" {
				task = "-"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				entry.GetTimestamp().Format(time.RFC3339),
				stringify(entry.GetLevel()),
				task,
				entry.GetMessage(),
			)
		}
		fmt.Fprintf(w, "\nTotal log entries: %d\n", len(logs))
		return nil
	})
}
