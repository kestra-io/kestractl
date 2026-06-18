package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	kestra "github.com/kestra-io/client-sdk/go-sdk/kestra_api_client"
	"github.com/spf13/cobra"
)

func newServiceAccountsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "service-accounts",
		Aliases: []string{"service-account", "sa"},
		Short:   "Manage IAM service accounts (list, get, create, update, delete)",
		Long: `Manage IAM service accounts in your Kestra instance.

Service accounts are instance-level resources. Managing them requires Kestra
Enterprise Edition.`,
	}

	cmd.AddCommand(newServiceAccountsListCommand())
	cmd.AddCommand(newServiceAccountsGetCommand())
	cmd.AddCommand(newServiceAccountsCreateCommand())
	cmd.AddCommand(newServiceAccountsUpdateCommand())
	cmd.AddCommand(newServiceAccountsDeleteCommand())
	cmd.AddCommand(newServiceAccountsTokensCommand())
	cmd.AddCommand(newServiceAccountsSetSuperAdminCommand())

	return cmd
}

// serviceAccountMutation holds the fields used to create a service account,
// plus a record of which flags the operator explicitly set.
//
// Unlike users/roles, update is a true PATCH (PatchServiceAccountDetails) that
// only touches name and description — so the *Set bookkeeping is only needed
// for create, where superAdmin and tenants are also settable.
type serviceAccountMutation struct {
	name        string
	description string
	superAdmin  bool
	tenants     []string

	nameSet        bool
	descriptionSet bool
	superAdminSet  bool
	tenantsSet     bool
}

// markChanged records which flags the operator actually passed on the command.
func (m *serviceAccountMutation) markChanged(cmd *cobra.Command) {
	f := cmd.Flags()
	m.nameSet = f.Changed("name")
	m.descriptionSet = f.Changed("description")
	m.superAdminSet = f.Changed("superadmin")
	m.tenantsSet = f.Changed("tenant-grant")
}

func newServiceAccountsListCommand() *cobra.Command {
	var (
		page int32
		size int32
		sort []string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List service accounts.",
		Long:  "List all service accounts in your Kestra instance.",
		Example: `  # List all service accounts
  kestractl service-accounts list

  # List service accounts as JSON
  kestractl service-accounts list --output json`,
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
			return runServiceAccountsList(client, page, size, sort, renderer)
		},
	}

	cmd.Flags().Int32Var(&page, "page", 1, "Page number")
	cmd.Flags().Int32Var(&size, "size", 100, "Page size")
	cmd.Flags().StringArrayVar(&sort, "sort", nil, "Sort expression (e.g. 'name:asc', repeatable)")

	return cmd
}

func runServiceAccountsList(client *Client, page, size int32, sort []string, renderer *Renderer) error {
	req := client.API.ServiceAccountAPI.ListServiceAccounts(client.Ctx).Page(page).Size(size)
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
		results = []kestra.IAMServiceAccountControllerApiServiceAccountDetail{}
	}

	return renderer.Render(results, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ID\tNAME\tSUPERADMIN\tTENANTS")
		for _, sa := range results {
			fmt.Fprintf(w, "%s\t%s\t%t\t%s\n",
				sa.GetId(), sa.GetName(), sa.GetSuperAdmin(), strings.Join(tenantIDs(sa.GetTenants()), ", "))
		}
		fmt.Fprintf(w, "\nTotal service accounts: %d\n", resp.GetTotal())
		return nil
	})
}

func newServiceAccountsGetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get <service_account_id>",
		Short:   "Get service account details.",
		Long:    "Retrieve detailed information about a specific service account.",
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
			return runServiceAccountsGet(client, args[0], renderer)
		},
	}
	return cmd
}

func runServiceAccountsGet(client *Client, id string, renderer *Renderer) error {
	sa, _, err := client.API.ServiceAccountAPI.ServiceAccount(client.Ctx, id).Execute()
	if err != nil {
		return formatSDKError(err)
	}
	return renderServiceAccountDetail(sa, renderer)
}

func renderServiceAccountDetail(sa *kestra.IAMServiceAccountControllerApiServiceAccountDetail, renderer *Renderer) error {
	return renderer.Render(sa, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "Service Account Details")
		fmt.Fprintln(w)
		fmt.Fprintf(w, "ID:\t%s\n", sa.GetId())
		fmt.Fprintf(w, "Name:\t%s\n", sa.GetName())
		fmt.Fprintf(w, "Description:\t%s\n", sa.GetDescription())
		fmt.Fprintf(w, "Super Admin:\t%t\n", sa.GetSuperAdmin())
		if tenants := sa.GetTenants(); len(tenants) > 0 {
			fmt.Fprintf(w, "Tenants:\t%s\n", strings.Join(tenantIDs(tenants), ", "))
		}
		return nil
	})
}

// tenantIDs extracts the tenant identifiers from a service account's tenant
// summaries, for compact display.
func tenantIDs(tenants []kestra.ApiTenantSummary) []string {
	ids := make([]string, 0, len(tenants))
	for _, t := range tenants {
		ids = append(ids, t.GetId())
	}
	return ids
}

func newServiceAccountsCreateCommand() *cobra.Command {
	var m serviceAccountMutation

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a service account.",
		Long: `Create a new service account. The --name flag is required.

Names must be lowercase alphanumeric, optionally separated by dashes.`,
		Example: `  # Create a service account
  kestractl service-accounts create --name ci-bot --description "CI pipeline"

  # Create a super-admin service account with tenant access
  kestractl service-accounts create --name ops-bot --superadmin --tenant-grant main`,
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
			return runServiceAccountsCreate(client, m, renderer)
		},
	}

	cmd.Flags().StringVar(&m.name, "name", "", "Service account name (lowercase alphanumeric and dashes)")
	cmd.Flags().StringVar(&m.description, "description", "", "Service account description")
	cmd.Flags().BoolVar(&m.superAdmin, "superadmin", false, "Grant super-admin privileges")
	cmd.Flags().StringArrayVar(&m.tenants, "tenant-grant", nil, "Tenant ID to grant access to (repeatable)")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func runServiceAccountsCreate(client *Client, m serviceAccountMutation, renderer *Renderer) error {
	body := kestra.IAMServiceAccountControllerApiCreateServiceAccountRequest{Name: m.name}
	if m.descriptionSet {
		body.Description = &m.description
	}
	if m.superAdminSet {
		body.SuperAdmin = &m.superAdmin
	}
	if m.tenantsSet {
		body.Tenants = m.tenants
	}

	sa, _, err := client.API.ServiceAccountAPI.CreateServiceAccount(client.Ctx).
		IAMServiceAccountControllerApiCreateServiceAccountRequest(body).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}
	return renderServiceAccountDetail(sa, renderer)
}

func newServiceAccountsUpdateCommand() *cobra.Command {
	var m serviceAccountMutation

	cmd := &cobra.Command{
		Use:   "update <service_account_id>",
		Short: "Update a service account's name or description.",
		Long: `Update an existing service account.

This is a partial update: only --name and --description are changed. Tenant
access, super-admin status and group membership are left untouched.`,
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
			return runServiceAccountsUpdate(client, args[0], m, renderer)
		},
	}

	cmd.Flags().StringVar(&m.name, "name", "", "New name (lowercase alphanumeric and dashes)")
	cmd.Flags().StringVar(&m.description, "description", "", "New description")

	return cmd
}

func runServiceAccountsUpdate(client *Client, id string, m serviceAccountMutation, renderer *Renderer) error {
	if !m.nameSet && !m.descriptionSet {
		return fmt.Errorf("nothing to update: pass --name and/or --description")
	}

	// PatchServiceAccountRequest.Name is required, so fetch the current account
	// to fill in the unchanged name when only --description is provided.
	current, _, err := client.API.ServiceAccountAPI.ServiceAccount(client.Ctx, id).Execute()
	if err != nil {
		return formatSDKError(err)
	}

	body := kestra.IAMServiceAccountControllerApiPatchServiceAccountRequest{Name: current.GetName()}
	if m.nameSet {
		body.Name = m.name
	}
	if m.descriptionSet {
		body.Description = &m.description
	} else if current.Description != nil {
		body.Description = current.Description
	}

	sa, _, err := client.API.ServiceAccountAPI.PatchServiceAccountDetails(client.Ctx, id).
		IAMServiceAccountControllerApiPatchServiceAccountRequest(body).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}
	return renderServiceAccountDetail(sa, renderer)
}

func newServiceAccountsDeleteCommand() *cobra.Command {
	var skipConfirm bool

	cmd := &cobra.Command{
		Use:     "delete <service_account_id>",
		Short:   "Delete a service account.",
		Long:    "Delete a service account. Prompts for confirmation unless --yes is provided.",
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
			return runServiceAccountsDelete(client, args[0], skipConfirm, cmd.InOrStdin(), renderer)
		},
	}

	cmd.Flags().BoolVarP(&skipConfirm, "yes", "y", false, "Skip the confirmation prompt")

	return cmd
}

func runServiceAccountsDelete(client *Client, id string, skipConfirm bool, in io.Reader, renderer *Renderer) error {
	if !skipConfirm {
		// Prompt on stderr so it never pollutes stdout (e.g. --output json).
		confirmed, err := confirm(in, os.Stderr,
			fmt.Sprintf("Are you sure you want to delete service account '%s'? [y/N]: ", id))
		if err != nil {
			return err
		}
		if !confirmed {
			return renderStatus(renderer, "Cancelled.", map[string]any{"id": id, "status": "cancelled"})
		}
	}

	if _, err := client.API.ServiceAccountAPI.DeleteServiceAccount(client.Ctx, id).Execute(); err != nil {
		return formatSDKError(err)
	}

	return renderStatus(renderer, fmt.Sprintf("Service account '%s' deleted.", id),
		map[string]any{"id": id, "status": "deleted"})
}

func newServiceAccountsTokensCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tokens",
		Short: "Manage API tokens for a service account (list, create, delete)",
	}

	cmd.AddCommand(newServiceAccountsTokensListCommand())
	cmd.AddCommand(newServiceAccountsTokensCreateCommand())
	cmd.AddCommand(newServiceAccountsTokensDeleteCommand())

	return cmd
}

// decodeTokenInto re-decodes a loosely-typed token response (the SDK's
// ServiceAccount token endpoints return map[string]interface{}) into the
// strongly-typed token struct via a JSON round-trip.
func decodeTokenInto(raw map[string]interface{}, out any) error {
	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decoding token response: %w", err)
	}
	return nil
}

func newServiceAccountsTokensListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list <service_account_id>",
		Short:   "List API tokens for a service account.",
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
			return runServiceAccountsTokensList(client, args[0], renderer)
		},
	}
	return cmd
}

func runServiceAccountsTokensList(client *Client, id string, renderer *Renderer) error {
	raw, _, err := client.API.ServiceAccountAPI.ListApiTokensForServiceAccount(client.Ctx, id).Execute()
	if err != nil {
		return formatSDKError(err)
	}

	var list kestra.ApiTokenList
	if err := decodeTokenInto(raw, &list); err != nil {
		return err
	}

	tokens := list.GetResults()
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

func newServiceAccountsTokensCreateCommand() *cobra.Command {
	var (
		name        string
		description string
		maxAge      string
		extended    bool
	)

	cmd := &cobra.Command{
		Use:   "create <service_account_id>",
		Short: "Create an API token for a service account.",
		Long: `Create an API token for a service account.

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
			return runServiceAccountsTokensCreate(client, args[0], name, description, maxAge, extended, renderer)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Token name (lowercase alphanumeric and dashes)")
	cmd.Flags().StringVar(&description, "description", "", "Token description")
	cmd.Flags().StringVar(&maxAge, "max-age", "", "Token max age (ISO-8601 duration, e.g. P30D)")
	cmd.Flags().BoolVar(&extended, "extended", false, "Create an extended token")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func runServiceAccountsTokensCreate(client *Client, id, name, description, maxAge string, extended bool, renderer *Renderer) error {
	body := kestra.CreateApiTokenRequest{Name: name, Extended: &extended}
	if description != "" {
		body.Description = &description
	}
	if maxAge != "" {
		body.MaxAge = &maxAge
	}

	raw, _, err := client.API.ServiceAccountAPI.CreateApiTokensForServiceAccount(client.Ctx, id).
		CreateApiTokenRequest(body).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	var resp kestra.CreateApiTokenResponse
	if err := decodeTokenInto(raw, &resp); err != nil {
		return err
	}

	return renderer.Render(&resp, func(w *tabwriter.Writer) error {
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

func newServiceAccountsTokensDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete <service_account_id> <token_id>",
		Short:   "Delete an API token for a service account.",
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
			return runServiceAccountsTokensDelete(client, args[0], args[1], renderer)
		},
	}
	return cmd
}

func runServiceAccountsTokensDelete(client *Client, id, tokenID string, renderer *Renderer) error {
	if _, _, err := client.API.ServiceAccountAPI.DeleteApiTokenForServiceAccount(client.Ctx, id, tokenID).Execute(); err != nil {
		return formatSDKError(err)
	}

	return renderStatus(renderer, fmt.Sprintf("Token '%s' deleted for service account '%s'.", tokenID, id),
		map[string]any{"id": tokenID, "serviceAccountId": id, "status": "deleted"})
}

func newServiceAccountsSetSuperAdminCommand() *cobra.Command {
	var superAdmin bool

	cmd := &cobra.Command{
		Use:   "set-super-admin <id>",
		Short: "Grant or revoke superadmin status for a service account. Superadmin only.",
		Args:  cobra.ExactArgs(1),
		Example: `  kestractl service-accounts set-super-admin <id> --super-admin=true
  kestractl service-accounts set-super-admin <id> --super-admin=false`,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runServiceAccountsSetSuperAdmin(client, args[0], superAdmin, renderer)
		},
	}

	cmd.Flags().BoolVar(&superAdmin, "super-admin", false, "Set superadmin status (true or false)")
	return cmd
}

func runServiceAccountsSetSuperAdmin(client *Client, id string, superAdmin bool, renderer *Renderer) error {
	body := map[string]any{"superAdmin": superAdmin}
	if err := client.Kestra.ServiceAccount().PatchServiceAccountSuperAdmin(client.Ctx, id, body); err != nil {
		return formatSDKError(err)
	}

	status := "revoked"
	if superAdmin {
		status = "granted"
	}
	return renderStatus(renderer,
		fmt.Sprintf("Superadmin status %s for service account '%s'.", status, id),
		map[string]any{"id": id, "superAdmin": superAdmin, "status": status})
}
