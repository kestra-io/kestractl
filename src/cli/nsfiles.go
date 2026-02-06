package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newNamespaceFilesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "nsfiles",
		Short: "Manage namespace files",
	}

	cmd.AddCommand(newNamespaceFilesListCommand())

	return cmd
}

func newNamespaceFilesListCommand() *cobra.Command {
	var path string
	var recursive bool

	cmd := &cobra.Command{
		Use:   "list <namespace>",
		Short: "List namespace files.",
		Long: `List files and directories within a namespace.

Supports listing the root or a specific directory path, with optional recursion.`,
		Example: `  # List files at the namespace root
  kestra nsfiles list my.namespace

  # List files in a directory
  kestra nsfiles list my.namespace --path workflows/

  # List files recursively
  kestra nsfiles list my.namespace --path workflows/ --recursive

  # List files with JSON output
  kestra nsfiles list my.namespace --output json`,
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

			return runNamespaceFilesList(client, args[0], path, recursive, renderer)
		},
	}

	cmd.Flags().StringVar(&path, "path", "", "Path within the namespace")
	cmd.Flags().BoolVar(&recursive, "recursive", false, "List files recursively")

	return cmd
}

func runNamespaceFilesList(client *Client, namespace, path string, recursive bool, renderer *Renderer) error {
	return fmt.Errorf("namespace files list not implemented")
}
