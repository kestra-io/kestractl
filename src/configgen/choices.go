package configgen

// AIConfig holds one AI provider's settings.
// Fields are reused across providers with different semantics:
//   - ApiKey: API key for most providers; AWS Access Key ID (bedrock); GCP Project ID (vertex)
//   - BaseURL: base URL for ollama/azure/etc.; AWS Secret Access Key (bedrock); GCP Location (vertex)
type AIConfig struct {
	Provider string `json:"provider"` // "openai", "anthropic", "ollama", "mistralai", "azure-openai", "bedrock", "googlevertexai", "deepseek", "open-router"
	ApiKey   string `json:"api_key,omitempty"`
	Model    string `json:"model"`
	BaseURL  string `json:"base_url,omitempty"`
}

// OAuthClient holds one OAuth2/OIDC provider's settings.
type OAuthClient struct {
	Name         string `json:"name"` // "microsoft", "google", "keycloak", or custom name
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	IssuerURL    string `json:"issuer_url"`
}

// Choices captures all user answers from the
// This is the contract between the wizard and the assembler.
type Choices struct {
	// Phase 1 - Required
	Edition            string `json:"edition"` // "oss", "ee"
	ProjectName        string `json:"project_name"`
	Queue              string `json:"queue"`                         // "h2", "kafka", "redis", "amqp", "jdbc"
	Repository         string `json:"repository"`                    // "h2", "elasticsearch", "jdbc"
	QueueDatabase      string `json:"queue_database,omitempty"`      // "postgres", "mysql" - set when queue=jdbc
	RepositoryDatabase string `json:"repository_database,omitempty"` // "postgres", "mysql" - set when repository=jdbc
	Storage            string `json:"storage"`                       // "local" (others later)
	LicenseID          string `json:"license_id,omitempty"`          // EE only
	LicenseKey         string `json:"license_key,omitempty"`         // EE only
	LicenseFingerprint string `json:"license_fingerprint,omitempty"` // EE only
	TmpDirPath         string `json:"tmp_dir_path"`                  // default: /tmp/kestra-wd/tmp
	KestraURL          string `json:"kestra_url"`                    // default: http://localhost:8080/

	// Phase 2 - Optional
	SuperAdmin            bool          `json:"super_admin,omitempty"`          // EE
	SuperAdminEmail       string        `json:"super_admin_email,omitempty"`    // default: root@root.com
	SuperAdminPass        string        `json:"super_admin_password,omitempty"` // default: Root!1234
	BasicAuth             bool          `json:"basic_auth,omitempty"`           // OSS only
	OIDC                  bool          `json:"oidc,omitempty"`                 // EE
	OAuthClients          []OAuthClient `json:"oauth_clients,omitempty"`        // EE - multiple OAuth2/OIDC providers
	LDAP                  bool          `json:"ldap,omitempty"`                 // EE
	LDAPServerURL         string        `json:"ldap_server_url,omitempty"`
	LDAPBindDN            string        `json:"ldap_bind_dn,omitempty"`
	LDAPBindPassword      string        `json:"ldap_bind_password,omitempty"`
	LDAPSearchBase        string        `json:"ldap_search_base,omitempty"`
	Tenants               bool          `json:"tenants,omitempty"`    // EE
	CopilotAI             bool          `json:"copilot_ai,omitempty"` // EE
	AIConfigs             []AIConfig    `json:"ai_configs,omitempty"` // EE - multiple providers
	Mail                  bool          `json:"mail,omitempty"`       // EE
	MailHost              string        `json:"mail_host,omitempty"`
	MailPort              string        `json:"mail_port,omitempty"`
	MailUsername          string        `json:"mail_username,omitempty"`
	MailPassword          string        `json:"mail_password,omitempty"`
	MailFrom              string        `json:"mail_from,omitempty"`
	Plugins               bool          `json:"plugins,omitempty"`                 // EE
	PluginIncludes        string        `json:"plugin_includes,omitempty"`         // EE - comma-separated allowlist
	PluginExcludes        string        `json:"plugin_excludes,omitempty"`         // EE - comma-separated blocklist
	WorkerGroups          bool          `json:"worker_groups,omitempty"`           // EE
	Airgapped             bool          `json:"airgapped,omitempty"`               // EE
	OIDCGroupsClaimPath   string        `json:"oidc_groups_claim_path,omitempty"`  // EE - OIDC groups claim path
	OIDCEmailVerification bool          `json:"oidc_email_verification,omitempty"` // EE
	DataRetention         string        `json:"data_retention,omitempty"`          // Both - e.g. "30d", "7d"
	EnvironmentName       string        `json:"environment_name,omitempty"`        // Both
}

// IsEE returns true when the Enterprise Edition is selected.
func (c *Choices) IsEE() bool {
	return c.Edition == "ee"
}

// NeedsDatabase returns true when at least one relational database is required.
func (c *Choices) NeedsDatabase() bool {
	return c.Queue == "jdbc" || c.Repository == "jdbc"
}

// NeedsPostgres returns true when postgres is used by queue or repository.
func (c *Choices) NeedsPostgres() bool {
	return c.QueueDatabase == "postgres" || c.RepositoryDatabase == "postgres"
}

// NeedsMySQL returns true when mysql is used by queue or repository.
func (c *Choices) NeedsMySQL() bool {
	return c.QueueDatabase == "mysql" || c.RepositoryDatabase == "mysql"
}
