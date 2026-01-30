package cli

import (
	"fmt"
	"sort"
	"strings"

	kestra "github.com/kestra-io/client-sdk/go-sdk/kestra_api_client"
)

var iamRolePermissionResources = map[string]struct{}{
	"FLOW":             {},
	"BLUEPRINT":        {},
	"TEMPLATE":         {},
	"NAMESPACE":        {},
	"EXECUTION":        {},
	"USER":             {},
	"GROUP":            {},
	"ROLE":             {},
	"BINDING":          {},
	"AUDITLOG":         {},
	"SECRET":           {},
	"KVSTORE":          {},
	"IMPERSONATE":      {},
	"SETTING":          {},
	"APP":              {},
	"ASSET":            {},
	"APPEXECUTION":     {},
	"TEST":             {},
	"DASHBOARD":        {},
	"TENANT_ACCESS":    {},
	"SERVICE_ACCOUNT":  {},
	"INVITATION":       {},
	"GROUP_MEMBERSHIP": {},
}

func parseIAMRolePermissions(values []string) (kestra.IAMRoleControllerApiRoleCreateOrUpdateRequestPermissions, error) {
	permissions := kestra.IAMRoleControllerApiRoleCreateOrUpdateRequestPermissions{}

	if len(values) == 0 {
		return permissions, fmt.Errorf("at least one --permission value is required")
	}

	aggregated := map[string]map[string]struct{}{}

	for _, raw := range values {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return permissions, fmt.Errorf("permission cannot be empty")
		}

		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			return permissions, fmt.Errorf("invalid permission %q: expected RESOURCE:ACTION", raw)
		}

		resource := strings.ToUpper(strings.TrimSpace(parts[0]))
		action := strings.ToUpper(strings.TrimSpace(parts[1]))
		if resource == "" || action == "" {
			return permissions, fmt.Errorf("invalid permission %q: resource and action are required", raw)
		}

		if _, ok := iamRolePermissionResources[resource]; !ok {
			return permissions, fmt.Errorf("unknown permission resource %q", resource)
		}

		if aggregated[resource] == nil {
			aggregated[resource] = map[string]struct{}{}
		}
		aggregated[resource][action] = struct{}{}
	}

	if len(aggregated) == 0 {
		return permissions, fmt.Errorf("at least one valid --permission value is required")
	}

	for resource, actionsSet := range aggregated {
		actions := make([]string, 0, len(actionsSet))
		for action := range actionsSet {
			actions = append(actions, action)
		}
		sort.Strings(actions)

		switch resource {
		case "FLOW":
			permissions.FLOW = actions
		case "BLUEPRINT":
			permissions.BLUEPRINT = actions
		case "TEMPLATE":
			permissions.TEMPLATE = actions
		case "NAMESPACE":
			permissions.NAMESPACE = actions
		case "EXECUTION":
			permissions.EXECUTION = actions
		case "USER":
			permissions.USER = actions
		case "GROUP":
			permissions.GROUP = actions
		case "ROLE":
			permissions.ROLE = actions
		case "BINDING":
			permissions.BINDING = actions
		case "AUDITLOG":
			permissions.AUDITLOG = actions
		case "SECRET":
			permissions.SECRET = actions
		case "KVSTORE":
			permissions.KVSTORE = actions
		case "IMPERSONATE":
			permissions.IMPERSONATE = actions
		case "SETTING":
			permissions.SETTING = actions
		case "APP":
			permissions.APP = actions
		case "ASSET":
			permissions.ASSET = actions
		case "APPEXECUTION":
			permissions.APPEXECUTION = actions
		case "TEST":
			permissions.TEST = actions
		case "DASHBOARD":
			permissions.DASHBOARD = actions
		case "TENANT_ACCESS":
			permissions.TENANT_ACCESS = actions
		case "SERVICE_ACCOUNT":
			permissions.SERVICE_ACCOUNT = actions
		case "INVITATION":
			permissions.INVITATION = actions
		case "GROUP_MEMBERSHIP":
			permissions.GROUP_MEMBERSHIP = actions
		}
	}

	return permissions, nil
}
