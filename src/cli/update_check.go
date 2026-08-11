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
	updateCheckDisabledEnv = "KESTRACTL_UPDATE_CHECK_DISABLED"
	updateCheckFileName    = "update_check.json"
	updateCheckInterval    = 24 * time.Hour
	updateReleasesURL      = "https://github.com/kestra-io/kestractl/releases/latest"
)

var (
	updateCheckLatestURL   = "https://api.github.com/repos/kestra-io/kestractl/releases/latest"
	updateCheckHTTPClient  = &http.Client{Timeout: 2 * time.Second}
	updateCheckNow         = time.Now
	updateCheckCIDetection = detectCIProvider
)

// updateCheckState is the on-disk cache backing the once-a-day update check.
// LastCheck is stamped before the network call so a failing call does not make
// every subsequent command retry.
type updateCheckState struct {
	LastCheck     time.Time `json:"last_check"`
	LatestVersion string    `json:"latest_version,omitempty"`
}

// checkForUpdates warns on stderr when a newer kestractl release exists.
//
// The network call happens at most once per updateCheckInterval: the result is
// cached in <state dir>/update_check.json, so only the first command run after
// a full day pays for it. Every other invocation reads the cached version and
// costs a single small file read. Any failure is silent — an update notice must
// never get in the way of the command the user actually asked for.
func checkForUpdates(out io.Writer) {
	if updateCheckSkipped() {
		return
	}

	stateDir := telemetryStateDir()
	state, fresh := loadUpdateCheckState(stateDir)

	if !fresh {
		latest := fetchLatestVersion()
		// Stamp the attempt either way so a broken network is retried tomorrow,
		// not on the very next command.
		state = updateCheckState{LastCheck: updateCheckNow(), LatestVersion: latest}
		saveUpdateCheckState(stateDir, state)
	}

	if state.LatestVersion == "" {
		return
	}

	if compareVersions(state.LatestVersion, version) > 0 {
		fmt.Fprintf(out, "\nA new version of kestractl is available: %s (you are on v%s)\n", normalizeVersion(state.LatestVersion), strings.TrimPrefix(version, "v"))
		fmt.Fprintf(out, "Download it from %s\n\n", updateReleasesURL)
	}
}

// updateCheckSkipped reports whether the update check should not run at all.
func updateCheckSkipped() bool {
	if envEnabled(os.Getenv(updateCheckDisabledEnv)) {
		return true
	}

	// Dev/local builds have no meaningful version to compare against.
	if version == "" || version == "dev" {
		return true
	}

	// CI runs are non-interactive and pinned to a version on purpose.
	if isCI, _ := updateCheckCIDetection(); isCI {
		return true
	}

	return false
}

// loadUpdateCheckState reads the cached state and reports whether it is still
// within updateCheckInterval.
func loadUpdateCheckState(stateDir string) (updateCheckState, bool) {
	if strings.TrimSpace(stateDir) == "" {
		return updateCheckState{}, false
	}

	data, err := os.ReadFile(filepath.Join(stateDir, updateCheckFileName))
	if err != nil {
		return updateCheckState{}, false
	}

	var state updateCheckState
	if err := json.Unmarshal(data, &state); err != nil {
		return updateCheckState{}, false
	}

	if state.LastCheck.IsZero() {
		return state, false
	}

	elapsed := updateCheckNow().Sub(state.LastCheck)
	// A clock moving backwards (or a state file written in the future) must not
	// pin the cache as fresh forever.
	if elapsed < 0 {
		return state, false
	}

	return state, elapsed < updateCheckInterval
}

func saveUpdateCheckState(stateDir string, state updateCheckState) {
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

	_ = os.WriteFile(filepath.Join(stateDir, updateCheckFileName), data, 0o600)
}

type githubReleaseResponse struct {
	TagName string `json:"tag_name"`
}

// fetchLatestVersion returns the latest published release tag, or "" on any failure.
func fetchLatestVersion() string {
	req, err := http.NewRequest(http.MethodGet, updateCheckLatestURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "kestractl/"+version)

	res, err := updateCheckHTTPClient.Do(req)
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
