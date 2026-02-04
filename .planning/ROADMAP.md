# Roadmap: Kestra CLI IAM Commands

## Overview

This roadmap delivers IAM-focused CLI commands so operators can manage users, roles, and groups end to end, then extend those workflows with assignments and automation-ready behavior. Phases are derived from the requirements to unlock complete, verifiable admin capabilities in a non-interactive CLI.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [x] **Phase 1: IAM Entities** - Operators can manage users, roles, and groups from the CLI
- [x] **Phase 2: Assignments and Membership** - Operators can connect users, roles, and groups
- [ ] **Phase 3: Automation-Ready Behavior** - IAM commands are safe and predictable for CI/CD

## Phase Details

### Phase 1: IAM Entities
**Goal**: Operators can create, list, and delete IAM users, roles, and groups from the CLI
**Depends on**: Nothing (first phase)
**Requirements**: USER-01, USER-02, USER-03, ROLE-01, ROLE-02, ROLE-03, GROUP-01, GROUP-02, GROUP-03
**Success Criteria** (what must be TRUE):
  1. Operator can create, list, and delete users using minimal required fields
  2. Operator can create, list, and delete roles using minimal required fields
  3. Operator can create, list, and delete groups using minimal required fields
**Plans**: 4 plans

Plans:
 - [x] 01-01-PLAN.md — Add IAM users commands
 - [x] 01-02-PLAN.md — Add IAM roles commands with permissions
 - [x] 01-03-PLAN.md — Add IAM groups commands
 - [x] 01-04-PLAN.md — Ensure CLI-created users are visible in UI

### Phase 2: Assignments and Membership
**Goal**: Operators can assign roles and manage group membership from the CLI
**Depends on**: Phase 1
**Requirements**: ASGN-01, ASGN-02, ASGN-03, ASGN-04, MEM-01, MEM-02
**Success Criteria** (what must be TRUE):
  1. Operator can attach and detach roles to users
  2. Operator can attach and detach roles to groups
  3. Operator can add and remove users from groups
**Plans**: 2 plans

Plans:
- [x] 02-01-PLAN.md — Add role bindings attach/detach commands
- [x] 02-02-PLAN.md — Add group membership attach/detach commands

### Phase 3: Automation-Ready Behavior
**Goal**: IAM commands run predictably in non-interactive automation
**Depends on**: Phase 2
**Requirements**: CLI-01, CLI-02, CLI-03
**Success Criteria** (what must be TRUE):
  1. Operator can run IAM commands non-interactively with no prompts
  2. Operator can request JSON or table output for IAM commands
  3. Operator can preview IAM changes with --dry-run without applying them
**Plans**: TBD

Plans:
- [ ] TBD during planning

## Progress

**Execution Order:**
Phases execute in numeric order: 1 → 2 → 3

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. IAM Entities | 4/4 | Complete | 2026-01-30 |
| 2. Assignments and Membership | 2/2 | Complete | 2026-01-30 |
| 3. Automation-Ready Behavior | 0/TBD | Not started | - |
