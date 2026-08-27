package cli

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	kestra "github.com/kestra-io/client-sdk/go-sdk/v2/kestra_api_client"
	"github.com/posthog/posthog-go"
	"github.com/spf13/viper"
)

const (
	telemetryDisabledEnv      = "KESTRACTL_TELEMETRY_DISABLED"
	telemetryGitHubActionEnv  = "KESTRACTL_GITHUB_ACTION"
	telemetryEventCommandDone = "cli_command_executed"
	telemetryEventGHADone     = "gha_command_executed"
	installationIDFileName    = "installation_id"
)

type telemetryReporter interface {
	CaptureCommand(commandPath string, commandErr error, duration time.Duration)
	Close()
}

type noopTelemetry struct{}

func (noopTelemetry) CaptureCommand(string, error, time.Duration) {}
func (noopTelemetry) Close()                                      {}

type posthogTelemetry struct {
	client     posthog.Client
	distinctID string
}

func (t posthogTelemetry) CaptureCommand(commandPath string, commandErr error, duration time.Duration) {
	properties := posthog.NewProperties().
		Set("command_path", commandPath).
		Set("success", commandErr == nil).
		Set("duration_ms", duration.Milliseconds()).
		Set("$process_person_profile", false)

	if commandErr != nil {
		properties.Set("error_type", telemetryErrorType(commandErr))
	}

	_ = t.client.Enqueue(posthog.Capture{
		DistinctId: t.distinctID,
		Event:      telemetryEventName(),
		Properties: properties,
	})
}

func telemetryEventName() string {
	if envEnabled(os.Getenv(telemetryGitHubActionEnv)) {
		return telemetryEventGHADone
	}

	return telemetryEventCommandDone
}

func (t posthogTelemetry) Close() {
	_ = t.client.Close()
}

var (
	activeTelemetry           telemetryReporter = noopTelemetry{}
	posthogFactory                              = posthog.NewWithConfig
	telemetryConfigURL                          = "https://api.kestra.io/v1/config"
	telemetryConfigHTTPClient                   = &http.Client{Timeout: 2 * time.Second}
)

type apiKestraConfigResponse struct {
	Posthog struct {
		Token   string `json:"token"`
		APIHost string `json:"apiHost"`
	} `json:"posthog"`
}

func initializeTelemetry() {
	activeTelemetry = newTelemetryReporter()
}

func newTelemetryReporter() telemetryReporter {
	if telemetryDisabled() {
		return noopTelemetry{}
	}

	posthogToken, posthogEndpoint := fetchPosthogConfig()
	if posthogToken == "" || posthogEndpoint == "" {
		return noopTelemetry{}
	}

	installationID := loadOrCreateInstallationID(telemetryStateDir())
	if installationID == "" {
		return noopTelemetry{}
	}

	host, tenant, _, _, _, _ := resolveConfig()
	instanceConfig := fetchInstanceConfig()

	phClient, err := posthogFactory(posthogToken, posthog.Config{
		Endpoint:               posthogEndpoint,
		DefaultEventProperties: defaultTelemetryProperties(host, tenant, instanceConfig),
		ShutdownTimeout:        2 * time.Second,
	})
	if err != nil {
		return noopTelemetry{}
	}

	return posthogTelemetry{client: phClient, distinctID: installationID}
}

func fetchPosthogConfig() (string, string) {
	req, err := http.NewRequest(http.MethodGet, telemetryConfigURL, nil)
	if err != nil {
		return "", ""
	}

	res, err := telemetryConfigHTTPClient.Do(req)
	if err != nil {
		return "", ""
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", ""
	}

	var payload apiKestraConfigResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return "", ""
	}

	token := strings.TrimSpace(payload.Posthog.Token)
	apiHost := strings.TrimSpace(payload.Posthog.APIHost)
	if token == "" || apiHost == "" {
		return "", ""
	}

	return token, apiHost
}

func fetchInstanceConfig() *kestra.MiscControllerEEConfiguration {
	client, err := NewClient()
	if err != nil || client.API == nil || client.API.MiscAPI == nil {
		return nil
	}

	instanceConfig, _, err := client.API.MiscAPI.Configuration(client.Ctx).Execute()
	if err != nil {
		return nil
	}

	return instanceConfig
}

func defaultTelemetryProperties(host string, tenant string, cfg *kestra.MiscControllerEEConfiguration) posthog.Properties {
	properties := posthog.NewProperties().
		Set("version", version).
		Set("cli_version", version).
		Set("host_url", host).
		Set("tenant", tenant).
		Set("os", runtime.GOOS).
		Set("arch", runtime.GOARCH)

	isCI, provider := detectCIProvider()
	properties.Set("is_ci", isCI)
	if provider != "" {
		properties.Set("ci_provider", provider)
	}

	if cfg == nil {
		return properties
	}

	if cfg.HasVersion() {
		properties.Set("kestra_version", cfg.GetVersion())
	}
	if cfg.HasEdition() {
		properties.Set("kestra_edition", string(cfg.GetEdition()))
	}
	if cfg.HasUrl() {
		properties.Set("kestra_url", cfg.GetUrl())
	}
	if cfg.HasUuid() {
		properties.Set("kestra_uuid", cfg.GetUuid())
	}

	return properties
}

func telemetryStateDir() string {
	if cfgFile := strings.TrimSpace(viper.GetString(FlagConfig)); cfgFile != "" {
		return filepath.Dir(cfgFile)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ".kestractl"
	}

	return filepath.Join(home, ".kestractl")
}

func loadOrCreateInstallationID(stateDir string) string {
	if strings.TrimSpace(stateDir) == "" {
		return newInstallationID()
	}

	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return newInstallationID()
	}

	path := filepath.Join(stateDir, installationIDFileName)
	data, err := os.ReadFile(path)
	if err == nil {
		if installationID := strings.TrimSpace(string(data)); installationID != "" {
			return installationID
		}
	} else if !os.IsNotExist(err) {
		return newInstallationID()
	}

	installationID := newInstallationID()
	if installationID == "" {
		return ""
	}

	if writeErr := os.WriteFile(path, []byte(installationID+"\n"), 0o600); writeErr != nil {
		return installationID
	}

	return installationID
}

func newInstallationID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return ""
	}

	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80

	encoded := hex.EncodeToString(bytes[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", encoded[0:8], encoded[8:12], encoded[12:16], encoded[16:20], encoded[20:32])
}

func telemetryDisabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(telemetryDisabledEnv)))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func telemetryErrorType(err error) string {
	if err == nil {
		return ""
	}

	if _, ok := err.(*kestra.GenericOpenAPIError); ok {
		return "api_error"
	}

	return "cli_error"
}

func detectCIProvider() (bool, string) {
	return detectCIProviderWithLookup(os.Getenv)
}

func detectCIProviderWithLookup(lookup func(string) string) (bool, string) {
	checks := []struct {
		env      string
		provider string
	}{
		{env: "GITHUB_ACTIONS", provider: "github_actions"},
		{env: "GITLAB_CI", provider: "gitlab_ci"},
		{env: "JENKINS_URL", provider: "jenkins"},
		{env: "BUILDKITE", provider: "buildkite"},
		{env: "CIRCLECI", provider: "circleci"},
		{env: "BITBUCKET_BUILD_NUMBER", provider: "bitbucket_pipelines"},
		{env: "TF_BUILD", provider: "azure_pipelines"},
		{env: "TEAMCITY_VERSION", provider: "teamcity"},
		{env: "TRAVIS", provider: "travis_ci"},
		{env: "DRONE", provider: "drone"},
	}

	for _, check := range checks {
		if envEnabled(lookup(check.env)) {
			return true, check.provider
		}
	}

	if envEnabled(lookup("CI")) {
		return true, "generic_ci"
	}

	return false, ""
}

func envEnabled(value string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "" {
		return false
	}

	return trimmed != "0" && trimmed != "false" && trimmed != "no" && trimmed != "off"
}
