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

func newAppsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apps",
		Short: "Manage Kestra apps (list, get, deploy, delete, enable, disable)",
		Long: `Manage Kestra apps in your tenant.

Apps are low-code interfaces built on top of flows. Requires Kestra Enterprise Edition.`,
	}

	cmd.AddCommand(newAppsListCommand())
	cmd.AddCommand(newAppsGetCommand())
	cmd.AddCommand(newAppsDeployCommand())
	cmd.AddCommand(newAppsUpdateCommand())
	cmd.AddCommand(newAppsDeleteCommand())
	cmd.AddCommand(newAppsEnableCommand())
	cmd.AddCommand(newAppsDisableCommand())
	cmd.AddCommand(newAppsExportCommand())
	cmd.AddCommand(newAppsImportCommand())

	return cmd
}

func newAppsListCommand() *cobra.Command {
	var (
		namespace string
		query     string
		flowID    string
		tags      []string
		page      int32
		size      int32
		sort      []string
	)

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List apps.",
		Aliases: []string{"ls"},
		Example: `  kestractl apps list
  kestractl apps list --namespace my.namespace
  kestractl apps list --query my-app --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runAppsList(client, namespace, query, flowID, tags, page, size, sort, renderer)
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Filter by namespace")
	cmd.Flags().StringVarP(&query, "query", "q", "", "Filter by query string")
	cmd.Flags().StringVar(&flowID, "flow-id", "", "Filter by flow ID")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "Filter by tag (repeatable)")
	cmd.Flags().Int32Var(&page, "page", 1, "Page number")
	cmd.Flags().Int32Var(&size, "size", 100, "Page size")
	cmd.Flags().StringArrayVar(&sort, "sort", nil, "Sort expression (repeatable)")

	return cmd
}

func runAppsList(client *Client, namespace, query, flowID string, tags []string, page, size int32, sort []string, renderer *Renderer) error {
	pageInt, sizeInt := int(page), int(size)

	var nsPtr, qPtr, flowPtr *string
	if namespace != "" {
		nsPtr = &namespace
	}
	if query != "" {
		qPtr = &query
	}
	if flowID != "" {
		flowPtr = &flowID
	}

	resp, err := client.Kestra.Apps().SearchApps(client.Ctx, client.Tenant, &pageInt, &sizeInt, qPtr, nsPtr, flowPtr, sort, tags, nil)
	if err != nil {
		return formatSDKError(err)
	}
	if resp == nil {
		resp = &kestra.PagedResultsAppsControllerApiApp{}
	}

	results := resp.GetResults()
	if results == nil {
		results = []kestra.AppsControllerApiApp{}
	}

	return renderer.Render(results, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "UID\tID\tNAME\tNAMESPACE\tENABLED")
		for _, a := range results {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%v\n",
				a.GetUid(), a.GetId(), a.GetName(), a.GetNamespace(), a.GetEnabled())
		}
		fmt.Fprintf(w, "\nTotal apps: %d\n", resp.GetTotal())
		return nil
	})
}

func newAppsGetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get <uid>",
		Short:   "Get app details.",
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
			return runAppsGet(client, args[0], renderer)
		},
	}
	return cmd
}

func runAppsGet(client *Client, uid string, renderer *Renderer) error {
	app, err := client.Kestra.Apps().App(client.Ctx, uid, client.Tenant)
	if err != nil {
		return formatSDKError(err)
	}

	return renderer.Render(app, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "App Details")
		fmt.Fprintln(w)
		fmt.Fprintf(w, "UID:\t%s\n", app.GetUid())
		fmt.Fprintf(w, "Name:\t%s\n", app.GetName())
		fmt.Fprintf(w, "Namespace:\t%s\n", app.GetNamespace())
		fmt.Fprintf(w, "Disabled:\t%v\n", app.GetDisabled())
		if tags := app.GetTags(); len(tags) > 0 {
			fmt.Fprintf(w, "Tags:\t%s\n", strings.Join(tags, ", "))
		}
		return nil
	})
}

func newAppsDeployCommand() *cobra.Command {
	var filePath string

	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Create a new app from a YAML file.",
		Long:  "Create a new app from a YAML definition file.",
		Example: `  kestractl apps deploy --file my-app.yml
  kestractl apps deploy --file my-app.yml --output json`,
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
			return runAppsDeploy(client, filePath, renderer)
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Path to the YAML app definition file (required)")
	return cmd
}

func runAppsDeploy(client *Client, filePath string, renderer *Renderer) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	app, err := client.Kestra.Apps().CreateApp(client.Ctx, client.Tenant, string(data))
	if err != nil {
		return formatSDKError(err)
	}

	return renderer.Render(app, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "App deployed successfully.\n\nUID:\t%s\nName:\t%s\nNamespace:\t%s\n",
			app.GetUid(), app.GetName(), app.GetNamespace())
		return nil
	})
}

func newAppsUpdateCommand() *cobra.Command {
	var filePath string

	cmd := &cobra.Command{
		Use:   "update <uid>",
		Short: "Update an existing app from a YAML file.",
		Args:  cobra.ExactArgs(1),
		Example: `  kestractl apps update <uid> --file my-app.yml`,
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
			return runAppsUpdate(client, args[0], filePath, renderer)
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Path to the YAML app definition file (required)")
	return cmd
}

func runAppsUpdate(client *Client, uid, filePath string, renderer *Renderer) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	if err := client.Kestra.Apps().UpdateApp(client.Ctx, uid, client.Tenant, string(data)); err != nil {
		return formatSDKError(err)
	}

	return renderStatus(renderer, fmt.Sprintf("App '%s' updated.", uid),
		map[string]any{"uid": uid, "status": "updated"})
}

func newAppsDeleteCommand() *cobra.Command {
	var skipConfirm bool

	cmd := &cobra.Command{
		Use:     "delete <uid>",
		Short:   "Delete an app.",
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
			return runAppsDelete(client, args[0], skipConfirm, cmd.InOrStdin(), renderer)
		},
	}

	cmd.Flags().BoolVarP(&skipConfirm, "yes", "y", false, "Skip the confirmation prompt")
	return cmd
}

func runAppsDelete(client *Client, uid string, skipConfirm bool, in io.Reader, renderer *Renderer) error {
	if !skipConfirm {
		confirmed, err := confirm(in, os.Stderr,
			fmt.Sprintf("Are you sure you want to delete app '%s'? [y/N]: ", uid))
		if err != nil {
			return err
		}
		if !confirmed {
			return renderStatus(renderer, "Cancelled.", map[string]any{"uid": uid, "status": "cancelled"})
		}
	}

	if err := client.Kestra.Apps().DeleteApp(client.Ctx, uid, client.Tenant); err != nil {
		return formatSDKError(err)
	}

	return renderStatus(renderer, fmt.Sprintf("App '%s' deleted.", uid),
		map[string]any{"uid": uid, "status": "deleted"})
}

func newAppsEnableCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enable <uid>",
		Short: "Enable an app.",
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
			return runAppsToggle(client, args[0], true, renderer)
		},
	}
	return cmd
}

func newAppsDisableCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disable <uid>",
		Short: "Disable an app.",
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
			return runAppsToggle(client, args[0], false, renderer)
		},
	}
	return cmd
}

func runAppsToggle(client *Client, uid string, enable bool, renderer *Renderer) error {
	var (
		app *kestra.AppsControllerApiApp
		err error
	)
	if enable {
		app, err = client.Kestra.Apps().EnableApp(client.Ctx, uid, client.Tenant)
	} else {
		app, err = client.Kestra.Apps().DisableApp(client.Ctx, uid, client.Tenant)
	}
	if err != nil {
		return formatSDKError(err)
	}

	action := "disabled"
	if enable {
		action = "enabled"
	}

	if app == nil {
		return renderStatus(renderer, fmt.Sprintf("App '%s' %s.", uid, action),
			map[string]any{"uid": uid, "status": action})
	}

	return renderer.Render(app, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "App '%s' %s.\n", app.GetUid(), action)
		return nil
	})
}

func newAppsExportCommand() *cobra.Command {
	var outputFile string

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export all apps as a ZIP archive.",
		Long:  "Export all apps as a ZIP archive. The file is written to stdout or --output-file.",
		Example: `  kestractl apps export --output-file apps.zip`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runAppsExport(client, outputFile, cmd)
		},
	}

	cmd.Flags().StringVar(&outputFile, "output-file", "", "Write ZIP to this file instead of stdout")
	return cmd
}

func runAppsExport(client *Client, outputFile string, cmd *cobra.Command) error {
	data, err := client.Kestra.Apps().BulkExportApps(client.Ctx, client.Tenant, nil)
	if err != nil {
		return formatSDKError(err)
	}

	if outputFile != "" {
		if err := os.WriteFile(outputFile, data, 0o644); err != nil {
			return fmt.Errorf("failed to write file: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Apps exported to %s (%d bytes)\n", outputFile, len(data))
		return nil
	}

	_, err = cmd.OutOrStdout().Write(data)
	return err
}

func newAppsImportCommand() *cobra.Command {
	var filePath string

	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import apps from a ZIP archive.",
		Long:  "Upload a ZIP archive containing app definitions to import them into the tenant.",
		Example: `  kestractl apps import --file apps.zip
  kestractl apps import --file apps.zip --output json`,
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
			return runAppsImport(client, filePath, renderer)
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Path to the ZIP archive to import (required)")
	return cmd
}

func runAppsImport(client *Client, filePath string, renderer *Renderer) error {
	resp, err := client.Kestra.Apps().BulkImportApps(client.Ctx, client.Tenant, filePath)
	if err != nil {
		return formatSDKError(err)
	}

	imported := 0
	var errors []string
	if resp != nil {
		imported = len(resp.GetSuccess())
		for _, e := range resp.GetErrors() {
			errors = append(errors, fmt.Sprintf("%s: %s", e.GetSource(), e.GetMessage()))
		}
	}

	result := map[string]any{
		"imported": imported,
		"errors":   errors,
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "%d app(s) imported.\n", imported)
		if len(errors) > 0 {
			fmt.Fprintln(w, "\nErrors:")
			for _, e := range errors {
				fmt.Fprintf(w, "  - %s\n", e)
			}
		}
		return nil
	})
}
