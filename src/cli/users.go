package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	kestra "github.com/kestra-io/client-sdk/go-sdk/v2/kestra_api_client"
	"github.com/spf13/cobra"
)

func newUsersCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "users",
		Short: "Manage IAM users (list, get, create, update, delete)",
		Long: `Manage IAM users in your Kestra instance.

Users are instance-level resources. Managing users requires Kestra Enterprise Edition.`,
	}

	cmd.AddCommand(newUsersListCommand())
	cmd.AddCommand(newUsersGetCommand())
	cmd.AddCommand(newUsersCreateCommand())
	cmd.AddCommand(newUsersUpdateCommand())
	cmd.AddCommand(newUsersDeleteCommand())
	cmd.AddCommand(newUsersSetGroupsCommand())
	cmd.AddCommand(newUsersSetPasswordCommand())
	cmd.AddCommand(newUsersTokensCommand())
	cmd.AddCommand(newUsersAutocompleteCommand())
	cmd.AddCommand(newUsersImpersonateCommand())
	cmd.AddCommand(newUsersRevokeRefreshTokenCommand())
	cmd.AddCommand(newUsersPatchSuperAdminCommand())
	cmd.AddCommand(newUsersSetRestrictedCommand())
	cmd.AddCommand(newUsersDeleteAuthMethodCommand())
	cmd.AddCommand(newUsersChangeMyPasswordCommand())

	return cmd
}

// userMutation holds the fields used to create or update a user, plus a record
// of which flags the operator explicitly set.
//
// The *Set fields matter because the update endpoint is a full-replace PUT: any
// attribute absent from the request body is reset to its default server-side
// (omitting superAdmin demotes the user, omitting groups clears them, etc.).
// So update fetches the current user and overlays only the changed flags via
// applyTo, while create builds a fresh request via toRequest.
type userMutation struct {
	email      string
	firstName  string
	lastName   string
	password   string
	groups     []string
	tenants    []string
	superAdmin bool
	restricted bool

	emailSet      bool
	firstNameSet  bool
	lastNameSet   bool
	passwordSet   bool
	groupsSet     bool
	tenantsSet    bool
	superAdminSet bool
	restrictedSet bool
}

// markChanged records which flags the operator actually passed on the command.
func (m *userMutation) markChanged(cmd *cobra.Command) {
	f := cmd.Flags()
	m.emailSet = f.Changed("email")
	m.firstNameSet = f.Changed("first-name")
	m.lastNameSet = f.Changed("last-name")
	m.passwordSet = f.Changed("user-password")
	m.groupsSet = f.Changed("group")
	m.tenantsSet = f.Changed("tenant-grant")
	m.superAdminSet = f.Changed("superadmin")
	m.restrictedSet = f.Changed("restricted")
}

// toRequest builds a create request from scratch.
func (m userMutation) toRequest() kestra.IAMUserControllerApiCreateOrUpdateUserRequest {
	req := kestra.IAMUserControllerApiCreateOrUpdateUserRequest{
		Email: m.email,
	}
	m.applyTo(&req)
	return req
}

// applyTo overlays the explicitly-set fields onto req, leaving everything else
// (e.g. an existing user's attributes seeded from a GET) untouched.
func (m userMutation) applyTo(req *kestra.IAMUserControllerApiCreateOrUpdateUserRequest) {
	if m.emailSet {
		req.Email = m.email
	}
	if m.firstNameSet {
		req.FirstName = &m.firstName
	}
	if m.lastNameSet {
		req.LastName = &m.lastName
	}
	if m.passwordSet {
		req.Password = &m.password
	}
	if m.groupsSet {
		req.Groups = m.groups
	}
	if m.tenantsSet {
		req.Tenants = m.tenants
	}
	if m.superAdminSet {
		req.InstanceOwner = &m.superAdmin
	}
	if m.restrictedSet {
		req.Restricted = &m.restricted
	}
}

// userRequestFromExisting seeds a full create/update request from a fetched
// user, so a full-replace PUT preserves attributes the operator didn't touch.
func userRequestFromExisting(u *kestra.IAMUserControllerApiUser) kestra.IAMUserControllerApiCreateOrUpdateUserRequest {
	req := kestra.IAMUserControllerApiCreateOrUpdateUserRequest{Email: u.GetEmail()}
	if u.FirstName != nil {
		req.FirstName = u.FirstName
	}
	if u.LastName != nil {
		req.LastName = u.LastName
	}
	if u.InstanceOwner != nil {
		req.InstanceOwner = u.InstanceOwner
	}
	if u.Restricted != nil {
		req.Restricted = u.Restricted
	}
	if groups := u.GetGroups(); len(groups) > 0 {
		ids := make([]string, 0, len(groups))
		for _, g := range groups {
			ids = append(ids, g.GetId())
		}
		req.Groups = ids
	}
	if tenants := u.GetTenants(); len(tenants) > 0 {
		ids := make([]string, 0, len(tenants))
		for _, tn := range tenants {
			ids = append(ids, tn.GetId())
		}
		req.Tenants = ids
	}
	return req
}

// addMutationFlags wires the create/update flags shared by both commands.
func addMutationFlags(cmd *cobra.Command, m *userMutation) {
	cmd.Flags().StringVar(&m.email, "email", "", "User email (login)")
	cmd.Flags().StringVar(&m.firstName, "first-name", "", "First name")
	cmd.Flags().StringVar(&m.lastName, "last-name", "", "Last name")
	cmd.Flags().StringVar(&m.password, "user-password", "", "Password for the user (avoids clashing with the global --password basic-auth flag)")
	cmd.Flags().StringArrayVar(&m.groups, "group", nil, "Group ID to assign (repeatable)")
	cmd.Flags().StringArrayVar(&m.tenants, "tenant-grant", nil, "Tenant ID to grant access to (repeatable)")
	cmd.Flags().BoolVar(&m.superAdmin, "superadmin", false, "Grant super-admin privileges")
	cmd.Flags().BoolVar(&m.restricted, "restricted", false, "Mark the user as restricted")
}

func newUsersListCommand() *cobra.Command {
	var (
		query string
		page  int32
		size  int32
		sort  []string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List users.",
		Long:  "List all users in your Kestra instance, optionally filtered with --query.",
		Example: `  # List all users
	  kestractl users list

	  # Filter users
	  kestractl users list --query alice

	  # List users as JSON
	  kestractl users list --output json`,
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
			return runUsersList(client, query, page, size, sort, renderer)
		},
	}

	cmd.Flags().StringVarP(&query, "query", "q", "", "Filter users by search query")
	cmd.Flags().Int32Var(&page, "page", 1, "Page number")
	cmd.Flags().Int32Var(&size, "size", 100, "Page size")
	cmd.Flags().StringArrayVar(&sort, "sort", nil, "Sort expression (e.g. 'username:asc', repeatable)")

	return cmd
}

func runUsersList(client *Client, query string, page, size int32, sort []string, renderer *Renderer) error {
	req := client.API.UsersAPI.ListUsers(client.Ctx).Page(page).Size(size)
	if query != "" {
		req = req.Q(query)
	}
	if len(sort) > 0 {
		req = req.Sort(sort)
	}

	resp, _, err := req.Execute()
	if err != nil {
		return formatSDKError(err)
	}

	results := resp.GetResults()
	if results == nil {
		// Render an empty JSON array ([]) rather than null on no results.
		results = []kestra.IAMUserControllerApiUserSummary{}
	}

	return renderer.Render(results, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ID\tUSERNAME\tDISPLAY NAME\tSUPERADMIN")
		for _, u := range results {
			fmt.Fprintf(w, "%s\t%s\t%s\t%t\n",
				u.GetId(), u.GetUsername(), u.GetDisplayName(), u.GetInstanceOwner())
		}
		fmt.Fprintf(w, "\nTotal users: %d\n", resp.GetTotal())
		return nil
	})
}

func newUsersGetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get <user_id>",
		Short:   "Get user details.",
		Long:    "Retrieve detailed information about a specific user.",
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
			return runUsersGet(client, args[0], renderer)
		},
	}
	return cmd
}

func runUsersGet(client *Client, id string, renderer *Renderer) error {
	user, _, err := client.API.UsersAPI.User(client.Ctx, id).Execute()
	if err != nil {
		return formatSDKError(err)
	}
	return renderUserDetail(user, renderer)
}

func renderUserDetail(user *kestra.IAMUserControllerApiUser, renderer *Renderer) error {
	return renderer.Render(user, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "User Details")
		fmt.Fprintln(w)
		fmt.Fprintf(w, "ID:\t%s\n", user.GetId())
		fmt.Fprintf(w, "Username:\t%s\n", user.GetUsername())
		fmt.Fprintf(w, "Display Name:\t%s\n", user.GetDisplayName())
		fmt.Fprintf(w, "Email:\t%s\n", user.GetEmail())
		fmt.Fprintf(w, "First Name:\t%s\n", user.GetFirstName())
		fmt.Fprintf(w, "Last Name:\t%s\n", user.GetLastName())
		fmt.Fprintf(w, "Super Admin:\t%t\n", user.GetInstanceOwner())
		fmt.Fprintf(w, "Restricted:\t%t\n", user.GetRestricted())

		if groups := user.GetGroups(); len(groups) > 0 {
			ids := make([]string, len(groups))
			for i, g := range groups {
				ids[i] = g.GetId()
			}
			fmt.Fprintf(w, "Groups:\t%s\n", strings.Join(ids, ", "))
		}
		if tenants := user.GetTenants(); len(tenants) > 0 {
			names := make([]string, len(tenants))
			for i, tn := range tenants {
				names[i] = tn.GetId()
			}
			fmt.Fprintf(w, "Tenants:\t%s\n", strings.Join(names, ", "))
		}
		return nil
	})
}

func newUsersCreateCommand() *cobra.Command {
	var m userMutation

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a user.",
		Long:  "Create a new user. The --email flag is required.",
		Example: `  # Create a user
	  kestractl users create --email alice@example.com --first-name Alice

	  # Create a super-admin with a password and group
	  kestractl users create --email bob@example.com --user-password 's3cret' --superadmin --group my-group-id`,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := NewClient()
			if err != nil {
				return err
			}
			m.markChanged(cmd)
			return runUsersCreate(client, m, renderer)
		},
	}

	addMutationFlags(cmd, &m)
	_ = cmd.MarkFlagRequired("email")

	return cmd
}

func runUsersCreate(client *Client, m userMutation, renderer *Renderer) error {
	if m.superAdminSet && m.superAdmin {
		if err := requireKestra2(client, "--superadmin"); err != nil {
			return err
		}
	}
	user, _, err := client.API.UsersAPI.CreateUser(client.Ctx).
		IAMUserControllerApiCreateOrUpdateUserRequest(m.toRequest()).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}
	return renderUserDetail(user, renderer)
}

func newUsersUpdateCommand() *cobra.Command {
	var m userMutation

	cmd := &cobra.Command{
		Use:   "update <user_id>",
		Short: "Update a user.",
		Long: `Update an existing user.

Only the flags you pass are changed; all other attributes are preserved. The
server endpoint is a full replace, so the current user is fetched first and the
changed fields are merged on top before saving.`,
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
			m.markChanged(cmd)
			return runUsersUpdate(client, args[0], m, renderer)
		},
	}

	addMutationFlags(cmd, &m)

	return cmd
}

func runUsersUpdate(client *Client, id string, m userMutation, renderer *Renderer) error {
	if m.superAdminSet && m.superAdmin {
		if err := requireKestra2(client, "--superadmin"); err != nil {
			return err
		}
	}
	// The update endpoint is a full-replace PUT, so fetch the current user and
	// overlay only the changed flags — otherwise unspecified attributes (e.g.
	// superAdmin, groups) would be reset to their defaults.
	current, _, err := client.API.UsersAPI.User(client.Ctx, id).Execute()
	if err != nil {
		return formatSDKError(err)
	}

	req := userRequestFromExisting(current)
	m.applyTo(&req)

	user, _, err := client.API.UsersAPI.UpdateUser(client.Ctx, id).
		IAMUserControllerApiCreateOrUpdateUserRequest(req).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}
	return renderUserDetail(user, renderer)
}

func newUsersDeleteCommand() *cobra.Command {
	var skipConfirm bool

	cmd := &cobra.Command{
		Use:     "delete <user_id>",
		Short:   "Delete a user.",
		Long:    "Delete a user. Prompts for confirmation unless --yes is provided.",
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
			return runUsersDelete(client, args[0], skipConfirm, cmd.InOrStdin(), renderer)
		},
	}

	cmd.Flags().BoolVarP(&skipConfirm, "yes", "y", false, "Skip the confirmation prompt")

	return cmd
}

func runUsersDelete(client *Client, id string, skipConfirm bool, in io.Reader, renderer *Renderer) error {
	if !skipConfirm {
		// Prompt on stderr so it never pollutes stdout (e.g. --output json).
		confirmed, err := confirm(in, os.Stderr,
			fmt.Sprintf("Are you sure you want to delete user '%s'? [y/N]: ", id))
		if err != nil {
			return err
		}
		if !confirmed {
			return renderStatus(renderer, "Cancelled.", map[string]any{"id": id, "status": "cancelled"})
		}
	}

	if _, err := client.API.UsersAPI.DeleteUser(client.Ctx, id).Execute(); err != nil {
		return formatSDKError(err)
	}

	return renderStatus(renderer, fmt.Sprintf("User '%s' deleted.", id),
		map[string]any{"id": id, "status": "deleted"})
}

func newUsersSetGroupsCommand() *cobra.Command {
	var groups []string

	cmd := &cobra.Command{
		Use:   "set-groups <user_id>",
		Short: "Set the groups a user belongs to.",
		Long:  "Replace the set of groups a user belongs to within the active tenant.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := NewClient()
			if err != nil {
				return err
			}
			return runUsersSetGroups(client, args[0], groups, renderer)
		},
	}

	cmd.Flags().StringArrayVar(&groups, "group", nil, "Group ID (repeatable). Pass none to clear all groups.")

	return cmd
}

func runUsersSetGroups(client *Client, id string, groups []string, renderer *Renderer) error {
	// A nil slice is dropped by the SDK serializer (IsNil), so "clear all groups"
	// (no --group flags) would be a no-op. Send a non-nil empty slice instead,
	// which serializes to "groupIds": [] and actually clears membership.
	if groups == nil {
		groups = []string{}
	}

	body := kestra.IAMUserGroupControllerApiUpdateUserGroupsRequest{GroupIds: groups}
	if _, err := client.API.UsersAPI.UpdateUserGroups(client.Ctx, id, client.Tenant).
		IAMUserGroupControllerApiUpdateUserGroupsRequest(body).
		Execute(); err != nil {
		return formatSDKError(err)
	}

	return renderStatus(renderer, fmt.Sprintf("Groups updated for user '%s'.", id),
		map[string]any{"id": id, "status": "groups-updated", "groups": groups})
}

func newUsersSetPasswordCommand() *cobra.Command {
	var password string

	cmd := &cobra.Command{
		Use:   "set-password <user_id>",
		Short: "Set a user's password.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := NewClient()
			if err != nil {
				return err
			}
			return runUsersSetPassword(client, args[0], password, renderer)
		},
	}

	cmd.Flags().StringVar(&password, "user-password", "", "New password for the user")
	_ = cmd.MarkFlagRequired("user-password")

	return cmd
}

func runUsersSetPassword(client *Client, id, password string, renderer *Renderer) error {
	body := kestra.IAMUserControllerApiPatchUserPasswordRequest{Password: password}
	if _, _, err := client.API.UsersAPI.PatchUserPassword(client.Ctx, id).
		IAMUserControllerApiPatchUserPasswordRequest(body).
		Execute(); err != nil {
		return formatSDKError(err)
	}

	return renderStatus(renderer, fmt.Sprintf("Password updated for user '%s'.", id),
		map[string]any{"id": id, "status": "password-updated"})
}

func newUsersTokensCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tokens",
		Short: "Manage API tokens for a user (list, create, delete)",
	}

	cmd.AddCommand(newUsersTokensListCommand())
	cmd.AddCommand(newUsersTokensCreateCommand())
	cmd.AddCommand(newUsersTokensDeleteCommand())

	return cmd
}

func newUsersTokensListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list <user_id>",
		Short:   "List API tokens for a user.",
		Aliases: []string{"ls"},
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
			return runUsersTokensList(client, args[0], renderer)
		},
	}
	return cmd
}

func runUsersTokensList(client *Client, userID string, renderer *Renderer) error {
	resp, _, err := client.API.UsersAPI.ListApiTokensForUser(client.Ctx, userID).Execute()
	if err != nil {
		return formatSDKError(err)
	}

	tokens := resp.GetResults()
	if tokens == nil {
		// Render an empty JSON array ([]) rather than null on no results.
		tokens = []kestra.ApiToken{}
	}

	return renderer.Render(tokens, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ID\tNAME\tPREFIX\tEXPIRED\tLAST USED")
		for _, tok := range tokens {
			lastUsed := ""
			if tok.LastUsed != nil {
				lastUsed = tok.LastUsed.Format(time.RFC3339)
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%t\t%s\n",
				tok.GetId(), tok.GetName(), tok.GetPrefix(), tok.GetExpired(), lastUsed)
		}
		fmt.Fprintf(w, "\nTotal tokens: %d\n", len(tokens))
		return nil
	})
}

func newUsersTokensCreateCommand() *cobra.Command {
	var (
		name        string
		description string
		maxAge      string
		extended    bool
	)

	cmd := &cobra.Command{
		Use:   "create <user_id>",
		Short: "Create an API token for a user.",
		Long: `Create an API token for a user.

The full token value is shown only once, at creation time. Store it securely.`,
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
			return runUsersTokensCreate(client, args[0], name, description, maxAge, extended, renderer)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Token name (lowercase alphanumeric and dashes)")
	cmd.Flags().StringVar(&description, "description", "", "Token description")
	cmd.Flags().StringVar(&maxAge, "max-age", "", "Token max age (ISO-8601 duration, e.g. P30D)")
	cmd.Flags().BoolVar(&extended, "extended", false, "Create an extended token")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func runUsersTokensCreate(client *Client, userID, name, description, maxAge string, extended bool, renderer *Renderer) error {
	body := kestra.CreateApiTokenRequest{Name: name, Extended: &extended}
	if description != "" {
		body.Description = &description
	}
	if maxAge != "" {
		body.MaxAge = &maxAge
	}

	resp, _, err := client.API.UsersAPI.CreateApiTokensForUser(client.Ctx, userID).
		CreateApiTokenRequest(body).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	return renderer.Render(resp, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "API token created.")
		fmt.Fprintln(w)
		fmt.Fprintf(w, "ID:\t%s\n", resp.GetId())
		fmt.Fprintf(w, "Name:\t%s\n", resp.GetName())
		fmt.Fprintf(w, "Token:\t%s\n", resp.GetFullToken())
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Store this token now — it will not be shown again.")
		return nil
	})
}

func newUsersTokensDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete <user_id> <token_id>",
		Short:   "Delete an API token for a user.",
		Aliases: []string{"rm"},
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := NewClient()
			if err != nil {
				return err
			}
			return runUsersTokensDelete(client, args[0], args[1], renderer)
		},
	}
	return cmd
}

func runUsersTokensDelete(client *Client, userID, tokenID string, renderer *Renderer) error {
	if _, err := client.API.UsersAPI.DeleteApiTokenForUser(client.Ctx, userID, tokenID).Execute(); err != nil {
		return formatSDKError(err)
	}

	return renderStatus(renderer, fmt.Sprintf("Token '%s' deleted for user '%s'.", tokenID, userID),
		map[string]any{"id": tokenID, "userId": userID, "status": "deleted"})
}

// renderStatus reports the outcome of a mutating command: a JSON object when
// --output json is set, or a plain-text message in table mode. This keeps
// commands that have no resource body to return (delete, set-groups, …)
// honoring the JSON output contract.
func renderStatus(renderer *Renderer, message string, fields map[string]any) error {
	return renderer.Render(fields, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, message)
		return nil
	})
}

// confirm prints prompt to w and reads a yes/no answer from in.
// It returns true only for "y" or "yes" (case-insensitive).
func confirm(in io.Reader, w io.Writer, prompt string) (bool, error) {
	fmt.Fprint(w, prompt)
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}

func newUsersAutocompleteCommand() *cobra.Command {
	var (
		query        string
		existingOnly bool
	)

	cmd := &cobra.Command{
		Use:   "autocomplete",
		Short: "List users for autocomplete.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutputFormat(); err != nil {
				return err
			}
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runUsersAutocomplete(client, query, existingOnly, renderer)
		},
	}

	cmd.Flags().StringVarP(&query, "query", "q", "", "Autocomplete search string")
	cmd.Flags().BoolVar(&existingOnly, "existing-only", false, "Return only existing users")

	return cmd
}

func runUsersAutocomplete(client *Client, query string, existingOnly bool, renderer *Renderer) error {
	ac := kestra.NewIAMTenantAccessControllerUserApiAutocomplete()
	if query != "" {
		ac.SetQ(query)
	}
	if existingOnly {
		ac.SetExistingOnly(existingOnly)
	}

	users, _, err := client.API.UsersAPI.
		AutocompleteUsers(client.Ctx, client.Tenant).
		IAMTenantAccessControllerUserApiAutocomplete(*ac).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	result := make([]map[string]any, len(users))
	for i, u := range users {
		result[i] = map[string]any{"id": u.GetId(), "username": u.GetUsername(), "displayName": u.GetDisplayName()}
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ID\tUSERNAME\tDISPLAY NAME")
		for _, u := range users {
			fmt.Fprintf(w, "%s\t%s\t%s\n", u.GetId(), u.GetUsername(), u.GetDisplayName())
		}
		fmt.Fprintf(w, "\nShowing %d user(s)\n", len(users))
		return nil
	})
}

func newUsersImpersonateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "impersonate <user_id>",
		Short: "Generate an impersonation token for a user.",
		Long: `Generate a short-lived token that lets you act as another user.

The response contains a JWT token. Use it in place of your regular credentials
to perform actions on behalf of the specified user. Requires superadmin access.`,
		Example: `  kestractl users impersonate <user_id>
  kestractl users impersonate <user_id> --output json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runUsersImpersonate(client, args[0], renderer)
		},
	}
	return cmd
}

func runUsersImpersonate(client *Client, id string, renderer *Renderer) error {
	resp, err := client.Kestra.Users().Impersonate(client.Ctx, id)
	if err != nil {
		return formatSDKError(err)
	}

	return renderer.Render(resp, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "Impersonation token generated.")
		fmt.Fprintln(w)
		for k, v := range resp {
			fmt.Fprintf(w, "%s:\t%v\n", k, v)
		}
		return nil
	})
}

func newUsersRevokeRefreshTokenCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "revoke-refresh-token <user_id>",
		Short: "Revoke all refresh tokens for a user.",
		Long: `Invalidate all active refresh tokens for the given user, forcing them to log in again.

This is a superadmin-only operation.`,
		Example: `  kestractl users revoke-refresh-token <user_id>`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runUsersRevokeRefreshToken(client, args[0], renderer)
		},
	}
	return cmd
}

func runUsersRevokeRefreshToken(client *Client, id string, renderer *Renderer) error {
	if err := client.Kestra.Users().DeleteRefreshToken(client.Ctx, id); err != nil {
		return formatSDKError(err)
	}

	return renderStatus(renderer,
		fmt.Sprintf("Refresh tokens revoked for user '%s'.", id),
		map[string]any{"id": id, "status": "revoked"})
}

func newUsersPatchSuperAdminCommand() *cobra.Command {
	var superAdmin bool

	cmd := &cobra.Command{
		Use:   "set-super-admin <id>",
		Short: "Grant or revoke superadmin status for a user. Superadmin only.",
		Args:  cobra.ExactArgs(1),
		Example: `  kestractl users set-super-admin <id> --super-admin=true
  kestractl users set-super-admin <id> --super-admin=false`,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runUsersPatchSuperAdmin(client, args[0], superAdmin, renderer)
		},
	}

	cmd.Flags().BoolVar(&superAdmin, "super-admin", false, "Set superadmin status (true or false)")
	return cmd
}

func runUsersPatchSuperAdmin(client *Client, id string, superAdmin bool, renderer *Renderer) error {
	if err := requireKestra2(client, "users set-super-admin"); err != nil {
		return err
	}
	body := map[string]any{"instanceOwner": superAdmin}
	if err := client.Kestra.Users().PatchUserInstanceOwner(client.Ctx, id, body); err != nil {
		return formatSDKError(err)
	}

	status := "revoked"
	if superAdmin {
		status = "granted"
	}
	return renderStatus(renderer,
		fmt.Sprintf("Superadmin status %s for user '%s'.", status, id),
		map[string]any{"id": id, "superAdmin": superAdmin, "status": status})
}

func newUsersSetRestrictedCommand() *cobra.Command {
	var restricted bool

	cmd := &cobra.Command{
		Use:   "set-restricted <id>",
		Short: "Mark a user as restricted, or lift the restriction. Superadmin only.",
		Long: `Set whether a user is restricted via the dedicated restricted endpoint.

This is a targeted PATCH, unlike 'users update --restricted' which performs a
full-replace of the user. Use this when you only want to toggle the flag.`,
		Args: cobra.ExactArgs(1),
		Example: `  kestractl users set-restricted <id> --restricted=true
  kestractl users set-restricted <id> --restricted=false`,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runUsersSetRestricted(client, args[0], restricted, renderer)
		},
	}

	cmd.Flags().BoolVar(&restricted, "restricted", false, "Set restricted status (true or false)")
	return cmd
}

func runUsersSetRestricted(client *Client, id string, restricted bool, renderer *Renderer) error {
	body := kestra.IAMUserControllerApiPatchRestrictedRequest{Restricted: restricted}
	if err := client.Kestra.Users().PatchUserDemo(client.Ctx, id, body); err != nil {
		return formatSDKError(err)
	}

	status := "lifted"
	if restricted {
		status = "applied"
	}
	return renderStatus(renderer,
		fmt.Sprintf("Restriction %s for user '%s'.", status, id),
		map[string]any{"id": id, "restricted": restricted, "status": status})
}

func newUsersDeleteAuthMethodCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete-auth-method <id> <auth-method>",
		Short: "Delete an authentication method from a user. Superadmin only.",
		Args:  cobra.ExactArgs(2),
		Example: `  kestractl users delete-auth-method <user-id> BASIC_AUTH
  kestractl users delete-auth-method <user-id> GOOGLE_OAUTH`,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runUsersDeleteAuthMethod(client, args[0], args[1], renderer)
		},
	}
	return cmd
}

func runUsersDeleteAuthMethod(client *Client, id, auth string, renderer *Renderer) error {
	user, err := client.Kestra.Users().DeleteUserAuthMethod(client.Ctx, id, auth)
	if err != nil {
		return formatSDKError(err)
	}

	if user == nil {
		return renderStatus(renderer,
			fmt.Sprintf("Auth method '%s' removed from user '%s'.", auth, id),
			map[string]any{"id": id, "auth": auth, "status": "removed"})
	}

	return renderer.Render(user, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Auth method '%s' removed.\n\nUser ID:\t%s\n", auth, user.GetId())
		return nil
	})
}

func newUsersChangeMyPasswordCommand() *cobra.Command {
	var oldPassword, newPassword string

	cmd := &cobra.Command{
		Use:     "change-my-password",
		Short:   "Change the current user's own password.",
		Long:    "Update the password for the currently authenticated user using the old and new passwords.",
		Example: `  kestractl users change-my-password --old-password <current> --new-password <new>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runUsersChangeMyPassword(client, oldPassword, newPassword, renderer)
		},
	}

	cmd.Flags().StringVar(&oldPassword, "old-password", "", "Current password")
	cmd.Flags().StringVar(&newPassword, "new-password", "", "New password")
	_ = cmd.MarkFlagRequired("old-password")
	_ = cmd.MarkFlagRequired("new-password")
	return cmd
}

func runUsersChangeMyPassword(client *Client, oldPassword, newPassword string, renderer *Renderer) error {
	req := kestra.MeControllerApiUpdatePasswordRequest{
		OldPassword: &oldPassword,
		NewPassword: &newPassword,
	}
	if _, err := client.Kestra.Users().UpdateCurrentUserPassword(client.Ctx, req); err != nil {
		return formatSDKError(err)
	}
	return renderStatus(renderer, "Password updated successfully.",
		map[string]any{"status": "updated"})
}
