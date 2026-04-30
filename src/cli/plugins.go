package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/spf13/cobra"
)

// pluginsAPIBase and pluginsMavenBase are vars so tests can point them at a local httptest server.
var (
	pluginsAPIBase  = "https://api.kestra.io/v1/plugins/artifacts/core-compatibility"
	pluginsMavenBase = "https://repo1.maven.org/maven2"
)

type pluginArtifact struct {
	GroupID    string `json:"groupId"`
	ArtifactID string `json:"artifactId"`
	License    string `json:"license"`
	Version    string `json:"version"`
}

type downloadResult struct {
	index  int
	plugin pluginArtifact
	bytes  int64
	err    error
}

func newPluginsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugins",
		Short: "Manage Kestra plugins",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
		Annotations: map[string]string{AnnotationOffline: "true"},
	}
	cmd.AddCommand(newPluginsDownloadCommand())
	return cmd
}

func newPluginsDownloadCommand() *cobra.Command {
	var pluginsDir string
	var concurrency int

	cmd := &cobra.Command{
		Use:   "download <version>",
		Short: "Download all plugins for a given Kestra version from Maven Central",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginsInstall(cmd.OutOrStdout(), args[0], pluginsDir, concurrency)
		},
		Annotations: map[string]string{AnnotationOffline: "true"},
	}

	cmd.Flags().StringVar(&pluginsDir, "plugins-dir", "./plugins", "Directory to write downloaded JARs into")
	cmd.Flags().IntVar(&concurrency, "concurrency", 1, "Number of parallel downloads")
	return cmd
}

// resolveVersion maps symbolic version aliases to their concrete equivalents.
func resolveVersion(version string) string {
	switch strings.ToLower(version) {
	case "develop", "latest":
		return "999.999.999"
	default:
		return version
	}
}

func runPluginsInstall(out io.Writer, kestraVersion string, pluginsDir string, concurrency int) error {
	kestraVersion = resolveVersion(kestraVersion)
	fmt.Fprintf(out, "Fetching plugin list for Kestra %s...\n", kestraVersion)

	plugins, err := fetchPluginList(kestraVersion)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Found %d plugins.\n\n", len(plugins))

	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		return fmt.Errorf("cannot create plugins directory %q: %w", pluginsDir, err)
	}

	width := len(fmt.Sprintf("%d", len(plugins)))
	lineFormat := fmt.Sprintf("[%%%dd/%%%dd]", width, width)

	sem := make(chan struct{}, concurrency)
	results := make(chan downloadResult, len(plugins))

	var wg sync.WaitGroup
	for i, p := range plugins {
		wg.Add(1)
		go func(i int, p pluginArtifact) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			n, err := downloadJAR(p, pluginsDir)
			results <- downloadResult{index: i, plugin: p, bytes: n, err: err}
		}(i, p)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var mu sync.Mutex
	installed := 0
	failed := 0

	for r := range results {
		label := fmt.Sprintf("%s:%s:%s", r.plugin.GroupID, r.plugin.ArtifactID, r.plugin.Version)
		mu.Lock()
		if r.err != nil {
			fmt.Fprintf(out, lineFormat+" Downloading %s ... FAILED (%v)\n", r.index+1, len(plugins), label, r.err)
			failed++
		} else {
			fmt.Fprintf(out, lineFormat+" Downloading %s ... done (%.1f MB)\n", r.index+1, len(plugins), label, float64(r.bytes)/(1024*1024))
			installed++
		}
		mu.Unlock()
	}

	fmt.Fprintf(out, "\nInstalled %d plugin(s) to %s", installed, pluginsDir)
	if failed > 0 {
		fmt.Fprintf(out, ", %d failed", failed)
	}
	fmt.Fprintln(out, ".")

	if failed > 0 {
		return fmt.Errorf("%d plugin(s) failed to download", failed)
	}
	return nil
}

func fetchPluginList(kestraVersion string) ([]pluginArtifact, error) {
	url := fmt.Sprintf("%s/%s/latest", pluginsAPIBase, kestraVersion)

	resp, err := http.Get(url) //nolint:gosec // URL is constructed from validated version arg
	if err != nil {
		return nil, fmt.Errorf("failed to reach plugin API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("plugin API returned HTTP %d for version %q — check that the version exists", resp.StatusCode, kestraVersion)
	}

	var plugins []pluginArtifact
	if err := json.NewDecoder(resp.Body).Decode(&plugins); err != nil {
		return nil, fmt.Errorf("failed to parse plugin list: %w", err)
	}
	return plugins, nil
}

// pluginFileName returns the Kestra-compatible filename for a plugin artifact.
// Format: <groupId>__<artifactId>__<version>.jar, with dots replaced by underscores
// in groupId and version (e.g. io_kestra_plugin__plugin-kafka__1_6_0.jar).
func pluginFileName(p pluginArtifact) string {
	groupID := strings.ReplaceAll(p.GroupID, ".", "_")
	version := strings.ReplaceAll(p.Version, ".", "_")
	return groupID + "__" + p.ArtifactID + "__" + version + ".jar"
}

func downloadJAR(p pluginArtifact, destDir string) (int64, error) {
	url := mavenJARURL(p)
	destPath := filepath.Join(destDir, pluginFileName(p))

	resp, err := http.Get(url) //nolint:gosec // URL is constructed from trusted API data
	if err != nil {
		return 0, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("Maven Central returned HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return 0, fmt.Errorf("cannot create file: %w", err)
	}
	defer f.Close()

	n, err := io.Copy(f, resp.Body)
	if err != nil {
		return 0, fmt.Errorf("write failed: %w", err)
	}
	return n, nil
}

// mavenJARURL builds the Maven Central download URL for a plugin artifact.
func mavenJARURL(p pluginArtifact) string {
	groupPath := strings.ReplaceAll(p.GroupID, ".", "/")
	return fmt.Sprintf("%s/%s/%s/%s/%s-%s.jar",
		pluginsMavenBase, groupPath, p.ArtifactID, p.Version, p.ArtifactID, p.Version)
}
