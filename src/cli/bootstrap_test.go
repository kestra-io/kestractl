package cli

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write config %q: %v", name, err)
	}
	return path
}

func TestRequiredArtifactIDs(t *testing.T) {
	tests := []struct {
		name string
		cfg  kestraConfig
		want []string
	}{
		{
			name: "storage and secret",
			cfg: configWith(map[string]string{
				"storage": "s3",
				"secret":  "vault",
			}),
			want: []string{"storage-s3", "secret-vault"},
		},
		{
			name: "bundled backends produce nothing",
			cfg: configWith(map[string]string{
				"storage":    "local",
				"secret":     "jdbc",
				"queue":      "postgres",
				"repository": "postgres",
			}),
			want: nil,
		},
		{
			name: "kafka queue is bundled, elasticsearch repository is a plugin",
			cfg: configWith(map[string]string{
				"queue":      "kafka",
				"repository": "elasticsearch",
			}),
			want: []string{"plugin-ee-elasticsearch"},
		},
		{
			name: "multi-word secret discriminators",
			cfg: configWith(map[string]string{
				"secret": "google-secret-manager",
			}),
			want: []string{"secret-googlecloud"},
		},
		{
			name: "type matching is case-insensitive",
			cfg: configWith(map[string]string{
				"storage": "GCS",
			}),
			want: []string{"storage-gcs"},
		},
		{
			name: "all four categories",
			cfg: configWith(map[string]string{
				"storage":    "azure",
				"secret":     "azure-key-vault",
				"queue":      "kafka",
				"repository": "opensearch",
			}),
			want: []string{"storage-azure", "secret-azure", "plugin-ee-opensearch"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := requiredArtifactIDs(tt.cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !equalUnordered(got, tt.want) {
				t.Errorf("requiredArtifactIDs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRequiredArtifactIDs_UnknownType(t *testing.T) {
	cfg := configWith(map[string]string{"storage": "ftp"})
	_, err := requiredArtifactIDs(cfg)
	if err == nil {
		t.Fatal("expected error for unknown storage type, got nil")
	}
	if !strings.Contains(err.Error(), "kestra.storage.type") || !strings.Contains(err.Error(), "ftp") {
		t.Errorf("error should name the offending key and value, got: %v", err)
	}
	// The error should list supported types to guide the user.
	if !strings.Contains(err.Error(), "s3") {
		t.Errorf("error should list supported types, got: %v", err)
	}
}

func TestLoadKestraConfig_Merge(t *testing.T) {
	base := writeConfig(t, "application.yaml", `
kestra:
  storage:
    type: local
  secret:
    type: jdbc
`)
	override := writeConfig(t, "override.yaml", `
kestra:
  storage:
    type: s3
`)

	cfg, err := loadKestraConfig([]string{base, override})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Kestra.Storage.Type != "s3" {
		t.Errorf("storage type = %q, want s3 (override should win)", cfg.Kestra.Storage.Type)
	}
	if cfg.Kestra.Secret.Type != "jdbc" {
		t.Errorf("secret type = %q, want jdbc (base should be preserved)", cfg.Kestra.Secret.Type)
	}
}

func TestLoadKestraConfig_MissingFile(t *testing.T) {
	_, err := loadKestraConfig([]string{filepath.Join(t.TempDir(), "nope.yaml")})
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !strings.Contains(err.Error(), "cannot read config file") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoadKestraConfig_InvalidYAML(t *testing.T) {
	path := writeConfig(t, "bad.yaml", "kestra:\n  storage:\n   type: : :")
	_, err := loadKestraConfig([]string{path})
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
	if !strings.Contains(err.Error(), "cannot parse config file") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCorePluginsFromConfig_ResolvesCoordinates(t *testing.T) {
	newMockServers(t, http.StatusOK, pluginListPayloadCore)

	path := writeConfig(t, "application.yaml", `
kestra:
  storage:
    type: s3
  secret:
    type: vault
  queue:
    type: postgres
  repository:
    type: postgres
`)

	plugins, err := corePluginsFromConfig([]string{path}, "1.3.9")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := make(map[string]string)
	for _, p := range plugins {
		got[p.ArtifactID] = p.GroupID + ":" + p.ArtifactID + ":" + p.Version
	}
	if got["storage-s3"] != "io.kestra.storage:storage-s3:1.3.5" {
		t.Errorf("storage-s3 coordinate = %q", got["storage-s3"])
	}
	if got["secret-vault"] != "io.kestra.ee.secret:secret-vault:1.3.9" {
		t.Errorf("secret-vault coordinate = %q", got["secret-vault"])
	}
	if len(plugins) != 2 {
		t.Errorf("expected exactly 2 core plugins (postgres queue/repo are bundled), got %d: %v", len(plugins), got)
	}
}

func TestCorePluginsFromConfig_AllBundled(t *testing.T) {
	newMockServers(t, http.StatusOK, pluginListPayloadCore)

	path := writeConfig(t, "application.yaml", `
kestra:
  storage:
    type: local
  queue:
    type: h2
  repository:
    type: h2
`)

	plugins, err := corePluginsFromConfig([]string{path}, "1.3.9")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 0 {
		t.Errorf("expected no plugins when all backends are bundled, got %d", len(plugins))
	}
}

func TestCorePluginsFromConfig_PluginMissingFromAPI(t *testing.T) {
	// API list omits storage-s3, so resolution must fail loudly.
	newMockServers(t, http.StatusOK, `[
  {"groupId":"io.kestra.ee.secret","artifactId":"secret-vault","license":"ENTERPRISE","version":"1.3.9"}
]`)

	path := writeConfig(t, "application.yaml", `
kestra:
  storage:
    type: s3
`)

	_, err := corePluginsFromConfig([]string{path}, "1.3.9")
	if err == nil {
		t.Fatal("expected error when a required plugin is absent from the API list, got nil")
	}
	if !strings.Contains(err.Error(), "storage-s3") {
		t.Errorf("error should name the missing plugin, got: %v", err)
	}
}

func TestRunPluginsList_FromConfig(t *testing.T) {
	newMockServers(t, http.StatusOK, pluginListPayloadCore)

	path := writeConfig(t, "application.yaml", `
kestra:
  storage:
    type: gcs
  secret:
    type: vault
`)

	var out bytes.Buffer
	if err := runPluginsList(&out, "1.3.9", "", "table", []string{path}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	line := strings.TrimSpace(out.String())
	coords := strings.Fields(line)
	sort.Strings(coords)
	want := []string{"io.kestra.ee.secret:secret-vault:1.3.9", "io.kestra.storage:storage-gcs:1.1.4"}
	if !equalUnordered(coords, want) {
		t.Errorf("list --from-config output = %v, want %v", coords, want)
	}
}

func TestPluginsDownloadCommand_FromConfigAllBundled(t *testing.T) {
	// An all-bundled config resolves to zero plugins. The command must report
	// "nothing to download" and must NOT fall through to downloading the entire
	// catalog. This path never touches the network, so no mock server is needed.
	path := writeConfig(t, "application.yaml", `
kestra:
  storage:
    type: local
  queue:
    type: postgres
  repository:
    type: postgres
`)

	cmd := newPluginsDownloadCommand()
	out, err := executeCommand(cmd, "1.3.9", "--from-config", path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "No core plugins required") {
		t.Errorf("expected 'No core plugins required' message, got:\n%s", out)
	}
	if strings.Contains(out, "Fetching plugin list") || strings.Contains(out, "Found ") {
		t.Errorf("must not fall through to full-catalog download, got:\n%s", out)
	}
}

func TestPluginsDownloadCommand_FromConfigPluginsMutuallyExclusive(t *testing.T) {
	cmd := newPluginsDownloadCommand()
	_, err := executeCommand(cmd, "1.3.9", "--from-config", "/tmp/x.yaml", "--plugins", "g:a:v")
	if err == nil {
		t.Fatal("expected error when --from-config and --plugins are both set")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPluginsDownloadCommand_FromConfigFlag(t *testing.T) {
	cmd := newPluginsDownloadCommand()
	if cmd.Flags().Lookup("from-config") == nil {
		t.Error("expected --from-config flag to exist on download")
	}
}

// configWith builds a kestraConfig from a category->type map for table tests.
func configWith(types map[string]string) kestraConfig {
	var cfg kestraConfig
	cfg.Kestra.Storage.Type = types["storage"]
	cfg.Kestra.Secret.Type = types["secret"]
	cfg.Kestra.Queue.Type = types["queue"]
	cfg.Kestra.Repository.Type = types["repository"]
	return cfg
}

func equalUnordered(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ac := append([]string(nil), a...)
	bc := append([]string(nil), b...)
	sort.Strings(ac)
	sort.Strings(bc)
	for i := range ac {
		if ac[i] != bc[i] {
			return false
		}
	}
	return true
}

// pluginListPayloadCore is a compatibility-API payload containing the core
// storage/secret/repository backends exercised by the bootstrap tests.
const pluginListPayloadCore = `[
  {"groupId":"io.kestra.storage","artifactId":"storage-s3","license":"OPEN_SOURCE","version":"1.3.5"},
  {"groupId":"io.kestra.storage","artifactId":"storage-gcs","license":"OPEN_SOURCE","version":"1.1.4"},
  {"groupId":"io.kestra.storage","artifactId":"storage-azure","license":"OPEN_SOURCE","version":"1.2.0"},
  {"groupId":"io.kestra.ee.secret","artifactId":"secret-vault","license":"ENTERPRISE","version":"1.3.9"},
  {"groupId":"io.kestra.ee.secret","artifactId":"secret-azure","license":"ENTERPRISE","version":"1.3.9"},
  {"groupId":"io.kestra.plugin.ee","artifactId":"plugin-ee-elasticsearch","license":"ENTERPRISE","version":"1.3.9"},
  {"groupId":"io.kestra.plugin.ee","artifactId":"plugin-ee-opensearch","license":"ENTERPRISE","version":"1.3.9"},
  {"groupId":"io.kestra.plugin","artifactId":"plugin-jdbc-postgres","license":"OPEN_SOURCE","version":"1.10.1"}
]`
