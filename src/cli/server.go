package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	kestra "github.com/kestra-io/client-sdk/go-sdk/kestra_api_client"
	"github.com/spf13/cobra"
)

func newServerCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Server information and diagnostics.",
	}

	cmd.AddCommand(newServerLicenseCommand())
	cmd.AddCommand(newServerActionsCommand())
	cmd.AddCommand(newServerGenerateCommand())

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

func newServerActionsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "actions",
		Short: "List available server actions.",
		Example: `  kestractl server actions
  kestractl server actions --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runServerActions(client, renderer)
		},
	}
	return cmd
}

func runServerActions(client *Client, renderer *Renderer) error {
	sdkActions, _, err := client.API.MiscAPI.ListActions(client.Ctx, client.Tenant).Execute()

	var actions []string
	if err != nil {
		// The SDK models actions as a generated enum whose allowed values do not
		// match what the server returns (e.g. "READ"), so a successful 200
		// response is surfaced as a deserialization error. Recover the raw list
		// from the response body before treating it as a real failure.
		actions = tryParseActionsFromError(err)
		if actions == nil {
			return formatSDKError(err)
		}
	} else {
		actions = make([]string, len(sdkActions))
		for i, a := range sdkActions {
			actions[i] = string(a)
		}
	}

	result := make([]map[string]any, len(actions))
	for i, a := range actions {
		result[i] = map[string]any{"action": a}
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ACTION")
		for _, a := range actions {
			fmt.Fprintln(w, a)
		}
		fmt.Fprintf(w, "\nTotal actions: %d\n", len(actions))
		return nil
	})
}

// tryParseActionsFromError recovers the action list from a deserialization
// error raised when the server returns action strings the SDK's generated enum
// does not recognize. Returns nil if the error is a genuine failure.
func tryParseActionsFromError(err error) []string {
	sdkErr, ok := err.(*kestra.GenericOpenAPIError)
	if !ok {
		return nil
	}
	body := sdkErr.Body()
	if len(body) == 0 {
		return nil
	}
	var actions []string
	if json.Unmarshal(body, &actions) != nil {
		return nil
	}
	return actions
}

func newServerGenerateCommand() *cobra.Command {
	var from string

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate a statistics report from the server.",
		Long: `Generate a statistics report from the server.

This calls the server's report generation endpoint and writes the resulting
report to stdout. Use --from to set the report's start date.`,
		Example: `  kestractl server generate
  kestractl server generate --from 2024-01-01`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runServerGenerate(client, from, cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVar(&from, "from", "", "Start date for the report (e.g. 2024-01-01)")
	return cmd
}

func runServerGenerate(client *Client, from string, out io.Writer) error {
	req := client.API.MiscAPI.Generate(client.Ctx, client.Tenant)
	if from != "" {
		req = req.From(from)
	}

	content, _, err := req.Execute()
	if err != nil {
		return formatSDKError(err)
	}

	fmt.Fprint(out, content)
	return nil
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
