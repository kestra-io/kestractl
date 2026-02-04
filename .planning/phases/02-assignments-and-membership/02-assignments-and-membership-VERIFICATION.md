---
phase: 02-assignments-and-membership
verified: 2026-01-30T18:45:05Z
status: human_needed
score: 6/6 must-haves verified
human_verification:
  - test: "Attach a role to a user"
    expected: "`kestra iam roles bindings attach --role <role> --user <user>` creates the binding and outputs action + target details"
    why_human: "Requires live API access and real IAM data"
  - test: "Detach a role from a group"
    expected: "`kestra iam roles bindings detach --role <role> --group <group>` removes the binding and outputs confirmation"
    why_human: "Requires live API access and existing bindings"
  - test: "Add/remove user membership"
    expected: "`kestra iam groups attach|detach --group <group> --user <user>` updates membership and prints formatted output"
    why_human: "Requires live API access and observable membership changes"
---

# Phase 2: Assignments and Membership Verification Report

**Phase Goal:** Operators can assign roles and manage group membership from the CLI
**Verified:** 2026-01-30T18:45:05Z
**Status:** human_needed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
| --- | --- | --- | --- |
| 1 | Operator can attach a role to a user by ID or name | ✓ VERIFIED | `src/cli/iam_role_bindings.go:41` attach command resolves role/user identifiers and posts binding via `createIamBinding` | 
| 2 | Operator can detach a role from a user by ID or name | ✓ VERIFIED | `src/cli/iam_role_bindings.go:80` detach command resolves identifiers, searches bindings, deletes matching binding | 
| 3 | Operator can attach a role to a group by ID or name | ✓ VERIFIED | `src/cli/iam_role_bindings.go:41` attach command supports `--group` target resolution and binding create | 
| 4 | Operator can detach a role from a group by ID or name | ✓ VERIFIED | `src/cli/iam_role_bindings.go:80` detach command supports `--group` target resolution and binding delete | 
| 5 | Operator can add a user to a group by ID or name | ✓ VERIFIED | `src/cli/iam_group_memberships.go:15` attach command resolves group/user and calls `GroupsAPI.AddUserToGroup` | 
| 6 | Operator can remove a user from a group by ID or name | ✓ VERIFIED | `src/cli/iam_group_memberships.go:54` detach command resolves group/user and calls `GroupsAPI.DeleteUserFromGroup` | 

**Score:** 6/6 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
| --- | --- | --- | --- |
| `src/cli/iam_resolvers.go` | ID-or-name resolution helpers with ambiguity errors | ✓ VERIFIED | Exists, substantive (212 lines), used by role bindings + group memberships | 
| `src/cli/iam_role_bindings.go` | Roles bindings attach/detach commands and SDK calls | ✓ VERIFIED | Exists, substantive (403 lines), wired via `newIamRolesBindingsCommand` and API calls | 
| `src/cli/iam_roles.go` | Roles bindings command wiring | ✓ VERIFIED | Adds `bindings` subcommand to roles | 
| `src/cli/iam_group_memberships.go` | Groups attach/detach membership commands and SDK calls | ✓ VERIFIED | Exists, substantive (175 lines), wired via `newIamGroupsAttachCommand`/`Detach` | 
| `src/cli/iam_groups.go` | Groups membership command wiring | ✓ VERIFIED | Adds `attach`/`detach` subcommands to groups | 

### Key Link Verification

| From | To | Via | Status | Details |
| --- | --- | --- | --- | --- |
| `src/cli/iam_roles.go` | `newIamRolesBindingsCommand` | `AddCommand` | ✓ WIRED | `cmd.AddCommand(newIamRolesBindingsCommand())` | 
| `src/cli/iam_role_bindings.go` | `resolveIamRoleIdentifier` | attach/detach run | ✓ WIRED | `runIamRolesBindingsAttach/Detach` call resolver | 
| `src/cli/iam_role_bindings.go` | `resolveIamUserIdentifier`/`resolveIamGroupIdentifier` | target resolution | ✓ WIRED | `resolveIamBindingTarget` routes to user/group resolvers | 
| `src/cli/iam_role_bindings.go` | API bindings endpoints | `doBindingRequest` | ✓ WIRED | Uses API config + HTTP client to create/search/delete bindings | 
| `src/cli/iam_groups.go` | `newIamGroupsAttachCommand` | `AddCommand` | ✓ WIRED | `cmd.AddCommand(newIamGroupsAttachCommand())` | 
| `src/cli/iam_group_memberships.go` | `GroupsAPI.AddUserToGroup/DeleteUserFromGroup` | attach/detach run | ✓ WIRED | Uses GroupsAPI for membership updates | 

### Requirements Coverage

| Requirement | Status | Blocking Issue |
| --- | --- | --- |
| ASGN-01 | ✓ SATISFIED | — |
| ASGN-02 | ✓ SATISFIED | — |
| ASGN-03 | ✓ SATISFIED | — |
| ASGN-04 | ✓ SATISFIED | — |
| MEM-01 | ✓ SATISFIED | — |
| MEM-02 | ✓ SATISFIED | — |

### Anti-Patterns Found

None found in the Phase 2 IAM CLI files.

### Human Verification Required

### 1. Attach a role to a user

**Test:** Run `kestra iam roles bindings attach --role <role> --user <user>`
**Expected:** Binding is created and output includes action, role, and target details
**Why human:** Requires live API access and real IAM data

### 2. Detach a role from a group

**Test:** Run `kestra iam roles bindings detach --role <role> --group <group>`
**Expected:** Binding is removed and output confirms action
**Why human:** Requires live API access and existing bindings

### 3. Add/remove user membership

**Test:** Run `kestra iam groups attach --group <group> --user <user>` then `kestra iam groups detach --group <group> --user <user>`
**Expected:** Membership changes are applied and output reflects the actions
**Why human:** Requires live API access and observable membership changes

### Gaps Summary

No structural gaps found. Functional verification depends on live API behavior.

---

_Verified: 2026-01-30T18:45:05Z_
_Verifier: Claude (gsd-verifier)_
