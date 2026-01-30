package cli

import "github.com/spf13/cobra"

func newIamCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "iam",
		Short: "Manage IAM resources",
	}

	cmd.AddCommand(newIamUsersCommand())

	return cmd
}
