package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	kestra "github.com/kestra-io/client-sdk/go-sdk/kestra_api_client"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newRolesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "roles",
		Short: "Manage IAM roles (list, get, create, update, delete)",
		Long: `Manage IAM roles in your Kestra instance.

Roles are tenant-scoped resources. Managing roles requires Kestra Enterprise Edition.`,
	}

	cmd.AddCommand(newRolesListCommand())
	cmd.AddCommand(newRolesGetCommand())
	cmd.AddCommand(newRolesCreateCommand())
	cmd.AddCommand(newRolesUpdateCommand())
	cmd.AddCommand(newRolesDeleteCommand())
	cmd.AddCommand(newRolesAutocompleteCommand())
	cmd.AddCommand(newRolesListFromIdsCommand())

	return cmd
}

// roleMutation holds the fields used to create or update a role, plus a record
// of which flags the operator explicitly set.
//
// As with users, the update endpoint is a full-replace PUT: any attribute
// absent from the request body is reset server-side. So update fetches the
// current role and overlays only the changed flags via applyTo, while create
// builds a fresh request via toRequest.
type roleMutation struct {
	name        string
	description string
	isDefault   bool
	permPairs   []string // raw --permission "FLOW:READ,CREATE" values (repeatable)
	permFile    string   // --permissions-file path

	nameSet        bool
	descriptionSet bool
	isDefaultSet   bool
	permsSet       bool
}

// addRoleMutationFlags wires the create/update flags shared by both commands.
func addRoleMutationFlags(cmd *cobra.Command, m *roleMutation) {
	cmd.Flags().StringVar(&m.name, "name", "", "Role name")
	cmd.Flags().StringVar(&m.description, "description", "", "Role description")
	cmd.Flags().BoolVar(&m.isDefault, "default", false, "Mark the role as a default role")
	cmd.Flags().StringArrayVar(&m.permPairs, "permission", nil,
		"Permission as TYPE:LEVEL[,LEVEL] (e.g. FLOW:READ,CREATE). Repeatable.")
	cmd.Flags().StringVar(&m.permFile, "permissions-file", "",
		"Path to a YAML or JSON file mapping resource types to permission levels")
}

// markChanged records which flags the operator actually passed on the command.
func (m *roleMutation) markChanged(cmd *cobra.Command) {
	f := cmd.Flags()
	m.nameSet = f.Changed("name")
	m.descriptionSet = f.Changed("description")
	m.isDefaultSet = f.Changed("default")
	m.permsSet = f.Changed("permission") || f.Changed("permissions-file")
}

// parsePermissions builds the permissions payload from either the repeatable
// --permission flag or --permissions-file. Passing both is an error.
func (m roleMutation) parsePermissions() (*kestra.IAMRoleControllerApiRoleCreateOrUpdateRequestPermissions, error) {
	if len(m.permPairs) > 0 && m.permFile != "" {
		return nil, fmt.Errorf("use either --permission or --permissions-file, not both")
	}

	perms := map[string][]string{}

	if m.permFile != "" {
		data, err := os.ReadFile(m.permFile)
		if err != nil {
			return nil, fmt.Errorf("reading permissions file: %w", err)
		}
		// yaml.v3 parses JSON too, since JSON is a subset of YAML.
		if err := yaml.Unmarshal(data, &perms); err != nil {
			return nil, fmt.Errorf("parsing permissions file: %w", err)
		}
	}

	for _, pair := range m.permPairs {
		key, levels, err := parsePermPair(pair)
		if err != nil {
			return nil, err
		}
		perms[key] = append(perms[key], levels...)
	}

	return permissionsToStruct(perms)
}

// parsePermPair parses "FLOW:READ,CREATE" into the resource type and its levels.
// Both the type and levels are upper-cased to match the API enum.
func parsePermPair(s string) (string, []string, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return "", nil, fmt.Errorf("invalid permission %q: expected TYPE:LEVEL[,LEVEL]", s)
	}
	key := strings.ToUpper(strings.TrimSpace(parts[0]))
	if key == "" {
		return "", nil, fmt.Errorf("invalid permission %q: empty resource type", s)
	}

	var levels []string
	for _, lvl := range strings.Split(parts[1], ",") {
		lvl = strings.ToUpper(strings.TrimSpace(lvl))
		if lvl != "" {
			levels = append(levels, lvl)
		}
	}
	if len(levels) == 0 {
		return "", nil, fmt.Errorf("invalid permission %q: no levels specified", s)
	}
	return key, levels, nil
}

// permissionsToStruct converts a resource-type → levels map into the SDK
// permissions struct via a JSON round-trip. This leans on the struct's JSON
// tags (which match the uppercase resource keys) so we don't maintain a switch
// over the ~24 resource types and stay forward-compatible with new ones.
func permissionsToStruct(m map[string][]string) (*kestra.IAMRoleControllerApiRoleCreateOrUpdateRequestPermissions, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	var perms kestra.IAMRoleControllerApiRoleCreateOrUpdateRequestPermissions
	if err := json.Unmarshal(data, &perms); err != nil {
		return nil, fmt.Errorf("invalid permissions: %w", err)
	}
	return &perms, nil
}

// permissionsToMap is the reverse of permissionsToStruct, used for display.
func permissionsToMap(p *kestra.IAMRoleControllerApiRoleCreateOrUpdateRequestPermissions) (map[string][]string, error) {
	if p == nil {
		return map[string][]string{}, nil
	}
	data, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	m := map[string][]string{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// toRequest builds a create request from scratch.
func (m roleMutation) toRequest() (kestra.IAMRoleControllerApiRoleCreateOrUpdateRequest, error) {
	req := kestra.IAMRoleControllerApiRoleCreateOrUpdateRequest{Name: m.name}
	if err := m.applyTo(&req); err != nil {
		return req, err
	}
	return req, nil
}

// applyTo overlays the explicitly-set fields onto req, leaving everything else
// (e.g. an existing role's attributes seeded from a GET) untouched.
func (m roleMutation) applyTo(req *kestra.IAMRoleControllerApiRoleCreateOrUpdateRequest) error {
	if m.nameSet {
		req.Name = m.name
	}
	if m.descriptionSet {
		req.Description = &m.description
	}
	if m.isDefaultSet {
		req.IsDefault = &m.isDefault
	}
	if m.permsSet {
		perms, err := m.parsePermissions()
		if err != nil {
			return err
		}
		req.Permissions = *perms
	}
	return nil
}

// roleRequestFromExisting seeds a full create/update request from a fetched
// role, so a full-replace PUT preserves attributes the operator didn't touch.
func roleRequestFromExisting(r *kestra.IAMRoleControllerApiRoleDetail) kestra.IAMRoleControllerApiRoleCreateOrUpdateRequest {
	req := kestra.IAMRoleControllerApiRoleCreateOrUpdateRequest{Name: r.GetName()}
	if r.Description != nil {
		req.Description = r.Description
	}
	if r.IsDefault != nil {
		req.IsDefault = r.IsDefault
	}
	if r.Permissions != nil {
		req.Permissions = *r.Permissions
	}
	return req
}

func newRolesListCommand() *cobra.Command {
	var (
		query string
		page  int32
		size  int32
		sort  []string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List roles.",
		Long:  "List all roles in the active tenant, optionally filtered with --query.",
		Example: `  # List all roles
  kestractl roles list

  # Filter roles
  kestractl roles list --query editor

  # List roles as JSON
  kestractl roles list --output json`,
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
			return runRolesList(client, query, page, size, sort, renderer)
		},
	}

	cmd.Flags().StringVarP(&query, "query", "q", "", "Filter roles by search query")
	cmd.Flags().Int32Var(&page, "page", 1, "Page number")
	cmd.Flags().Int32Var(&size, "size", 100, "Page size")
	cmd.Flags().StringArrayVar(&sort, "sort", nil, "Sort expression (e.g. 'name:asc', repeatable)")

	return cmd
}

func runRolesList(client *Client, query string, page, size int32, sort []string, renderer *Renderer) error {
	req := client.API.RolesAPI.SearchRoles(client.Ctx, client.Tenant).Page(page).Size(size)
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
		results = []kestra.ApiRoleSummary{}
	}

	return renderer.Render(results, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ID\tNAME\tDEFAULT\tMANAGED")
		for _, r := range results {
			fmt.Fprintf(w, "%s\t%s\t%t\t%t\n",
				r.GetId(), r.GetName(), r.GetIsDefault(), r.GetIsManaged())
		}
		fmt.Fprintf(w, "\nTotal roles: %d\n", resp.GetTotal())
		return nil
	})
}

func newRolesGetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get <role_id>",
		Short:   "Get role details.",
		Long:    "Retrieve detailed information about a specific role, including its permissions.",
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
			return runRolesGet(client, args[0], renderer)
		},
	}
	return cmd
}

func runRolesGet(client *Client, id string, renderer *Renderer) error {
	role, _, err := client.API.RolesAPI.Role(client.Ctx, id, client.Tenant).Execute()
	if err != nil {
		return formatSDKError(err)
	}
	return renderRoleDetail(role, renderer)
}

func renderRoleDetail(role *kestra.IAMRoleControllerApiRoleDetail, renderer *Renderer) error {
	return renderer.Render(role, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "Role Details")
		fmt.Fprintln(w)
		fmt.Fprintf(w, "ID:\t%s\n", role.GetId())
		fmt.Fprintf(w, "Name:\t%s\n", role.GetName())
		fmt.Fprintf(w, "Description:\t%s\n", role.GetDescription())
		fmt.Fprintf(w, "Default:\t%t\n", role.GetIsDefault())
		fmt.Fprintf(w, "Managed:\t%t\n", role.GetIsManaged())

		perms, err := permissionsToMap(role.Permissions)
		if err != nil {
			return err
		}
		if len(perms) > 0 {
			fmt.Fprintln(w, "Permissions:")
			keys := make([]string, 0, len(perms))
			for k := range perms {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Fprintf(w, "  %s:\t%s\n", k, strings.Join(perms[k], ", "))
			}
		}
		return nil
	})
}

func newRolesCreateCommand() *cobra.Command {
	var m roleMutation

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a role.",
		Long: `Create a new role. The --name flag is required, along with at least one
permission (via --permission or --permissions-file).`,
		Example: `  # Create a role with inline permissions
  kestractl roles create --name editor \
    --permission FLOW:READ,CREATE,UPDATE \
    --permission EXECUTION:READ

  # Create a role from a permissions file
  kestractl roles create --name editor --permissions-file perms.yaml`,
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
			return runRolesCreate(client, m, renderer)
		},
	}

	addRoleMutationFlags(cmd, &m)
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func runRolesCreate(client *Client, m roleMutation, renderer *Renderer) error {
	if !m.permsSet {
		return fmt.Errorf("at least one permission is required: use --permission or --permissions-file")
	}

	req, err := m.toRequest()
	if err != nil {
		return err
	}

	role, _, err := client.API.RolesAPI.CreateRole(client.Ctx, client.Tenant).
		IAMRoleControllerApiRoleCreateOrUpdateRequest(req).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}
	return renderRoleDetail(role, renderer)
}

func newRolesUpdateCommand() *cobra.Command {
	var m roleMutation

	cmd := &cobra.Command{
		Use:   "update <role_id>",
		Short: "Update a role.",
		Long: `Update an existing role.

Only the flags you pass are changed; all other attributes are preserved. The
server endpoint is a full replace, so the current role is fetched first and the
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
			return runRolesUpdate(client, args[0], m, renderer)
		},
	}

	addRoleMutationFlags(cmd, &m)

	return cmd
}

func runRolesUpdate(client *Client, id string, m roleMutation, renderer *Renderer) error {
	// The update endpoint is a full-replace PUT, so fetch the current role and
	// overlay only the changed flags — otherwise unspecified attributes (e.g.
	// permissions, isDefault) would be reset to their defaults.
	current, _, err := client.API.RolesAPI.Role(client.Ctx, id, client.Tenant).Execute()
	if err != nil {
		return formatSDKError(err)
	}

	req := roleRequestFromExisting(current)
	if err := m.applyTo(&req); err != nil {
		return err
	}

	role, _, err := client.API.RolesAPI.UpdateRole(client.Ctx, id, client.Tenant).
		IAMRoleControllerApiRoleCreateOrUpdateRequest(req).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}
	return renderRoleDetail(role, renderer)
}

func newRolesDeleteCommand() *cobra.Command {
	var skipConfirm bool

	cmd := &cobra.Command{
		Use:     "delete <role_id>",
		Short:   "Delete a role.",
		Long:    "Delete a role. Prompts for confirmation unless --yes is provided.",
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
			return runRolesDelete(client, args[0], skipConfirm, cmd.InOrStdin(), renderer)
		},
	}

	cmd.Flags().BoolVarP(&skipConfirm, "yes", "y", false, "Skip the confirmation prompt")

	return cmd
}

func runRolesDelete(client *Client, id string, skipConfirm bool, in io.Reader, renderer *Renderer) error {
	if !skipConfirm {
		// Prompt on stderr so it never pollutes stdout (e.g. --output json).
		confirmed, err := confirm(in, os.Stderr,
			fmt.Sprintf("Are you sure you want to delete role '%s'? [y/N]: ", id))
		if err != nil {
			return err
		}
		if !confirmed {
			return renderStatus(renderer, "Cancelled.", map[string]any{"id": id, "status": "cancelled"})
		}
	}

	if _, err := client.API.RolesAPI.DeleteRole(client.Ctx, id, client.Tenant).Execute(); err != nil {
		return formatSDKError(err)
	}

	return renderStatus(renderer, fmt.Sprintf("Role '%s' deleted.", id),
		map[string]any{"id": id, "status": "deleted"})
}

func newRolesAutocompleteCommand() *cobra.Command {
	var query string

	cmd := &cobra.Command{
		Use:   "autocomplete",
		Short: "List roles for autocomplete.",
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
			return runRolesAutocomplete(client, query, renderer)
		},
	}

	cmd.Flags().StringVarP(&query, "query", "q", "", "Autocomplete search string")

	return cmd
}

func runRolesAutocomplete(client *Client, query string, renderer *Renderer) error {
	ac := kestra.NewApiAutocomplete()
	if query != "" {
		ac.SetQ(query)
	}

	roles, _, err := client.API.RolesAPI.
		AutocompleteRoles(client.Ctx, client.Tenant).
		ApiAutocomplete(*ac).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	result := make([]map[string]any, len(roles))
	for i, r := range roles {
		result[i] = map[string]any{"id": r.GetId(), "name": r.GetName()}
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ID\tNAME")
		for _, r := range roles {
			fmt.Fprintf(w, "%s\t%s\n", r.GetId(), r.GetName())
		}
		fmt.Fprintf(w, "\nShowing %d role(s)\n", len(roles))
		return nil
	})
}

func newRolesListFromIdsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list-from-ids <id>...",
		Short: "List roles by ID.",
		Long:  "Fetch multiple roles by their IDs in a single request.",
		Args:  cobra.MinimumNArgs(1),
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
			return runRolesListFromIds(client, args, renderer)
		},
	}
	return cmd
}

func runRolesListFromIds(client *Client, ids []string, renderer *Renderer) error {
	body := kestra.NewApiIds()
	body.SetIds(ids)

	roles, _, err := client.API.RolesAPI.
		ListRolesFromGivenIds(client.Ctx, client.Tenant).
		ApiIds(*body).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	result := make([]map[string]any, len(roles))
	for i, r := range roles {
		result[i] = map[string]any{"id": r.GetId(), "name": r.GetName()}
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ID\tNAME\tDEFAULT")
		for _, r := range roles {
			fmt.Fprintf(w, "%s\t%s\t%v\n", r.GetId(), r.GetName(), r.GetIsDefault())
		}
		fmt.Fprintf(w, "\nShowing %d role(s)\n", len(roles))
		return nil
	})
}
