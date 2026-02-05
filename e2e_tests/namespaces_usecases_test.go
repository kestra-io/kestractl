package e2e_tests

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNamespacesList_system(t *testing.T) {
	stdout, stderr, err := RunAuthenticatedCliCmd(t, "namespaces", "list", "company")

	require.NoError(t, err)
	require.Empty(t, stderr)

	require.Contains(t, stdout, "system")
}

func TestNamespacesList_system_json(t *testing.T) {
	stdout, stderr, err := RunAuthenticatedCliCmd(t, "namespaces", "list", "company", "--output", "json")

	require.NoError(t, err)
	require.Empty(t, stderr)

	require.NotEmpty(t, stdout)
	var parsedJson []map[string]interface{}
	err = json.Unmarshal([]byte(stdout), &parsedJson)

	require.NoError(t, err, "json from --output json should be valid")
	require.NotEmpty(t, parsedJson)
	require.Equal(t, "system", parsedJson[0]["id"])
	require.Equal(t, false, parsedJson[0]["deleted"])
}
