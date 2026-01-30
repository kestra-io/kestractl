package cli

import (
	"fmt"
	"strings"

	kestra "github.com/kestra-io/client-sdk/go-sdk/kestra_api_client"
	"github.com/spf13/cobra"
)

type iamUserCreateOptions struct {
	Email         string
	FirstName     string
	LastName      string
	Password      string
	Tenants       []string
	Groups        []string
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

Optional flags can set names, password, admin attributes, or tenant/group assignments.`,
		Example: `  # Create a user with the required email
  kestra iam users create --email user@example.com

  # Create a user with names
  kestra iam users create --email user@example.com --first-name Jane --last-name Doe

  # Create a user and assign tenant and group
  kestra iam users create --email user@example.com --assign-tenant main --group grp_123

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
	cmd.Flags().StringArrayVar(&opts.Tenants, "assign-tenant", []string{}, "Assign tenant(s) to the user (repeatable)")
	cmd.Flags().StringArrayVar(&opts.Groups, "group", []string{}, "Assign group ID(s) to the user (repeatable)")
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
	if strings.TrimSpace(opts.Email) == "" {
		return fmt.Errorf("email is required")
	}

	req := kestra.IAMUserControllerApiCreateOrUpdateUserRequest{Email: opts.Email}
	if opts.FirstName != "" {
		req.SetFirstName(opts.FirstName)
	}
	if opts.LastName != "" {
		req.SetLastName(opts.LastName)
	}
	if opts.Password != "" {
		req.SetPassword(opts.Password)
	}
	if len(opts.Tenants) > 0 {
		req.SetTenants(opts.Tenants)
	} else if strings.TrimSpace(client.Tenant) != "" {
		req.SetTenants([]string{client.Tenant})
	}
	if len(opts.Groups) > 0 {
		req.SetGroups(opts.Groups)
	}
	if opts.SuperAdminSet {
		req.SetSuperAdmin(opts.SuperAdmin)
	}
	if opts.RestrictedSet {
		req.SetRestricted(opts.Restricted)
	}

	resp, _, err := client.API.UsersAPI.CreateUser(client.Ctx).
		IAMUserControllerApiCreateOrUpdateUserRequest(req).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	result := map[string]any{
		"id":          resp.GetId(),
		"username":    resp.GetUsername(),
		"displayName": resp.GetDisplayName(),
	}

	if globalFlags.Output == "json" {
		return printJSON(result)
	}

	w := tabWriter()
	fmt.Fprintln(w, "ID\tUsername\tDisplayName")
	fmt.Fprintf(w, "%s\t%s\t%s\n",
		withFallback(resp.GetId()),
		withFallback(resp.GetUsername()),
		withFallback(resp.GetDisplayName()),
	)
	w.Flush()

	return nil
}

func runIamUsersList(client *Client) error {
	resp, _, err := client.API.UsersAPI.ListUsers(client.Ctx).
		Page(1).
		Size(1000).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	results := resp.GetResults()

	if globalFlags.Output == "json" {
		jsonResults := make([]map[string]any, len(results))
		for i, user := range results {
			jsonResults[i] = map[string]any{
				"id":          user.GetId(),
				"username":    user.GetUsername(),
				"displayName": user.GetDisplayName(),
			}
		}
		return printJSON(jsonResults)
	}

	w := tabWriter()
	fmt.Fprintln(w, "ID\tUsername\tDisplayName")
	for _, user := range results {
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			withFallback(user.GetId()),
			withFallback(user.GetUsername()),
			withFallback(user.GetDisplayName()),
		)
	}
	w.Flush()

	return nil
}

func runIamUsersDelete(client *Client, id string) error {
	_, err := client.API.UsersAPI.DeleteUser(client.Ctx, id).Execute()
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

func withFallback(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}
