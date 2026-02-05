package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

type iamGroupMembershipOptions struct {
	Group string
	User  string
}

func newIamGroupsAttachCommand() *cobra.Command {
	var opts iamGroupMembershipOptions

	cmd := &cobra.Command{
		Use:   "attach",
		Short: "Add a user to an IAM group.",
		Example: `  # Add a user to a group by ID
  kestra iam groups attach --group grp_123 --user usr_456

  # Add a user to a group by name
  kestra iam groups attach --group ops --user jane.doe`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutputFormat(); err != nil {
				return err
			}

			client, err := NewClient()
			if err != nil {
				return err
			}

			if err := validateIamGroupMembershipFlags(opts); err != nil {
				return err
			}

			return runIamGroupsAttach(client, opts)
		},
	}

	cmd.Flags().StringVar(&opts.Group, "group", "", "Group ID or name (required)")
	cmd.Flags().StringVar(&opts.User, "user", "", "User ID or name (required)")

	_ = cmd.MarkFlagRequired("group")
	_ = cmd.MarkFlagRequired("user")

	return cmd
}

func newIamGroupsDetachCommand() *cobra.Command {
	var opts iamGroupMembershipOptions

	cmd := &cobra.Command{
		Use:   "detach",
		Short: "Remove a user from an IAM group.",
		Example: `  # Remove a user from a group by ID
  kestra iam groups detach --group grp_123 --user usr_456

  # Remove a user from a group by name
  kestra iam groups detach --group ops --user jane.doe`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutputFormat(); err != nil {
				return err
			}

			client, err := NewClient()
			if err != nil {
				return err
			}

			if err := validateIamGroupMembershipFlags(opts); err != nil {
				return err
			}

			return runIamGroupsDetach(client, opts)
		},
	}

	cmd.Flags().StringVar(&opts.Group, "group", "", "Group ID or name (required)")
	cmd.Flags().StringVar(&opts.User, "user", "", "User ID or name (required)")

	_ = cmd.MarkFlagRequired("group")
	_ = cmd.MarkFlagRequired("user")

	return cmd
}

func validateIamGroupMembershipFlags(opts iamGroupMembershipOptions) error {
	group := strings.TrimSpace(opts.Group)
	user := strings.TrimSpace(opts.User)

	if group == "" {
		return fmt.Errorf("group is required")
	}
	if user == "" {
		return fmt.Errorf("user is required")
	}

	return nil
}

func runIamGroupsAttach(client *Client, opts iamGroupMembershipOptions) error {
	group, err := resolveIamGroupIdentifier(client, opts.Group)
	if err != nil {
		return err
	}

	user, err := resolveIamUserIdentifier(client, opts.User)
	if err != nil {
		return err
	}

	_, _, err = client.API.GroupsAPI.AddUserToGroup(client.Ctx, group.ID, user.ID, client.Tenant).Execute()
	if err != nil {
		return formatSDKError(err)
	}

	return printIamGroupMembershipResult("attach", group, user)
}

func runIamGroupsDetach(client *Client, opts iamGroupMembershipOptions) error {
	group, err := resolveIamGroupIdentifier(client, opts.Group)
	if err != nil {
		return err
	}

	user, err := resolveIamUserIdentifier(client, opts.User)
	if err != nil {
		return err
	}

	_, _, err = client.API.GroupsAPI.DeleteUserFromGroup(client.Ctx, group.ID, user.ID, client.Tenant).Execute()
	if err != nil {
		return formatSDKError(err)
	}

	return printIamGroupMembershipResult("detach", group, user)
}

func printIamGroupMembershipResult(action string, group *iamResolvedIdentifier, user *iamResolvedUser) error {
	result := map[string]any{
		"action": strings.ToLower(action),
		"group": map[string]any{
			"id":   group.ID,
			"name": group.Name,
		},
		"user": map[string]any{
			"id":          user.ID,
			"name":        user.Name,
			"displayName": user.DisplayName,
		},
	}

	renderer, err := NewRendererFromFlags(nil)
	if err != nil {
		return err
	}
	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ACTION\tGROUP_ID\tGROUP_NAME\tUSER_ID\tUSERNAME\tDISPLAY_NAME")
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			strings.ToUpper(action),
			withFallback(group.ID),
			withFallback(group.Name),
			withFallback(user.ID),
			withFallback(user.Username),
			withFallback(user.DisplayName),
		)
		return nil
	})
}
