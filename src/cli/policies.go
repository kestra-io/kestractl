package cli

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	kestra "github.com/kestra-io/client-sdk/go-sdk/v2/kestra_api_client"
	"github.com/spf13/cobra"
)

func newPoliciesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "policies",
		Short: "Manage governance policies (list, get, create, update, delete, validate, export)",
		Long: `Manage governance policies in the active tenant.

Policies are created from YAML files, like flows. Managing policies requires
Kestra Enterprise Edition.`,
	}

	cmd.AddCommand(newPoliciesListCommand())
	cmd.AddCommand(newPoliciesGetCommand())
	cmd.AddCommand(newPoliciesCreateCommand())
	cmd.AddCommand(newPoliciesUpdateCommand())
	cmd.AddCommand(newPoliciesDeleteCommand())
	cmd.AddCommand(newPoliciesValidateCommand())
	cmd.AddCommand(newPoliciesExportCommand())

	return cmd
}

func newPoliciesListCommand() *cobra.Command {
	var (
		page int
		size int
		sort []string
	)

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List policies.",
		Aliases: []string{"ls"},
		Example: `  kestractl policies list
  kestractl policies list --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runPoliciesList(client, page, size, sort, renderer)
		},
	}

	cmd.Flags().IntVar(&page, "page", 1, "Page number")
	cmd.Flags().IntVar(&size, "size", 100, "Page size")
	cmd.Flags().StringArrayVar(&sort, "sort", nil, "Sort expression (e.g. 'id:asc', repeatable)")

	return cmd
}

func runPoliciesList(client *Client, page, size int, sort []string, renderer *Renderer) error {
	resp, err := client.Kestra.Policies().SearchTenantPolicies(client.Ctx, client.Tenant, &page, &size, sort, nil)
	if err != nil {
		return formatSDKError(err)
	}

	results := resp.GetResults()
	if results == nil {
		results = []kestra.ApiPolicySummary{}
	}

	return renderer.Render(results, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ID\tNAME\tSCOPE\tENFORCEMENT\tVALIDATE\tMUTATE")
		for _, p := range results {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%d\n",
				p.Id, p.DisplayName, p.Scope, p.Enforcement, p.Rules.Validate, p.Rules.Mutate)
		}
		fmt.Fprintf(w, "\nTotal policies: %d\n", resp.GetTotal())
		return nil
	})
}

func newPoliciesGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "get <policy_id>",
		Short:   "Get policy details.",
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
			return runPoliciesGet(client, args[0], renderer)
		},
	}
}

func runPoliciesGet(client *Client, id string, renderer *Renderer) error {
	p, err := client.Kestra.Policies().TenantPolicy(client.Ctx, client.Tenant, id)
	if err != nil {
		return formatSDKError(err)
	}
	return renderPolicyDetail(p, renderer)
}

func renderPolicyDetail(p *kestra.Policy, renderer *Renderer) error {
	return renderer.Render(p, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "Policy Details")
		fmt.Fprintln(w)
		fmt.Fprintf(w, "ID:\t%s\n", p.GetId())
		fmt.Fprintf(w, "Name:\t%s\n", p.GetDisplayName())
		fmt.Fprintf(w, "Description:\t%s\n", p.Description)
		fmt.Fprintf(w, "Scope:\t%s\n", p.GetScope())
		fmt.Fprintf(w, "Enforcement:\t%s\n", p.GetEnforcement())
		fmt.Fprintf(w, "Rules:\t%d\n", len(p.GetRules()))
		if p.GetSource() != "" {
			fmt.Fprintf(w, "\n%s\n", p.GetSource())
		}
		return nil
	})
}

// addPolicyFileFlag wires the required -f/--file flag shared by create, update and validate.
func addPolicyFileFlag(cmd *cobra.Command, filePath *string) {
	cmd.Flags().StringVarP(filePath, "file", "f", "", "Path to the YAML policy file (required)")
	_ = cmd.MarkFlagRequired("file")
}

func newPoliciesCreateCommand() *cobra.Command {
	var filePath string

	cmd := &cobra.Command{
		Use:     "create",
		Short:   "Create a policy from a YAML file.",
		Example: `  kestractl policies create --file my-policy.yml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runPoliciesCreate(client, filePath, renderer)
		},
	}

	addPolicyFileFlag(cmd, &filePath)
	return cmd
}

func runPoliciesCreate(client *Client, filePath string, renderer *Renderer) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}
	p, err := client.Kestra.Policies().CreateTenantPolicy(client.Ctx, client.Tenant, string(data))
	if err != nil {
		return formatSDKError(err)
	}
	return renderPolicyDetail(p, renderer)
}

func newPoliciesUpdateCommand() *cobra.Command {
	var filePath string

	cmd := &cobra.Command{
		Use:     "update <policy_id>",
		Short:   "Update a policy from a YAML file.",
		Args:    cobra.ExactArgs(1),
		Example: `  kestractl policies update my-policy --file my-policy.yml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runPoliciesUpdate(client, args[0], filePath, renderer)
		},
	}

	addPolicyFileFlag(cmd, &filePath)
	return cmd
}

func runPoliciesUpdate(client *Client, id, filePath string, renderer *Renderer) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}
	p, err := client.Kestra.Policies().UpdateTenantPolicy(client.Ctx, client.Tenant, id, string(data))
	if err != nil {
		return formatSDKError(err)
	}
	return renderPolicyDetail(p, renderer)
}

func newPoliciesDeleteCommand() *cobra.Command {
	var skipConfirm bool

	cmd := &cobra.Command{
		Use:     "delete <policy_id>",
		Short:   "Delete a policy.",
		Long:    "Delete a policy. Prompts for confirmation unless --yes is provided.",
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
			return runPoliciesDelete(client, args[0], skipConfirm, cmd.InOrStdin(), renderer)
		},
	}

	cmd.Flags().BoolVarP(&skipConfirm, "yes", "y", false, "Skip the confirmation prompt")
	return cmd
}

func runPoliciesDelete(client *Client, id string, skipConfirm bool, in io.Reader, renderer *Renderer) error {
	if !skipConfirm {
		confirmed, err := confirm(in, os.Stderr,
			fmt.Sprintf("Are you sure you want to delete policy '%s'? [y/N]: ", id))
		if err != nil {
			return err
		}
		if !confirmed {
			return renderStatus(renderer, "Cancelled.", map[string]any{"id": id, "status": "cancelled"})
		}
	}

	if err := client.Kestra.Policies().DeleteTenantPolicy(client.Ctx, client.Tenant, id); err != nil {
		return formatSDKError(err)
	}
	return renderStatus(renderer, fmt.Sprintf("Policy '%s' deleted.", id),
		map[string]any{"id": id, "status": "deleted"})
}

func newPoliciesValidateCommand() *cobra.Command {
	var filePath string

	cmd := &cobra.Command{
		Use:     "validate",
		Short:   "Validate a policy YAML file.",
		Example: `  kestractl policies validate --file my-policy.yml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runPoliciesValidate(client, filePath, renderer)
		},
	}

	addPolicyFileFlag(cmd, &filePath)
	return cmd
}

func runPoliciesValidate(client *Client, filePath string, renderer *Renderer) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}
	result, err := client.Kestra.Policies().ValidateTenantPolicy(client.Ctx, client.Tenant, string(data))
	if err != nil {
		return formatSDKError(err)
	}
	return renderValidateConstraintViolation(result, renderer)
}

func newPoliciesExportCommand() *cobra.Command {
	var outputFile string

	cmd := &cobra.Command{
		Use:   "export [policy_id...]",
		Short: "Export policies as an archive.",
		Long:  "Export all policies of the active tenant, or only the given IDs, as a ZIP archive.",
		Example: `  kestractl policies export --output-file policies.zip
  kestractl policies export my-policy other-policy --output-file policies.zip`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runPoliciesExport(client, args, outputFile)
		},
	}

	cmd.Flags().StringVar(&outputFile, "output-file", "", "File to write the archive to (required)")
	_ = cmd.MarkFlagRequired("output-file")
	return cmd
}

func runPoliciesExport(client *Client, ids []string, outputFile string) error {
	var (
		data []byte
		err  error
	)
	if len(ids) > 0 {
		data, err = client.Kestra.Policies().ExportTenantPoliciesByIds(client.Ctx, client.Tenant, ids)
	} else {
		data, err = client.Kestra.Policies().ExportTenantPolicies(client.Ctx, client.Tenant)
	}
	if err != nil {
		return formatSDKError(err)
	}
	if err := os.WriteFile(outputFile, data, 0o644); err != nil {
		return fmt.Errorf("failed to write %q: %w", outputFile, err)
	}
	fmt.Fprintf(os.Stderr, "Policies exported to %s (%d bytes)\n", outputFile, len(data))
	return nil
}
