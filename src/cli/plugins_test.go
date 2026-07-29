package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Real-world payload sampled from
// GET https://api.kestra.io/v1/plugins/artifacts/core-compatibility/1.3.9/latest
const pluginListPayload = `[
  {"groupId":"io.kestra.storage","artifactId":"storage-gcs","license":"OPEN_SOURCE","version":"1.1.4"},
  {"groupId":"io.kestra.plugin","artifactId":"plugin-jdbc-oracle","license":"OPEN_SOURCE","version":"1.10.1"},
  {"groupId":"io.kestra.plugin","artifactId":"plugin-kafka","license":"OPEN_SOURCE","version":"1.6.0"},
  {"groupId":"io.kestra.plugin","artifactId":"plugin-docker","license":"OPEN_SOURCE","version":"1.3.0"},
  {"groupId":"io.kestra.plugin","artifactId":"plugin-jdbc-postgres","license":"OPEN_SOURCE","version":"1.10.1"},
  {"groupId":"io.kestra.plugin","artifactId":"plugin-airbyte","license":"OPEN_SOURCE","version":"1.3.1"},
  {"groupId":"io.kestra.plugin","artifactId":"plugin-slack","license":"OPEN_SOURCE","version":"1.3.0"},
  {"groupId":"io.kestra.plugin","artifactId":"plugin-azure","license":"OPEN_SOURCE","version":"2.2.1"},
  {"groupId":"io.kestra.plugin","artifactId":"plugin-jdbc-as400","license":"OPEN_SOURCE","version":"1.10.1"},
  {"groupId":"io.kestra.plugin","artifactId":"plugin-jdbc-trino","license":"OPEN_SOURCE","version":"1.10.1"},
  {"groupId":"io.kestra.plugin","artifactId":"plugin-gcp","license":"OPEN_SOURCE","version":"2.2.1"},
  {"groupId":"io.kestra.plugin","artifactId":"plugin-cloudquery","license":"OPEN_SOURCE","version":"1.2.1"},
  {"groupId":"io.kestra.plugin","artifactId":"plugin-kubernetes","license":"OPEN_SOURCE","version":"1.8.0"},
  {"groupId":"io.kestra.plugin","artifactId":"plugin-aws","license":"OPEN_SOURCE","version":"2.2.0"},
  {"groupId":"io.kestra.plugin","artifactId":"plugin-telegram","license":"OPEN_SOURCE","version":"1.1.0"},
  {"groupId":"io.kestra.plugin","artifactId":"plugin-tencent","license":"OPEN_SOURCE","version":"1.1.0"},
  {"groupId":"io.kestra.plugin","artifactId":"plugin-dbt","license":"OPEN_SOURCE","version":"1.5.0"},
  {"groupId":"io.kestra.plugin","artifactId":"plugin-scripts","license":"OPEN_SOURCE","version":"1.8.0"},
  {"groupId":"io.kestra.plugin","artifactId":"plugin-weaviate","license":"OPEN_SOURCE","version":"1.1.0"},
  {"groupId":"io.kestra.plugin","artifactId":"plugin-openai","license":"OPEN_SOURCE","version":"1.3.0"},
  {"groupId":"io.kestra.plugin.ee","artifactId":"plugin-ee-core","license":"ENTERPRISE","version":"1.3.9"},
  {"groupId":"io.kestra.plugin.ee","artifactId":"plugin-ee-jdbc","license":"ENTERPRISE","version":"1.3.9"}
]`

// mockJARBody is the dummy JAR content returned by the mock Maven server.
const mockJARBody = "PK\x03\x04"

// newMockServers sets up:
//   - apiServer: returns pluginListPayload for any request
//   - mavenServer: returns a dummy JAR body for .jar requests and its SHA-1 for .sha1 requests
//
// It overrides pluginsAPIBase and pluginsMavenBase and restores them via t.Cleanup.
func newMockServers(t *testing.T, apiStatus int, apiBody string) (apiServer, mavenServer *httptest.Server) {
	t.Helper()

	apiServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(apiStatus)
		fmt.Fprint(w, apiBody)
	}))

	mavenServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/java-archive")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, mockJARBody)
	}))

	origAPI := pluginsAPIBase
	origMaven := pluginsMavenBase
	pluginsAPIBase = apiServer.URL
	pluginsMavenBase = mavenServer.URL

	t.Cleanup(func() {
		pluginsAPIBase = origAPI
		pluginsMavenBase = origMaven
		apiServer.Close()
		mavenServer.Close()
	})

	return apiServer, mavenServer
}

func TestRunPluginsInstall_HappyPath(t *testing.T) {
	newMockServers(t, http.StatusOK, pluginListPayload)

	tmpDir := t.TempDir()
	var out bytes.Buffer

	err := runPluginsInstall(&out, "1.3.9", tmpDir, 1, false, "", false, 5*time.Minute, "", "", "", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()

	if !strings.Contains(output, "Found 22 plugins") {
		t.Errorf("expected 'Found 22 plugins' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Downloaded 22") {
		t.Errorf("expected 'Downloaded 22' in output, got:\n%s", output)
	}

	// Verify JAR files were written to disk.
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to read plugins dir: %v", err)
	}
	if len(entries) != 22 {
		t.Errorf("expected 22 JAR files, got %d", len(entries))
	}

	// Spot-check one file from each group.
	for _, name := range []string{
		"io_kestra_storage__storage-gcs__1_1_4.jar",
		"io_kestra_plugin__plugin-kafka__1_6_0.jar",
		"io_kestra_plugin__plugin-jdbc-postgres__1_10_1.jar",
		"io_kestra_plugin_ee__plugin-ee-core__1_3_9.jar",
	} {
		path := filepath.Join(tmpDir, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected file %s to exist", name)
		}
	}
}

func TestRunPluginsInstall_ConcurrentDownloads(t *testing.T) {
	newMockServers(t, http.StatusOK, pluginListPayload)

	tmpDir := t.TempDir()
	var out bytes.Buffer

	err := runPluginsInstall(&out, "1.3.9", tmpDir, 4, false, "", false, 5*time.Minute, "", "", "", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error with concurrency=4: %v", err)
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to read plugins dir: %v", err)
	}
	if len(entries) != 22 {
		t.Errorf("expected 22 JAR files with concurrency=4, got %d", len(entries))
	}
}

func TestRunPluginsInstall_APINotFound(t *testing.T) {
	newMockServers(t, http.StatusNotFound, `{"message":"version not found"}`)

	var out bytes.Buffer
	err := runPluginsInstall(&out, "99.99.99", t.TempDir(), 1, false, "", false, 5*time.Minute, "", "", "", nil, nil)
	if err == nil {
		t.Fatal("expected error for HTTP 404, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("expected HTTP 404 in error, got: %v", err)
	}
}

func TestRunPluginsInstall_APIInvalidJSON(t *testing.T) {
	newMockServers(t, http.StatusOK, `not-json`)

	var out bytes.Buffer
	err := runPluginsInstall(&out, "1.3.9", t.TempDir(), 1, false, "", false, 5*time.Minute, "", "", "", nil, nil)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), "failed to parse plugin list") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunPluginsInstall_MavenDownloadFailure(t *testing.T) {
	// API returns one plugin; Maven returns 404 for all downloads.
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `[{"groupId":"io.kestra.plugin","artifactId":"plugin-kafka","license":"OPEN_SOURCE","version":"1.6.0"}]`)
	}))
	mavenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	origAPI := pluginsAPIBase
	origMaven := pluginsMavenBase
	pluginsAPIBase = apiServer.URL
	pluginsMavenBase = mavenServer.URL
	t.Cleanup(func() {
		pluginsAPIBase = origAPI
		pluginsMavenBase = origMaven
		apiServer.Close()
		mavenServer.Close()
	})

	var out bytes.Buffer
	err := runPluginsInstall(&out, "1.3.9", t.TempDir(), 1, false, "", false, 5*time.Minute, "", "", "", nil, nil)
	if err == nil {
		t.Fatal("expected error when Maven returns 404, got nil")
	}
	if !strings.Contains(err.Error(), "1 plugin(s) failed") {
		t.Errorf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "FAILED") {
		t.Errorf("expected 'FAILED' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "1 failed") {
		t.Errorf("expected '1 failed' summary in output, got:\n%s", output)
	}
}

func TestPluginFileName(t *testing.T) {
	tests := []struct {
		plugin   pluginArtifact
		wantName string
	}{
		{
			plugin:   pluginArtifact{GroupID: "io.kestra.plugin", ArtifactID: "plugin-kafka", Version: "1.6.0"},
			wantName: "io_kestra_plugin__plugin-kafka__1_6_0.jar",
		},
		{
			plugin:   pluginArtifact{GroupID: "io.kestra.storage", ArtifactID: "storage-gcs", Version: "1.1.4"},
			wantName: "io_kestra_storage__storage-gcs__1_1_4.jar",
		},
		{
			plugin:   pluginArtifact{GroupID: "io.kestra.plugin.ee", ArtifactID: "plugin-ee-core", Version: "1.3.9"},
			wantName: "io_kestra_plugin_ee__plugin-ee-core__1_3_9.jar",
		},
		{
			plugin:   pluginArtifact{GroupID: "io.kestra.plugin", ArtifactID: "plugin-serdes", Version: "0.20.0-SNAPSHOT"},
			wantName: "io_kestra_plugin__plugin-serdes__0_20_0-SNAPSHOT.jar",
		},
	}
	for _, tt := range tests {
		got := pluginFileName(tt.plugin)
		if got != tt.wantName {
			t.Errorf("pluginFileName(%+v)\n  got:  %s\n  want: %s", tt.plugin, got, tt.wantName)
		}
	}
}

func TestMavenJARURL(t *testing.T) {
	const base = "https://repo1.maven.org/maven2"

	tests := []struct {
		plugin  pluginArtifact
		wantURL string
	}{
		{
			plugin:  pluginArtifact{GroupID: "io.kestra.plugin", ArtifactID: "plugin-kafka", Version: "1.6.0"},
			wantURL: "https://repo1.maven.org/maven2/io/kestra/plugin/plugin-kafka/1.6.0/plugin-kafka-1.6.0.jar",
		},
		{
			plugin:  pluginArtifact{GroupID: "io.kestra.storage", ArtifactID: "storage-gcs", Version: "1.1.4"},
			wantURL: "https://repo1.maven.org/maven2/io/kestra/storage/storage-gcs/1.1.4/storage-gcs-1.1.4.jar",
		},
		{
			plugin:  pluginArtifact{GroupID: "io.kestra.plugin.ee", ArtifactID: "plugin-ee-core", Version: "1.3.9"},
			wantURL: "https://repo1.maven.org/maven2/io/kestra/plugin/ee/plugin-ee-core/1.3.9/plugin-ee-core-1.3.9.jar",
		},
	}

	for _, tt := range tests {
		got := mavenJARURL(tt.plugin, base)
		if got != tt.wantURL {
			t.Errorf("mavenJARURL(%+v)\n  got:  %s\n  want: %s", tt.plugin, got, tt.wantURL)
		}
	}
}

func TestPluginsDownloadCommand_Flags(t *testing.T) {
	cmd := newPluginsDownloadCommand()

	for _, flag := range []string{"plugins-dir", "concurrency", "force-redownload", "edition", "keep-only-last-version"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag --%s to exist", flag)
		}
	}

	// --keep-only-last-version must default to true.
	f := cmd.Flags().Lookup("keep-only-last-version")
	if f.DefValue != "true" {
		t.Errorf("expected --keep-only-last-version default to be true, got %q", f.DefValue)
	}
}

func TestResolveVersion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"develop", "999.999.999"},
		{"DEVELOP", "999.999.999"},
		{"latest", "999.999.999"},
		{"LATEST", "999.999.999"},
		{"1.3.9", "1.3.9"},
		{"2.0.0", "2.0.0"},
	}
	for _, tt := range tests {
		got := resolveVersion(tt.input)
		if got != tt.want {
			t.Errorf("resolveVersion(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestPluginsDownloadCommand_RequiresVersion(t *testing.T) {
	cmd := newPluginsDownloadCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when version argument is missing")
	}
	if !strings.Contains(err.Error(), "version argument") {
		t.Errorf("unexpected error: %v", err)
	}
}

// singlePluginPayload is a minimal plugin list used for skip/force tests.
const singlePluginPayload = `[{"groupId":"io.kestra.plugin","artifactId":"plugin-kafka","license":"OPEN_SOURCE","version":"1.6.0"}]`

func TestRunPluginsInstall_SkipsExistingValidJAR(t *testing.T) {
	newMockServers(t, http.StatusOK, singlePluginPayload)

	tmpDir := t.TempDir()

	// Pre-create the JAR with the same content the mock Maven server serves.
	jarPath := filepath.Join(tmpDir, "io_kestra_plugin__plugin-kafka__1_6_0.jar")
	if err := os.WriteFile(jarPath, []byte(mockJARBody), 0o644); err != nil {
		t.Fatalf("failed to create pre-existing JAR: %v", err)
	}

	var out bytes.Buffer
	if err := runPluginsInstall(&out, "1.3.9", tmpDir, 1, false, "", false, 5*time.Minute, "", "", "", nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "already up to date") {
		t.Errorf("expected 'already up to date' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "skipped 1") {
		t.Errorf("expected 'skipped 1' in output, got:\n%s", output)
	}
	if strings.Contains(output, "Downloaded 1") {
		t.Errorf("did not expect 'Downloaded 1' when plugin is up to date, got:\n%s", output)
	}
}

func TestRunPluginsInstall_ForceRedownloadsExistingJAR(t *testing.T) {
	newMockServers(t, http.StatusOK, singlePluginPayload)

	tmpDir := t.TempDir()

	// Pre-create the JAR with matching content.
	jarPath := filepath.Join(tmpDir, "io_kestra_plugin__plugin-kafka__1_6_0.jar")
	if err := os.WriteFile(jarPath, []byte(mockJARBody), 0o644); err != nil {
		t.Fatalf("failed to create pre-existing JAR: %v", err)
	}

	var out bytes.Buffer
	if err := runPluginsInstall(&out, "1.3.9", tmpDir, 1, true, "", false, 5*time.Minute, "", "", "", nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if strings.Contains(output, "already up to date") {
		t.Errorf("did not expect 'already up to date' with --force-redownload, got:\n%s", output)
	}
	if !strings.Contains(output, "Downloaded 1") {
		t.Errorf("expected 'Downloaded 1' with --force-redownload, got:\n%s", output)
	}
}

func TestEditionToLicense(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"ALL", "", false},
		{"all", "", false},
		{"OSS", "OPEN_SOURCE", false},
		{"oss", "OPEN_SOURCE", false},
		{"EE", "ENTERPRISE", false},
		{"ee", "ENTERPRISE", false},
		{"INVALID", "", true},
		{"CE", "", true},
	}
	for _, tt := range tests {
		got, err := editionToLicense(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("editionToLicense(%q): expected error, got nil", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("editionToLicense(%q): unexpected error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("editionToLicense(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFetchPluginList_LicenseQueryParam(t *testing.T) {
	var capturedQuery string
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `[]`)
	}))
	t.Cleanup(apiServer.Close)

	orig := pluginsAPIBase
	pluginsAPIBase = apiServer.URL
	t.Cleanup(func() { pluginsAPIBase = orig })

	tests := []struct {
		license   string
		wantQuery string
	}{
		{"", ""},
		{"OPEN_SOURCE", "license=OPEN_SOURCE"},
		{"ENTERPRISE", "license=ENTERPRISE"},
	}
	for _, tt := range tests {
		capturedQuery = ""
		if _, err := fetchPluginList("1.3.9", tt.license); err != nil {
			t.Fatalf("fetchPluginList(%q): unexpected error: %v", tt.license, err)
		}
		if capturedQuery != tt.wantQuery {
			t.Errorf("fetchPluginList license=%q: query = %q, want %q", tt.license, capturedQuery, tt.wantQuery)
		}
	}
}

func TestRunPluginsInstall_EditionOSS(t *testing.T) {
	// API returns only OSS plugins (simulating a ?license=OPEN_SOURCE filtered response).
	ossPayload := `[
	  {"groupId":"io.kestra.plugin","artifactId":"plugin-kafka","license":"OPEN_SOURCE","version":"1.6.0"},
	  {"groupId":"io.kestra.plugin","artifactId":"plugin-docker","license":"OPEN_SOURCE","version":"1.3.0"}
	]`
	newMockServers(t, http.StatusOK, ossPayload)

	var out bytes.Buffer
	if err := runPluginsInstall(&out, "1.3.9", t.TempDir(), 1, false, "OPEN_SOURCE", false, 5*time.Minute, "", "", "", nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Found 2 plugins") {
		t.Errorf("expected 'Found 2 plugins' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Downloaded 2") {
		t.Errorf("expected 'Downloaded 2' in output, got:\n%s", output)
	}
}

func TestRunPluginsInstall_EditionEE(t *testing.T) {
	// API returns only EE plugins (simulating a ?license=ENTERPRISE filtered response).
	eePayload := `[
	  {"groupId":"io.kestra.plugin.ee","artifactId":"plugin-ee-core","license":"ENTERPRISE","version":"1.3.9"}
	]`
	newMockServers(t, http.StatusOK, eePayload)

	var out bytes.Buffer
	if err := runPluginsInstall(&out, "1.3.9", t.TempDir(), 1, false, "ENTERPRISE", false, 5*time.Minute, "", "", "", nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Found 1 plugins") {
		t.Errorf("expected 'Found 1 plugins' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Downloaded 1") {
		t.Errorf("expected 'Downloaded 1' in output, got:\n%s", output)
	}
}

func TestRunPluginsInstall_KeepOnlyLastVersion_RemovesOldVersions(t *testing.T) {
	newMockServers(t, http.StatusOK, singlePluginPayload) // serves plugin-kafka 1.6.0

	tmpDir := t.TempDir()

	// Pre-place two old versions of the same plugin in the directory.
	for _, oldName := range []string{
		"io_kestra_plugin__plugin-kafka__1_5_0.jar",
		"io_kestra_plugin__plugin-kafka__1_4_0.jar",
	} {
		if err := os.WriteFile(filepath.Join(tmpDir, oldName), []byte(mockJARBody), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	// Also put an unrelated JAR that must NOT be removed.
	unrelated := "io_kestra_plugin__plugin-docker__1_3_0.jar"
	if err := os.WriteFile(filepath.Join(tmpDir, unrelated), []byte(mockJARBody), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	var out bytes.Buffer
	if err := runPluginsInstall(&out, "1.3.9", tmpDir, 1, false, "", true, 5*time.Minute, "", "", "", nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Removed 2 old version(s)") {
		t.Errorf("expected 'Removed 2 old version(s)' in output, got:\n%s", output)
	}

	// Old versions must be gone.
	for _, oldName := range []string{
		"io_kestra_plugin__plugin-kafka__1_5_0.jar",
		"io_kestra_plugin__plugin-kafka__1_4_0.jar",
	} {
		if _, err := os.Stat(filepath.Join(tmpDir, oldName)); !os.IsNotExist(err) {
			t.Errorf("expected old version %s to be removed", oldName)
		}
	}

	// Current version must still exist.
	if _, err := os.Stat(filepath.Join(tmpDir, "io_kestra_plugin__plugin-kafka__1_6_0.jar")); os.IsNotExist(err) {
		t.Error("expected current version io_kestra_plugin__plugin-kafka__1_6_0.jar to still exist")
	}

	// Unrelated plugin must still exist.
	if _, err := os.Stat(filepath.Join(tmpDir, unrelated)); os.IsNotExist(err) {
		t.Errorf("expected unrelated plugin %s to still exist", unrelated)
	}
}

func TestRunPluginsInstall_KeepOnlyLastVersionFalse_LeavesOldVersions(t *testing.T) {
	newMockServers(t, http.StatusOK, singlePluginPayload)

	tmpDir := t.TempDir()

	oldName := "io_kestra_plugin__plugin-kafka__1_5_0.jar"
	if err := os.WriteFile(filepath.Join(tmpDir, oldName), []byte(mockJARBody), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	var out bytes.Buffer
	if err := runPluginsInstall(&out, "1.3.9", tmpDir, 1, false, "", false, 5*time.Minute, "", "", "", nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Old version must still be present.
	if _, err := os.Stat(filepath.Join(tmpDir, oldName)); os.IsNotExist(err) {
		t.Errorf("expected old version %s to remain when --keep-only-last-version=false", oldName)
	}

	output := out.String()
	if strings.Contains(output, "Removed") {
		t.Errorf("did not expect 'Removed' in output when flag is false, got:\n%s", output)
	}
}

func TestPruneOldVersions(t *testing.T) {
	tmpDir := t.TempDir()

	current := []pluginArtifact{
		{GroupID: "io.kestra.plugin", ArtifactID: "plugin-kafka", Version: "1.6.0"},
		{GroupID: "io.kestra.plugin", ArtifactID: "plugin-docker", Version: "1.3.0"},
	}

	// Create current-version files (should be kept).
	for _, p := range current {
		if err := os.WriteFile(filepath.Join(tmpDir, pluginFileName(p)), []byte("jar"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	// Create old-version files (should be removed).
	oldFiles := []string{
		"io_kestra_plugin__plugin-kafka__1_5_0.jar",
		"io_kestra_plugin__plugin-docker__1_2_0.jar",
	}
	for _, name := range oldFiles {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte("jar"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	// Create a JAR for a plugin not in the current list (should be kept).
	unknown := "io_kestra_plugin__plugin-unknown__9_9_9.jar"
	if err := os.WriteFile(filepath.Join(tmpDir, unknown), []byte("jar"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	removed, err := pruneOldVersions(tmpDir, current)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if removed != 2 {
		t.Errorf("expected 2 files removed, got %d", removed)
	}

	for _, name := range oldFiles {
		if _, err := os.Stat(filepath.Join(tmpDir, name)); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed", name)
		}
	}
	for _, p := range current {
		if _, err := os.Stat(filepath.Join(tmpDir, pluginFileName(p))); os.IsNotExist(err) {
			t.Errorf("expected current file %s to remain", pluginFileName(p))
		}
	}
	if _, err := os.Stat(filepath.Join(tmpDir, unknown)); os.IsNotExist(err) {
		t.Errorf("expected unknown plugin %s to remain", unknown)
	}
}

func TestRunPluginsInstall_429RetryThenSucceeds(t *testing.T) {
	// Override waits to zero so the test doesn't actually sleep.
	orig := rateLimitWaits
	rateLimitWaits = []time.Duration{0, 0, 0}
	t.Cleanup(func() { rateLimitWaits = orig })

	var attempts atomic.Int32
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, singlePluginPayload)
	}))
	mavenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// First two attempts return 429; third succeeds.
		if attempts.Add(1) <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/java-archive")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, mockJARBody)
	}))
	origAPI, origMaven := pluginsAPIBase, pluginsMavenBase
	pluginsAPIBase, pluginsMavenBase = apiServer.URL, mavenServer.URL
	t.Cleanup(func() {
		pluginsAPIBase, pluginsMavenBase = origAPI, origMaven
		apiServer.Close()
		mavenServer.Close()
	})

	var out bytes.Buffer
	if err := runPluginsInstall(&out, "1.3.9", t.TempDir(), 1, false, "", false, 5*time.Minute, "", "", "", nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "[429]") {
		t.Errorf("expected '[429]' retry log in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Downloaded 1") {
		t.Errorf("expected 'Downloaded 1' after retry succeeded, got:\n%s", output)
	}
	if attempts.Load() != 3 {
		t.Errorf("expected 3 Maven attempts, got %d", attempts.Load())
	}
}

func TestRunPluginsInstall_429ExhaustsRetriesStopsEarly(t *testing.T) {
	orig := rateLimitWaits
	rateLimitWaits = []time.Duration{0, 0, 0}
	t.Cleanup(func() { rateLimitWaits = orig })

	newMockServers(t, http.StatusOK, singlePluginPayload)

	// Override Maven to always return 429.
	mavenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	origMaven := pluginsMavenBase
	pluginsMavenBase = mavenServer.URL
	t.Cleanup(func() {
		pluginsMavenBase = origMaven
		mavenServer.Close()
	})

	var out bytes.Buffer
	err := runPluginsInstall(&out, "1.3.9", t.TempDir(), 1, false, "", false, 5*time.Minute, "", "", "", nil, nil)
	if err == nil {
		t.Fatal("expected error when Maven always returns 429")
	}
	if !strings.Contains(err.Error(), "stopped early") {
		t.Errorf("expected 'stopped early' in error, got: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "rate limited by repository — stopping early") {
		t.Errorf("expected early-stop message in output, got:\n%s", output)
	}
}

func TestPluginsDownloadCommand_GlobalTimeoutFlag(t *testing.T) {
	cmd := newPluginsDownloadCommand()
	f := cmd.Flags().Lookup("global-timeout")
	if f == nil {
		t.Fatal("expected --global-timeout flag to exist")
	}
	if f.DefValue != "5m0s" {
		t.Errorf("expected --global-timeout default to be 5m0s, got %q", f.DefValue)
	}
}

func TestPluginsDownloadCommand_MavenRepositoryFlags(t *testing.T) {
	cmd := newPluginsDownloadCommand()

	for _, name := range []string{"maven-repository", "maven-username", "maven-password"} {
		f := cmd.Flags().Lookup(name)
		if f == nil {
			t.Errorf("expected --%s flag to exist", name)
			continue
		}
		if f.DefValue != "" {
			t.Errorf("expected --%s default to be empty, got %q", name, f.DefValue)
		}
	}
}

func TestRunPluginsInstall_CustomMavenRepository(t *testing.T) {
	// Set up a custom Maven server that records the host it was called on.
	var capturedHost string
	customMaven := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHost = r.Host
		w.Header().Set("Content-Type", "application/java-archive")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, mockJARBody)
	}))
	t.Cleanup(customMaven.Close)

	// API server uses the global override so newMockServers handles cleanup for it.
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, singlePluginPayload)
	}))
	t.Cleanup(apiServer.Close)

	origAPI := pluginsAPIBase
	pluginsAPIBase = apiServer.URL
	t.Cleanup(func() { pluginsAPIBase = origAPI })

	var out bytes.Buffer
	if err := runPluginsInstall(&out, "1.3.9", t.TempDir(), 1, false, "", false, 5*time.Minute, customMaven.URL, "", "", nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedHost == "" {
		t.Error("expected custom Maven server to be called, but it was not")
	}
	if !strings.Contains(out.String(), "Downloaded 1") {
		t.Errorf("expected 'Downloaded 1' in output, got:\n%s", out.String())
	}
}

func TestRunPluginsInstall_MavenBasicAuth(t *testing.T) {
	var capturedAuth string
	customMaven := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/java-archive")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, mockJARBody)
	}))
	t.Cleanup(customMaven.Close)

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, singlePluginPayload)
	}))
	t.Cleanup(apiServer.Close)

	origAPI := pluginsAPIBase
	pluginsAPIBase = apiServer.URL
	t.Cleanup(func() { pluginsAPIBase = origAPI })

	var out bytes.Buffer
	if err := runPluginsInstall(&out, "1.3.9", t.TempDir(), 1, false, "", false, 5*time.Minute, customMaven.URL, "alice", "s3cr3t", nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedAuth == "" {
		t.Fatal("expected Authorization header to be set, got none")
	}
	if !strings.HasPrefix(capturedAuth, "Basic ") {
		t.Errorf("expected Basic auth header, got: %s", capturedAuth)
	}
}

func TestRunPluginsList_DefaultOutput(t *testing.T) {
	newMockServers(t, http.StatusOK, pluginListPayload)

	var out bytes.Buffer
	if err := runPluginsList(&out, "1.3.9", "", "table", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := strings.TrimSpace(out.String())

	// Output must be a single line.
	if strings.Contains(output, "\n") {
		t.Errorf("expected single-line output, got multiple lines:\n%s", output)
	}

	// Each entry must be groupId:artifactId:version.
	entries := strings.Split(output, " ")
	if len(entries) != 22 {
		t.Errorf("expected 22 space-separated entries, got %d", len(entries))
	}
	for _, e := range entries {
		parts := strings.Split(e, ":")
		if len(parts) != 3 {
			t.Errorf("expected groupId:artifactId:version format, got %q", e)
		}
	}

	// Spot-check a known entry.
	if !strings.Contains(output, "io.kestra.plugin:plugin-kafka:1.6.0") {
		t.Errorf("expected io.kestra.plugin:plugin-kafka:1.6.0 in output, got:\n%s", output)
	}
}

func TestRunPluginsList_JSONOutput(t *testing.T) {
	newMockServers(t, http.StatusOK, pluginListPayload)

	var out bytes.Buffer
	if err := runPluginsList(&out, "1.3.9", "", "json", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var plugins []pluginArtifact
	if err := json.NewDecoder(&out).Decode(&plugins); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if len(plugins) != 22 {
		t.Errorf("expected 22 plugins in JSON, got %d", len(plugins))
	}
	// Spot-check fields are populated.
	p := plugins[0]
	if p.GroupID == "" || p.ArtifactID == "" || p.Version == "" {
		t.Errorf("expected all fields populated, got %+v", p)
	}
}

func TestRunPluginsList_APIError(t *testing.T) {
	newMockServers(t, http.StatusNotFound, `{"message":"version not found"}`)

	var out bytes.Buffer
	err := runPluginsList(&out, "99.99.99", "", "table", nil)
	if err == nil {
		t.Fatal("expected error for HTTP 404, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParsePluginCoordinates(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		want    []pluginArtifact
		wantErr bool
	}{
		{
			name:  "single coordinate",
			input: []string{"io.kestra.plugin:plugin-kafka:1.6.0"},
			want:  []pluginArtifact{{GroupID: "io.kestra.plugin", ArtifactID: "plugin-kafka", Version: "1.6.0"}},
		},
		{
			name:  "space-separated (plugins list output)",
			input: []string{"io.kestra.plugin:plugin-kafka:1.6.0 io.kestra.plugin:plugin-docker:1.3.0"},
			want: []pluginArtifact{
				{GroupID: "io.kestra.plugin", ArtifactID: "plugin-kafka", Version: "1.6.0"},
				{GroupID: "io.kestra.plugin", ArtifactID: "plugin-docker", Version: "1.3.0"},
			},
		},
		{
			name:  "repeated flag values",
			input: []string{"io.kestra.plugin:plugin-kafka:1.6.0", "io.kestra.plugin:plugin-docker:1.3.0"},
			want: []pluginArtifact{
				{GroupID: "io.kestra.plugin", ArtifactID: "plugin-kafka", Version: "1.6.0"},
				{GroupID: "io.kestra.plugin", ArtifactID: "plugin-docker", Version: "1.3.0"},
			},
		},
		{
			name:    "missing version",
			input:   []string{"io.kestra.plugin:plugin-kafka"},
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   []string{""},
			wantErr: true,
		},
		{
			name:    "malformed coordinate",
			input:   []string{"notacoordinate"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePluginCoordinates(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d artifacts, want %d", len(got), len(tt.want))
			}
			for i, p := range got {
				if p.GroupID != tt.want[i].GroupID || p.ArtifactID != tt.want[i].ArtifactID || p.Version != tt.want[i].Version {
					t.Errorf("artifact[%d] = %+v, want %+v", i, p, tt.want[i])
				}
			}
		})
	}
}

func TestRunPluginsInstall_ExplicitPlugins(t *testing.T) {
	_, mavenServer := newMockServers(t, http.StatusOK, pluginListPayload)

	// Capture which paths the maven server was asked for.
	var requestedPaths []string
	mavenServer.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPaths = append(requestedPaths, r.URL.Path)
		w.Header().Set("Content-Type", "application/java-archive")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, mockJARBody)
	})

	explicit := []pluginArtifact{
		{GroupID: "io.kestra.plugin", ArtifactID: "plugin-kafka", Version: "1.6.0"},
		{GroupID: "io.kestra.plugin", ArtifactID: "plugin-docker", Version: "1.3.0"},
	}

	var out bytes.Buffer
	if err := runPluginsInstall(&out, "", t.TempDir(), 1, false, "", false, 5*time.Minute, "", "", "", nil, explicit); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()

	// Should not hit the API — no "Fetching plugin list" message.
	if strings.Contains(output, "Fetching plugin list") {
		t.Errorf("did not expect API fetch when explicit plugins provided, got:\n%s", output)
	}
	if !strings.Contains(output, "Downloaded 2") {
		t.Errorf("expected 'Downloaded 2' in output, got:\n%s", output)
	}
	// Only 2 Maven requests, not 22.
	if len(requestedPaths) != 2 {
		t.Errorf("expected 2 Maven requests, got %d", len(requestedPaths))
	}
}

func TestPluginsDownloadCommand_PluginsFlag(t *testing.T) {
	cmd := newPluginsDownloadCommand()
	if cmd.Flags().Lookup("plugins") == nil {
		t.Error("expected --plugins flag to exist")
	}
}

func TestPluginsListCommand_Flags(t *testing.T) {
	cmd := newPluginsListCommand()
	if cmd.Flags().Lookup("edition") == nil {
		t.Error("expected --edition flag to exist")
	}
	if _, err := executeCommand(cmd); err == nil {
		t.Error("expected error when version argument is missing")
	}
}

func TestParsePluginGetCoordinate(t *testing.T) {
	tests := []struct {
		name       string
		coordinate string
		want       pluginArtifact
		wantErr    bool
	}{
		{
			name:       "valid standard coordinate",
			coordinate: "io.kestra.plugin:plugin-aws:0.20.0",
			want:       pluginArtifact{GroupID: "io.kestra.plugin", ArtifactID: "plugin-aws", Version: "0.20.0"},
			wantErr:    false,
		},
		{
			name:       "valid coordinate with latest alias",
			coordinate: "io.kestra.plugin:plugin-aws:latest",
			want:       pluginArtifact{GroupID: "io.kestra.plugin", ArtifactID: "plugin-aws", Version: "999.999.999"},
			wantErr:    false,
		},
		{
			name:       "missing version",
			coordinate: "io.kestra.plugin:plugin-aws",
			want:       pluginArtifact{},
			wantErr:    true,
		},
		{
			name:       "missing artifact",
			coordinate: "io.kestra.plugin::0.20.0",
			want:       pluginArtifact{},
			wantErr:    true,
		},
		{
			name:       "empty string",
			coordinate: "",
			want:       pluginArtifact{},
			wantErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePluginGetCoordinate(tt.coordinate)
			if tt.wantErr {
				if err == nil {
					t.Error("expected an invalid coordinate error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.GroupID != tt.want.GroupID || got.ArtifactID != tt.want.ArtifactID || got.Version != tt.want.Version {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestPluginsGetCommand_Flags(t *testing.T) {
	cmd := newPluginsGetCommand()

	for _, flag := range []string{"plugins-dir", "force-redownload"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag --%s to exist", flag)
		}
	}
}

func TestPluginsGetCommand_RequiresArgs(t *testing.T) {
	cmd := newPluginsGetCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected an invalid coordinate error, got nil")
	}
}

func TestRunPluginsGet_HappyPath(t *testing.T) {
	newMockServers(t, http.StatusOK, "[]")

	tmpDir := t.TempDir()
	var out bytes.Buffer

	err := runPluginsGet(&out, "io.kestra.plugin:plugin-kafka:1.6.0", tmpDir, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "done") {
		t.Errorf("expected 'done' in output, got:\n%s", output)
	}
	if strings.Contains(output, "FAILED") {
		t.Errorf("did not expect 'FAILED' in output, got:\n%s", output)
	}

	expectedPath := filepath.Join(tmpDir, "io_kestra_plugin__plugin-kafka__1_6_0.jar")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("expected file %s to exist", expectedPath)
	}
}

func TestRunPluginsGet_SkipsExistingValidJAR(t *testing.T) {
	newMockServers(t, http.StatusOK, "[]")

	tmpDir := t.TempDir()
	jarPath := filepath.Join(tmpDir, "io_kestra_plugin__plugin-kafka__1_6_0.jar")

	if err := os.WriteFile(jarPath, []byte(mockJARBody), 0o644); err != nil {
		t.Fatalf("failed to create pre-existing JAR: %v", err)
	}

	var out bytes.Buffer
	err := runPluginsGet(&out, "io.kestra.plugin:plugin-kafka:1.6.0", tmpDir, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "already up to date") {
		t.Errorf("expected 'already up to date' in output, got:\n%s", output)
	}
	if strings.Contains(output, "done") {
		t.Errorf("did not expect 'done' in output when skipped, got:\n%s", output)
	}
}

func TestRunPluginsGet_ForceRedownloadsExistingJAR(t *testing.T) {
	newMockServers(t, http.StatusOK, "[]")

	tmpDir := t.TempDir()
	jarPath := filepath.Join(tmpDir, "io_kestra_plugin__plugin-kafka__1_6_0.jar")

	if err := os.WriteFile(jarPath, []byte(mockJARBody), 0o644); err != nil {
		t.Fatalf("failed to create pre-existing JAR: %v", err)
	}

	var out bytes.Buffer
	err := runPluginsGet(&out, "io.kestra.plugin:plugin-kafka:1.6.0", tmpDir, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if strings.Contains(output, "already up to date") {
		t.Errorf("did not expect 'already up to date' with force-redownload, got:\n%s", output)
	}
	if !strings.Contains(output, "done") {
		t.Errorf("expected 'done' in output, got:\n%s", output)
	}
}

func TestRunPluginsGet_DownloadFailure(t *testing.T) {
	mavenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	origMaven := pluginsMavenBase
	pluginsMavenBase = mavenServer.URL
	t.Cleanup(func() {
		pluginsMavenBase = origMaven
		mavenServer.Close()
	})

	var out bytes.Buffer
	err := runPluginsGet(&out, "io.kestra.plugin:plugin-kafka:1.6.0", t.TempDir(), false)

	if err == nil {
		t.Fatal("expected error when Maven returns 404, got nil")
	}

	output := out.String()
	if !strings.Contains(output, "FAILED") {
		t.Errorf("expected 'FAILED' in output, got:\n%s", output)
	}
}

func TestRunPluginsGet_InvalidCoordinate(t *testing.T) {
	var out bytes.Buffer
	err := runPluginsGet(&out, "invalid-coordinate-format", t.TempDir(), false)

	if err == nil {
		t.Fatal("expected error for invalid coordinate format, got nil")
	}
	if !strings.Contains(err.Error(), "expected groupId:artifactId:version") {
		t.Errorf("unexpected error message: %v", err)
	}
}
