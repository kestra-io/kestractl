package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	kestra "github.com/kestra-io/client-sdk/go-sdk/kestra_api_client"
	"github.com/spf13/cobra"
)

var kvAllowedTypes = []string{"STRING", "NUMBER", "BOOLEAN", "DATETIME", "DATE", "DURATION", "JSON"}

type kvTypedValue struct {
	Type  string `json:"type"`
	Value any    `json:"value"`
}

func newKVCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "kv",
		Short: "Manage key-value pairs",
	}

	cmd.AddCommand(newKVListCommand())
	cmd.AddCommand(newKVSetCommand())
	cmd.AddCommand(newKVUpdateCommand())
	cmd.AddCommand(newKVGetCommand())
	cmd.AddCommand(newKVDeleteCommand())

	return cmd
}

func newKVListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list [namespace]",
		Short: "List key-value entries.",
		Long:  "List all key-value entries, optionally filtered by namespace.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}

			client, err := NewClient()
			if err != nil {
				return err
			}

			namespace := ""
			if len(args) > 0 {
				namespace = args[0]
			}

			return runKVList(client, namespace, renderer)
		},
	}

	return cmd
}

func runKVList(client *Client, namespace string, renderer *Renderer) error {
	entries, err := listAllKVEntries(client, namespace)
	if err != nil {
		return err
	}

	result := make([]map[string]any, len(entries))
	for i, entry := range entries {
		result[i] = map[string]any{
			"namespace":    entry.GetNamespace(),
			"key":          entry.GetKey(),
			"creationDate": formatOptionalTime(entry.GetCreationDate()),
			"updateDate":   formatOptionalTime(entry.GetUpdateDate()),
		}
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Found %d key-value entries.\n\n", len(entries))
		fmt.Fprintln(w, "Namespace\tKey\tCreated\tUpdated")
		for _, entry := range entries {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				entry.GetNamespace(),
				entry.GetKey(),
				formatOptionalTime(entry.GetCreationDate()),
				formatOptionalTime(entry.GetUpdateDate()),
			)
		}
		return nil
	})
}

func listAllKVEntries(client *Client, namespace string) ([]kestra.KVEntry, error) {
	const pageSize int32 = 100
	page := int32(1)
	results := make([]kestra.KVEntry, 0)

	for {
		req := client.API.KVAPI.ListAllKeys(client.Ctx, client.Tenant).
			Page(page).
			Size(pageSize)

		if namespace != "" {
			filter := kestra.NewQueryFilter()
			filter.SetField(kestra.QUERYFILTERFIELD_NAMESPACE)
			filter.SetOperation(kestra.QUERYFILTEROP_EQUALS)
			filter.Value = namespace
			req = req.Filters([]kestra.QueryFilter{*filter})
		}

		resp, _, err := req.Execute()
		if err != nil {
			return nil, formatSDKError(err)
		}
		batch := resp.GetResults()
		results = append(results, batch...)

		if len(batch) == 0 || int64(len(results)) >= resp.GetTotal() {
			break
		}
		page++
	}

	return results, nil
}

func formatOptionalTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format(time.RFC3339)
}

func newKVSetCommand() *cobra.Command {
	return newKVWriteCommand("set", false)
}

func newKVUpdateCommand() *cobra.Command {
	return newKVWriteCommand("update", true)
}

func newKVWriteCommand(name string, requireExisting bool) *cobra.Command {
	short := "Set a key-value pair in a namespace."
	if requireExisting {
		short = "Update an existing key-value pair in a namespace."
	}

	cmd := &cobra.Command{
		Use:   fmt.Sprintf("%s <namespace> <type> <key> <value>", name),
		Short: short,
		Long: `Set or update a key-value entry in a namespace.

The value is validated against <type> before sending it to Kestra.`,
		Example: `  # Set a string value
	  kestractl kv set my.namespace STRING api_key "my-secret"

	  # Set a JSON object
	  kestractl kv set my.namespace JSON settings '{"feature":true}'

	  # Update an existing key
	  kestractl kv update my.namespace NUMBER retries 3`,
		Args: cobra.ExactArgs(4),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}

			client, err := NewClient()
			if err != nil {
				return err
			}

			namespace := args[0]
			valueType := args[1]
			return runKVWrite(client, namespace, args[2], args[3], valueType, requireExisting, renderer)
		},
	}

	return cmd
}

func runKVWrite(client *Client, namespace, key, rawValue, valueType string, requireExisting bool, renderer *Renderer) error {
	kvType, err := parseKVType(valueType)
	if err != nil {
		return err
	}

	body, err := formatKVRequestBody(kvType, rawValue)
	if err != nil {
		return err
	}

	if requireExisting {
		_, _, err = client.API.KVAPI.KeyValue(client.Ctx, namespace, key, client.Tenant).Execute()
		if err != nil {
			if tryParseKVTypedValueFromError(err) == nil {
				if isNotFoundSDKError(err) {
					return fmt.Errorf("key %q not found in namespace %q", key, namespace)
				}
				return formatSDKError(err)
			}
		}
	}

	_, err = client.API.KVAPI.SetKeyValue(client.Ctx, namespace, key, client.Tenant).Body(body).Execute()
	if err != nil {
		return formatSDKError(err)
	}

	displayValue := parseKVDisplayValue(kvType, rawValue)
	operation := "set"
	successMessage := "Key-value pair set successfully!"
	if requireExisting {
		operation = "update"
		successMessage = "Key-value pair updated successfully!"
	}

	result := map[string]any{
		"operation": operation,
		"namespace": namespace,
		"key":       key,
		"type":      string(kvType),
		"value":     displayValue,
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, successMessage)
		fmt.Fprintln(w)
		fmt.Fprintf(w, "Operation: %s\n", operation)
		fmt.Fprintf(w, "Namespace: %s\n", namespace)
		fmt.Fprintf(w, "Key: %s\n", key)
		fmt.Fprintf(w, "Type: %s\n", kvType)
		fmt.Fprintf(w, "Value: %s\n", toPrettyString(displayValue))
		return nil
	})
}

func newKVGetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <namespace> <key>",
		Short: "Get a key-value pair.",
		Long:  "Retrieve a key-value entry with its detected type.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}

			client, err := NewClient()
			if err != nil {
				return err
			}

			return runKVGet(client, args[0], args[1], renderer)
		},
	}

	return cmd
}

func runKVGet(client *Client, namespace, key string, renderer *Renderer) error {
	resp, _, err := client.API.KVAPI.KeyValue(client.Ctx, namespace, key, client.Tenant).Execute()

	var typedValue *kvTypedValue
	if err != nil {
		typedValue = tryParseKVTypedValueFromError(err)
		if typedValue == nil {
			return formatSDKError(err)
		}
	} else {
		typedValue = extractKVTypedValue(resp)
	}

	if typedValue == nil {
		return fmt.Errorf("empty response for key %q in namespace %q", key, namespace)
	}

	result := map[string]any{
		"namespace": namespace,
		"key":       key,
		"type":      typedValue.Type,
		"value":     typedValue.Value,
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "Key-value details")
		fmt.Fprintln(w)
		fmt.Fprintf(w, "Namespace: %s\n", namespace)
		fmt.Fprintf(w, "Key: %s\n", key)
		fmt.Fprintf(w, "Type: %s\n", typedValue.Type)
		fmt.Fprintf(w, "Value: %s\n", toPrettyString(typedValue.Value))
		return nil
	})
}

func newKVDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete <namespace> <key>",
		Short:   "Delete a key-value pair.",
		Aliases: []string{"rm"},
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}

			client, err := NewClient()
			if err != nil {
				return err
			}

			return runKVDelete(client, args[0], args[1], renderer)
		},
	}

	return cmd
}

func runKVDelete(client *Client, namespace, key string, renderer *Renderer) error {
	deleted, _, err := client.API.KVAPI.DeleteKeyValue(client.Ctx, namespace, key, client.Tenant).Execute()
	if err != nil {
		return formatSDKError(err)
	}
	if !deleted {
		return fmt.Errorf("key %q in namespace %q was not deleted", key, namespace)
	}

	result := map[string]any{
		"namespace": namespace,
		"key":       key,
		"deleted":   deleted,
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "Key-value pair deleted successfully!")
		fmt.Fprintln(w)
		fmt.Fprintf(w, "Namespace: %s\n", namespace)
		fmt.Fprintf(w, "Key: %s\n", key)
		return nil
	})
}

func parseKVType(rawType string) (kestra.KVType, error) {
	normalized := strings.ToUpper(strings.TrimSpace(rawType))
	kvType, err := kestra.NewKVTypeFromValue(normalized)
	if err != nil {
		return "", fmt.Errorf("invalid type %q, allowed values: %s", rawType, strings.Join(kvAllowedTypes, ", "))
	}
	return *kvType, nil
}

func formatKVRequestBody(kvType kestra.KVType, rawValue string) (string, error) {
	trimmed := strings.TrimSpace(rawValue)

	switch kvType {
	case kestra.KVTYPE_STRING:
		encoded, err := json.Marshal(rawValue)
		if err != nil {
			return "", err
		}
		return string(encoded), nil
	case kestra.KVTYPE_NUMBER:
		var parsed any
		if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
			return "", fmt.Errorf("invalid NUMBER value %q: %w", rawValue, err)
		}
		if _, ok := parsed.(float64); !ok {
			return "", fmt.Errorf("invalid NUMBER value %q", rawValue)
		}
		return trimmed, nil
	case kestra.KVTYPE_BOOLEAN:
		lower := strings.ToLower(trimmed)
		if lower != "true" && lower != "false" {
			return "", fmt.Errorf("invalid BOOLEAN value %q, expected true or false", rawValue)
		}
		return lower, nil
	case kestra.KVTYPE_JSON:
		var parsed any
		if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
			return "", fmt.Errorf("invalid JSON value %q: %w", rawValue, err)
		}
		encoded, err := json.Marshal(parsed)
		if err != nil {
			return "", err
		}
		return string(encoded), nil
	case kestra.KVTYPE_DATE:
		if _, err := time.Parse("2006-01-02", trimmed); err != nil {
			return "", fmt.Errorf("invalid DATE value %q, expected format YYYY-MM-DD", rawValue)
		}
		return trimmed, nil
	case kestra.KVTYPE_DATETIME:
		if _, err := time.Parse(time.RFC3339, trimmed); err != nil {
			return "", fmt.Errorf("invalid DATETIME value %q, expected RFC3339 format", rawValue)
		}
		return trimmed, nil
	case kestra.KVTYPE_DURATION:
		if !isLikelyISO8601Duration(trimmed) {
			return "", fmt.Errorf("invalid DURATION value %q, expected ISO-8601 duration (for example PT15M)", rawValue)
		}
		return trimmed, nil
	default:
		return "", fmt.Errorf("unsupported KV type %q", kvType)
	}
}

func parseKVDisplayValue(kvType kestra.KVType, rawValue string) any {
	trimmed := strings.TrimSpace(rawValue)

	switch kvType {
	case kestra.KVTYPE_NUMBER, kestra.KVTYPE_BOOLEAN, kestra.KVTYPE_JSON:
		var parsed any
		if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
			return parsed
		}
		return rawValue
	default:
		return rawValue
	}
}

func isLikelyISO8601Duration(value string) bool {
	if len(value) < 2 {
		return false
	}
	if !strings.HasPrefix(value, "P") {
		return false
	}
	for _, r := range value {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return false
		}
	}
	return true
}

func extractKVTypedValue(resp *kestra.KVControllerKvDetail) *kvTypedValue {
	if resp == nil {
		return nil
	}

	result := &kvTypedValue{}
	if typ, ok := resp.GetTypeOk(); ok && typ != nil {
		result.Type = string(*typ)
	}

	if resp.Value != nil {
		if value, ok := resp.Value["value"]; ok {
			result.Value = value
		} else if value, ok := resp.Value["Value"]; ok {
			result.Value = value
		} else {
			result.Value = resp.Value
		}
	}

	if resp.AdditionalProperties != nil {
		if result.Type == "" {
			if typ, ok := resp.AdditionalProperties["type"].(string); ok {
				result.Type = typ
			}
		}
		if result.Value == nil {
			if value, ok := resp.AdditionalProperties["value"]; ok {
				result.Value = value
			} else if value, ok := resp.AdditionalProperties["Value"]; ok {
				result.Value = value
			}
		}
	}

	if result.Type == "" && result.Value == nil {
		return nil
	}

	return result
}

func tryParseKVTypedValueFromError(err error) *kvTypedValue {
	sdkErr, ok := err.(*kestra.GenericOpenAPIError)
	if !ok {
		return nil
	}

	body := sdkErr.Body()
	if len(body) == 0 {
		return nil
	}

	var rawResp map[string]any
	if json.Unmarshal(body, &rawResp) != nil {
		return nil
	}

	result := &kvTypedValue{}
	if typ, ok := rawResp["type"].(string); ok {
		result.Type = typ
	}
	if value, ok := rawResp["value"]; ok {
		result.Value = value
	}

	if result.Type == "" && result.Value == nil {
		return nil
	}

	return result
}

func isNotFoundSDKError(err error) bool {
	sdkErr, ok := err.(*kestra.GenericOpenAPIError)
	if !ok {
		return false
	}

	return strings.HasPrefix(sdkErr.Error(), "404")
}
