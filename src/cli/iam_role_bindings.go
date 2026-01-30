package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

type iamRoleBindingOptions struct {
	Role  string
	User  string
	Group string
}

type iamRoleBindingTarget struct {
	Type string
	ID   string
	Name string
}

func newIamRolesBindingsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bindings",
		Short: "Attach or detach IAM roles to users or groups",
	}

	cmd.AddCommand(newIamRolesBindingsAttachCommand())
	cmd.AddCommand(newIamRolesBindingsDetachCommand())

	return cmd
}

func newIamRolesBindingsAttachCommand() *cobra.Command {
	var opts iamRoleBindingOptions

	cmd := &cobra.Command{
		Use:   "attach",
		Short: "Attach an IAM role to a user or group.",
		Example: `  # Attach a role to a user
  kestra iam roles bindings attach --role ops --user usr_123

  # Attach a role to a group
  kestra iam roles bindings attach --role ops --group grp_456`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutputFormat(); err != nil {
				return err
			}

			client, err := NewClient()
			if err != nil {
				return err
			}

			if err := validateIamRoleBindingFlags(opts); err != nil {
				return err
			}

			return runIamRolesBindingsAttach(client, opts)
		},
	}

	cmd.Flags().StringVar(&opts.Role, "role", "", "Role ID or name to attach (required)")
	cmd.Flags().StringVar(&opts.User, "user", "", "User ID, username, or display name to attach")
	cmd.Flags().StringVar(&opts.Group, "group", "", "Group ID or name to attach")

	_ = cmd.MarkFlagRequired("role")

	return cmd
}

func newIamRolesBindingsDetachCommand() *cobra.Command {
	var opts iamRoleBindingOptions

	cmd := &cobra.Command{
		Use:   "detach",
		Short: "Detach an IAM role from a user or group.",
		Example: `  # Detach a role from a user
  kestra iam roles bindings detach --role ops --user usr_123

  # Detach a role from a group
  kestra iam roles bindings detach --role ops --group grp_456`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutputFormat(); err != nil {
				return err
			}

			client, err := NewClient()
			if err != nil {
				return err
			}

			if err := validateIamRoleBindingFlags(opts); err != nil {
				return err
			}

			return runIamRolesBindingsDetach(client, opts)
		},
	}

	cmd.Flags().StringVar(&opts.Role, "role", "", "Role ID or name to detach (required)")
	cmd.Flags().StringVar(&opts.User, "user", "", "User ID, username, or display name to detach")
	cmd.Flags().StringVar(&opts.Group, "group", "", "Group ID or name to detach")

	_ = cmd.MarkFlagRequired("role")

	return cmd
}

func validateIamRoleBindingFlags(opts iamRoleBindingOptions) error {
	role := strings.TrimSpace(opts.Role)
	user := strings.TrimSpace(opts.User)
	group := strings.TrimSpace(opts.Group)

	if role == "" {
		return fmt.Errorf("role is required")
	}
	if user == "" && group == "" {
		return fmt.Errorf("either --user or --group is required")
	}
	if user != "" && group != "" {
		return fmt.Errorf("only one of --user or --group can be set")
	}

	return nil
}

func runIamRolesBindingsAttach(client *Client, opts iamRoleBindingOptions) error {
	return fmt.Errorf("role binding attach is not implemented")
}

func runIamRolesBindingsDetach(client *Client, opts iamRoleBindingOptions) error {
	return fmt.Errorf("role binding detach is not implemented")
}
