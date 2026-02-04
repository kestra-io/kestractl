# Phase 1: IAM Entities - Context

**Gathered:** 2026-01-30
**Status:** Ready for planning

<domain>
## Phase Boundary

Operators can create, list, and delete IAM users, roles, and groups from the CLI.

</domain>

<decisions>
## Implementation Decisions

### Command structure
- Use plural-noun command groups: `kestra iam users|roles|groups <verb>`
- Delete uses positional identifier (e.g., `kestra iam users delete <id>`)
- Create accepts flags only (no JSON file input) and keeps list commands simple with no pagination flags

### Entity identifiers
- Delete identifiers are IDs only (not names)
- Create requires name only; IDs are server-generated (no `--id` on create)
- List output should include both name and ID by default

### Delete behavior
- Delete runs without a `--force` flag
- Delete returns an error if the entity does not exist
- Delete prints a confirmation summary and does not support batch deletes in v1

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

*Phase: 01-iam-entities*
*Context gathered: 2026-01-30*
