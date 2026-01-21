package cli

import (
	"errors"
	"fmt"

	apiclient "github.com/kestra-io/kestra-cli/src/api_client"
	"github.com/spf13/cobra"
)

type namespacesService interface {
	ListNamespaces(tenant string, ctx *apiclient.AuthContext, query string, page, size int) ([]any, error)
}

func newNamespacesCommand() *cobra.Command {
	service := apiclient.NewNamespacesAPI(newKestraClient())
	return newNamespacesCommandWithService(service)
}

func newNamespacesCommandWithService(service namespacesService) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "namespaces",
		Short: "Manage namespaces",
	}

	cmd.AddCommand(newNamespacesListCommand(service))

	return cmd
}

func newNamespacesListCommand(service namespacesService) *cobra.Command {
	var query string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List namespaces.",
		Long: `List all namespaces in your Kestra instance.

Optionally filter results using the --query flag to search for specific namespaces.`,
		Example: `  # List all namespaces
  kestra namespaces list

  # Filter namespaces with a search query
  kestra namespaces list --query my.namespace

  # List namespaces as JSON
  kestra namespaces list --output json`,
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutputFormat(); err != nil {
				return err
			}

			context := temporaryContext()
			if service == nil {
				return errors.New("namespaces service not configured")
			}

			namespaces, err := service.ListNamespaces(globalFlags.Tenant, context, query, 1, 100)
			if err != nil {
				return err
			}

			if globalFlags.Output == "json" {
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

	return cmd
}
