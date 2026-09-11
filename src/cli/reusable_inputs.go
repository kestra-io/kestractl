package cli

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"

	kestra "github.com/kestra-io/client-sdk/go-sdk/v2/kestra_api_client"
	"github.com/spf13/cobra"
)

func newReusableInputsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "reusable-inputs",
		Aliases: []string{"ri"},
		Short:   "Manage reusable inputs blocks (list, get, revisions, create, update, delete)",
		Long: `Manage namespace-scoped reusable inputs blocks.

A reusable inputs block is a named set of flow input definitions that flows
reference through a REUSABLE_INPUTS input. Requires Kestra Enterprise Edition.

'list' and 'get' resolve namespace inheritance — a block defined in a parent
namespace is visible from its children. 'revisions' and 'delete' do not: the
block must live in the exact namespace given.`,
	}

	cmd.AddCommand(newReusableInputsListCommand())
	cmd.AddCommand(newReusableInputsGetCommand())
	cmd.AddCommand(newReusableInputsRevisionsCommand())
	cmd.AddCommand(newReusableInputsCreateCommand())
	cmd.AddCommand(newReusableInputsUpdateCommand())
	cmd.AddCommand(newReusableInputsDeleteCommand())
	cmd.AddCommand(newReusableInputsNamespacesCommand())

	return cmd
}

func newReusableInputsListCommand() *cobra.Command {
	var (
		page int
		size int
	)

	cmd := &cobra.Command{
		Use:     "list <namespace>",
		Short:   "List the reusable inputs blocks visible from a namespace.",
		Aliases: []string{"ls"},
		Long: `List every reusable inputs block visible from a namespace.

Namespace inheritance is resolved, so blocks defined in parent namespaces are
included — check the NAMESPACE column to see which namespace owns each block.`,
		Example: `  # List the blocks visible from a namespace
  kestractl reusable-inputs list my.namespace

  # Second page, 20 per page, as JSON
  kestractl reusable-inputs list my.namespace --page 2 --size 20 --output json`,
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
			return runReusableInputsList(client, args[0], page, size, renderer)
		},
	}

	cmd.Flags().IntVar(&page, "page", 1, "Page number")
	cmd.Flags().IntVar(&size, "size", 100, "Page size")

	return cmd
}

func runReusableInputsList(client *Client, namespace string, page, size int, renderer *Renderer) error {
	resp, err := client.Kestra.ReusableInputs().ListReusableInputs(client.Ctx, namespace, client.Tenant, &page, &size)
	if err != nil {
		return formatSDKError(err)
	}
	if resp == nil {
		resp = &kestra.PagedResultsReusableInputsWithSource{}
	}

	results := resp.GetResults()
	if results == nil {
		results = []kestra.ReusableInputsWithSource{}
	}

	return renderer.Render(results, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ID\tNAMESPACE\tREVISION\tINPUTS\tDESCRIPTION")
		for _, block := range results {
			description := block.GetDescription()
			if description == "" {
				description = "-"
			}
			fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%s\n",
				block.GetId(), block.GetNamespace(), block.GetRevision(),
				len(block.GetInputs()), description)
		}
		fmt.Fprintf(w, "\nTotal reusable inputs blocks: %d\n", resp.GetTotal())
		return nil
	})
}

func newReusableInputsGetCommand() *cobra.Command {
	var revision int

	cmd := &cobra.Command{
		Use:     "get <namespace> <id>",
		Short:   "Get a reusable inputs block.",
		Aliases: []string{"show", "describe"},
		Long: `Print the YAML source of a reusable inputs block.

Namespace inheritance is resolved, so the block returned may be defined in a
parent namespace — use --output json to see its owning namespace and revision.`,
		Example: `  # Print the latest source
  kestractl reusable-inputs get my.namespace my-block

  # Pin an older revision, with the server-managed fields
  kestractl reusable-inputs get my.namespace my-block --revision 1 --output json`,
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
			var revPtr *int
			if cmd.Flags().Changed("revision") {
				revPtr = &revision
			}
			return runReusableInputsGet(client, args[0], args[1], revPtr, renderer)
		},
	}

	cmd.Flags().IntVar(&revision, "revision", 0, "Revision to retrieve (defaults to the latest)")

	return cmd
}

func runReusableInputsGet(client *Client, namespace, id string, revision *int, renderer *Renderer) error {
	block, err := client.Kestra.ReusableInputs().ReusableInputs(client.Ctx, namespace, id, client.Tenant, revision)
	if err != nil {
		return formatSDKError(err)
	}
	if block == nil {
		return fmt.Errorf("reusable inputs block not found")
	}

	if renderer.IsJSON() {
		return renderer.RenderJSON(block)
	}

	source := block.GetSource()
	if source == "" {
		return fmt.Errorf("reusable inputs block source is empty")
	}
	_, err = io.WriteString(renderer.Writer(), source)
	return err
}

func newReusableInputsRevisionsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "revisions <namespace> <id>",
		Short: "List the revisions of a reusable inputs block.",
		Long: `List every stored revision of a reusable inputs block, oldest first.

Namespace inheritance is not resolved here: the block must be defined in the
namespace given.`,
		Example: `  kestractl reusable-inputs revisions my.namespace my-block

  # JSON output (includes the source of each revision)
  kestractl reusable-inputs revisions my.namespace my-block --output json`,
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
			return runReusableInputsRevisions(client, args[0], args[1], renderer)
		},
	}
	return cmd
}

func runReusableInputsRevisions(client *Client, namespace, id string, renderer *Renderer) error {
	revisions, err := client.Kestra.ReusableInputs().ListReusableInputsRevisions(client.Ctx, namespace, id, client.Tenant)
	if err != nil {
		return formatSDKError(err)
	}

	return renderer.Render(revisions, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "REVISION\tLAST\tUPDATED\tDESCRIPTION")
		for _, rev := range revisions {
			updated := "-"
			if rev.Updated.IsSet() && rev.Updated.Get() != nil {
				updated = rev.Updated.Get().Format(time.RFC3339)
			}
			description := rev.GetDescription()
			if description == "" {
				description = "-"
			}
			fmt.Fprintf(w, "%d\t%t\t%s\t%s\n", rev.GetRevision(), rev.GetLast(), updated, description)
		}
		fmt.Fprintf(w, "\nTotal revisions: %d\n", len(revisions))
		return nil
	})
}

func newReusableInputsCreateCommand() *cobra.Command {
	var filePath string

	cmd := &cobra.Command{
		Use:   "create <namespace> <id>",
		Short: "Create a reusable inputs block from a YAML file.",
		Long: `Create a reusable inputs block from a YAML definition file.

Fails if the block already exists — use 'update' to save a new revision.`,
		Example: `  kestractl reusable-inputs create my.namespace my-block --file my-block.yaml`,
		Args:    cobra.ExactArgs(2),
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
			return runReusableInputsSave(client, args[0], args[1], filePath, true, renderer)
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Path to the YAML reusable inputs definition file (required)")
	return cmd
}

func newReusableInputsUpdateCommand() *cobra.Command {
	var filePath string

	cmd := &cobra.Command{
		Use:   "update <namespace> <id>",
		Short: "Update a reusable inputs block from a YAML file.",
		Long: `Save a new revision of a reusable inputs block from a YAML definition file.

The block is created if it does not exist yet.`,
		Example: `  kestractl reusable-inputs update my.namespace my-block --file my-block.yaml`,
		Args:    cobra.ExactArgs(2),
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
			return runReusableInputsSave(client, args[0], args[1], filePath, false, renderer)
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Path to the YAML reusable inputs definition file (required)")
	return cmd
}

func runReusableInputsSave(client *Client, namespace, id, filePath string, failIfExists bool, renderer *Renderer) error {
	// Sent verbatim: the server stores the bytes as the block's source, so a YAML round trip would reorder keys.
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	block, err := client.Kestra.ReusableInputs().
		CreateOrUpdateReusableInputs(client.Ctx, namespace, id, client.Tenant, string(data), &failIfExists)
	if err != nil {
		return formatSDKError(err)
	}
	if block == nil {
		return fmt.Errorf("reusable inputs block not returned by the server")
	}

	return renderer.Render(block, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Reusable inputs block '%s' saved.\n\nID:\t%s\nNamespace:\t%s\nRevision:\t%d\n",
			block.GetId(), block.GetId(), block.GetNamespace(), block.GetRevision())
		return nil
	})
}

func newReusableInputsDeleteCommand() *cobra.Command {
	var skipConfirm bool

	cmd := &cobra.Command{
		Use:     "delete <namespace> <id>",
		Short:   "Delete a reusable inputs block.",
		Aliases: []string{"rm"},
		Long: `Delete a reusable inputs block.

Namespace inheritance is not resolved here: the block must be defined in the
namespace given.`,
		Example: `  kestractl reusable-inputs delete my.namespace my-block --yes`,
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runReusableInputsDelete(client, args[0], args[1], skipConfirm, cmd.InOrStdin(), renderer)
		},
	}

	cmd.Flags().BoolVarP(&skipConfirm, "yes", "y", false, "Skip the confirmation prompt")
	return cmd
}

func runReusableInputsDelete(client *Client, namespace, id string, skipConfirm bool, in io.Reader, renderer *Renderer) error {
	if !skipConfirm {
		confirmed, err := confirm(in, os.Stderr,
			fmt.Sprintf("Are you sure you want to delete reusable inputs block '%s' in namespace '%s'? [y/N]: ", id, namespace))
		if err != nil {
			return err
		}
		if !confirmed {
			return renderStatus(renderer, "Cancelled.",
				map[string]any{"id": id, "namespace": namespace, "status": "cancelled"})
		}
	}

	if err := client.Kestra.ReusableInputs().DeleteReusableInputs(client.Ctx, namespace, id, client.Tenant); err != nil {
		return formatSDKError(err)
	}

	return renderStatus(renderer, fmt.Sprintf("Reusable inputs block '%s' deleted.", id),
		map[string]any{"id": id, "namespace": namespace, "status": "deleted"})
}

func newReusableInputsNamespacesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "namespaces",
		Short:   "List the namespaces that define at least one reusable inputs block.",
		Example: `  kestractl reusable-inputs namespaces --output json`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runReusableInputsNamespaces(client, renderer)
		},
	}
	return cmd
}

func runReusableInputsNamespaces(client *Client, renderer *Renderer) error {
	namespaces, err := client.Kestra.ReusableInputs().ListReusableInputsNamespaces(client.Ctx, client.Tenant)
	if err != nil {
		return formatSDKError(err)
	}

	return renderer.Render(namespaces, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "NAMESPACE")
		for _, ns := range namespaces {
			fmt.Fprintln(w, ns)
		}
		fmt.Fprintf(w, "\nTotal namespaces: %d\n", len(namespaces))
		return nil
	})
}
