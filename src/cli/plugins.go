package cli

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
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

// validCoordinatePart matches Maven-style groupId/artifactId/version segments:
// letters, digits, dot, dash, underscore. Nothing else is allowed, so
// URL-significant characters (/, \, ?, #, space, @, ..) are rejected by
var validCoordinatePart = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

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
	cmd.AddCommand(newPluginsGetCommand())
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
	var compatibleFor string

	cmd := &cobra.Command{
		Use:   "download [version]",
		Short: "Download all plugins for a given Kestra version from a Maven repository",
		Long: `Download all plugins for a given Kestra version from a Maven repository.

By default the plugin list is fetched from the Kestra API for the given version.
Alternatively, pass an explicit list of plugins with --plugins, in which case the
version argument is optional and the API is not called. The --plugins format matches
the output of "kestractl plugins list", making it easy to pipe the two commands:

  kestractl plugins download --plugins "$(kestractl plugins list 1.3.9 --edition OSS)"

Coordinates given to --plugins may omit the version (groupId:artifactId). In that
case --compatible-for <kestra-version> — or the version argument, which means the
same thing — resolves each one against the compatibility catalog "plugins list"
prints, so a single plugin can be pinned to a Kestra version in one command:

  kestractl plugins download --compatible-for 2.0.2 --plugins io.kestra.storage:storage-s3

An artifact absent from that version's compatibility set is an error naming it.
A coordinate that pins its own version keeps it, and the ignored --compatible-for
is reported. --edition still filters the catalog used for resolution.

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
			if compatibleFor != "" {
				if len(configPaths) > 0 {
					return fmt.Errorf("--compatible-for and --from-config are mutually exclusive: --from-config already resolves versions from the version argument")
				}
				if len(pluginsList) == 0 {
					return fmt.Errorf("--compatible-for requires --plugins: without it, pass the Kestra version as the argument to download the whole compatibility set")
				}
			}
			version := ""
			if len(args) > 0 {
				version = args[0]
			}
			var explicit []pluginArtifact
			switch {
			case len(pluginsList) > 0:
				explicit, err = resolveDownloadPlugins(cmd.OutOrStdout(), pluginsList, compatibleFor, version, license)
			case len(configPaths) > 0:
				explicit, err = corePluginsFromConfig(configPaths, resolveVersion(version))
			}
			if err != nil {
				return err
			}
			// When --from-config resolves to no plugins (every configured backend is
			// bundled in Kestra), stop here: an empty explicit list would otherwise
			// fall through to downloading the entire catalog.
			if len(configPaths) > 0 && len(explicit) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No core plugins required by the provided configuration (all configured backends are bundled in Kestra).")
				return nil
			}
			headers, _ := cmd.Root().PersistentFlags().GetStringArray(FlagHeader)
			return runPluginsInstall(cmd.OutOrStdout(), version, pluginsDir, concurrency, forceRedownload, license, keepOnlyLastVersion, globalTimeout, mavenRepository, mavenUsername, mavenPassword, headers, explicit)
		},
		Annotations: map[string]string{AnnotationOffline: "true"},
	}

	cmd.Flags().StringVar(&pluginsDir, "plugins-dir", "./plugins", "Directory to write downloaded JARs into")
	cmd.Flags().IntVar(&concurrency, "concurrency", 1, "Number of parallel downloads")
	cmd.Flags().BoolVar(&forceRedownload, "force-redownload", false, "Re-download plugins even if they already exist")
	cmd.Flags().StringVar(&edition, "edition", "ALL", "Edition to download: ALL, OSS (open-source only), or EE (enterprise only)")
	cmd.Flags().BoolVar(&keepOnlyLastVersion, "keep-only-last-version", true, "Remove older versions of each plugin from the plugins directory after downloading")
	cmd.Flags().DurationVar(&globalTimeout, "global-timeout", 5*time.Minute, "Maximum total time allowed for all downloads")
	cmd.Flags().StringVar(&mavenRepository, "maven-repository", "", "Custom Maven repository base URL (defaults to Maven Central)")
	cmd.Flags().StringVar(&mavenUsername, "maven-username", "", "Username for Maven repository basic authentication")
	cmd.Flags().StringVar(&mavenPassword, "maven-password", "", "Password for Maven repository basic authentication")
	cmd.Flags().StringArrayVar(&pluginsList, "plugins", nil, "Explicit list of plugins to download as groupId:artifactId:version (space-separated or repeated flag; bypasses API lookup). The version may be omitted when --compatible-for or a version argument is given")
	cmd.Flags().StringVar(&compatibleFor, "compatible-for", "", "Kestra version to resolve unversioned --plugins coordinates against, using the same compatibility catalog as \"plugins list\"")
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

This queries api.kestra.io's public compatibility catalog for the given <version>
argument — it does not talk to your Kestra server, so --server/KESTRACTL_HOST and
the active auth context have no effect on it. To see what's actually installed on
a running instance, check that instance directly (e.g. its plugins page or API).

Output format matches the legacy npx @kestra-io/kestra-devtools getCompatiblePlugins
command: a single space-separated line of groupId:artifactId:version coordinates.

With --output json the full plugin metadata (groupId, artifactId, license, version)
is printed as a JSON array.

With --from-config, the list is restricted to the "core" plugins required to start
Kestra with the given configuration — the internal storage backend, the secret
manager, and the queue/repository backend. This is the set a standalone worker
needs before its task plugins. Bundled backends (local storage, JDBC, Kafka) emit
no plugin, and --edition is ignored (the exact set is taken from the config). The
output pipes directly into "plugins download --plugins":

  kestractl plugins download 1.3.9 \
    --plugins "$(kestractl plugins list 1.3.9 --from-config /etc/kestra/application.yaml)"

Note: enterprise backends (external secret managers, Elasticsearch/OpenSearch)
are not published to Maven Central — pass --maven-repository (and credentials)
pointing at Kestra's plugin registry to download them.`,
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

func newPluginsGetCommand() *cobra.Command {
	var pluginsDir string
	var forceRedownload bool
	var globalTimeout time.Duration
	var mavenRepository string
	var mavenUsername string
	var mavenPassword string
	var compatibleFor string

	cmd := &cobra.Command{
		Use:          "get <groupId:artifactId[:version]>",
		Short:        "Download a single plugin by Maven coordinates",
		SilenceUsage: true,
		Long: `Download a single plugin JAR by its Maven coordinates (groupId:artifactId:version)
into --plugins-dir, without downloading the full compatibility set for a version.
This lets users install a single plugin into their plugins/ directory without pulling every plugin for a Kestra version.

The version may be omitted (groupId:artifactId) when --compatible-for <kestra-version>
is given: it is then resolved from the same compatibility catalog "plugins list"
prints. An artifact absent from that set is an error naming it.`,
		Example: `  # Download only the Kafka plugin version 1.6.0
  kestractl plugins get io.kestra.plugin:plugin-kafka:1.6.0

  # Download the S3 storage plugin at the version compatible with Kestra 2.0.2
  kestractl plugins get io.kestra.storage:storage-s3 --compatible-for 2.0.2

  # Download an Enterprise Edition (EE) plugin from a custom registry with credentials
  kestractl plugins get io.kestra.ee:ee-plugin:1.6.0 \
    --maven-repository https://registry.kestra.io/maven \
    --maven-username myuser \
    --maven-password mypassword`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			headers, _ := cmd.Root().PersistentFlags().GetStringArray(FlagHeader)
			return runPluginsGet(cmd.OutOrStdout(), args[0], compatibleFor, pluginsDir, forceRedownload, headers, mavenRepository, mavenUsername, mavenPassword, globalTimeout)
		},
		Annotations: map[string]string{AnnotationOffline: "true"},
	}

	cmd.Flags().StringVar(&pluginsDir, "plugins-dir", "./plugins", "Destination directory")
	cmd.Flags().BoolVar(&forceRedownload, "force-redownload", false, "Re-download the plugin even if it already exists")
	cmd.Flags().DurationVar(&globalTimeout, "global-timeout", 5*time.Minute, "Maximum total time allowed for the download")
	cmd.Flags().StringVar(&mavenRepository, "maven-repository", "", "Custom Maven repository base URL (defaults to Maven Central)")
	cmd.Flags().StringVar(&mavenUsername, "maven-username", "", "Username for Maven repository basic authentication")
	cmd.Flags().StringVar(&mavenPassword, "maven-password", "", "Password for Maven repository basic authentication")
	cmd.Flags().StringVar(&compatibleFor, "compatible-for", "", "Kestra version to resolve the coordinate's version against when it is given as groupId:artifactId")

	return cmd
}

func runPluginsList(out io.Writer, kestraVersion string, license string, outputFormat string, configPaths []string) error {
	kestraVersion = resolveVersion(kestraVersion)

	var plugins []pluginArtifact
	var err error
	if len(configPaths) > 0 {
		plugins, err = corePluginsFromConfig(configPaths, kestraVersion)
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
			fmt.Fprintf(out, lineFormat+" %s ... done (%s)\n", r.index+1, len(plugins), label, humanize.Bytes(uint64(r.bytes)))
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

func runPluginsGet(out io.Writer, coordinate string, compatibleFor string, pluginsDir string, forceRedownload bool, headers []string, mavenRepository string, mavenUsername string, mavenPassword string, globalTimeout time.Duration) error {
	p, err := parsePluginCoordinate(coordinate, true)
	if err != nil {
		return err
	}
	resolved, err := resolvePluginVersions(out, []pluginArtifact{p}, compatibleFor, compatibleFor != "", "")
	if err != nil {
		return err
	}
	p = resolved[0]

	effectiveMavenBase := pluginsMavenBase
	if mavenRepository != "" {
		effectiveMavenBase = mavenRepository
	}

	parsedHeaders, err := parseHeaders(headers)
	if err != nil {
		return fmt.Errorf("invalid --header value: %w", err)
	}

	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		return fmt.Errorf("cannot create plugins directory %q: %w", pluginsDir, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), globalTimeout)
	defer cancel()
	logf := func(format string, args ...any) {
		fmt.Fprintf(out, format, args...)
	}

	label := fmt.Sprintf("%s:%s:%s", p.GroupID, p.ArtifactID, p.Version)

	n, skipped, err := downloadJAR(ctx, logf, p, pluginsDir, forceRedownload, effectiveMavenBase, mavenUsername, mavenPassword, parsedHeaders)

	if err != nil {
		return fmt.Errorf("failed to download %s: %w", label, err)
	}
	if skipped {
		fmt.Fprintf(out, "%s ... already up to date\n", label)
	} else {
		fmt.Fprintf(out, "%s ... done (%s)\n", label, humanize.Bytes(uint64(n)))
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

// validateCoordinateVersion returns an error if the version is symbolic alias
// like "latest" or "develop" that cannot be safely resolved to an exact Maven artifact.
func validateCoordinateVersion(version string) error {
	switch strings.ToLower(version) {
	case "latest", "develop":
		return fmt.Errorf("version %q is not supported — please specify an exact version (e.g. 1.2.3)", version)
	default:
		return nil
	}
}

// parsePluginCoordinate parses a single Maven-style plugin coordinate string
// in the format "groupId:artifactId:version" into a pluginArtifact struct.
// It returns an error if the coordinate is malformed or missing required parts.
// When allowUnversioned is set, "groupId:artifactId" is also accepted and yields
// an artifact with an empty Version, to be filled in from the compatibility
// catalogue by resolvePluginVersions.
func parsePluginCoordinate(coord string, allowUnversioned bool) (pluginArtifact, error) {
	expected := "expected groupId:artifactId:version"
	if allowUnversioned {
		expected = "expected groupId:artifactId:version or groupId:artifactId"
	}
	colons := strings.Count(coord, ":")
	if colons != 2 && !(allowUnversioned && colons == 1) {
		return pluginArtifact{}, fmt.Errorf("invalid plugin coordinate %q: %s", coord, expected)
	}
	parts := strings.SplitN(coord, ":", 3)
	for _, part := range parts {
		if part == "" {
			return pluginArtifact{}, fmt.Errorf("invalid plugin coordinate %q: %s", coord, expected)
		}
		if !validCoordinatePart.MatchString(part) {
			return pluginArtifact{}, fmt.Errorf("invalid plugin coordinate %q: parts may only contain letters, digits, '.', '-', or '_'", coord)
		}
	}
	artifact := pluginArtifact{GroupID: parts[0], ArtifactID: parts[1]}
	if len(parts) == 3 {
		if err := validateCoordinateVersion(parts[2]); err != nil {
			return pluginArtifact{}, fmt.Errorf("invalid plugin coordinate %q: %w", coord, err)
		}
		artifact.Version = parts[2]
	}
	return artifact, nil
}

// parsePluginCoordinates parses a list of strings — each may be a single
// "groupId:artifactId:version" coordinate or a space-separated list of them
// (matching the output of "kestractl plugins list") — into pluginArtifact values.
func parsePluginCoordinates(values []string, allowUnversioned bool) ([]pluginArtifact, error) {
	var result []pluginArtifact
	for _, v := range values {
		for _, coord := range strings.Fields(v) {
			artifact, err := parsePluginCoordinate(coord, allowUnversioned)
			if err != nil {
				return nil, err
			}
			result = append(result, artifact)
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("--plugins was set but no valid coordinates were found")
	}
	return result, nil
}

// resolvePluginVersions fills in the version of every coordinate that was given
// without one, from the plugin compatibility catalogue for kestraVersion — the
// same catalogue "kestractl plugins list <version>" prints.
//
// A coordinate that already pins a version is left untouched: the explicit
// version wins. When the Kestra version came from an explicit --compatible-for
// (compatibleForExplicit), that override is reported on warnOut, so it is never
// silent. Piping "plugins list" output into --plugins alongside a version
// argument is the same shape and stays quiet.
//
// An artifact that is absent from the compatibility set is an error naming it —
// that is the whole point over grepping the list by hand.
func resolvePluginVersions(warnOut io.Writer, coords []pluginArtifact, kestraVersion string, compatibleForExplicit bool, license string) ([]pluginArtifact, error) {
	needsCatalog := false
	for _, c := range coords {
		if c.Version == "" {
			needsCatalog = true
			break
		}
	}

	if needsCatalog && kestraVersion == "" {
		for _, c := range coords {
			if c.Version == "" {
				return nil, fmt.Errorf("plugin coordinate %s:%s has no version — pass --compatible-for <kestra-version> to resolve it from the compatibility catalog, or give the full groupId:artifactId:version", c.GroupID, c.ArtifactID)
			}
		}
	}

	var catalog map[string]string
	if needsCatalog {
		resolved := resolveVersion(kestraVersion)
		plugins, err := fetchPluginList(resolved, license)
		if err != nil {
			return nil, err
		}
		catalog = make(map[string]string, len(plugins))
		for _, p := range plugins {
			key := p.GroupID + ":" + p.ArtifactID
			if _, seen := catalog[key]; !seen {
				catalog[key] = p.Version
			}
		}
	}

	out := make([]pluginArtifact, len(coords))
	copy(out, coords)
	for i, c := range out {
		key := c.GroupID + ":" + c.ArtifactID
		if c.Version != "" {
			if compatibleForExplicit {
				fmt.Fprintf(warnOut, "[warn] %s pins version %s explicitly — --compatible-for %s is not applied to it\n", key, c.Version, kestraVersion)
			}
			continue
		}
		version, ok := catalog[key]
		if !ok {
			return nil, fmt.Errorf("plugin %s is not in the compatibility set for Kestra %s — check the artifact name with \"kestractl plugins list %s\"", key, kestraVersion, kestraVersion)
		}
		out[i].Version = version
	}
	return out, nil
}

// resolveDownloadPlugins parses the --plugins values of "plugins download" and
// resolves any unversioned coordinate against the compatibility catalog for
// compatibleFor, falling back to the positional version argument.
func resolveDownloadPlugins(warnOut io.Writer, values []string, compatibleFor string, versionArg string, license string) ([]pluginArtifact, error) {
	if compatibleFor != "" && versionArg != "" && resolveVersion(compatibleFor) != resolveVersion(versionArg) {
		return nil, fmt.Errorf("conflicting Kestra versions: --compatible-for %s and version argument %s — pass only one", compatibleFor, versionArg)
	}
	coords, err := parsePluginCoordinates(values, true)
	if err != nil {
		return nil, err
	}
	kestraVersion := compatibleFor
	if kestraVersion == "" {
		kestraVersion = versionArg
	}
	return resolvePluginVersions(warnOut, coords, kestraVersion, compatibleFor != "", license)
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
	url := mavenJARURL(p, mavenBase)
	label := fmt.Sprintf("%s:%s:%s", p.GroupID, p.ArtifactID, p.Version)

	if !forceRedownload {
		if _, err := os.Stat(destPath); err == nil {
			return 0, true, nil
		}
	}
	expectedSHA1, err := fetchExpectedSHA1(ctx, logf, url, label, mavenUsername, mavenPassword, headers)
	if err != nil {
		if errors.Is(err, errRateLimited) {
			return 0, false, err
		}
		logf("  [WARN] failed to fetch expected SHA-1 for %s: %v, proceeding without checksum validation\n", label, err)
		expectedSHA1 = ""
	} else if expectedSHA1 != "" {
		if len(expectedSHA1) != 40 {
			logf("  [WARN] expected SHA-1 for %s has invalid length (%d), proceeding without checksum validation\n", label, len(expectedSHA1))
			expectedSHA1 = ""
		} else if _, hexErr := hex.DecodeString(expectedSHA1); hexErr != nil {
			logf("  [WARN] expected SHA-1 for %s is not valid hex, proceeding without checksum validation\n", label)
			expectedSHA1 = ""
		}
	}

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
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			if resp.StatusCode == http.StatusNotFound {
				return 0, false, fmt.Errorf("artifact not found at %s (HTTP 404), check that the groupId, artifactId and version exist on the repository", url)
			}
			return 0, false, fmt.Errorf("repository returned HTTP %d", resp.StatusCode)
		}

		f, err := os.CreateTemp(destDir, filepath.Base(destPath)+".part*")
		if err != nil {
			resp.Body.Close()
			return 0, false, fmt.Errorf("cannot create temp file: %w", err)
		}
		partPath := f.Name()
		// os.CreateTemp creates files with mode 0600; make the finished JAR
		// readable by any user/process (e.g. Kestra running as another user).
		if err := os.Chmod(partPath, 0o644); err != nil {
			resp.Body.Close()
			f.Close()
			os.Remove(partPath)
			return 0, false, fmt.Errorf("failed to set permissions on temp file: %w", err)
		}

		n, copyErr := io.Copy(f, resp.Body)
		resp.Body.Close()
		closeErr := f.Close()
		if copyErr != nil {
			os.Remove(partPath)
			return 0, false, fmt.Errorf("write failed: %w", copyErr)
		}
		if closeErr != nil {
			os.Remove(partPath)
			return 0, false, fmt.Errorf("failed to close file: %w", closeErr)
		}
		if err := os.Rename(partPath, destPath); err != nil {
			os.Remove(partPath)
			return 0, false, fmt.Errorf("failed to finalize download (rename error): %w", err)
		}

		if expectedSHA1 != "" {
			actualSHA1, hashErr := fileSHA1(destPath)
			if hashErr != nil {
				os.Remove(destPath)
				return 0, false, fmt.Errorf("failed to compute SHA-1 of downloaded file: %w", hashErr)
			}
			if actualSHA1 != expectedSHA1 {
				os.Remove(destPath)
				return 0, false, fmt.Errorf("checksum mismatch for %s: expected %s, got %s", label, expectedSHA1, actualSHA1)
			}
		}
		return n, false, nil
	}
}

// fileSHA1 computes and returns the hex-encoded SHA-1 checksum of the file at the
// given path.
func fileSHA1(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("cannot open file: %w", err)
	}
	defer f.Close()
	hasher := sha1.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", fmt.Errorf("cannot read file: %w", err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// fetchExpectedSHA1 retrieves the expected SHA-1 checksum for a given Maven artifact URL.
// It executes a retriable HTTP request using the provided credentials and headers and limits the
// response read to 256 bytes, handles both types of hash files
// (just the plain hash, or the hash followed by a filename)
func fetchExpectedSHA1(ctx context.Context, logf func(string, ...any), jarURL string, label string, mavenUsername string, mavenPassword string, headers map[string]string) (string, error) {
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, jarURL+".sha1", nil)
		if err != nil {
			return "", fmt.Errorf("failed to build checksum request: %w", err)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		if mavenUsername != "" || mavenPassword != "" {
			req.SetBasicAuth(mavenUsername, mavenPassword)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("failed to reach repository for checksum: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			if attempt >= len(rateLimitWaits) {
				return "", errRateLimited
			}
			wait := rateLimitWaits[attempt]
			logf("  [429] rate limited on %s (SHA-1) — waiting %s (retry %d/%d)\n", label, wait, attempt+1, len(rateLimitWaits))
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return "", ctx.Err()
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			if resp.StatusCode == http.StatusNotFound {
				logf("  [warn] no .sha1 published for %s — skipping checksum verification\n", label)
				return "", nil
			}
			return "", fmt.Errorf("checksum file returned HTTP %d at %s.sha1", resp.StatusCode, jarURL)
		}

		body, err := io.ReadAll(io.LimitReader(resp.Body, 256))
		resp.Body.Close()
		if err != nil {
			return "", fmt.Errorf("failed to read checksum file: %w", err)
		}

		fields := strings.Fields(string(body))
		if len(fields) == 0 {
			return "", fmt.Errorf("checksum file is empty")
		}

		return strings.ToLower(fields[0]), nil
	}
}

// mavenJARURL builds the Maven repository download URL for a plugin artifact.
func mavenJARURL(p pluginArtifact, base string) string {
	groupPath := strings.ReplaceAll(p.GroupID, ".", "/")
	return fmt.Sprintf("%s/%s/%s/%s/%s-%s.jar",
		base, groupPath, p.ArtifactID, p.Version, p.ArtifactID, p.Version)
}
