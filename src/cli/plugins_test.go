package cli

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

// newMockServers sets up:
//   - apiServer: returns pluginListPayload for any request
//   - mavenServer: returns a dummy JAR body for any request
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
		fmt.Fprint(w, "PK\x03\x04") // minimal ZIP/JAR magic bytes
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

	err := runPluginsInstall(&out, "1.3.9", tmpDir, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()

	if !strings.Contains(output, "Found 22 plugins") {
		t.Errorf("expected 'Found 22 plugins' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Installed 22 plugin(s)") {
		t.Errorf("expected 'Installed 22 plugin(s)' in output, got:\n%s", output)
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
		"storage-gcs-1.1.4.jar",
		"plugin-kafka-1.6.0.jar",
		"plugin-jdbc-postgres-1.10.1.jar",
		"plugin-ee-core-1.3.9.jar",
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

	err := runPluginsInstall(&out, "1.3.9", tmpDir, 4)
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
	err := runPluginsInstall(&out, "99.99.99", t.TempDir(), 1)
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
	err := runPluginsInstall(&out, "1.3.9", t.TempDir(), 1)
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
	err := runPluginsInstall(&out, "1.3.9", t.TempDir(), 1)
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

func TestMavenJARURL(t *testing.T) {
	origMaven := pluginsMavenBase
	pluginsMavenBase = "https://repo1.maven.org/maven2"
	defer func() { pluginsMavenBase = origMaven }()

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
		got := mavenJARURL(tt.plugin)
		if got != tt.wantURL {
			t.Errorf("mavenJARURL(%+v)\n  got:  %s\n  want: %s", tt.plugin, got, tt.wantURL)
		}
	}
}

func TestPluginsInstallCommand_Flags(t *testing.T) {
	cmd := newPluginsInstallCommand()

	for _, flag := range []string{"plugins-dir", "concurrency"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag --%s to exist", flag)
		}
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

func TestPluginsInstallCommand_RequiresVersion(t *testing.T) {
	cmd := newPluginsInstallCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when version argument is missing")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Errorf("unexpected error: %v", err)
	}
}
