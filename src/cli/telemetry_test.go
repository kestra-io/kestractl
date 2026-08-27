package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	kestra "github.com/kestra-io/client-sdk/go-sdk/v2/kestra_api_client"
)

func TestFetchPosthogConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/config" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":"x","posthog":{"token":"phc_test","apiHost":"https://kestra.io/t/"}}`))
	}))
	defer server.Close()

	originalURL := telemetryConfigURL
	originalClient := telemetryConfigHTTPClient
	telemetryConfigURL = server.URL + "/v1/config"
	telemetryConfigHTTPClient = server.Client()
	defer func() {
		telemetryConfigURL = originalURL
		telemetryConfigHTTPClient = originalClient
	}()

	token, apiHost := fetchPosthogConfig()
	if token != "phc_test" {
		t.Fatalf("expected token phc_test, got %q", token)
	}
	if apiHost != "https://kestra.io/t/" {
		t.Fatalf("expected apiHost https://kestra.io/t/, got %q", apiHost)
	}
}

func TestFetchPosthogConfigFallbacksToEmptyOnError(t *testing.T) {
	t.Run("missing fields", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"posthog":{}}`))
		}))
		defer server.Close()

		originalURL := telemetryConfigURL
		originalClient := telemetryConfigHTTPClient
		telemetryConfigURL = server.URL
		telemetryConfigHTTPClient = server.Client()
		defer func() {
			telemetryConfigURL = originalURL
			telemetryConfigHTTPClient = originalClient
		}()

		token, apiHost := fetchPosthogConfig()
		if token != "" || apiHost != "" {
			t.Fatalf("expected empty token and apiHost, got %q and %q", token, apiHost)
		}
	})

	t.Run("non-2xx", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		defer server.Close()

		originalURL := telemetryConfigURL
		originalClient := telemetryConfigHTTPClient
		telemetryConfigURL = server.URL
		telemetryConfigHTTPClient = server.Client()
		defer func() {
			telemetryConfigURL = originalURL
			telemetryConfigHTTPClient = originalClient
		}()

		token, apiHost := fetchPosthogConfig()
		if token != "" || apiHost != "" {
			t.Fatalf("expected empty token and apiHost, got %q and %q", token, apiHost)
		}
	})
}

func TestLoadOrCreateInstallationID(t *testing.T) {
	dir := t.TempDir()

	first := loadOrCreateInstallationID(dir)
	if first == "" {
		t.Fatal("expected installation id to be created")
	}

	second := loadOrCreateInstallationID(dir)
	if second != first {
		t.Fatalf("expected persisted installation id %q, got %q", first, second)
	}

	stored, err := os.ReadFile(filepath.Join(dir, installationIDFileName))
	if err != nil {
		t.Fatalf("expected installation id file to exist: %v", err)
	}

	if string(stored) == "" {
		t.Fatal("expected installation id file to contain data")
	}
}

func TestDetectCIProvider(t *testing.T) {
	t.Run("specific provider", func(t *testing.T) {
		env := map[string]string{"GITHUB_ACTIONS": "true"}
		isCI, provider := detectCIProviderWithLookup(func(key string) string {
			return env[key]
		})
		if !isCI {
			t.Fatal("expected CI detection to be true")
		}
		if provider != "github_actions" {
			t.Fatalf("expected github_actions provider, got %q", provider)
		}
	})

	t.Run("generic CI", func(t *testing.T) {
		env := map[string]string{"CI": "true"}
		isCI, provider := detectCIProviderWithLookup(func(key string) string {
			return env[key]
		})
		if !isCI {
			t.Fatal("expected CI detection to be true")
		}
		if provider != "generic_ci" {
			t.Fatalf("expected generic_ci provider, got %q", provider)
		}
	})

	t.Run("CI false value", func(t *testing.T) {
		env := map[string]string{"CI": "false"}
		isCI, provider := detectCIProviderWithLookup(func(key string) string {
			return env[key]
		})
		if isCI {
			t.Fatal("expected CI detection to be false")
		}
		if provider != "" {
			t.Fatalf("expected empty provider, got %q", provider)
		}
	})

	t.Run("not in CI", func(t *testing.T) {
		env := map[string]string{}
		isCI, provider := detectCIProviderWithLookup(func(key string) string {
			return env[key]
		})
		if isCI {
			t.Fatal("expected CI detection to be false")
		}
		if provider != "" {
			t.Fatalf("expected empty provider, got %q", provider)
		}
	})
}

func TestDefaultTelemetryProperties(t *testing.T) {
	cfg := kestra.NewMiscControllerEEConfiguration()
	cfg.SetVersion("1.2.3")
	cfg.SetEdition(kestra.EDITIONPROVIDEREDITION_OSS)
	cfg.SetUrl("https://instance.kestra.io")
	cfg.SetUuid("instance-uuid")

	properties := defaultTelemetryProperties("https://host.kestra.io", "main", cfg)

	if properties["host_url"] != "https://host.kestra.io" {
		t.Fatalf("expected host_url, got %v", properties["host_url"])
	}
	if properties["tenant"] != "main" {
		t.Fatalf("expected tenant, got %v", properties["tenant"])
	}
	if properties["os"] != runtime.GOOS {
		t.Fatalf("expected os %q, got %v", runtime.GOOS, properties["os"])
	}
	if properties["arch"] != runtime.GOARCH {
		t.Fatalf("expected arch %q, got %v", runtime.GOARCH, properties["arch"])
	}
	if properties["kestra_version"] != "1.2.3" {
		t.Fatalf("expected kestra_version, got %v", properties["kestra_version"])
	}
	if properties["kestra_edition"] != "OSS" {
		t.Fatalf("expected kestra_edition, got %v", properties["kestra_edition"])
	}
	if properties["kestra_url"] != "https://instance.kestra.io" {
		t.Fatalf("expected kestra_url, got %v", properties["kestra_url"])
	}
	if properties["kestra_uuid"] != "instance-uuid" {
		t.Fatalf("expected kestra_uuid, got %v", properties["kestra_uuid"])
	}
}

func TestTelemetryDisabled(t *testing.T) {
	t.Setenv(telemetryDisabledEnv, "true")
	if !telemetryDisabled() {
		t.Fatal("expected telemetry to be disabled")
	}

	t.Setenv(telemetryDisabledEnv, "false")
	if telemetryDisabled() {
		t.Fatal("expected telemetry to be enabled")
	}
}

func TestTelemetryEventName(t *testing.T) {
	t.Run("default event", func(t *testing.T) {
		t.Setenv(telemetryGitHubActionEnv, "")
		if event := telemetryEventName(); event != telemetryEventCommandDone {
			t.Fatalf("expected %q, got %q", telemetryEventCommandDone, event)
		}
	})

	t.Run("gha event when enabled", func(t *testing.T) {
		t.Setenv(telemetryGitHubActionEnv, "true")
		if event := telemetryEventName(); event != telemetryEventGHADone {
			t.Fatalf("expected %q, got %q", telemetryEventGHADone, event)
		}
	})

	t.Run("default event when disabled", func(t *testing.T) {
		t.Setenv(telemetryGitHubActionEnv, "false")
		if event := telemetryEventName(); event != telemetryEventCommandDone {
			t.Fatalf("expected %q, got %q", telemetryEventCommandDone, event)
		}
	})
}
