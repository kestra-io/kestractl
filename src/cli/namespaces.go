package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	kestra "github.com/kestra-io/client-sdk/go-sdk/kestra_api_client"
	"github.com/spf13/cobra"
)

func newNamespacesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "namespaces",
		Short: "Manage namespaces (create, list, delete)",
	}

	cmd.AddCommand(newNamespacesListCommand())
	cmd.AddCommand(newNamespacesGetCommand())
	cmd.AddCommand(newNamespacesCreateCommand())
	cmd.AddCommand(newNamespacesDeleteCommand())
	cmd.AddCommand(newNamespacesUpdateCommand())
	cmd.AddCommand(newNamespacesInheritedSecretsCommand())
	cmd.AddCommand(newNamespacesInheritedVariablesCommand())
	cmd.AddCommand(newNamespacesSearchCommand())
	cmd.AddCommand(newNamespacesInheritedPluginDefaultsCommand())
	cmd.AddCommand(newNamespacesAutocompleteCommand())
	cmd.AddCommand(newNamespacesExportPluginDefaultsCommand())
	cmd.AddCommand(newNamespacesImportPluginDefaultsCommand())

	return cmd
}

func newNamespacesListCommand() *cobra.Command {
	var query string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List namespaces.",
		Long: `List all namespaces in your Kestra instance.

Optionally filter results using the --query flag to search for specific namespaces.`,
		Example: `  # List all namespaces
	  kestractl namespaces list

	  # Filter namespaces with a search query
	  kestractl namespaces list --query my.namespace

	  # List namespaces as JSON
	  kestractl namespaces list --output json`,
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

			return runNamespacesList(client, query, renderer)
		},
	}

	cmd.Flags().StringVarP(&query, "query", "q", "", "Filter namespaces by search query")

	return cmd
}

func runNamespacesList(client *Client, query string, renderer *Renderer) error {
	req := client.API.NamespacesAPI.SearchNamespaces(client.Ctx, client.Tenant).
		Page(1).
		Size(100).
		Existing(true)

	if query != "" {
		req = req.Q(query)
	}

	resp, _, err := req.Execute()
	if err != nil {
		fallback := tryParseNamespaceListFromError(err)
		if fallback == nil {
			return formatSDKError(err)
		}

		jsonResults := make([]map[string]any, len(fallback))
		for i, ns := range fallback {
			jsonResults[i] = map[string]any{
				"id": ns.ID,
			}
		}

		return renderer.Render(jsonResults, func(w *tabwriter.Writer) error {
			fmt.Fprintln(w, "ID")
			for _, ns := range fallback {
				fmt.Fprintf(w, "%s\n", ns.ID)
			}
			fmt.Fprintf(w, "\nTotal namespaces: %d\n", len(fallback))
			return nil
		})
	}

	results := resp.GetResults()

	jsonResults := make([]map[string]any, len(results))
	for i, ns := range results {
		jsonResults[i] = map[string]any{
			"id": ns.GetId(),
		}
	}

	return renderer.Render(jsonResults, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ID")
		for _, ns := range results {
			fmt.Fprintf(w, "%s\n", ns.GetId())
		}
		fmt.Fprintf(w, "\nTotal namespaces: %d\n", len(results))
		return nil
	})
}

func newNamespacesGetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <namespace_id>",
		Short: "Get a namespace by ID.",
		Example: `  kestractl namespaces get my.namespace
  kestractl namespaces get my.namespace --output json`,
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
			return runNamespacesGet(client, args[0], renderer)
		},
	}
	return cmd
}

func runNamespacesGet(client *Client, id string, renderer *Renderer) error {
	ns, _, err := client.API.NamespacesAPI.Namespace(client.Ctx, id, client.Tenant).Execute()
	if err != nil {
		return formatSDKError(err)
	}
	if ns == nil {
		ns = kestra.NewNamespaceWithDefaults()
	}

	result := map[string]any{
		"id":          ns.GetId(),
		"description": ns.GetDescription(),
		"deleted":     ns.GetDeleted(),
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "ID\t%s\n", ns.GetId())
		if desc := ns.GetDescription(); desc != "" {
			fmt.Fprintf(w, "DESCRIPTION\t%s\n", desc)
		}
		fmt.Fprintf(w, "DELETED\t%v\n", ns.GetDeleted())
		return nil
	})
}

func newNamespacesCreateCommand() *cobra.Command {
	var description string

	cmd := &cobra.Command{
		Use:   "create <namespace_id>",
		Short: "Create a namespace.",
		Example: `  kestractl namespaces create my.namespace
  kestractl namespaces create my.namespace --description "My team namespace"
  kestractl namespaces create my.namespace --output json`,
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
			return runNamespacesCreate(client, args[0], description, renderer)
		},
	}

	cmd.Flags().StringVar(&description, "description", "", "Namespace description")
	return cmd
}

func runNamespacesCreate(client *Client, id, description string, renderer *Renderer) error {
	ns := kestra.NewNamespace(id, false)
	if description != "" {
		ns.SetDescription(description)
	}

	created, _, err := client.API.NamespacesAPI.
		CreateNamespace(client.Ctx, client.Tenant).
		Namespace(*ns).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}
	if created == nil {
		created = ns
	}

	result := map[string]any{
		"id":          created.GetId(),
		"description": created.GetDescription(),
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "ID\t%s\n", created.GetId())
		if desc := created.GetDescription(); desc != "" {
			fmt.Fprintf(w, "DESCRIPTION\t%s\n", desc)
		}
		fmt.Fprintln(w, "\nNamespace created.")
		return nil
	})
}

func newNamespacesDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <namespace_id>",
		Short: "Delete a namespace.",
		Example: `  kestractl namespaces delete my.namespace`,
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
			return runNamespacesDelete(client, args[0], renderer)
		},
	}
	return cmd
}

func runNamespacesDelete(client *Client, id string, renderer *Renderer) error {
	_, err := client.API.NamespacesAPI.DeleteNamespace(client.Ctx, id, client.Tenant).Execute()
	if err != nil {
		return formatSDKError(err)
	}

	result := map[string]any{"id": id, "deleted": true}
	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Namespace '%s' deleted.\n", id)
		return nil
	})
}

func newNamespacesUpdateCommand() *cobra.Command {
	var description string

	cmd := &cobra.Command{
		Use:   "update <namespace_id>",
		Short: "Update a namespace.",
		Example: `  kestractl namespaces update my.namespace --description "Updated description"
  kestractl namespaces update my.namespace --output json`,
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
			return runNamespacesUpdate(client, args[0], description, renderer)
		},
	}

	cmd.Flags().StringVar(&description, "description", "", "New namespace description")
	return cmd
}

func runNamespacesUpdate(client *Client, id, description string, renderer *Renderer) error {
	ns := kestra.NewNamespace(id, false)
	if description != "" {
		ns.SetDescription(description)
	}

	updated, _, err := client.API.NamespacesAPI.
		UpdateNamespace(client.Ctx, id, client.Tenant).
		Namespace(*ns).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}
	if updated == nil {
		updated = ns
	}

	result := map[string]any{
		"id":          updated.GetId(),
		"description": updated.GetDescription(),
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "ID\t%s\n", updated.GetId())
		if desc := updated.GetDescription(); desc != "" {
			fmt.Fprintf(w, "DESCRIPTION\t%s\n", desc)
		}
		fmt.Fprintln(w, "\nNamespace updated.")
		return nil
	})
}

func newNamespacesInheritedSecretsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inherited-secrets <namespace_id>",
		Short: "List inherited secret keys for a namespace.",
		Example: `  kestractl namespaces inherited-secrets my.namespace
  kestractl namespaces inherited-secrets my.namespace --output json`,
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
			return runNamespacesInheritedSecrets(client, args[0], renderer)
		},
	}
	return cmd
}

func runNamespacesInheritedSecrets(client *Client, namespace string, renderer *Renderer) error {
	resp, _, err := client.API.NamespacesAPI.InheritedSecrets(client.Ctx, namespace, client.Tenant).Execute()
	if err != nil {
		return formatSDKError(err)
	}

	type row struct {
		Namespace string `json:"namespace"`
		Key       string `json:"key"`
	}

	var rows []row
	if resp != nil {
		for ns, keys := range *resp {
			for _, k := range keys {
				rows = append(rows, row{Namespace: ns, Key: k})
			}
		}
	}

	jsonRows := make([]map[string]any, len(rows))
	for i, r := range rows {
		jsonRows[i] = map[string]any{"namespace": r.Namespace, "key": r.Key}
	}

	return renderer.Render(jsonRows, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "NAMESPACE\tKEY")
		for _, r := range rows {
			fmt.Fprintf(w, "%s\t%s\n", r.Namespace, r.Key)
		}
		fmt.Fprintf(w, "\nTotal secret keys: %d\n", len(rows))
		return nil
	})
}

func newNamespacesInheritedVariablesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inherited-variables <namespace_id>",
		Short: "List inherited variables for a namespace.",
		Example: `  kestractl namespaces inherited-variables my.namespace
  kestractl namespaces inherited-variables my.namespace --output json`,
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
			return runNamespacesInheritedVariables(client, args[0], renderer)
		},
	}
	return cmd
}

func runNamespacesInheritedVariables(client *Client, id string, renderer *Renderer) error {
	resp, _, err := client.API.NamespacesAPI.InheritedVariables(client.Ctx, id, client.Tenant).Execute()
	if err != nil {
		return formatSDKError(err)
	}

	type row struct {
		Namespace string `json:"namespace"`
		Key       string `json:"key"`
		Value     string `json:"value"`
	}

	var rows []row
	for ns, vars := range resp {
		for k, v := range vars {
			rows = append(rows, row{Namespace: ns, Key: k, Value: fmt.Sprintf("%v", v)})
		}
	}

	jsonRows := make([]map[string]any, len(rows))
	for i, r := range rows {
		jsonRows[i] = map[string]any{"namespace": r.Namespace, "key": r.Key, "value": r.Value}
	}

	return renderer.Render(jsonRows, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "NAMESPACE\tKEY\tVALUE")
		for _, r := range rows {
			fmt.Fprintf(w, "%s\t%s\t%s\n", r.Namespace, r.Key, r.Value)
		}
		fmt.Fprintf(w, "\nTotal variables: %d\n", len(rows))
		return nil
	})
}

func newNamespacesInheritedPluginDefaultsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin-defaults <namespace>",
		Short: "List inherited plugin defaults for a namespace.",
		Example: `  kestractl namespaces plugin-defaults my.namespace
  kestractl namespaces plugin-defaults my.namespace --output json`,
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
			return runNamespacesInheritedPluginDefaults(client, args[0], renderer)
		},
	}
	return cmd
}

func runNamespacesInheritedPluginDefaults(client *Client, namespace string, renderer *Renderer) error {
	defaults, _, err := client.API.NamespacesAPI.
		InheritedPluginDefaults(client.Ctx, namespace, client.Tenant).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	result := make([]map[string]any, len(defaults))
	for i, d := range defaults {
		result[i] = map[string]any{
			"type":   d.GetType(),
			"forced": d.GetForced(),
		}
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "TYPE\tFORCED")
		for _, d := range defaults {
			fmt.Fprintf(w, "%s\t%v\n", d.GetType(), d.GetForced())
		}
		fmt.Fprintf(w, "\nTotal plugin defaults: %d\n", len(defaults))
		return nil
	})
}

func newNamespacesSearchCommand() *cobra.Command {
	var page, size int32
	var query string
	var existing bool

	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search for namespaces.",
		Example: `  kestractl namespaces search
  kestractl namespaces search --query my.namespace
  kestractl namespaces search --existing --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runNamespacesSearch(client, page, size, query, existing, renderer)
		},
	}

	cmd.Flags().Int32Var(&page, "page", 1, "Page number")
	cmd.Flags().Int32Var(&size, "size", 50, "Page size")
	cmd.Flags().StringVarP(&query, "query", "q", "", "Filter namespaces by query string")
	cmd.Flags().BoolVar(&existing, "existing", false, "Return only existing namespaces")
	return cmd
}

func runNamespacesSearch(client *Client, page, size int32, query string, existing bool, renderer *Renderer) error {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 50
	}

	req := client.API.NamespacesAPI.
		SearchNamespaces(client.Ctx, client.Tenant).
		Page(page).
		Size(size).
		Existing(existing)
	if query != "" {
		req = req.Q(query)
	}

	resp, _, err := req.Execute()
	if err != nil {
		return formatSDKError(err)
	}

	namespaces := resp.GetResults()
	result := make([]map[string]any, len(namespaces))
	for i, ns := range namespaces {
		result[i] = map[string]any{
			"id":          ns.GetId(),
			"description": ns.GetDescription(),
			"deleted":     ns.GetDeleted(),
		}
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ID\tDESCRIPTION\tDELETED")
		for _, ns := range namespaces {
			fmt.Fprintf(w, "%s\t%s\t%v\n",
				ns.GetId(),
				ns.GetDescription(),
				ns.GetDeleted(),
			)
		}
		fmt.Fprintf(w, "\nShowing %d namespace(s) (page %d, total %d)\n", len(namespaces), page, resp.GetTotal())
		return nil
	})
}

func newNamespacesAutocompleteCommand() *cobra.Command {
	var query string
	var existingOnly bool

	cmd := &cobra.Command{
		Use:   "autocomplete",
		Short: "List namespaces for autocomplete.",
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
			return runNamespacesAutocomplete(client, query, existingOnly, renderer)
		},
	}

	cmd.Flags().StringVarP(&query, "query", "q", "", "Autocomplete search string")
	cmd.Flags().BoolVar(&existingOnly, "existing-only", false, "Only return namespaces that exist")

	return cmd
}

func runNamespacesAutocomplete(client *Client, query string, existingOnly bool, renderer *Renderer) error {
	ac := kestra.NewApiAutocomplete()
	if query != "" {
		ac.SetQ(query)
	}
	ac.SetExistingOnly(existingOnly)

	namespaces, _, err := client.API.NamespacesAPI.
		AutocompleteNamespaces(client.Ctx, client.Tenant).
		ApiAutocomplete(*ac).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	result := make([]map[string]any, len(namespaces))
	for i, ns := range namespaces {
		result[i] = map[string]any{"namespace": ns}
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "NAMESPACE")
		for _, ns := range namespaces {
			fmt.Fprintln(w, ns)
		}
		fmt.Fprintf(w, "\nShowing %d namespace(s)\n", len(namespaces))
		return nil
	})
}

func newNamespacesExportPluginDefaultsCommand() *cobra.Command {
	var outputFile string

	cmd := &cobra.Command{
		Use:   "export-plugin-defaults <namespace>",
		Short: "Export plugin defaults for a namespace as YAML.",
		Long: `Download the plugin defaults defined for the given namespace as a YAML file.

The YAML content is written to stdout by default. Use --output-file to save it
to a file instead.`,
		Example: `  # Print plugin defaults to stdout
  kestractl namespaces export-plugin-defaults my.namespace

  # Save to file
  kestractl namespaces export-plugin-defaults my.namespace --output-file defaults.yml`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runNamespacesExportPluginDefaults(client, args[0], outputFile, cmd)
		},
	}

	cmd.Flags().StringVar(&outputFile, "output-file", "", "Write YAML output to this file instead of stdout")
	return cmd
}

func runNamespacesExportPluginDefaults(client *Client, namespace, outputFile string, cmd *cobra.Command) error {
	data, err := client.Kestra.Namespaces().ExportPluginDefaults(client.Ctx, namespace, client.Tenant)
	if err != nil {
		return formatSDKError(err)
	}

	if outputFile != "" {
		if err := os.WriteFile(outputFile, data, 0o644); err != nil {
			return fmt.Errorf("failed to write file: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Plugin defaults exported to %s (%d bytes)\n", outputFile, len(data))
		return nil
	}

	_, err = cmd.OutOrStdout().Write(data)
	return err
}

func newNamespacesImportPluginDefaultsCommand() *cobra.Command {
	var filePath string

	cmd := &cobra.Command{
		Use:   "import-plugin-defaults <namespace>",
		Short: "Import plugin defaults for a namespace from a YAML file.",
		Long: `Upload a YAML file containing plugin defaults for the given namespace.
The file replaces any existing plugin defaults for that namespace.`,
		Example: `  kestractl namespaces import-plugin-defaults my.namespace --file defaults.yml`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if filePath == "" {
				return fmt.Errorf("--file is required")
			}
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runNamespacesImportPluginDefaults(client, args[0], filePath, renderer)
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Path to the YAML plugin defaults file (required)")
	return cmd
}

func runNamespacesImportPluginDefaults(client *Client, namespace, filePath string, renderer *Renderer) error {
	warnings, err := client.Kestra.Namespaces().ImportPluginDefaults(client.Ctx, namespace, client.Tenant, filePath)
	if err != nil {
		return formatSDKError(err)
	}

	result := map[string]any{
		"namespace": namespace,
		"warnings":  warnings,
		"status":    "imported",
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Plugin defaults imported for namespace %q.\n", namespace)
		if len(warnings) > 0 {
			fmt.Fprintln(w, "\nWarnings:")
			for _, warn := range warnings {
				fmt.Fprintf(w, "  - %s\n", warn)
			}
		}
		return nil
	})
}
