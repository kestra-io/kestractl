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

// Coverage for the trigger read commands across the version matrix.
//
// Kestra 2.0 changed the shape of every /triggers/** response: a per-flow
// search that used to answer with a flat `Trigger` (namespace/flowId/triggerId/
// date/nextExecutionDate) now answers with an `ApiTriggerState` (updatedAt/
// nextEvaluationDate, no `date` at all), and the paged search nests the two
// halves under `trigger` and `state`. The generated Go SDK still marks the 1.x
// `date` required, so reading a 2.0 body through it failed the whole command
// with "no value given for required property date" — issue #131 for
// search-for-flow and the single backfill ops, #118 for `triggers list`.
//
// The unit tests pin each shape against a fake server; these run the real
// binary against every version in COMPATIBLE_KESTRA_VERSION.properties, which
// is the gap that let the decode failure reach a release unnoticed.

const triggersTestNamespace = "e2e.triggers"

// triggersTestTriggerID is the trigger id deployScheduledTestFlow declares.
const triggersTestTriggerID = "every_min"

// triggerDecodeFailure is the error a trigger response that the SDK model
// rejects produces. It can never be a legitimate answer, on either server line,
// so its absence is the assertion these tests are built around.
const triggerDecodeFailure = "no value given for required property"

// deployScheduledTestFlow deploys a flow carrying one disabled Schedule
// trigger, and returns its flow id.
//
// The trigger is disabled so a run of this suite does not leave a schedule
// firing every few minutes behind on a persistent instance; a disabled trigger
// still gets a trigger state row, which is what these commands read.
func deployScheduledTestFlow(t *testing.T, flowID string) string {
	t.Helper()

	source := fmt.Sprintf(`id: %s
namespace: %s
triggers:
  - id: %s
    type: io.kestra.plugin.core.trigger.Schedule
    cron: "*/5 * * * *"
    disabled: true
tasks:
  - id: hello
    type: io.kestra.plugin.core.log.Log
    message: kestractl e2e
`, flowID, triggersTestNamespace, triggersTestTriggerID)

	path := filepath.Join(t.TempDir(), flowID+".yml")
	require.NoError(t, os.WriteFile(path, []byte(source), 0o600))

	stdout, stderr, err := RunAuthenticatedCliCmd(t, "flows", "deploy", path, "--override")
	require.NoError(t, err, "deploy failed\nstdout: %s\nstderr: %s", stdout, stderr)
	require.Contains(t, stdout, "1 flow(s) deployed successfully")

	return flowID
}

// eventuallyTriggerListed runs the given command until its output contains
// want, and returns that output.
//
// The wait is for the server, not the CLI: a trigger state row appears only
// once the scheduler has registered the freshly deployed flow, which on a 1.x
// instance takes a few seconds. A decode failure is not waited out — it is a
// terminal answer, and the whole point of these tests — so the loop fails on it
// immediately rather than retrying until the deadline.
func eventuallyTriggerListed(t *testing.T, want string, args ...string) string {
	t.Helper()

	deadline := time.Now().Add(60 * time.Second)
	var stdout, stderr string
	var err error
	for {
		stdout, stderr, err = RunAuthenticatedCliCmd(t, args...)
		require.NotContains(t, stdout+stderr, triggerDecodeFailure,
			"the trigger response was rejected at decode\nstdout: %s\nstderr: %s", stdout, stderr)

		if err == nil && strings.Contains(stdout, want) {
			return stdout
		}
		if time.Now().After(deadline) {
			t.Fatalf("%q never listed %q within the deadline: %v\nstdout: %s\nstderr: %s",
				strings.Join(args, " "), want, err, stdout, stderr)
		}
		time.Sleep(2 * time.Second)
	}
}

// TestTriggers_searchForFlow_decodes covers issue #131: on Kestra 2.0 the server
// answers 200 and kestractl exited 1 on the response body alone.
func TestTriggers_searchForFlow_decodes(t *testing.T) {
	flowID := deployScheduledTestFlow(t, "e2e-triggers-search")

	stdout := eventuallyTriggerListed(t, flowID,
		"triggers", "search-for-flow", triggersTestNamespace, flowID)

	require.Contains(t, stdout, triggersTestTriggerID, "the trigger id is missing from the output")
	require.Contains(t, stdout, triggersTestNamespace, "the namespace is missing from the output")
}

// TestTriggers_backfillOps_decode covers the other half of issue #131. Whether
// a backfill exists to act on differs by server line — 1.x answers 404 for a
// trigger that has none, 2.0 accepts the call — so the assertion is only that
// the response is read rather than rejected over a missing property.
func TestTriggers_backfillOps_decode(t *testing.T) {
	flowID := deployScheduledTestFlow(t, "e2e-triggers-backfill")

	for _, op := range []string{"backfill-pause", "backfill-unpause", "backfill-delete"} {
		t.Run(op, func(t *testing.T) {
			// A non-zero exit is a legitimate outcome here: a server with no
			// backfill for this trigger reports that. Being unable to read the
			// answer at all is not.
			stdout, stderr, _ := RunAuthenticatedCliCmd(t, "triggers", op,
				triggersTestNamespace, flowID, triggersTestTriggerID)
			require.NotContains(t, stdout+stderr, triggerDecodeFailure,
				"%s: the trigger response was rejected at decode\nstdout: %s\nstderr: %s", op, stdout, stderr)
		})
	}
}

// TestTriggers_list_rendersRows covers issue #118: the 2.0 {trigger, state}
// body decoded into empty fields, so every row rendered blank.
func TestTriggers_list_rendersRows(t *testing.T) {
	flowID := deployScheduledTestFlow(t, "e2e-triggers-list")

	stdout := eventuallyTriggerListed(t, flowID, "triggers", "list", "--size", "200")

	require.Contains(t, stdout, triggersTestTriggerID, "the trigger id is missing from the listing")
	require.Contains(t, stdout, "io.kestra.plugin.core.trigger.Schedule",
		"the trigger type is missing, so the row rendered blank")
}
