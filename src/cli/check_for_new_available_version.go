package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	newVersionCheckDisabledEnv = "KESTRACTL_VERSION_CHECK_DISABLED"
	newVersionCheckFileName    = "check_for_new_available_version.json"
	newVersionCheckInterval    = 24 * time.Hour
	releasesDownloadURL        = "https://github.com/kestra-io/kestractl/releases/latest"
)

var (
	newVersionCheckLatestReleaseURL = "https://api.github.com/repos/kestra-io/kestractl/releases/latest"
	newVersionCheckHTTPClient       = &http.Client{Timeout: 2 * time.Second}
	newVersionCheckNow              = time.Now
	newVersionCheckCIDetection      = detectCIProvider

	// newVersionCheckGrace bounds how long awaitNewVersionCheck waits for an
	// in-flight refresh once the command itself is done. Kept short on purpose:
	// the 24h throttle is already stamped when the refresh starts, so giving up
	// here costs at most a notice that shows one command later instead.
	newVersionCheckGrace = 500 * time.Millisecond
)

// newVersionCheckState is the on-disk cache backing the once-a-day update check.
// LastCheck is stamped before the network call so a failing call does not make
// every subsequent command retry.
type newVersionCheckState struct {
	LastCheck     time.Time `json:"last_check"`
	LatestVersion string    `json:"latest_version,omitempty"`
}

// pendingNewVersionCheck tracks a refresh running concurrently with the command.
type pendingNewVersionCheck struct {
	out    io.Writer
	done   chan string // receives the fetched version, or "" on failure
	warned bool        // a cached notice was already printed, do not repeat it
}

var activeNewVersionCheck *pendingNewVersionCheck

// startNewVersionCheck warns on stderr when a newer kestractl release exists,
// without ever making the user wait for the network.
//
// The remote lookup happens at most once per newVersionCheckInterval and its
// result is cached in <state dir>/check_for_new_available_version.json. On a
// normal run this function only reads that small file, prints the notice if the
// cached version is newer, and returns — no network at all.
//
// When the cache is stale it hands the lookup to a goroutine and returns
// immediately, so the request overlaps the command the user actually asked for
// instead of delaying it. awaitNewVersionCheck picks the result up afterwards.
// Any failure is silent.
func startNewVersionCheck(out io.Writer) {
	activeNewVersionCheck = nil

	if newVersionCheckSkipped() {
		return
	}

	stateDir := telemetryStateDir()
	state, fresh := loadNewVersionCheckState(stateDir)
	warned := warnIfNewerVersion(out, state.LatestVersion)

	if fresh {
		return
	}

	// Stamp the attempt up front, carrying the previously known version over.
	// Doing it here rather than after the response keeps the once-a-day
	// guarantee even if the process exits before the goroutine finishes.
	saveNewVersionCheckState(stateDir, newVersionCheckState{
		LastCheck:     newVersionCheckNow(),
		LatestVersion: state.LatestVersion,
	})

	pending := &pendingNewVersionCheck{out: out, done: make(chan string, 1), warned: warned}
	activeNewVersionCheck = pending

	go func() {
		latest := fetchLatestVersion()
		if latest != "" {
			saveNewVersionCheckState(stateDir, newVersionCheckState{
				LastCheck:     newVersionCheckNow(),
				LatestVersion: latest,
			})
		}
		pending.done <- latest
	}()
}

// awaitNewVersionCheck collects the result of a refresh started by
// startNewVersionCheck, waiting at most newVersionCheckGrace for it.
//
// By the time this runs the command has already done its work, so the refresh
// has usually finished and the wait is zero. If it has not, we give up rather
// than hold the process open: the cache write is best-effort and the notice
// simply appears on the next run.
func awaitNewVersionCheck() {
	pending := activeNewVersionCheck
	activeNewVersionCheck = nil
	if pending == nil {
		return
	}

	timer := time.NewTimer(newVersionCheckGrace)
	defer timer.Stop()

	select {
	case latest := <-pending.done:
		if !pending.warned {
			warnIfNewerVersion(pending.out, latest)
		}
	case <-timer.C:
	}
}

// warnIfNewerVersion prints the upgrade notice when latest is newer than the
// running build, and reports whether it printed anything.
func warnIfNewerVersion(out io.Writer, latest string) bool {
	if latest == "" || compareVersions(latest, version) <= 0 {
		return false
	}

	fmt.Fprintf(out, "\nA new version of kestractl is available: %s (you are on v%s)\n", normalizeVersion(latest), strings.TrimPrefix(version, "v"))
	fmt.Fprintf(out, "Download it from %s\n\n", releasesDownloadURL)
	return true
}

// newVersionCheckSkipped reports whether the update check should not run at all.
func newVersionCheckSkipped() bool {
	if envEnabled(os.Getenv(newVersionCheckDisabledEnv)) {
		return true
	}

	// Dev/local builds have no meaningful version to compare against.
	if version == "" || version == "dev" {
		return true
	}

	// CI runs are non-interactive and pinned to a version on purpose.
	if isCI, _ := newVersionCheckCIDetection(); isCI {
		return true
	}

	return false
}

// loadNewVersionCheckState reads the cached state and reports whether it is still
// within newVersionCheckInterval.
func loadNewVersionCheckState(stateDir string) (newVersionCheckState, bool) {
	if strings.TrimSpace(stateDir) == "" {
		return newVersionCheckState{}, false
	}

	data, err := os.ReadFile(filepath.Join(stateDir, newVersionCheckFileName))
	if err != nil {
		return newVersionCheckState{}, false
	}

	var state newVersionCheckState
	if err := json.Unmarshal(data, &state); err != nil {
		return newVersionCheckState{}, false
	}

	if state.LastCheck.IsZero() {
		return state, false
	}

	elapsed := newVersionCheckNow().Sub(state.LastCheck)
	// A clock moving backwards (or a state file written in the future) must not
	// pin the cache as fresh forever.
	if elapsed < 0 {
		return state, false
	}

	return state, elapsed < newVersionCheckInterval
}

func saveNewVersionCheckState(stateDir string, state newVersionCheckState) {
	if strings.TrimSpace(stateDir) == "" {
		return
	}

	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return
	}

	data, err := json.Marshal(state)
	if err != nil {
		return
	}

	// Write through a temp file: the refresh goroutine can still be writing when
	// the process exits, and a half-written cache would be read back as corrupt.
	path := filepath.Join(stateDir, newVersionCheckFileName)
	tmp, err := os.CreateTemp(stateDir, newVersionCheckFileName+".*")
	if err != nil {
		return
	}

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		_ = os.Remove(tmp.Name())
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		_ = os.Remove(tmp.Name())
		return
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
	}
}

type githubReleaseResponse struct {
	TagName string `json:"tag_name"`
}

// fetchLatestVersion returns the latest published release tag, or "" on any failure.
func fetchLatestVersion() string {
	req, err := http.NewRequest(http.MethodGet, newVersionCheckLatestReleaseURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "kestractl/"+version)

	res, err := newVersionCheckHTTPClient.Do(req)
	if err != nil {
		return ""
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return ""
	}

	var payload githubReleaseResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return ""
	}

	return strings.TrimSpace(payload.TagName)
}

func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return v
}

// compareVersions compares two semver-ish versions, returning >0 when a is
// newer than b, <0 when older, and 0 when equal. A "v" prefix is optional and
// a pre-release suffix (1.2.0-rc1) sorts below its release (1.2.0).
func compareVersions(a, b string) int {
	aCore, aPre := splitVersion(a)
	bCore, bPre := splitVersion(b)

	for i := 0; i < 3; i++ {
		if aCore[i] != bCore[i] {
			if aCore[i] > bCore[i] {
				return 1
			}
			return -1
		}
	}

	switch {
	case aPre == bPre:
		return 0
	case aPre == "":
		return 1
	case bPre == "":
		return -1
	case aPre > bPre:
		return 1
	default:
		return -1
	}
}

// splitVersion parses "v1.2.3-rc1" into [1 2 3] and "rc1". Unparseable segments
// count as 0, which makes a malformed version compare as older than any real one.
func splitVersion(v string) ([3]int, string) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")

	// Drop build metadata, then split off any pre-release suffix.
	if idx := strings.Index(v, "+"); idx >= 0 {
		v = v[:idx]
	}

	pre := ""
	if idx := strings.Index(v, "-"); idx >= 0 {
		pre = v[idx+1:]
		v = v[:idx]
	}

	var parts [3]int
	for i, segment := range strings.SplitN(v, ".", 3) {
		if i > 2 {
			break
		}
		n, err := strconv.Atoi(strings.TrimSpace(segment))
		if err != nil {
			return [3]int{}, pre
		}
		parts[i] = n
	}

	return parts, pre
}
