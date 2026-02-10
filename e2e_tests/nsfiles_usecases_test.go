package e2e_tests

import (
	"crypto/rand"
	"encoding/hex"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNsfilesList_fileNotFound(t *testing.T) {
	_, stderr, err := RunAuthenticatedCliCmd(t, "nsfiles", "get", "company", "--path", "unknown-file.txt")

	require.Contains(t, stderr, "File '/unknown-file.txt' was not found in namespace 'company'")
	require.Error(t, err)
}

func TestNsfilesUpload_namespaceNotFound(t *testing.T) {
	t.Skip("to handle, right now it uploads the file without the namespace being created") // TODO
	_, _, err := RunAuthenticatedCliCmd(t, "nsfiles", "upload", "unknownnamespace", "./", "--path", "./o")

	require.Error(t, err)
}

func TestNsfilesUpload_allowRelative(t *testing.T) {
	t.Skip("to handle") // TODO
	_, stderr, err := RunAuthenticatedCliCmd(t, "nsfiles", "upload", "system", "./", "--path", "./", "--override")
	require.Empty(t, stderr)
	require.NoError(t, err)
}

func TestNsfilesUpload_e2e(t *testing.T) {
	fileNsPath := getRandomNsFilePath()
	_, _, err := RunAuthenticatedCliCmd(t, "nsfiles", "get", "system", "--path", fileNsPath)
	require.Error(t, err, "file should not exist before test")

	_, stderr, err := RunAuthenticatedCliCmd(t, "nsfiles", "upload", "system", "./README.md", "--path", fileNsPath)
	require.Empty(t, stderr)
	require.NoError(t, err)

	stdout, stderr, err := RunAuthenticatedCliCmd(t, "nsfiles", "get", "system", "--path", fileNsPath)
	require.Empty(t, stderr)
	require.NoError(t, err, "file should exist after upload")
	require.Contains(t, stdout, "e2e tests")

	_, _, err = RunAuthenticatedCliCmd(t, "nsfiles", "delete", "system", "--path", fileNsPath)
	require.NoError(t, err)

	_, _, err = RunAuthenticatedCliCmd(t, "nsfiles", "get", "system", "--path", fileNsPath)
	require.Error(t, err, "file should have been deleted")
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
