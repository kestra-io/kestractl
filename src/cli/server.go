package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func newServerCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Server information and diagnostics.",
	}

	cmd.AddCommand(newServerLicenseCommand())

	return cmd
}

func newServerLicenseCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "license",
		Short: "Show server license information.",
		Example: `  kestractl server license
  kestractl server license --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runServerLicense(client, renderer)
		},
	}
	return cmd
}

func runServerLicense(client *Client, renderer *Renderer) error {
	info, _, err := client.API.MiscAPI.LicenseInfo(client.Ctx).Execute()
	if err != nil {
		return formatSDKError(err)
	}

	result := map[string]any{
		"type":         info.GetType(),
		"expired":      info.GetExpired(),
		"standalone":   info.GetStandalone(),
		"workerGroups": info.GetWorkerGroups(),
		"maxServers":   info.GetMaxServers(),
	}
	if exp := info.GetExpiry(); !exp.IsZero() {
		result["expiry"] = exp.Format("2006-01-02")
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "TYPE\t%s\n", info.GetType())
		if exp := info.GetExpiry(); !exp.IsZero() {
			fmt.Fprintf(w, "EXPIRY\t%s\n", exp.Format("2006-01-02"))
		}
		fmt.Fprintf(w, "EXPIRED\t%v\n", info.GetExpired())
		fmt.Fprintf(w, "MAX SERVERS\t%d\n", info.GetMaxServers())
		fmt.Fprintf(w, "STANDALONE\t%v\n", info.GetStandalone())
		fmt.Fprintf(w, "WORKER GROUPS\t%v\n", info.GetWorkerGroups())
		return nil
	})
}
