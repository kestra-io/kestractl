package e2e_tests

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Coverage for 'logs delete-flow' (#133).
//
// The flow-scoped log deletion endpoint declares triggerId as a *required*
// query parameter on both server lines, so the command used to fail with a raw
// server 400 whenever --trigger-id was omitted — while its help text advertised
// deleting "all logs for a flow". Verified by raw curl on 1.3.35 and
// 2.0.0-rc13:
//
//	DELETE /api/v1/main/logs/<ns>/<flow>              -> 400 (triggerId not specified)
//	DELETE /api/v1/main/logs/<ns>/<flow>?triggerId=   -> 200, but deletes nothing
//
// An empty value is therefore not a usable stand-in: it is a silent no-op. The
// command now rejects a missing --trigger-id itself, before any request, and
// this test pins that on every version in the matrix.

const logsDeleteTestNamespace = "e2e.logsdelete"

// deployLogsDeleteFlow deploys the one-task subject flow. --override keeps a
// rerun against a persistent instance from being a hard failure, and
// io.kestra.plugin.core.log.Log is available in the plugin-less image this
// suite runs against.
func deployLogsDeleteFlow(t *testing.T, flowID string) {
	t.Helper()

	source := fmt.Sprintf(`id: %s
namespace: %s
tasks:
  - id: hello
    type: io.kestra.plugin.core.log.Log
    message: kestractl e2e logs delete
`, flowID, logsDeleteTestNamespace)

	path := filepath.Join(t.TempDir(), flowID+".yml")
	require.NoError(t, os.WriteFile(path, []byte(source), 0o600))

	stdout, stderr, err := RunAuthenticatedCliCmd(t, "flows", "deploy", path, "--override")
	require.NoError(t, err, "deploy failed\nstdout: %s\nstderr: %s", stdout, stderr)
	require.Contains(t, stdout, "1 flow(s) deployed successfully")
}

// TestLogs_deleteFlow_withoutTrigger asserts the command fails locally with its
// own message when --trigger-id is omitted, and reaches the endpoint
// successfully when it is supplied.
func TestLogs_deleteFlow_withoutTrigger(t *testing.T) {
	flowID := "e2e-logs-delete-flow"
	deployLogsDeleteFlow(t, flowID)

	t.Run("without --trigger-id", func(t *testing.T) {
		stdout, stderr, err := RunAuthenticatedCliCmd(t, "logs", "delete-flow", logsDeleteTestNamespace, flowID)

		require.Error(t, err, "expected a non-zero exit\nstdout: %s\nstderr: %s", stdout, stderr)
		require.Contains(t, stderr, "--trigger-id is required")
		// The point of the fix: kestractl answers, the server is never asked.
		require.NotContains(t, stderr, "triggerId] not specified")
	})

	t.Run("with --trigger-id", func(t *testing.T) {
		stdout, stderr, err := RunAuthenticatedCliCmd(t,
			"logs", "delete-flow", logsDeleteTestNamespace, flowID, "--trigger-id", "e2e-trigger")

		require.NoError(t, err, "stdout: %s\nstderr: %s", stdout, stderr)
		require.Contains(t, stdout, "deleted")
	})
}
