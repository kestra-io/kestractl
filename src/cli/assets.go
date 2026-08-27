package cli

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	kestra "github.com/kestra-io/client-sdk/go-sdk/v2/kestra_api_client"
	"github.com/spf13/cobra"
)

func newAssetsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "assets",
		Short: "Manage Kestra assets (list, get, create, delete)",
		Long: `Manage Kestra assets in your tenant.

Assets are data-catalog entities tracked for lineage and dependency analysis. Requires Kestra Enterprise Edition.`,
	}

	cmd.AddCommand(newAssetsListCommand())
	cmd.AddCommand(newAssetsGetCommand())
	cmd.AddCommand(newAssetsCreateCommand())
	cmd.AddCommand(newAssetsDeleteCommand())
	cmd.AddCommand(newAssetsDependenciesCommand())
	cmd.AddCommand(newAssetsDeleteByIdsCommand())
	cmd.AddCommand(newAssetsDeleteByQueryCommand())
	cmd.AddCommand(newAssetsLineageEventsCommand())
	cmd.AddCommand(newAssetsUsagesCommand())

	return cmd
}

func newAssetsListCommand() *cobra.Command {
	var (
		page int32
		size int32
		sort []string
	)

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List assets.",
		Aliases: []string{"ls"},
		Example: `  kestractl assets list
  kestractl assets list --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runAssetsList(client, page, size, sort, renderer)
		},
	}

	cmd.Flags().Int32Var(&page, "page", 1, "Page number")
	cmd.Flags().Int32Var(&size, "size", 100, "Page size")
	cmd.Flags().StringArrayVar(&sort, "sort", nil, "Sort expression (repeatable)")

	return cmd
}

func runAssetsList(client *Client, page, size int32, sort []string, renderer *Renderer) error {
	pageInt, sizeInt := int(page), int(size)

	resp, err := client.Kestra.Assets().SearchAssets(client.Ctx, client.Tenant, &pageInt, &sizeInt, sort, nil)
	if err != nil {
		return formatSDKError(err)
	}

	results := resp.GetResults()

	return renderer.Render(results, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ID\tNAMESPACE\tTYPE\tDISPLAY NAME")
		for _, a := range results {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				a.GetId(), a.GetNamespace(), a.GetType(), a.GetDisplayName())
		}
		fmt.Fprintf(w, "\nTotal assets: %d\n", resp.GetTotal())
		return nil
	})
}

func newAssetsGetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get <id>",
		Short:   "Get asset details.",
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
			return runAssetsGet(client, args[0], renderer)
		},
	}
	return cmd
}

func runAssetsGet(client *Client, id string, renderer *Renderer) error {
	a, err := client.Kestra.Assets().Asset(client.Ctx, id, client.Tenant, nil)
	if err != nil {
		return formatSDKError(err)
	}

	return renderer.Render(a, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "Asset Details")
		fmt.Fprintln(w)
		fmt.Fprintf(w, "ID:\t%s\n", a.GetId())
		fmt.Fprintf(w, "Namespace:\t%s\n", a.GetNamespace())
		fmt.Fprintf(w, "Type:\t%s\n", a.GetType())
		fmt.Fprintf(w, "Display Name:\t%s\n", a.GetDisplayName())
		if desc := a.GetDescription(); desc != "" {
			fmt.Fprintf(w, "Description:\t%s\n", desc)
		}
		fmt.Fprintf(w, "Deleted:\t%v\n", a.GetDeleted())
		return nil
	})
}

func newAssetsCreateCommand() *cobra.Command {
	var filePath string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new asset from a YAML file.",
		Example: `  kestractl assets create --file my-asset.yml
  kestractl assets create --file my-asset.yml --output json`,
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
			return runAssetsCreate(client, filePath, renderer)
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Path to the YAML asset definition file (required)")
	return cmd
}

func runAssetsCreate(client *Client, filePath string, renderer *Renderer) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	a, err := client.Kestra.Assets().CreateAsset(client.Ctx, client.Tenant, string(data))
	if err != nil {
		return formatSDKError(err)
	}

	return renderer.Render(a, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Asset created successfully.\n\nID:\t%s\nNamespace:\t%s\nType:\t%s\n",
			a.GetId(), a.GetNamespace(), a.GetType())
		return nil
	})
}

func newAssetsDeleteCommand() *cobra.Command {
	var skipConfirm bool

	cmd := &cobra.Command{
		Use:     "delete <id>",
		Short:   "Delete an asset.",
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
			return runAssetsDelete(client, args[0], skipConfirm, cmd.InOrStdin(), renderer)
		},
	}

	cmd.Flags().BoolVarP(&skipConfirm, "yes", "y", false, "Skip the confirmation prompt")
	return cmd
}

func runAssetsDelete(client *Client, id string, skipConfirm bool, in io.Reader, renderer *Renderer) error {
	if !skipConfirm {
		confirmed, err := confirm(in, os.Stderr,
			fmt.Sprintf("Are you sure you want to delete asset '%s'? [y/N]: ", id))
		if err != nil {
			return err
		}
		if !confirmed {
			return renderStatus(renderer, "Cancelled.", map[string]any{"id": id, "status": "cancelled"})
		}
	}

	if err := client.Kestra.Assets().DeleteAsset(client.Ctx, id, client.Tenant); err != nil {
		return formatSDKError(err)
	}

	return renderStatus(renderer, fmt.Sprintf("Asset '%s' deleted.", id),
		map[string]any{"id": id, "status": "deleted"})
}

func newAssetsDependenciesCommand() *cobra.Command {
	var (
		destinationOnly bool
		expandAll       bool
	)

	cmd := &cobra.Command{
		Use:     "dependencies <id>",
		Short:   "Show the dependency graph of an asset.",
		Aliases: []string{"deps"},
		Args:    cobra.ExactArgs(1),
		Example: `  kestractl assets dependencies <id>
  kestractl assets dependencies <id> --expand-all --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runAssetsDependencies(client, args[0], destinationOnly, expandAll, renderer)
		},
	}

	cmd.Flags().BoolVar(&destinationOnly, "destination-only", false, "Only include downstream (destination) dependencies")
	cmd.Flags().BoolVar(&expandAll, "expand-all", false, "Expand all transitive dependencies")
	return cmd
}

func runAssetsDependencies(client *Client, id string, destinationOnly, expandAll bool, renderer *Renderer) error {
	graph, err := client.Kestra.Assets().AssetDependencies(client.Ctx, id, client.Tenant, &destinationOnly, &expandAll)
	if err != nil {
		return formatSDKError(err)
	}

	return renderer.Render(graph, func(w *tabwriter.Writer) error {
		nodes := graph.GetNodes()
		edges := graph.GetEdges()
		fmt.Fprintln(w, "UID\tNAMESPACE\tTYPE")
		for _, n := range nodes {
			fmt.Fprintf(w, "%s\t%s\t%v\n", n.GetUid(), n.GetNamespace(), n.GetType())
		}
		fmt.Fprintf(w, "\nNodes: %d, Edges: %d\n", len(nodes), len(edges))
		fmt.Fprintln(w, "Use --output json to see the full graph.")
		return nil
	})
}

func newAssetsDeleteByIdsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete-by-ids <id> [<id>...]",
		Short: "Delete multiple assets by their IDs.",
		Args:  cobra.MinimumNArgs(1),
		Example: `  kestractl assets delete-by-ids id1 id2 id3
  kestractl assets delete-by-ids id1 id2 --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runAssetsDeleteByIds(client, args, renderer)
		},
	}
	return cmd
}

func runAssetsDeleteByIds(client *Client, ids []string, renderer *Renderer) error {
	resp, err := client.Kestra.Assets().DeleteAssetsByIds(client.Ctx, client.Tenant, ids)
	if err != nil {
		return formatSDKError(err)
	}
	return renderAssetsBulkResponse(resp, renderer)
}

func newAssetsDeleteByQueryCommand() *cobra.Command {
	var (
		filterFlags byQueryFilterFlags
		purge       bool
	)

	cmd := &cobra.Command{
		Use:   "delete-by-query",
		Short: "Delete assets matching query filters.",
		Example: `  kestractl assets delete-by-query --namespace my.namespace
  kestractl assets delete-by-query --filter NAMESPACE:EQUALS:my.namespace --purge`,
		RunE: func(cmd *cobra.Command, args []string) error {
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
			return runAssetsDeleteByQuery(client, filters, purge, renderer)
		},
	}

	addByQueryFilterFlags(cmd, &filterFlags)
	cmd.Flags().BoolVar(&purge, "purge", false, "Permanently purge the matched assets instead of soft-deleting")
	return cmd
}

func runAssetsDeleteByQuery(client *Client, filters []kestra.QueryFilter, purge bool, renderer *Renderer) error {
	resp, err := client.Kestra.Assets().DeleteAssetsByQuery(client.Ctx, client.Tenant, queryFiltersToSearchFilters(filters), &purge)
	if err != nil {
		return formatSDKError(err)
	}
	return renderAssetsBulkResponse(resp, renderer)
}

func renderAssetsBulkResponse(resp *kestra.BulkResponse, renderer *Renderer) error {
	count := resp.GetCount()
	return renderer.Render(map[string]any{"count": count}, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "%d asset(s) affected.\n", count)
		return nil
	})
}

func newAssetsLineageEventsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "lineage-events",
		Short:   "Inspect and manage asset lineage events.",
		Aliases: []string{"lineage"},
	}
	cmd.AddCommand(newAssetsLineageEventsListCommand())
	cmd.AddCommand(newAssetsLineageEventsDeleteByQueryCommand())
	return cmd
}

func newAssetsLineageEventsListCommand() *cobra.Command {
	var (
		page int32
		size int32
		sort []string
	)

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List asset lineage events.",
		Aliases: []string{"ls"},
		Example: `  kestractl assets lineage-events list
  kestractl assets lineage-events list --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runAssetsLineageEventsList(client, page, size, sort, renderer)
		},
	}

	cmd.Flags().Int32Var(&page, "page", 1, "Page number")
	cmd.Flags().Int32Var(&size, "size", 100, "Page size")
	cmd.Flags().StringArrayVar(&sort, "sort", nil, "Sort expression (repeatable)")
	return cmd
}

func runAssetsLineageEventsList(client *Client, page, size int32, sort []string, renderer *Renderer) error {
	pageInt, sizeInt := int(page), int(size)

	resp, err := client.Kestra.Assets().SearchAssetLineageEvents(client.Ctx, client.Tenant, &pageInt, &sizeInt, sort, nil)
	if err != nil {
		return formatSDKError(err)
	}

	results := resp.GetResults()

	return renderer.Render(results, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "UID\tNAMESPACE\tFLOW\tEXECUTION\tSTATE")
		for _, e := range results {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				e.GetUid(), e.GetNamespace(), e.GetFlowId(), e.GetExecutionId(), e.GetState())
		}
		fmt.Fprintf(w, "\nTotal lineage events: %d\n", resp.GetTotal())
		return nil
	})
}

func newAssetsLineageEventsDeleteByQueryCommand() *cobra.Command {
	var filterFlags byQueryFilterFlags

	cmd := &cobra.Command{
		Use:     "delete-by-query",
		Short:   "Delete asset lineage events matching query filters.",
		Example: `  kestractl assets lineage-events delete-by-query --namespace my.namespace`,
		RunE: func(cmd *cobra.Command, args []string) error {
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
			resp, err := client.Kestra.Assets().DeleteAssetLineageEventsByQuery(client.Ctx, client.Tenant, queryFiltersToSearchFilters(filters))
			if err != nil {
				return formatSDKError(err)
			}
			return renderAssetsBulkResponse(resp, renderer)
		},
	}

	addByQueryFilterFlags(cmd, &filterFlags)
	return cmd
}

func newAssetsUsagesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "usages",
		Short:   "Inspect and manage asset usages.",
		Aliases: []string{"usage"},
	}
	cmd.AddCommand(newAssetsUsagesListCommand())
	cmd.AddCommand(newAssetsUsagesDeleteByQueryCommand())
	return cmd
}

func newAssetsUsagesListCommand() *cobra.Command {
	var (
		page int32
		size int32
		sort []string
	)

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List asset usages.",
		Aliases: []string{"ls"},
		Example: `  kestractl assets usages list
  kestractl assets usages list --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runAssetsUsagesList(client, page, size, sort, renderer)
		},
	}

	cmd.Flags().Int32Var(&page, "page", 1, "Page number")
	cmd.Flags().Int32Var(&size, "size", 100, "Page size")
	cmd.Flags().StringArrayVar(&sort, "sort", nil, "Sort expression (repeatable)")
	return cmd
}

func runAssetsUsagesList(client *Client, page, size int32, sort []string, renderer *Renderer) error {
	pageInt, sizeInt := int(page), int(size)

	resp, err := client.Kestra.Assets().SearchAssetUsages(client.Ctx, client.Tenant, &pageInt, &sizeInt, sort, nil)
	if err != nil {
		return formatSDKError(err)
	}

	results := resp.GetResults()

	return renderer.Render(results, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ASSET ID\tNAMESPACE\tFLOW\tEXECUTION\tTASK")
		for _, u := range results {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				u.GetAssetId(), u.GetNamespace(), u.GetFlowId(), u.GetExecutionId(), u.GetTaskId())
		}
		fmt.Fprintf(w, "\nTotal usages: %d\n", resp.GetTotal())
		return nil
	})
}

func newAssetsUsagesDeleteByQueryCommand() *cobra.Command {
	var filterFlags byQueryFilterFlags

	cmd := &cobra.Command{
		Use:     "delete-by-query",
		Short:   "Delete asset usages matching query filters.",
		Example: `  kestractl assets usages delete-by-query --namespace my.namespace`,
		RunE: func(cmd *cobra.Command, args []string) error {
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
			resp, err := client.Kestra.Assets().DeleteAssetUsagesByQuery(client.Ctx, client.Tenant, queryFiltersToSearchFilters(filters))
			if err != nil {
				return formatSDKError(err)
			}
			return renderAssetsBulkResponse(resp, renderer)
		},
	}

	addByQueryFilterFlags(cmd, &filterFlags)
	return cmd
}
