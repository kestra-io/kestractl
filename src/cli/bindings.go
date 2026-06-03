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

func newBindingsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bindings",
		Short: "Manage IAM role bindings (list, get, create, delete)",
		Long: `Manage role bindings in your Kestra instance.

A binding assigns a role to a user or group, optionally scoped to a namespace.
Bindings are tenant-scoped resources. Managing bindings requires Kestra
Enterprise Edition.`,
	}

	cmd.AddCommand(newBindingsListCommand())
	cmd.AddCommand(newBindingsGetCommand())
	cmd.AddCommand(newBindingsCreateCommand())
	cmd.AddCommand(newBindingsDeleteCommand())

	return cmd
}

func newBindingsListCommand() *cobra.Command {
	var (
		bindingType string
		externalID  string
		namespace   string
		page        int
		size        int
		sort        []string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List bindings.",
		Long:  "List all role bindings in the active tenant, optionally filtered by --type, --external-id or --namespace.",
		Example: `  # List all bindings
  kestractl bindings list

  # List bindings of a group
  kestractl bindings list --type GROUP --external-id group_id

  # List bindings scoped to a namespace
  kestractl bindings list --namespace company.team

  # List bindings as JSON
  kestractl bindings list --output json`,
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
			return runBindingsList(client, bindingType, externalID, namespace, page, size, sort, renderer)
		},
	}

	cmd.Flags().StringVar(&bindingType, "type", "", "Filter bindings by subject type (USER or GROUP)")
	cmd.Flags().StringVar(&externalID, "external-id", "", "Filter bindings by the bound user/group ID")
	cmd.Flags().StringVar(&namespace, "namespace", "", "Filter bindings by namespace")
	cmd.Flags().IntVar(&page, "page", 1, "Page number")
	cmd.Flags().IntVar(&size, "size", 100, "Page size")
	cmd.Flags().StringArrayVar(&sort, "sort", nil, "Sort expression (repeatable)")

	return cmd
}

func runBindingsList(client *Client, bindingType, externalID, namespace string, page, size int, sort []string, renderer *Renderer) error {
	typeFilter, err := parseBindingType(bindingType)
	if err != nil {
		return err
	}

	var externalIDFilter, namespaceFilter *string
	if externalID != "" {
		externalIDFilter = &externalID
	}
	if namespace != "" {
		namespaceFilter = &namespace
	}

	resp, err := client.Kestra.Bindings().SearchBindings(
		client.Ctx, client.Tenant, &page, &size, sort, typeFilter, externalIDFilter, namespaceFilter, nil)
	if err != nil {
		return formatSDKError(err)
	}

	results := resp.Results
	if results == nil {
		// Render an empty JSON array ([]) rather than null on no results.
		results = []kestra.IAMBindingControllerApiBindingDetail{}
	}

	return renderer.Render(results, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ID\tTYPE\tSUBJECT\tROLE\tNAMESPACE")
		for _, b := range results {
			role := ""
			if b.Role != nil {
				role = b.Role.GetName()
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				b.GetId(), b.GetType(), bindingSubject(b), role, b.GetNamespace())
		}
		fmt.Fprintf(w, "\nTotal bindings: %d\n", resp.Total)
		return nil
	})
}

// bindingSubject renders the bound principal: the user's username or the
// group's name, depending on the binding type.
func bindingSubject(b kestra.IAMBindingControllerApiBindingDetail) string {
	if user, ok := b.GetUserOk(); ok && user != nil {
		return user.GetUsername()
	}
	if group, ok := b.GetGroupOk(); ok && group != nil {
		return group.GetName()
	}
	return ""
}

func newBindingsGetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get <binding_id>",
		Short:   "Get binding details.",
		Long:    "Retrieve detailed information about a specific role binding.",
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
			return runBindingsGet(client, args[0], renderer)
		},
	}
	return cmd
}

func runBindingsGet(client *Client, id string, renderer *Renderer) error {
	binding, err := client.Kestra.Bindings().Binding(client.Ctx, id, client.Tenant)
	if err != nil {
		return formatSDKError(err)
	}
	return renderBindingDetail(binding, renderer)
}

func renderBindingDetail(b *kestra.IAMBindingControllerApiBindingDetail, renderer *Renderer) error {
	return renderer.Render(b, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "Binding Details")
		fmt.Fprintln(w)
		fmt.Fprintf(w, "ID:\t%s\n", b.GetId())
		fmt.Fprintf(w, "Type:\t%s\n", b.GetType())
		fmt.Fprintf(w, "Subject:\t%s\n", bindingSubject(*b))
		if b.Role != nil {
			fmt.Fprintf(w, "Role:\t%s (%s)\n", b.Role.GetName(), b.Role.GetId())
		}
		if ns := b.GetNamespace(); ns != "" {
			fmt.Fprintf(w, "Namespace:\t%s\n", ns)
		}
		if user, ok := b.GetUserOk(); ok && user != nil {
			fmt.Fprintf(w, "User ID:\t%s\n", user.GetId())
			if dn := user.GetDisplayName(); dn != "" {
				fmt.Fprintf(w, "Display name:\t%s\n", dn)
			}
		}
		if group, ok := b.GetGroupOk(); ok && group != nil {
			fmt.Fprintf(w, "Group ID:\t%s\n", group.GetId())
		}
		return nil
	})
}

func newBindingsCreateCommand() *cobra.Command {
	var (
		bindingType string
		externalID  string
		roleID      string
		namespace   string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a binding.",
		Long: `Create a role binding, assigning a role to a user or group.

Pass --namespace to scope the binding to a namespace; otherwise it applies
tenant-wide.`,
		Example: `  # Assign a role to a user tenant-wide
  kestractl bindings create --type USER --external-id user_id --role role_id

  # Assign a role to a group within a namespace
  kestractl bindings create --type GROUP --external-id group_id --role role_id \
    --namespace company.team`,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := NewClient()
			if err != nil {
				return err
			}
			return runBindingsCreate(client, bindingType, externalID, roleID, namespace, renderer)
		},
	}

	cmd.Flags().StringVar(&bindingType, "type", "", "Subject type (USER or GROUP)")
	cmd.Flags().StringVar(&externalID, "external-id", "", "ID of the user or group to bind")
	cmd.Flags().StringVar(&roleID, "role", "", "Role ID to assign")
	cmd.Flags().StringVar(&namespace, "namespace", "", "Namespace to scope the binding to (optional)")
	_ = cmd.MarkFlagRequired("type")
	_ = cmd.MarkFlagRequired("external-id")
	_ = cmd.MarkFlagRequired("role")

	return cmd
}

func runBindingsCreate(client *Client, bindingType, externalID, roleID, namespace string, renderer *Renderer) error {
	typeValue, err := parseBindingType(bindingType)
	if err != nil {
		return err
	}

	req := kestra.IAMBindingControllerApiCreateBindingRequest{
		Type:       *typeValue,
		ExternalId: externalID,
		RoleId:     roleID,
	}
	if namespace != "" {
		req.NamespaceId = *kestra.NewNullableString(&namespace)
	}

	binding, err := client.Kestra.Bindings().CreateBinding(client.Ctx, client.Tenant, req)
	if err != nil {
		return formatSDKError(err)
	}
	return renderBindingDetail(binding, renderer)
}

func newBindingsDeleteCommand() *cobra.Command {
	var skipConfirm bool

	cmd := &cobra.Command{
		Use:     "delete <binding_id>",
		Short:   "Delete a binding.",
		Long:    "Delete a role binding. Prompts for confirmation unless --yes is provided.",
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
			return runBindingsDelete(client, args[0], skipConfirm, cmd.InOrStdin(), renderer)
		},
	}

	cmd.Flags().BoolVarP(&skipConfirm, "yes", "y", false, "Skip the confirmation prompt")

	return cmd
}

func runBindingsDelete(client *Client, id string, skipConfirm bool, in io.Reader, renderer *Renderer) error {
	if !skipConfirm {
		// Prompt on stderr so it never pollutes stdout (e.g. --output json).
		confirmed, err := confirm(in, os.Stderr,
			fmt.Sprintf("Are you sure you want to delete binding '%s'? [y/N]: ", id))
		if err != nil {
			return err
		}
		if !confirmed {
			return renderStatus(renderer, "Cancelled.", map[string]any{"id": id, "status": "cancelled"})
		}
	}

	if err := client.Kestra.Bindings().DeleteBinding(client.Ctx, id, client.Tenant); err != nil {
		return formatSDKError(err)
	}

	return renderStatus(renderer, fmt.Sprintf("Binding '%s' deleted.", id),
		map[string]any{"id": id, "status": "deleted"})
}

// parseBindingType validates an optional --type value against the SDK enum.
// An empty input returns (nil, nil) so it can be passed straight to search.
func parseBindingType(s string) (*kestra.BindingType, error) {
	if s == "" {
		return nil, nil
	}
	parsed, err := kestra.NewBindingTypeFromValue(strings.ToUpper(strings.TrimSpace(s)))
	if err != nil {
		values := make([]string, 0, len(kestra.AllowedBindingTypeEnumValues))
		for _, v := range kestra.AllowedBindingTypeEnumValues {
			values = append(values, string(v))
		}
		return nil, fmt.Errorf("invalid type %q: expected one of %s", s, strings.Join(values, ", "))
	}
	return parsed, nil
}
