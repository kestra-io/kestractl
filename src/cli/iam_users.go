package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

type iamUserCreateOptions struct {
	Email         string
	FirstName     string
	LastName      string
	Password      string
	SuperAdmin    bool
	SuperAdminSet bool
	Restricted    bool
	RestrictedSet bool
}

func newIamUsersCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "users",
		Short: "Manage IAM users",
	}

	cmd.AddCommand(newIamUsersCreateCommand())
	cmd.AddCommand(newIamUsersListCommand())
	cmd.AddCommand(newIamUsersDeleteCommand())

	return cmd
}

func newIamUsersCreateCommand() *cobra.Command {
	var opts iamUserCreateOptions

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an IAM user.",
		Long: `Create an IAM user with required fields only.

Optional flags can set names, password, or admin attributes.`,
		Example: `  # Create a user with the required email
  kestra iam users create --email user@example.com

  # Create a user with names
  kestra iam users create --email user@example.com --first-name Jane --last-name Doe

  # Create a super admin user
  kestra iam users create --email admin@example.com --super-admin`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutputFormat(); err != nil {
				return err
			}

			client, err := NewClient()
			if err != nil {
				return err
			}

			opts.SuperAdminSet = cmd.Flags().Changed("super-admin")
			opts.RestrictedSet = cmd.Flags().Changed("restricted")

			return runIamUsersCreate(client, opts)
		},
	}

	cmd.Flags().StringVar(&opts.Email, "email", "", "Email address for the user (required)")
	cmd.Flags().StringVar(&opts.FirstName, "first-name", "", "First name for the user")
	cmd.Flags().StringVar(&opts.LastName, "last-name", "", "Last name for the user")
	cmd.Flags().StringVar(&opts.Password, "password", "", "Password for the user")
	cmd.Flags().BoolVar(&opts.SuperAdmin, "super-admin", false, "Grant super admin privileges")
	cmd.Flags().BoolVar(&opts.Restricted, "restricted", false, "Restrict access to assigned tenants")

	_ = cmd.MarkFlagRequired("email")

	return cmd
}

func newIamUsersListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List IAM users.",
		Long:    "List IAM users in the current tenant.",
		Example: "  kestra iam users list\n\n  kestra iam users list --output json",
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

			return runIamUsersList(client)
		},
	}

	return cmd
}

func newIamUsersDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete an IAM user.",
		Long: `Delete an IAM user by ID.

This action is immediate and cannot be undone.`,
		Example: `  # Delete a user by ID
  kestra iam users delete usr_12345`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutputFormat(); err != nil {
				return err
			}

			client, err := NewClient()
			if err != nil {
				return err
			}

			return runIamUsersDelete(client, args[0])
		},
	}

	return cmd
}

func runIamUsersCreate(client *Client, opts iamUserCreateOptions) error {
	return fmt.Errorf("not implemented")
}

func runIamUsersList(client *Client) error {
	return fmt.Errorf("not implemented")
}

func runIamUsersDelete(client *Client, id string) error {
	return fmt.Errorf("not implemented")
}
