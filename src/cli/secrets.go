package cli

import (
	"fmt"
	"text/tabwriter"

	kestra "github.com/kestra-io/client-sdk/go-sdk/kestra_api_client"
	"github.com/spf13/cobra"
)

func newSecretsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secrets",
		Short: "Manage namespace secrets.",
	}

	cmd.AddCommand(newSecretsListCommand())
	cmd.AddCommand(newSecretsSetCommand())
	cmd.AddCommand(newSecretsDeleteCommand())
	cmd.AddCommand(newSecretsPatchCommand())

	return cmd
}

func newSecretsListCommand() *cobra.Command {
	var page, size int32

	cmd := &cobra.Command{
		Use:   "list <namespace>",
		Short: "List secrets in a namespace.",
		Example: `  kestractl secrets list my.namespace
  kestractl secrets list my.namespace --output json`,
		Aliases: []string{"ls"},
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
			return runSecretsList(client, args[0], page, size, renderer)
		},
	}

	cmd.Flags().Int32Var(&page, "page", 1, "Page number")
	cmd.Flags().Int32Var(&size, "size", 50, "Page size")
	return cmd
}

func runSecretsList(client *Client, namespace string, page, size int32, renderer *Renderer) error {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 50
	}

	resp, _, err := client.API.NamespacesAPI.
		ListNamespaceSecrets(client.Ctx, namespace, client.Tenant).
		Page(page).
		Size(size).
		Filters([]kestra.QueryFilter{}).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	secrets := resp.GetResults()
	result := make([]map[string]any, len(secrets))
	for i, s := range secrets {
		result[i] = map[string]any{
			"key":         s.GetKey(),
			"namespace":   s.GetNamespace(),
			"description": s.GetDescription(),
		}
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "KEY\tNAMESPACE\tDESCRIPTION")
		for _, s := range secrets {
			fmt.Fprintf(w, "%s\t%s\t%s\n",
				s.GetKey(),
				s.GetNamespace(),
				s.GetDescription(),
			)
		}
		total := int64(0)
		if t, ok := resp.GetTotalOk(); ok && t != nil {
			total = *t
		}
		fmt.Fprintf(w, "\nShowing %d secret(s) (page %d, total %d)\n", len(secrets), page, total)
		return nil
	})
}

func newSecretsSetCommand() *cobra.Command {
	var description string

	cmd := &cobra.Command{
		Use:   "set <namespace> <key> <value>",
		Short: "Create or update a secret in a namespace.",
		Example: `  kestractl secrets set my.namespace MY_API_KEY "secret-value"
  kestractl secrets set my.namespace MY_API_KEY "secret-value" --description "API key"`,
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runSecretsSet(client, args[0], args[1], args[2], description, renderer)
		},
	}

	cmd.Flags().StringVar(&description, "description", "", "Secret description")
	return cmd
}

func runSecretsSet(client *Client, namespace, key, value, description string, renderer *Renderer) error {
	sv := kestra.NewApiSecretValue(key, value)
	if description != "" {
		sv.SetDescription(description)
	}

	secrets, _, err := client.API.NamespacesAPI.
		PutSecrets(client.Ctx, namespace, client.Tenant).
		ApiSecretValue(*sv).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	result := map[string]any{
		"namespace": namespace,
		"key":       key,
		"count":     len(secrets),
	}
	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Secret '%s' set in namespace '%s'.\n", key, namespace)
		return nil
	})
}

func newSecretsDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete <namespace> <key>",
		Short:   "Delete a secret from a namespace.",
		Aliases: []string{"rm"},
		Example: `  kestractl secrets delete my.namespace MY_API_KEY
  kestractl secrets delete my.namespace MY_API_KEY --output json`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runSecretsDelete(client, args[0], args[1], renderer)
		},
	}
	return cmd
}

func runSecretsDelete(client *Client, namespace, key string, renderer *Renderer) error {
	_, err := client.API.NamespacesAPI.
		DeleteSecret(client.Ctx, namespace, key, client.Tenant).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	result := map[string]any{
		"namespace": namespace,
		"key":       key,
		"deleted":   true,
	}
	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Secret '%s' deleted from namespace '%s'.\n", key, namespace)
		return nil
	})
}

func newSecretsPatchCommand() *cobra.Command {
	var description string

	cmd := &cobra.Command{
		Use:   "patch <namespace> <key>",
		Short: "Update secret metadata (description) in a namespace.",
		Example: `  kestractl secrets patch my.namespace MY_API_KEY --description "Updated description"
  kestractl secrets patch my.namespace MY_API_KEY --output json`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runSecretsPatch(client, args[0], args[1], description, renderer)
		},
	}

	cmd.Flags().StringVar(&description, "description", "", "New description for the secret")
	return cmd
}

func runSecretsPatch(client *Client, namespace, key, description string, renderer *Renderer) error {
	meta := kestra.NewApiSecretMetaEEWithDefaults()
	meta.SetKey(key)
	meta.SetNamespace(namespace)
	if description != "" {
		meta.SetDescription(description)
	}

	secrets, _, err := client.API.NamespacesAPI.
		PatchSecret(client.Ctx, namespace, key, client.Tenant).
		ApiSecretMetaEE(*meta).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	result := map[string]any{
		"namespace": namespace,
		"key":       key,
		"count":     len(secrets),
	}
	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Secret '%s' in namespace '%s' patched.\n", key, namespace)
		return nil
	})
}
