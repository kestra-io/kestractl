# Requirements: Kestra CLI IAM Commands

**Defined:** 2026-01-30
**Core Value:** Provision IAM users, roles, and groups safely and repeatably from the CLI for ops and automation.

## v1 Requirements

Requirements for initial release. Each maps to roadmap phases.

### Users

- [ ] **USER-01**: Operator can create a user with minimal required fields
- [ ] **USER-02**: Operator can list users
- [ ] **USER-03**: Operator can delete a user by identifier

### Roles

- [ ] **ROLE-01**: Operator can create a role with minimal required fields
- [ ] **ROLE-02**: Operator can list roles
- [ ] **ROLE-03**: Operator can delete a role by identifier

### Groups

- [ ] **GROUP-01**: Operator can create a group with minimal required fields
- [ ] **GROUP-02**: Operator can list groups
- [ ] **GROUP-03**: Operator can delete a group by identifier

### Assignments

- [ ] **ASGN-01**: Operator can attach a role to a user
- [ ] **ASGN-02**: Operator can detach a role from a user
- [ ] **ASGN-03**: Operator can attach a role to a group
- [ ] **ASGN-04**: Operator can detach a role from a group

### Membership

- [ ] **MEM-01**: Operator can add a user to a group
- [ ] **MEM-02**: Operator can remove a user from a group

### CLI Behavior

- [ ] **CLI-01**: Commands run non-interactively with no prompts
- [ ] **CLI-02**: Commands support `--output=json` and table output
- [ ] **CLI-03**: Commands support `--dry-run` to preview changes

## v2 Requirements

Deferred to future release. Tracked but not in current roadmap.

### Users

- **USER-04**: Operator can get user details by identifier
- **USER-05**: Operator can update user fields

### Roles

- **ROLE-04**: Operator can get role details by identifier
- **ROLE-05**: Operator can update role fields

### Groups

- **GROUP-04**: Operator can get group details by identifier
- **GROUP-05**: Operator can update group fields

### Service Accounts

- **SVC-01**: Operator can create, list, and delete service accounts
- **SVC-02**: Operator can attach and detach roles for service accounts

## Out of Scope

Explicitly excluded. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Interactive prompts | CI/CD and automation require non-interactive behavior |
| Separate IAM auth config | Reuse existing CLI auth for consistency |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| USER-01 | Phase 1 | Complete |
| USER-02 | Phase 1 | Complete |
| USER-03 | Phase 1 | Complete |
| ROLE-01 | Phase 1 | Complete |
| ROLE-02 | Phase 1 | Complete |
| ROLE-03 | Phase 1 | Complete |
| GROUP-01 | Phase 1 | Complete |
| GROUP-02 | Phase 1 | Complete |
| GROUP-03 | Phase 1 | Complete |
| ASGN-01 | Phase 2 | Pending |
| ASGN-02 | Phase 2 | Pending |
| ASGN-03 | Phase 2 | Pending |
| ASGN-04 | Phase 2 | Pending |
| MEM-01 | Phase 2 | Pending |
| MEM-02 | Phase 2 | Pending |
| CLI-01 | Phase 3 | Pending |
| CLI-02 | Phase 3 | Pending |
| CLI-03 | Phase 3 | Pending |

**Coverage:**
- v1 requirements: 18 total
- Mapped to phases: 18
- Unmapped: 0

---
*Requirements defined: 2026-01-30*
*Last updated: 2026-01-30 after roadmap creation*
