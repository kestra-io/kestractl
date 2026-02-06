package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
	"unsafe"

	"github.com/spf13/cobra"

	kestra "github.com/kestra-io/client-sdk/go-sdk/kestra_api_client"
)

func newNamespaceFilesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "nsfiles",
		Short: "Manage namespace files",
	}

	cmd.AddCommand(newNamespaceFilesListCommand())
	cmd.AddCommand(newNamespaceFilesGetCommand())

	return cmd
}

func newNamespaceFilesListCommand() *cobra.Command {
	var path string
	var recursive bool

	cmd := &cobra.Command{
		Use:   "list <namespace>",
		Short: "List namespace files.",
		Long: `List files and directories within a namespace.

Supports listing the root or a specific directory path, with optional recursion.`,
		Example: `  # List files at the namespace root
  kestra nsfiles list my.namespace

  # List files in a directory
  kestra nsfiles list my.namespace --path workflows/

  # List files recursively
  kestra nsfiles list my.namespace --path workflows/ --recursive

  # List files with JSON output
  kestra nsfiles list my.namespace --output json`,
		Aliases: []string{"ls"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := NewClient()
			if err != nil {
				return err
			}

			return runNamespaceFilesList(client, args[0], path, recursive, renderer)
		},
	}

	cmd.Flags().StringVar(&path, "path", "", "Path within the namespace")
	cmd.Flags().BoolVar(&recursive, "recursive", false, "List files recursively")

	return cmd
}

func newNamespaceFilesGetCommand() *cobra.Command {
	var path string
	var revision string

	cmd := &cobra.Command{
		Use:   "get <namespace>",
		Short: "Get namespace file content.",
		Long: `Retrieve a namespace file and stream its raw bytes to stdout.

If the provided path is a directory, the command returns a directory listing.`,
		Example: `  # Get a file's raw content
  kestra nsfiles get my.namespace --path workflows/example.yaml

  # Get a specific revision
  kestra nsfiles get my.namespace --path workflows/example.yaml --revision 3

  # List a directory instead of reading a file
  kestra nsfiles get my.namespace --path workflows/`,
		Aliases: []string{"cat"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := NewClient()
			if err != nil {
				return err
			}

			return runNamespaceFilesGet(client, args[0], path, revision, renderer, cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVar(&path, "path", "", "Path within the namespace")
	_ = cmd.MarkFlagRequired("path")
	cmd.Flags().StringVar(&revision, "revision", "", "Revision number to fetch")

	return cmd
}

func runNamespaceFilesList(client *Client, namespace, path string, recursive bool, renderer *Renderer) error {
	normalizedPath := normalizeNamespacePath(path)
	entries, err := collectNamespaceFiles(client, namespace, normalizedPath, recursive)
	if err != nil {
		return err
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	return renderer.Render(entries, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "Name\tType\tSize\tModified")
		for _, entry := range entries {
			modified := entry.Modified
			if modified == "" {
				modified = "-"
			}
			fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", entry.Name, entry.Type, entry.Size, modified)
		}
		return nil
	})
}

func runNamespaceFilesGet(client *Client, namespace, path, revision string, renderer *Renderer, out io.Writer) error {
	normalizedPath := normalizeNamespacePath(path)
	if normalizedPath == "" {
		return runNamespaceFilesList(client, namespace, normalizedPath, false, renderer)
	}

	attributes, err := getNamespaceFileStats(client, namespace, normalizedPath)
	if err != nil {
		return err
	}

	if strings.EqualFold(attributes.Type, "Directory") {
		return runNamespaceFilesList(client, namespace, normalizedPath, false, renderer)
	}

	resp, err := getNamespaceFileContent(client, namespace, normalizedPath, revision)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

type namespaceFileAttributes struct {
	FileName         string `json:"fileName"`
	LastModifiedTime int64  `json:"lastModifiedTime"`
	CreationTime     int64  `json:"creationTime"`
	Type             string `json:"type"`
	Size             int64  `json:"size"`
}

type namespaceFileEntry struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"`
}

func collectNamespaceFiles(client *Client, namespace, path string, recursive bool) ([]namespaceFileEntry, error) {
	return collectNamespaceFilesWithPrefix(client, namespace, path, path, recursive)
}

func collectNamespaceFilesWithPrefix(client *Client, namespace, path, prefix string, recursive bool) ([]namespaceFileEntry, error) {
	attributes, err := listNamespaceDirectory(client, namespace, path)
	if err != nil {
		return nil, err
	}

	entries := make([]namespaceFileEntry, 0, len(attributes))
	for _, attr := range attributes {
		name := strings.TrimSpace(attr.FileName)
		if name == "" {
			continue
		}

		entryName := name
		if prefix != "" {
			entryName = prefix + "/" + name
		}

		entry := namespaceFileEntry{
			Name:     entryName,
			Type:     attr.Type,
			Size:     attr.Size,
			Modified: formatNamespaceFileModified(attr.LastModifiedTime, attr.CreationTime),
		}

		if strings.EqualFold(attr.Type, "Directory") {
			entry.Size = 0
			if entry.Modified == "" {
				entry.Modified = ""
			}
		}

		entries = append(entries, entry)

		if recursive && strings.EqualFold(attr.Type, "Directory") {
			childPath := joinNamespacePath(path, name)
			childEntries, err := collectNamespaceFilesWithPrefix(client, namespace, childPath, entryName, recursive)
			if err != nil {
				return nil, err
			}
			entries = append(entries, childEntries...)
		}
	}

	return entries, nil
}

func listNamespaceDirectory(client *Client, namespace, path string) ([]namespaceFileAttributes, error) {
	baseURL, cfg, err := namespaceFilesBaseURL(client)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/api/v1/%s/namespaces/%s/files/directory",
		baseURL,
		url.PathEscape(client.Tenant),
		url.PathEscape(namespace),
	)

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	query := req.URL.Query()
	if path != "" {
		query.Set("path", path)
	}
	req.URL.RawQuery = query.Encode()

	req.Header.Set("Accept", "application/json")
	if cfg.UserAgent != "" {
		req.Header.Set("User-Agent", cfg.UserAgent)
	}
	for header, value := range cfg.DefaultHeader {
		req.Header.Add(header, value)
	}

	applyNamespaceAuth(client.Ctx, req)

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= http.StatusMultipleChoices {
		return nil, formatNamespaceFilesHTTPError(resp.Status, body)
	}

	if len(body) == 0 {
		return []namespaceFileAttributes{}, nil
	}

	var attributes []namespaceFileAttributes
	if err := json.Unmarshal(body, &attributes); err != nil {
		return nil, err
	}

	return attributes, nil
}

func getNamespaceFileStats(client *Client, namespace, path string) (namespaceFileAttributes, error) {
	baseURL, cfg, err := namespaceFilesBaseURL(client)
	if err != nil {
		return namespaceFileAttributes{}, err
	}

	endpoint := fmt.Sprintf("%s/api/v1/%s/namespaces/%s/files/stats",
		baseURL,
		url.PathEscape(client.Tenant),
		url.PathEscape(namespace),
	)

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return namespaceFileAttributes{}, err
	}

	query := req.URL.Query()
	if path != "" {
		query.Set("path", path)
	}
	req.URL.RawQuery = query.Encode()

	req.Header.Set("Accept", "application/json")
	if cfg.UserAgent != "" {
		req.Header.Set("User-Agent", cfg.UserAgent)
	}
	for header, value := range cfg.DefaultHeader {
		req.Header.Add(header, value)
	}

	applyNamespaceAuth(client.Ctx, req)

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return namespaceFileAttributes{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return namespaceFileAttributes{}, err
	}

	if resp.StatusCode >= http.StatusMultipleChoices {
		return namespaceFileAttributes{}, formatNamespaceFilesHTTPError(resp.Status, body)
	}

	if len(body) == 0 {
		return namespaceFileAttributes{}, nil
	}

	var attributes namespaceFileAttributes
	if err := json.Unmarshal(body, &attributes); err != nil {
		return namespaceFileAttributes{}, err
	}

	return attributes, nil
}

func getNamespaceFileContent(client *Client, namespace, path, revision string) (*http.Response, error) {
	baseURL, cfg, err := namespaceFilesBaseURL(client)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/api/v1/%s/namespaces/%s/files",
		baseURL,
		url.PathEscape(client.Tenant),
		url.PathEscape(namespace),
	)

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	query := req.URL.Query()
	if path != "" {
		query.Set("path", path)
	}
	if revision != "" {
		query.Set("revision", revision)
	}
	req.URL.RawQuery = query.Encode()

	req.Header.Set("Accept", "application/octet-stream")
	if cfg.UserAgent != "" {
		req.Header.Set("User-Agent", cfg.UserAgent)
	}
	for header, value := range cfg.DefaultHeader {
		req.Header.Add(header, value)
	}

	applyNamespaceAuth(client.Ctx, req)

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= http.StatusMultipleChoices {
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		return nil, formatNamespaceFilesHTTPError(resp.Status, body)
	}

	return resp, nil
}

func namespaceFilesBaseURL(client *Client) (string, *kestra.Configuration, error) {
	cfg := client.API.GetConfig()
	baseURL := ""
	if len(cfg.Servers) > 0 {
		baseURL = cfg.Servers[0].URL
	}
	if baseURL == "" && cfg.Scheme != "" && cfg.Host != "" {
		baseURL = fmt.Sprintf("%s://%s", cfg.Scheme, cfg.Host)
	}
	if baseURL == "" {
		return "", nil, fmt.Errorf("missing API base URL")
	}
	return strings.TrimRight(baseURL, "/"), cfg, nil
}

func applyNamespaceAuth(ctx context.Context, req *http.Request) {
	if ctx == nil || req == nil {
		return
	}

	if token, ok := ctx.Value(kestra.ContextAccessToken).(string); ok && token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
		return
	}

	if auth, ok := ctx.Value(kestra.ContextBasicAuth).(kestra.BasicAuth); ok {
		req.SetBasicAuth(auth.UserName, auth.Password)
	}
}

func formatNamespaceFilesHTTPError(status string, body []byte) error {
	sdkErr := &kestra.GenericOpenAPIError{}
	setGenericOpenAPIErrorFields(sdkErr, body, status)
	return formatSDKError(sdkErr)
}

func setGenericOpenAPIErrorFields(err *kestra.GenericOpenAPIError, body []byte, message string) {
	val := reflect.ValueOf(err).Elem()
	bodyField := val.FieldByName("body")
	reflect.NewAt(bodyField.Type(), unsafe.Pointer(bodyField.UnsafeAddr())).Elem().SetBytes(body)

	if message == "" {
		return
	}

	msgField := val.FieldByName("error")
	reflect.NewAt(msgField.Type(), unsafe.Pointer(msgField.UnsafeAddr())).Elem().SetString(message)
}

func normalizeNamespacePath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" || trimmed == "/" || trimmed == "." {
		return ""
	}
	if strings.HasPrefix(trimmed, "/") {
		trimmed = strings.TrimPrefix(trimmed, "/")
	}
	trimmed = strings.TrimSuffix(trimmed, "/")
	if trimmed == "" || trimmed == "." {
		return ""
	}
	return trimmed
}

func joinNamespacePath(base, child string) string {
	if base == "" {
		return child
	}
	return base + "/" + child
}

func formatNamespaceFileModified(lastModified, created int64) string {
	timestamp := lastModified
	if timestamp == 0 {
		timestamp = created
	}
	if timestamp == 0 {
		return ""
	}
	timeValue := time.Unix(0, timestamp*int64(time.Millisecond)).UTC()
	return timeValue.Format(time.RFC3339)
}
