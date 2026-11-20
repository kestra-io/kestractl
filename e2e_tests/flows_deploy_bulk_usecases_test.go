package e2e_tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

type deployRow struct {
	FlowID   string `json:"flow_id"`
	Revision int32  `json:"revision"`
	Success  bool   `json:"success"`
	Error    string `json:"error"`
}

func deployRows(t *testing.T, stdout string) map[string]deployRow {
	t.Helper()
	var rows []deployRow
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows), stdout)
	byID := make(map[string]deployRow, len(rows))
	for _, r := range rows {
		byID[r.FlowID] = r
	}
	return byID
}

// A directory of flows goes through POST /flows/bulk. Runs on every supported
// line: Kestra 1.x only accepts application/yaml there, so a wrong content type
// fails here.
func TestFlowsDeploy_directoryBulk(t *testing.T) {
	namespace := "e2e.deploybulk." + randomId()[:8]
	ids := []string{"flow-a", "flow-b", "flow-c"}
	dir := t.TempDir()
	for _, id := range ids {
		writeSyncFlow(t, dir, namespace, id)
	}
	// The same flow id in two namespaces: Kestra 1.x rejects a bulk body that
	// repeats an id, so the CLI must put them in separate requests.
	otherNamespace := namespace + ".other"
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "other"), 0o755))
	writeSyncFlow(t, filepath.Join(dir, "other"), otherNamespace, "flow-a")
	t.Cleanup(func() {
		for _, id := range ids {
			_, _, _ = RunAuthenticatedCliCmd(t, "flows", "delete", namespace, id, "--yes")
		}
		_, _, _ = RunAuthenticatedCliCmd(t, "flows", "delete", otherNamespace, "flow-a", "--yes")
	})

	stdout, stderr, err := RunAuthenticatedCliCmd(t, "flows", "deploy", dir, "--output", "json")
	require.NoError(t, err, stderr)
	first := deployRows(t, stdout)
	for _, id := range ids {
		require.True(t, first[id].Success, "%s: %s", id, first[id].Error)
		require.Equal(t, int32(1), first[id].Revision)
	}

	// Without --override the existing flows are reported and the run fails.
	stdout, _, err = RunAuthenticatedCliCmd(t, "flows", "deploy", dir, "--output", "json")
	require.Error(t, err)
	for _, id := range ids {
		row := deployRows(t, stdout)[id]
		require.False(t, row.Success)
		require.Contains(t, row.Error, "already exists")
	}

	// --override with unchanged sources keeps the revisions.
	stdout, stderr, err = RunAuthenticatedCliCmd(t, "flows", "deploy", dir, "--override", "--output", "json")
	require.NoError(t, err, stderr)
	again := deployRows(t, stdout)
	for _, id := range ids {
		require.True(t, again[id].Success, "%s: %s", id, again[id].Error)
		require.Equal(t, first[id].Revision, again[id].Revision)
	}

	// A single-file deploy of an unchanged file takes the per-flow path and must
	// not create a revision either.
	stdout, stderr, err = RunAuthenticatedCliCmd(t, "flows", "deploy", filepath.Join(dir, "flow-a.yaml"), "--override", "--output", "json")
	require.NoError(t, err, stderr)
	require.Equal(t, first["flow-a"].Revision, deployRows(t, stdout)["flow-a"].Revision)
}
