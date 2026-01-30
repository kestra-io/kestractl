# Kestra CLI IAM Commands

## What This Is

Add IAM-focused CLI commands to the Kestra CLI so ops teams and automation can provision users, roles, and groups. The goal is to support practical admin workflows like creating users and attaching them to existing roles, including CI/CD and agent-driven automation.

## Core Value

Provision IAM users, roles, and groups safely and repeatably from the CLI for ops and automation.

## Requirements

### Validated

- ✓ CLI uses the Kestra Go SDK for API calls and supports authenticated client creation — existing
- ✓ CLI supports JSON output formatting for machine-readable results — existing
- ✓ CLI is structured around Cobra commands with shared helpers — existing

### Active

- [ ] CLI commands for IAM users, roles, and groups with CRUD + list
- [ ] Role assignment commands (attach/detach roles to users)
- [ ] Group membership commands (add/remove users to groups)
- [ ] IAM provisioning flow support: create role/group, create user, attach role, add to group
- [ ] Non-interactive behavior for CI/CD and agents
- [ ] Machine-readable JSON output for all IAM commands
- [ ] Dry-run support for IAM changes
- [ ] Minimal required fields on create; avoid forcing optional fields

### Out of Scope

- Service account support — deferred to v2
- Interactive prompts — CI/CD and automation require non-interactive behavior

## Context

- Existing CLI uses `github.com/kestra-io/client-sdk/go-sdk` and Cobra/Viper for command wiring and config.
- Auth and config are shared across commands and should be reused for IAM operations.
- Target users include ops teams and automation (including AI agents) provisioning IAM manually or from CI/CD.

## Constraints

- **Auth**: Reuse existing CLI authentication and config flow — avoids separate IAM credentials
- **Automation**: Must be non-interactive and support machine-readable output — CI/CD and agents

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Use existing CLI auth | Keep IAM commands consistent with current CLI workflows | — Pending |
| Command shape: `iam users|roles|groups` | Align with plural noun grouping and clarity | — Pending |
| V1 includes users/roles/groups + assignments | Supports core admin workflows | — Pending |
| Service accounts deferred | Focus on core IAM first | — Pending |

---
*Last updated: 2026-01-30 after initialization*
