package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

type iamGroupCreateOptions struct {
	Name        string
	Description string
}

func newIamGroupsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "groups",
		Short: "Manage IAM groups",
	}

	cmd.AddCommand(newIamGroupsCreateCommand())
	cmd.AddCommand(newIamGroupsListCommand())
	cmd.AddCommand(newIamGroupsDeleteCommand())

	return cmd
}

func newIamGroupsCreateCommand() *cobra.Command {
	var opts iamGroupCreateOptions

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an IAM group.",
		Long: `Create an IAM group with required fields only.

Optional flags can set the group description.`,
		Example: `  # Create a group with the required name
  kestra iam groups create --name ops

  # Create a group with a description
  kestra iam groups create --name ops --description "Operations team"`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutputFormat(); err != nil {
				return err
			}

			client, err := NewClient()
			if err != nil {
				return err
			}

			return runIamGroupsCreate(client, opts)
		},
	}

	cmd.Flags().StringVar(&opts.Name, "name", "", "Name for the group (required)")
	cmd.Flags().StringVar(&opts.Description, "description", "", "Description for the group")

	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func newIamGroupsListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List IAM groups.",
		Long:    "List IAM groups in the current tenant.",
		Example: "  kestra iam groups list\n\n  kestra iam groups list --output json",
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutputFormat(); err != nil {
				return err
			}

			client, err := NewClient()
			if err != nil {
				return err
			}

			return runIamGroupsList(client)
		},
	}

	return cmd
}

func newIamGroupsDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete an IAM group.",
		Long: `Delete an IAM group by ID.

This action is immediate and cannot be undone.`,
		Example: "  kestra iam groups delete grp_12345",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutputFormat(); err != nil {
				return err
			}

			client, err := NewClient()
			if err != nil {
				return err
			}

			return runIamGroupsDelete(client, args[0])
		},
	}

	return cmd
}

func runIamGroupsCreate(client *Client, opts iamGroupCreateOptions) error {
	return fmt.Errorf("not implemented")
}

func runIamGroupsList(client *Client) error {
	return fmt.Errorf("not implemented")
}

func runIamGroupsDelete(client *Client, id string) error {
	return fmt.Errorf("not implemented")
}
