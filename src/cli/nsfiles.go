package cli

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

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

	if attributes.GetType() == kestra.FILEATTRIBUTESFILETYPE_Directory {
		return runNamespaceFilesList(client, namespace, normalizedPath, false, renderer)
	}

	var revisionNumber *int32
	if revision != "" {
		parsed, err := strconv.ParseInt(revision, 10, 32)
		if err != nil {
			return fmt.Errorf("invalid revision %q: %w", revision, err)
		}
		value := int32(parsed)
		revisionNumber = &value
	}

	file, err := getNamespaceFileContent(client, namespace, normalizedPath, revisionNumber)
	if err != nil {
		return err
	}
	defer cleanupNamespaceTempFile(file)

	_, err = io.Copy(out, file)
	return err
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
		name := strings.TrimSpace(attr.GetFileName())
		if name == "" {
			continue
		}

		entryName := name
		if prefix != "" {
			entryName = prefix + "/" + name
		}

		entry := namespaceFileEntry{
			Name:     entryName,
			Type:     string(attr.GetType()),
			Size:     attr.GetSize(),
			Modified: formatNamespaceFileModified(attr.GetLastModifiedTime(), attr.GetCreationTime()),
		}

		if attr.GetType() == kestra.FILEATTRIBUTESFILETYPE_Directory {
			entry.Size = 0
			if entry.Modified == "" {
				entry.Modified = ""
			}
		}

		entries = append(entries, entry)

		if recursive && attr.GetType() == kestra.FILEATTRIBUTESFILETYPE_Directory {
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

func listNamespaceDirectory(client *Client, namespace, path string) ([]kestra.FileAttributes, error) {
	req := client.API.FilesAPI.ListNamespaceDirectoryFiles(client.Ctx, namespace, client.Tenant)
	if path != "" {
		req = req.Path(path)
	}

	attributes, _, err := req.Execute()
	if err != nil {
		return nil, formatSDKError(err)
	}

	return attributes, nil
}

func getNamespaceFileStats(client *Client, namespace, path string) (kestra.FileAttributes, error) {
	attributes, _, err := client.API.FilesAPI.FileMetadatas(client.Ctx, namespace, client.Tenant).Path(path).Execute()
	if err != nil {
		return kestra.FileAttributes{}, formatSDKError(err)
	}
	if attributes == nil {
		return kestra.FileAttributes{}, fmt.Errorf("namespace file stats not found")
	}

	return *attributes, nil
}

func getNamespaceFileContent(client *Client, namespace, path string, revision *int32) (*os.File, error) {
	req := client.API.FilesAPI.FileContent(client.Ctx, namespace, client.Tenant).Path(path)
	if revision != nil {
		req = req.Revision(*revision)
	}

	file, _, err := req.Execute()
	if err != nil {
		return nil, formatSDKError(err)
	}
	if file == nil {
		return nil, fmt.Errorf("namespace file content not found")
	}

	return file, nil
}

func cleanupNamespaceTempFile(file *os.File) {
	if file == nil {
		return
	}
	name := file.Name()
	_ = file.Close()
	if name != "" {
		_ = os.Remove(name)
	}
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
