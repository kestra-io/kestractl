package cli

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"

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
