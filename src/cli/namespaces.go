package cli

import (
	"errors"
	"fmt"
	"strings"

	apiclient "github.com/kestra-io/kestra-cli/src/api_client"
	"github.com/spf13/cobra"
)

func newNamespacesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "namespaces",
		Short: "Manage namespaces",
	}

	cmd.AddCommand(newNamespacesListCommand())

	return cmd
}

func newNamespacesListCommand() *cobra.Command {
	var query string
	var tenant string
	var host string
	var token string
	var output string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List namespaces.",
		RunE: func(cmd *cobra.Command, args []string) error {
			output = strings.ToLower(output)
			if output != "table" && output != "json" {
				return errors.New("output must be 'table' or 'json'")
			}

			client := newKestraClient()
			context := temporaryContext(host, tenant, token)
			api := apiclient.NewNamespacesAPI(client)

			namespaces, err := api.ListNamespaces(tenant, context, query, 1, 100)
			if err != nil {
				return err
			}

			if output == "json" {
				return printJSON(namespaces)
			}

			w := tabWriter()
			fmt.Fprintln(w, "ID\tDeleted")
			for _, item := range namespaces {
				switch v := item.(type) {
				case string:
					fmt.Fprintf(w, "%s\tfalse\n", v)
				case map[string]any:
					id := stringify(v["id"])
					deleted := "false"
					if del, ok := v["deleted"].(bool); ok && del {
						deleted = "true"
					}
					fmt.Fprintf(w, "%s\t%s\n", id, deleted)
				default:
					fmt.Fprintf(w, "%v\tfalse\n", v)
				}
			}
			w.Flush()
			fmt.Printf("\nTotal namespaces: %d\n", len(namespaces))

			return nil
		},
	}

	cmd.Flags().StringVarP(&query, "query", "q", "", "Filter namespaces by search query")
	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant name")
	cmd.Flags().StringVar(&host, "host", "", "Kestra host URL")
	cmd.Flags().StringVarP(&token, "token", "t", "", "API token")
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format (table or json)")

	return cmd
}
