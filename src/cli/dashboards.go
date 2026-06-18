package cli

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func newDashboardsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dashboards",
		Short: "Manage Kestra dashboards (list, get, create, update, delete)",
		Long: `Manage Kestra dashboards in your tenant.

Dashboards provide configurable views of execution metrics and flow data. Requires Kestra Enterprise Edition.`,
	}

	cmd.AddCommand(newDashboardsListCommand())
	cmd.AddCommand(newDashboardsGetCommand())
	cmd.AddCommand(newDashboardsCreateCommand())
	cmd.AddCommand(newDashboardsUpdateCommand())
	cmd.AddCommand(newDashboardsDeleteCommand())

	return cmd
}

func newDashboardsListCommand() *cobra.Command {
	var (
		query string
		page  int32
		size  int32
		sort  []string
	)

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List dashboards.",
		Aliases: []string{"ls"},
		Example: `  kestractl dashboards list
  kestractl dashboards list --query my-dashboard --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runDashboardsList(client, query, page, size, sort, renderer)
		},
	}

	cmd.Flags().StringVarP(&query, "query", "q", "", "Filter by query string")
	cmd.Flags().Int32Var(&page, "page", 1, "Page number")
	cmd.Flags().Int32Var(&size, "size", 100, "Page size")
	cmd.Flags().StringArrayVar(&sort, "sort", nil, "Sort expression (repeatable)")

	return cmd
}

func runDashboardsList(client *Client, query string, page, size int32, sort []string, renderer *Renderer) error {
	pageInt, sizeInt := int(page), int(size)

	var qPtr *string
	if query != "" {
		qPtr = &query
	}

	resp, err := client.Kestra.Dashboards().SearchDashboards(client.Ctx, client.Tenant, &pageInt, &sizeInt, qPtr, sort)
	if err != nil {
		return formatSDKError(err)
	}

	results := resp.GetResults()

	return renderer.Render(results, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ID\tTITLE\tDELETED")
		for _, d := range results {
			fmt.Fprintf(w, "%s\t%s\t%v\n", d.GetId(), d.GetTitle(), d.GetDeleted())
		}
		fmt.Fprintf(w, "\nTotal dashboards: %d\n", resp.GetTotal())
		return nil
	})
}

func newDashboardsGetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get <id>",
		Short:   "Get dashboard details.",
		Aliases: []string{"show", "describe"},
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
			return runDashboardsGet(client, args[0], renderer)
		},
	}
	return cmd
}

func runDashboardsGet(client *Client, id string, renderer *Renderer) error {
	d, err := client.Kestra.Dashboards().Dashboard(client.Ctx, id, client.Tenant)
	if err != nil {
		return formatSDKError(err)
	}

	return renderer.Render(d, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "Dashboard Details")
		fmt.Fprintln(w)
		fmt.Fprintf(w, "ID:\t%s\n", d.GetId())
		fmt.Fprintf(w, "Title:\t%s\n", d.GetTitle())
		if desc := d.GetDescription(); desc != "" {
			fmt.Fprintf(w, "Description:\t%s\n", desc)
		}
		fmt.Fprintf(w, "Deleted:\t%v\n", d.GetDeleted())
		return nil
	})
}

func newDashboardsCreateCommand() *cobra.Command {
	var filePath string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new dashboard from a YAML file.",
		Example: `  kestractl dashboards create --file my-dashboard.yml
  kestractl dashboards create --file my-dashboard.yml --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if filePath == "" {
				return fmt.Errorf("--file is required")
			}
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runDashboardsCreate(client, filePath, renderer)
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Path to the YAML dashboard definition file (required)")
	return cmd
}

func runDashboardsCreate(client *Client, filePath string, renderer *Renderer) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	d, err := client.Kestra.Dashboards().CreateDashboard(client.Ctx, client.Tenant, string(data))
	if err != nil {
		return formatSDKError(err)
	}

	return renderer.Render(d, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Dashboard created successfully.\n\nID:\t%s\nTitle:\t%s\n",
			d.GetId(), d.GetTitle())
		return nil
	})
}

func newDashboardsUpdateCommand() *cobra.Command {
	var filePath string

	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update an existing dashboard from a YAML file.",
		Args:  cobra.ExactArgs(1),
		Example: `  kestractl dashboards update <id> --file my-dashboard.yml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if filePath == "" {
				return fmt.Errorf("--file is required")
			}
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runDashboardsUpdate(client, args[0], filePath, renderer)
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Path to the YAML dashboard definition file (required)")
	return cmd
}

func runDashboardsUpdate(client *Client, id, filePath string, renderer *Renderer) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	d, err := client.Kestra.Dashboards().UpdateDashboard(client.Ctx, id, client.Tenant, string(data))
	if err != nil {
		return formatSDKError(err)
	}

	return renderer.Render(d, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Dashboard updated successfully.\n\nID:\t%s\nTitle:\t%s\n",
			d.GetId(), d.GetTitle())
		return nil
	})
}

func newDashboardsDeleteCommand() *cobra.Command {
	var skipConfirm bool

	cmd := &cobra.Command{
		Use:     "delete <id>",
		Short:   "Delete a dashboard.",
		Aliases: []string{"rm"},
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
			return runDashboardsDelete(client, args[0], skipConfirm, cmd.InOrStdin(), renderer)
		},
	}

	cmd.Flags().BoolVarP(&skipConfirm, "yes", "y", false, "Skip the confirmation prompt")
	return cmd
}

func runDashboardsDelete(client *Client, id string, skipConfirm bool, in io.Reader, renderer *Renderer) error {
	if !skipConfirm {
		confirmed, err := confirm(in, os.Stderr,
			fmt.Sprintf("Are you sure you want to delete dashboard '%s'? [y/N]: ", id))
		if err != nil {
			return err
		}
		if !confirmed {
			return renderStatus(renderer, "Cancelled.", map[string]any{"id": id, "status": "cancelled"})
		}
	}

	if err := client.Kestra.Dashboards().DeleteDashboard(client.Ctx, id, client.Tenant); err != nil {
		return formatSDKError(err)
	}

	return renderStatus(renderer, fmt.Sprintf("Dashboard '%s' deleted.", id),
		map[string]any{"id": id, "status": "deleted"})
}
