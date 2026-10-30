package cli

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"

	kestra "github.com/kestra-io/client-sdk/go-sdk/v2/kestra_api_client"
	"github.com/spf13/cobra"
)

const mcpServersFeature = "MCP servers"

func newMcpServersCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "mcp-servers",
		Aliases: []string{"mcp-server", "mcp"},
		Short:   "Manage MCP servers (list, get, tools, create, update, delete, enable, disable)",
		Long: `Manage the tenant's MCP servers.

An MCP server exposes the flows that declare an MCP Tool trigger as tools to
MCP clients. Every tenant has a 'default' server, which cannot be deleted.
Requires Kestra 2.0 or later.`,
	}

	cmd.AddCommand(newMcpServersListCommand())
	cmd.AddCommand(newMcpServersGetCommand())
	cmd.AddCommand(newMcpServersToolsCommand())
	cmd.AddCommand(newMcpServersCreateCommand())
	cmd.AddCommand(newMcpServersUpdateCommand())
	cmd.AddCommand(newMcpServersDeleteCommand())
	cmd.AddCommand(newMcpServersSetDisabledCommand("enable", "Enable an MCP server.", false))
	cmd.AddCommand(newMcpServersSetDisabledCommand("disable", "Disable an MCP server.", true))

	return cmd
}

func newMcpServersListCommand() *cobra.Command {
	var (
		page int
		size int
		sort []string
	)

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List MCP servers.",
		Aliases: []string{"ls"},
		Example: `  kestractl mcp-servers list
  kestractl mcp-servers list --output json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runMcpServersList(client, page, size, sort, renderer)
		},
	}

	cmd.Flags().IntVar(&page, "page", 1, "Page number")
	cmd.Flags().IntVar(&size, "size", 100, "Page size")
	cmd.Flags().StringArrayVar(&sort, "sort", nil, "Sort expression (e.g. 'id:asc', repeatable)")

	return cmd
}

func runMcpServersList(client *Client, page, size int, sort []string, renderer *Renderer) error {
	if err := requireKestra2(client, mcpServersFeature); err != nil {
		return err
	}
	resp, err := client.Kestra.McpServers().ListMcpServers(client.Ctx, client.Tenant, &page, &size, sort)
	if err != nil {
		return formatSDKError(err)
	}
	if resp == nil {
		resp = &kestra.PagedResultsApiMcpServer{}
	}

	results := resp.GetResults()
	if results == nil {
		results = []kestra.ApiMcpServer{}
	}

	return renderer.Render(results, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ID\tDEFAULT\tDISABLED\tTYPE\tAUTH\tDESCRIPTION")
		for _, server := range results {
			fmt.Fprintf(w, "%s\t%t\t%t\t%s\t%s\t%s\n",
				server.GetId(), server.GetIsDefault(), server.GetDisabled(),
				dashIfEmpty(server.GetServerType()), dashIfEmpty(server.GetAuthType()), dashIfEmpty(server.GetDescription()))
		}
		fmt.Fprintf(w, "\nTotal MCP servers: %d\n", resp.GetTotal())
		return nil
	})
}

func newMcpServersGetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get <id>",
		Short:   "Get an MCP server.",
		Aliases: []string{"show", "describe"},
		Example: `  kestractl mcp-servers get default
  kestractl mcp-servers get default --output json`,
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
			return runMcpServersGet(client, args[0], renderer)
		},
	}
	return cmd
}

func runMcpServersGet(client *Client, id string, renderer *Renderer) error {
	if err := requireKestra2(client, mcpServersFeature); err != nil {
		return err
	}
	server, err := fetchMcpServer(client, id)
	if err != nil {
		return err
	}
	return renderMcpServer(renderer, server)
}

func newMcpServersToolsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tools <id>",
		Short: "List the tools an MCP server exposes.",
		Long: `List the tools an MCP server exposes, one per flow with an MCP Tool trigger,
with the flow and trigger that back each tool.`,
		Example: `  kestractl mcp-servers tools default
  kestractl mcp-servers tools default --output json`,
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
			return runMcpServersTools(client, args[0], renderer)
		},
	}
	return cmd
}

func runMcpServersTools(client *Client, id string, renderer *Renderer) error {
	if err := requireKestra2(client, mcpServersFeature); err != nil {
		return err
	}
	tools, err := client.Kestra.McpServers().McpServerTools(client.Ctx, client.Tenant, id)
	if err != nil {
		return formatSDKError(err)
	}
	if tools == nil {
		tools = []kestra.ApiMcpTool{}
	}

	return renderer.Render(tools, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "TOOL\tNAMESPACE\tFLOW\tTRIGGER\tDISABLED")
		for _, tool := range tools {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%t\n",
				tool.GetToolName(), tool.GetNamespace(), tool.FlowId, tool.TriggerId, tool.Disabled)
		}
		fmt.Fprintf(w, "\nTotal tools: %d\n", len(tools))
		return nil
	})
}

func newMcpServersCreateCommand() *cobra.Command {
	var description, instructions string

	cmd := &cobra.Command{
		Use:   "create <id>",
		Short: "Create an MCP server.",
		Long: `Create an MCP server. The server type and authentication type are set by the
server.`,
		Example: `  kestractl mcp-servers create ops --description "Operations flows" \
    --instructions "Only call a tool when the request clearly maps to one of the flows."`,
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
			return runMcpServersCreate(client, args[0], description, instructions, renderer)
		},
	}

	cmd.Flags().StringVar(&description, "description", "", "Description of the server")
	cmd.Flags().StringVar(&instructions, "instructions", "", "Instructions sent to MCP clients on connection")
	return cmd
}

func runMcpServersCreate(client *Client, id, description, instructions string, renderer *Renderer) error {
	if err := requireKestra2(client, mcpServersFeature); err != nil {
		return err
	}
	server, err := client.Kestra.McpServers().CreateMcpServer(client.Ctx, client.Tenant, kestra.ApiMcpServer{
		Id:           id,
		Description:  description,
		Instructions: instructions,
	})
	if err != nil {
		return formatSDKError(err)
	}
	if server == nil {
		return fmt.Errorf("MCP server not returned by the server")
	}
	return renderer.Render(server, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "MCP server '%s' created.\n", server.GetId())
		return nil
	})
}

func newMcpServersUpdateCommand() *cobra.Command {
	var description, instructions string

	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update an MCP server's description or instructions.",
		Long: `Update an MCP server's description or instructions. Only the flags you pass
are changed; the other fields, including whether the server is disabled, are
kept as they are.`,
		Example: `  kestractl mcp-servers update ops --description "Operations and on-call flows"`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			changes := mcpServerChanges{}
			if cmd.Flags().Changed("description") {
				changes.description = &description
			}
			if cmd.Flags().Changed("instructions") {
				changes.instructions = &instructions
			}
			if changes.description == nil && changes.instructions == nil {
				return fmt.Errorf("nothing to update: pass --description and/or --instructions")
			}
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runMcpServersUpdate(client, args[0], changes, renderer)
		},
	}

	cmd.Flags().StringVar(&description, "description", "", "New description")
	cmd.Flags().StringVar(&instructions, "instructions", "", "New instructions sent to MCP clients on connection")
	return cmd
}

// mcpServerChanges holds the fields to change; nil means keep the current value.
type mcpServerChanges struct {
	description  *string
	instructions *string
}

func runMcpServersUpdate(client *Client, id string, changes mcpServerChanges, renderer *Renderer) error {
	if err := requireKestra2(client, mcpServersFeature); err != nil {
		return err
	}
	// PUT replaces the whole server: fields left out of the body are cleared and
	// `disabled` is always sent, so start from the current server.
	server, err := fetchMcpServer(client, id)
	if err != nil {
		return err
	}
	if changes.description != nil {
		server.Description = *changes.description
	}
	if changes.instructions != nil {
		server.Instructions = *changes.instructions
	}

	updated, err := client.Kestra.McpServers().UpdateMcpServer(client.Ctx, client.Tenant, id, *server)
	if err != nil {
		return formatSDKError(err)
	}
	if updated == nil {
		return fmt.Errorf("MCP server not returned by the server")
	}
	return renderer.Render(updated, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "MCP server '%s' updated.\n", updated.GetId())
		return nil
	})
}

func newMcpServersDeleteCommand() *cobra.Command {
	var skipConfirm bool

	cmd := &cobra.Command{
		Use:     "delete <id>",
		Short:   "Delete an MCP server.",
		Aliases: []string{"rm"},
		Long:    `Delete an MCP server. The tenant's 'default' server cannot be deleted.`,
		Example: `  kestractl mcp-servers delete ops --yes`,
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
			return runMcpServersDelete(client, args[0], skipConfirm, cmd.InOrStdin(), renderer)
		},
	}

	cmd.Flags().BoolVarP(&skipConfirm, "yes", "y", false, "Skip the confirmation prompt")
	return cmd
}

func runMcpServersDelete(client *Client, id string, skipConfirm bool, in io.Reader, renderer *Renderer) error {
	if err := requireKestra2(client, mcpServersFeature); err != nil {
		return err
	}
	if !skipConfirm {
		confirmed, err := confirm(in, os.Stderr,
			fmt.Sprintf("Are you sure you want to delete MCP server '%s'? [y/N]: ", id))
		if err != nil {
			return err
		}
		if !confirmed {
			return renderStatus(renderer, "Cancelled.", map[string]any{"id": id, "status": "cancelled"})
		}
	}

	if err := client.Kestra.McpServers().DeleteMcpServer(client.Ctx, client.Tenant, id); err != nil {
		return formatSDKError(err)
	}
	return renderStatus(renderer, fmt.Sprintf("MCP server '%s' deleted.", id),
		map[string]any{"id": id, "status": "deleted"})
}

func newMcpServersSetDisabledCommand(name, short string, disabled bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:     name + " <id>",
		Short:   short,
		Example: fmt.Sprintf("  kestractl mcp-servers %s ops", name),
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
			return runMcpServersSetDisabled(client, args[0], disabled, renderer)
		},
	}
	return cmd
}

// runMcpServersSetDisabled sets the state explicitly instead of calling the
// toggle endpoint, so running `disable` twice leaves the server disabled.
func runMcpServersSetDisabled(client *Client, id string, disabled bool, renderer *Renderer) error {
	if err := requireKestra2(client, mcpServersFeature); err != nil {
		return err
	}
	state := "enabled"
	if disabled {
		state = "disabled"
	}

	server, err := fetchMcpServer(client, id)
	if err != nil {
		return err
	}
	if server.GetDisabled() == disabled {
		return renderStatus(renderer, fmt.Sprintf("MCP server '%s' is already %s.", id, state),
			map[string]any{"id": id, "status": state, "changed": false})
	}

	server.Disabled = disabled
	if _, err := client.Kestra.McpServers().UpdateMcpServer(client.Ctx, client.Tenant, id, *server); err != nil {
		return formatSDKError(err)
	}
	return renderStatus(renderer, fmt.Sprintf("MCP server '%s' %s.", id, state),
		map[string]any{"id": id, "status": state, "changed": true})
}

func fetchMcpServer(client *Client, id string) (*kestra.ApiMcpServer, error) {
	server, err := client.Kestra.McpServers().McpServer(client.Ctx, client.Tenant, id)
	if err != nil {
		return nil, formatSDKError(err)
	}
	if server == nil {
		return nil, fmt.Errorf("MCP server '%s' not found", id)
	}
	return server, nil
}

func renderMcpServer(renderer *Renderer, server *kestra.ApiMcpServer) error {
	return renderer.Render(server, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "ID:\t%s\n", server.GetId())
		fmt.Fprintf(w, "Default:\t%t\n", server.GetIsDefault())
		fmt.Fprintf(w, "Disabled:\t%t\n", server.GetDisabled())
		fmt.Fprintf(w, "Type:\t%s\n", dashIfEmpty(server.GetServerType()))
		fmt.Fprintf(w, "Auth:\t%s\n", dashIfEmpty(server.GetAuthType()))
		fmt.Fprintf(w, "Description:\t%s\n", dashIfEmpty(server.GetDescription()))
		fmt.Fprintf(w, "Instructions:\t%s\n", dashIfEmpty(server.Instructions))
		if server.Updated != nil {
			fmt.Fprintf(w, "Updated:\t%s\n", server.Updated.Format(time.RFC3339))
		}
		return nil
	})
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
