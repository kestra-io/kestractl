package cli

import (
	"fmt"
	"strings"

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
	return nil
}

func runIamGroupsDetach(client *Client, opts iamGroupMembershipOptions) error {
	return nil
}
