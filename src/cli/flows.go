package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	kestra "github.com/kestra-io/client-sdk/go-sdk/kestra_api_client"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// DeployResult holds the result of deploying a single flow.
type DeployResult struct {
	FilePath  string `json:"file_path"`
	FlowID    string `json:"flow_id"`
	Namespace string `json:"namespace"`
	Revision  int32  `json:"revision,omitempty"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
}

// ValidateResult holds the result of validating a single flow file.
type ValidateResult struct {
	FilePath         string   `json:"file_path"`
	FlowID           string   `json:"flow_id,omitempty"`
	Namespace        string   `json:"namespace,omitempty"`
	Constraints      []string `json:"constraints,omitempty"`
	Warnings         []string `json:"warnings,omitempty"`
	Infos            []string `json:"infos,omitempty"`
	DeprecationPaths []string `json:"deprecation_paths,omitempty"`
	Outdated         bool     `json:"outdated,omitempty"`
	Success          bool     `json:"success"`
}

func newFlowsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "flows",
		Short: "Manage flows (create, update, list, delete)",
	}

	cmd.AddCommand(newFlowsListCommand())
	cmd.AddCommand(newFlowsGetCommand())
	cmd.AddCommand(newFlowsNamespacesCommand())
	cmd.AddCommand(newFlowsRevisionsCommand())
	cmd.AddCommand(newFlowsDependenciesCommand())
	cmd.AddCommand(newFlowsDeployCommand())
	cmd.AddCommand(newFlowsValidateCommand())
	cmd.AddCommand(newFlowsEnableCommand())
	cmd.AddCommand(newFlowsDisableCommand())
	cmd.AddCommand(newFlowsExportCommand())
	cmd.AddCommand(newFlowsImportCommand())
	cmd.AddCommand(newFlowsDeleteCommand())

	return cmd
}

func newFlowsDeleteCommand() *cobra.Command {
	var skipConfirm bool

	cmd := &cobra.Command{
		Use:   "delete <namespace> <flow_id>",
		Short: "Delete a flow.",
		Long: `Delete a flow from a namespace.

Prompts for confirmation unless --yes is provided.`,
		Example: `  # Delete a flow (with confirmation)
	  kestractl flows delete my.namespace my-flow

	  # Delete without confirmation
	  kestractl flows delete my.namespace my-flow --yes`,
		Aliases: []string{"rm", "del"},
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

			return runFlowsDelete(client, args[0], args[1], skipConfirm, cmd.InOrStdin(), renderer)
		},
	}

	cmd.Flags().BoolVarP(&skipConfirm, "yes", "y", false, "Skip the confirmation prompt")

	return cmd
}

func runFlowsDelete(client *Client, namespace, flowID string, skipConfirm bool, in io.Reader, renderer *Renderer) error {
	if !skipConfirm {
		confirmed, err := confirm(in, os.Stderr,
			fmt.Sprintf("Are you sure you want to delete flow '%s' in namespace '%s'? [y/N]: ", flowID, namespace))
		if err != nil {
			return err
		}
		if !confirmed {
			return renderStatus(renderer, "Cancelled.",
				map[string]any{"namespace": namespace, "flowId": flowID, "status": "cancelled"})
		}
	}

	_, err := client.API.FlowsAPI.DeleteFlow(client.Ctx, namespace, flowID, client.Tenant).Execute()
	if err != nil {
		return formatSDKError(err)
	}

	return renderStatus(renderer, fmt.Sprintf("Flow '%s' deleted from namespace '%s'.", flowID, namespace),
		map[string]any{"namespace": namespace, "flowId": flowID, "status": "deleted"})
}

func newFlowsEnableCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enable <namespace> <flow_id> [flow_id...]",
		Short: "Enable one or more flows.",
		Long: `Enable one or more flows in a namespace. Disabled flows do not execute,
even when their triggers fire; enabling them restores normal scheduling.`,
		Example: `  # Enable a single flow
	  kestractl flows enable my.namespace my-flow

	  # Enable several flows at once
	  kestractl flows enable my.namespace flow-a flow-b flow-c`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := NewClient()
			if err != nil {
				return err
			}
			return runFlowsToggle(client, args[0], args[1:], true, renderer)
		},
	}

	return cmd
}

func newFlowsDisableCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disable <namespace> <flow_id> [flow_id...]",
		Short: "Disable one or more flows.",
		Long: `Disable one or more flows in a namespace. Disabled flows are not executed,
even when their triggers fire, until they are re-enabled.`,
		Example: `  # Disable a single flow
	  kestractl flows disable my.namespace my-flow

	  # Disable several flows at once
	  kestractl flows disable my.namespace flow-a flow-b flow-c`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := NewClient()
			if err != nil {
				return err
			}
			return runFlowsToggle(client, args[0], args[1:], false, renderer)
		},
	}

	return cmd
}

// runFlowsToggle enables or disables the given flows (all within one namespace)
// via the bulk by-ids endpoints.
func runFlowsToggle(client *Client, namespace string, flowIDs []string, enable bool, renderer *Renderer) error {
	items := make([]kestra.IdWithNamespace, 0, len(flowIDs))
	for _, id := range flowIDs {
		item := kestra.NewIdWithNamespace()
		item.SetNamespace(namespace)
		item.SetId(id)
		items = append(items, *item)
	}

	var (
		resp *kestra.BulkResponse
		err  error
	)
	if enable {
		resp, _, err = client.API.FlowsAPI.EnableFlowsByIds(client.Ctx, client.Tenant).
			IdWithNamespace(items).Execute()
	} else {
		resp, _, err = client.API.FlowsAPI.DisableFlowsByIds(client.Ctx, client.Tenant).
			IdWithNamespace(items).Execute()
	}
	if err != nil {
		return formatSDKError(err)
	}

	action := "enabled"
	if !enable {
		action = "disabled"
	}
	count := len(flowIDs)
	if resp != nil {
		count = int(resp.GetCount())
	}
	return renderStatus(renderer, fmt.Sprintf("%d flow(s) %s in namespace '%s'.", count, action, namespace),
		map[string]any{"namespace": namespace, "count": count, "status": action})
}

func newFlowsDependenciesCommand() *cobra.Command {
	var (
		expandAll       bool
		destinationOnly bool
	)

	cmd := &cobra.Command{
		Use:   "dependencies <namespace> <flow_id>",
		Short: "Show the dependency graph of a flow.",
		Long: `Show the flows that depend on, or are depended upon by, a flow.

Dependencies are produced by subflow tasks and flow triggers. Use --expand-all
to recursively expand the graph, and --destination-only to only show flows that
this flow depends on (its downstream destinations).`,
		Example: `  # Show a flow's direct dependencies
	  kestractl flows dependencies my.namespace my-flow

	  # Expand the full dependency graph as JSON
	  kestractl flows dependencies my.namespace my-flow --expand-all --output json`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := NewClient()
			if err != nil {
				return err
			}
			return runFlowsDependencies(client, args[0], args[1], expandAll, destinationOnly, renderer)
		},
	}

	cmd.Flags().BoolVar(&expandAll, "expand-all", false, "Recursively expand the full dependency graph")
	cmd.Flags().BoolVar(&destinationOnly, "destination-only", false, "Only show flows this flow depends on")

	return cmd
}

func runFlowsDependencies(client *Client, namespace, flowID string, expandAll, destinationOnly bool, renderer *Renderer) error {
	req := client.API.FlowsAPI.FlowDependencies(client.Ctx, namespace, flowID, client.Tenant)
	if expandAll {
		req = req.ExpandAll(true)
	}
	if destinationOnly {
		req = req.DestinationOnly(true)
	}
	graph, _, err := req.Execute()
	if err != nil {
		return formatSDKError(err)
	}
	if graph == nil {
		graph = kestra.NewFlowTopologyGraphWithDefaults()
	}

	nodes := graph.GetNodes()
	edges := graph.GetEdges()

	if renderer.IsJSON() {
		nodeList := make([]map[string]any, len(nodes))
		for i, n := range nodes {
			nodeList[i] = map[string]any{
				"uid":       n.GetUid(),
				"id":        n.GetId(),
				"namespace": n.GetNamespace(),
			}
		}
		edgeList := make([]map[string]any, len(edges))
		for i, e := range edges {
			edgeList[i] = map[string]any{
				"source":   e.GetSource(),
				"target":   e.GetTarget(),
				"relation": stringify(e.GetRelation()),
			}
		}
		return renderer.RenderJSON(map[string]any{"nodes": nodeList, "edges": edgeList})
	}

	return renderer.Render(edges, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "SOURCE\tTARGET\tRELATION")
		for _, e := range edges {
			fmt.Fprintf(w, "%s\t%s\t%s\n", e.GetSource(), e.GetTarget(), stringify(e.GetRelation()))
		}
		fmt.Fprintf(w, "\n%d node(s), %d dependency edge(s)\n", len(nodes), len(edges))
		return nil
	})
}

func newFlowsExportCommand() *cobra.Command {
	var (
		namespace  string
		query      string
		outputFile string
	)

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export flows as a ZIP archive.",
		Long: `Export flows matching the given filters as a ZIP archive of YAML files.

By default every flow in the tenant is exported. Use --namespace and/or
--query to narrow the selection. The archive is written to flows.zip unless
--output-file is given (use '-' to stream to stdout).`,
		Example: `  # Export all flows to flows.zip
	  kestractl flows export

	  # Export a single namespace to a custom file
	  kestractl flows export --namespace my.namespace --output-file my-ns.zip

	  # Stream the archive to stdout
	  kestractl flows export --output-file -`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := NewClient()
			if err != nil {
				return err
			}
			filters := buildFlowExportFilters(namespace, query)
			return runFlowsExport(client, filters, outputFile, cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Only export flows in this namespace")
	cmd.Flags().StringVarP(&query, "query", "q", "", "Only export flows matching this free-text query")
	cmd.Flags().StringVarP(&outputFile, "output-file", "f", "flows.zip", "Write the archive to this file ('-' for stdout)")

	return cmd
}

// buildFlowExportFilters assembles the QueryFilter list for a flow export from
// the optional namespace and free-text query selectors.
func buildFlowExportFilters(namespace, query string) []kestra.QueryFilter {
	filters := make([]kestra.QueryFilter, 0, 2)
	if namespace != "" {
		filters = append(filters, equalsFilter(kestra.QUERYFILTERFIELD_NAMESPACE, namespace))
	}
	if query != "" {
		filters = append(filters, equalsFilter(kestra.QUERYFILTERFIELD_QUERY, query))
	}
	return filters
}

func runFlowsExport(client *Client, filters []kestra.QueryFilter, outputFile string, out io.Writer) error {
	req := client.API.FlowsAPI.ExportFlowsByQuery(client.Ctx, client.Tenant)
	if len(filters) > 0 {
		req = req.Filters(filters)
	}
	archive, _, err := req.Execute()
	if err != nil {
		return formatSDKError(err)
	}

	if outputFile == "" || outputFile == "-" {
		_, err = io.WriteString(out, archive)
		return err
	}

	if err := os.WriteFile(outputFile, []byte(archive), 0o644); err != nil {
		return fmt.Errorf("failed to write archive to %q: %w", outputFile, err)
	}
	fmt.Fprintf(out, "Flows exported to %s (%d bytes)\n", outputFile, len(archive))
	return nil
}

func newFlowsImportCommand() *cobra.Command {
	var failOnError bool

	cmd := &cobra.Command{
		Use:   "import <file>",
		Short: "Import flows from a YAML file or ZIP archive.",
		Long: `Import flows from a single YAML file or a ZIP archive of YAML files,
such as one produced by 'flows export'.

By default the import continues past individual failures. Use --fail-on-error
to abort the whole import as soon as one flow fails to import.`,
		Example: `  # Import flows from an archive
	  kestractl flows import flows.zip

	  # Import a single flow file, aborting on the first error
	  kestractl flows import my-flow.yaml --fail-on-error`,
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
			return runFlowsImport(client, args[0], failOnError, renderer)
		},
	}

	cmd.Flags().BoolVar(&failOnError, "fail-on-error", false, "Abort the import as soon as one flow fails")

	return cmd
}

func runFlowsImport(client *Client, path string, failOnError bool, renderer *Renderer) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open %q: %w", path, err)
	}
	defer file.Close()

	imported, _, err := client.API.FlowsAPI.ImportFlows(client.Ctx, client.Tenant).
		FileUpload(file).
		FailOnError(failOnError).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	return renderer.Render(imported, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "IMPORTED")
		for _, entry := range imported {
			fmt.Fprintln(w, entry)
		}
		fmt.Fprintf(w, "\nTotal flows imported: %d\n", len(imported))
		return nil
	})
}

func newFlowsListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list [namespace]",
		Short: "List flows.",
		Long: `List flows in a namespace or across all namespaces.

Returns a table showing flow ID, namespace, description, and revision number.`,
		Example: `  # List all flows in a namespace
	  kestractl flows list my.namespace

	  # List flows across all namespaces
	  kestractl flows list

	  # List flows with JSON output
	  kestractl flows list my.namespace --output json`,
		Aliases: []string{"ls"},
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := NewClient()
			if err != nil {
				return err
			}

			namespace := ""
			if len(args) == 1 {
				namespace = args[0]
			}
			return runFlowsList(client, namespace, renderer)
		},
	}
}

func runFlowsList(client *Client, namespace string, renderer *Renderer) error {
	var flows []kestra.Flow
	if namespace == "" {
		var err error
		flows, err = listAllFlows(client)
		if err != nil {
			return err
		}
	} else {
		var err error
		flows, _, err = client.API.FlowsAPI.ListFlowsByNamespace(client.Ctx, namespace, client.Tenant).Execute()
		if err != nil {
			return formatSDKError(err)
		}
	}

	result := make([]map[string]any, len(flows))
	for i, flow := range flows {
		result[i] = map[string]any{
			"id":          flow.GetId(),
			"namespace":   flow.GetNamespace(),
			"description": flow.GetDescription(),
			"revision":    flow.GetRevision(),
		}
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ID\tNamespace\tDescription\tRevision")
		for _, flow := range flows {
			desc := flow.GetDescription()
			if desc == "" {
				desc = "-"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\n",
				flow.GetId(),
				flow.GetNamespace(),
				desc,
				flow.GetRevision())
		}
		return nil
	})
}

func listAllFlows(client *Client) ([]kestra.Flow, error) {
	const pageSize int32 = 1000
	page := int32(1)
	results := make([]kestra.Flow, 0)

	for {
		resp, _, err := client.API.FlowsAPI.SearchFlows(client.Ctx, client.Tenant).
			Page(page).
			Size(pageSize).
			Execute()
		if err != nil {
			return nil, formatSDKError(err)
		}
		batch := resp.GetResults()
		results = append(results, batch...)

		if len(batch) == 0 || int64(len(results)) >= resp.GetTotal() {
			break
		}
		page++
	}

	return results, nil
}

func newFlowsGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "get <namespace> <flow_id>",
		Short: "Get a flow definition.",
		Long: `Retrieve the flow source and write it to stdout.

Use --output json to include metadata alongside the source.`,
		Example: `  # Get a flow source (YAML)
	  kestractl flows get my.namespace my-flow

	  # Get a flow as JSON with source
	  kestractl flows get my.namespace my-flow --output json`,
		Aliases: []string{"show", "describe"},
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

			return runFlowsGet(client, args[0], args[1], renderer)
		},
	}
}

func runFlowsGet(client *Client, namespace, flowID string, renderer *Renderer) error {
	flow, _, err := client.API.FlowsAPI.Flow(client.Ctx, namespace, flowID, client.Tenant).
		Source(true).
		AllowDeleted(false).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}
	if flow == nil {
		return fmt.Errorf("flow not found")
	}

	source := flow.GetSource()

	if renderer.IsJSON() {
		result := map[string]any{
			"id":        flow.GetId(),
			"namespace": flow.GetNamespace(),
			"revision":  flow.GetRevision(),
			"source":    source,
		}
		return renderer.RenderJSON(result)
	}

	if source == "" {
		return fmt.Errorf("flow source is empty")
	}
	_, err = io.WriteString(renderer.Writer(), source)
	return err
}

func newFlowsNamespacesCommand() *cobra.Command {
	var query string

	cmd := &cobra.Command{
		Use:   "namespaces",
		Short: "List the distinct namespaces that contain flows.",
		Long: `List every namespace that currently holds at least one flow.

Use --query to filter the namespaces by a free-text prefix.`,
		Example: `  # List all namespaces with flows
	  kestractl flows namespaces

	  # Filter namespaces
	  kestractl flows namespaces --query company.team`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := NewClient()
			if err != nil {
				return err
			}
			return runFlowsNamespaces(client, query, renderer)
		},
	}

	cmd.Flags().StringVarP(&query, "query", "q", "", "Filter namespaces by a free-text query")

	return cmd
}

func runFlowsNamespaces(client *Client, query string, renderer *Renderer) error {
	req := client.API.FlowsAPI.ListDistinctNamespaces(client.Ctx, client.Tenant)
	if query != "" {
		req = req.Q(query)
	}
	namespaces, _, err := req.Execute()
	if err != nil {
		return formatSDKError(err)
	}

	return renderer.Render(namespaces, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "NAMESPACE")
		for _, ns := range namespaces {
			fmt.Fprintln(w, ns)
		}
		fmt.Fprintf(w, "\nTotal namespaces: %d\n", len(namespaces))
		return nil
	})
}

func newFlowsRevisionsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "revisions <namespace> <flow_id>",
		Short: "List the revisions of a flow.",
		Long: `List every stored revision of a flow, newest revisions last.

Each revision is an immutable snapshot of the flow source created when the
flow was updated.`,
		Example: `  # List a flow's revisions
	  kestractl flows revisions my.namespace my-flow

	  # JSON output (includes the source of each revision)
	  kestractl flows revisions my.namespace my-flow --output json`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := NewClient()
			if err != nil {
				return err
			}
			return runFlowsRevisions(client, args[0], args[1], renderer)
		},
	}

	return cmd
}

func runFlowsRevisions(client *Client, namespace, flowID string, renderer *Renderer) error {
	revisions, _, err := client.API.FlowsAPI.ListFlowRevisions(client.Ctx, namespace, flowID, client.Tenant).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	if renderer.IsJSON() {
		result := make([]map[string]any, len(revisions))
		for i, rev := range revisions {
			result[i] = map[string]any{
				"revision":  rev.GetRevision(),
				"namespace": rev.GetNamespace(),
				"id":        rev.GetId(),
				"disabled":  rev.GetDisabled(),
				"source":    rev.GetSource(),
			}
		}
		return renderer.RenderJSON(result)
	}

	return renderer.Render(revisions, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "REVISION\tDISABLED\tDESCRIPTION")
		for _, rev := range revisions {
			description := rev.GetDescription()
			if description == "" {
				description = "-"
			}
			fmt.Fprintf(w, "%d\t%t\t%s\n", rev.GetRevision(), rev.GetDisabled(), description)
		}
		fmt.Fprintf(w, "\nTotal revisions: %d\n", len(revisions))
		return nil
	})
}

func newFlowsDeployCommand() *cobra.Command {
	var (
		override          bool
		namespaceOverride string
		failFast          bool
		recursive         bool
	)

	cmd := &cobra.Command{
		Use:          "deploy <path>",
		Short:        "Deploy flows from a YAML file or directory.",
		SilenceUsage: true,
		Long: `Deploy flow definitions from a local YAML file or directory to Kestra.

When a directory is provided, all .yaml and .yml files are deployed recursively by default.
Pass --recursive=false to scan only the top-level directory.
Hidden files and directories (starting with .) are skipped.

By default, the deployment will fail if a flow already exists.
Use --override to update existing flows.

By default, when deploying multiple flows, all files are processed even if some fail.
Use --fail-fast to stop on the first error.`,
		Example: `  # Deploy a single flow
	  kestractl flows deploy flow.yaml

	  # Deploy all flows in a directory (recursive)
	  kestractl flows deploy ./flows/

	  # Deploy only top-level flows in a directory (no recursion)
	  kestractl flows deploy ./flows/ --recursive=false

	  # Deploy with namespace override (all flows go to specified namespace)
	  kestractl flows deploy ./flows/ --namespace prod.namespace

	  # Stop on first error (fail-fast)
	  kestractl flows deploy ./flows/ --fail-fast

	  # Override existing flows
	  kestractl flows deploy ./flows/ --override

	  # Combine flags
	  kestractl flows deploy ./flows/ --namespace prod --override --fail-fast

	  # Deploy with JSON output
	  kestractl flows deploy flow.yaml --output json`,
		Aliases: []string{"create", "apply"},
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

			return runFlowsDeploy(client, args[0], override, namespaceOverride, failFast, recursive, renderer)
		},
	}

	cmd.Flags().BoolVar(&override, "override", false, "Override the flow if it already exists")
	cmd.Flags().StringVar(&namespaceOverride, "namespace", "", "Override the namespace for all deployed flows")
	cmd.Flags().BoolVar(&failFast, "fail-fast", false, "Stop on the first deployment error")
	cmd.Flags().BoolVar(&recursive, "recursive", true, "Recurse into subdirectories when a directory is provided")

	return cmd
}

func newFlowsValidateCommand() *cobra.Command {
	var recursive bool

	cmd := &cobra.Command{
		Use:          "validate <path>",
		Short:        "Validate flows from a YAML file or directory.",
		SilenceUsage: true,
		Long: `Validate flow definitions from a local YAML file or directory.

When a directory is provided, all .yaml and .yml files are validated recursively by default.
Pass --recursive=false to scan only the top-level directory.
Hidden files and directories (starting with .) are skipped.

Validation fails if any flow has constraint violations.
Warnings, infos, deprecations, and outdated flags are reported but do not fail validation.`,
		Example: `  # Validate a single flow
	  kestractl flows validate flow.yaml

	  # Validate all flows in a directory (recursive)
	  kestractl flows validate ./flows/

	  # Validate only top-level flows in a directory (no recursion)
	  kestractl flows validate ./flows/ --recursive=false

	  # Validate with JSON output
	  kestractl flows validate ./flows/ --output json`,
		Aliases: []string{"check"},
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

			return runFlowsValidate(client, args[0], recursive, renderer)
		},
	}

	cmd.Flags().BoolVar(&recursive, "recursive", true, "Recurse into subdirectories when a directory is provided")

	return cmd
}

func runFlowsValidate(client *Client, path string, recursive bool, renderer *Renderer) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to access path '%s': %w", path, err)
	}

	var files []string
	if info.IsDir() {
		files, err = collectFlowFiles(path, recursive)
		if err != nil {
			return fmt.Errorf("failed to scan directory '%s': %w", path, err)
		}
		if len(files) == 0 {
			return fmt.Errorf("no .yaml or .yml files found in directory '%s'", path)
		}
	} else {
		files = []string{path}
	}

	contents := make([]string, 0, len(files))
	for _, file := range files {
		data, readErr := os.ReadFile(file)
		if readErr != nil {
			return fmt.Errorf("failed to read file '%s': %w", file, readErr)
		}
		contents = append(contents, string(data))
	}

	body := strings.Join(contents, "\n---\n")

	violations, _, err := client.API.FlowsAPI.ValidateFlows(client.Ctx, client.Tenant).
		Body(body).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	return formatValidateResults(violations, files, renderer)
}

func formatValidateResults(violations []kestra.ValidateConstraintViolation, files []string, renderer *Renderer) error {
	results := make([]ValidateResult, len(files))
	for i, file := range files {
		results[i] = ValidateResult{
			FilePath: file,
			Success:  true,
		}
	}

	unknownResults := make([]ValidateResult, 0)

	for _, violation := range violations {
		index := int(violation.GetIndex())
		var target *ValidateResult
		if index >= 0 && index < len(results) {
			target = &results[index]
		} else {
			unknownResults = append(unknownResults, ValidateResult{
				FilePath: fmt.Sprintf("<index %d>", index),
				Success:  true,
			})
			target = &unknownResults[len(unknownResults)-1]
		}

		if flowID := violation.GetFlow(); strings.TrimSpace(flowID) != "" {
			target.FlowID = flowID
		}
		if namespace := violation.GetNamespace(); strings.TrimSpace(namespace) != "" {
			target.Namespace = namespace
		}
		if constraint := strings.TrimSpace(violation.GetConstraints()); constraint != "" {
			constraint = strings.ReplaceAll(constraint, "\n", "; ")
			target.Constraints = appendUniqueString(target.Constraints, constraint)
		}
		if warnings := violation.GetWarnings(); len(warnings) > 0 {
			for _, warning := range warnings {
				warning = strings.ReplaceAll(strings.TrimSpace(warning), "\n", "; ")
				if warning != "" {
					target.Warnings = appendUniqueString(target.Warnings, warning)
				}
			}
		}
		if infos := violation.GetInfos(); len(infos) > 0 {
			for _, info := range infos {
				info = strings.ReplaceAll(strings.TrimSpace(info), "\n", "; ")
				if info != "" {
					target.Infos = appendUniqueString(target.Infos, info)
				}
			}
		}
		if deps := violation.GetDeprecationPaths(); len(deps) > 0 {
			for _, dep := range deps {
				dep = strings.TrimSpace(dep)
				if dep != "" {
					target.DeprecationPaths = appendUniqueString(target.DeprecationPaths, dep)
				}
			}
		}
		if violation.GetOutdated() {
			target.Outdated = true
		}
	}

	allResults := results
	if len(unknownResults) > 0 {
		allResults = append(allResults, unknownResults...)
	}

	failed := 0
	for i := range allResults {
		if len(allResults[i].Constraints) > 0 {
			allResults[i].Success = false
			failed++
		} else {
			allResults[i].Success = true
		}
	}

	valid := len(allResults) - failed
	if err := renderer.Render(allResults, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "FILE\tSTATUS\tCONSTRAINTS\tWARNINGS\tINFOS\tOUTDATED\tDEPRECATIONS")
		for _, result := range allResults {
			status := "OK"
			constraints := "-"
			if len(result.Constraints) > 0 {
				status = "FAILED"
				constraints = strings.Join(result.Constraints, "; ")
			}
			warnings := "-"
			if len(result.Warnings) > 0 {
				warnings = fmt.Sprintf("%d", len(result.Warnings))
			}
			infos := "-"
			if len(result.Infos) > 0 {
				infos = fmt.Sprintf("%d", len(result.Infos))
			}
			outdated := "false"
			if result.Outdated {
				outdated = "true"
			}
			deprecations := "-"
			if len(result.DeprecationPaths) > 0 {
				deprecations = fmt.Sprintf("%d", len(result.DeprecationPaths))
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				result.FilePath,
				status,
				constraints,
				warnings,
				infos,
				outdated,
				deprecations,
			)
		}
		fmt.Fprintf(w, "\n%d valid flow(s), %d failed\n", valid, failed)
		return nil
	}); err != nil {
		return err
	}
	if failed > 0 {
		return fmt.Errorf("validation failed for %d flow(s)", failed)
	}
	return nil
}

func appendUniqueString(list []string, value string) []string {
	for _, existing := range list {
		if existing == value {
			return list
		}
	}
	return append(list, value)
}

func runFlowsDeploy(client *Client, path string, override bool, namespaceOverride string, failFast bool, recursive bool, renderer *Renderer) error {
	// Check if path is a file or directory
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to access path '%s': %w", path, err)
	}

	var files []string
	if info.IsDir() {
		files, err = collectFlowFiles(path, recursive)
		if err != nil {
			return fmt.Errorf("failed to scan directory '%s': %w", path, err)
		}
		if len(files) == 0 {
			return fmt.Errorf("no .yaml or .yml files found in directory '%s'", path)
		}
	} else {
		// Single file
		files = []string{path}
	}

	// Deploy all flows
	results := make([]DeployResult, 0, len(files))
	for _, file := range files {
		result := deployFlow(client, file, namespaceOverride, override)
		results = append(results, result)

		if !result.Success && failFast {
			// Stop on first error when fail-fast is enabled
			break
		}
	}

	// Output results
	return formatDeployResults(results, len(files) == 1, renderer)
}

// collectFlowFiles collects .yaml and .yml files from a directory.
// Hidden files and directories (starting with .) are skipped.
// When recursive is false, only the top-level directory is scanned.
func collectFlowFiles(rootPath string, recursive bool) ([]string, error) {
	var files []string
	err := filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Skip hidden files/dirs (but not the root path itself if it starts with .)
		if strings.HasPrefix(d.Name(), ".") && path != rootPath {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if !recursive && path != rootPath {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".yaml" || ext == ".yml" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files) // Deterministic order
	return files, nil
}

// deployFlow deploys a single flow file and returns the result.
func deployFlow(client *Client, filePath string, namespaceOverride string, override bool) DeployResult {
	result := DeployResult{
		FilePath: filePath,
		Success:  false,
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		result.Error = fmt.Sprintf("failed to read file: %v", err)
		return result
	}

	yamlContent := string(content)
	namespace, flowID, err := parseFlowYAML(yamlContent)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	result.FlowID = flowID

	// Apply namespace override if provided
	if namespaceOverride != "" {
		// Modify YAML content to use the overridden namespace
		yamlContent, err = replaceNamespaceInYAML(yamlContent, namespaceOverride)
		if err != nil {
			result.Error = fmt.Sprintf("failed to override namespace: %v", err)
			return result
		}
		namespace = namespaceOverride
	}
	result.Namespace = namespace

	// Check if flow exists
	exists := false
	_, resp, checkErr := client.API.FlowsAPI.Flow(client.Ctx, namespace, flowID, client.Tenant).
		Source(false).
		AllowDeleted(false).
		Execute()
	if checkErr == nil {
		exists = true
	} else if resp != nil && resp.StatusCode != 404 {
		result.Error = formatSDKError(checkErr).Error()
		return result
	}

	if exists && !override {
		result.Error = fmt.Sprintf("flow '%s' already exists in namespace '%s'; use --override to update", flowID, namespace)
		return result
	}

	if exists && override {
		// Update existing flow
		updateResp, _, err := client.API.FlowsAPI.UpdateFlow(client.Ctx, namespace, flowID, client.Tenant).
			Body(yamlContent).
			Execute()
		if err != nil {
			result.Error = formatSDKError(err).Error()
			return result
		}
		result.Revision = updateResp.GetRevision()
	} else {
		// Create new flow
		flowResp, _, err := client.API.FlowsAPI.CreateFlow(client.Ctx, client.Tenant).
			Body(yamlContent).
			Execute()
		if err != nil {
			result.Error = formatSDKError(err).Error()
			return result
		}
		result.Revision = flowResp.GetRevision()
	}

	result.Success = true
	return result
}

// formatDeployResults outputs the deployment results in the appropriate format.
func formatDeployResults(results []DeployResult, singleFile bool, renderer *Renderer) error {
	// Count successes and failures
	var successCount, failCount int
	for _, r := range results {
		if r.Success {
			successCount++
		} else {
			failCount++
		}
	}

	if err := renderer.Render(results, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "FILE\tFLOW ID\tNAMESPACE\tSTATUS\tERROR")
		for _, r := range results {
			status := "OK"
			errMsg := "-"
			if !r.Success {
				status = "FAILED"
				errMsg = r.Error
			}
			flowID := r.FlowID
			if flowID == "" {
				flowID = "-"
			}
			namespace := r.Namespace
			if namespace == "" {
				namespace = "-"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", r.FilePath, flowID, namespace, status, errMsg)
		}
		fmt.Fprintf(w, "\n%d flow(s) deployed successfully, %d failed\n", successCount, failCount)
		return nil
	}); err != nil {
		return err
	}

	if failCount > 0 {
		return fmt.Errorf("deployment completed with %d error(s)", failCount)
	}
	return nil
}

// parseFlowYAML extracts namespace and flow ID from YAML content.
func parseFlowYAML(content string) (string, string, error) {
	var payload map[string]any
	if err := yaml.Unmarshal([]byte(content), &payload); err != nil {
		return "", "", fmt.Errorf("invalid YAML content: %w", err)
	}

	rawNamespace, ok := payload["namespace"]
	if !ok {
		return "", "", errors.New("flow YAML must contain a 'namespace' field")
	}
	namespace, ok := rawNamespace.(string)
	if !ok || strings.TrimSpace(namespace) == "" {
		return "", "", errors.New("flow YAML must contain a valid 'namespace' field")
	}

	rawID, ok := payload["id"]
	if !ok {
		return "", "", errors.New("flow YAML must contain an 'id' field")
	}
	flowID, ok := rawID.(string)
	if !ok || strings.TrimSpace(flowID) == "" {
		return "", "", errors.New("flow YAML must contain a valid 'id' field")
	}

	return namespace, flowID, nil
}

// replaceNamespaceInYAML modifies the namespace field in YAML content.
func replaceNamespaceInYAML(content string, newNamespace string) (string, error) {
	var payload map[string]any
	if err := yaml.Unmarshal([]byte(content), &payload); err != nil {
		return "", fmt.Errorf("invalid YAML content: %w", err)
	}

	payload["namespace"] = newNamespace

	modified, err := yaml.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal modified YAML: %w", err)
	}

	return string(modified), nil
}
