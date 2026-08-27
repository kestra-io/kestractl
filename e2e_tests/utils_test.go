package e2e_tests

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const KESTRACTL_CLI_GENERATED_FOR_E2E = "KESTRACTL_CLI_GENERATED_FOR_E2E"

var (
	builtCli    string
	builtCliErr error
)

func RunAuthenticatedCliCmd(t *testing.T, args ...string) (stdout string, stderr string, err error) {
	args = append(args, "--host", "http://localhost:9801", "--username", "root@root.com", "--password", "Root!1234")
	return RunCliCmd(t, args...)
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
