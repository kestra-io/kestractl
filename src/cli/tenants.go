package cli

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	kestra "github.com/kestra-io/client-sdk/go-sdk/v2/kestra_api_client"
	"github.com/spf13/cobra"
)

func newTenantsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tenants",
		Short: "Manage tenants (list, get, create, update, delete)",
		Long: `Manage tenants in your Kestra instance.

Tenants are instance-level resources: managing them requires Kestra
Enterprise Edition and instance-owner permissions.`,
	}

	cmd.AddCommand(newTenantsListCommand())
	cmd.AddCommand(newTenantsGetCommand())
	cmd.AddCommand(newTenantsCreateCommand())
	cmd.AddCommand(newTenantsUpdateCommand())
	cmd.AddCommand(newTenantsDeleteCommand())

	return cmd
}

func newTenantsListCommand() *cobra.Command {
	var (
		page int32
		size int32
		sort []string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tenants.",
		Long:  "List all tenants in your Kestra instance.",
		Example: `  # List all tenants
  kestractl tenants list

  # List tenants as JSON
  kestractl tenants list --output json`,
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
			return runTenantsList(client, page, size, sort, renderer)
		},
	}

	cmd.Flags().Int32Var(&page, "page", 1, "Page number")
	cmd.Flags().Int32Var(&size, "size", 100, "Page size")
	cmd.Flags().StringArrayVar(&sort, "sort", nil, "Sort expression (e.g. 'id:asc', repeatable)")

	return cmd
}

func runTenantsList(client *Client, page, size int32, sort []string, renderer *Renderer) error {
	pageParam, sizeParam := int(page), int(size)
	resp, err := client.Kestra.Tenants().SearchTenants(client.Ctx, &pageParam, &sizeParam, sort, nil)
	if err != nil {
		return formatSDKError(err)
	}
	if resp == nil {
		resp = &kestra.PagedResultsTenant{}
	}

	results := resp.Results
	if results == nil {
		results = []kestra.Tenant{}
	}

	return renderer.Render(results, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ID\tNAME\tCONCURRENCY LIMIT\tQUOTAS")
		for _, tenant := range results {
			concurrencyLimit := ""
			if c, ok := tenant.GetConcurrencyOk(); ok {
				concurrencyLimit = fmt.Sprintf("%d (%s)", c.GetLimit(), c.GetBehavior())
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\n",
				tenant.GetId(), tenant.GetName(), concurrencyLimit, len(tenant.GetQuotas()))
		}
		fmt.Fprintf(w, "\nTotal tenants: %d\n", resp.Total)
		return nil
	})
}

func newTenantsGetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <tenant_id>",
		Short: "Get a tenant by ID.",
		Example: `  kestractl tenants get my-tenant
  kestractl tenants get my-tenant --output json`,
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
			return runTenantsGet(client, args[0], renderer)
		},
	}
	return cmd
}

func runTenantsGet(client *Client, id string, renderer *Renderer) error {
	tenant, err := client.Kestra.Tenants().Tenant(client.Ctx, id)
	if err != nil {
		return formatSDKError(err)
	}
	if tenant == nil {
		tenant = kestra.NewTenantWithDefaults()
	}

	return renderTenant(renderer, tenant, "")
}

func newTenantsCreateCommand() *cobra.Command {
	var (
		name                string
		concurrencyLimit    int32
		concurrencyBehavior string
		quotaSpecs          []string
	)

	cmd := &cobra.Command{
		Use:   "create <tenant_id>",
		Short: "Create a tenant.",
		Example: `  kestractl tenants create my-tenant
  kestractl tenants create my-tenant --name "My Tenant"
  kestractl tenants create my-tenant --concurrency-limit 10 --concurrency-behavior QUEUE
  kestractl tenants create my-tenant --quota duration=PT1H,limit=100,behavior=FAIL
  kestractl tenants create my-tenant --output json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
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
			return runTenantsCreate(client, args[0], name, concurrency, quotas, renderer)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Tenant display name (defaults to the tenant ID)")
	addConcurrencyFlags(cmd, &concurrencyLimit, &concurrencyBehavior)
	addQuotaFlag(cmd, &quotaSpecs, "Execution quota as duration=<ISO-8601>,limit=<n>,behavior=<FAIL|CANCEL> (repeatable)")

	return cmd
}

func runTenantsCreate(client *Client, id, name string, concurrency *kestra.Concurrency, quotas []kestra.Quota, renderer *Renderer) error {
	if concurrency != nil || quotas != nil {
		if err := requireKestra2(client, "setting concurrency limits and quotas"); err != nil {
			return err
		}
	}
	if name == "" {
		name = id
	}

	tenant := kestra.NewTenant(id, name, false)
	if concurrency != nil {
		tenant.SetConcurrency(*concurrency)
	}
	if quotas != nil {
		tenant.SetQuotas(quotas)
	}

	created, err := client.Kestra.Tenants().CreateTenant(client.Ctx, *tenant)
	if err != nil {
		return formatSDKError(err)
	}
	if created == nil {
		created = tenant
	}

	return renderTenant(renderer, created, "Tenant created.")
}

func newTenantsUpdateCommand() *cobra.Command {
	var (
		name                string
		concurrencyLimit    int32
		concurrencyBehavior string
		quotaSpecs          []string
	)

	cmd := &cobra.Command{
		Use:   "update <tenant_id>",
		Short: "Update a tenant.",
		Long: `Update a tenant.

Fields not passed on the command line keep their current value: the
existing tenant is fetched first, and only the flags provided here are
applied on top before saving.

--quota sets the full list of execution quotas, replacing any quotas
previously set on the tenant.`,
		Example: `  kestractl tenants update my-tenant --name "Renamed Tenant"
  kestractl tenants update my-tenant --concurrency-limit 20 --concurrency-behavior FAIL
  kestractl tenants update my-tenant --quota duration=PT1H,limit=100,behavior=FAIL --quota duration=P1D,limit=1000,behavior=CANCEL
  kestractl tenants update my-tenant --output json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
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
			return runTenantsUpdate(client, args[0], name, cmd.Flags().Changed("name"), concurrency, quotas, renderer)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "New tenant display name")
	addConcurrencyFlags(cmd, &concurrencyLimit, &concurrencyBehavior)
	addQuotaFlag(cmd, &quotaSpecs, "Execution quota as duration=<ISO-8601>,limit=<n>,behavior=<FAIL|CANCEL> (repeatable); replaces existing quotas")

	return cmd
}

func runTenantsUpdate(client *Client, id, name string, nameSet bool, concurrency *kestra.Concurrency, quotas []kestra.Quota, renderer *Renderer) error {
	if concurrency != nil || quotas != nil {
		if err := requireKestra2(client, "setting concurrency limits and quotas"); err != nil {
			return err
		}
	}
	// UpdateTenant is a full-replace PUT: start from the current tenant so
	// fields not passed on this invocation aren't wiped.
	tenant, err := client.Kestra.Tenants().Tenant(client.Ctx, id)
	if err != nil {
		return formatSDKError(err)
	}
	if tenant == nil {
		tenant = kestra.NewTenant(id, id, false)
	}

	if nameSet {
		tenant.Name = name
	}
	if concurrency != nil {
		tenant.SetConcurrency(*concurrency)
	}
	if quotas != nil {
		tenant.SetQuotas(quotas)
	}

	updated, err := client.Kestra.Tenants().UpdateTenant(client.Ctx, id, *tenant)
	if err != nil {
		return formatSDKError(err)
	}
	if updated == nil {
		updated = tenant
	}

	return renderTenant(renderer, updated, "Tenant updated.")
}

func newTenantsDeleteCommand() *cobra.Command {
	var skipConfirm bool

	cmd := &cobra.Command{
		Use:     "delete <tenant_id>",
		Short:   "Delete a tenant.",
		Long:    "Delete a tenant and all resources linked to it (flows, namespaces, apps, ...). Prompts for confirmation unless --yes is provided.",
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
			return runTenantsDelete(client, args[0], skipConfirm, cmd.InOrStdin(), renderer)
		},
	}

	cmd.Flags().BoolVarP(&skipConfirm, "yes", "y", false, "Skip the confirmation prompt")

	return cmd
}

func runTenantsDelete(client *Client, id string, skipConfirm bool, in io.Reader, renderer *Renderer) error {
	if !skipConfirm {
		// Prompt on stderr so it never pollutes stdout (e.g. --output json).
		confirmed, err := confirm(in, os.Stderr,
			fmt.Sprintf("Are you sure you want to delete tenant '%s' and all resources linked to it? [y/N]: ", id))
		if err != nil {
			return err
		}
		if !confirmed {
			return renderStatus(renderer, "Cancelled.", map[string]any{"id": id, "status": "cancelled"})
		}
	}

	if err := client.Kestra.Tenants().DeleteTenant(client.Ctx, id); err != nil {
		return formatSDKError(err)
	}

	return renderStatus(renderer, fmt.Sprintf("Tenant '%s' deleted.", id),
		map[string]any{"id": id, "status": "deleted"})
}

func renderTenant(renderer *Renderer, tenant *kestra.Tenant, footer string) error {
	result := map[string]any{
		"id":      tenant.GetId(),
		"name":    tenant.GetName(),
		"deleted": tenant.GetDeleted(),
	}
	if c, ok := tenant.GetConcurrencyOk(); ok {
		result["concurrency"] = c
	}
	if quotas := tenant.GetQuotas(); len(quotas) > 0 {
		result["quotas"] = quotas
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "ID\t%s\n", tenant.GetId())
		fmt.Fprintf(w, "NAME\t%s\n", tenant.GetName())
		fmt.Fprintf(w, "DELETED\t%v\n", tenant.GetDeleted())
		writeConcurrencyAndQuotas(w, tenant.Concurrency, tenant.GetQuotas())
		if footer != "" {
			fmt.Fprintf(w, "\n%s\n", footer)
		}
		return nil
	})
}

func writeConcurrencyAndQuotas(w *tabwriter.Writer, concurrency *kestra.Concurrency, quotas []kestra.Quota) {
	if concurrency != nil {
		fmt.Fprintf(w, "CONCURRENCY\tlimit=%d behavior=%s\n", concurrency.GetLimit(), concurrency.GetBehavior())
	}
	if len(quotas) > 0 {
		fmt.Fprintln(w, "\nQUOTAS:")
		fmt.Fprintln(w, "  DURATION\tLIMIT\tBEHAVIOR")
		for _, q := range quotas {
			fmt.Fprintf(w, "  %s\t%d\t%s\n", q.GetDuration(), q.GetLimit(), q.GetBehavior())
		}
	}
}

func addConcurrencyFlags(cmd *cobra.Command, limit *int32, behavior *string) {
	cmd.Flags().Int32Var(limit, "concurrency-limit", 0, "Maximum number of concurrent executions")
	cmd.Flags().StringVar(behavior, "concurrency-behavior", "QUEUE", "Behavior when the concurrency limit is reached (QUEUE, CANCEL or FAIL)")
}

func addQuotaFlag(cmd *cobra.Command, specs *[]string, usage string) {
	cmd.Flags().StringArrayVar(specs, "quota", nil, usage)
}

// parseConcurrencyFlags builds a concurrency setting from the
// --concurrency-limit and --concurrency-behavior flags. Returns nil if
// neither flag was provided.
func parseConcurrencyFlags(limit int32, limitSet bool, behavior string, behaviorSet bool) (*kestra.Concurrency, error) {
	if !limitSet && !behaviorSet {
		return nil, nil
	}
	if !limitSet {
		return nil, fmt.Errorf("--concurrency-behavior requires --concurrency-limit")
	}

	parsed, err := kestra.NewConcurrencyBehaviorFromValue(strings.ToUpper(behavior))
	if err != nil {
		return nil, fmt.Errorf("invalid --concurrency-behavior %q: expected QUEUE, CANCEL or FAIL", behavior)
	}

	return kestra.NewConcurrency(limit, *parsed), nil
}

// parseQuotaFlags builds a quota list from repeatable --quota specs of the
// form "duration=PT1H,limit=100,behavior=FAIL". Returns nil if no spec was
// provided.
func parseQuotaFlags(specs []string) ([]kestra.Quota, error) {
	if len(specs) == 0 {
		return nil, nil
	}

	quotas := make([]kestra.Quota, 0, len(specs))
	for _, spec := range specs {
		var duration, limitValue, behaviorValue string
		for _, part := range strings.Split(spec, ",") {
			key, value, ok := strings.Cut(part, "=")
			if !ok || key == "" || value == "" {
				return nil, fmt.Errorf("invalid --quota %q: expected format duration=<ISO-8601>,limit=<n>,behavior=<FAIL|CANCEL>", spec)
			}
			switch key {
			case "duration":
				duration = value
			case "limit":
				limitValue = value
			case "behavior":
				behaviorValue = value
			default:
				return nil, fmt.Errorf("invalid --quota %q: unknown key %q (expected duration, limit or behavior)", spec, key)
			}
		}
		if duration == "" || limitValue == "" || behaviorValue == "" {
			return nil, fmt.Errorf("invalid --quota %q: duration, limit and behavior are all required", spec)
		}

		limit, err := strconv.ParseInt(limitValue, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid --quota %q: limit must be an integer", spec)
		}

		behavior, err := kestra.NewQuotaBehaviorFromValue(strings.ToUpper(behaviorValue))
		if err != nil {
			return nil, fmt.Errorf("invalid --quota %q: behavior must be FAIL or CANCEL", spec)
		}

		quotas = append(quotas, *kestra.NewQuota(duration, limit, *behavior))
	}

	return quotas, nil
}
