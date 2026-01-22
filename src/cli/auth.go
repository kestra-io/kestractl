package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// AuthContext describes an authentication context for Kestra.
type AuthContext struct {
	Name       string `json:"name"`
	Host       string `json:"host"`
	Tenant     string `json:"tenant"`
	AuthMethod string `json:"auth_method"`
	Token      string `json:"token,omitempty"`
	Username   string `json:"username,omitempty"`
	Password   string `json:"password,omitempty"`
}

// authConfigEntry is the internal storage format where the name is stored as the map key, not as a field.
type authConfigEntry struct {
	Host       string `json:"host"`
	Tenant     string `json:"tenant"`
	AuthMethod string `json:"auth_method"`
	Token      string `json:"token,omitempty"`
	Username   string `json:"username,omitempty"`
	Password   string `json:"password,omitempty"`
}

type authConfig struct {
	Contexts       map[string]authConfigEntry `json:"contexts"`
	DefaultContext string                     `json:"default_context"`
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
			configDir = ".kestra"
		} else {
			configDir = filepath.Join(home, ".kestra")
		}
	}

	return &AuthManager{
		configDir:  configDir,
		configFile: filepath.Join(configDir, "config"),
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
		Contexts:       map[string]authConfigEntry{},
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

	if err := json.Unmarshal(data, &cfg); err != nil {
		// Return empty config if the file is invalid to match Python behaviour.
		return authConfig{
			Contexts:       map[string]authConfigEntry{},
			DefaultContext: "",
		}, nil
	}

	if cfg.Contexts == nil {
		cfg.Contexts = map[string]authConfigEntry{}
	}

	return cfg, nil
}

func (m *AuthManager) saveConfig(cfg authConfig) error {
	if err := m.ensureConfigDir(); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
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
		cfg.Contexts = map[string]authConfigEntry{}
	}

	cfg.Contexts[ctx.Name] = authConfigEntry{
		Host:       ctx.Host,
		Tenant:     ctx.Tenant,
		AuthMethod: ctx.AuthMethod,
		Token:      ctx.Token,
		Username:   ctx.Username,
		Password:   ctx.Password,
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
			return nil, errors.New("no default context configured; use 'kestra config add' first")
		}
	}

	entry, ok := cfg.Contexts[name]
	if !ok {
		return nil, fmt.Errorf("context '%s' not found", name)
	}

	return &AuthContext{
		Name:       name,
		Host:       entry.Host,
		Tenant:     entry.Tenant,
		AuthMethod: entry.AuthMethod,
		Token:      entry.Token,
		Username:   entry.Username,
		Password:   entry.Password,
	}, nil
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
	for name, entry := range cfg.Contexts {
		result[name] = AuthContext{
			Name:       name,
			Host:       entry.Host,
			Tenant:     entry.Tenant,
			AuthMethod: entry.AuthMethod,
			Token:      entry.Token,
			Username:   entry.Username,
			Password:   entry.Password,
		}
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
