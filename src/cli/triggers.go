package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	kestra "github.com/kestra-io/client-sdk/go-sdk/kestra_api_client"
	"github.com/spf13/cobra"
)

func newTriggersCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "triggers",
		Short: "Manage triggers (list, delete, restart, unlock)",
	}

	cmd.AddCommand(newTriggersListCommand())
	cmd.AddCommand(newTriggersDeleteCommand())
	cmd.AddCommand(newTriggersUnlockCommand())
	cmd.AddCommand(newTriggersRestartCommand())
	cmd.AddCommand(newTriggersUpdateCommand())
	cmd.AddCommand(newTriggersSearchForFlowCommand())
	cmd.AddCommand(newTriggersBackfillPauseCommand())
	cmd.AddCommand(newTriggersBackfillUnpauseCommand())
	cmd.AddCommand(newTriggersBackfillDeleteCommand())
	cmd.AddCommand(newTriggersDeleteByIdsCommand())
	cmd.AddCommand(newTriggersUnlockByIdsCommand())
	cmd.AddCommand(newTriggersDisableByIdsCommand())
	cmd.AddCommand(newTriggersEnableByIdsCommand())
	cmd.AddCommand(newTriggersDisableByQueryCommand())
	cmd.AddCommand(newTriggersEnableByQueryCommand())
	cmd.AddCommand(newTriggersDeleteByQueryCommand())
	cmd.AddCommand(newTriggersUnlockByQueryCommand())
	cmd.AddCommand(newTriggersDisableCommand())
	cmd.AddCommand(newTriggersEnableCommand())
	cmd.AddCommand(newTriggersCreateBackfillCommand())

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

func newTriggersUnlockCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unlock <namespace> <flow_id> <trigger_id>",
		Short: "Unlock a locked trigger.",
		Example: `  kestractl triggers unlock my.namespace my-flow my-trigger
  kestractl triggers unlock my.namespace my-flow my-trigger --output json`,
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
			return runTriggersUnlock(client, args[0], args[1], args[2], renderer)
		},
	}
	return cmd
}

func runTriggersUnlock(client *Client, namespace, flowID, triggerID string, renderer *Renderer) error {
	t, _, err := client.API.TriggersAPI.
		UnlockTrigger(client.Ctx, namespace, flowID, triggerID, client.Tenant).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}
	if t == nil {
		result := map[string]any{"namespace": namespace, "flowId": flowID, "triggerId": triggerID}
		return renderer.Render(result, func(w *tabwriter.Writer) error {
			fmt.Fprintf(w, "Trigger '%s' unlocked.\n", triggerID)
			return nil
		})
	}

	result := map[string]any{
		"namespace": t.GetNamespace(),
		"flowId":    t.GetFlowId(),
		"triggerId": t.GetTriggerId(),
		"disabled":  t.GetDisabled(),
	}
	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "NAMESPACE\t%s\n", t.GetNamespace())
		fmt.Fprintf(w, "FLOW\t%s\n", t.GetFlowId())
		fmt.Fprintf(w, "TRIGGER\t%s\n", t.GetTriggerId())
		fmt.Fprintf(w, "DISABLED\t%v\n", t.GetDisabled())
		fmt.Fprintln(w, "\nTrigger unlocked.")
		return nil
	})
}

func newTriggersRestartCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restart <namespace> <flow_id> <trigger_id>",
		Short: "Restart a trigger.",
		Example: `  kestractl triggers restart my.namespace my-flow my-trigger
  kestractl triggers restart my.namespace my-flow my-trigger --output json`,
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
			return runTriggersRestart(client, args[0], args[1], args[2], renderer)
		},
	}
	return cmd
}

func runTriggersRestart(client *Client, namespace, flowID, triggerID string, renderer *Renderer) error {
	resp, _, err := client.API.TriggersAPI.
		RestartTrigger(client.Ctx, namespace, flowID, triggerID, client.Tenant).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	if resp == nil {
		resp = make(map[string]interface{})
	}

	return renderer.Render(resp, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Trigger '%s' in flow '%s/%s' restarted.\n", triggerID, namespace, flowID)
		return nil
	})
}

func newTriggersUpdateCommand() *cobra.Command {
	var disabled bool

	cmd := &cobra.Command{
		Use:   "update <namespace> <flow_id> <trigger_id>",
		Short: "Update a trigger.",
		Example: `  kestractl triggers update my.namespace my-flow my-trigger --disabled
  kestractl triggers update my.namespace my-flow my-trigger --output json`,
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
			return runTriggersUpdate(client, args[0], args[1], args[2], disabled, renderer)
		},
	}

	cmd.Flags().BoolVar(&disabled, "disabled", false, "Set the trigger as disabled")
	return cmd
}

func newTriggersSearchForFlowCommand() *cobra.Command {
	var page, size int32
	var query string

	cmd := &cobra.Command{
		Use:   "search-for-flow <namespace> <flow_id>",
		Short: "List triggers for a specific flow.",
		Example: `  kestractl triggers search-for-flow my.namespace my-flow
  kestractl triggers search-for-flow my.namespace my-flow --output json`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runTriggersSearchForFlow(client, args[0], args[1], page, size, query, renderer)
		},
	}

	cmd.Flags().Int32Var(&page, "page", 1, "Page number")
	cmd.Flags().Int32Var(&size, "size", 50, "Page size")
	cmd.Flags().StringVarP(&query, "query", "q", "", "Filter triggers by query string")
	return cmd
}

func runTriggersSearchForFlow(client *Client, namespace, flowID string, page, size int32, query string, renderer *Renderer) error {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 50
	}

	req := client.API.TriggersAPI.
		SearchTriggersForFlow(client.Ctx, namespace, flowID, client.Tenant).
		Page(page).
		Size(size)
	if query != "" {
		req = req.Q(query)
	}

	resp, _, err := req.Execute()
	if err != nil {
		return formatSDKError(err)
	}

	triggers := resp.GetResults()
	result := make([]map[string]any, len(triggers))
	for i, t := range triggers {
		result[i] = map[string]any{
			"namespace": t.GetNamespace(),
			"flowId":    t.GetFlowId(),
			"triggerId": t.GetTriggerId(),
			"disabled":  t.GetDisabled(),
		}
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "NAMESPACE\tFLOW\tTRIGGER ID\tDISABLED")
		for _, t := range triggers {
			fmt.Fprintf(w, "%s\t%s\t%s\t%v\n",
				t.GetNamespace(),
				t.GetFlowId(),
				t.GetTriggerId(),
				t.GetDisabled(),
			)
		}
		fmt.Fprintf(w, "\nShowing %d trigger(s) (page %d, total %d)\n", len(triggers), page, resp.GetTotal())
		return nil
	})
}

func newTriggersBackfillPauseCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backfill-pause <namespace> <flow_id> <trigger_id>",
		Short: "Pause a trigger backfill.",
		Example: `  kestractl triggers backfill-pause my.namespace my-flow my-trigger
  kestractl triggers backfill-pause my.namespace my-flow my-trigger --output json`,
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
			return runTriggersBackfillOp(client, args[0], args[1], args[2], "pause", renderer)
		},
	}
	return cmd
}

func newTriggersBackfillUnpauseCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backfill-unpause <namespace> <flow_id> <trigger_id>",
		Short: "Unpause a trigger backfill.",
		Example: `  kestractl triggers backfill-unpause my.namespace my-flow my-trigger
  kestractl triggers backfill-unpause my.namespace my-flow my-trigger --output json`,
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
			return runTriggersBackfillOp(client, args[0], args[1], args[2], "unpause", renderer)
		},
	}
	return cmd
}

func newTriggersBackfillDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backfill-delete <namespace> <flow_id> <trigger_id>",
		Short: "Delete a trigger backfill.",
		Example: `  kestractl triggers backfill-delete my.namespace my-flow my-trigger
  kestractl triggers backfill-delete my.namespace my-flow my-trigger --output json`,
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
			return runTriggersBackfillOp(client, args[0], args[1], args[2], "delete", renderer)
		},
	}
	return cmd
}

func runTriggersBackfillOp(client *Client, namespace, flowID, triggerID, op string, renderer *Renderer) error {
	t := kestra.NewTrigger(namespace, flowID, triggerID, time.Now())

	var updated *kestra.Trigger
	var err error

	switch op {
	case "pause":
		updated, _, err = client.API.TriggersAPI.
			PauseBackfill(client.Ctx, client.Tenant).
			Trigger(*t).
			Execute()
	case "unpause":
		updated, _, err = client.API.TriggersAPI.
			UnpauseBackfill(client.Ctx, client.Tenant).
			Trigger(*t).
			Execute()
	default: // delete
		updated, _, err = client.API.TriggersAPI.
			DeleteBackfill(client.Ctx, client.Tenant).
			Trigger(*t).
			Execute()
	}

	if err != nil {
		return formatSDKError(err)
	}

	ns, fid, tid := namespace, flowID, triggerID
	if updated != nil {
		ns = updated.GetNamespace()
		fid = updated.GetFlowId()
		tid = updated.GetTriggerId()
	}

	result := map[string]any{
		"operation": op,
		"namespace": ns,
		"flowId":    fid,
		"triggerId": tid,
	}
	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Backfill %s for trigger '%s' in flow '%s/%s'.\n", op+"d", tid, ns, fid)
		return nil
	})
}

func runTriggersUpdate(client *Client, namespace, flowID, triggerID string, disabled bool, renderer *Renderer) error {
	t := kestra.NewTrigger(namespace, flowID, triggerID, time.Now())
	t.SetDisabled(disabled)

	updated, _, err := client.API.TriggersAPI.
		UpdateTrigger(client.Ctx, client.Tenant).
		Trigger(*t).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}
	if updated == nil {
		updated = kestra.NewTriggerWithDefaults()
	}

	result := map[string]any{
		"namespace": updated.GetNamespace(),
		"flowId":    updated.GetFlowId(),
		"triggerId": updated.GetTriggerId(),
		"disabled":  updated.GetDisabled(),
	}
	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "NAMESPACE\t%s\n", updated.GetNamespace())
		fmt.Fprintf(w, "FLOW\t%s\n", updated.GetFlowId())
		fmt.Fprintf(w, "TRIGGER\t%s\n", updated.GetTriggerId())
		fmt.Fprintf(w, "DISABLED\t%v\n", updated.GetDisabled())
		fmt.Fprintln(w, "\nTrigger updated.")
		return nil
	})
}

// parseTriggerIds parses <namespace>/<flowId>/<triggerId> arguments.
func parseTriggerIds(args []string) ([]kestra.Trigger, error) {
	triggers := make([]kestra.Trigger, 0, len(args))
	for _, arg := range args {
		parts := strings.SplitN(arg, "/", 3)
		if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
			return nil, fmt.Errorf("invalid trigger id %q: expected <namespace>/<flowId>/<triggerId>", arg)
		}
		t := kestra.NewTrigger(parts[0], parts[1], parts[2], time.Time{})
		triggers = append(triggers, *t)
	}
	return triggers, nil
}

func parseTriggerApiIds(args []string) ([]kestra.TriggerControllerApiTriggerId, error) {
	result := make([]kestra.TriggerControllerApiTriggerId, 0, len(args))
	for _, arg := range args {
		parts := strings.SplitN(arg, "/", 3)
		if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
			return nil, fmt.Errorf("invalid trigger id %q: expected <namespace>/<flowId>/<triggerId>", arg)
		}
		t := kestra.NewTriggerControllerApiTriggerId()
		t.SetNamespace(parts[0])
		t.SetFlowId(parts[1])
		t.SetTriggerId(parts[2])
		result = append(result, *t)
	}
	return result, nil
}

func newTriggersDeleteByIdsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete-by-ids <namespace/flowId/triggerId>...",
		Short: "Delete multiple triggers by IDs.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutputFormat(); err != nil {
				return err
			}
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			triggers, err := parseTriggerIds(args)
			if err != nil {
				return err
			}
			return runTriggersBulkByIds(client, triggers, "delete", renderer)
		},
	}
	return cmd
}

func newTriggersUnlockByIdsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unlock-by-ids <namespace/flowId/triggerId>...",
		Short: "Unlock multiple triggers by IDs.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutputFormat(); err != nil {
				return err
			}
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			triggers, err := parseTriggerIds(args)
			if err != nil {
				return err
			}
			return runTriggersBulkByIds(client, triggers, "unlock", renderer)
		},
	}
	return cmd
}

func runTriggersBulkByIds(client *Client, triggers []kestra.Trigger, op string, renderer *Renderer) error {
	m, _, err := func() (map[string]interface{}, interface{}, error) {
		switch op {
		case "delete":
			m, resp, err := client.API.TriggersAPI.
				DeleteTriggersByIds(client.Ctx, client.Tenant).
				Trigger(triggers).
				Execute()
			return m, resp, err
		default: // unlock
			m, resp, err := client.API.TriggersAPI.
				UnlockTriggersByIds(client.Ctx, client.Tenant).
				Trigger(triggers).
				Execute()
			return m, resp, err
		}
	}()
	if err != nil {
		return formatSDKError(err)
	}

	count := extractCount(m)
	return renderer.Render(map[string]any{"count": count, "operation": op}, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Bulk %s: %d trigger(s) affected.\n", op, count)
		return nil
	})
}

func newTriggersDisableByIdsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disable-by-ids <namespace/flowId/triggerId>...",
		Short: "Disable multiple triggers by IDs.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutputFormat(); err != nil {
				return err
			}
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runTriggersSetDisabledByIds(client, args, true, renderer)
		},
	}
	return cmd
}

func newTriggersEnableByIdsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enable-by-ids <namespace/flowId/triggerId>...",
		Short: "Enable multiple triggers by IDs.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutputFormat(); err != nil {
				return err
			}
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runTriggersSetDisabledByIds(client, args, false, renderer)
		},
	}
	return cmd
}

func runTriggersSetDisabledByIds(client *Client, args []string, disabled bool, renderer *Renderer) error {
	triggerIDs, err := parseTriggerApiIds(args)
	if err != nil {
		return err
	}

	body := kestra.NewTriggerControllerSetDisabledRequest(triggerIDs, disabled)

	m, _, err := client.API.TriggersAPI.
		DisabledTriggersByIds(client.Ctx, client.Tenant).
		TriggerControllerSetDisabledRequest(*body).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	count := extractCount(m)
	op := "disabled"
	if !disabled {
		op = "enabled"
	}
	return renderer.Render(map[string]any{"count": count, "operation": op}, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Bulk %s: %d trigger(s) affected.\n", op, count)
		return nil
	})
}

func newTriggersDisableByQueryCommand() *cobra.Command {
	var filterFlags byQueryFilterFlags

	cmd := &cobra.Command{
		Use:   "disable-by-query",
		Short: "Disable triggers matching query filters.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutputFormat(); err != nil {
				return err
			}
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			filters, err := filterFlags.resolve()
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runTriggersSetDisabledByQuery(client, true, filters, renderer)
		},
	}
	addByQueryFilterFlags(cmd, &filterFlags)
	return cmd
}

func newTriggersEnableByQueryCommand() *cobra.Command {
	var filterFlags byQueryFilterFlags

	cmd := &cobra.Command{
		Use:   "enable-by-query",
		Short: "Enable triggers matching query filters.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutputFormat(); err != nil {
				return err
			}
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			filters, err := filterFlags.resolve()
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runTriggersSetDisabledByQuery(client, false, filters, renderer)
		},
	}
	addByQueryFilterFlags(cmd, &filterFlags)
	return cmd
}

func runTriggersSetDisabledByQuery(client *Client, disabled bool, filters []kestra.QueryFilter, renderer *Renderer) error {
	m, _, err := client.API.TriggersAPI.
		DisabledTriggersByQuery(client.Ctx, client.Tenant).
		Disabled(disabled).
		Filters(filters).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	count := extractCount(m)
	op := "disabled"
	if !disabled {
		op = "enabled"
	}
	return renderer.Render(map[string]any{"count": count, "operation": op}, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Bulk %s: %d trigger(s) affected.\n", op, count)
		return nil
	})
}

func newTriggersDeleteByQueryCommand() *cobra.Command {
	var filterFlags byQueryFilterFlags

	cmd := &cobra.Command{
		Use:   "delete-by-query",
		Short: "Delete triggers matching query filters.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutputFormat(); err != nil {
				return err
			}
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			filters, err := filterFlags.resolve()
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runTriggersBulkByQuery(client, "delete", filters, renderer)
		},
	}
	addByQueryFilterFlags(cmd, &filterFlags)
	return cmd
}

func newTriggersUnlockByQueryCommand() *cobra.Command {
	var filterFlags byQueryFilterFlags

	cmd := &cobra.Command{
		Use:   "unlock-by-query",
		Short: "Unlock triggers matching query filters.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutputFormat(); err != nil {
				return err
			}
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			filters, err := filterFlags.resolve()
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runTriggersBulkByQuery(client, "unlock", filters, renderer)
		},
	}
	addByQueryFilterFlags(cmd, &filterFlags)
	return cmd
}

func runTriggersBulkByQuery(client *Client, op string, filters []kestra.QueryFilter, renderer *Renderer) error {
	m, _, err := func() (map[string]interface{}, interface{}, error) {
		switch op {
		case "delete":
			m, resp, err := client.API.TriggersAPI.
				DeleteTriggersByQuery(client.Ctx, client.Tenant).
				DeleteTriggersByQueryRequest(kestra.DeleteTriggersByQueryRequest{
					Filters: queryFiltersToSearchFilters(filters),
				}).
				Execute()
			return m, resp, err
		default: // unlock
			m, resp, err := client.API.TriggersAPI.
				UnlockTriggersByQuery(client.Ctx, client.Tenant).
				Filters(filters).
				Execute()
			return m, resp, err
		}
	}()
	if err != nil {
		return formatSDKError(err)
	}

	count := extractCount(m)
	return renderer.Render(map[string]any{"count": count, "operation": op}, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Bulk %s: %d trigger(s) affected.\n", op, count)
		return nil
	})
}

func newTriggersDisableCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disable <namespace> <flow_id> <trigger_id>",
		Short: "Disable a single trigger.",
		Example: `  kestractl triggers disable my.namespace my-flow my-trigger
  kestractl triggers disable my.namespace my-flow my-trigger --output json`,
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
			return runTriggersSetDisabled(client, args[0], args[1], args[2], true, renderer)
		},
	}
	return cmd
}

func newTriggersEnableCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enable <namespace> <flow_id> <trigger_id>",
		Short: "Enable a single trigger.",
		Example: `  kestractl triggers enable my.namespace my-flow my-trigger
  kestractl triggers enable my.namespace my-flow my-trigger --output json`,
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
			return runTriggersSetDisabled(client, args[0], args[1], args[2], false, renderer)
		},
	}
	return cmd
}

func runTriggersSetDisabled(client *Client, namespace, flowID, triggerID string, disabled bool, renderer *Renderer) error {
	req := kestra.NewTriggerControllerApiDisableTriggerRequest()
	req.SetNamespace(namespace)
	req.SetFlowId(flowID)
	req.SetTriggerId(triggerID)
	req.SetDisabled(disabled)

	result, err := client.Kestra.Triggers().DisableTriggerById(client.Ctx, client.Tenant, *req)
	if err != nil {
		return formatSDKError(err)
	}

	ns, fid, tid := namespace, flowID, triggerID
	if result != nil {
		ns = result.GetNamespace()
		fid = result.GetFlowId()
		tid = result.GetTriggerId()
	}

	op := "disabled"
	if !disabled {
		op = "enabled"
	}

	row := map[string]any{
		"namespace": ns,
		"flowId":    fid,
		"triggerId": tid,
		"disabled":  disabled,
	}
	return renderer.Render(row, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "NAMESPACE\t%s\n", ns)
		fmt.Fprintf(w, "FLOW\t%s\n", fid)
		fmt.Fprintf(w, "TRIGGER\t%s\n", tid)
		fmt.Fprintf(w, "\nTrigger %s.\n", op)
		return nil
	})
}

func newTriggersCreateBackfillCommand() *cobra.Command {
	var namespace, flowID, triggerID, startStr, endStr string

	cmd := &cobra.Command{
		Use:   "create-backfill",
		Short: "Create a backfill for a trigger.",
		Example: `  kestractl triggers create-backfill --namespace my.ns --flow-id my-flow --trigger-id my-trigger --start 2024-01-01T00:00:00Z --end 2024-02-01T00:00:00Z
  kestractl triggers create-backfill --namespace my.ns --flow-id my-flow --trigger-id my-trigger --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			if namespace == "" || flowID == "" || triggerID == "" {
				return fmt.Errorf("--namespace, --flow-id, and --trigger-id are required")
			}
			var startTime, endTime time.Time
			if startStr != "" {
				t, parseErr := time.Parse(time.RFC3339, startStr)
				if parseErr != nil {
					return fmt.Errorf("invalid --start time (expected RFC3339): %w", parseErr)
				}
				startTime = t
			}
			if endStr != "" {
				t, parseErr := time.Parse(time.RFC3339, endStr)
				if parseErr != nil {
					return fmt.Errorf("invalid --end time (expected RFC3339): %w", parseErr)
				}
				endTime = t
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runTriggersCreateBackfill(client, namespace, flowID, triggerID, startTime, endTime, renderer)
		},
	}

	cmd.Flags().StringVar(&namespace, "namespace", "", "Namespace of the trigger (required)")
	cmd.Flags().StringVar(&flowID, "flow-id", "", "Flow ID of the trigger (required)")
	cmd.Flags().StringVar(&triggerID, "trigger-id", "", "Trigger ID (required)")
	cmd.Flags().StringVar(&startStr, "start", "", "Backfill start time in RFC3339 format")
	cmd.Flags().StringVar(&endStr, "end", "", "Backfill end time in RFC3339 format")
	return cmd
}

func runTriggersCreateBackfill(client *Client, namespace, flowID, triggerID string, startTime, endTime time.Time, renderer *Renderer) error {
	req := kestra.NewTriggerControllerApiCreateBackfillRequest()
	req.SetNamespace(namespace)
	req.SetFlowId(flowID)
	req.SetTriggerId(triggerID)

	backfill := kestra.NewTriggerControllerApiCreateBackfillRequestBackfill()
	if !startTime.IsZero() {
		backfill.SetStart(startTime)
	}
	if !endTime.IsZero() {
		backfill.SetEnd(endTime)
	}
	req.SetBackfill(*backfill)

	result, err := client.Kestra.Triggers().CreateBackfill(client.Ctx, client.Tenant, *req)
	if err != nil {
		return formatSDKError(err)
	}

	ns, fid, tid := namespace, flowID, triggerID
	if result != nil {
		ns = result.GetNamespace()
		fid = result.GetFlowId()
		tid = result.GetTriggerId()
	}

	row := map[string]any{
		"namespace": ns,
		"flowId":    fid,
		"triggerId": tid,
	}
	return renderer.Render(row, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "NAMESPACE\t%s\n", ns)
		fmt.Fprintf(w, "FLOW\t%s\n", fid)
		fmt.Fprintf(w, "TRIGGER\t%s\n", tid)
		fmt.Fprintln(w, "\nBackfill created.")
		return nil
	})
}
