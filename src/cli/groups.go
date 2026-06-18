package cli

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	kestra "github.com/kestra-io/client-sdk/go-sdk/kestra_api_client"
	"github.com/spf13/cobra"
)

func newGroupsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "groups",
		Short: "Manage IAM groups (list, get, create, update, delete, members)",
		Long: `Manage IAM groups in your Kestra instance.

Groups are tenant-scoped resources. Managing groups requires Kestra Enterprise Edition.`,
	}

	cmd.AddCommand(newGroupsListCommand())
	cmd.AddCommand(newGroupsGetCommand())
	cmd.AddCommand(newGroupsCreateCommand())
	cmd.AddCommand(newGroupsUpdateCommand())
	cmd.AddCommand(newGroupsDeleteCommand())
	cmd.AddCommand(newGroupsMembersCommand())
	cmd.AddCommand(newGroupsAutocompleteCommand())

	return cmd
}

func newGroupsListCommand() *cobra.Command {
	var (
		query string
		page  int32
		size  int32
		sort  []string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List groups.",
		Long:  "List all groups in the active tenant, optionally filtered with --query.",
		Example: `  # List all groups
	  kestractl groups list

	  # Filter groups
	  kestractl groups list --query admins

	  # List groups as JSON
	  kestractl groups list --output json`,
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
			return runGroupsList(client, query, page, size, sort, renderer)
		},
	}

	cmd.Flags().StringVarP(&query, "query", "q", "", "Filter groups by search query")
	cmd.Flags().Int32Var(&page, "page", 1, "Page number")
	cmd.Flags().Int32Var(&size, "size", 100, "Page size")
	cmd.Flags().StringArrayVar(&sort, "sort", nil, "Sort expression (e.g. 'name:asc', repeatable)")

	return cmd
}

func runGroupsList(client *Client, query string, page, size int32, sort []string, renderer *Renderer) error {
	req := client.API.GroupsAPI.SearchGroups(client.Ctx, client.Tenant).Page(page).Size(size)
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
		results = []kestra.ApiGroupSummary{}
	}

	return renderer.Render(results, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ID\tNAME")
		for _, g := range results {
			fmt.Fprintf(w, "%s\t%s\n", g.GetId(), g.GetName())
		}
		fmt.Fprintf(w, "\nTotal groups: %d\n", resp.GetTotal())
		return nil
	})
}

func newGroupsGetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get <group_id>",
		Short:   "Get group details.",
		Long:    "Retrieve detailed information about a specific group.",
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
			return runGroupsGet(client, args[0], renderer)
		},
	}
	return cmd
}

func runGroupsGet(client *Client, id string, renderer *Renderer) error {
	group, _, err := client.API.GroupsAPI.Group(client.Ctx, id, client.Tenant).Execute()
	if err != nil {
		return formatSDKError(err)
	}
	return renderGroupDetail(group, renderer)
}

func renderGroupDetail(group *kestra.IAMGroupControllerApiGroupDetail, renderer *Renderer) error {
	return renderer.Render(group, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "Group Details")
		fmt.Fprintln(w)
		fmt.Fprintf(w, "ID:\t%s\n", group.GetId())
		fmt.Fprintf(w, "Name:\t%s\n", group.GetName())
		fmt.Fprintf(w, "Description:\t%s\n", group.GetDescription())
		return nil
	})
}

func newGroupsCreateCommand() *cobra.Command {
	var (
		name        string
		description string
		members     []string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a group.",
		Long:  "Create a new group. The --name flag is required.",
		Example: `  # Create a group
	  kestractl groups create --name admins

	  # Create a group with a description and initial members
	  kestractl groups create --name admins --description 'Platform admins' --member user-id-1 --member user-id-2`,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := NewClient()
			if err != nil {
				return err
			}
			return runGroupsCreate(client, name, description, members, renderer)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Group name")
	cmd.Flags().StringVar(&description, "description", "", "Group description")
	cmd.Flags().StringArrayVar(&members, "member", nil, "User ID to add as an initial member (repeatable)")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func runGroupsCreate(client *Client, name, description string, members []string, renderer *Renderer) error {
	body := kestra.IAMGroupControllerApiCreateGroupRequest{Name: name}
	if description != "" {
		body.Description = &description
	}
	if len(members) > 0 {
		body.MembersId = members
	}

	group, _, err := client.API.GroupsAPI.CreateGroup(client.Ctx, client.Tenant).
		IAMGroupControllerApiCreateGroupRequest(body).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}
	return renderGroupDetail(group, renderer)
}

func newGroupsUpdateCommand() *cobra.Command {
	var (
		name        string
		description string
	)

	cmd := &cobra.Command{
		Use:   "update <group_id>",
		Short: "Update a group.",
		Long: `Update an existing group.

Only the flags you pass are changed; all other attributes are preserved. The
server endpoint is a full replace, so the current group is fetched first and the
changed fields are merged on top before saving.

Group membership is not changed here — use 'groups members add/remove'.`,
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
			nameSet := cmd.Flags().Changed("name")
			descSet := cmd.Flags().Changed("description")
			return runGroupsUpdate(client, args[0], name, nameSet, description, descSet, renderer)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Group name")
	cmd.Flags().StringVar(&description, "description", "", "Group description")

	return cmd
}

func runGroupsUpdate(client *Client, id, name string, nameSet bool, description string, descSet bool, renderer *Renderer) error {
	// The update endpoint is a full-replace PUT, so fetch the current group and
	// overlay only the changed flags — otherwise unspecified attributes (e.g.
	// name when only --description is passed) would be reset to their defaults.
	current, _, err := client.API.GroupsAPI.Group(client.Ctx, id, client.Tenant).Execute()
	if err != nil {
		return formatSDKError(err)
	}

	body := kestra.IAMGroupControllerApiUpdateGroupRequest{Name: current.GetName()}
	if desc := current.GetDescription(); desc != "" {
		body.Description = &desc
	}
	if nameSet {
		body.Name = name
	}
	if descSet {
		body.Description = &description
	}

	group, _, err := client.API.GroupsAPI.UpdateGroup(client.Ctx, id, client.Tenant).
		IAMGroupControllerApiUpdateGroupRequest(body).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}
	return renderGroupDetail(group, renderer)
}

func newGroupsDeleteCommand() *cobra.Command {
	var skipConfirm bool

	cmd := &cobra.Command{
		Use:     "delete <group_id>",
		Short:   "Delete a group.",
		Long:    "Delete a group. Prompts for confirmation unless --yes is provided.",
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
			return runGroupsDelete(client, args[0], skipConfirm, cmd.InOrStdin(), renderer)
		},
	}

	cmd.Flags().BoolVarP(&skipConfirm, "yes", "y", false, "Skip the confirmation prompt")

	return cmd
}

func runGroupsDelete(client *Client, id string, skipConfirm bool, in io.Reader, renderer *Renderer) error {
	if !skipConfirm {
		// Prompt on stderr so it never pollutes stdout (e.g. --output json).
		confirmed, err := confirm(in, os.Stderr,
			fmt.Sprintf("Are you sure you want to delete group '%s'? [y/N]: ", id))
		if err != nil {
			return err
		}
		if !confirmed {
			return renderStatus(renderer, "Cancelled.", map[string]any{"id": id, "status": "cancelled"})
		}
	}

	if _, err := client.API.GroupsAPI.DeleteGroup(client.Ctx, id, client.Tenant).Execute(); err != nil {
		return formatSDKError(err)
	}

	return renderStatus(renderer, fmt.Sprintf("Group '%s' deleted.", id),
		map[string]any{"id": id, "status": "deleted"})
}

func newGroupsMembersCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "members",
		Short: "Manage members of a group (list, add, remove)",
	}

	cmd.AddCommand(newGroupsMembersListCommand())
	cmd.AddCommand(newGroupsMembersAddCommand())
	cmd.AddCommand(newGroupsMembersRemoveCommand())

	return cmd
}

func newGroupsMembersListCommand() *cobra.Command {
	var (
		query string
		page  int32
		size  int32
		sort  []string
	)

	cmd := &cobra.Command{
		Use:     "list <group_id>",
		Short:   "List members of a group.",
		Long:    "List the users that belong to a group, optionally filtered with --query.",
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
			return runGroupsMembersList(client, args[0], query, page, size, sort, renderer)
		},
	}

	cmd.Flags().StringVarP(&query, "query", "q", "", "Filter members by search query")
	cmd.Flags().Int32Var(&page, "page", 1, "Page number")
	cmd.Flags().Int32Var(&size, "size", 100, "Page size")
	cmd.Flags().StringArrayVar(&sort, "sort", nil, "Sort expression (e.g. 'username:asc', repeatable)")

	return cmd
}

func runGroupsMembersList(client *Client, id, query string, page, size int32, sort []string, renderer *Renderer) error {
	req := client.API.GroupsAPI.SearchGroupMembers(client.Ctx, id, client.Tenant).Page(page).Size(size)
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

	members := resp.GetResults()
	if members == nil {
		// Render an empty JSON array ([]) rather than null on no results.
		members = []kestra.IAMGroupControllerApiGroupMember{}
	}

	return renderer.Render(members, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ID\tUSERNAME\tDISPLAY NAME")
		for _, m := range members {
			fmt.Fprintf(w, "%s\t%s\t%s\n", m.GetId(), m.GetUsername(), m.GetDisplayName())
		}
		fmt.Fprintf(w, "\nTotal members: %d\n", resp.GetTotal())
		return nil
	})
}

func newGroupsMembersAddCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <group_id> <user_id>",
		Short: "Add a user to a group.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := NewClient()
			if err != nil {
				return err
			}
			return runGroupsMembersAdd(client, args[0], args[1], renderer)
		},
	}
	return cmd
}

func runGroupsMembersAdd(client *Client, groupID, userID string, renderer *Renderer) error {
	if _, _, err := client.API.GroupsAPI.AddUserToGroup(client.Ctx, groupID, userID, client.Tenant).Execute(); err != nil {
		return formatSDKError(err)
	}

	return renderStatus(renderer, fmt.Sprintf("User '%s' added to group '%s'.", userID, groupID),
		map[string]any{"groupId": groupID, "userId": userID, "status": "added"})
}

func newGroupsMembersRemoveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove <group_id> <user_id>",
		Short:   "Remove a user from a group.",
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
			return runGroupsMembersRemove(client, args[0], args[1], renderer)
		},
	}
	return cmd
}

func newGroupsAutocompleteCommand() *cobra.Command {
	var query string

	cmd := &cobra.Command{
		Use:   "autocomplete",
		Short: "List groups for autocomplete.",
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
			return runGroupsAutocomplete(client, query, renderer)
		},
	}

	cmd.Flags().StringVarP(&query, "query", "q", "", "Autocomplete search string")

	return cmd
}

func runGroupsAutocomplete(client *Client, query string, renderer *Renderer) error {
	ac := kestra.NewApiAutocomplete()
	if query != "" {
		ac.SetQ(query)
	}

	groups, _, err := client.API.GroupsAPI.
		AutocompleteGroups(client.Ctx, client.Tenant).
		ApiAutocomplete(*ac).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	result := make([]map[string]any, len(groups))
	for i, g := range groups {
		result[i] = map[string]any{"id": g.GetId(), "name": g.GetName()}
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ID\tNAME")
		for _, g := range groups {
			fmt.Fprintf(w, "%s\t%s\n", g.GetId(), g.GetName())
		}
		fmt.Fprintf(w, "\nShowing %d group(s)\n", len(groups))
		return nil
	})
}

func runGroupsMembersRemove(client *Client, groupID, userID string, renderer *Renderer) error {
	if _, _, err := client.API.GroupsAPI.DeleteUserFromGroup(client.Ctx, groupID, userID, client.Tenant).Execute(); err != nil {
		return formatSDKError(err)
	}

	return renderStatus(renderer, fmt.Sprintf("User '%s' removed from group '%s'.", userID, groupID),
		map[string]any{"groupId": groupID, "userId": userID, "status": "removed"})
}
