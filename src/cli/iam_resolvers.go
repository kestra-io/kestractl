package cli

import (
	"fmt"
	"strings"

	kestra "github.com/kestra-io/client-sdk/go-sdk/kestra_api_client"
)

type iamResolvedIdentifier struct {
	ID   string
	Name string
}

type iamResolvedUser struct {
	ID          string
	Username    string
	DisplayName string
	Name        string
}

func resolveIamRoleIdentifier(client *Client, value string) (*iamResolvedIdentifier, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, fmt.Errorf("role identifier is required")
	}

	resp, _, err := client.API.RolesAPI.SearchRoles(client.Ctx, client.Tenant).
		Page(1).
		Size(1000).
		Execute()
	if err != nil {
		return nil, formatSDKError(err)
	}

	results := resp.GetResults()
	idMatches := make([]kestra.ApiRoleSummary, 0)
	nameMatches := make([]kestra.ApiRoleSummary, 0)

	for _, role := range results {
		if role.GetId() == trimmed {
			idMatches = append(idMatches, role)
			continue
		}
		if role.GetName() == trimmed {
			nameMatches = append(nameMatches, role)
		}
	}

	if len(idMatches) > 1 {
		return nil, fmt.Errorf("multiple roles matched id '%s': %s", trimmed, formatRoleMatches(idMatches))
	}
	if len(idMatches) == 1 {
		role := idMatches[0]
		return &iamResolvedIdentifier{ID: role.GetId(), Name: role.GetName()}, nil
	}

	if len(nameMatches) == 0 {
		return nil, fmt.Errorf("role not found: %s", trimmed)
	}
	if len(nameMatches) > 1 {
		return nil, fmt.Errorf("multiple roles matched name '%s': %s", trimmed, formatRoleMatches(nameMatches))
	}

	role := nameMatches[0]
	return &iamResolvedIdentifier{ID: role.GetId(), Name: role.GetName()}, nil
}

func resolveIamGroupIdentifier(client *Client, value string) (*iamResolvedIdentifier, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, fmt.Errorf("group identifier is required")
	}

	resp, _, err := client.API.GroupsAPI.SearchGroups(client.Ctx, client.Tenant).
		Page(1).
		Size(1000).
		Execute()
	if err != nil {
		return nil, formatSDKError(err)
	}

	results := resp.GetResults()
	idMatches := make([]kestra.ApiGroupSummary, 0)
	nameMatches := make([]kestra.ApiGroupSummary, 0)

	for _, group := range results {
		if group.GetId() == trimmed {
			idMatches = append(idMatches, group)
			continue
		}
		if group.GetName() == trimmed {
			nameMatches = append(nameMatches, group)
		}
	}

	if len(idMatches) > 1 {
		return nil, fmt.Errorf("multiple groups matched id '%s': %s", trimmed, formatGroupMatches(idMatches))
	}
	if len(idMatches) == 1 {
		group := idMatches[0]
		return &iamResolvedIdentifier{ID: group.GetId(), Name: group.GetName()}, nil
	}

	if len(nameMatches) == 0 {
		return nil, fmt.Errorf("group not found: %s", trimmed)
	}
	if len(nameMatches) > 1 {
		return nil, fmt.Errorf("multiple groups matched name '%s': %s", trimmed, formatGroupMatches(nameMatches))
	}

	group := nameMatches[0]
	return &iamResolvedIdentifier{ID: group.GetId(), Name: group.GetName()}, nil
}

func resolveIamUserIdentifier(client *Client, value string) (*iamResolvedUser, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, fmt.Errorf("user identifier is required")
	}

	resp, _, err := client.API.UsersAPI.ListUsers(client.Ctx).
		Page(1).
		Size(1000).
		Execute()
	if err != nil {
		return nil, formatSDKError(err)
	}

	results := resp.GetResults()
	idMatches := make([]kestra.IAMUserControllerApiUserSummary, 0)
	usernameMatches := make([]kestra.IAMUserControllerApiUserSummary, 0)
	displayMatches := make([]kestra.IAMUserControllerApiUserSummary, 0)

	for _, user := range results {
		if user.GetId() == trimmed {
			idMatches = append(idMatches, user)
			continue
		}
		if user.GetUsername() == trimmed {
			usernameMatches = append(usernameMatches, user)
			continue
		}
		if user.GetDisplayName() == trimmed {
			displayMatches = append(displayMatches, user)
		}
	}

	if len(idMatches) > 1 {
		return nil, fmt.Errorf("multiple users matched id '%s': %s", trimmed, formatUserMatches(idMatches))
	}
	if len(idMatches) == 1 {
		return mapUserIdentity(idMatches[0]), nil
	}

	if len(usernameMatches) > 1 {
		return nil, fmt.Errorf("multiple users matched username '%s': %s", trimmed, formatUserMatches(usernameMatches))
	}
	if len(usernameMatches) == 1 {
		return mapUserIdentity(usernameMatches[0]), nil
	}

	if len(displayMatches) == 0 {
		return nil, fmt.Errorf("user not found: %s", trimmed)
	}
	if len(displayMatches) > 1 {
		return nil, fmt.Errorf("multiple users matched display name '%s': %s", trimmed, formatUserMatches(displayMatches))
	}

	return mapUserIdentity(displayMatches[0]), nil
}

func mapUserIdentity(user kestra.IAMUserControllerApiUserSummary) *iamResolvedUser {
	displayName := user.GetDisplayName()
	username := user.GetUsername()
	name := displayName
	if strings.TrimSpace(name) == "" {
		name = username
	}

	return &iamResolvedUser{
		ID:          user.GetId(),
		Username:    username,
		DisplayName: displayName,
		Name:        name,
	}
}

func formatRoleMatches(matches []kestra.ApiRoleSummary) string {
	parts := make([]string, 0, len(matches))
	for _, role := range matches {
		parts = append(parts, fmt.Sprintf("%s (%s)", role.GetId(), withFallback(role.GetName())))
	}
	return strings.Join(parts, ", ")
}

func formatGroupMatches(matches []kestra.ApiGroupSummary) string {
	parts := make([]string, 0, len(matches))
	for _, group := range matches {
		parts = append(parts, fmt.Sprintf("%s (%s)", group.GetId(), withFallback(group.GetName())))
	}
	return strings.Join(parts, ", ")
}

func formatUserMatches(matches []kestra.IAMUserControllerApiUserSummary) string {
	parts := make([]string, 0, len(matches))
	for _, user := range matches {
		parts = append(parts, fmt.Sprintf("%s (%s, %s)", user.GetId(), withFallback(user.GetUsername()), withFallback(user.GetDisplayName())))
	}
	return strings.Join(parts, ", ")
}
