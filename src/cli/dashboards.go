package cli

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	kestra "github.com/kestra-io/client-sdk/go-sdk/kestra_api_client"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newDashboardsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dashboards",
		Short: "Manage Kestra dashboards (list, get, create, update, delete)",
		Long: `Manage Kestra dashboards in your tenant.

Dashboards provide configurable views of execution metrics and flow data. Requires Kestra Enterprise Edition.`,
	}

	cmd.AddCommand(newDashboardsListCommand())
	cmd.AddCommand(newDashboardsGetCommand())
	cmd.AddCommand(newDashboardsCreateCommand())
	cmd.AddCommand(newDashboardsUpdateCommand())
	cmd.AddCommand(newDashboardsDeleteCommand())
	cmd.AddCommand(newDashboardsDefaultsCommand())
	cmd.AddCommand(newDashboardsValidateCommand())
	cmd.AddCommand(newDashboardsValidateChartCommand())
	cmd.AddCommand(newDashboardsPreviewChartCommand())
	cmd.AddCommand(newDashboardsChartDataCommand())
	cmd.AddCommand(newDashboardsExportChartCSVCommand())
	cmd.AddCommand(newDashboardsExportChartDataCSVCommand())

	return cmd
}

func newDashboardsListCommand() *cobra.Command {
	var (
		query string
		page  int32
		size  int32
		sort  []string
	)

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List dashboards.",
		Aliases: []string{"ls"},
		Example: `  kestractl dashboards list
  kestractl dashboards list --query my-dashboard --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runDashboardsList(client, query, page, size, sort, renderer)
		},
	}

	cmd.Flags().StringVarP(&query, "query", "q", "", "Filter by query string")
	cmd.Flags().Int32Var(&page, "page", 1, "Page number")
	cmd.Flags().Int32Var(&size, "size", 100, "Page size")
	cmd.Flags().StringArrayVar(&sort, "sort", nil, "Sort expression (repeatable)")

	return cmd
}

func runDashboardsList(client *Client, query string, page, size int32, sort []string, renderer *Renderer) error {
	pageInt, sizeInt := int(page), int(size)

	var qPtr *string
	if query != "" {
		qPtr = &query
	}

	resp, err := client.Kestra.Dashboards().SearchDashboards(client.Ctx, client.Tenant, &pageInt, &sizeInt, qPtr, sort)
	if err != nil {
		return formatSDKError(err)
	}

	results := resp.GetResults()

	return renderer.Render(results, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ID\tTITLE\tDELETED")
		for _, d := range results {
			fmt.Fprintf(w, "%s\t%s\t%v\n", d.GetId(), d.GetTitle(), d.GetDeleted())
		}
		fmt.Fprintf(w, "\nTotal dashboards: %d\n", resp.GetTotal())
		return nil
	})
}

func newDashboardsGetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get <id>",
		Short:   "Get dashboard details.",
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
			return runDashboardsGet(client, args[0], renderer)
		},
	}
	return cmd
}

func runDashboardsGet(client *Client, id string, renderer *Renderer) error {
	d, err := client.Kestra.Dashboards().Dashboard(client.Ctx, id, client.Tenant)
	if err != nil {
		return formatSDKError(err)
	}

	return renderer.Render(d, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "Dashboard Details")
		fmt.Fprintln(w)
		fmt.Fprintf(w, "ID:\t%s\n", d.GetId())
		fmt.Fprintf(w, "Title:\t%s\n", d.GetTitle())
		if desc := d.GetDescription(); desc != "" {
			fmt.Fprintf(w, "Description:\t%s\n", desc)
		}
		fmt.Fprintf(w, "Deleted:\t%v\n", d.GetDeleted())
		return nil
	})
}

func newDashboardsCreateCommand() *cobra.Command {
	var filePath string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new dashboard from a YAML file.",
		Example: `  kestractl dashboards create --file my-dashboard.yml
  kestractl dashboards create --file my-dashboard.yml --output json`,
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
			return runDashboardsCreate(client, filePath, renderer)
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Path to the YAML dashboard definition file (required)")
	return cmd
}

func runDashboardsCreate(client *Client, filePath string, renderer *Renderer) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	d, err := client.Kestra.Dashboards().CreateDashboard(client.Ctx, client.Tenant, string(data))
	if err != nil {
		return formatSDKError(err)
	}

	return renderer.Render(d, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Dashboard created successfully.\n\nID:\t%s\nTitle:\t%s\n",
			d.GetId(), d.GetTitle())
		return nil
	})
}

func newDashboardsUpdateCommand() *cobra.Command {
	var filePath string

	cmd := &cobra.Command{
		Use:     "update <id>",
		Short:   "Update an existing dashboard from a YAML file.",
		Args:    cobra.ExactArgs(1),
		Example: `  kestractl dashboards update <id> --file my-dashboard.yml`,
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
			return runDashboardsUpdate(client, args[0], filePath, renderer)
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Path to the YAML dashboard definition file (required)")
	return cmd
}

func runDashboardsUpdate(client *Client, id, filePath string, renderer *Renderer) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	d, err := client.Kestra.Dashboards().UpdateDashboard(client.Ctx, id, client.Tenant, string(data))
	if err != nil {
		return formatSDKError(err)
	}

	return renderer.Render(d, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Dashboard updated successfully.\n\nID:\t%s\nTitle:\t%s\n",
			d.GetId(), d.GetTitle())
		return nil
	})
}

func newDashboardsDeleteCommand() *cobra.Command {
	var skipConfirm bool

	cmd := &cobra.Command{
		Use:     "delete <id>",
		Short:   "Delete a dashboard.",
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
			return runDashboardsDelete(client, args[0], skipConfirm, cmd.InOrStdin(), renderer)
		},
	}

	cmd.Flags().BoolVarP(&skipConfirm, "yes", "y", false, "Skip the confirmation prompt")
	return cmd
}

func runDashboardsDelete(client *Client, id string, skipConfirm bool, in io.Reader, renderer *Renderer) error {
	if !skipConfirm {
		confirmed, err := confirm(in, os.Stderr,
			fmt.Sprintf("Are you sure you want to delete dashboard '%s'? [y/N]: ", id))
		if err != nil {
			return err
		}
		if !confirmed {
			return renderStatus(renderer, "Cancelled.", map[string]any{"id": id, "status": "cancelled"})
		}
	}

	if err := client.Kestra.Dashboards().DeleteDashboard(client.Ctx, id, client.Tenant); err != nil {
		return formatSDKError(err)
	}

	return renderStatus(renderer, fmt.Sprintf("Dashboard '%s' deleted.", id),
		map[string]any{"id": id, "status": "deleted"})
}

func newDashboardsDefaultsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "defaults",
		Short: "Show the tenant's default dashboard settings.",
		Long:  "Show which dashboards are configured as defaults for the home, flow overview, and namespace overview views.",
		Example: `  kestractl dashboards defaults
  kestractl dashboards defaults --output json`,
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
			return runDashboardsDefaults(client, renderer)
		},
	}
	return cmd
}

func runDashboardsDefaults(client *Client, renderer *Renderer) error {
	settings, err := client.Kestra.Dashboards().DefaultDashboards(client.Ctx, client.Tenant)
	if err != nil {
		return formatSDKError(err)
	}

	return renderer.Render(settings, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "VIEW\tDASHBOARD")
		fmt.Fprintf(w, "Home\t%s\n", orNone(settings.GetDefaultHomeDashboard()))
		fmt.Fprintf(w, "Flow overview\t%s\n", orNone(settings.GetDefaultFlowOverviewDashboard()))
		fmt.Fprintf(w, "Namespace overview\t%s\n", orNone(settings.GetDefaultNamespaceOverviewDashboard()))
		return nil
	})
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func newDashboardsValidateCommand() *cobra.Command {
	var filePath string

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate a dashboard definition from a YAML file.",
		Example: `  kestractl dashboards validate --file my-dashboard.yml
  kestractl dashboards validate --file my-dashboard.yml --output json`,
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
			return runDashboardsValidate(client, filePath, renderer)
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Path to the YAML dashboard definition file (required)")
	return cmd
}

func runDashboardsValidate(client *Client, filePath string, renderer *Renderer) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	result, err := client.Kestra.Dashboards().ValidateDashboard(client.Ctx, client.Tenant, string(data))
	if err != nil {
		return formatSDKError(err)
	}

	return renderValidateConstraintViolation(result, renderer)
}

func newDashboardsValidateChartCommand() *cobra.Command {
	var filePath string

	cmd := &cobra.Command{
		Use:   "validate-chart",
		Short: "Validate a single chart definition from a YAML file.",
		Example: `  kestractl dashboards validate-chart --file my-chart.yml
  kestractl dashboards validate-chart --file my-chart.yml --output json`,
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
			return runDashboardsValidateChart(client, filePath, renderer)
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Path to the YAML chart definition file (required)")
	return cmd
}

func runDashboardsValidateChart(client *Client, filePath string, renderer *Renderer) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	result, err := client.Kestra.Dashboards().ValidateChart(client.Ctx, client.Tenant, string(data))
	if err != nil {
		return formatSDKError(err)
	}

	return renderValidateConstraintViolation(result, renderer)
}

func newDashboardsPreviewChartCommand() *cobra.Command {
	var filePath string

	cmd := &cobra.Command{
		Use:   "preview-chart",
		Short: "Preview the data for a chart definition without saving it.",
		Long:  "Send a chart definition to the server and return the resulting data, without persisting a dashboard. The file is the same chart definition accepted by 'validate-chart'.",
		Example: `  kestractl dashboards preview-chart --file my-chart.yml
  kestractl dashboards preview-chart --file my-chart.yml --output json`,
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
			return runDashboardsPreviewChart(client, filePath, renderer)
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Path to the YAML chart definition file (required)")
	return cmd
}

// readChartPreviewRequest reads a chart definition file and wraps it into the
// preview-request body the server expects. The "chart" field is a string
// holding the raw chart definition (the same format 'validate-chart' accepts),
// not a nested object.
func readChartPreviewRequest(filePath string) (map[string]interface{}, error) {
	chart, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	return map[string]interface{}{"chart": string(chart)}, nil
}

func runDashboardsPreviewChart(client *Client, filePath string, renderer *Renderer) error {
	request, err := readChartPreviewRequest(filePath)
	if err != nil {
		return err
	}

	resp, err := client.Kestra.Dashboards().PreviewChart(client.Ctx, client.Tenant, request)
	if err != nil {
		return formatSDKError(err)
	}

	return renderChartData(resp, renderer)
}

func newDashboardsChartDataCommand() *cobra.Command {
	var filePath string

	cmd := &cobra.Command{
		Use:   "chart-data <dashboard-id> <chart-id>",
		Short: "Fetch the data for a chart of an existing dashboard.",
		Args:  cobra.ExactArgs(2),
		Example: `  kestractl dashboards chart-data my-dashboard my-chart
  kestractl dashboards chart-data my-dashboard my-chart --file filters.yml --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runDashboardsChartData(client, args[0], args[1], filePath, renderer)
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Path to a YAML file with chart filters (optional)")
	return cmd
}

func runDashboardsChartData(client *Client, dashboardID, chartID, filePath string, renderer *Renderer) error {
	// The endpoint requires a (possibly empty) filters body; default to an empty
	// object so --file stays optional.
	filters := map[string]interface{}{}
	if filePath != "" {
		body, err := readYAMLBody(filePath)
		if err != nil {
			return err
		}
		filters = body
	}

	resp, err := client.Kestra.Dashboards().DashboardChartData(client.Ctx, dashboardID, chartID, client.Tenant, filters)
	if err != nil {
		return formatSDKError(err)
	}

	return renderChartData(resp, renderer)
}

func newDashboardsExportChartCSVCommand() *cobra.Command {
	var (
		filePath   string
		outputFile string
	)

	cmd := &cobra.Command{
		Use:   "export-chart-csv",
		Short: "Export a chart definition's data as CSV.",
		Long:  "Send a chart definition to the server and export the resulting data as CSV. The file is the same chart definition accepted by 'validate-chart'. The output is written to stdout or --output-file.",
		Example: `  kestractl dashboards export-chart-csv --file my-chart.yml
  kestractl dashboards export-chart-csv --file my-chart.yml --output-file chart.csv`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if filePath == "" {
				return fmt.Errorf("--file is required")
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runDashboardsExportChartCSV(client, filePath, outputFile, cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Path to the YAML chart definition file (required)")
	cmd.Flags().StringVar(&outputFile, "output-file", "", "Write CSV to this file instead of stdout")
	return cmd
}

func runDashboardsExportChartCSV(client *Client, filePath, outputFile string, out io.Writer) error {
	request, err := readChartPreviewRequest(filePath)
	if err != nil {
		return err
	}

	data, err := client.Kestra.Dashboards().ExportChartToCsv(client.Ctx, client.Tenant, request)
	if err != nil {
		return formatSDKError(err)
	}

	return writeCSVOutput(data, outputFile, out)
}

func newDashboardsExportChartDataCSVCommand() *cobra.Command {
	var (
		filePath   string
		outputFile string
	)

	cmd := &cobra.Command{
		Use:   "export-chart-data-csv <dashboard-id> <chart-id>",
		Short: "Export an existing dashboard chart's data as CSV.",
		Long:  "Export the data of a chart belonging to an existing dashboard as CSV. The output is written to stdout or --output-file.",
		Args:  cobra.ExactArgs(2),
		Example: `  kestractl dashboards export-chart-data-csv my-dashboard my-chart
  kestractl dashboards export-chart-data-csv my-dashboard my-chart --output-file chart.csv`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runDashboardsExportChartDataCSV(client, args[0], args[1], filePath, outputFile, cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Path to a YAML file with chart filters (optional)")
	cmd.Flags().StringVar(&outputFile, "output-file", "", "Write CSV to this file instead of stdout")
	return cmd
}

func runDashboardsExportChartDataCSV(client *Client, dashboardID, chartID, filePath, outputFile string, out io.Writer) error {
	// The endpoint requires a (possibly empty) global filter body; default to an
	// empty object so --file stays optional.
	filters := map[string]interface{}{}
	if filePath != "" {
		body, err := readYAMLBody(filePath)
		if err != nil {
			return err
		}
		filters = body
	}

	data, err := client.Kestra.Dashboards().ExportDashboardChartDataToCSV(client.Ctx, dashboardID, chartID, client.Tenant, filters)
	if err != nil {
		return formatSDKError(err)
	}

	return writeCSVOutput(data, outputFile, out)
}

// readYAMLBody reads a YAML file and unmarshals it into a generic map for use
// as a JSON request body.
func readYAMLBody(filePath string) (map[string]interface{}, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	var body map[string]interface{}
	if unmarshalErr := yaml.Unmarshal(data, &body); unmarshalErr != nil {
		return nil, fmt.Errorf("invalid YAML: %w", unmarshalErr)
	}
	return body, nil
}

// renderChartData renders a paged chart-data result: a count summary in table
// mode (the raw rows are only meaningful as JSON).
func renderChartData(resp *kestra.PagedResultsMapStringObject, renderer *Renderer) error {
	results := resp.GetResults()
	return renderer.Render(results, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "KEY\tVALUE")
		fmt.Fprintf(w, "Rows\t%d\n", len(results))
		fmt.Fprintf(w, "Total\t%d\n", resp.GetTotal())
		fmt.Fprintln(w, "\nUse --output json to see the full data.")
		return nil
	})
}

func writeCSVOutput(data []byte, outputFile string, out io.Writer) error {
	if outputFile != "" {
		if err := os.WriteFile(outputFile, data, 0o644); err != nil {
			return fmt.Errorf("failed to write %q: %w", outputFile, err)
		}
		fmt.Fprintf(out, "Chart data exported to %s (%d bytes)\n", outputFile, len(data))
		return nil
	}
	_, err := out.Write(data)
	return err
}
