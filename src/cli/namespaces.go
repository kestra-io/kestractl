package cli

import (
	"context"
	"errors"
	"fmt"

	kestra "github.com/kestra-io/client-sdk/go-sdk/kestra_api_client"
	"github.com/spf13/cobra"
)

type namespacesService interface {
	ListNamespaces(ctx context.Context, tenant string, query string, page, size int) ([]any, error)
}

// sdkNamespacesService implements namespacesService using the Kestra SDK
type sdkNamespacesService struct {
	client  *kestra.APIClient
	authCtx context.Context
}

func (s *sdkNamespacesService) ListNamespaces(ctx context.Context, tenant string, query string, page, size int) ([]any, error) {
	req := s.client.NamespacesAPI.SearchNamespaces(s.authCtx, tenant).
		Page(int32(page)).
		Size(int32(size)).
		Existing(true) // Only return existing namespaces

	if query != "" {
		req = req.Q(query)
	}

	resp, _, err := req.Execute()
	if err != nil {
		return nil, formatSDKError(err)
	}

	results := resp.GetResults()
	result := make([]any, len(results))
	for i, ns := range results {
		result[i] = map[string]any{
			"id":      ns.GetId(),
			"deleted": ns.GetDeleted(),
		}
	}

	return result, nil
}

func newNamespacesCommand() *cobra.Command {
	factory := newSDKClientFactory()
	client, authCtx, err := factory.createClient()
	if err != nil {
		return newNamespacesCommandWithService(nil)
	}
	service := &sdkNamespacesService{client: client, authCtx: authCtx}
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

			if service == nil {
				return errors.New("namespaces service not configured")
			}

			authCtx := temporaryContext()
			tenant := resolveTenant(authCtx)

			namespaces, err := service.ListNamespaces(context.Background(), tenant, query, 1, 100)
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
