package cli

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
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
	cmd.AddCommand(newNamespaceFilesUploadCommand())
	cmd.AddCommand(newNamespaceFilesDeleteCommand())

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
	  kestractl nsfiles list my.namespace

	  # List files in a directory
	  kestractl nsfiles list my.namespace --path workflows/

	  # List files recursively
	  kestractl nsfiles list my.namespace --path workflows/ --recursive

	  # List files with JSON output
	  kestractl nsfiles list my.namespace --output json`,
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
	  kestractl nsfiles get my.namespace --path workflows/example.yaml

	  # Get a specific revision
	  kestractl nsfiles get my.namespace --path workflows/example.yaml --revision 3

	  # List a directory instead of reading a file
	  kestractl nsfiles get my.namespace --path workflows/`,
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

func newNamespaceFilesUploadCommand() *cobra.Command {
	var destination string
	var override bool
	var failFast bool

	cmd := &cobra.Command{
		Use:          "upload <namespace> <local-path>",
		Short:        "Upload namespace files.",
		SilenceUsage: true,
		Long: `Upload a local file or directory to a namespace path.

When a directory is provided, all files are uploaded recursively.
Hidden files and directories (starting with .) are skipped.

The destination path is required and missing directories are created automatically.
By default, uploads fail if a destination file exists. Use --override to replace files.

When uploading multiple files, failures are collected unless --fail-fast is set.`,
		Example: `  # Upload a single file
	  kestractl nsfiles upload my.namespace ./local.txt --path workflows/local.txt

	  # Upload a directory (recursive)
	  kestractl nsfiles upload my.namespace ./assets --path resources

	  # Override existing files
	  kestractl nsfiles upload my.namespace ./assets --path resources --override

	  # Stop on the first error
	  kestractl nsfiles upload my.namespace ./assets --path resources --fail-fast

	  # Upload with JSON output
	  kestractl nsfiles upload my.namespace ./assets --path resources --output json`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := NewClient()
			if err != nil {
				return err
			}

			return runNamespaceFilesUpload(client, args[0], args[1], destination, override, failFast, renderer)
		},
	}

	cmd.Flags().StringVar(&destination, "path", "", "Destination path within the namespace")
	_ = cmd.MarkFlagRequired("path")
	cmd.Flags().BoolVar(&override, "override", false, "Override destination files if they already exist")
	cmd.Flags().BoolVar(&failFast, "fail-fast", false, "Stop on the first upload error")

	return cmd
}

func newNamespaceFilesDeleteCommand() *cobra.Command {
	var targetPath string
	var recursive bool
	var force bool

	cmd := &cobra.Command{
		Use:          "delete <namespace>",
		Short:        "Delete namespace files.",
		SilenceUsage: true,
		Long: `Delete a namespace file or directory.

Deleting directories requires --recursive. Missing targets return an error by default.
Use --force to continue even when some targets are missing.`,
		Example: `  # Delete a file
	  kestractl nsfiles delete my.namespace --path workflows/example.yaml

	  # Delete a directory recursively
	  kestractl nsfiles delete my.namespace --path workflows --recursive

	  # Ignore missing targets
	  kestractl nsfiles delete my.namespace --path workflows/example.yaml --force

	  # Delete with JSON output
	  kestractl nsfiles delete my.namespace --path workflows/example.yaml --output json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := NewClient()
			if err != nil {
				return err
			}

			return runNamespaceFilesDelete(client, args[0], targetPath, recursive, force, renderer)
		},
	}

	cmd.Flags().StringVar(&targetPath, "path", "", "Path within the namespace")
	_ = cmd.MarkFlagRequired("path")
	cmd.Flags().BoolVar(&recursive, "recursive", false, "Delete directories recursively")
	cmd.Flags().BoolVar(&force, "force", false, "Continue even if the target does not exist")

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

type namespaceFileUploadResult struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Size        int64  `json:"size,omitempty"`
	Success     bool   `json:"success"`
	Error       string `json:"error,omitempty"`
}

type namespaceFileUploadSummary struct {
	Total   int                         `json:"total"`
	Success int                         `json:"success"`
	Failed  int                         `json:"failed"`
	Results []namespaceFileUploadResult `json:"results"`
}

type namespaceFileDeleteResult struct {
	Path    string `json:"path"`
	Type    string `json:"type"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

type namespaceFileDeleteSummary struct {
	Total   int                         `json:"total"`
	Success int                         `json:"success"`
	Failed  int                         `json:"failed"`
	Results []namespaceFileDeleteResult `json:"results"`
}

type namespaceFileEntry struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"`
}

type localNamespaceUploadFile struct {
	Path     string
	Relative string
	Size     int64
}

func runNamespaceFilesUpload(client *Client, namespace, localPath, destination string, override bool, failFast bool, renderer *Renderer) error {
	normalizedDest := normalizeNamespacePath(destination)
	if normalizedDest == "" {
		return fmt.Errorf("destination path is required")
	}

	info, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("failed to access local path '%s': %w", localPath, err)
	}

	uploadTargets := make([]namespaceFileUploadResult, 0)
	var items []namespaceFileUploadResult

	if info.IsDir() {
		files, err := collectNamespaceUploadFiles(localPath)
		if err != nil {
			return fmt.Errorf("failed to scan directory '%s': %w", localPath, err)
		}
		if len(files) == 0 {
			return fmt.Errorf("no files found in directory '%s'", localPath)
		}
		destRoot := joinNamespacePath(normalizedDest, filepath.Base(localPath))
		items = make([]namespaceFileUploadResult, 0, len(files))
		for _, file := range files {
			entry := namespaceFileUploadResult{
				Source:      file.Path,
				Destination: joinNamespacePath(destRoot, filepath.ToSlash(file.Relative)),
				Size:        file.Size,
			}
			items = append(items, entry)
		}
	} else {
		items = []namespaceFileUploadResult{{
			Source:      localPath,
			Destination: normalizedDest,
			Size:        info.Size(),
		}}
	}

	failed := 0
	for _, item := range items {
		result := uploadNamespaceFile(client, namespace, item.Source, item.Destination, item.Size, override)
		uploadTargets = append(uploadTargets, result)
		if !result.Success {
			failed++
			if failFast {
				break
			}
		}
	}

	if err := renderNamespaceFilesUploadResults(uploadTargets, renderer); err != nil {
		return err
	}
	if failed > 0 {
		return fmt.Errorf("upload completed with %d error(s)", failed)
	}
	return nil
}

func runNamespaceFilesDelete(client *Client, namespace, targetPath string, recursive bool, force bool, renderer *Renderer) error {
	normalizedPath := normalizeNamespacePath(targetPath)
	if normalizedPath == "" {
		return fmt.Errorf("path is required")
	}

	attrs, exists, err := lookupNamespaceFileStats(client, namespace, normalizedPath)
	if err != nil {
		return err
	}

	results := make([]namespaceFileDeleteResult, 0)
	if !exists {
		results = append(results, namespaceFileDeleteResult{
			Path:    normalizedPath,
			Type:    "unknown",
			Success: false,
			Error:   fmt.Sprintf("path '%s' does not exist", normalizedPath),
		})
		if err := renderNamespaceFilesDeleteResults(results, renderer); err != nil {
			return err
		}
		if force {
			return nil
		}
		return fmt.Errorf("delete completed with 1 error(s)")
	}

	if attrs.GetType() == kestra.FILEATTRIBUTESFILETYPE_Directory && !recursive {
		results = append(results, namespaceFileDeleteResult{
			Path:    normalizedPath,
			Type:    "directory",
			Success: false,
			Error:   "target is a directory; use --recursive to delete",
		})
		if err := renderNamespaceFilesDeleteResults(results, renderer); err != nil {
			return err
		}
		return fmt.Errorf("delete completed with 1 error(s)")
	}

	deleteTargets := []namespaceFileDeleteResult{}
	if attrs.GetType() == kestra.FILEATTRIBUTESFILETYPE_Directory {
		entries, err := collectNamespaceFiles(client, namespace, normalizedPath, true)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			deleteTargets = append(deleteTargets, namespaceFileDeleteResult{
				Path: entry.Name,
				Type: strings.ToLower(entry.Type),
			})
		}
		deleteTargets = append(deleteTargets, namespaceFileDeleteResult{
			Path: normalizedPath,
			Type: "directory",
		})
		deleteTargets = sortDeleteTargets(deleteTargets)
	} else {
		deleteTargets = append(deleteTargets, namespaceFileDeleteResult{
			Path: normalizedPath,
			Type: "file",
		})
	}

	failed := 0
	for _, target := range deleteTargets {
		err := deleteNamespaceFilePath(client, namespace, target.Path)
		result := namespaceFileDeleteResult{
			Path: target.Path,
			Type: target.Type,
		}
		if err != nil {
			result.Success = false
			result.Error = err.Error()
			failed++
		} else {
			result.Success = true
		}
		results = append(results, result)
	}

	if err := renderNamespaceFilesDeleteResults(results, renderer); err != nil {
		return err
	}
	if failed > 0 && !force {
		return fmt.Errorf("delete completed with %d error(s)", failed)
	}
	return nil
}

func collectNamespaceFiles(client *Client, namespace, path string, recursive bool) ([]namespaceFileEntry, error) {
	return collectNamespaceFilesWithPrefix(client, namespace, path, path, recursive)
}

func collectNamespaceUploadFiles(rootPath string) ([]localNamespaceUploadFile, error) {
	var files []localNamespaceUploadFile
	rootPath = filepath.Clean(rootPath)
	return files, filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(d.Name(), ".") && path != rootPath {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(rootPath, path)
		if err != nil {
			return err
		}
		files = append(files, localNamespaceUploadFile{
			Path:     path,
			Relative: rel,
			Size:     info.Size(),
		})
		return nil
	})
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

func lookupNamespaceFileStats(client *Client, namespace, path string) (*kestra.FileAttributes, bool, error) {
	attributes, resp, err := client.API.FilesAPI.FileMetadatas(client.Ctx, namespace, client.Tenant).Path(path).Execute()
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return nil, false, nil
		}
		return nil, false, formatSDKError(err)
	}
	if attributes == nil {
		return nil, false, fmt.Errorf("namespace file stats not found")
	}
	return attributes, true, nil
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

func ensureNamespaceDirectories(client *Client, namespace, dirPath string) error {
	normalized := normalizeNamespacePath(dirPath)
	if normalized == "" {
		return nil
	}

	segments := strings.Split(normalized, "/")
	current := ""
	for _, segment := range segments {
		if segment == "" {
			continue
		}
		current = joinNamespacePath(current, segment)
		attrs, exists, err := lookupNamespaceFileStats(client, namespace, current)
		if err != nil {
			return err
		}
		if exists {
			if attrs.GetType() != kestra.FILEATTRIBUTESFILETYPE_Directory {
				return fmt.Errorf("path '%s' exists and is not a directory", current)
			}
			continue
		}

		_, err = client.API.FilesAPI.CreateNamespaceDirectory(client.Ctx, namespace, client.Tenant).Path(current).Execute()
		if err != nil {
			attrs, exists, statErr := lookupNamespaceFileStats(client, namespace, current)
			if statErr == nil && exists && attrs.GetType() == kestra.FILEATTRIBUTESFILETYPE_Directory {
				continue
			}
			return formatSDKError(err)
		}
	}

	return nil
}

func deleteNamespaceFilePath(client *Client, namespace, path string) error {
	_, err := client.API.FilesAPI.DeleteFileDirectory(client.Ctx, namespace, client.Tenant).Path(path).Execute()
	if err != nil {
		return formatSDKError(err)
	}
	return nil
}

func uploadNamespaceFile(client *Client, namespace, source, destination string, size int64, override bool) namespaceFileUploadResult {
	result := namespaceFileUploadResult{
		Source:      source,
		Destination: destination,
		Size:        size,
		Success:     false,
	}

	if destination == "" {
		result.Error = "destination path is required"
		return result
	}

	attrs, exists, err := lookupNamespaceFileStats(client, namespace, destination)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	if exists {
		if attrs.GetType() == kestra.FILEATTRIBUTESFILETYPE_Directory {
			result.Error = fmt.Sprintf("destination '%s' is a directory", destination)
			return result
		}
		if !override {
			result.Error = fmt.Sprintf("file already exists at '%s'; use --override to replace", destination)
			return result
		}
		if err := deleteNamespaceFilePath(client, namespace, destination); err != nil {
			result.Error = err.Error()
			return result
		}
	}

	if err := ensureNamespaceDirectories(client, namespace, path.Dir(destination)); err != nil {
		result.Error = err.Error()
		return result
	}

	file, err := os.Open(source)
	if err != nil {
		result.Error = fmt.Sprintf("failed to open file: %v", err)
		return result
	}

	_, err = client.API.FilesAPI.CreateNamespaceFile(client.Ctx, namespace, client.Tenant).
		Path(destination).
		FileContent(file).
		Execute()
	if err != nil {
		result.Error = formatSDKError(err).Error()
		return result
	}

	result.Success = true
	return result
}

func renderNamespaceFilesUploadResults(results []namespaceFileUploadResult, renderer *Renderer) error {
	failed := 0
	for _, result := range results {
		if !result.Success {
			failed++
		}
	}
	summary := namespaceFileUploadSummary{
		Total:   len(results),
		Success: len(results) - failed,
		Failed:  failed,
		Results: results,
	}

	return renderer.Render(summary, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "SOURCE\tDESTINATION\tSIZE\tSTATUS\tERROR")
		for _, result := range results {
			status := "OK"
			errMsg := "-"
			if !result.Success {
				status = "FAILED"
				errMsg = result.Error
			}
			fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\n",
				result.Source,
				result.Destination,
				result.Size,
				status,
				errMsg,
			)
		}
		fmt.Fprintf(w, "\n%d file(s) uploaded successfully, %d failed\n", summary.Success, summary.Failed)
		return nil
	})
}

func sortDeleteTargets(targets []namespaceFileDeleteResult) []namespaceFileDeleteResult {
	sorted := make([]namespaceFileDeleteResult, len(targets))
	copy(sorted, targets)
	sort.SliceStable(sorted, func(i, j int) bool {
		depthI := strings.Count(sorted[i].Path, "/")
		depthJ := strings.Count(sorted[j].Path, "/")
		if depthI != depthJ {
			return depthI > depthJ
		}
		if sorted[i].Type != sorted[j].Type {
			return sorted[i].Type != "directory"
		}
		return sorted[i].Path > sorted[j].Path
	})
	return sorted
}

func renderNamespaceFilesDeleteResults(results []namespaceFileDeleteResult, renderer *Renderer) error {
	failed := 0
	for _, result := range results {
		if !result.Success {
			failed++
		}
	}
	summary := namespaceFileDeleteSummary{
		Total:   len(results),
		Success: len(results) - failed,
		Failed:  failed,
		Results: results,
	}

	return renderer.Render(summary, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "PATH\tTYPE\tSTATUS\tERROR")
		for _, result := range results {
			status := "OK"
			errMsg := "-"
			if !result.Success {
				status = "FAILED"
				errMsg = result.Error
			}
			typeValue := result.Type
			if typeValue == "" {
				typeValue = "-"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", result.Path, typeValue, status, errMsg)
		}
		fmt.Fprintf(w, "\n%d path(s) deleted successfully, %d failed\n", summary.Success, summary.Failed)
		return nil
	})
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
