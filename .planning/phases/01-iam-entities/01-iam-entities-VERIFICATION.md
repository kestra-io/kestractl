---
phase: 01-iam-entities
verified: 2026-01-30T15:17:23Z
status: human_needed
score: 11/12 must-haves verified
re_verification:
  previous_status: human_needed
  previous_score: 9/9
  gaps_closed: []
  gaps_remaining: []
  regressions: []
human_verification:
  - test: "Create/list/delete IAM users via CLI"
    expected: "Create returns ID+name fields, list shows ID+name columns, delete returns confirmation"
    why_human: "Requires live API/tenant and auth to validate runtime behavior"
  - test: "Create/list/delete IAM roles via CLI with permissions"
    expected: "Create accepts --permission values and outputs ID+name, list shows ID+name+flags, delete confirms"
    why_human: "Requires live API/tenant and auth to validate runtime behavior"
  - test: "Create/list/delete IAM groups via CLI"
    expected: "Create outputs ID+name, list shows ID+name, delete confirms"
    why_human: "Requires live API/tenant and auth to validate runtime behavior"
  - test: "CLI-created users appear in UI for current tenant"
    expected: "User created via CLI appears in the UI IAM users list for the same tenant"
    why_human: "Requires live API/tenant and UI access"
---

# Phase 1: IAM Entities Verification Report

**Phase Goal:** Operators can create, list, and delete IAM users, roles, and groups from the CLI
**Verified:** 2026-01-30T15:17:23Z
**Status:** human_needed
**Re-verification:** Yes — re-verification of existing phase

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
| --- | --- | --- | --- |
| 1 | Operator can create IAM users from the CLI with required fields only | ✓ VERIFIED | `src/cli/iam_users.go` builds request with required `--email` and calls `UsersAPI.CreateUser` |
| 2 | Operator can list IAM users and see ID plus a name field | ✓ VERIFIED | `src/cli/iam_users.go` calls `UsersAPI.ListUsers` and renders `ID`, `Username`, `DisplayName` |
| 3 | Operator can delete an IAM user by ID and receive a confirmation summary | ✓ VERIFIED | `src/cli/iam_users.go` calls `UsersAPI.DeleteUser` and prints ID + message |
| 4 | Operator can create IAM roles with required permissions and name fields | ✓ VERIFIED | `src/cli/iam_roles.go` requires `--name` + `--permission` and calls `RolesAPI.CreateRole` |
| 5 | Operator can list IAM roles and see ID plus name | ✓ VERIFIED | `src/cli/iam_roles.go` calls `RolesAPI.SearchRoles` and renders `ID`, `Name` |
| 6 | Operator can delete an IAM role by ID and receive a confirmation summary | ✓ VERIFIED | `src/cli/iam_roles.go` calls `RolesAPI.DeleteRole` and prints ID + message |
| 7 | Operator can create IAM groups from the CLI with required fields only | ✓ VERIFIED | `src/cli/iam_groups.go` requires `--name` and calls `GroupsAPI.CreateGroup` |
| 8 | Operator can list IAM groups and see ID plus name | ✓ VERIFIED | `src/cli/iam_groups.go` calls `GroupsAPI.SearchGroups` and renders `ID`, `Name` |
| 9 | Operator can delete an IAM group by ID and receive a confirmation summary | ✓ VERIFIED | `src/cli/iam_groups.go` calls `GroupsAPI.DeleteGroup` and prints ID + message |
| 10 | CLI can assign tenants and groups when creating a user | ✓ VERIFIED | `src/cli/iam_users.go` adds `--assign-tenant`/`--group` and sets `req.SetTenants`/`req.SetGroups` |
| 11 | Default tenant assignment uses the CLI current tenant when none provided | ✓ VERIFIED | `src/cli/iam_users.go` defaults to `client.Tenant` when `--assign-tenant` not set |
| 12 | User created via CLI appears in UI for current tenant | ? UNCERTAIN | Requires live tenant + UI verification; not provable from code alone |

**Score:** 11/12 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
| --- | --- | --- | --- |
| `src/cli/iam.go` | IAM command group with users/roles/groups | ✓ VERIFIED | Defines `newIamCommand()` and adds users, roles, groups subcommands |
| `src/cli/iam_users.go` | Users create/list/delete commands and SDK calls | ✓ VERIFIED | Cobra commands + `UsersAPI.CreateUser/ListUsers/DeleteUser` + tenant/group wiring |
| `src/cli/iam_roles.go` | Roles create/list/delete commands and SDK calls | ✓ VERIFIED | Cobra commands + `RolesAPI.CreateRole/SearchRoles/DeleteRole` |
| `src/cli/iam_role_permissions.go` | Role permissions parsing | ✓ VERIFIED | Parses `--permission` values into SDK permissions struct |
| `src/cli/iam_groups.go` | Groups create/list/delete commands and SDK calls | ✓ VERIFIED | Cobra commands + `GroupsAPI.CreateGroup/SearchGroups/DeleteGroup` |
| `src/cli/root.go` | Root command registration for iam | ✓ VERIFIED | `root.AddCommand(newIamCommand())` |

### Key Link Verification

| From | To | Via | Status | Details |
| --- | --- | --- | --- | --- |
| `src/cli/root.go` | `newIamCommand` | `AddCommand(newIamCommand())` | ✓ WIRED | IAM group command registered on root |
| `src/cli/iam_users.go` | `client.API.UsersAPI.CreateUser` | create run function | ✓ WIRED | `CreateUser(...).IAMUserControllerApiCreateOrUpdateUserRequest(req).Execute()` |
| `src/cli/iam_users.go` | `client.API.UsersAPI.ListUsers` | list run function | ✓ WIRED | `ListUsers(...).Page(1).Size(1000).Execute()` |
| `src/cli/iam_users.go` | `client.API.UsersAPI.DeleteUser` | delete run function | ✓ WIRED | `DeleteUser(...).Execute()` |
| `src/cli/iam_roles.go` | `client.API.RolesAPI.CreateRole` | create run function | ✓ WIRED | `CreateRole(...).IAMRoleControllerApiRoleCreateOrUpdateRequest(req).Execute()` |
| `src/cli/iam_roles.go` | `client.API.RolesAPI.SearchRoles` | list run function | ✓ WIRED | `SearchRoles(...).Page(1).Size(1000).Execute()` |
| `src/cli/iam_roles.go` | `client.API.RolesAPI.DeleteRole` | delete run function | ✓ WIRED | `DeleteRole(...).Execute()` |
| `src/cli/iam_groups.go` | `client.API.GroupsAPI.CreateGroup` | create run function | ✓ WIRED | `CreateGroup(...).IAMGroupControllerApiCreateGroupRequest(req).Execute()` |
| `src/cli/iam_groups.go` | `client.API.GroupsAPI.SearchGroups` | list run function | ✓ WIRED | `SearchGroups(...).Page(1).Size(1000).Execute()` |
| `src/cli/iam_groups.go` | `client.API.GroupsAPI.DeleteGroup` | delete run function | ✓ WIRED | `DeleteGroup(...).Execute()` |
| `src/cli/iam_users.go` | `IAMUserControllerApiCreateOrUpdateUserRequest` | `SetTenants`/`SetGroups` | ✓ WIRED | Request sets tenants/groups from flags or defaults |
| `src/cli/iam_users.go` | `client.Tenant` | default tenant fallback | ✓ WIRED | Uses `client.Tenant` when `--assign-tenant` not provided |

### Requirements Coverage

| Requirement | Status | Blocking Issue |
| --- | --- | --- |
| USER-01 | ✓ SATISFIED | None |
| USER-02 | ✓ SATISFIED | None |
| USER-03 | ✓ SATISFIED | None |
| ROLE-01 | ✓ SATISFIED | None |
| ROLE-02 | ✓ SATISFIED | None |
| ROLE-03 | ✓ SATISFIED | None |
| GROUP-01 | ✓ SATISFIED | None |
| GROUP-02 | ✓ SATISFIED | None |
| GROUP-03 | ✓ SATISFIED | None |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| --- | --- | --- | --- | --- |
| None | - | - | - | No stub patterns detected in IAM CLI files |

### Human Verification Required

### 1. Create/list/delete IAM users via CLI

**Test:** Run `kestra iam users create --email user@example.com`, `kestra iam users list`, `kestra iam users delete <id>`
**Expected:** Create outputs ID + name fields; list returns ID + name columns; delete prints confirmation summary
**Why human:** Requires live API connectivity and credentials

### 2. Create/list/delete IAM roles via CLI with permissions

**Test:** Run `kestra iam roles create --name ops --permission FLOW:READ`, `kestra iam roles list`, `kestra iam roles delete <id>`
**Expected:** Create accepts permissions; list shows ID + name and flags; delete prints confirmation summary
**Why human:** Requires live API connectivity and credentials

### 3. Create/list/delete IAM groups via CLI

**Test:** Run `kestra iam groups create --name ops`, `kestra iam groups list`, `kestra iam groups delete <id>`
**Expected:** Create outputs ID + name; list shows ID + name; delete prints confirmation summary
**Why human:** Requires live API connectivity and credentials

### 4. CLI-created users appear in UI for current tenant

**Test:** Run `kestra iam users create --email ui-check@example.com` and check the UI IAM users list for the same tenant
**Expected:** Newly created user appears in the UI list
**Why human:** Requires live tenant and UI access

### Gaps Summary

No code-level gaps found. Runtime verification against a live Kestra API (and UI for tenant visibility) is still required.

---

_Verified: 2026-01-30T15:17:23Z_
_Verifier: Claude (gsd-verifier)_
