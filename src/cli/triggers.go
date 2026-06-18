package cli

import (
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

func newTriggersCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "triggers",
		Short: "Manage triggers (list, delete, restart, unlock)",
	}

	cmd.AddCommand(newTriggersListCommand())
	cmd.AddCommand(newTriggersDeleteCommand())

	return cmd
}

func newTriggersListCommand() *cobra.Command {
	var page, size int32

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List triggers.",
		Long: `List triggers across all flows, optionally filtered.

Results are paginated. Use --page and --size to navigate.`,
		Example: `  kestractl triggers list
  kestractl triggers list --page 2 --size 20
  kestractl triggers list --output json`,
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runTriggersList(client, page, size, renderer)
		},
	}

	cmd.Flags().Int32Var(&page, "page", 1, "Page number")
	cmd.Flags().Int32Var(&size, "size", 50, "Page size")
	return cmd
}

func runTriggersList(client *Client, page, size int32, renderer *Renderer) error {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 50
	}

	resp, _, err := client.API.TriggersAPI.
		SearchTriggers(client.Ctx, client.Tenant).
		Page(page).
		Size(size).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	triggers := resp.GetResults()
	result := make([]map[string]any, len(triggers))
	for i, t := range triggers {
		ctx := t.GetTriggerContext()
		row := map[string]any{
			"namespace": ctx.GetNamespace(),
			"flowId":    ctx.GetFlowId(),
			"triggerId": ctx.GetTriggerId(),
			"disabled":  ctx.GetDisabled(),
		}
		if next, ok := ctx.GetNextExecutionDateOk(); ok && next != nil && !next.IsZero() {
			row["nextExecutionDate"] = next.Format(time.RFC3339)
		}
		ab := t.GetAbstractTrigger()
		row["type"] = ab.GetType()
		result[i] = row
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "NAMESPACE\tFLOW\tTRIGGER ID\tTYPE\tDISABLED\tNEXT RUN")
		for _, row := range result {
			nextRun := "-"
			if s, ok := row["nextExecutionDate"].(string); ok && s != "" {
				nextRun = s
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%v\t%s\n",
				stringify(row["namespace"]),
				stringify(row["flowId"]),
				stringify(row["triggerId"]),
				stringify(row["type"]),
				row["disabled"],
				nextRun,
			)
		}
		fmt.Fprintf(w, "\nShowing %d trigger(s) (page %d, total %d)\n", len(result), page, resp.GetTotal())
		return nil
	})
}

func newTriggersDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <namespace> <flow_id> <trigger_id>",
		Short: "Delete a trigger.",
		Example: `  kestractl triggers delete my.namespace my-flow my-trigger
  kestractl triggers delete my.namespace my-flow my-trigger --output json`,
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
			return runTriggersDelete(client, args[0], args[1], args[2], renderer)
		},
	}
	return cmd
}

func runTriggersDelete(client *Client, namespace, flowID, triggerID string, renderer *Renderer) error {
	_, _, err := client.API.TriggersAPI.
		DeleteTrigger(client.Ctx, namespace, flowID, triggerID, client.Tenant).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	result := map[string]any{
		"namespace": namespace,
		"flowId":    flowID,
		"triggerId": triggerID,
		"deleted":   true,
	}
	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Trigger '%s' deleted from flow '%s/%s'.\n", triggerID, namespace, flowID)
		return nil
	})
}
