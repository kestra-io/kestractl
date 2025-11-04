package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

const cliVersion = "0.1.0"

// NewRootCommand builds the root CLI command.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "kestra",
		Short: "Kestra CLI - Manage flows, namespaces, and executions",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	root.AddCommand(newVersionCommand())
	root.AddCommand(newConfigCommand())
	root.AddCommand(newFlowsCommand())
	root.AddCommand(newNamespacesCommand())
	root.AddCommand(newExecutionsCommand())

	return root
}

// Execute runs the CLI.
func Execute() error {
	return NewRootCommand().Execute()
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show version information.",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Kestra CLI v%s\n", cliVersion)
		},
	}
}
