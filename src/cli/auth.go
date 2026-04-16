package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// AuthContext describes an authentication context for Kestra.
type AuthContext struct {
	Name       string   `json:"name" yaml:"-"` // Name is used as map key, not serialized to YAML
	Host       string   `json:"host" yaml:"host"`
	Tenant     string   `json:"tenant" yaml:"tenant"`
	AuthMethod string   `json:"auth_method" yaml:"auth_method"`
	Token      string   `json:"token,omitempty" yaml:"token,omitempty"`
	Username   string   `json:"username,omitempty" yaml:"username,omitempty"`
	Password   string   `json:"password,omitempty" yaml:"password,omitempty"`
	Headers    []string `json:"headers,omitempty" yaml:"headers,omitempty"`
}

type authConfig struct {
	Contexts       map[string]AuthContext `yaml:"contexts,omitempty"`
	DefaultContext string                 `yaml:"default_context,omitempty"`
}

// AuthManager persists and retrieves Kestra authentication contexts.
type AuthManager struct {
	configDir  string
	configFile string
}

// NewAuthManager returns an AuthManager with an optional custom config directory.
func NewAuthManager(configDir string) *AuthManager {
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			// Fall back to current directory if we cannot determine the home directory.
			configDir = ".kestractl"
		} else {
			configDir = filepath.Join(home, ".kestractl")
		}
	}

	return &AuthManager{
		configDir:  configDir,
		configFile: filepath.Join(configDir, "config.yaml"),
	}
}

func (m *AuthManager) ensureConfigDir() error {
	return os.MkdirAll(m.configDir, 0o755)
}

func (m *AuthManager) loadConfig() (authConfig, error) {
	if err := m.ensureConfigDir(); err != nil {
		return authConfig{}, err
	}

	cfg := authConfig{
		Contexts:       map[string]AuthContext{},
		DefaultContext: "",
	}

	data, err := os.ReadFile(m.configFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}

	if len(data) == 0 {
		return cfg, nil
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		// Return empty config if the file is invalid
		return authConfig{
			Contexts:       map[string]AuthContext{},
			DefaultContext: "",
		}, nil
	}

	if cfg.Contexts == nil {
		cfg.Contexts = map[string]AuthContext{}
	}

	return cfg, nil
}

func (m *AuthManager) saveConfig(cfg authConfig) error {
	if err := m.ensureConfigDir(); err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	// Restrict file permissions since it stores credentials.
	return os.WriteFile(m.configFile, data, 0o600)
}

// AddContext adds or updates a saved authentication context.
func (m *AuthManager) AddContext(ctx AuthContext) error {
	cfg, err := m.loadConfig()
	if err != nil {
		return err
	}

	if cfg.Contexts == nil {
		cfg.Contexts = map[string]AuthContext{}
	}

	// Store context (Name field is excluded from YAML via yaml:"-" tag)
	cfg.Contexts[ctx.Name] = ctx

	// If this is the first context or no default is set, make it the default
	if len(cfg.Contexts) == 1 || cfg.DefaultContext == "" {
		cfg.DefaultContext = ctx.Name
	}

	return m.saveConfig(cfg)
}

// GetContext returns the requested context or, when name is empty, the default context.
func (m *AuthManager) GetContext(name string) (*AuthContext, error) {
	cfg, err := m.loadConfig()
	if err != nil {
		return nil, err
	}

	if name == "" {
		name = cfg.DefaultContext
		if name == "" {
			return nil, errors.New("no default context configured; use 'kestractl config add' first")
		}
	}

	ctx, ok := cfg.Contexts[name]
	if !ok {
		return nil, fmt.Errorf("context '%s' not found", name)
	}

	// Set the name field (not stored in YAML)
	ctx.Name = name
	return &ctx, nil
}

// SetDefaultContext marks a context as default.
func (m *AuthManager) SetDefaultContext(name string) error {
	cfg, err := m.loadConfig()
	if err != nil {
		return err
	}

	if _, ok := cfg.Contexts[name]; !ok {
		return fmt.Errorf("context '%s' does not exist", name)
	}

	cfg.DefaultContext = name
	return m.saveConfig(cfg)
}

// ListContexts returns all contexts keyed by name.
func (m *AuthManager) ListContexts() (map[string]AuthContext, string, error) {
	cfg, err := m.loadConfig()
	if err != nil {
		return nil, "", err
	}

	result := make(map[string]AuthContext, len(cfg.Contexts))
	for name, ctx := range cfg.Contexts {
		ctx.Name = name // Set the name field (not stored in YAML)
		result[name] = ctx
	}

	return result, cfg.DefaultContext, nil
}

// DeleteContext removes a context by name and unsets the default if necessary.
func (m *AuthManager) DeleteContext(name string) error {
	cfg, err := m.loadConfig()
	if err != nil {
		return err
	}

	if _, ok := cfg.Contexts[name]; !ok {
		return fmt.Errorf("context '%s' does not exist", name)
	}

	delete(cfg.Contexts, name)
	if cfg.DefaultContext == name {
		cfg.DefaultContext = ""
	}

	return m.saveConfig(cfg)
}
