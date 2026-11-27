package configgen

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// validEditions lists accepted edition values.
var validEditions = map[string]bool{"oss": true, "ee": true}

// validQueues lists accepted queue values per edition.
var validQueues = map[string]map[string]bool{
	"oss": {"h2": true, "jdbc": true},
	"ee":  {"h2": true, "kafka": true, "redis": true, "amqp": true, "jdbc": true},
}

// validRepositories lists accepted repository values per edition.
var validRepositories = map[string]map[string]bool{
	"oss": {"jdbc": true},
	"ee":  {"elasticsearch": true, "jdbc": true},
}

// validDatabases lists accepted database values.
var validDatabases = map[string]bool{"postgres": true, "mysql": true}

// validDataRetention lists accepted retention periods.
var validDataRetention = map[string]bool{
	"7d": true, "14d": true, "30d": true, "60d": true, "90d": true, "180d": true, "365d": true,
}

// LoadFromFile reads a JSON config file and returns a validated Choices.
func LoadFromFile(path string) (Choices, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Choices{}, fmt.Errorf("reading config file: %w", err)
	}

	c := Choices{
		// Same defaults as the wizard
		Storage:         "local",
		TmpDirPath:      defaultTmpDir,
		KestraURL:       "http://localhost:8080/",
		SuperAdminEmail: "root@root.com",
		SuperAdminPass:  "Root!1234",
		MailPort:        "587",
	}

	if err := json.Unmarshal(data, &c); err != nil {
		return Choices{}, fmt.Errorf("parsing config file: %w", err)
	}

	if err := Validate(&c); err != nil {
		return Choices{}, err
	}

	return c, nil
}

// Validate checks a Choices struct for consistency and required fields.
// It returns a descriptive error listing all problems found.
func Validate(c *Choices) error {
	var errs []string

	// --- Required fields ---
	if !validEditions[c.Edition] {
		errs = append(errs, fmt.Sprintf("edition must be \"oss\" or \"ee\", got %q", c.Edition))
	}
	if c.ProjectName == "" {
		errs = append(errs, "project_name is required")
	}

	// Queue
	if c.Edition != "" {
		if allowed, ok := validQueues[c.Edition]; ok {
			if !allowed[c.Queue] {
				valid := keys(allowed)
				errs = append(errs, fmt.Sprintf("queue %q is not valid for edition %q (valid: %s)", c.Queue, c.Edition, strings.Join(valid, ", ")))
			}
		}
	}

	// H2 implies h2 repository - apply forced values like the wizard does
	if c.Queue == "h2" {
		c.Repository = "h2"
		c.QueueDatabase = ""
		c.RepositoryDatabase = ""
	}

	// Repository
	if c.Queue != "h2" {
		if c.Repository == "" {
			errs = append(errs, "repository is required when queue is not h2")
		} else if c.Edition != "" {
			if allowed, ok := validRepositories[c.Edition]; ok {
				if !allowed[c.Repository] {
					valid := keys(allowed)
					errs = append(errs, fmt.Sprintf("repository %q is not valid for edition %q (valid: %s)", c.Repository, c.Edition, strings.Join(valid, ", ")))
				}
			}
		}
	}

	// A database only applies to a jdbc queue or repository
	if c.Queue != "jdbc" {
		c.QueueDatabase = ""
	}
	if c.Repository != "jdbc" {
		c.RepositoryDatabase = ""
	}
	// Kestra needs one database for both, mixing engines does not start
	if c.QueueDatabase != "" && c.RepositoryDatabase != "" && c.QueueDatabase != c.RepositoryDatabase {
		errs = append(errs, fmt.Sprintf("queue_database %q and repository_database %q must match when both are jdbc", c.QueueDatabase, c.RepositoryDatabase))
	}

	// Queue database
	if c.Queue == "jdbc" && c.QueueDatabase == "" {
		errs = append(errs, "queue_database is required when queue is \"jdbc\"")
	}
	if c.QueueDatabase != "" && !validDatabases[c.QueueDatabase] {
		errs = append(errs, fmt.Sprintf("queue_database must be \"postgres\" or \"mysql\", got %q", c.QueueDatabase))
	}

	// Repository database
	if c.Repository == "jdbc" && c.RepositoryDatabase == "" {
		errs = append(errs, "repository_database is required when repository is \"jdbc\"")
	}
	if c.RepositoryDatabase != "" && !validDatabases[c.RepositoryDatabase] {
		errs = append(errs, fmt.Sprintf("repository_database must be \"postgres\" or \"mysql\", got %q", c.RepositoryDatabase))
	}

	// License (EE only)
	if c.IsEE() {
		if c.LicenseID == "" {
			errs = append(errs, "license_id is required for EE edition")
		}
		if c.LicenseKey == "" {
			errs = append(errs, "license_key is required for EE edition")
		}
		if c.LicenseFingerprint == "" {
			errs = append(errs, "license_fingerprint is required for EE edition")
		}
	}

	// --- Edition-gated features ---
	if !c.IsEE() {
		eeOnly := []struct {
			flag bool
			name string
		}{
			{c.SuperAdmin, "super_admin"},
			{c.OIDC, "oidc"},
			{c.LDAP, "ldap"},
			{c.Tenants, "tenants"},
			{c.CopilotAI, "copilot_ai"},
			{c.Mail, "mail"},
			{c.Plugins, "plugins"},
			{c.WorkerGroups, "worker_groups"},
			{c.Airgapped, "airgapped"},
		}
		for _, f := range eeOnly {
			if f.flag {
				errs = append(errs, fmt.Sprintf("%s is only available for EE edition", f.name))
			}
		}
	}

	if !c.IsEE() {
		for name, set := range map[string]bool{
			"oauth_clients":   len(c.OAuthClients) > 0,
			"ai_configs":      len(c.AIConfigs) > 0,
			"plugin_includes": c.PluginIncludes != "",
			"plugin_excludes": c.PluginExcludes != "",
		} {
			if set {
				errs = append(errs, fmt.Sprintf("%s is only available for EE edition", name))
			}
		}
	}

	// --- Feature-specific validation ---
	if c.LDAP {
		if c.LDAPServerURL == "" {
			errs = append(errs, "ldap_server_url is required when ldap is enabled")
		}
		if c.LDAPBindDN == "" {
			errs = append(errs, "ldap_bind_dn is required when ldap is enabled")
		}
		if c.LDAPBindPassword == "" {
			errs = append(errs, "ldap_bind_password is required when ldap is enabled")
		}
		if c.LDAPSearchBase == "" {
			errs = append(errs, "ldap_search_base is required when ldap is enabled")
		}
	}

	if c.OIDC && len(c.OAuthClients) == 0 {
		errs = append(errs, "oauth_clients must contain at least one provider when oidc is enabled")
	}
	for i, oc := range c.OAuthClients {
		if oc.Name == "" {
			errs = append(errs, fmt.Sprintf("oauth_clients[%d].name is required", i))
		}
		if oc.ClientID == "" {
			errs = append(errs, fmt.Sprintf("oauth_clients[%d].client_id is required", i))
		}
		if oc.ClientSecret == "" {
			errs = append(errs, fmt.Sprintf("oauth_clients[%d].client_secret is required", i))
		}
		if oc.IssuerURL == "" {
			errs = append(errs, fmt.Sprintf("oauth_clients[%d].issuer_url is required", i))
		}
	}

	if c.CopilotAI && len(c.AIConfigs) == 0 {
		errs = append(errs, "ai_configs must contain at least one provider when copilot_ai is enabled")
	}
	for i, ai := range c.AIConfigs {
		if ai.Provider == "" {
			errs = append(errs, fmt.Sprintf("ai_configs[%d].provider is required", i))
		}
		if ai.Model == "" {
			errs = append(errs, fmt.Sprintf("ai_configs[%d].model is required", i))
		}
	}

	if c.Mail {
		if c.MailHost == "" {
			errs = append(errs, "mail_host is required when mail is enabled")
		}
		if c.MailFrom == "" {
			errs = append(errs, "mail_from is required when mail is enabled")
		}
	}

	if c.DataRetention != "" && !validDataRetention[c.DataRetention] {
		valid := keys(validDataRetention)
		errs = append(errs, fmt.Sprintf("data_retention %q is not valid (valid: %s)", c.DataRetention, strings.Join(valid, ", ")))
	}

	if len(errs) > 0 {
		return fmt.Errorf("validation errors:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

// PrintDefaults outputs an annotated JSON config with all defaults and documentation.
func PrintDefaults(w io.Writer) error {
	example := map[string]any{
		"_comment": "kestractl config generate input file. Remove fields you don't need.",

		"edition":      "oss | ee",
		"project_name": "my-kestra-stack",
		"queue":        "h2 | jdbc | kafka (ee) | redis (ee) | amqp (ee)",
		"repository":   "jdbc | elasticsearch (ee) - omit when queue=h2",

		"queue_database":      "postgres | mysql - required when queue=jdbc",
		"repository_database": "postgres | mysql - required when repository=jdbc",

		"storage":      "local",
		"tmp_dir_path": "/tmp/kestra-wd/tmp",
		"kestra_url":   "http://localhost:8080/",

		"_comment_ee": "EE-only fields below - remove for OSS",

		"license_id":          "your-license-id",
		"license_key":         "your-multiline-license-key",
		"license_fingerprint": "your-fingerprint",

		"super_admin":          false,
		"super_admin_email":    "root@root.com",
		"super_admin_password": "Root!1234",

		"basic_auth": false,

		"oidc": false,
		"oauth_clients": []map[string]any{
			{
				"name":          "microsoft | google | keycloak",
				"client_id":     "your-client-id",
				"client_secret": "your-client-secret",
				"issuer_url":    "https://login.microsoftonline.com/{tenant-id}/v2.0",
			},
		},

		"ldap":               false,
		"ldap_server_url":    "ldap://ldap.example.com:389",
		"ldap_bind_dn":       "cn=admin,dc=example,dc=com",
		"ldap_bind_password": "password",
		"ldap_search_base":   "dc=example,dc=com",

		"tenants": false,

		"copilot_ai": false,
		"ai_configs": []map[string]any{
			{
				"provider": "openai | anthropic | ollama | mistralai | azure-openai | bedrock | googlevertexai | gemini | deepseek | open-router",
				"api_key":  "your-api-key",
				"model":    "model-name",
				"base_url": "optional - required for ollama, azure-openai",
			},
		},

		"mail":          false,
		"mail_host":     "smtp.example.com",
		"mail_port":     "587",
		"mail_username": "user",
		"mail_password": "password",
		"mail_from":     "noreply@example.com",

		"plugins":         false,
		"plugin_includes": "comma,separated,allowlist",
		"plugin_excludes": "comma,separated,blocklist",

		"worker_groups": false,
		"airgapped":     false,

		"oidc_groups_claim_path":  "",
		"oidc_email_verification": false,

		"data_retention":   "7d | 14d | 30d | 60d | 90d | 180d | 365d",
		"environment_name": "production",
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(example)
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
