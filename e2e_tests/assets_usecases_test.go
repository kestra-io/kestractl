package e2e_tests

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Kestra 2.1 dropped the asset lineage-event routes and the destinationOnly
// dependency graph, so kestractl picks the route by server version. These pin
// that choice on every line of the matrix: assets exist from 1.2, lineage
// events from 1.3.

func isBeforeKestra21(t *testing.T) bool {
	t.Helper()
	major, minor := serverKestraVersion(t)
	return major < 2 || (major == 2 && minor < 1)
}

func createE2EAsset(t *testing.T) string {
	t.Helper()

	id := fmt.Sprintf("e2e_asset_%d", time.Now().UnixNano())
	source := fmt.Sprintf(`id: %s
type: io.kestra.plugin.ee.assets.Table
system: e2e_system
database: e2e_db
schema: e2e_schema
name: %s
`, id, id)

	path := filepath.Join(t.TempDir(), id+".yml")
	require.NoError(t, os.WriteFile(path, []byte(source), 0o600))

	stdout, stderr, err := RunAuthenticatedCliCmd(t, "assets", "create", "--file", path)
	require.NoError(t, err, "create failed\nstdout: %s\nstderr: %s", stdout, stderr)
	require.Contains(t, stdout, "Asset created successfully.")

	t.Cleanup(func() {
		_, _, _ = RunAuthenticatedCliCmd(t, "assets", "delete", id, "--yes")
	})
	return id
}

func TestAssets_listAndDependencies(t *testing.T) {
	skipBelowKestra(t, 1, 2, "assets were added in Kestra 1.2")
	id := createE2EAsset(t)

	t.Run("list", func(t *testing.T) {
		stdout, stderr, err := RunAuthenticatedCliCmd(t, "assets", "list", "--output", "json")
		require.NoError(t, err, "stdout: %s\nstderr: %s", stdout, stderr)
		require.Contains(t, stdout, id)
	})

	t.Run("dependencies", func(t *testing.T) {
		stdout, stderr, err := RunAuthenticatedCliCmd(t, "assets", "dependencies", id)
		require.NoError(t, err, "stdout: %s\nstderr: %s", stdout, stderr)
		require.Contains(t, stdout, "Nodes: ")
	})

	t.Run("dependencies --destination-only", func(t *testing.T) {
		stdout, stderr, err := RunAuthenticatedCliCmd(t, "assets", "dependencies", id, "--destination-only", "--output", "json")
		if isBeforeKestra21(t) {
			require.NoError(t, err, "stdout: %s\nstderr: %s", stdout, stderr)
			require.Contains(t, stdout, `"nodes"`)
			return
		}
		require.Error(t, err, "stdout: %s\nstderr: %s", stdout, stderr)
		require.Contains(t, stderr, "--destination-only is only available before Kestra 2.1")
	})
}

func TestAssets_lineageEventsAndUsages(t *testing.T) {
	skipBelowKestra(t, 1, 3, "asset lineage events were added in Kestra 1.3")

	t.Run("lineage-events list", func(t *testing.T) {
		stdout, stderr, err := RunAuthenticatedCliCmd(t, "assets", "lineage-events", "list")
		require.NoError(t, err, "stdout: %s\nstderr: %s", stdout, stderr)
		if isBeforeKestra21(t) {
			require.Contains(t, stdout, "Total lineage events:")
		} else {
			require.Contains(t, stdout, "Total usages:")
		}
	})

	t.Run("lineage-events delete-by-query", func(t *testing.T) {
		stdout, stderr, err := RunAuthenticatedCliCmd(t, "assets", "lineage-events", "delete-by-query", "--namespace", "e2e.assets.none")
		require.NoError(t, err, "stdout: %s\nstderr: %s", stdout, stderr)
		require.Contains(t, stdout, "asset(s) affected")
	})

	t.Run("usages list", func(t *testing.T) {
		stdout, stderr, err := RunAuthenticatedCliCmd(t, "assets", "usages", "list")
		require.NoError(t, err, "stdout: %s\nstderr: %s", stdout, stderr)
		require.Contains(t, stdout, "Total usages:")
	})
}
