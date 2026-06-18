package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func newTestSuitesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test-suites",
		Short: "Manage test suites (list, get, delete, run)",
	}

	cmd.AddCommand(newTestSuitesListCommand())

	return cmd
}

func newTestSuitesListCommand() *cobra.Command {
	var namespace, flowID string
	var page, size int32

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List test suites.",
		Long: `List test suites, optionally filtered by namespace or flow ID.

Results are paginated. Use --page and --size to navigate.`,
		Example: `  kestractl test-suites list
  kestractl test-suites list --namespace my.ns --flow-id my-flow
  kestractl test-suites list --output json`,
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
			return runTestSuitesList(client, namespace, flowID, page, size, renderer)
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Filter by namespace")
	cmd.Flags().StringVar(&flowID, "flow-id", "", "Filter by flow ID")
	cmd.Flags().Int32Var(&page, "page", 1, "Page number")
	cmd.Flags().Int32Var(&size, "size", 50, "Page size")
	return cmd
}

func runTestSuitesList(client *Client, namespace, flowID string, page, size int32, renderer *Renderer) error {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 50
	}

	req := client.API.TestSuitesAPI.
		SearchTestSuites(client.Ctx, client.Tenant).
		Page(page).
		Size(size)
	if namespace != "" {
		req = req.Namespace(namespace)
	}
	if flowID != "" {
		req = req.FlowId(flowID)
	}

	resp, _, err := req.Execute()
	if err != nil {
		return formatSDKError(err)
	}

	suites := resp.GetResults()
	result := make([]map[string]any, len(suites))
	for i, s := range suites {
		result[i] = map[string]any{
			"id":          s.GetId(),
			"namespace":   s.GetNamespace(),
			"flowId":      s.GetFlowId(),
			"description": s.GetDescription(),
			"disabled":    s.GetDisabled(),
		}
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ID\tNAMESPACE\tFLOW\tDESCRIPTION\tDISABLED")
		for _, row := range result {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%v\n",
				stringify(row["id"]),
				stringify(row["namespace"]),
				stringify(row["flowId"]),
				stringify(row["description"]),
				row["disabled"],
			)
		}
		fmt.Fprintf(w, "\nShowing %d test suite(s) (page %d, total %d)\n", len(result), page, resp.GetTotal())
		return nil
	})
}
