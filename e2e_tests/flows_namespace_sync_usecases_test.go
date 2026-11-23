package e2e_tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeSyncFlow(t *testing.T, dir, namespace, id string) {
	t.Helper()
	content := "id: " + id + "\nnamespace: " + namespace + "\ntasks:\n  - id: hello\n    type: io.kestra.plugin.core.log.Log\n    message: hello\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, id+".yaml"), []byte(content), 0o600))
}

func TestFlowsNamespaceSync_directory(t *testing.T) {
	// The multipart overload of POST /flows/{namespace} was backported to 1.0.x
	// (v1.0.24) and 1.2.x (v1.2.1) but never to 1.1.x, which answers 401.
	skipOnKestra(t, 1, 1, "no multipart overload of POST /flows/{namespace} on the 1.1 line")
	namespace := "e2e.sync." + randomId()[:8]
	dir := t.TempDir()
	writeSyncFlow(t, dir, namespace, "flow-a")
	writeSyncFlow(t, dir, namespace, "flow-b")

	t.Cleanup(func() {
		_, _, _ = RunAuthenticatedCliCmd(t, "flows", "delete", namespace, "flow-a", "--yes")
		_, _, _ = RunAuthenticatedCliCmd(t, "flows", "delete", namespace, "flow-b", "--yes")
	})

	stdout, stderr, err := RunAuthenticatedCliCmd(t, "flows", "namespace-sync", namespace, dir)
	require.NoError(t, err, stderr)
	require.Contains(t, stdout, "2 flow(s) created or updated, 0 deleted")

	for _, id := range []string{"flow-a", "flow-b"} {
		_, stderr, err = RunAuthenticatedCliCmd(t, "flows", "get", namespace, id)
		require.NoError(t, err, "flow %s should exist after sync: %s", id, stderr)
	}

	// Re-sync with one file removed: --delete must remove the missing flow.
	require.NoError(t, os.Remove(filepath.Join(dir, "flow-b.yaml")))
	stdout, stderr, err = RunAuthenticatedCliCmd(t, "flows", "namespace-sync", namespace, dir, "--delete")
	require.NoError(t, err, stderr)
	require.Contains(t, stdout, "1 flow(s) created or updated, 1 deleted")

	_, _, err = RunAuthenticatedCliCmd(t, "flows", "get", namespace, "flow-a")
	require.NoError(t, err, "flow-a should still exist")
	_, _, err = RunAuthenticatedCliCmd(t, "flows", "get", namespace, "flow-b")
	require.Error(t, err, "flow-b should have been deleted by --delete")
}
