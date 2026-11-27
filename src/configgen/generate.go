package configgen

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

// ComposeData holds the flags used to render the compose template.
type ComposeData struct {
	IsEE          bool
	Postgres      bool
	MySQL         bool
	Kafka         bool
	Elasticsearch bool
	Redis         bool
	RabbitMQ      bool
	HasInfra      bool
}

// Generate writes application.yml, docker-compose.yml, and .env into dir
// and returns the written file paths.
func Generate(c Choices, dir string) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating output dir: %w", err)
	}

	encryptionKey, err := generateRandomKey(32)
	if err != nil {
		return nil, fmt.Errorf("generating encryption key: %w", err)
	}
	jwtSecret, err := generateRandomKey(32)
	if err != nil {
		return nil, fmt.Errorf("generating JWT secret: %w", err)
	}

	replacements := buildReplacements(c, encryptionKey, jwtSecret)

	if err := generateApplicationYML(dir, c, replacements); err != nil {
		return nil, err
	}
	if err := generateDockerCompose(dir, c); err != nil {
		return nil, err
	}
	if err := generateEnvFile(dir, c, replacements); err != nil {
		return nil, err
	}
	return []string{
		filepath.Join(dir, "application.yml"),
		filepath.Join(dir, "docker-compose.yml"),
		filepath.Join(dir, ".env"),
	}, nil
}

func buildReplacements(c Choices, encryptionKey, jwtSecret string) map[string]string {
	r := map[string]string{
		"__TMP_DIR_PATH__":   c.TmpDirPath,
		"__KESTRA_URL__":     c.KestraURL,
		"__ENCRYPTION_KEY__": encryptionKey,
		"__JWT_SECRET__":     jwtSecret,
		"__QUEUE_DB_TYPE__":  c.QueueDatabase,
		"__REPO_DB_TYPE__":   c.RepositoryDatabase,
	}

	// EE fields
	if c.IsEE() {
		r["__LICENSE_ID__"] = c.LicenseID
		r["__LICENSE_KEY__"] = c.LicenseKey
		r["__LICENSE_FINGERPRINT__"] = c.LicenseFingerprint
	}

	if c.SuperAdmin {
		r["__SUPER_ADMIN_EMAIL__"] = c.SuperAdminEmail
		r["__SUPER_ADMIN_PASSWORD__"] = c.SuperAdminPass
	}

	if c.LDAP {
		r["__LDAP_SERVER_URL__"] = c.LDAPServerURL
		r["__LDAP_BIND_DN__"] = c.LDAPBindDN
		r["__LDAP_BIND_PASSWORD__"] = c.LDAPBindPassword
		r["__LDAP_SEARCH_BASE__"] = c.LDAPSearchBase
	}

	if c.Mail {
		r["__MAIL_HOST__"] = c.MailHost
		r["__MAIL_PORT__"] = c.MailPort
		r["__MAIL_USERNAME__"] = c.MailUsername
		r["__MAIL_PASSWORD__"] = c.MailPassword
		r["__MAIL_FROM__"] = c.MailFrom
	}

	if c.EnvironmentName != "" {
		r["__ENVIRONMENT_NAME__"] = c.EnvironmentName
	}

	if c.DataRetention != "" {
		r["__DATA_RETENTION__"] = c.DataRetention
	}

	if c.OIDCGroupsClaimPath != "" {
		r["__OIDC_GROUPS_CLAIM_PATH__"] = c.OIDCGroupsClaimPath
	}
	r["__OIDC_EMAIL_VERIFICATION__"] = fmt.Sprintf("%t", c.OIDCEmailVerification)

	return r
}

// generateApplicationYML assembles config snippets, merges, replaces, and writes.
func generateApplicationYML(dir string, c Choices, replacements map[string]string) error {
	snippetNames := selectConfigSnippets(c)

	merged := map[string]any{}
	for _, name := range snippetNames {
		data, err := templatesFS.ReadFile("templates/config/" + name)
		if err != nil {
			return fmt.Errorf("reading snippet %s: %w", name, err)
		}
		var m map[string]any
		if err := yaml.Unmarshal(data, &m); err != nil {
			return fmt.Errorf("parsing snippet %s: %w", name, err)
		}
		merged = deepMerge(merged, m)
	}

	// AI providers: build kestra.ai.providers list
	if len(c.AIConfigs) > 0 {
		providers := buildAIProviders(c.AIConfigs)
		aiConfig := map[string]any{
			"kestra": map[string]any{
				"ai": map[string]any{
					"providers": providers,
				},
			},
		}
		merged = deepMerge(merged, aiConfig)
	}

	// Plugin security: build includes/excludes lists from comma-separated strings
	if c.PluginIncludes != "" || c.PluginExcludes != "" {
		pluginSecurity := buildPluginSecurity(c.PluginIncludes, c.PluginExcludes)
		merged = deepMerge(merged, pluginSecurity)
	}

	// OAuth2/OIDC providers: build micronaut.security.oauth2.clients map
	if len(c.OAuthClients) > 0 {
		oauthConfig := buildOAuthConfig(c.OAuthClients)
		merged = deepMerge(merged, oauthConfig)
	}

	merged = replacePlaceholders(merged, replacements)

	// Marshal to yaml.Node tree so we can set literal block style on the license key
	var doc yaml.Node
	out, err := yaml.Marshal(merged)
	if err != nil {
		return fmt.Errorf("marshaling application.yml: %w", err)
	}
	if err := yaml.Unmarshal(out, &doc); err != nil {
		return fmt.Errorf("parsing yaml node tree: %w", err)
	}
	setLiteralBlockStyle(&doc, "key")

	finalOut, err := yaml.Marshal(&doc)
	if err != nil {
		return fmt.Errorf("marshaling final application.yml: %w", err)
	}

	return os.WriteFile(filepath.Join(dir, "application.yml"), finalOut, 0o600)
}

// buildAIProviders constructs the kestra.ai.providers list from user choices.
// Each entry has: id, displayName, type, isDefault, configuration.
func buildAIProviders(configs []AIConfig) []any {
	providers := make([]any, 0, len(configs))
	for i, ai := range configs {
		config := buildAIProviderConfig(ai)
		provider := map[string]any{
			"id":            fmt.Sprintf("%s-%d", ai.Provider, i),
			"displayName":   aiDisplayName(ai.Provider, ai.Model),
			"type":          ai.Provider,
			"isDefault":     i == 0,
			"configuration": config,
		}
		providers = append(providers, provider)
	}
	return providers
}

func buildAIProviderConfig(ai AIConfig) map[string]any {
	config := map[string]any{
		"model-name": ai.Model,
	}
	switch ai.Provider {
	case "ollama":
		config["base-url"] = ai.BaseURL
	case "azure-openai":
		config["endpoint"] = ai.BaseURL
		config["api-key"] = ai.ApiKey
	case "bedrock":
		config["access-key-id"] = ai.ApiKey
		config["secret-access-key"] = ai.BaseURL
	case "googlevertexai":
		config["project"] = ai.ApiKey
		config["location"] = ai.BaseURL
	default:
		// openai, anthropic, mistralai, deepseek, open-router
		config["api-key"] = ai.ApiKey
	}
	return config
}

func aiDisplayName(provider, model string) string {
	labels := map[string]string{
		"openai":         "OpenAI",
		"anthropic":      "Anthropic",
		"ollama":         "Ollama",
		"mistralai":      "Mistral AI",
		"azure-openai":   "Azure OpenAI",
		"bedrock":        "Amazon Bedrock",
		"gemini":         "Google Gemini",
		"googlevertexai": "Google Vertex AI",
		"deepseek":       "DeepSeek",
		"open-router":    "OpenRouter",
	}
	label := provider
	if l, ok := labels[provider]; ok {
		label = l
	}
	if model != "" {
		return label + " (" + model + ")"
	}
	return label
}

// buildPluginSecurity constructs kestra.plugins.security from comma-separated strings.
func buildPluginSecurity(includes, excludes string) map[string]any {
	security := map[string]any{}
	if includes != "" {
		list := splitAndTrim(includes)
		security["includes"] = list
	}
	if excludes != "" {
		list := splitAndTrim(excludes)
		security["excludes"] = list
	}
	return map[string]any{
		"kestra": map[string]any{
			"plugins": map[string]any{
				"security": security,
			},
		},
	}
}

func splitAndTrim(s string) []any {
	parts := strings.Split(s, ",")
	result := make([]any, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// buildOAuthConfig constructs the micronaut.security.oauth2 config from OAuthClients.
func buildOAuthConfig(clients []OAuthClient) map[string]any {
	clientsMap := map[string]any{}
	for _, c := range clients {
		envVar := fmt.Sprintf("OIDC_%s_CLIENT_SECRET", strings.ToUpper(strings.ReplaceAll(c.Name, "-", "_")))
		clientsMap[c.Name] = map[string]any{
			"client-id":     c.ClientID,
			"client-secret": fmt.Sprintf("${%s}", envVar),
			"openid": map[string]any{
				"issuer": c.IssuerURL,
			},
		}
	}

	return map[string]any{
		"micronaut": map[string]any{
			"security": map[string]any{
				"enabled":        true,
				"authentication": "cookie",
				"oauth2": map[string]any{
					"enabled": true,
					"clients": clientsMap,
				},
				"token": map[string]any{
					"jwt": map[string]any{
						"enabled": true,
						"signatures": map[string]any{
							"secret": map[string]any{
								"generator": map[string]any{
									"secret": "${JWT_SECRET}",
								},
							},
						},
						"generator": map[string]any{
							"refresh-token": map[string]any{
								"enabled": true,
							},
						},
					},
					"cookie": map[string]any{
						"enabled":          true,
						"cookie-same-site": "strict",
					},
				},
			},
		},
	}
}

// selectConfigSnippets returns the list of snippet filenames based on choices.
func selectConfigSnippets(c Choices) []string {
	snippets := []string{"base.yml", "storage-local.yml"}

	// License only for EE
	if c.IsEE() {
		snippets = append(snippets, "license.yml")
	}

	if c.Queue == "h2" {
		snippets = append(snippets, "datasource-h2.yml")
	} else {
		// Queue snippet
		switch c.Queue {
		case "kafka":
			snippets = append(snippets, "queue-kafka.yml")
		case "redis":
			snippets = append(snippets, "queue-redis.yml")
		case "amqp":
			snippets = append(snippets, "queue-amqp.yml")
		case "jdbc":
			snippets = append(snippets, "queue-jdbc.yml")
		}

		// Repository snippet
		switch c.Repository {
		case "elasticsearch":
			snippets = append(snippets, "repository-elasticsearch.yml")
			snippets = append(snippets, "secret-elasticsearch.yml")
		case "jdbc":
			snippets = append(snippets, "repository-jdbc.yml")
			snippets = append(snippets, "secret-jdbc.yml")
		}

		// Database snippets - include each datasource needed by queue or repository
		if c.NeedsPostgres() {
			snippets = append(snippets, "datasource-postgres.yml")
		}
		if c.NeedsMySQL() {
			snippets = append(snippets, "datasource-mysql.yml")
		}
	}

	// Optional features
	if c.SuperAdmin {
		snippets = append(snippets, "super-admin.yml")
	}
	if c.BasicAuth {
		snippets = append(snippets, "auth-basic.yml")
	}
	// OIDC is handled directly in generateApplicationYML via buildOAuthConfig
	if c.LDAP {
		snippets = append(snippets, "auth-ldap.yml")
	}
	if c.Tenants {
		snippets = append(snippets, "tenants.yml")
	}
	if c.Mail {
		snippets = append(snippets, "mail.yml")
	}
	if c.Plugins {
		snippets = append(snippets, "plugins.yml")
	}
	if c.WorkerGroups {
		snippets = append(snippets, "worker-groups.yml")
	}
	if c.Airgapped {
		snippets = append(snippets, "airgapped.yml")
	}
	if c.OIDCGroupsClaimPath != "" || c.OIDCEmailVerification {
		snippets = append(snippets, "oidc-settings.yml")
	}
	if c.DataRetention != "" {
		snippets = append(snippets, "data-retention.yml")
	}
	if c.EnvironmentName != "" {
		snippets = append(snippets, "environment.yml")
	}

	// AI snippets and plugin security are handled separately in generateApplicationYML

	return snippets
}

// generateDockerCompose renders the compose template and writes it.
func generateDockerCompose(dir string, c Choices) error {
	data := ComposeData{
		IsEE:          c.IsEE(),
		Postgres:      c.NeedsPostgres(),
		MySQL:         c.NeedsMySQL(),
		Kafka:         c.Queue == "kafka",
		Elasticsearch: c.Repository == "elasticsearch",
		Redis:         c.Queue == "redis",
		RabbitMQ:      c.Queue == "amqp",
	}
	data.HasInfra = data.Postgres || data.MySQL || data.Kafka || data.Elasticsearch || data.Redis || data.RabbitMQ

	tmplData, err := templatesFS.ReadFile("templates/compose-standalone.yml.tmpl")
	if err != nil {
		return fmt.Errorf("reading compose template: %w", err)
	}

	tmpl, err := template.New("compose").Parse(string(tmplData))
	if err != nil {
		return fmt.Errorf("parsing compose template: %w", err)
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("rendering compose template: %w", err)
	}

	return os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(buf.String()), 0644)
}

// generateEnvFile assembles .env snippets and writes the file.
func generateEnvFile(dir string, c Choices, replacements map[string]string) error {
	envSnippets := []string{"base.env"}

	if c.NeedsPostgres() {
		envSnippets = append(envSnippets, "postgres.env")
	}
	if c.NeedsMySQL() {
		envSnippets = append(envSnippets, "mysql.env")
	}
	if c.BasicAuth {
		envSnippets = append(envSnippets, "auth-basic.env")
	}
	if c.LDAP {
		envSnippets = append(envSnippets, "auth-ldap.env")
	}
	if c.Mail {
		envSnippets = append(envSnippets, "mail.env")
	}

	var content strings.Builder
	for _, name := range envSnippets {
		data, err := templatesFS.ReadFile("templates/env/" + name)
		if err != nil {
			return fmt.Errorf("reading env snippet %s: %w", name, err)
		}
		s := string(data)
		for placeholder, replacement := range replacements {
			s = strings.ReplaceAll(s, placeholder, replacement)
		}
		content.WriteString(s)
	}

	// OIDC: one env var per OAuth client secret + JWT secret
	if len(c.OAuthClients) > 0 {
		for _, client := range c.OAuthClients {
			envVar := fmt.Sprintf("OIDC_%s_CLIENT_SECRET", strings.ToUpper(strings.ReplaceAll(client.Name, "-", "_")))
			content.WriteString(fmt.Sprintf("%s=%s\n", envVar, client.ClientSecret))
		}
		content.WriteString(fmt.Sprintf("JWT_SECRET=%s\n", replacements["__JWT_SECRET__"]))
	}

	return os.WriteFile(filepath.Join(dir, ".env"), []byte(content.String()), 0o600)
}

func generateRandomKey(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(b), nil
}
