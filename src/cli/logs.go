package cli

import (
	"fmt"
	"text/tabwriter"
	"time"

	kestra "github.com/kestra-io/client-sdk/go-sdk/kestra_api_client"
	"github.com/spf13/cobra"
)

func newLogsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Inspect execution logs (list)",
		Long:  `View logs produced by executions.`,
	}

	cmd.AddCommand(newLogsListCommand())

	return cmd
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
			"level":     stringify(entry.GetLevel()),
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
