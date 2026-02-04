# Phase 2: Assignments and Membership - Context

**Gathered:** 2026-01-30
**Status:** Ready for planning

<domain>
## Phase Boundary

Operators can assign roles to users or groups, and manage group membership from the CLI.

</domain>

<decisions>
## Implementation Decisions

### Role assignment commands
- Commands live under `kestra iam roles bindings`
- Verbs: `attach` / `detach`
- Targets specified via flags: `--user <id-or-name>` or `--group <id-or-name>` (exactly one)
- Role specified via `--role <id-or-name>`
- Single assignment per command (no bulk)

### Membership commands
- Commands live under `kestra iam groups`
- Verbs: `attach` / `detach`
- Single membership change per command (no bulk)
- Accept IDs or names for user/group

### Output + feedback
- Table output returns updated entity view
- JSON output returns a compact result payload
- Include resolved ID + name when names are provided
- Success output includes action + target

### Error behavior
- Not found is an error (user/group/role)
- Already assigned is an error
- Unassign/remove of non-existent relationship is an error
- Name ambiguity is an error with choices

### Claude's Discretion
None specified.

</decisions>

<specifics>
## Specific Ideas

No specific requirements — open to standard approaches.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 02-assignments-and-membership*
*Context gathered: 2026-01-30*
