package cli

import (
	"fmt"
	"strings"

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
	cmd.AddCommand(newIamRolesAttachCommand())
	cmd.AddCommand(newIamRolesDetachCommand())

	return cmd
}

func newIamRolesCreateCommand() *cobra.Command {
	var opts iamRoleCreateOptions

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an IAM role.",
		Long: `Create an IAM role with required name and permissions.

Permissions must be specified as RESOURCE:ACTION pairs and can be repeated or passed as a comma-separated list.`,
		Example: `  # Create a role with permissions
  kestra iam roles create --name ops --permission FLOW:READ --permission NAMESPACE:READ

  # Create a role with comma-separated permissions
  kestra iam roles create --name ops --permission FLOW:READ,NAMESPACE:READ`,
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
	cmd.Flags().StringArrayVar(&opts.PermissionValues, "permission", []string{}, "Permission in RESOURCE:ACTION format (repeatable or comma-separated)")

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
	if strings.TrimSpace(opts.Name) == "" {
		return fmt.Errorf("name is required")
	}

	req := kestra.IAMRoleControllerApiRoleCreateOrUpdateRequest{
		Permissions: opts.Permissions,
		Name:        opts.Name,
	}

	if opts.Description != "" {
		req.SetDescription(opts.Description)
	}
	if opts.DefaultSet {
		req.SetIsDefault(opts.Default)
	}

	resp, _, err := client.API.RolesAPI.CreateRole(client.Ctx, client.Tenant).
		IAMRoleControllerApiRoleCreateOrUpdateRequest(req).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	result := map[string]any{
		"id":        resp.GetId(),
		"name":      resp.GetName(),
		"isDefault": resp.GetIsDefault(),
		"isManaged": resp.GetIsManaged(),
	}

	if globalFlags.Output == "json" {
		return printJSON(result)
	}

	w := tabWriter()
	fmt.Fprintln(w, "ID\tName\tDefault\tManaged")
	fmt.Fprintf(w, "%s\t%s\t%t\t%t\n",
		withFallback(resp.GetId()),
		withFallback(resp.GetName()),
		resp.GetIsDefault(),
		resp.GetIsManaged(),
	)
	w.Flush()

	return nil
}

func runIamRolesList(client *Client) error {
	resp, _, err := client.API.RolesAPI.SearchRoles(client.Ctx, client.Tenant).
		Page(1).
		Size(1000).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	results := resp.GetResults()

	if globalFlags.Output == "json" {
		jsonResults := make([]map[string]any, len(results))
		for i, role := range results {
			jsonResults[i] = map[string]any{
				"id":        role.GetId(),
				"name":      role.GetName(),
				"isDefault": role.GetIsDefault(),
				"isManaged": role.GetIsManaged(),
			}
		}
		return printJSON(jsonResults)
	}

	w := tabWriter()
	fmt.Fprintln(w, "ID\tName\tDefault\tManaged")
	for _, role := range results {
		fmt.Fprintf(w, "%s\t%s\t%t\t%t\n",
			withFallback(role.GetId()),
			withFallback(role.GetName()),
			role.GetIsDefault(),
			role.GetIsManaged(),
		)
	}
	w.Flush()

	return nil
}

func runIamRolesDelete(client *Client, id string) error {
	_, err := client.API.RolesAPI.DeleteRole(client.Ctx, id, client.Tenant).Execute()
	if err != nil {
		return formatSDKError(err)
	}

	if globalFlags.Output == "json" {
		return printJSON(map[string]any{
			"id":      id,
			"deleted": true,
		})
	}

	w := tabWriter()
	fmt.Fprintln(w, "ID\tMessage")
	fmt.Fprintf(w, "%s\tDeleted\n", id)
	w.Flush()

	return nil
}
