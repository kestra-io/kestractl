package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"text/tabwriter"
	"time"

	kestra "github.com/kestra-io/client-sdk/go-sdk/v2/kestra_api_client"
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
	cmd.AddCommand(newKVDeleteAllCommand())
	cmd.AddCommand(newKVListInheritedCommand())

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

	var ttl string

	cmd := &cobra.Command{
		Use:   fmt.Sprintf("%s <namespace> <type> <key> <value>", name),
		Short: short,
		Long: `Set or update a key-value entry in a namespace.

The value is validated against <type> before sending it to Kestra.`,
		Example: `  # Set a string value
	  kestractl kv set my.namespace STRING api_key "my-secret"

	  # Set a JSON object
	  kestractl kv set my.namespace JSON settings '{"feature":true}'

	  # Set a value with a TTL (ISO 8601 duration)
	  kestractl kv set my.namespace STRING cache_token "abc" --ttl PT1H

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
			return runKVWrite(client, namespace, args[2], args[3], valueType, ttl, requireExisting, renderer)
		},
	}

	cmd.Flags().StringVar(&ttl, "ttl", "", "Time-to-live as ISO 8601 duration (e.g. PT1H, P7D). Omit for no expiry.")

	return cmd
}

func runKVWrite(client *Client, namespace, key, rawValue, valueType, ttl string, requireExisting bool, renderer *Renderer) error {
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

	var ttlPtr *string
	if ttl != "" {
		ttlPtr = &ttl
	}
	if err = client.Kestra.Kv().SetKeyValueWithTTL(client.Ctx, namespace, key, client.Tenant, body, ttlPtr); err != nil {
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
	if ttl != "" {
		result["ttl"] = ttl
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, successMessage)
		fmt.Fprintln(w)
		fmt.Fprintf(w, "Operation: %s\n", operation)
		fmt.Fprintf(w, "Namespace: %s\n", namespace)
		fmt.Fprintf(w, "Key: %s\n", key)
		fmt.Fprintf(w, "Type: %s\n", kvType)
		fmt.Fprintf(w, "Value: %s\n", toPrettyString(displayValue))
		if ttl != "" {
			fmt.Fprintf(w, "TTL: %s\n", ttl)
		}
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
	typedValue, err := fetchKVTypedValue(client, namespace, key)
	if err != nil {
		return err
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
		if err := decodeJSONPreservingNumbers([]byte(trimmed), &parsed); err != nil {
			return "", fmt.Errorf("invalid NUMBER value %q: %w", rawValue, err)
		}
		if _, ok := parsed.(json.Number); !ok {
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
		// Validate without re-encoding: a decode into any followed by
		// json.Marshal turns every integer above 2^53 into a lossy float64 and
		// would store corrupted digits server-side (#121).
		var parsed any
		if err := decodeJSONPreservingNumbers([]byte(trimmed), &parsed); err != nil {
			return "", fmt.Errorf("invalid JSON value %q: %w", rawValue, err)
		}
		var compacted bytes.Buffer
		if err := json.Compact(&compacted, []byte(trimmed)); err != nil {
			return "", fmt.Errorf("invalid JSON value %q: %w", rawValue, err)
		}
		return compacted.String(), nil
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
		if err := decodeJSONPreservingNumbers([]byte(trimmed), &parsed); err == nil {
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

// fetchKVTypedValue reads a KV entry with a direct HTTP request instead of the
// SDK. The SDK types KVControllerKvDetail.Value as map[string]interface{} and
// decodes with plain json.Unmarshal, so a JSON value's integers are already
// float64 (and above 2^53 already wrong) by the time kestractl sees them, and a
// scalar value does not fit the map at all. Reading the body here lets it be
// decoded with UseNumber, keeping every digit (#121). The URL mirrors the SDK's
// KeyValue endpoint, and the request reuses the SDK's configured host, HTTP
// client, default headers and context-based auth.
func fetchKVTypedValue(client *Client, namespace, key string) (*kvTypedValue, error) {
	body, err := rawGet(client, fmt.Sprintf("/api/v1/%s/namespaces/%s/kv/%s",
		url.PathEscape(client.Tenant),
		url.PathEscape(namespace),
		url.PathEscape(key),
	))
	if err != nil {
		return nil, err
	}

	return parseKVTypedValueBody(body), nil
}

// parseKVTypedValueBody reads a KV detail payload, preserving number literals.
func parseKVTypedValueBody(body []byte) *kvTypedValue {
	if len(body) == 0 {
		return nil
	}

	var rawResp map[string]any
	if decodeJSONPreservingNumbers(body, &rawResp) != nil {
		return nil
	}

	// A problem document also carries a "type"; reading that as a value type
	// would turn an error response into a rendered record.
	if isProblemDocument(rawResp) {
		return nil
	}

	result := &kvTypedValue{}
	if typ, ok := rawResp["type"].(string); ok {
		result.Type = typ
	}
	if value, ok := rawResp["value"]; ok {
		result.Value = value
	} else if value, ok := rawResp["Value"]; ok {
		result.Value = value
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
	if decodeJSONPreservingNumbers(body, &rawResp) != nil {
		return nil
	}

	// A 2.0 problem document also carries a "type", and reading that as the
	// value's type would turn a 404 into a rendered record and a zero exit.
	if isProblemDocument(rawResp) {
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

func newKVDeleteAllCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete-all <namespace> <key>...",
		Short: "Bulk-delete multiple key-value pairs from a namespace.",
		Example: `  kestractl kv delete-all my.namespace key1 key2 key3
  kestractl kv delete-all my.namespace key1 --output json`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runKVDeleteAll(client, args[0], args[1:], renderer)
		},
	}
	return cmd
}

func runKVDeleteAll(client *Client, namespace string, keys []string, renderer *Renderer) error {
	req := kestra.NewKVControllerApiDeleteBulkRequest()
	req.SetKeys(keys)

	resp, _, err := client.API.KVAPI.
		DeleteKeyValues(client.Ctx, namespace, client.Tenant).
		KVControllerApiDeleteBulkRequest(*req).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	deleted := resp.GetKeys()
	result := map[string]any{
		"namespace": namespace,
		"deleted":   deleted,
		"count":     len(deleted),
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Deleted %d key(s) from namespace %q.\n\n", len(deleted), namespace)
		fmt.Fprintln(w, "KEY")
		for _, k := range deleted {
			fmt.Fprintln(w, k)
		}
		return nil
	})
}

func newKVListInheritedCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list-inherited <namespace>",
		Short: "List KV entries including inherited keys from parent namespaces.",
		Example: `  kestractl kv list-inherited my.namespace
  kestractl kv list-inherited my.namespace --output json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runKVListInherited(client, args[0], renderer)
		},
	}
	return cmd
}

func runKVListInherited(client *Client, namespace string, renderer *Renderer) error {
	entries, _, err := client.API.KVAPI.
		ListKeysWithInheritence(client.Ctx, namespace, client.Tenant).
		Execute()
	if err != nil {
		return formatSDKError(err)
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
		fmt.Fprintf(w, "Found %d inherited key-value entries.\n\n", len(entries))
		fmt.Fprintln(w, "NAMESPACE\tKEY\tCREATED\tUPDATED")
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

func isNotFoundSDKError(err error) bool {
	sdkErr, ok := err.(*kestra.GenericOpenAPIError)
	if !ok {
		return false
	}

	return strings.HasPrefix(sdkErr.Error(), "404")
}
