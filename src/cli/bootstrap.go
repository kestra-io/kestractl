package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// builtinType is the sentinel value used in the *TypeToArtifact maps below to
// mean "this backend is bundled in the Kestra distribution — there is no plugin
// to download". It is distinct from an unknown type, which is an error.
const builtinType = ""

// storageTypeToArtifact maps a kestra.storage.type discriminator to the Maven
// artifactId of the plugin that provides it. Values are sourced from the Kestra
// runtime & storage configuration documentation.
var storageTypeToArtifact = map[string]string{
	"local":      builtinType,
	"s3":         "storage-s3",
	"gcs":        "storage-gcs",
	"azure":      "storage-azure",
	"minio":      "storage-minio",
	"seaweedfs":  "storage-seaweedfs",
	"cloudflare": "storage-cloudflare",
}

// secretTypeToArtifact maps a kestra.secret.type discriminator to the secret
// manager plugin artifactId. The jdbc and elasticsearch backends are bundled.
var secretTypeToArtifact = map[string]string{
	"jdbc":                  builtinType,
	"elasticsearch":         builtinType,
	"vault":                 "secret-vault",
	"aws-secret-manager":    "secret-aws",
	"azure-key-vault":       "secret-azure",
	"google-secret-manager": "secret-googlecloud",
	"cyberark":              "secret-cyberark",
	"doppler":               "secret-doppler",
	"1password":             "secret-1password",
	"beyondtrust":           "secret-beyondtrust",
	"delinea":               "secret-delinea",
}

// queueTypeToArtifact maps a kestra.queue.type discriminator to a plugin
// artifactId. All currently supported queue backends (JDBC variants and the
// EE Kafka queue) are bundled in the distribution, so none require a plugin.
var queueTypeToArtifact = map[string]string{
	"memory":   builtinType,
	"h2":       builtinType,
	"postgres": builtinType,
	"mysql":    builtinType,
	"kafka":    builtinType, // bundled in the Kestra EE distribution
}

// repositoryTypeToArtifact maps a kestra.repository.type discriminator to a
// plugin artifactId. JDBC variants are bundled; the EE search backends ship as
// plugins.
var repositoryTypeToArtifact = map[string]string{
	"memory":        builtinType,
	"h2":            builtinType,
	"postgres":      builtinType,
	"mysql":         builtinType,
	"elasticsearch": "plugin-ee-elasticsearch",
	"opensearch":    "plugin-ee-opensearch",
}

// kestraConfig is the minimal subset of a Kestra application.yaml needed to
// determine the core plugins required to start a component.
type kestraConfig struct {
	Kestra struct {
		Storage    typedBackend `yaml:"storage"`
		Secret     typedBackend `yaml:"secret"`
		Queue      typedBackend `yaml:"queue"`
		Repository typedBackend `yaml:"repository"`
	} `yaml:"kestra"`
}

type typedBackend struct {
	Type string `yaml:"type"`
}

// loadKestraConfig reads and merges one or more Kestra configuration files.
// Only the four core discriminators are read; for each, the last file that sets
// a non-empty value wins, matching Micronaut's "later source overrides earlier"
// precedence for these scalar properties.
func loadKestraConfig(paths []string) (kestraConfig, error) {
	var merged kestraConfig
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return kestraConfig{}, fmt.Errorf("cannot read config file %q: %w", p, err)
		}
		var c kestraConfig
		if err := yaml.Unmarshal(data, &c); err != nil {
			return kestraConfig{}, fmt.Errorf("cannot parse config file %q: %w", p, err)
		}
		if c.Kestra.Storage.Type != "" {
			merged.Kestra.Storage.Type = c.Kestra.Storage.Type
		}
		if c.Kestra.Secret.Type != "" {
			merged.Kestra.Secret.Type = c.Kestra.Secret.Type
		}
		if c.Kestra.Queue.Type != "" {
			merged.Kestra.Queue.Type = c.Kestra.Queue.Type
		}
		if c.Kestra.Repository.Type != "" {
			merged.Kestra.Repository.Type = c.Kestra.Repository.Type
		}
	}
	return merged, nil
}

// requiredArtifactIDs inspects a parsed Kestra config and returns the plugin
// artifactIds required to start, skipping bundled backends. An unrecognized
// type value is an error so a typo or a new backend surfaces loudly rather than
// silently resolving to "no plugins".
func requiredArtifactIDs(cfg kestraConfig) ([]string, error) {
	categories := []struct {
		label   string
		typeVal string
		mapping map[string]string
	}{
		{"storage", cfg.Kestra.Storage.Type, storageTypeToArtifact},
		{"secret", cfg.Kestra.Secret.Type, secretTypeToArtifact},
		{"queue", cfg.Kestra.Queue.Type, queueTypeToArtifact},
		{"repository", cfg.Kestra.Repository.Type, repositoryTypeToArtifact},
	}

	var ids []string
	for _, c := range categories {
		if c.typeVal == "" {
			continue
		}
		artifact, ok := c.mapping[strings.ToLower(c.typeVal)]
		if !ok {
			return nil, fmt.Errorf("unknown kestra.%s.type %q — no known core plugin mapping (supported: %s)",
				c.label, c.typeVal, strings.Join(sortedKeys(c.mapping), ", "))
		}
		if artifact == builtinType {
			continue
		}
		ids = append(ids, artifact)
	}
	return ids, nil
}

// resolveCorePlugins fetches the compatibility list for the given version and
// returns the pluginArtifact entries matching the required artifactIds, pinning
// each to the version the API reports for that Kestra release.
func resolveCorePlugins(kestraVersion string, license string, artifactIDs []string) ([]pluginArtifact, error) {
	plugins, err := fetchPluginList(kestraVersion, license)
	if err != nil {
		return nil, err
	}

	byID := make(map[string]pluginArtifact, len(plugins))
	for _, p := range plugins {
		if _, exists := byID[p.ArtifactID]; !exists {
			byID[p.ArtifactID] = p
		}
	}

	var result []pluginArtifact
	var missing []string
	for _, id := range artifactIDs {
		if p, ok := byID[id]; ok {
			result = append(result, p)
		} else {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("core plugin(s) not available for Kestra %s: %s — enterprise plugins require --edition ALL or EE",
			kestraVersion, strings.Join(missing, ", "))
	}
	return result, nil
}

// corePluginsFromConfig is the end-to-end helper shared by "plugins list" and
// "plugins download": it parses the config file(s), maps the configured
// backends to plugin coordinates, and pins them to the given Kestra version.
func corePluginsFromConfig(configPaths []string, kestraVersion string, license string) ([]pluginArtifact, error) {
	cfg, err := loadKestraConfig(configPaths)
	if err != nil {
		return nil, err
	}
	ids, err := requiredArtifactIDs(cfg)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	return resolveCorePlugins(kestraVersion, license, ids)
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
