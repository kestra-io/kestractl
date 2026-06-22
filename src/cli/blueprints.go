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

func newBlueprintsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "blueprints",
		Short: "Manage Kestra blueprints (community search/get, flow list/get/create/update/delete)",
		Long: `Manage Kestra blueprints.

Community blueprints are read-only templates from Kestra's public catalog.
Flow blueprints are custom templates stored in your tenant.`,
	}

	cmd.AddCommand(newBlueprintsCommunityCommand())
	cmd.AddCommand(newBlueprintsFlowCommand())
	cmd.AddCommand(newBlueprintsCustomCommand())

	return cmd
}

// community sub-group

func newBlueprintsCommunityCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "community",
		Short: "Browse community (public) blueprints.",
	}
	cmd.AddCommand(newBlueprintsCommunitySearchCommand())
	cmd.AddCommand(newBlueprintsCommunityGetCommand())
	cmd.AddCommand(newBlueprintsCommunitySourceCommand())
	cmd.AddCommand(newBlueprintsCommunityGraphCommand())
	return cmd
}

func newBlueprintsCommunityGraphCommand() *cobra.Command {
	var kind string

	cmd := &cobra.Command{
		Use:   "graph <id>",
		Short: "Get the topology graph of a community blueprint.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runBlueprintsCommunityGraph(client, args[0], kind, renderer)
		},
	}

	cmd.Flags().StringVar(&kind, "kind", "FLOW", "Blueprint kind (e.g. FLOW)")
	return cmd
}

func runBlueprintsCommunityGraph(client *Client, id, kind string, renderer *Renderer) error {
	graph, err := client.Kestra.Blueprints().BlueprintGraph(client.Ctx, id, kind, client.Tenant)
	if err != nil {
		return formatSDKError(err)
	}

	return renderer.Render(graph, func(w *tabwriter.Writer) error {
		nodes, _ := graph["nodes"].([]interface{})
		edges, _ := graph["edges"].([]interface{})
		fmt.Fprintln(w, "KEY\tVALUE")
		fmt.Fprintf(w, "Nodes\t%d\n", len(nodes))
		fmt.Fprintf(w, "Edges\t%d\n", len(edges))
		fmt.Fprintln(w, "\nUse --output json to see the full graph.")
		return nil
	})
}

func newBlueprintsCommunitySearchCommand() *cobra.Command {
	var (
		kind  string
		query string
		tags  []string
		page  int32
		size  int32
		sort  []string
	)

	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search community blueprints.",
		Example: `  kestractl blueprints community search --kind FLOW
  kestractl blueprints community search --kind FLOW --query "http" --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runBlueprintsCommunitySearch(client, kind, query, tags, page, size, sort, renderer)
		},
	}

	cmd.Flags().StringVar(&kind, "kind", "FLOW", "Blueprint kind (e.g. FLOW)")
	cmd.Flags().StringVarP(&query, "query", "q", "", "Filter by query string")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "Filter by tag (repeatable)")
	cmd.Flags().Int32Var(&page, "page", 1, "Page number")
	cmd.Flags().Int32Var(&size, "size", 25, "Page size")
	cmd.Flags().StringArrayVar(&sort, "sort", nil, "Sort expression (repeatable)")

	return cmd
}

func runBlueprintsCommunitySearch(client *Client, kind, query string, tags []string, page, size int32, sort []string, renderer *Renderer) error {
	pageInt, sizeInt := int(page), int(size)

	var qPtr *string
	if query != "" {
		qPtr = &query
	}

	resp, err := client.Kestra.Blueprints().SearchBlueprints(client.Ctx, kind, client.Tenant, qPtr, sort, tags, &pageInt, &sizeInt)
	if err != nil {
		return formatSDKError(err)
	}

	results := resp.GetResults()

	return renderer.Render(results, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ID\tTITLE\tTAGS")
		for _, b := range results {
			fmt.Fprintf(w, "%s\t%s\t%s\n",
				b.GetId(), b.GetTitle(), strings.Join(b.GetTags(), ", "))
		}
		fmt.Fprintf(w, "\nTotal: %d\n", resp.GetTotal())
		return nil
	})
}

func newBlueprintsCommunityGetCommand() *cobra.Command {
	var kind string

	cmd := &cobra.Command{
		Use:     "get <id>",
		Short:   "Get a community blueprint.",
		Aliases: []string{"show", "describe"},
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
			return runBlueprintsCommunityGet(client, args[0], kind, renderer)
		},
	}

	cmd.Flags().StringVar(&kind, "kind", "FLOW", "Blueprint kind (e.g. FLOW)")
	return cmd
}

func runBlueprintsCommunityGet(client *Client, id, kind string, renderer *Renderer) error {
	b, err := client.Kestra.Blueprints().Blueprint(client.Ctx, id, kind, client.Tenant)
	if err != nil {
		return formatSDKError(err)
	}

	return renderer.Render(b, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "Blueprint Details")
		fmt.Fprintln(w)
		fmt.Fprintf(w, "ID:\t%s\n", b.GetId())
		fmt.Fprintf(w, "Title:\t%s\n", b.GetTitle())
		if desc := b.GetDescription(); desc != "" {
			fmt.Fprintf(w, "Description:\t%s\n", desc)
		}
		if tags := b.GetTags(); len(tags) > 0 {
			fmt.Fprintf(w, "Tags:\t%s\n", strings.Join(tags, ", "))
		}
		return nil
	})
}

func newBlueprintsCommunitySourceCommand() *cobra.Command {
	var kind string

	cmd := &cobra.Command{
		Use:   "source <id>",
		Short: "Print the YAML source of a community blueprint.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			src, err := client.Kestra.Blueprints().BlueprintSource(client.Ctx, args[0], kind, client.Tenant)
			if err != nil {
				return formatSDKError(err)
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), src)
			return err
		},
	}

	cmd.Flags().StringVar(&kind, "kind", "FLOW", "Blueprint kind (e.g. FLOW)")
	return cmd
}

// flow sub-group

func newBlueprintsFlowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "flow",
		Short: "Manage custom flow blueprints.",
	}
	cmd.AddCommand(newBlueprintsFlowListCommand())
	cmd.AddCommand(newBlueprintsFlowGetCommand())
	cmd.AddCommand(newBlueprintsFlowCreateCommand())
	cmd.AddCommand(newBlueprintsFlowUpdateCommand())
	cmd.AddCommand(newBlueprintsFlowDeleteCommand())
	cmd.AddCommand(newBlueprintsFlowUseTemplateCommand())
	return cmd
}

func newBlueprintsFlowListCommand() *cobra.Command {
	var (
		query string
		tags  []string
		page  int32
		size  int32
		sort  []string
	)

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List custom flow blueprints.",
		Aliases: []string{"ls"},
		Example: `  kestractl blueprints flow list
  kestractl blueprints flow list --query "http" --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runBlueprintsFlowList(client, query, tags, page, size, sort, renderer)
		},
	}

	cmd.Flags().StringVarP(&query, "query", "q", "", "Filter by query string")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "Filter by tag (repeatable)")
	cmd.Flags().Int32Var(&page, "page", 1, "Page number")
	cmd.Flags().Int32Var(&size, "size", 100, "Page size")
	cmd.Flags().StringArrayVar(&sort, "sort", nil, "Sort expression (repeatable)")

	return cmd
}

func runBlueprintsFlowList(client *Client, query string, tags []string, page, size int32, sort []string, renderer *Renderer) error {
	pageInt, sizeInt := int(page), int(size)

	var qPtr *string
	if query != "" {
		qPtr = &query
	}

	resp, err := client.Kestra.Blueprints().SearchInternalBlueprints(client.Ctx, client.Tenant, qPtr, sort, tags, &pageInt, &sizeInt, nil)
	if err != nil {
		return formatSDKError(err)
	}

	results := resp.GetResults()

	return renderer.Render(results, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ID\tTITLE\tTAGS")
		for _, b := range results {
			fmt.Fprintf(w, "%s\t%s\t%s\n",
				b.GetId(), b.GetTitle(), strings.Join(b.GetTags(), ", "))
		}
		fmt.Fprintf(w, "\nTotal: %d\n", resp.GetTotal())
		return nil
	})
}

func newBlueprintsFlowGetCommand() *cobra.Command {
	var legacy bool

	cmd := &cobra.Command{
		Use:     "get <id>",
		Short:   "Get a flow blueprint.",
		Aliases: []string{"show", "describe"},
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
			return runBlueprintsFlowGet(client, args[0], legacy, renderer)
		},
	}
	cmd.Flags().BoolVar(&legacy, "legacy", false, "Use the legacy /blueprints/flow/{id} endpoint")
	return cmd
}

func runBlueprintsFlowGet(client *Client, id string, legacy bool, renderer *Renderer) error {
	getFn := client.Kestra.Blueprints().FlowBlueprintById
	if legacy {
		getFn = client.Kestra.Blueprints().FlowBlueprint
	}
	b, err := getFn(client.Ctx, id, client.Tenant)
	if err != nil {
		return formatSDKError(err)
	}

	return renderer.Render(b, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "Blueprint Details")
		fmt.Fprintln(w)
		fmt.Fprintf(w, "ID:\t%s\n", b.GetId())
		fmt.Fprintf(w, "Title:\t%s\n", b.GetTitle())
		if desc := b.GetDescription(); desc != "" {
			fmt.Fprintf(w, "Description:\t%s\n", desc)
		}
		if tags := b.GetTags(); len(tags) > 0 {
			fmt.Fprintf(w, "Tags:\t%s\n", strings.Join(tags, ", "))
		}
		return nil
	})
}

func newBlueprintsFlowCreateCommand() *cobra.Command {
	var (
		title       string
		description string
		sourceFile  string
		tags        []string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new flow blueprint.",
		Example: `  kestractl blueprints flow create --title "My Blueprint" --source-file flow.yml
  kestractl blueprints flow create --title "My Blueprint" --source-file flow.yml --tag etl --tag prod`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if title == "" {
				return fmt.Errorf("--title is required")
			}
			if sourceFile == "" {
				return fmt.Errorf("--source-file is required")
			}
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runBlueprintsFlowCreate(client, title, description, sourceFile, tags, renderer)
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "Blueprint title (required)")
	cmd.Flags().StringVar(&description, "description", "", "Blueprint description")
	cmd.Flags().StringVar(&sourceFile, "source-file", "", "Path to the YAML flow source file (required)")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "Tag (repeatable)")

	return cmd
}

func runBlueprintsFlowCreate(client *Client, title, description, sourceFile string, tags []string, renderer *Renderer) error {
	src, err := os.ReadFile(sourceFile)
	if err != nil {
		return fmt.Errorf("failed to read source file: %w", err)
	}

	req := &kestra.BlueprintControllerFlowBlueprintCreateOrUpdate{
		Title:  title,
		Source: string(src),
	}
	if description != "" {
		req.Description = &description
	}
	if len(tags) > 0 {
		req.Tags = tags
	}

	b, err := client.Kestra.Blueprints().CreateFlowBlueprint(client.Ctx, client.Tenant, req)
	if err != nil {
		return formatSDKError(err)
	}

	return renderer.Render(b, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Blueprint created successfully.\n\nID:\t%s\nTitle:\t%s\n",
			b.GetId(), b.GetTitle())
		return nil
	})
}

func newBlueprintsFlowUpdateCommand() *cobra.Command {
	var (
		title       string
		description string
		sourceFile  string
		tags        []string
	)

	cmd := &cobra.Command{
		Use:     "update <id>",
		Short:   "Update a flow blueprint.",
		Args:    cobra.ExactArgs(1),
		Example: `  kestractl blueprints flow update <id> --title "Updated Blueprint" --source-file flow.yml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if title == "" {
				return fmt.Errorf("--title is required")
			}
			if sourceFile == "" {
				return fmt.Errorf("--source-file is required")
			}
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runBlueprintsFlowUpdate(client, args[0], title, description, sourceFile, tags, renderer)
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "Blueprint title (required)")
	cmd.Flags().StringVar(&description, "description", "", "Blueprint description")
	cmd.Flags().StringVar(&sourceFile, "source-file", "", "Path to the YAML flow source file (required)")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "Tag (repeatable)")

	return cmd
}

func runBlueprintsFlowUpdate(client *Client, id, title, description, sourceFile string, tags []string, renderer *Renderer) error {
	src, err := os.ReadFile(sourceFile)
	if err != nil {
		return fmt.Errorf("failed to read source file: %w", err)
	}

	req := &kestra.BlueprintControllerFlowBlueprintCreateOrUpdate{
		Title:  title,
		Source: string(src),
	}
	if description != "" {
		req.Description = &description
	}
	if len(tags) > 0 {
		req.Tags = tags
	}

	b, err := client.Kestra.Blueprints().UpdateFlowBlueprint(client.Ctx, id, client.Tenant, req)
	if err != nil {
		return formatSDKError(err)
	}

	return renderer.Render(b, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Blueprint updated successfully.\n\nID:\t%s\nTitle:\t%s\n",
			b.GetId(), b.GetTitle())
		return nil
	})
}

func newBlueprintsFlowDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete <id>",
		Short:   "Delete a flow blueprint.",
		Aliases: []string{"rm"},
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
			return runBlueprintsFlowDelete(client, args[0], renderer)
		},
	}
	return cmd
}

func runBlueprintsFlowDelete(client *Client, id string, renderer *Renderer) error {
	if err := client.Kestra.Blueprints().DeleteFlowBlueprints(client.Ctx, id, client.Tenant); err != nil {
		return formatSDKError(err)
	}
	return renderStatus(renderer, fmt.Sprintf("Blueprint '%s' deleted.", id),
		map[string]any{"id": id, "status": "deleted"})
}

func newBlueprintsFlowUseTemplateCommand() *cobra.Command {
	var inputs []string

	cmd := &cobra.Command{
		Use:   "use-template <id>",
		Short: "Generate a flow source from a flow blueprint template.",
		Args:  cobra.ExactArgs(1),
		Example: `  kestractl blueprints flow use-template <id>
  kestractl blueprints flow use-template <id> --input env=prod --input region=eu`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runBlueprintsFlowUseTemplate(client, args[0], inputs, cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringArrayVar(&inputs, "input", nil, "Template argument as key=value (repeatable)")
	return cmd
}

func runBlueprintsFlowUseTemplate(client *Client, id string, inputs []string, out io.Writer) error {
	args, err := parseBlueprintInputs(inputs)
	if err != nil {
		return err
	}

	req := kestra.NewBlueprintControllerUseBlueprintTemplateRequest()
	if len(args) > 0 {
		req.TemplateArgumentsInputs = args
	}

	resp, err := client.Kestra.Blueprints().UseBlueprintTemplate(client.Ctx, id, client.Tenant, req)
	if err != nil {
		return formatSDKError(err)
	}

	_, err = fmt.Fprint(out, resp.GetGeneratedFlowSource())
	return err
}

func parseBlueprintInputs(inputs []string) (map[string]interface{}, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	out := make(map[string]interface{}, len(inputs))
	for _, in := range inputs {
		parts := strings.SplitN(in, "=", 2)
		if len(parts) != 2 || parts[0] == "" {
			return nil, fmt.Errorf("invalid --input %q: expected key=value", in)
		}
		out[parts[0]] = parts[1]
	}
	return out, nil
}

// custom (internal) blueprint sub-group

func newBlueprintsCustomCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "custom",
		Short: "Manage internal (custom) blueprints.",
		Long: `Manage internal/custom blueprints.

These are tenant-stored blueprints served from the /blueprints/custom endpoints.
Use "kestractl blueprints flow list" to search them.`,
	}
	cmd.AddCommand(newBlueprintsCustomGetCommand())
	cmd.AddCommand(newBlueprintsCustomSourceCommand())
	cmd.AddCommand(newBlueprintsCustomCreateCommand())
	cmd.AddCommand(newBlueprintsCustomUpdateCommand())
	cmd.AddCommand(newBlueprintsCustomDeleteCommand())
	return cmd
}

func newBlueprintsCustomGetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get <id>",
		Short:   "Get an internal blueprint.",
		Aliases: []string{"show", "describe"},
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
			return runBlueprintsCustomGet(client, args[0], renderer)
		},
	}
	return cmd
}

func runBlueprintsCustomGet(client *Client, id string, renderer *Renderer) error {
	b, err := client.Kestra.Blueprints().InternalBlueprint(client.Ctx, id, client.Tenant)
	if err != nil {
		return formatSDKError(err)
	}

	return renderer.Render(b, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "Blueprint Details")
		fmt.Fprintln(w)
		fmt.Fprintf(w, "ID:\t%s\n", b.GetId())
		fmt.Fprintf(w, "Title:\t%s\n", b.GetTitle())
		if desc := b.GetDescription(); desc != "" {
			fmt.Fprintf(w, "Description:\t%s\n", desc)
		}
		if tags := b.GetTags(); len(tags) > 0 {
			fmt.Fprintf(w, "Tags:\t%s\n", strings.Join(tags, ", "))
		}
		return nil
	})
}

func newBlueprintsCustomSourceCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "source <id>",
		Short: "Print the YAML source of an internal blueprint.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runBlueprintsCustomSource(client, args[0], cmd.OutOrStdout())
		},
	}
	return cmd
}

// runBlueprintsCustomSource prints the YAML source of an internal blueprint.
//
// The dedicated /blueprints/custom/{id}/source endpoint only honors an
// "Accept: */*" header and returns 401 for the SDK's hardcoded text/plain
// Accept, so we read the source field from the full blueprint object instead.
func runBlueprintsCustomSource(client *Client, id string, out io.Writer) error {
	b, err := client.Kestra.Blueprints().InternalBlueprint(client.Ctx, id, client.Tenant)
	if err != nil {
		return formatSDKError(err)
	}
	_, err = fmt.Fprint(out, b.GetSource())
	return err
}

func newBlueprintsCustomCreateCommand() *cobra.Command {
	var (
		title       string
		description string
		sourceFile  string
		tags        []string
	)

	cmd := &cobra.Command{
		Use:     "create",
		Short:   "Create a new internal blueprint.",
		Example: `  kestractl blueprints custom create --title "My Blueprint" --source-file flow.yml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if title == "" {
				return fmt.Errorf("--title is required")
			}
			if sourceFile == "" {
				return fmt.Errorf("--source-file is required")
			}
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runBlueprintsCustomCreate(client, title, description, sourceFile, tags, renderer)
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "Blueprint title (required)")
	cmd.Flags().StringVar(&description, "description", "", "Blueprint description")
	cmd.Flags().StringVar(&sourceFile, "source-file", "", "Path to the YAML flow source file (required)")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "Tag (repeatable)")

	return cmd
}

func runBlueprintsCustomCreate(client *Client, title, description, sourceFile string, tags []string, renderer *Renderer) error {
	src, err := os.ReadFile(sourceFile)
	if err != nil {
		return fmt.Errorf("failed to read source file: %w", err)
	}

	req := &kestra.BlueprintControllerFlowBlueprintCreateOrUpdate{
		Title:  title,
		Source: string(src),
	}
	if description != "" {
		req.Description = &description
	}
	if len(tags) > 0 {
		req.Tags = tags
	}

	b, err := client.Kestra.Blueprints().CreateInternalBlueprints(client.Ctx, client.Tenant, req)
	if err != nil {
		return formatSDKError(err)
	}

	return renderer.Render(b, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Blueprint created successfully.\n\nID:\t%s\nTitle:\t%s\n",
			b.GetId(), b.GetTitle())
		return nil
	})
}

func newBlueprintsCustomUpdateCommand() *cobra.Command {
	var (
		title       string
		description string
		sourceFile  string
		tags        []string
	)

	cmd := &cobra.Command{
		Use:     "update <id>",
		Short:   "Update an internal blueprint.",
		Args:    cobra.ExactArgs(1),
		Example: `  kestractl blueprints custom update <id> --title "Updated" --source-file flow.yml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if title == "" {
				return fmt.Errorf("--title is required")
			}
			if sourceFile == "" {
				return fmt.Errorf("--source-file is required")
			}
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runBlueprintsCustomUpdate(client, args[0], title, description, sourceFile, tags, renderer)
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "Blueprint title (required)")
	cmd.Flags().StringVar(&description, "description", "", "Blueprint description")
	cmd.Flags().StringVar(&sourceFile, "source-file", "", "Path to the YAML flow source file (required)")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "Tag (repeatable)")

	return cmd
}

func runBlueprintsCustomUpdate(client *Client, id, title, description, sourceFile string, tags []string, renderer *Renderer) error {
	src, err := os.ReadFile(sourceFile)
	if err != nil {
		return fmt.Errorf("failed to read source file: %w", err)
	}

	req := &kestra.BlueprintControllerFlowBlueprintCreateOrUpdate{
		Title:  title,
		Source: string(src),
	}
	if description != "" {
		req.Description = &description
	}
	if len(tags) > 0 {
		req.Tags = tags
	}

	b, err := client.Kestra.Blueprints().UpdateInternalBlueprints(client.Ctx, id, client.Tenant, req)
	if err != nil {
		return formatSDKError(err)
	}

	return renderer.Render(b, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Blueprint updated successfully.\n\nID:\t%s\nTitle:\t%s\n",
			b.GetId(), b.GetTitle())
		return nil
	})
}

func newBlueprintsCustomDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete <id>",
		Short:   "Delete an internal blueprint.",
		Aliases: []string{"rm"},
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
			return runBlueprintsCustomDelete(client, args[0], renderer)
		},
	}
	return cmd
}

func runBlueprintsCustomDelete(client *Client, id string, renderer *Renderer) error {
	if err := client.Kestra.Blueprints().DeleteInternalBlueprints(client.Ctx, id, client.Tenant); err != nil {
		return formatSDKError(err)
	}
	return renderStatus(renderer, fmt.Sprintf("Blueprint '%s' deleted.", id),
		map[string]any{"id": id, "status": "deleted"})
}
