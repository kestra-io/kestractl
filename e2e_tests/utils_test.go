package e2e_tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const KESTRACTL_CLI_GENERATED_FOR_E2E = "KESTRACTL_CLI_GENERATED_FOR_E2E"

var (
	builtCli    string
	builtCliErr error
)

// The instance this suite expects, per e2e_tests/README.md and
// e2e_tests/docker-setup/application.yml.
const (
	e2eHost     = "http://localhost:9801"
	e2eUsername = "root@root.com"
	e2ePassword = "Root!1234"
)

func RunAuthenticatedCliCmd(t *testing.T, args ...string) (stdout string, stderr string, err error) {
	args = append(args, "--host", e2eHost, "--username", e2eUsername, "--password", e2ePassword)
	return RunCliCmd(t, args...)
}

// serverKestraVersion returns the major and minor version of the Kestra the
// suite is running against.
//
// It asks the server rather than reading KESTRA_VERSION, because the documented
// way to run this suite is against an instance you started yourself, where that
// variable is not set. Tests use it to skip assertions about endpoints a given
// server line does not have, rather than hard-coding a version matrix here.
func serverKestraVersion(t *testing.T) (major int, minor int) {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, e2eHost+"/api/v1/configs", nil)
	require.NoError(t, err)
	req.SetBasicAuth(e2eUsername, e2ePassword)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "could not reach the Kestra under test")
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode, "unexpected status from /api/v1/configs")

	var configs struct {
		Version string `json:"version"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&configs))

	// Versions look like "1.3.35", "2.0.0-rc13" or "SNAPSHOT" on a develop
	// build. An unparseable version counts as current: this suite's develop job
	// tracks the newest server, not an old one.
	parts := strings.SplitN(configs.Version, ".", 3)
	if len(parts) < 2 {
		return math.MaxInt, 0
	}
	major, err = strconv.Atoi(parts[0])
	if err != nil {
		return math.MaxInt, 0
	}
	minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return major, 0
	}
	return major, minor
}

// skipBelowKestra skips the test when the server predates the given version.
func skipBelowKestra(t *testing.T, major, minor int, reason string) {
	t.Helper()

	haveMajor, haveMinor := serverKestraVersion(t)
	if haveMajor < major || (haveMajor == major && haveMinor < minor) {
		t.Skipf("needs Kestra %d.%d or later (server is %d.%d): %s", major, minor, haveMajor, haveMinor, reason)
	}
}

/*
*
this function will build a KestraCtl cli with go build in a tmp dir, and run it using exec.CommandContext, then returns stdout and stderr
*/
func RunCliCmd(t *testing.T, args ...string) (stdout string, stderr string, err error) {
	t.Helper()

	// Build before starting the clock: the first test pays the whole `go build`
	// cost, which on CI is already longer than the 30s budget meant for the CLI
	// run itself. Inside the context, that cancelled the process before it could
	// write anything, and the test saw empty output instead of the real error.
	cliPath, buildErr := getCliPathForE2E()
	if buildErr != nil {
		t.Fatal(fmt.Sprintf("could not even run local generated cli, err: %v", buildErr))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, cliPath, args...)
	cmd.Env = isolatedEnv(t)

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err = cmd.Run()

	// If it timed out, cmd.Run() returns an error; ctx.Err() will be context deadline exceeded.
	if ctx.Err() != nil {
		return outBuf.String(), errBuf.String(), ctx.Err()
	}

	return outBuf.String(), errBuf.String(), err
}

// isolatedEnv returns the environment for a CLI run with the developer's own
// kestractl config kept out of it.
//
// The CLI resolves credentials as flags > env > ~/.kestractl/config.yaml, and
// prefers a token over username/password. So a developer who has ever run
// `kestractl config` has a token on disk that outranks the --username/--password
// these tests pass, and the whole suite fails with "Authentication required"
// against a perfectly good server. TestSimpleCmd_Unauthenticated has the same
// dependency from the other side: it can only pass when no config is readable.
// CI never notices because its HOME is empty.
//
// Pointing HOME at a per-test temp dir makes the suite hermetic. The KESTRACTL_*
// credential variables are dropped for the same reason, minus the harness's own.
func isolatedEnv(t *testing.T) []string {
	t.Helper()

	home := t.TempDir()
	env := []string{"HOME=" + home, "USERPROFILE=" + home}
	for _, kv := range os.Environ() {
		key, _, found := strings.Cut(kv, "=")
		if !found {
			continue
		}
		switch key {
		case "HOME", "USERPROFILE":
			continue
		case KESTRACTL_CLI_GENERATED_FOR_E2E:
			// The harness's own handle on the built binary, not CLI config.
		default:
			if strings.HasPrefix(key, "KESTRACTL_") {
				continue
			}
		}
		env = append(env, kv)
	}
	return env
}

func cliExeName() string {
	if runtime.GOOS == "windows" {
		return "e2e-kestractl.exe"
	}
	return "e2e-kestractl"
}

func findRepoRoot(start string) (string, error) {
	dir := start
	for i := 0; i < 20; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("could not find go.mod from %s", start)
}

/*
*
get the generated CLI path from KESTRACTL_CLI_GENERATED_FOR_E2E env var,
OR build this CLI in a tmp dir and update KESTRACTL_CLI_GENERATED_FOR_E2E env var
*/
func getCliPathForE2E() (string, error) {
	if v := strings.TrimSpace(os.Getenv(KESTRACTL_CLI_GENERATED_FOR_E2E)); v != "" {
		return v, nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// get ../this-working-dir
	repoRoot := filepath.Dir(wd)

	tmpDir, err := os.MkdirTemp("", "kestractl-cli-e2e-*")
	if err != nil {
		return "", err

	}

	outPath := filepath.Join(tmpDir, cliExeName())

	buildCmd := exec.Command("go", "build", "-o", outPath, repoRoot)
	buildCmd.Dir = repoRoot

	var buildErrBuf bytes.Buffer
	buildCmd.Stderr = &buildErrBuf

	if err := buildCmd.Run(); err != nil {
		builtCliErr = fmt.Errorf("go build failed : %w\nstderr: %s", err, buildErrBuf.String())
		return "", builtCliErr
	}

	builtCli = outPath
	// Make it available to the rest of the process and any sub-process that inherits env.
	_ = os.Setenv(KESTRACTL_CLI_GENERATED_FOR_E2E, outPath)

	if builtCliErr != nil {
		return "", builtCliErr
	}
	if builtCli == "" {
		return "", fmt.Errorf("CLI path is empty after build")
	}
	return builtCli, nil
}
