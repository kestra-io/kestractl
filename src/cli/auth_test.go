package cli

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spf13/viper"
)

func TestAuthManagerPersistsHeaders(t *testing.T) {
	manager := NewAuthManager(t.TempDir())

	err := manager.AddContext(AuthContext{
		Name:       "ose",
		Host:       "https://example.dev.kestra.io",
		Tenant:     "main",
		AuthMethod: "basicAuth",
		Username:   "user@kestra.io",
		Password:   "Root!1234",
		Headers: []string{
			"Cookie:TRAEFIK_AUTH=token",
			"X-Forwarded-User:user@kestra.io",
		},
	})
	if err != nil {
		t.Fatalf("AddContext returned error: %v", err)
	}

	ctx, err := manager.GetContext("ose")
	if err != nil {
		t.Fatalf("GetContext returned error: %v", err)
	}

	want := []string{
		"Cookie:TRAEFIK_AUTH=token",
		"X-Forwarded-User:user@kestra.io",
	}
	if !reflect.DeepEqual(ctx.Headers, want) {
		t.Fatalf("expected headers %v, got %v", want, ctx.Headers)
	}
	if ctx.Name != "ose" {
		t.Fatalf("expected context name to be restored, got %q", ctx.Name)
	}
}

func TestInitializeConfigLoadsHeadersFromFlag(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	root := NewRootCommand()
	if err := root.PersistentFlags().Set(FlagHeader, "Cookie:TRAEFIK_AUTH=token"); err != nil {
		t.Fatalf("failed to set first header flag: %v", err)
	}
	if err := root.PersistentFlags().Set(FlagHeader, "X-Forwarded-User:user@kestra.io"); err != nil {
		t.Fatalf("failed to set second header flag: %v", err)
	}

	if err := initializeConfig(root); err != nil {
		t.Fatalf("initializeConfig returned error: %v", err)
	}

	want := []string{"Cookie:TRAEFIK_AUTH=token", "X-Forwarded-User:user@kestra.io"}
	if !reflect.DeepEqual(globalFlags.Headers, want) {
		t.Fatalf("expected headers %v, got %v", want, globalFlags.Headers)
	}
}

func TestInitializeConfigLoadsHeadersFromDefaultContext(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	configDir := t.TempDir()
	manager := NewAuthManager(configDir)
	err := manager.AddContext(AuthContext{
		Name:       "ose",
		Host:       "https://example.dev.kestra.io",
		Tenant:     "main",
		AuthMethod: "token",
		Token:      "secret-token",
		Headers: []string{
			"Cookie:TRAEFIK_AUTH=token",
		},
	})
	if err != nil {
		t.Fatalf("AddContext returned error: %v", err)
	}

	root := NewRootCommand()
	if err := root.PersistentFlags().Set(FlagConfig, filepath.Join(configDir, "config.yaml")); err != nil {
		t.Fatalf("failed to set config flag: %v", err)
	}

	if err := initializeConfig(root); err != nil {
		t.Fatalf("initializeConfig returned error: %v", err)
	}

	want := []string{"Cookie:TRAEFIK_AUTH=token"}
	if !reflect.DeepEqual(globalFlags.Headers, want) {
		t.Fatalf("expected headers %v, got %v", want, globalFlags.Headers)
	}
	if got := viper.GetStringSlice(FlagHeader); !reflect.DeepEqual(got, want) {
		t.Fatalf("expected viper headers %v, got %v", want, got)
	}
}
