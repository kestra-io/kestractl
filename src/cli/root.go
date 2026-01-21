package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const cliVersion = "0.1.0"

// GlobalFlags holds flags that are available to all commands
type GlobalFlags struct {
	Host   string
	Token  string
	Tenant string
	Output string
}

var globalFlags GlobalFlags

// NewRootCommand builds the root CLI command.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "kestra",
		Short: "Kestra CLI - Manage flows, namespaces, and executions",
		Long: `Kestra CLI is a command-line tool for managing Kestra workflows.

It provides commands to manage flows, namespaces, and executions,
with support for multiple authentication contexts and output formats.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	// Add persistent flags available to all subcommands
	root.PersistentFlags().StringVar(&globalFlags.Host, "host", getEnvOrDefault("KESTRA_HOST", ""), "Kestra host URL")
	root.PersistentFlags().StringVarP(&globalFlags.Token, "token", "t", getEnvOrDefault("KESTRA_TOKEN", ""), "API token")
	root.PersistentFlags().StringVar(&globalFlags.Tenant, "tenant", getEnvOrDefault("KESTRA_TENANT", ""), "Tenant name")
	root.PersistentFlags().StringVarP(&globalFlags.Output, "output", "o", getEnvOrDefault("KESTRA_OUTPUT", "table"), "Output format (table or json)")

	root.AddCommand(newVersionCommand())
	root.AddCommand(newConfigCommand())
	root.AddCommand(newFlowsCommand())
	root.AddCommand(newNamespacesCommand())
	root.AddCommand(newExecutionsCommand())

	return root
}

// getEnvOrDefault returns the environment variable value or a default
func getEnvOrDefault(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
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
