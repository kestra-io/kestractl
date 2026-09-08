package e2e_tests

import (
	"encoding/json"
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

// triggersTestTriggerType is the trigger type deployScheduledTestFlow declares,
// and the column that rendered blank under issue #118.
const triggersTestTriggerType = "io.kestra.plugin.core.trigger.Schedule"

// triggerDecodeFailure is the error a trigger response that the SDK model
// rejects produces. It can never be a legitimate answer, on either server line,
// so its absence is the assertion these tests are built around.
const triggerDecodeFailure = "no value given for required property"

// triggerRegistrationTimeout bounds the wait for the scheduler to register a
// freshly deployed flow, which is what makes a trigger state row exist.
const triggerRegistrationTimeout = 60 * time.Second

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
    type: %s
    cron: "*/5 * * * *"
    disabled: true
tasks:
  - id: hello
    type: io.kestra.plugin.core.log.Log
    message: kestractl e2e
`, flowID, triggersTestNamespace, triggersTestTriggerID, triggersTestTriggerType)

	path := filepath.Join(t.TempDir(), flowID+".yml")
	require.NoError(t, os.WriteFile(path, []byte(source), 0o600))

	stdout, stderr, err := RunAuthenticatedCliCmd(t, "flows", "deploy", path, "--override")
	require.NoError(t, err, "deploy failed\nstdout: %s\nstderr: %s", stdout, stderr)
	require.Contains(t, stdout, "1 flow(s) deployed successfully")

	return flowID
}

// requireNoTriggerDecodeFailure fails the test at once when a trigger command
// could not read the server's answer.
//
// This is never waited out: a decode failure is a terminal answer, and the whole
// point of these tests, so a poll loop stops on it rather than retrying until
// its deadline and reporting a timeout instead of the real cause.
func requireNoTriggerDecodeFailure(t *testing.T, label, stdout, stderr string) {
	t.Helper()

	require.NotContains(t, stdout+stderr, triggerDecodeFailure,
		"%s: the trigger response was rejected at decode\nstdout: %s\nstderr: %s", label, stdout, stderr)
}

// eventuallyTriggerListed runs the given command until its output contains
// want, and returns that output.
//
// The wait is for the server, not the CLI: a trigger state row appears only
// once the scheduler has registered the freshly deployed flow, which on a 1.x
// instance takes a few seconds.
func eventuallyTriggerListed(t *testing.T, want string, args ...string) string {
	t.Helper()

	deadline := time.Now().Add(triggerRegistrationTimeout)
	for {
		stdout, stderr, err := RunAuthenticatedCliCmd(t, args...)
		requireNoTriggerDecodeFailure(t, strings.Join(args, " "), stdout, stderr)

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

// triggerRow is one row of `triggers list --output json`.
type triggerRow struct {
	Namespace         string `json:"namespace"`
	FlowID            string `json:"flowId"`
	TriggerID         string `json:"triggerId"`
	Type              string `json:"type"`
	Disabled          bool   `json:"disabled"`
	NextExecutionDate string `json:"nextExecutionDate"`
}

// triggersListPageSize and triggersListMaxPages bound the paging below.
const (
	triggersListPageSize = 200
	triggersListMaxPages = 50
)

// findListedTriggerRow returns the `triggers list` row for the given flow.
//
// The listing is tenant-wide with no server-side filter for a flow, so it is
// paged through rather than assuming the row under test lands on page 1: the
// docker-setup README documents reusing one instance across runs, and on an
// instance carrying more triggers than a single page the row is simply
// elsewhere — which would read as a render regression that does not exist.
func findListedTriggerRow(t *testing.T, flowID string) (triggerRow, bool) {
	t.Helper()

	for page := 1; page <= triggersListMaxPages; page++ {
		args := []string{"triggers", "list", "--output", "json",
			"--page", fmt.Sprint(page), "--size", fmt.Sprint(triggersListPageSize)}
		stdout, stderr, err := RunAuthenticatedCliCmd(t, args...)
		requireNoTriggerDecodeFailure(t, strings.Join(args, " "), stdout, stderr)
		require.NoError(t, err, "triggers list failed\nstdout: %s\nstderr: %s", stdout, stderr)

		var rows []triggerRow
		require.NoError(t, json.Unmarshal([]byte(stdout), &rows),
			"json from --output json should be valid, got: %s", stdout)

		for _, row := range rows {
			if row.FlowID == flowID && row.Namespace == triggersTestNamespace {
				return row, true
			}
		}
		if len(rows) < triggersListPageSize {
			return triggerRow{}, false
		}
	}
	return triggerRow{}, false
}

// eventuallyListedTriggerRow waits for the row of the flow under test to appear
// in `triggers list`, and returns it.
func eventuallyListedTriggerRow(t *testing.T, flowID string) triggerRow {
	t.Helper()

	deadline := time.Now().Add(triggerRegistrationTimeout)
	for {
		if row, ok := findListedTriggerRow(t, flowID); ok {
			return row
		}
		if time.Now().After(deadline) {
			t.Fatalf("`triggers list` never listed a row for flow %q in namespace %q within the deadline",
				flowID, triggersTestNamespace)
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

	// Wait for the trigger to exist server-side first. Called straight after the
	// deploy, the ops answer not-found before the scheduler has registered the
	// flow, and every subtest would go green without ever decoding a trigger
	// body — letting the regression these tests exist to catch re-land.
	eventuallyTriggerListed(t, triggersTestTriggerID,
		"triggers", "search-for-flow", triggersTestNamespace, flowID)

	for _, op := range []string{"backfill-pause", "backfill-unpause", "backfill-delete"} {
		t.Run(op, func(t *testing.T) {
			// A non-zero exit is a legitimate outcome here: a server with no
			// backfill for this trigger reports that. Being unable to read the
			// answer at all is not.
			stdout, stderr, _ := RunAuthenticatedCliCmd(t, "triggers", op,
				triggersTestNamespace, flowID, triggersTestTriggerID)
			requireNoTriggerDecodeFailure(t, op, stdout, stderr)
		})
	}
}

// TestTriggers_list_rendersRows covers issue #118: the 2.0 {trigger, state}
// body decoded into empty fields, so every row rendered blank.
//
// The assertions are made against the single row for the flow under test, not
// against the whole listing: any other flow on the instance carrying a Schedule
// trigger — a leftover from an earlier run of this suite included — would
// satisfy a listing-wide type assertion even while the row under test rendered
// its TYPE column blank, which is exactly what #118 was.
func TestTriggers_list_rendersRows(t *testing.T) {
	flowID := deployScheduledTestFlow(t, "e2e-triggers-list")

	row := eventuallyListedTriggerRow(t, flowID)

	require.Equal(t, triggersTestTriggerID, row.TriggerID, "the trigger id is missing from the row")
	require.Equal(t, triggersTestTriggerType, row.Type, "the trigger type is missing, so the row rendered blank")
}
