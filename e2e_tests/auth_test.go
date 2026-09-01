package e2e_tests

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSimpleCmd_Unauthenticated(t *testing.T) {
	_, stderr, _ := RunCliCmd(t, "namespaces", "list", "company")
	require.Contains(t, stderr, "Error: could not init client without any auth")
}
func TestSimpleCmd_WrongServer(t *testing.T) {
	_, stderr, _ := RunCliCmd(t, "namespaces", "list", "company", "--username", "wrongusername", "--password", "wrongpassword")
	require.Contains(t, stderr, "connect: connection refused")
}
func TestSimpleCmd_WronglyAuthenticated(t *testing.T) {
	_, stderr, _ := RunCliCmd(t, "namespaces", "list", "company", "--host", "http://localhost:9801", "--username", "wrongusername", "--password", "wrongpassword")
	// Pre-2.0 servers answer {"message":"Unauthorized"}; 2.0 answers an RFC 7807
	// problem document whose title is "Authentication required". Both must surface
	// as a formatted API error rather than a raw status line.
	require.Regexp(t, `Error: API error: (Unauthorized|Authentication required)`, stderr)
}

func TestSimpleCmd_Authenticated(t *testing.T) {
	stdout, stderr, err := RunCliCmd(t, "namespaces", "list", "company", "--host", "http://localhost:9801", "--username", "root@root.com", "--password", "Root!1234")

	require.NoError(t, err)
	require.Empty(t, stderr)

	require.NotEmpty(t, stdout)
}
