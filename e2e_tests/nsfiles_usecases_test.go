package e2e_tests

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNsfilesList_fileNotFound(t *testing.T) {
	_, stderr, err := RunAuthenticatedCliCmd(t, "nsfiles", "get", "company", "--path", "unknown-file.txt")

	require.Contains(t, stderr, "File '/unknown-file.txt' was not found in namespace 'company'")
	require.Error(t, err)
}

func TestNsfilesUpload_namespaceNotFound(t *testing.T) {
	_, stderr, err := RunAuthenticatedCliCmd(t, "nsfiles", "upload", "unknownnamespace", "./", "./o")

	require.Error(t, err)
	require.Contains(t, stderr, "namespace 'unknownnamespace' does not exist")
}
