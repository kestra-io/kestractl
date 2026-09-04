package e2e_tests

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Coverage for the single-execution action commands.
//
// Kestra 2.0 moved these under /executions/{executionId}/actions/ while 1.x
// serves them bare, and the CLI normalises whichever shape its SDK emits into
// the one the server in front of it routes (see #117 and
// kestra-io/client-sdk#409). Because this suite runs against every version in
// COMPATIBLE_KESTRA_VERSION.properties, these tests pin that routing on both
// server lines at once — the gap that let #117 reach a release unnoticed.
//
// Assertions are on kestractl's own output rather than the server's error text,
// which differs between 1.x and 2.0 for the same condition.

const executionsTestNamespace = "e2e.executions"

// executionIDPattern matches the line `executions run` prints on success.
var executionIDPattern = regexp.MustCompile(`Execution ID: ([A-Za-z0-9]+)`)

// wrongRouteSignatures are the errors a request gets when it reaches a path the
// server does not route. None of them can be a legitimate answer to an action
// on an execution that exists, so their absence is the routing assertion.
//
//   - On 2.0 a bare action path falls through to POST /executions/{namespace}/
//     {flowId}, which reports a missing *flow* — the misleading symptom in #117.
//   - "Access denied" / "Forbidden" is what a catch-all route answers when it
//     matched but the action segment did not: 2.0 for a bare `kill`/`labels`,
//     1.x for anything under /actions/.
var wrongRouteSignatures = []string{
	"Requested Flow is not found.",
	"Access denied",
	"Page Not Found",
	"Forbidden",
}

func requireRoutedCorrectly(t *testing.T, action, stdout, stderr string) {
	t.Helper()
	for _, signature := range wrongRouteSignatures {
		if strings.Contains(stderr, signature) || strings.Contains(stdout, signature) {
			t.Errorf("executions %s did not reach a routed endpoint: %q\nstdout: %s\nstderr: %s",
				action, signature, stdout, stderr)
		}
	}
}

// deployExecutionsTestFlow deploys the one-task flow these tests act on. It is
// --override so a rerun against a persistent instance is not a hard failure,
// and uses only io.kestra.plugin.core.log.Log, which the plugin-less image this
// suite runs against still ships.
func deployExecutionsTestFlow(t *testing.T, flowID string) {
	t.Helper()

	source := fmt.Sprintf(`id: %s
namespace: %s
tasks:
  - id: hello
    type: io.kestra.plugin.core.log.Log
    message: kestractl e2e
`, flowID, executionsTestNamespace)

	path := filepath.Join(t.TempDir(), flowID+".yml")
	require.NoError(t, os.WriteFile(path, []byte(source), 0o600))

	stdout, stderr, err := RunAuthenticatedCliCmd(t, "flows", "deploy", path, "--override")
	require.NoError(t, err, "deploy failed\nstdout: %s\nstderr: %s", stdout, stderr)
	require.Contains(t, stdout, "1 flow(s) deployed successfully")
}

// runTerminatedExecution runs the flow to completion and returns the execution
// id. --wait is what makes the subject deterministic: the actions that require
// a terminated execution behave identically on every run, with no sleeping.
func runTerminatedExecution(t *testing.T, flowID string) string {
	t.Helper()

	stdout, stderr, err := RunAuthenticatedCliCmd(t, "executions", "run", executionsTestNamespace, flowID, "--wait")
	require.NoError(t, err, "run failed\nstdout: %s\nstderr: %s", stdout, stderr)

	match := executionIDPattern.FindStringSubmatch(stdout)
	require.Len(t, match, 2, "could not read an execution id from:\n%s", stdout)
	require.Contains(t, stdout, "State: SUCCESS", "expected a terminated execution:\n%s", stdout)
	return match[1]
}

// TestExecutionActions_succeedOnTerminatedExecution covers the actions that a
// terminated execution accepts, so each one asserts a real success rather than
// merely the absence of a routing error. Every one of these failed on Kestra
// 2.0 before #117, and eval-expression failed on 1.3.
func TestExecutionActions_succeedOnTerminatedExecution(t *testing.T) {
	flowID := "e2e-executions-actions"
	deployExecutionsTestFlow(t, flowID)

	tests := []struct {
		action string
		args   []string
		expect string
	}{
		// Reached through the SDK's hand-written client, which emits the 2.0
		// path: the one command that was broken on 1.3 rather than on 2.0.
		{action: "eval-expression", args: []string{"{{ flow.id }}"}, expect: flowID},
		{action: "set-labels", args: []string{"env=e2e"}, expect: "Set 1 label(s)"},
		{action: "replay", expect: "replayed as"},
		{action: "change-status", args: []string{"SUCCESS"}, expect: "status changed to"},
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			// A fresh execution per action, so one action cannot move the
			// subject's state out from under the next.
			executionID := runTerminatedExecution(t, flowID)

			args := append([]string{"executions", tt.action, executionID}, tt.args...)
			stdout, stderr, err := RunAuthenticatedCliCmd(t, args...)

			requireRoutedCorrectly(t, tt.action, stdout, stderr)
			require.NoError(t, err, "stdout: %s\nstderr: %s", stdout, stderr)
			require.Contains(t, stdout, tt.expect)
		})
	}
}

// TestExecutionActions_reachTheRoutedEndpoint is the broad net: every action,
// asserting only that the request reached an endpoint the server routes.
//
// Several of these cannot succeed against a terminated execution — pause wants
// a running one, restart a failed one — and the conflict they return differs by
// version. That conflict is the point: it proves the path resolved. A wrong
// path answers with one of wrongRouteSignatures instead.
func TestExecutionActions_reachTheRoutedEndpoint(t *testing.T) {
	flowID := "e2e-executions-routing"
	deployExecutionsTestFlow(t, flowID)
	executionID := runTerminatedExecution(t, flowID)

	for _, tt := range []struct {
		action string
		args   []string
	}{
		{action: "kill"},
		{action: "pause"},
		{action: "resume"},
		{action: "restart"},
		{action: "force-run"},
		{action: "replay"},
		{action: "replay-with-inputs"},
		{action: "unqueue", args: []string{"--state", "RUNNING"}},
		{action: "change-status", args: []string{"SUCCESS"}},
		{action: "set-labels", args: []string{"env=e2e"}},
		{action: "eval-expression", args: []string{"{{ flow.id }}"}},
	} {
		t.Run(tt.action, func(t *testing.T) {
			args := append([]string{"executions", tt.action, executionID}, tt.args...)
			stdout, stderr, _ := RunAuthenticatedCliCmd(t, args...)
			requireRoutedCorrectly(t, tt.action, stdout, stderr)
		})
	}
}

// TestExecutionsBulk_reportsTheAffectedCount guards the silent half of #117:
// the bulk paths never moved, but 2.0 answers them with
// {"operationId":..., "totalItems":N} where the SDK modelled only
// {"count":N}, so the number was dropped at decode and every bulk command
// reported 0 executions affected while having actually acted on them.
func TestExecutionsBulk_reportsTheAffectedCount(t *testing.T) {
	flowID := "e2e-executions-bulk"
	deployExecutionsTestFlow(t, flowID)
	executionID := runTerminatedExecution(t, flowID)

	stdout, stderr, err := RunAuthenticatedCliCmd(t, "executions", "replay-bulk", executionID)

	requireRoutedCorrectly(t, "replay-bulk", stdout, stderr)
	require.NoError(t, err, "stdout: %s\nstderr: %s", stdout, stderr)
	require.Contains(t, stdout, "1 execution(s) affected",
		"the affected count was dropped at decode\nstdout: %s", stdout)
}
