package e2e_tests

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNamespacesList_IamRoleAuthorization(t *testing.T) {
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	roleName := fmt.Sprintf("dev-e2e-%s", suffix)
	roleID := createIamRole(t, roleName, []string{"NAMESPACE:READ"})

	allowedEmail := fmt.Sprintf("dev.user.%s@example.com", suffix)
	allowedPassword := "DevUser!1234"
	allowedUserID := createIamUser(t, allowedEmail, allowedPassword)

	deniedEmail := fmt.Sprintf("no.role.%s@example.com", suffix)
	deniedPassword := "NoRole!1234"
	deniedUserID := createIamUser(t, deniedEmail, deniedPassword)

	t.Cleanup(func() {
		deleteIamUser(t, allowedUserID)
		deleteIamUser(t, deniedUserID)
		deleteIamRole(t, roleID)
	})

	attachIamRoleToUser(t, roleName, allowedUserID)

	stdout, stderr, err := RunCliCmd(
		t,
		"namespaces",
		"list",
		"--query",
		"system",
		"--host",
		"http://localhost:9801",
		"--tenant",
		"main",
		"--username",
		allowedEmail,
		"--password",
		allowedPassword,
	)

	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "system")

	_, stderr, err = RunCliCmd(
		t,
		"namespaces",
		"list",
		"--query",
		"system",
		"--host",
		"http://localhost:9801",
		"--tenant",
		"main",
		"--username",
		deniedEmail,
		"--password",
		deniedPassword,
	)

	require.Error(t, err)
	require.Contains(t, stderr, "Forbidden")
}

func createIamRole(t *testing.T, name string, permissions []string) string {
	t.Helper()

	args := []string{"iam", "roles", "create", "--name", name, "--output", "json"}
	for _, permission := range permissions {
		args = append(args, "--permission", permission)
	}

	stdout, stderr, err := RunAuthenticatedCliCmd(t, args...)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload map[string]any
	err = json.Unmarshal([]byte(stdout), &payload)
	require.NoError(t, err)

	id, ok := payload["id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, id)

	return id
}

func createIamUser(t *testing.T, email string, password string) string {
	t.Helper()

	stdout, stderr, err := RunAuthenticatedCliCmd(
		t,
		"iam",
		"users",
		"create",
		"--email",
		email,
		"--password",
		password,
		"--assign-tenant",
		"main",
		"--output",
		"json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload map[string]any
	err = json.Unmarshal([]byte(stdout), &payload)
	require.NoError(t, err)

	id, ok := payload["id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, id)

	return id
}

func attachIamRoleToUser(t *testing.T, role string, userID string) {
	t.Helper()

	_, stderr, err := RunAuthenticatedCliCmd(
		t,
		"iam",
		"roles",
		"attach",
		"--role",
		role,
		"--user",
		userID,
	)
	require.NoError(t, err)
	require.Empty(t, stderr)
}

func deleteIamUser(t *testing.T, id string) {
	t.Helper()
	if id == "" {
		return
	}

	_, stderr, err := RunAuthenticatedCliCmd(t, "iam", "users", "delete", id)
	require.NoError(t, err)
	require.Empty(t, stderr)
}

func deleteIamRole(t *testing.T, id string) {
	t.Helper()
	if id == "" {
		return
	}

	_, stderr, err := RunAuthenticatedCliCmd(t, "iam", "roles", "delete", id)
	require.NoError(t, err)
	require.Empty(t, stderr)
}
