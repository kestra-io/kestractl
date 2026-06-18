package cli

import (
	"fmt"
	"text/tabwriter"

	kestra "github.com/kestra-io/client-sdk/go-sdk/kestra_api_client"
	"github.com/spf13/cobra"
)

func newNamespacesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "namespaces",
		Short: "Manage namespaces (create, list, delete)",
	}

	cmd.AddCommand(newNamespacesListCommand())
	cmd.AddCommand(newNamespacesGetCommand())
	cmd.AddCommand(newNamespacesCreateCommand())
	cmd.AddCommand(newNamespacesDeleteCommand())

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
	  kestractl namespaces list

	  # Filter namespaces with a search query
	  kestractl namespaces list --query my.namespace

	  # List namespaces as JSON
	  kestractl namespaces list --output json`,
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
				"id": ns.ID,
			}
		}

		return renderer.Render(jsonResults, func(w *tabwriter.Writer) error {
			fmt.Fprintln(w, "ID")
			for _, ns := range fallback {
				fmt.Fprintf(w, "%s\n", ns.ID)
			}
			fmt.Fprintf(w, "\nTotal namespaces: %d\n", len(fallback))
			return nil
		})
	}

	results := resp.GetResults()

	jsonResults := make([]map[string]any, len(results))
	for i, ns := range results {
		jsonResults[i] = map[string]any{
			"id": ns.GetId(),
		}
	}

	return renderer.Render(jsonResults, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ID")
		for _, ns := range results {
			fmt.Fprintf(w, "%s\n", ns.GetId())
		}
		fmt.Fprintf(w, "\nTotal namespaces: %d\n", len(results))
		return nil
	})
}

func newNamespacesGetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <namespace_id>",
		Short: "Get a namespace by ID.",
		Example: `  kestractl namespaces get my.namespace
  kestractl namespaces get my.namespace --output json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runNamespacesGet(client, args[0], renderer)
		},
	}
	return cmd
}

func runNamespacesGet(client *Client, id string, renderer *Renderer) error {
	ns, _, err := client.API.NamespacesAPI.Namespace(client.Ctx, id, client.Tenant).Execute()
	if err != nil {
		return formatSDKError(err)
	}
	if ns == nil {
		ns = kestra.NewNamespaceWithDefaults()
	}

	result := map[string]any{
		"id":          ns.GetId(),
		"description": ns.GetDescription(),
		"deleted":     ns.GetDeleted(),
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "ID\t%s\n", ns.GetId())
		if desc := ns.GetDescription(); desc != "" {
			fmt.Fprintf(w, "DESCRIPTION\t%s\n", desc)
		}
		fmt.Fprintf(w, "DELETED\t%v\n", ns.GetDeleted())
		return nil
	})
}

func newNamespacesCreateCommand() *cobra.Command {
	var description string

	cmd := &cobra.Command{
		Use:   "create <namespace_id>",
		Short: "Create a namespace.",
		Example: `  kestractl namespaces create my.namespace
  kestractl namespaces create my.namespace --description "My team namespace"
  kestractl namespaces create my.namespace --output json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runNamespacesCreate(client, args[0], description, renderer)
		},
	}

	cmd.Flags().StringVar(&description, "description", "", "Namespace description")
	return cmd
}

func runNamespacesCreate(client *Client, id, description string, renderer *Renderer) error {
	ns := kestra.NewNamespace(id, false)
	if description != "" {
		ns.SetDescription(description)
	}

	created, _, err := client.API.NamespacesAPI.
		CreateNamespace(client.Ctx, client.Tenant).
		Namespace(*ns).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}
	if created == nil {
		created = ns
	}

	result := map[string]any{
		"id":          created.GetId(),
		"description": created.GetDescription(),
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "ID\t%s\n", created.GetId())
		if desc := created.GetDescription(); desc != "" {
			fmt.Fprintf(w, "DESCRIPTION\t%s\n", desc)
		}
		fmt.Fprintln(w, "\nNamespace created.")
		return nil
	})
}

func newNamespacesDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <namespace_id>",
		Short: "Delete a namespace.",
		Example: `  kestractl namespaces delete my.namespace`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runNamespacesDelete(client, args[0], renderer)
		},
	}
	return cmd
}

func runNamespacesDelete(client *Client, id string, renderer *Renderer) error {
	_, err := client.API.NamespacesAPI.DeleteNamespace(client.Ctx, id, client.Tenant).Execute()
	if err != nil {
		return formatSDKError(err)
	}

	result := map[string]any{"id": id, "deleted": true}
	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Namespace '%s' deleted.\n", id)
		return nil
	})
}
