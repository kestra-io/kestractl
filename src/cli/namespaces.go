package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func newNamespacesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "namespaces",
		Short: "Manage namespaces",
	}

	cmd.AddCommand(newNamespacesListCommand())

	return cmd
}

func newNamespacesListCommand() *cobra.Command {
	var query string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List namespaces.",
		Long: `List all namespaces in your Kestra instance.

Optionally filter results using the --query flag to search for specific namespaces.`,
		Example: `  # List all namespaces
  kestra namespaces list

  # Filter namespaces with a search query
  kestra namespaces list --query my.namespace

  # List namespaces as JSON
  kestra namespaces list --output json`,
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := NewClient()
			if err != nil {
				return err
			}

			return runNamespacesList(client, query, renderer)
		},
	}

	cmd.Flags().StringVarP(&query, "query", "q", "", "Filter namespaces by search query")

	return cmd
}

func runNamespacesList(client *Client, query string, renderer *Renderer) error {
	req := client.API.NamespacesAPI.SearchNamespaces(client.Ctx, client.Tenant).
		Page(1).
		Size(100).
		Existing(true)

	if query != "" {
		req = req.Q(query)
	}

	resp, _, err := req.Execute()
	if err != nil {
		fallback := tryParseNamespaceListFromError(err)
		if fallback == nil {
			return formatSDKError(err)
		}

		jsonResults := make([]map[string]any, len(fallback))
		for i, ns := range fallback {
			jsonResults[i] = map[string]any{
				"id":      ns.ID,
				"deleted": ns.Deleted,
			}
		}

		return renderer.Render(jsonResults, func(w *tabwriter.Writer) error {
			fmt.Fprintln(w, "ID\tDeleted")
			for _, ns := range fallback {
				deleted := "false"
				if ns.Deleted {
					deleted = "true"
				}
				fmt.Fprintf(w, "%s\t%s\n", ns.ID, deleted)
			}
			fmt.Fprintf(w, "\nTotal namespaces: %d\n", len(fallback))
			return nil
		})
	}

	results := resp.GetResults()

	jsonResults := make([]map[string]any, len(results))
	for i, ns := range results {
		jsonResults[i] = map[string]any{
			"id":      ns.GetId(),
			"deleted": ns.GetDeleted(),
		}
	}

	return renderer.Render(jsonResults, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ID\tDeleted")
		for _, ns := range results {
			deleted := "false"
			if ns.GetDeleted() {
				deleted = "true"
			}
			fmt.Fprintf(w, "%s\t%s\n", ns.GetId(), deleted)
		}
		fmt.Fprintf(w, "\nTotal namespaces: %d\n", len(results))
		return nil
	})
}
