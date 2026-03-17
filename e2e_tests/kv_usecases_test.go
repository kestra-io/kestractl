package e2e_tests

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKVGet_keyNotFound(t *testing.T) {
	_, stderr, err := RunAuthenticatedCliCmd(t, "kv", "get", "system", "nonexistent-key-"+randomId())

	require.Error(t, err)
	require.Contains(t, stderr, "not found")
}

func TestKVDelete_keyNotFound(t *testing.T) {
	_, stderr, err := RunAuthenticatedCliCmd(t, "kv", "delete", "system", "nonexistent-key-"+randomId())

	require.Error(t, err)
	require.NotEmpty(t, stderr)
}

func TestKVUpdate_keyNotFound(t *testing.T) {
	_, stderr, err := RunAuthenticatedCliCmd(t, "kv", "update", "system", "STRING", "nonexistent-key-"+randomId(), "value")

	require.Error(t, err)
	require.Contains(t, stderr, "not found")
}

func TestKVSet_invalidType(t *testing.T) {
	_, stderr, err := RunAuthenticatedCliCmd(t, "kv", "set", "system", "INVALID", "mykey", "myvalue")

	require.Error(t, err)
	require.Contains(t, stderr, "invalid type")
}

func TestKVSet_invalidNumberValue(t *testing.T) {
	_, stderr, err := RunAuthenticatedCliCmd(t, "kv", "set", "system", "NUMBER", "mykey", "not-a-number")

	require.Error(t, err)
	require.Contains(t, stderr, "invalid NUMBER value")
}

func TestKVSet_invalidBooleanValue(t *testing.T) {
	_, stderr, err := RunAuthenticatedCliCmd(t, "kv", "set", "system", "BOOLEAN", "mykey", "yes")

	require.Error(t, err)
	require.Contains(t, stderr, "invalid BOOLEAN value")
}

func TestKV_stringE2E(t *testing.T) {
	key := "e2e-kv-string-" + randomId()
	namespace := "system"

	// Verify key doesn't exist
	_, _, err := RunAuthenticatedCliCmd(t, "kv", "get", namespace, key)
	require.Error(t, err, "key should not exist before test")

	// Set a string value
	stdout, stderr, err := RunAuthenticatedCliCmd(t, "kv", "set", namespace, "STRING", key, "hello-world")
	require.NoError(t, err, "set should succeed, stderr: %s", stderr)
	require.Contains(t, stdout, "successfully")

	// Get and verify
	stdout, stderr, err = RunAuthenticatedCliCmd(t, "kv", "get", namespace, key)
	require.NoError(t, err, "get should succeed, stderr: %s", stderr)
	require.Contains(t, stdout, key)
	require.Contains(t, stdout, "hello-world")

	// Update the value
	stdout, stderr, err = RunAuthenticatedCliCmd(t, "kv", "update", namespace, "STRING", key, "updated-value")
	require.NoError(t, err, "update should succeed, stderr: %s", stderr)
	require.Contains(t, stdout, "successfully")

	// Get and verify update
	stdout, stderr, err = RunAuthenticatedCliCmd(t, "kv", "get", namespace, key)
	require.NoError(t, err, "get after update should succeed, stderr: %s", stderr)
	require.Contains(t, stdout, "updated-value")

	// List and verify the key appears
	stdout, stderr, err = RunAuthenticatedCliCmd(t, "kv", "list", namespace)
	require.NoError(t, err, "list should succeed, stderr: %s", stderr)
	require.Contains(t, stdout, key)

	// Delete the key
	stdout, stderr, err = RunAuthenticatedCliCmd(t, "kv", "delete", namespace, key)
	require.NoError(t, err, "delete should succeed, stderr: %s", stderr)
	require.Contains(t, stdout, "successfully")

	// Verify deleted
	_, _, err = RunAuthenticatedCliCmd(t, "kv", "get", namespace, key)
	require.Error(t, err, "key should have been deleted")
}

func TestKV_numberE2E(t *testing.T) {
	key := "e2e-kv-number-" + randomId()
	namespace := "system"

	stdout, stderr, err := RunAuthenticatedCliCmd(t, "kv", "set", namespace, "NUMBER", key, "42")
	require.NoError(t, err, "set NUMBER should succeed, stderr: %s", stderr)
	require.Contains(t, stdout, "successfully")

	stdout, stderr, err = RunAuthenticatedCliCmd(t, "kv", "get", namespace, key)
	require.NoError(t, err, "get NUMBER should succeed, stderr: %s", stderr)
	require.Contains(t, stdout, "42")

	// Cleanup
	_, _, _ = RunAuthenticatedCliCmd(t, "kv", "delete", namespace, key)
}

func TestKV_booleanE2E(t *testing.T) {
	key := "e2e-kv-boolean-" + randomId()
	namespace := "system"

	stdout, stderr, err := RunAuthenticatedCliCmd(t, "kv", "set", namespace, "BOOLEAN", key, "true")
	require.NoError(t, err, "set BOOLEAN should succeed, stderr: %s", stderr)
	require.Contains(t, stdout, "successfully")

	stdout, stderr, err = RunAuthenticatedCliCmd(t, "kv", "get", namespace, key)
	require.NoError(t, err, "get BOOLEAN should succeed, stderr: %s", stderr)
	require.Contains(t, stdout, "true")

	// Cleanup
	_, _, _ = RunAuthenticatedCliCmd(t, "kv", "delete", namespace, key)
}

func TestKV_jsonE2E(t *testing.T) {
	key := "e2e-kv-json-" + randomId()
	namespace := "system"

	stdout, stderr, err := RunAuthenticatedCliCmd(t, "kv", "set", namespace, "JSON", key, `{"feature":true,"count":5}`)
	require.NoError(t, err, "set JSON should succeed, stderr: %s", stderr)
	require.Contains(t, stdout, "successfully")

	stdout, stderr, err = RunAuthenticatedCliCmd(t, "kv", "get", namespace, key)
	require.NoError(t, err, "get JSON should succeed, stderr: %s", stderr)
	require.Contains(t, stdout, "feature")

	// Cleanup
	_, _, _ = RunAuthenticatedCliCmd(t, "kv", "delete", namespace, key)
}

func TestKV_durationE2E(t *testing.T) {
	key := "e2e-kv-duration-" + randomId()
	namespace := "system"

	stdout, stderr, err := RunAuthenticatedCliCmd(t, "kv", "set", namespace, "DURATION", key, "PT15M")
	require.NoError(t, err, "set DURATION should succeed, stderr: %s", stderr)
	require.Contains(t, stdout, "successfully")

	stdout, stderr, err = RunAuthenticatedCliCmd(t, "kv", "get", namespace, key)
	require.NoError(t, err, "get DURATION should succeed, stderr: %s", stderr)
	require.Contains(t, stdout, "PT15M")

	// Cleanup
	_, _, _ = RunAuthenticatedCliCmd(t, "kv", "delete", namespace, key)
}

func TestKV_deleteAlias(t *testing.T) {
	key := "e2e-kv-rm-" + randomId()
	namespace := "system"

	// Create a key
	_, stderr, err := RunAuthenticatedCliCmd(t, "kv", "set", namespace, "STRING", key, "to-delete")
	require.NoError(t, err, "set should succeed, stderr: %s", stderr)

	// Delete using the "rm" alias
	stdout, stderr, err := RunAuthenticatedCliCmd(t, "kv", "rm", namespace, key)
	require.NoError(t, err, "rm alias should succeed, stderr: %s", stderr)
	require.Contains(t, stdout, "successfully")

	// Verify deleted
	_, _, err = RunAuthenticatedCliCmd(t, "kv", "get", namespace, key)
	require.Error(t, err, "key should have been deleted via rm alias")
}

func TestKV_listEmpty(t *testing.T) {
	// List with a namespace that likely has no KV entries
	stdout, stderr, err := RunAuthenticatedCliCmd(t, "kv", "list", "nonexistent-ns-"+randomId())
	require.NoError(t, err, "list on empty namespace should succeed, stderr: %s", stderr)
	require.Contains(t, stdout, "0")
}

func TestKV_jsonOutput(t *testing.T) {
	key := "e2e-kv-json-out-" + randomId()
	namespace := "system"

	_, stderr, err := RunAuthenticatedCliCmd(t, "kv", "set", namespace, "STRING", key, "json-test")
	require.NoError(t, err, "set should succeed, stderr: %s", stderr)

	// Get with JSON output
	stdout, stderr, err := RunAuthenticatedCliCmd(t, "kv", "get", namespace, key, "--output", "json")
	require.NoError(t, err, "get with json output should succeed, stderr: %s", stderr)
	require.Contains(t, stdout, `"key"`)
	require.Contains(t, stdout, `"namespace"`)
	require.Contains(t, stdout, "json-test")

	// Cleanup
	_, _, _ = RunAuthenticatedCliCmd(t, "kv", "delete", namespace, key)
}
