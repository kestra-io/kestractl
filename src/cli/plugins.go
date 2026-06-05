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
var errRateLimited = errors.New("rate limited by repository (429)")

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
	index     int
	plugin    pluginArtifact
	bytes     int64
	err       error
	skipped   bool
	cancelled bool
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
	cmd.AddCommand(newPluginsListCommand())
	return cmd
}

func newPluginsDownloadCommand() *cobra.Command {
	var pluginsDir string
	var concurrency int
	var forceRedownload bool
	var edition string
	var keepOnlyLastVersion bool
	var globalTimeout time.Duration
	var mavenRepository string
	var mavenUsername string
	var mavenPassword string
	var pluginsList []string
	var configPaths []string

	cmd := &cobra.Command{
		Use:   "download [version]",
		Short: "Download all plugins for a given Kestra version from a Maven repository",
		Long: `Download all plugins for a given Kestra version from a Maven repository.

By default the plugin list is fetched from the Kestra API for the given version.
Alternatively, pass an explicit list of plugins with --plugins, in which case the
version argument is optional and the API is not called. The --plugins format matches
the output of "kestractl plugins list", making it easy to pipe the two commands:

  kestractl plugins download --plugins "$(kestractl plugins list 1.3.9 --edition OSS)"

By default plugins are fetched from Maven Central. Use --maven-repository to
point at a custom registry (mirror, internal Nexus/Artifactory, etc.).

Authentication:

  Basic auth (--maven-username / --maven-password):
    kestractl plugins download 1.3.9 \
      --maven-repository https://nexus.example.com/repository/maven-central \
      --maven-username myuser \
      --maven-password mypassword

  Bearer token (--header):
    kestractl plugins download 1.3.9 \
      --maven-repository https://nexus.example.com/repository/maven-central \
      --header "Authorization:Bearer <token>"

  GCP Artifact Registry — service account key (GOOGLE_APPLICATION_CREDENTIALS set):
    kestractl plugins download 1.3.9 \
      --maven-repository https://europe-west1-maven.pkg.dev/my-project/my-repo \
      --maven-username _json_key \
      --maven-password "$(cat $GOOGLE_APPLICATION_CREDENTIALS)"

  GCP Artifact Registry — gcloud access token:
    kestractl plugins download 1.3.9 \
      --maven-repository https://europe-west1-maven.pkg.dev/my-project/my-repo \
      --maven-username oauth2accesstoken \
      --maven-password "$(gcloud auth print-access-token)"`,
		Args: func(cmd *cobra.Command, args []string) error {
			hasPlugins, _ := cmd.Flags().GetStringArray("plugins")
			if len(hasPlugins) == 0 && len(args) == 0 {
				return fmt.Errorf("requires a version argument (the version is only optional when --plugins is set)")
			}
			if len(args) > 1 {
				return fmt.Errorf("accepts at most 1 arg, received %d", len(args))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			license, err := editionToLicense(edition)
			if err != nil {
				return err
			}
			if len(pluginsList) > 0 && len(configPaths) > 0 {
				return fmt.Errorf("--plugins and --from-config are mutually exclusive")
			}
			version := ""
			if len(args) > 0 {
				version = args[0]
			}
			var explicit []pluginArtifact
			switch {
			case len(pluginsList) > 0:
				explicit, err = parsePluginCoordinates(pluginsList)
			case len(configPaths) > 0:
				explicit, err = corePluginsFromConfig(configPaths, resolveVersion(version), license)
			}
			if err != nil {
				return err
			}
			headers, _ := cmd.Root().PersistentFlags().GetStringArray(FlagHeader)
			return runPluginsInstall(cmd.OutOrStdout(), version, pluginsDir, concurrency, forceRedownload, license, keepOnlyLastVersion, globalTimeout, mavenRepository, mavenUsername, mavenPassword, headers, explicit)
		},
		Annotations: map[string]string{AnnotationOffline: "true"},
	}

	cmd.Flags().StringVar(&pluginsDir, "plugins-dir", "./plugins", "Directory to write downloaded JARs into")
	cmd.Flags().IntVar(&concurrency, "concurrency", 1, "Number of parallel downloads")
	cmd.Flags().BoolVar(&forceRedownload, "force-redownload", false, "Re-download plugins even if they already exist and pass checksum verification")
	cmd.Flags().StringVar(&edition, "edition", "ALL", "Edition to download: ALL, OSS (open-source only), or EE (enterprise only)")
	cmd.Flags().BoolVar(&keepOnlyLastVersion, "keep-only-last-version", true, "Remove older versions of each plugin from the plugins directory after downloading")
	cmd.Flags().DurationVar(&globalTimeout, "global-timeout", 5*time.Minute, "Maximum total time allowed for all downloads")
	cmd.Flags().StringVar(&mavenRepository, "maven-repository", "", "Custom Maven repository base URL (defaults to Maven Central)")
	cmd.Flags().StringVar(&mavenUsername, "maven-username", "", "Username for Maven repository basic authentication")
	cmd.Flags().StringVar(&mavenPassword, "maven-password", "", "Password for Maven repository basic authentication")
	cmd.Flags().StringArrayVar(&pluginsList, "plugins", nil, "Explicit list of plugins to download as groupId:artifactId:version (space-separated or repeated flag; bypasses API lookup)")
	cmd.Flags().StringArrayVar(&configPaths, "from-config", nil, "Download only the core plugins (storage, secret manager, queue/repository backend) required by one or more Kestra configuration files; requires a version argument")
	return cmd
}

func newPluginsListCommand() *cobra.Command {
	var edition string
	var configPaths []string

	cmd := &cobra.Command{
		Use:   "list <version>",
		Short: "List all compatible plugins for a given Kestra version",
		Long: `List all compatible plugins for a given Kestra version.

Output format matches the legacy npx @kestra-io/kestra-devtools getCompatiblePlugins
command: a single space-separated line of groupId:artifactId:version coordinates.

With --output json the full plugin metadata (groupId, artifactId, license, version)
is printed as a JSON array.

With --from-config, the list is restricted to the "core" plugins required to start
Kestra with the given configuration — the internal storage backend, the secret
manager, and the queue/repository backend. This is the set a standalone worker
needs before its task plugins. Bundled backends (local storage, JDBC, Kafka) emit
no plugin. The output pipes directly into "plugins download --plugins":

  kestractl plugins download 1.3.9 \
    --plugins "$(kestractl plugins list 1.3.9 --from-config /etc/kestra/application.yaml)"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutputFormat(); err != nil {
				return err
			}
			license, err := editionToLicense(edition)
			if err != nil {
				return err
			}
			return runPluginsList(cmd.OutOrStdout(), args[0], license, globalFlags.Output, configPaths)
		},
		Annotations: map[string]string{AnnotationOffline: "true"},
	}

	cmd.Flags().StringVar(&edition, "edition", "ALL", "Edition to list: ALL, OSS (open-source only), or EE (enterprise only)")
	cmd.Flags().StringArrayVar(&configPaths, "from-config", nil, "Derive the required core plugins (storage, secret manager, queue/repository backend) from one or more Kestra configuration files")
	return cmd
}

func runPluginsList(out io.Writer, kestraVersion string, license string, outputFormat string, configPaths []string) error {
	kestraVersion = resolveVersion(kestraVersion)

	var plugins []pluginArtifact
	var err error
	if len(configPaths) > 0 {
		plugins, err = corePluginsFromConfig(configPaths, kestraVersion, license)
	} else {
		plugins, err = fetchPluginList(kestraVersion, license)
	}
	if err != nil {
		return err
	}

	if outputFormat == "json" {
		data, err := json.Marshal(plugins)
		if err != nil {
			return fmt.Errorf("failed to encode plugin list: %w", err)
		}
		fmt.Fprintln(out, string(data))
		return nil
	}

	coords := make([]string, len(plugins))
	for i, p := range plugins {
		coords[i] = p.GroupID + ":" + p.ArtifactID + ":" + p.Version
	}
	fmt.Fprintln(out, strings.Join(coords, " "))
	return nil
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

func runPluginsInstall(out io.Writer, kestraVersion string, pluginsDir string, concurrency int, forceRedownload bool, license string, keepOnlyLastVersion bool, globalTimeout time.Duration, mavenRepository string, mavenUsername string, mavenPassword string, headers []string, explicitPlugins []pluginArtifact) error {
	effectiveMavenBase := pluginsMavenBase
	if mavenRepository != "" {
		effectiveMavenBase = mavenRepository
	}

	parsedHeaders, err := parseHeaders(headers)
	if err != nil {
		return fmt.Errorf("invalid --header value: %w", err)
	}

	var plugins []pluginArtifact
	if len(explicitPlugins) > 0 {
		plugins = explicitPlugins
		fmt.Fprintf(out, "Downloading %d plugin(s)...\n\n", len(plugins))
	} else {
		kestraVersion = resolveVersion(kestraVersion)
		fmt.Fprintf(out, "Fetching plugin list for Kestra %s...\n", kestraVersion)
		plugins, err = fetchPluginList(kestraVersion, license)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "Found %d plugins.\n\n", len(plugins))
	}

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

	type job struct {
		index  int
		plugin pluginArtifact
	}
	jobs := make(chan job, len(plugins))
	for i, p := range plugins {
		jobs <- job{i, p}
	}
	close(jobs)

	results := make(chan downloadResult, len(plugins))

	var wg sync.WaitGroup
	for range concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				select {
				case <-ctx.Done():
					results <- downloadResult{index: j.index, plugin: j.plugin, cancelled: true}
					continue
				default:
				}
				n, skipped, err := downloadJAR(ctx, logf, j.plugin, pluginsDir, forceRedownload, effectiveMavenBase, mavenUsername, mavenPassword, parsedHeaders)
				results <- downloadResult{index: j.index, plugin: j.plugin, bytes: n, err: err, skipped: skipped}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	downloaded := 0
	skippedCount := 0
	cancelledCount := 0
	failed := 0
	stoppedEarly := false

	for r := range results {
		label := fmt.Sprintf("%s:%s:%s", r.plugin.GroupID, r.plugin.ArtifactID, r.plugin.Version)
		outMu.Lock()
		if r.cancelled || errors.Is(r.err, context.Canceled) {
			cancelledCount++
		} else if errors.Is(r.err, errRateLimited) {
			fmt.Fprintf(out, lineFormat+" %s ... FAILED (rate limited by repository — stopping early)\n", r.index+1, len(plugins), label)
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
	if cancelledCount > 0 {
		fmt.Fprintf(out, ", %d not started (cancelled)", cancelledCount)
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
		return fmt.Errorf("download stopped early: repository is rate limiting — %d failed, %d not started", failed, cancelledCount)
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

// parsePluginCoordinates parses a list of strings — each may be a single
// "groupId:artifactId:version" coordinate or a space-separated list of them
// (matching the output of "kestractl plugins list") — into pluginArtifact values.
func parsePluginCoordinates(values []string) ([]pluginArtifact, error) {
	var result []pluginArtifact
	for _, v := range values {
		for _, coord := range strings.Fields(v) {
			parts := strings.SplitN(coord, ":", 3)
			if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
				return nil, fmt.Errorf("invalid plugin coordinate %q: expected groupId:artifactId:version", coord)
			}
			result = append(result, pluginArtifact{GroupID: parts[0], ArtifactID: parts[1], Version: parts[2]})
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("--plugins was set but no valid coordinates were found")
	}
	return result, nil
}

// pluginFileName returns the Kestra-compatible filename for a plugin artifact.
// Format: <groupId>__<artifactId>__<version>.jar, with dots replaced by underscores
// in groupId and version (e.g. io_kestra_plugin__plugin-kafka__1_6_0.jar).
func pluginFileName(p pluginArtifact) string {
	groupID := strings.ReplaceAll(p.GroupID, ".", "_")
	version := strings.ReplaceAll(p.Version, ".", "_")
	return groupID + "__" + p.ArtifactID + "__" + version + ".jar"
}

func downloadJAR(ctx context.Context, logf func(string, ...any), p pluginArtifact, destDir string, forceRedownload bool, mavenBase string, mavenUsername string, mavenPassword string, headers map[string]string) (int64, bool, error) {
	destPath := filepath.Join(destDir, pluginFileName(p))

	if !forceRedownload {
		if _, err := os.Stat(destPath); err == nil {
			return 0, true, nil
		}
	}

	url := mavenJARURL(p, mavenBase)
	label := fmt.Sprintf("%s:%s:%s", p.GroupID, p.ArtifactID, p.Version)

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return 0, false, fmt.Errorf("failed to build request: %w", err)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		if mavenUsername != "" || mavenPassword != "" {
			req.SetBasicAuth(mavenUsername, mavenPassword)
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
			return 0, false, fmt.Errorf("repository returned HTTP %d", resp.StatusCode)
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

// mavenJARURL builds the Maven repository download URL for a plugin artifact.
func mavenJARURL(p pluginArtifact, base string) string {
	groupPath := strings.ReplaceAll(p.GroupID, ".", "/")
	return fmt.Sprintf("%s/%s/%s/%s/%s-%s.jar",
		base, groupPath, p.ArtifactID, p.Version, p.ArtifactID, p.Version)
}
