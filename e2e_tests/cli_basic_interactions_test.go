package e2e_tests

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestHelpFlag(t *testing.T) {
	stdout, stderr, err := RunCliCmd(t, "--help")
	require.NoError(t, err)

	require.Empty(t, stderr)
	require.Contains(t, stdout, "kestractl is a command-line tool")
}
func TestUnknownCmd(t *testing.T) {
	_, stderr, _ := RunCliCmd(t, "unknownCMD")

	require.Contains(t, stderr, "Error: unknown command \"unknownCMD\"")
	require.Contains(t, stderr, "Run 'kestractl --help' for usage.")
}
func TestUnknownFlag(t *testing.T) {
	_, stderr, _ := RunCliCmd(t, "--unknownFlag")

	require.Contains(t, stderr, "Error: unknown flag: --unknownFlag")
}
