package cli

import (
	kestra "github.com/kestra-io/client-sdk/go-sdk/kestra_api_client"
	"github.com/spf13/cobra"
)

type iamRoleCreateOptions struct {
	Name             string
	Description      string
	Default          bool
	DefaultSet       bool
	PermissionValues []string
	Permissions      kestra.IAMRoleControllerApiRoleCreateOrUpdateRequestPermissions
}

func newIamRolesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "roles",
		Short: "Manage IAM roles",
	}

	cmd.AddCommand(newIamRolesCreateCommand())
	cmd.AddCommand(newIamRolesListCommand())
	cmd.AddCommand(newIamRolesDeleteCommand())

	return cmd
}

func newIamRolesCreateCommand() *cobra.Command {
	var opts iamRoleCreateOptions

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an IAM role.",
		Long: `Create an IAM role with required name and permissions.

Permissions must be specified as RESOURCE:ACTION pairs and can be repeated.`,
		Example: `  # Create a role with permissions
  kestra iam roles create --name ops --permission FLOW:READ --permission NAMESPACE:READ`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutputFormat(); err != nil {
				return err
			}

			client, err := NewClient()
			if err != nil {
				return err
			}

			opts.DefaultSet = cmd.Flags().Changed("default")

			permissions, err := parseIAMRolePermissions(opts.PermissionValues)
			if err != nil {
				return err
			}
			opts.Permissions = permissions

			return runIamRolesCreate(client, opts)
		},
	}

	cmd.Flags().StringVar(&opts.Name, "name", "", "Name for the role (required)")
	cmd.Flags().StringVar(&opts.Description, "description", "", "Description for the role")
	cmd.Flags().BoolVar(&opts.Default, "default", false, "Mark the role as default")
	cmd.Flags().StringArrayVar(&opts.PermissionValues, "permission", []string{}, "Permission in RESOURCE:ACTION format (repeatable)")

	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("permission")

	return cmd
}

func newIamRolesListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List IAM roles.",
		Long:    "List IAM roles in the current tenant.",
		Example: "  kestra iam roles list\n\n  kestra iam roles list --output json",
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

			return runIamRolesList(client)
		},
	}

	return cmd
}

func newIamRolesDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete an IAM role.",
		Long: `Delete an IAM role by ID.

This action is immediate and cannot be undone.`,
		Example: "  kestra iam roles delete role_12345",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutputFormat(); err != nil {
				return err
			}

			client, err := NewClient()
			if err != nil {
				return err
			}

			return runIamRolesDelete(client, args[0])
		},
	}

	return cmd
}

func runIamRolesCreate(client *Client, opts iamRoleCreateOptions) error {
	return nil
}

func runIamRolesList(client *Client) error {
	return nil
}

func runIamRolesDelete(client *Client, id string) error {
	return nil
}
