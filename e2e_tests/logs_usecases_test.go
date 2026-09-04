package e2e_tests

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const logsTestNamespace = "e2e.logs"

// TestLogs_searchMinLevel covers #132: `logs search --min-level` sent
// filters[level][EQUALS], which Kestra 2.0 rejects with
// "Operation EQUALS is not supported for field LEVEL". The two server lines
// accept different operations on that field and each 400s the other's, so this
// runs on both halves of the version matrix and is the assertion that neither
// regresses.
//
// The subject is a flow that logs one line per level, so "minimum level" has an
// observable meaning: searching from WARN up must return the WARN and ERROR
// lines and never the INFO one.
func TestLogs_searchMinLevel(t *testing.T) {
	marker := "kestractl-e2e-minlevel-" + randomId()
	flowID := "e2e-logs-min-level-" + randomId()

	deployLogLevelsFlow(t, flowID, marker)
	runLogLevelsExecution(t, flowID)

	// The log store is written asynchronously by the worker, so the search can
	// legitimately be empty for a moment after the execution terminates.
	var stdout string
	require.Eventually(t, func() bool {
		out, stderr, err := RunAuthenticatedCliCmd(t,
			"logs", "search", "-n", logsTestNamespace, "-q", marker, "--min-level", "WARN")
		if err != nil {
			t.Logf("logs search failed, retrying\nstdout: %s\nstderr: %s", out, stderr)
			return false
		}
		stdout = out
		return strings.Contains(out, marker+"-warn") && strings.Contains(out, marker+"-error")
	}, 60*time.Second, 2*time.Second, "expected the WARN and ERROR lines in the search output")

	require.NotContains(t, stdout, marker+"-info",
		"--min-level WARN must not return the INFO line:\n%s", stdout)
	require.Contains(t, stdout, "WARN")
	require.Contains(t, stdout, "ERROR")
}

// deployLogLevelsFlow deploys a flow that logs one message per level, each
// tagged with marker so the search can find exactly these lines. It uses only
// io.kestra.plugin.core.log.Log, which the plugin-less image this suite runs
// against ships, and --override so a rerun against a persistent instance is
// not a hard failure.
func deployLogLevelsFlow(t *testing.T, flowID, marker string) {
	t.Helper()

	source := fmt.Sprintf(`id: %s
namespace: %s
tasks:
  - id: log-info
    type: io.kestra.plugin.core.log.Log
    level: INFO
    message: %s-info
  - id: log-warn
    type: io.kestra.plugin.core.log.Log
    level: WARN
    message: %s-warn
  - id: log-error
    type: io.kestra.plugin.core.log.Log
    level: ERROR
    message: %s-error
`, flowID, logsTestNamespace, marker, marker, marker)

	path := filepath.Join(t.TempDir(), flowID+".yml")
	require.NoError(t, os.WriteFile(path, []byte(source), 0o600))

	stdout, stderr, err := RunAuthenticatedCliCmd(t, "flows", "deploy", path, "--override")
	require.NoError(t, err, "deploy failed\nstdout: %s\nstderr: %s", stdout, stderr)
	require.Contains(t, stdout, "1 flow(s) deployed successfully")
}

// runLogLevelsExecution runs the flow to completion. A Log task at level ERROR
// does not fail the flow, so the execution is expected to succeed.
func runLogLevelsExecution(t *testing.T, flowID string) {
	t.Helper()

	stdout, stderr, err := RunAuthenticatedCliCmd(t, "executions", "run", logsTestNamespace, flowID, "--wait")
	require.NoError(t, err, "run failed\nstdout: %s\nstderr: %s", stdout, stderr)
	require.Contains(t, stdout, "State: SUCCESS", "expected a terminated execution:\n%s", stdout)
}
