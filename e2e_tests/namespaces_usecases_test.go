package e2e_tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
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
}

// Both Kestra 1.3 and 2.0 answer /inherited-variables with a flat
// {variable: value} map, which the SDK's map-of-maps type cannot decode as soon
// as one value is a scalar; the command used to fail outright on 2.0. A value
// above 2^53 must also keep every digit rather than being rendered through
// float64. See https://github.com/kestra-io/kestractl/issues/128.
func TestNamespacesInheritedVariables_flatScalarsAndLargeIntegers(t *testing.T) {
	parent := "e2e-inherited-vars-" + randomId()
	child := parent + ".child"

	variablesFile := filepath.Join(t.TempDir(), "variables.yml")
	require.NoError(t, os.WriteFile(variablesFile, []byte("big: 1725000000000000001\nname: hello\nnested:\n  a: 1\n"), 0o600))

	_, stderr, err := RunAuthenticatedCliCmd(t, "namespaces", "create", parent, "--variables-file", variablesFile)
	require.NoError(t, err, "stderr: %s", stderr)
	t.Cleanup(func() { _, _, _ = RunAuthenticatedCliCmd(t, "namespaces", "delete", parent) })

	_, stderr, err = RunAuthenticatedCliCmd(t, "namespaces", "create", child)
	require.NoError(t, err, "stderr: %s", stderr)
	t.Cleanup(func() { _, _, _ = RunAuthenticatedCliCmd(t, "namespaces", "delete", child) })

	stdout, stderr, err := RunAuthenticatedCliCmd(t, "namespaces", "inherited-variables", child)
	require.NoError(t, err, "stderr: %s", stderr)
	require.Contains(t, stdout, "hello")
	require.Contains(t, stdout, "1725000000000000001", "a large integer must not be rendered through float64")
	require.NotContains(t, stdout, "e+18")

	stdout, stderr, err = RunAuthenticatedCliCmd(t, "namespaces", "inherited-variables", child, "--output", "json")
	require.NoError(t, err, "stderr: %s", stderr)

	var rows []map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows), "json from --output json should be valid")

	values := map[string]interface{}{}
	for _, row := range rows {
		key, _ := row["key"].(string)
		values[key] = row["value"]
		require.Equal(t, child, row["namespace"])
	}
	require.Equal(t, "1725000000000000001", values["big"])
	require.Equal(t, "hello", values["name"])
	require.Equal(t, `{"a":1}`, values["nested"])
}
