package cli

import (
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	kestra "github.com/kestra-io/client-sdk/go-sdk/v2/kestra_api_client"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
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
	cmd.AddCommand(newNamespacesAutocompleteCommand())

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

	variables := namespaceVariables(client, id, ns.GetVariables())

	result := map[string]any{
		"id":          ns.GetId(),
		"description": ns.GetDescription(),
		"deleted":     ns.GetDeleted(),
		"variables":   variables,
	}
	if c, ok := ns.GetConcurrencyOk(); ok {
		result["concurrency"] = c
	}
	if quotas := ns.GetQuotas(); len(quotas) > 0 {
		result["quotas"] = quotas
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "ID\t%s\n", ns.GetId())
		if desc := ns.GetDescription(); desc != "" {
			fmt.Fprintf(w, "DESCRIPTION\t%s\n", desc)
		}
		fmt.Fprintf(w, "DELETED\t%v\n", ns.GetDeleted())
		writeConcurrencyAndQuotas(w, ns.Concurrency, ns.GetQuotas())
		if len(variables) > 0 {
			fmt.Fprintln(w, "\nVARIABLES:")
			keys := make([]string, 0, len(variables))
			for k := range variables {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				// stringify, not %v: a json.Number prints its digits and a
				// nested object prints as JSON rather than Go's map[...] form.
				fmt.Fprintf(w, "  %s\t%s\n", k, stringify(variables[k]))
			}
		}
		return nil
	})
}

// parseVariableFlags builds a namespace variables map from repeatable "key=value"
// pairs and/or a YAML/JSON file. Pairs take precedence over file entries with the
// same key. Returns nil if neither source was provided.
func parseVariableFlags(pairs []string, filePath string) (map[string]interface{}, error) {
	if len(pairs) == 0 && filePath == "" {
		return nil, nil
	}

	variables := map[string]interface{}{}

	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read variables file: %w", err)
		}
		if err := yaml.Unmarshal(data, &variables); err != nil {
			return nil, fmt.Errorf("failed to parse variables file: %w", err)
		}
	}

	for _, p := range pairs {
		key, value, ok := strings.Cut(p, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("invalid --variable %q: expected format key=value", p)
		}
		variables[key] = value
	}

	return variables, nil
}

// namespaceVariables re-reads a namespace straight from the API so its
// variables keep every digit: the SDK types Namespace.Variables as
// map[string]interface{} and decodes it with plain json.Unmarshal, which turns
// any integer above 2^53 into a lossy float64 before kestractl sees it
// (follow-up to #121). The SDK's own map is returned as a fallback, so a failed
// extra read degrades to the old rendering instead of failing the command.
func namespaceVariables(client *Client, id string, fallback map[string]any) map[string]any {
	// No variables at all: the extra read cannot add anything, so the common
	// case stays at a single request.
	if len(fallback) == 0 {
		return fallback
	}

	body, err := rawGet(client, fmt.Sprintf("/api/v1/%s/namespaces/%s",
		url.PathEscape(client.Tenant),
		url.PathEscape(id),
	))
	if err != nil {
		return fallback
	}

	var raw map[string]any
	if decodeJSONPreservingNumbers(body, &raw) != nil {
		return fallback
	}
	variables, ok := raw["variables"].(map[string]any)
	if !ok {
		return fallback
	}
	return variables
}

func newNamespacesCreateCommand() *cobra.Command {
	var description string
	var variablePairs []string
	var variablesFile string
	var concurrencyLimit int32
	var concurrencyBehavior string
	var quotaSpecs []string

	cmd := &cobra.Command{
		Use:   "create <namespace_id>",
		Short: "Create a namespace.",
		Example: `  kestractl namespaces create my.namespace
  kestractl namespaces create my.namespace --description "My team namespace"
  kestractl namespaces create my.namespace --variable env=prod --variable region=eu
  kestractl namespaces create my.namespace --variables-file variables.yml
  kestractl namespaces create my.namespace --concurrency-limit 10 --concurrency-behavior QUEUE
  kestractl namespaces create my.namespace --quota duration=PT1H,limit=100,behavior=FAIL
  kestractl namespaces create my.namespace --output json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			variables, err := parseVariableFlags(variablePairs, variablesFile)
			if err != nil {
				return err
			}
			concurrency, err := parseConcurrencyFlags(
				concurrencyLimit, cmd.Flags().Changed("concurrency-limit"),
				concurrencyBehavior, cmd.Flags().Changed("concurrency-behavior"))
			if err != nil {
				return err
			}
			quotas, err := parseQuotaFlags(quotaSpecs)
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runNamespacesCreate(client, args[0], description, variables, concurrency, quotas, renderer)
		},
	}

	cmd.Flags().StringVar(&description, "description", "", "Namespace description")
	cmd.Flags().StringArrayVar(&variablePairs, "variable", nil, "Namespace variable as key=value (repeatable)")
	cmd.Flags().StringVar(&variablesFile, "variables-file", "", "Path to a YAML or JSON file defining namespace variables")
	addConcurrencyFlags(cmd, &concurrencyLimit, &concurrencyBehavior)
	addQuotaFlag(cmd, &quotaSpecs, "Execution quota as duration=<ISO-8601>,limit=<n>,behavior=<FAIL|CANCEL> (repeatable)")
	return cmd
}

func runNamespacesCreate(client *Client, id, description string, variables map[string]interface{}, concurrency *kestra.Concurrency, quotas []kestra.Quota, renderer *Renderer) error {
	if concurrency != nil || quotas != nil {
		if err := requireKestra2(client, "setting concurrency limits and quotas"); err != nil {
			return err
		}
	}
	ns := kestra.NewNamespace(id, false)
	if description != "" {
		ns.SetDescription(description)
	}
	if len(variables) > 0 {
		ns.SetVariables(variables)
	}
	if concurrency != nil {
		ns.SetConcurrency(*concurrency)
	}
	if quotas != nil {
		ns.SetQuotas(quotas)
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
		"variables":   namespaceVariables(client, id, created.GetVariables()),
	}
	if c, ok := created.GetConcurrencyOk(); ok {
		result["concurrency"] = c
	}
	if quotas := created.GetQuotas(); len(quotas) > 0 {
		result["quotas"] = quotas
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "ID\t%s\n", created.GetId())
		if desc := created.GetDescription(); desc != "" {
			fmt.Fprintf(w, "DESCRIPTION\t%s\n", desc)
		}
		writeConcurrencyAndQuotas(w, created.Concurrency, created.GetQuotas())
		fmt.Fprintln(w, "\nNamespace created.")
		return nil
	})
}

func newNamespacesDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete <namespace_id>",
		Short:   "Delete a namespace.",
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
	var variablePairs []string
	var variablesFile string
	var concurrencyLimit int32
	var concurrencyBehavior string
	var quotaSpecs []string

	cmd := &cobra.Command{
		Use:   "update <namespace_id>",
		Short: "Update a namespace.",
		Long: `Update a namespace.

Fields not passed on the command line keep their current value: the
existing namespace is fetched first, and only the flags provided here are
applied on top before saving.

--variable and --variables-file set the full list of namespace variables,
replacing any variables previously set on the namespace. Combine them to
layer inline overrides on top of a file: --variable entries win on key
conflicts.

--quota sets the full list of execution quotas, replacing any quotas
previously set on the namespace.`,
		Example: `  kestractl namespaces update my.namespace --description "Updated description"
  kestractl namespaces update my.namespace --variable env=prod --variable region=eu
  kestractl namespaces update my.namespace --variables-file variables.yml
  kestractl namespaces update my.namespace --concurrency-limit 20 --concurrency-behavior FAIL
  kestractl namespaces update my.namespace --quota duration=PT1H,limit=100,behavior=FAIL
  kestractl namespaces update my.namespace --output json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			variables, err := parseVariableFlags(variablePairs, variablesFile)
			if err != nil {
				return err
			}
			concurrency, err := parseConcurrencyFlags(
				concurrencyLimit, cmd.Flags().Changed("concurrency-limit"),
				concurrencyBehavior, cmd.Flags().Changed("concurrency-behavior"))
			if err != nil {
				return err
			}
			quotas, err := parseQuotaFlags(quotaSpecs)
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runNamespacesUpdate(client, args[0], description, cmd.Flags().Changed("description"), variables, concurrency, quotas, renderer)
		},
	}

	cmd.Flags().StringVar(&description, "description", "", "New namespace description")
	cmd.Flags().StringArrayVar(&variablePairs, "variable", nil, "Namespace variable as key=value (repeatable); replaces existing variables")
	cmd.Flags().StringVar(&variablesFile, "variables-file", "", "Path to a YAML or JSON file defining namespace variables; replaces existing variables")
	addConcurrencyFlags(cmd, &concurrencyLimit, &concurrencyBehavior)
	addQuotaFlag(cmd, &quotaSpecs, "Execution quota as duration=<ISO-8601>,limit=<n>,behavior=<FAIL|CANCEL> (repeatable); replaces existing quotas")
	return cmd
}

func runNamespacesUpdate(client *Client, id, description string, descriptionSet bool, variables map[string]interface{}, concurrency *kestra.Concurrency, quotas []kestra.Quota, renderer *Renderer) error {
	if concurrency != nil || quotas != nil {
		if err := requireKestra2(client, "setting concurrency limits and quotas"); err != nil {
			return err
		}
	}
	// UpdateNamespace is a full-replace PUT: start from the current namespace
	// so fields not passed on this invocation aren't wiped.
	ns, _, err := client.API.NamespacesAPI.Namespace(client.Ctx, id, client.Tenant).Execute()
	if err != nil {
		return formatSDKError(err)
	}
	if ns == nil {
		ns = kestra.NewNamespace(id, false)
	}

	if descriptionSet {
		ns.SetDescription(description)
	}
	if variables != nil {
		ns.SetVariables(variables)
	}
	if concurrency != nil {
		ns.SetConcurrency(*concurrency)
	}
	if quotas != nil {
		ns.SetQuotas(quotas)
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
		"variables":   namespaceVariables(client, id, updated.GetVariables()),
	}
	if c, ok := updated.GetConcurrencyOk(); ok {
		result["concurrency"] = c
	}
	if quotas := updated.GetQuotas(); len(quotas) > 0 {
		result["quotas"] = quotas
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "ID\t%s\n", updated.GetId())
		if desc := updated.GetDescription(); desc != "" {
			fmt.Fprintf(w, "DESCRIPTION\t%s\n", desc)
		}
		writeConcurrencyAndQuotas(w, updated.Concurrency, updated.GetQuotas())
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
