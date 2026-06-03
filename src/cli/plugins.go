package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
)

// errRateLimited is returned by downloadJAR when Maven Central responds with 429
// after all retry attempts are exhausted.
var errRateLimited = errors.New("rate limited by Maven Central (429)")

// rateLimitWaits defines the back-off delays between successive 429 retries.
// Exposed as a var so tests can override it to avoid sleeping.
var rateLimitWaits = []time.Duration{5 * time.Second, 10 * time.Second, 30 * time.Second}

// pluginsAPIBase and pluginsMavenBase are vars so tests can point them at a local httptest server.
var (
	pluginsAPIBase   = "https://api.kestra.io/v1/plugins/artifacts/core-compatibility"
	pluginsMavenBase = "https://repo1.maven.org/maven2"
)

type pluginArtifact struct {
	GroupID    string `json:"groupId"`
	ArtifactID string `json:"artifactId"`
	License    string `json:"license"`
	Version    string `json:"version"`
}

type downloadResult struct {
	index   int
	plugin  pluginArtifact
	bytes   int64
	err     error
	skipped bool
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
	var forceRedownload bool
	var edition string
	var keepOnlyLastVersion bool
	var globalTimeout time.Duration

	cmd := &cobra.Command{
		Use:   "download <version>",
		Short: "Download all plugins for a given Kestra version from Maven Central",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			license, err := editionToLicense(edition)
			if err != nil {
				return err
			}
			return runPluginsInstall(cmd.OutOrStdout(), args[0], pluginsDir, concurrency, forceRedownload, license, keepOnlyLastVersion, globalTimeout)
		},
		Annotations: map[string]string{AnnotationOffline: "true"},
	}

	cmd.Flags().StringVar(&pluginsDir, "plugins-dir", "./plugins", "Directory to write downloaded JARs into")
	cmd.Flags().IntVar(&concurrency, "concurrency", 1, "Number of parallel downloads")
	cmd.Flags().BoolVar(&forceRedownload, "force-redownload", false, "Re-download plugins even if they already exist and pass checksum verification")
	cmd.Flags().StringVar(&edition, "edition", "ALL", "Edition to download: ALL, OSS (open-source only), or EE (enterprise only)")
	cmd.Flags().BoolVar(&keepOnlyLastVersion, "keep-only-last-version", true, "Remove older versions of each plugin from the plugins directory after downloading")
	cmd.Flags().DurationVar(&globalTimeout, "global-timeout", 5*time.Minute, "Maximum total time allowed for all downloads")
	return cmd
}

// editionToLicense maps the user-facing --edition value to the API license query param.
// Returns an empty string for ALL (no filtering).
func editionToLicense(edition string) (string, error) {
	switch strings.ToUpper(edition) {
	case "ALL":
		return "", nil
	case "OSS":
		return "OPEN_SOURCE", nil
	case "EE":
		return "ENTERPRISE", nil
	default:
		return "", fmt.Errorf("invalid --edition %q: must be ALL, OSS, or EE", edition)
	}
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

func runPluginsInstall(out io.Writer, kestraVersion string, pluginsDir string, concurrency int, forceRedownload bool, license string, keepOnlyLastVersion bool, globalTimeout time.Duration) error {
	kestraVersion = resolveVersion(kestraVersion)
	fmt.Fprintf(out, "Fetching plugin list for Kestra %s...\n", kestraVersion)

	plugins, err := fetchPluginList(kestraVersion, license)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Found %d plugins.\n\n", len(plugins))

	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		return fmt.Errorf("cannot create plugins directory %q: %w", pluginsDir, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), globalTimeout)
	defer cancel()

	width := len(fmt.Sprintf("%d", len(plugins)))
	lineFormat := fmt.Sprintf("[%%%dd/%%%dd]", width, width)

	// outMu serializes all writes to out, including retry log lines from goroutines.
	var outMu sync.Mutex
	logf := func(format string, args ...any) {
		outMu.Lock()
		fmt.Fprintf(out, format, args...)
		outMu.Unlock()
	}

	sem := make(chan struct{}, concurrency)
	results := make(chan downloadResult, len(plugins))

	var wg sync.WaitGroup
	for i, p := range plugins {
		wg.Add(1)
		go func(i int, p pluginArtifact) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results <- downloadResult{index: i, plugin: p, err: ctx.Err()}
				return
			}
			defer func() { <-sem }()
			n, skipped, err := downloadJAR(ctx, logf, p, pluginsDir, forceRedownload)
			results <- downloadResult{index: i, plugin: p, bytes: n, err: err, skipped: skipped}
		}(i, p)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	downloaded := 0
	skippedCount := 0
	failed := 0
	stoppedEarly := false

	for r := range results {
		label := fmt.Sprintf("%s:%s:%s", r.plugin.GroupID, r.plugin.ArtifactID, r.plugin.Version)
		outMu.Lock()
		if errors.Is(r.err, errRateLimited) {
			fmt.Fprintf(out, lineFormat+" %s ... FAILED (rate limited — stopping early)\n", r.index+1, len(plugins), label)
			failed++
			if !stoppedEarly {
				stoppedEarly = true
				cancel()
			}
		} else if r.err != nil {
			fmt.Fprintf(out, lineFormat+" %s ... FAILED (%v)\n", r.index+1, len(plugins), label, r.err)
			failed++
		} else if r.skipped {
			fmt.Fprintf(out, lineFormat+" %s ... already up to date\n", r.index+1, len(plugins), label)
			skippedCount++
		} else {
			fmt.Fprintf(out, lineFormat+" %s ... done (%.1f MB)\n", r.index+1, len(plugins), label, float64(r.bytes)/(1024*1024))
			downloaded++
		}
		outMu.Unlock()
	}

	fmt.Fprintf(out, "\nDownloaded %d", downloaded)
	if skippedCount > 0 {
		fmt.Fprintf(out, ", skipped %d (already up to date)", skippedCount)
	}
	fmt.Fprintf(out, " plugin(s) to %s", pluginsDir)
	if failed > 0 {
		fmt.Fprintf(out, ", %d failed", failed)
	}
	fmt.Fprintln(out, ".")

	if keepOnlyLastVersion {
		removed, pruneErr := pruneOldVersions(pluginsDir, plugins)
		if pruneErr != nil {
			return pruneErr
		}
		if removed > 0 {
			fmt.Fprintf(out, "Removed %d old version(s) from %s.\n", removed, pluginsDir)
		}
	}

	if stoppedEarly {
		return fmt.Errorf("download stopped early: Maven Central is rate limiting — %d plugin(s) failed", failed)
	}
	if failed > 0 {
		return fmt.Errorf("%d plugin(s) failed to download", failed)
	}
	return nil
}

// pruneOldVersions removes JARs in pluginsDir that belong to a plugin in the current list
// but have a different (older) version than what was just downloaded.
func pruneOldVersions(pluginsDir string, current []pluginArtifact) (int, error) {
	currentFiles := make(map[string]struct{}, len(current))
	for _, p := range current {
		currentFiles[pluginFileName(p)] = struct{}{}
	}

	// Build prefixes of the form "<groupId>__<artifactId>__" to identify which dir entries
	// belong to a known plugin regardless of version.
	type prefix struct{ s string }
	prefixes := make([]prefix, 0, len(current))
	for _, p := range current {
		groupID := strings.ReplaceAll(p.GroupID, ".", "_")
		prefixes = append(prefixes, prefix{groupID + "__" + p.ArtifactID + "__"})
	}

	entries, err := os.ReadDir(pluginsDir)
	if err != nil {
		return 0, fmt.Errorf("cannot read plugins directory: %w", err)
	}

	removed := 0
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".jar") {
			continue
		}
		if _, isCurrent := currentFiles[name]; isCurrent {
			continue
		}
		for _, pfx := range prefixes {
			if strings.HasPrefix(name, pfx.s) {
				if err := os.Remove(filepath.Join(pluginsDir, name)); err != nil {
					return removed, fmt.Errorf("failed to remove old version %q: %w", name, err)
				}
				removed++
				break
			}
		}
	}
	return removed, nil
}

func fetchPluginList(kestraVersion string, license string) ([]pluginArtifact, error) {
	url := fmt.Sprintf("%s/%s/latest", pluginsAPIBase, kestraVersion)
	if license != "" {
		url += "?license=" + license
	}

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

func downloadJAR(ctx context.Context, logf func(string, ...any), p pluginArtifact, destDir string, forceRedownload bool) (int64, bool, error) {
	destPath := filepath.Join(destDir, pluginFileName(p))

	if !forceRedownload {
		if _, err := os.Stat(destPath); err == nil {
			return 0, true, nil
		}
	}

	url := mavenJARURL(p)
	label := fmt.Sprintf("%s:%s:%s", p.GroupID, p.ArtifactID, p.Version)

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return 0, false, fmt.Errorf("failed to build request: %w", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return 0, false, fmt.Errorf("HTTP request failed: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			if attempt >= len(rateLimitWaits) {
				return 0, false, errRateLimited
			}
			wait := rateLimitWaits[attempt]
			logf("  [429] rate limited on %s — waiting %s (retry %d/%d)\n", label, wait, attempt+1, len(rateLimitWaits))
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return 0, false, ctx.Err()
			}
			continue
		}

		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return 0, false, fmt.Errorf("Maven Central returned HTTP %d", resp.StatusCode)
		}

		f, err := os.Create(destPath)
		if err != nil {
			return 0, false, fmt.Errorf("cannot create file: %w", err)
		}
		defer f.Close()

		n, err := io.Copy(f, resp.Body)
		if err != nil {
			return 0, false, fmt.Errorf("write failed: %w", err)
		}
		return n, false, nil
	}
}

// mavenJARURL builds the Maven Central download URL for a plugin artifact.
func mavenJARURL(p pluginArtifact) string {
	groupPath := strings.ReplaceAll(p.GroupID, ".", "/")
	return fmt.Sprintf("%s/%s/%s/%s/%s-%s.jar",
		pluginsMavenBase, groupPath, p.ArtifactID, p.Version, p.ArtifactID, p.Version)
}
