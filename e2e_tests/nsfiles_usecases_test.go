package e2e_tests

import (
	"crypto/rand"
	"encoding/hex"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNsfilesList_fileNotFound(t *testing.T) {
	_, stderr, err := RunAuthenticatedCliCmd(t, "nsfiles", "get", "company", "unknown-file.txt")

	require.Contains(t, stderr, "Error: namespace file")
	require.Contains(t, stderr, "not found")
	require.Error(t, err)
}

func TestNsfilesUpload_namespaceNotFound(t *testing.T) {
	_, stderr, err := RunAuthenticatedCliCmd(t, "nsfiles", "upload", "unknownnamespace", "./", "./o")

	require.Error(t, err)
	require.Contains(t, stderr, "namespace 'unknownnamespace' does not exist")
}

func TestNsfilesUpload_allowRelative(t *testing.T) {
	t.Skip("to handle") // TODO
	_, stderr, err := RunAuthenticatedCliCmd(t, "nsfiles", "upload", "system", "./", "./", "--override")
	require.Empty(t, stderr)
	require.NoError(t, err)
}

func TestNsfilesUpload_e2e(t *testing.T) {
	fileNsPath := getRandomNsFilePath()
	_, _, err := RunAuthenticatedCliCmd(t, "nsfiles", "get", "system", fileNsPath)
	require.Error(t, err, "file should not exist before test")

	_, stderr, err := RunAuthenticatedCliCmd(t, "nsfiles", "upload", "system", "./README.md", fileNsPath)
	require.Empty(t, stderr)
	require.NoError(t, err)

	stdout, stderr, err := RunAuthenticatedCliCmd(t, "nsfiles", "get", "system", fileNsPath)
	require.Empty(t, stderr)
	require.NoError(t, err, "file should exist after upload")
	require.Contains(t, stdout, "e2e tests")

	_, _, err = RunAuthenticatedCliCmd(t, "nsfiles", "delete", "system", fileNsPath)
	require.NoError(t, err)

	_, _, err = RunAuthenticatedCliCmd(t, "nsfiles", "get", "system", fileNsPath)
	require.Error(t, err, "file should have been deleted")
}

// Regression test for #130: deleting a subdirectory recursively used to wipe
// every sibling file in the parent directory.
func TestNsfilesDeleteRecursive_keepsSiblings(t *testing.T) {
	dir := "e2e-nsfiles-tests/" + randomId()
	keep := dir + "/keep.txt"
	nested := dir + "/sub/x.txt"

	_, stderr, err := RunAuthenticatedCliCmd(t, "nsfiles", "upload", "system", "./README.md", keep)
	require.Empty(t, stderr)
	require.NoError(t, err)

	_, stderr, err = RunAuthenticatedCliCmd(t, "nsfiles", "upload", "system", "./README.md", nested)
	require.Empty(t, stderr)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _, _ = RunAuthenticatedCliCmd(t, "nsfiles", "delete", "system", dir, "--recursive", "--force")
	})

	_, stderr, err = RunAuthenticatedCliCmd(t, "nsfiles", "delete", "system", dir+"/sub", "--recursive")
	require.Empty(t, stderr)
	require.NoError(t, err)

	_, _, err = RunAuthenticatedCliCmd(t, "nsfiles", "get", "system", nested)
	require.Error(t, err, "deleted file should be gone")

	stdout, stderr, err := RunAuthenticatedCliCmd(t, "nsfiles", "get", "system", keep)
	require.Empty(t, stderr)
	require.NoError(t, err, "sibling file must survive a recursive delete of a subdirectory")
	require.Contains(t, stdout, "e2e tests")
}

func getRandomNsFilePath() string {
	return "e2e-nsfiles-tests/" + randomId()
}
func randomId() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
