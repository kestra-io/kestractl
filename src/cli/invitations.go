package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	kestra "github.com/kestra-io/client-sdk/go-sdk/kestra_api_client"
	"github.com/spf13/cobra"
)

func newInvitationsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "invitations",
		Short: "Manage IAM invitations (list, get, create, delete)",
		Long: `Manage user invitations in your Kestra instance.

Invitations are tenant-scoped resources. Managing invitations requires Kestra
Enterprise Edition.`,
	}

	cmd.AddCommand(newInvitationsListCommand())
	cmd.AddCommand(newInvitationsListByEmailCommand())
	cmd.AddCommand(newInvitationsGetCommand())
	cmd.AddCommand(newInvitationsCreateCommand())
	cmd.AddCommand(newInvitationsDeleteCommand())

	return cmd
}

func newInvitationsListCommand() *cobra.Command {
	var (
		email  string
		status string
		page   int32
		size   int32
		sort   []string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List invitations.",
		Long:  "List all invitations in the active tenant, optionally filtered by --email or --status.",
		Example: `  # List all invitations
  kestractl invitations list

  # List pending invitations only
  kestractl invitations list --status PENDING

  # List invitations sent to an address
  kestractl invitations list --email jane@example.com

  # List invitations as JSON
  kestractl invitations list --output json`,
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := NewClient()
			if err != nil {
				return err
			}
			return runInvitationsList(client, email, status, page, size, sort, renderer)
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "Filter invitations by invitee email")
	cmd.Flags().StringVar(&status, "status", "", "Filter invitations by status (PENDING, ACCEPTED or EXPIRED)")
	cmd.Flags().Int32Var(&page, "page", 1, "Page number")
	cmd.Flags().Int32Var(&size, "size", 100, "Page size")
	cmd.Flags().StringArrayVar(&sort, "sort", nil, "Sort expression (e.g. 'sentAt:desc', repeatable)")

	return cmd
}

func runInvitationsList(client *Client, email, status string, page, size int32, sort []string, renderer *Renderer) error {
	statusFilter, err := parseInvitationStatus(status)
	if err != nil {
		return err
	}

	var emailFilter *string
	if email != "" {
		emailFilter = &email
	}

	pageParam, sizeParam := int(page), int(size)
	resp, err := client.Kestra.Invitations().SearchInvitations(
		client.Ctx, client.Tenant, &pageParam, &sizeParam, sort, emailFilter, statusFilter, nil)
	if err != nil {
		return formatSDKError(err)
	}
	if resp == nil {
		// The SDK returns (nil, nil) on a 2xx response with an empty body.
		resp = &kestra.PagedResultsIAMInvitationControllerApiInvitationDetail{}
	}

	results := resp.Results
	if results == nil {
		// Render an empty JSON array ([]) rather than null on no results.
		results = []kestra.IAMInvitationControllerApiInvitationDetail{}
	}

	return renderer.Render(results, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ID\tEMAIL\tSTATUS\tSENT AT\tEXPIRED AT\tSUPERADMIN")
		for _, inv := range results {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%t\n",
				inv.GetId(), inv.GetEmail(), inv.GetStatus(),
				formatOptionalTime(inv.GetSentAt()), formatOptionalTime(inv.GetExpiredAt()),
				inv.GetInstanceOwner())
		}
		fmt.Fprintf(w, "\nTotal invitations: %d\n", resp.Total)
		return nil
	})
}

func newInvitationsListByEmailCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list-by-email <email>",
		Short: "List all invitations for an email address.",
		Long:  "Retrieve every invitation sent to the given email address, regardless of status.",
		Example: `  kestractl invitations list-by-email jane@example.com
  kestractl invitations list-by-email jane@example.com --output json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := NewClient()
			if err != nil {
				return err
			}
			return runInvitationsListByEmail(client, args[0], renderer)
		},
	}
	return cmd
}

func runInvitationsListByEmail(client *Client, email string, renderer *Renderer) error {
	results, err := client.Kestra.Invitations().ListInvitationsByEmail(client.Ctx, email, client.Tenant)
	if err != nil {
		return formatSDKError(err)
	}
	if results == nil {
		results = []kestra.IAMInvitationControllerApiInvitationDetail{}
	}
	return renderer.Render(results, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ID\tEMAIL\tSTATUS\tSENT AT\tEXPIRED AT\tSUPERADMIN")
		for _, inv := range results {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%t\n",
				inv.GetId(), inv.GetEmail(), inv.GetStatus(),
				formatOptionalTime(inv.GetSentAt()), formatOptionalTime(inv.GetExpiredAt()),
				inv.GetInstanceOwner())
		}
		fmt.Fprintf(w, "\nTotal invitations: %d\n", len(results))
		return nil
	})
}

func newInvitationsGetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get <invitation_id>",
		Short:   "Get invitation details.",
		Long:    "Retrieve detailed information about a specific invitation, including its status and assigned roles/groups.",
		Aliases: []string{"show", "describe"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := NewClient()
			if err != nil {
				return err
			}
			return runInvitationsGet(client, args[0], renderer)
		},
	}
	return cmd
}

func runInvitationsGet(client *Client, id string, renderer *Renderer) error {
	inv, err := client.Kestra.Invitations().Invitation(client.Ctx, id, client.Tenant)
	if err != nil {
		return formatSDKError(err)
	}
	return renderInvitationDetail(inv, renderer)
}

func renderInvitationDetail(inv *kestra.IAMInvitationControllerApiInvitationDetail, renderer *Renderer) error {
	return renderer.Render(inv, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "Invitation Details")
		fmt.Fprintln(w)
		fmt.Fprintf(w, "ID:\t%s\n", inv.GetId())
		fmt.Fprintf(w, "Email:\t%s\n", inv.GetEmail())
		fmt.Fprintf(w, "Status:\t%s\n", inv.GetStatus())
		fmt.Fprintf(w, "Tenant:\t%s\n", inv.GetTenantId())
		fmt.Fprintf(w, "Sent at:\t%s\n", formatOptionalTime(inv.GetSentAt()))
		fmt.Fprintf(w, "Expired at:\t%s\n", formatOptionalTime(inv.GetExpiredAt()))
		fmt.Fprintf(w, "Accepted at:\t%s\n", formatOptionalTime(inv.GetAcceptedAt()))
		fmt.Fprintf(w, "Superadmin:\t%t\n", inv.GetInstanceOwner())

		if roles := inv.GetRoles(); len(roles) > 0 {
			names := make([]string, 0, len(roles))
			for _, r := range roles {
				names = append(names, r.GetName())
			}
			fmt.Fprintf(w, "Roles:\t%s\n", strings.Join(names, ", "))
		}
		if groups := inv.GetGroups(); len(groups) > 0 {
			names := make([]string, 0, len(groups))
			for _, g := range groups {
				names = append(names, g.GetName())
			}
			fmt.Fprintf(w, "Groups:\t%s\n", strings.Join(names, ", "))
		}
		if link := inv.GetLink(); link != "" {
			fmt.Fprintf(w, "Link:\t%s\n", link)
		}
		return nil
	})
}

func newInvitationsCreateCommand() *cobra.Command {
	var (
		email                string
		roles                []string
		groups               []string
		superAdmin           bool
		createUserIfNotExist bool
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an invitation.",
		Long: `Create and send a user invitation. The --email flag is required.

Roles and groups can be pre-assigned so the invitee receives them upon
acceptance. If the invitee already has a user account (or with
--create-user-if-not-exist), the server grants tenant access directly instead
of sending an invitation.`,
		Example: `  # Invite a user with a role
  kestractl invitations create --email jane@example.com --role admin_role_id

  # Invite a user into groups
  kestractl invitations create --email jane@example.com --group g1 --group g2

  # Grant access directly, creating the user if needed
  kestractl invitations create --email jane@example.com --create-user-if-not-exist`,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := NewClient()
			if err != nil {
				return err
			}
			return runInvitationsCreate(client, email, roles, groups, superAdmin, createUserIfNotExist, renderer)
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "Email address of the invitee")
	cmd.Flags().StringArrayVar(&roles, "role", nil, "Role ID to pre-assign (repeatable)")
	cmd.Flags().StringArrayVar(&groups, "group", nil, "Group ID to pre-assign (repeatable)")
	cmd.Flags().BoolVar(&superAdmin, "superadmin", false, "Grant superadmin to the invitee")
	cmd.Flags().BoolVar(&createUserIfNotExist, "create-user-if-not-exist", false,
		"Create the user and grant access directly instead of sending an invitation")
	_ = cmd.MarkFlagRequired("email")

	return cmd
}

func runInvitationsCreate(client *Client, email string, roles, groups []string, superAdmin, createUserIfNotExist bool, renderer *Renderer) error {
	req := kestra.IAMInvitationControllerApiInvitationCreateRequest{Email: email, Groups: groups}
	for _, roleID := range roles {
		req.Roles = append(req.Roles, kestra.IAMInvitationControllerApiInvitationRole{Id: roleID})
	}
	if superAdmin {
		req.InstanceOwner = &superAdmin
	}
	if createUserIfNotExist {
		req.CreateUserIfNotExist = &createUserIfNotExist
	}

	inv, err := client.Kestra.Invitations().CreateInvitation(client.Ctx, client.Tenant, req)
	if err != nil {
		return formatSDKError(err)
	}

	// A 204 response (nil detail) means the server granted tenant access
	// directly — the invitee already had an account or
	// --create-user-if-not-exist was set — so there is no invitation to show.
	if inv == nil {
		return renderStatus(renderer,
			fmt.Sprintf("Access granted directly to '%s'; no invitation was sent.", email),
			map[string]any{"email": email, "status": "access_granted"})
	}

	return renderInvitationDetail(inv, renderer)
}

func newInvitationsDeleteCommand() *cobra.Command {
	var skipConfirm bool

	cmd := &cobra.Command{
		Use:     "delete <invitation_id>",
		Short:   "Delete an invitation.",
		Long:    "Delete (revoke) an invitation. Prompts for confirmation unless --yes is provided.",
		Aliases: []string{"rm"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := NewClient()
			if err != nil {
				return err
			}
			return runInvitationsDelete(client, args[0], skipConfirm, cmd.InOrStdin(), renderer)
		},
	}

	cmd.Flags().BoolVarP(&skipConfirm, "yes", "y", false, "Skip the confirmation prompt")

	return cmd
}

func runInvitationsDelete(client *Client, id string, skipConfirm bool, in io.Reader, renderer *Renderer) error {
	if !skipConfirm {
		// Prompt on stderr so it never pollutes stdout (e.g. --output json).
		confirmed, err := confirm(in, os.Stderr,
			fmt.Sprintf("Are you sure you want to delete invitation '%s'? [y/N]: ", id))
		if err != nil {
			return err
		}
		if !confirmed {
			return renderStatus(renderer, "Cancelled.", map[string]any{"id": id, "status": "cancelled"})
		}
	}

	if err := client.Kestra.Invitations().DeleteInvitation(client.Ctx, id, client.Tenant); err != nil {
		return formatSDKError(err)
	}

	return renderStatus(renderer, fmt.Sprintf("Invitation '%s' deleted.", id),
		map[string]any{"id": id, "status": "deleted"})
}

// parseInvitationStatus validates an optional --status value against the SDK
// enum. An empty input returns (nil, nil) so it can be passed straight to
// search.
func parseInvitationStatus(s string) (*kestra.InvitationInvitationStatus, error) {
	if s == "" {
		return nil, nil
	}
	parsed, err := kestra.NewInvitationInvitationStatusFromValue(strings.ToUpper(strings.TrimSpace(s)))
	if err != nil {
		return nil, fmt.Errorf("invalid status %q: expected one of %s",
			s, joinEnumValues(kestra.AllowedInvitationInvitationStatusEnumValues))
	}
	return parsed, nil
}
