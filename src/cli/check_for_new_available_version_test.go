package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
)

// withNewVersionCheckEnv isolates the state dir, version and clock for a test.
func withNewVersionCheckEnv(t *testing.T, cliVersion string, now time.Time) string {
	t.Helper()

	stateDir := t.TempDir()
	t.Setenv("HOME", stateDir)
	t.Setenv(newVersionCheckDisabledEnv, "")

	// telemetryStateDir() prefers --config; make sure no other test leaked one.
	prevConfig := viper.GetString(FlagConfig)
	viper.Set(FlagConfig, "")
	t.Cleanup(func() { viper.Set(FlagConfig, prevConfig) })

	prevVersion := version
	prevNow := newVersionCheckNow
	prevCI := newVersionCheckCIDetection
	version = cliVersion
	newVersionCheckNow = func() time.Time { return now }
	newVersionCheckCIDetection = func() (bool, string) { return false, "" }
	t.Cleanup(func() {
		version = prevVersion
		newVersionCheckNow = prevNow
		newVersionCheckCIDetection = prevCI
	})

	return filepath.Join(stateDir, ".kestractl")
}

// stubLatestRelease points the update check at a local server and counts hits.
func stubLatestRelease(t *testing.T, tag string) *int {
	t.Helper()

	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"` + tag + `"}`))
	}))
	t.Cleanup(server.Close)

	prevURL := newVersionCheckLatestReleaseURL
	newVersionCheckLatestReleaseURL = server.URL
	t.Cleanup(func() { newVersionCheckLatestReleaseURL = prevURL })

	return &calls
}

func writeNewVersionCheckState(t *testing.T, stateDir string, state newVersionCheckState) {
	t.Helper()

	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, newVersionCheckFileName), data, 0o600); err != nil {
		t.Fatalf("write state: %v", err)
	}
}

func readNewVersionCheckState(t *testing.T, stateDir string) newVersionCheckState {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(stateDir, newVersionCheckFileName))
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	var state newVersionCheckState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	return state
}

func TestCheckForNewAvailableVersion_FirstRunFetchesAndWarns(t *testing.T) {
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	stateDir := withNewVersionCheckEnv(t, "1.2.0", now)
	calls := stubLatestRelease(t, "v1.3.0")

	var out bytes.Buffer
	checkForNewAvailableVersion(&out)

	if *calls != 1 {
		t.Fatalf("expected 1 network call, got %d", *calls)
	}
	if !strings.Contains(out.String(), "v1.3.0") {
		t.Errorf("expected warning to mention v1.3.0, got %q", out.String())
	}
	if !strings.Contains(out.String(), releasesDownloadURL) {
		t.Errorf("expected warning to link the releases page, got %q", out.String())
	}

	state := readNewVersionCheckState(t, stateDir)
	if !state.LastCheck.Equal(now) {
		t.Errorf("expected last check %v, got %v", now, state.LastCheck)
	}
	if state.LatestVersion != "v1.3.0" {
		t.Errorf("expected cached version v1.3.0, got %q", state.LatestVersion)
	}
}

func TestCheckForNewAvailableVersion_WithinIntervalUsesCacheWithoutNetwork(t *testing.T) {
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	stateDir := withNewVersionCheckEnv(t, "1.2.0", now)
	calls := stubLatestRelease(t, "v9.9.9")
	writeNewVersionCheckState(t, stateDir, newVersionCheckState{
		LastCheck:     now.Add(-2 * time.Hour),
		LatestVersion: "v1.3.0",
	})

	var out bytes.Buffer
	checkForNewAvailableVersion(&out)

	if *calls != 0 {
		t.Fatalf("expected no network call within the interval, got %d", *calls)
	}
	if !strings.Contains(out.String(), "v1.3.0") {
		t.Errorf("expected cached version in warning, got %q", out.String())
	}
}

func TestCheckForNewAvailableVersion_RefetchesAfterInterval(t *testing.T) {
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	stateDir := withNewVersionCheckEnv(t, "1.2.0", now)
	calls := stubLatestRelease(t, "v1.4.0")
	writeNewVersionCheckState(t, stateDir, newVersionCheckState{
		LastCheck:     now.Add(-25 * time.Hour),
		LatestVersion: "v1.3.0",
	})

	var out bytes.Buffer
	checkForNewAvailableVersion(&out)

	if *calls != 1 {
		t.Fatalf("expected 1 network call after the interval, got %d", *calls)
	}
	if !strings.Contains(out.String(), "v1.4.0") {
		t.Errorf("expected refreshed version in warning, got %q", out.String())
	}
}

func TestCheckForNewAvailableVersion_SilentWhenUpToDate(t *testing.T) {
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	withNewVersionCheckEnv(t, "1.3.0", now)
	stubLatestRelease(t, "v1.3.0")

	var out bytes.Buffer
	checkForNewAvailableVersion(&out)

	if out.String() != "" {
		t.Errorf("expected no output when up to date, got %q", out.String())
	}
}

func TestCheckForNewAvailableVersion_SilentWhenAheadOfLatest(t *testing.T) {
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	withNewVersionCheckEnv(t, "2.0.0", now)
	stubLatestRelease(t, "v1.3.0")

	var out bytes.Buffer
	checkForNewAvailableVersion(&out)

	if out.String() != "" {
		t.Errorf("expected no output when ahead of latest, got %q", out.String())
	}
}

func TestCheckForNewAvailableVersion_NetworkFailureIsSilentAndStampsCheck(t *testing.T) {
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	stateDir := withNewVersionCheckEnv(t, "1.2.0", now)

	prevURL := newVersionCheckLatestReleaseURL
	newVersionCheckLatestReleaseURL = "http://127.0.0.1:0/nope"
	t.Cleanup(func() { newVersionCheckLatestReleaseURL = prevURL })

	var out bytes.Buffer
	checkForNewAvailableVersion(&out)

	if out.String() != "" {
		t.Errorf("expected silence on network failure, got %q", out.String())
	}

	// The attempt must still be stamped so the next command does not retry.
	state := readNewVersionCheckState(t, stateDir)
	if !state.LastCheck.Equal(now) {
		t.Errorf("expected failed attempt to be stamped at %v, got %v", now, state.LastCheck)
	}
	if state.LatestVersion != "" {
		t.Errorf("expected no cached version after failure, got %q", state.LatestVersion)
	}
}

func TestCheckForNewAvailableVersion_DisabledByEnv(t *testing.T) {
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	withNewVersionCheckEnv(t, "1.2.0", now)
	calls := stubLatestRelease(t, "v1.3.0")
	t.Setenv(newVersionCheckDisabledEnv, "true")

	var out bytes.Buffer
	checkForNewAvailableVersion(&out)

	if *calls != 0 || out.String() != "" {
		t.Errorf("expected the check to be fully disabled, got %d calls and %q", *calls, out.String())
	}
}

func TestCheckForNewAvailableVersion_SkippedForDevBuild(t *testing.T) {
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	withNewVersionCheckEnv(t, "dev", now)
	calls := stubLatestRelease(t, "v1.3.0")

	var out bytes.Buffer
	checkForNewAvailableVersion(&out)

	if *calls != 0 || out.String() != "" {
		t.Errorf("expected dev builds to skip the check, got %d calls and %q", *calls, out.String())
	}
}

func TestCheckForNewAvailableVersion_SkippedInCI(t *testing.T) {
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	withNewVersionCheckEnv(t, "1.2.0", now)
	calls := stubLatestRelease(t, "v1.3.0")
	newVersionCheckCIDetection = func() (bool, string) { return true, "github_actions" }

	var out bytes.Buffer
	checkForNewAvailableVersion(&out)

	if *calls != 0 || out.String() != "" {
		t.Errorf("expected CI runs to skip the check, got %d calls and %q", *calls, out.String())
	}
}

func TestLoadNewVersionCheckState_FutureTimestampIsStale(t *testing.T) {
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	stateDir := withNewVersionCheckEnv(t, "1.2.0", now)
	writeNewVersionCheckState(t, stateDir, newVersionCheckState{
		LastCheck:     now.Add(48 * time.Hour),
		LatestVersion: "v1.3.0",
	})

	if _, fresh := loadNewVersionCheckState(stateDir); fresh {
		t.Error("expected a future timestamp to be treated as stale")
	}
}

func TestLoadNewVersionCheckState_CorruptFileIsStale(t *testing.T) {
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	stateDir := withNewVersionCheckEnv(t, "1.2.0", now)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, newVersionCheckFileName), []byte("not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, fresh := loadNewVersionCheckState(stateDir); fresh {
		t.Error("expected a corrupt state file to be treated as stale")
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"v1.3.0", "1.2.0", 1},
		{"1.2.0", "v1.3.0", -1},
		{"v1.2.0", "1.2.0", 0},
		{"1.2.10", "1.2.9", 1},
		{"2.0.0", "1.99.99", 1},
		{"1.3.0", "1.3.0-rc1", 1},
		{"1.3.0-rc1", "1.3.0", -1},
		{"1.3.0-rc2", "1.3.0-rc1", 1},
		{"1.3.0+build5", "1.3.0", 0},
		{"1.3", "1.2.9", 1},
		{"garbage", "1.0.0", -1},
	}

	for _, tt := range tests {
		if got := compareVersions(tt.a, tt.b); got != tt.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestNormalizeVersion(t *testing.T) {
	tests := map[string]string{
		"1.2.3":  "v1.2.3",
		"v1.2.3": "v1.2.3",
		" 1.2.3": "v1.2.3",
		"":       "",
	}

	for in, want := range tests {
		if got := normalizeVersion(in); got != want {
			t.Errorf("normalizeVersion(%q) = %q, want %q", in, got, want)
		}
	}
}
