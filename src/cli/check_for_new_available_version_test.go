package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
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
	prevGrace := newVersionCheckGrace
	version = cliVersion
	newVersionCheckNow = func() time.Time { return now }
	newVersionCheckCIDetection = func() (bool, string) { return false, "" }
	t.Cleanup(func() {
		version = prevVersion
		newVersionCheckNow = prevNow
		newVersionCheckCIDetection = prevCI
		newVersionCheckGrace = prevGrace
		activeNewVersionCheck = nil
	})

	return filepath.Join(stateDir, ".kestractl")
}

// stubLatestRelease points the version check at a local server and counts hits.
// The optional delay simulates a slow network.
func stubLatestRelease(t *testing.T, tag string, delay time.Duration) *atomic.Int64 {
	t.Helper()

	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if delay > 0 {
			time.Sleep(delay)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"` + tag + `"}`))
	}))
	t.Cleanup(server.Close)

	prevURL := newVersionCheckLatestReleaseURL
	newVersionCheckLatestReleaseURL = server.URL
	t.Cleanup(func() { newVersionCheckLatestReleaseURL = prevURL })

	return &calls
}

// runNewVersionCheck performs a full check the way Execute does: start it, then
// collect the result once the "command" is done.
func runNewVersionCheck(out *bytes.Buffer) {
	startNewVersionCheck(out)
	awaitNewVersionCheck()
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
	calls := stubLatestRelease(t, "v1.3.0", 0)

	var out bytes.Buffer
	runNewVersionCheck(&out)

	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 network call, got %d", got)
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
	calls := stubLatestRelease(t, "v9.9.9", 0)
	writeNewVersionCheckState(t, stateDir, newVersionCheckState{
		LastCheck:     now.Add(-2 * time.Hour),
		LatestVersion: "v1.3.0",
	})

	var out bytes.Buffer
	runNewVersionCheck(&out)

	if got := calls.Load(); got != 0 {
		t.Fatalf("expected no network call within the interval, got %d", got)
	}
	if !strings.Contains(out.String(), "v1.3.0") {
		t.Errorf("expected cached version in warning, got %q", out.String())
	}
}

func TestCheckForNewAvailableVersion_RefetchesAfterInterval(t *testing.T) {
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	stateDir := withNewVersionCheckEnv(t, "1.2.0", now)
	calls := stubLatestRelease(t, "v1.4.0", 0)
	writeNewVersionCheckState(t, stateDir, newVersionCheckState{
		LastCheck:     now.Add(-25 * time.Hour),
		LatestVersion: "v1.3.0",
	})

	var out bytes.Buffer
	runNewVersionCheck(&out)

	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 network call after the interval, got %d", got)
	}
	if state := readNewVersionCheckState(t, stateDir); state.LatestVersion != "v1.4.0" {
		t.Errorf("expected refreshed version to be cached, got %q", state.LatestVersion)
	}
	// The stale cache already told the user about v1.3.0, so the notice must not
	// be printed a second time for v1.4.0 in the same run.
	if n := strings.Count(out.String(), "A new version of kestractl is available"); n != 1 {
		t.Errorf("expected exactly one notice, got %d in %q", n, out.String())
	}
}

func TestStartNewVersionCheck_ReturnsBeforeSlowFetchCompletes(t *testing.T) {
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	withNewVersionCheckEnv(t, "1.2.0", now)
	stubLatestRelease(t, "v1.3.0", 300*time.Millisecond)

	var out bytes.Buffer
	start := time.Now()
	startNewVersionCheck(&out)
	elapsed := time.Since(start)

	if elapsed > 100*time.Millisecond {
		t.Errorf("startNewVersionCheck blocked for %v; it must not wait on the network", elapsed)
	}
	if activeNewVersionCheck == nil {
		t.Fatal("expected a refresh to be in flight")
	}

	awaitNewVersionCheck()
	if !strings.Contains(out.String(), "v1.3.0") {
		t.Errorf("expected the notice once the refresh landed, got %q", out.String())
	}
}

func TestStartNewVersionCheck_StampsThrottleBeforeFetchCompletes(t *testing.T) {
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	stateDir := withNewVersionCheckEnv(t, "1.2.0", now)

	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		_, _ = w.Write([]byte(`{"tag_name":"v1.3.0"}`))
	}))
	defer server.Close()
	prevURL := newVersionCheckLatestReleaseURL
	newVersionCheckLatestReleaseURL = server.URL
	defer func() { newVersionCheckLatestReleaseURL = prevURL }()

	var out bytes.Buffer
	startNewVersionCheck(&out)

	// The 24h throttle must already hold while the request is still in flight,
	// otherwise a killed process would refetch on every command.
	state := readNewVersionCheckState(t, stateDir)
	if !state.LastCheck.Equal(now) {
		t.Errorf("expected the attempt to be stamped before the response, got %v", state.LastCheck)
	}

	close(release)
	awaitNewVersionCheck()
}

func TestAwaitNewVersionCheck_GivesUpAfterGrace(t *testing.T) {
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	stateDir := withNewVersionCheckEnv(t, "1.2.0", now)
	stubLatestRelease(t, "v1.3.0", 300*time.Millisecond)
	newVersionCheckGrace = 10 * time.Millisecond

	var out bytes.Buffer
	startNewVersionCheck(&out)
	pending := activeNewVersionCheck

	start := time.Now()
	awaitNewVersionCheck()
	elapsed := time.Since(start)

	if elapsed > 150*time.Millisecond {
		t.Errorf("await waited %v; it must give up after the grace period", elapsed)
	}
	if out.String() != "" {
		t.Errorf("expected no notice when the refresh misses the grace period, got %q", out.String())
	}
	// The throttle still holds, so the abandoned refresh costs nothing.
	if state := readNewVersionCheckState(t, stateDir); !state.LastCheck.Equal(now) {
		t.Errorf("expected the attempt to stay stamped, got %v", state.LastCheck)
	}

	<-pending.done // let the goroutine finish before the test restores globals
}

func TestCheckForNewAvailableVersion_SilentWhenUpToDate(t *testing.T) {
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	withNewVersionCheckEnv(t, "1.3.0", now)
	stubLatestRelease(t, "v1.3.0", 0)

	var out bytes.Buffer
	runNewVersionCheck(&out)

	if out.String() != "" {
		t.Errorf("expected no output when up to date, got %q", out.String())
	}
}

func TestCheckForNewAvailableVersion_SilentWhenAheadOfLatest(t *testing.T) {
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	withNewVersionCheckEnv(t, "2.0.0", now)
	stubLatestRelease(t, "v1.3.0", 0)

	var out bytes.Buffer
	runNewVersionCheck(&out)

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
	runNewVersionCheck(&out)

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

func TestCheckForNewAvailableVersion_FailedRefreshKeepsCachedVersion(t *testing.T) {
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	stateDir := withNewVersionCheckEnv(t, "1.2.0", now)
	writeNewVersionCheckState(t, stateDir, newVersionCheckState{
		LastCheck:     now.Add(-25 * time.Hour),
		LatestVersion: "v1.3.0",
	})

	prevURL := newVersionCheckLatestReleaseURL
	newVersionCheckLatestReleaseURL = "http://127.0.0.1:0/nope"
	t.Cleanup(func() { newVersionCheckLatestReleaseURL = prevURL })

	var out bytes.Buffer
	runNewVersionCheck(&out)

	// The stale-but-known version is still worth showing, and worth keeping.
	if !strings.Contains(out.String(), "v1.3.0") {
		t.Errorf("expected the cached version to still be reported, got %q", out.String())
	}
	if state := readNewVersionCheckState(t, stateDir); state.LatestVersion != "v1.3.0" {
		t.Errorf("expected the cached version to survive a failed refresh, got %q", state.LatestVersion)
	}
}

func TestCheckForNewAvailableVersion_DisabledByEnv(t *testing.T) {
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	withNewVersionCheckEnv(t, "1.2.0", now)
	calls := stubLatestRelease(t, "v1.3.0", 0)
	t.Setenv(newVersionCheckDisabledEnv, "true")

	var out bytes.Buffer
	runNewVersionCheck(&out)

	if calls.Load() != 0 || out.String() != "" {
		t.Errorf("expected the check to be fully disabled, got %d calls and %q", calls.Load(), out.String())
	}
}

func TestCheckForNewAvailableVersion_SkippedForDevBuild(t *testing.T) {
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	withNewVersionCheckEnv(t, "dev", now)
	calls := stubLatestRelease(t, "v1.3.0", 0)

	var out bytes.Buffer
	runNewVersionCheck(&out)

	if calls.Load() != 0 || out.String() != "" {
		t.Errorf("expected dev builds to skip the check, got %d calls and %q", calls.Load(), out.String())
	}
}

func TestCheckForNewAvailableVersion_SkippedInCI(t *testing.T) {
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	withNewVersionCheckEnv(t, "1.2.0", now)
	calls := stubLatestRelease(t, "v1.3.0", 0)
	newVersionCheckCIDetection = func() (bool, string) { return true, "github_actions" }

	var out bytes.Buffer
	runNewVersionCheck(&out)

	if calls.Load() != 0 || out.String() != "" {
		t.Errorf("expected CI runs to skip the check, got %d calls and %q", calls.Load(), out.String())
	}
}

func TestAwaitNewVersionCheck_NoPendingCheckIsNoop(t *testing.T) {
	activeNewVersionCheck = nil
	awaitNewVersionCheck() // must not panic or block
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

func TestSaveNewVersionCheckState_WritesPrivateFile(t *testing.T) {
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	stateDir := withNewVersionCheckEnv(t, "1.2.0", now)

	saveNewVersionCheckState(stateDir, newVersionCheckState{LastCheck: now, LatestVersion: "v1.3.0"})

	info, err := os.Stat(filepath.Join(stateDir, newVersionCheckFileName))
	if err != nil {
		t.Fatalf("stat state file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("expected 0600 permissions, got %o", perm)
	}

	entries, err := os.ReadDir(stateDir)
	if err != nil {
		t.Fatalf("read state dir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected the temp file to be renamed away, found %d entries", len(entries))
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
