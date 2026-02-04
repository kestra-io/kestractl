# Codebase Structure

**Analysis Date:** 2026-01-30

## Directory Layout

```
[project-root]/
├── .github/            # CI workflows
├── .opencode/          # Local agent configuration and skills
├── scripts/            # Local debug/utility scripts
├── src/                # Application source
├── CONTRIBUTING.md     # Contribution guidelines
├── README.md           # Project overview and usage
├── go.mod              # Go module definition
├── go.sum              # Go module lockfile
├── kestra              # Built CLI binary (local)
└── main.go             # CLI entry point
```

## Directory Purposes

**src/cli:**
- Purpose: CLI command implementation and shared helpers
- Contains: Cobra commands, client/auth/config, formatting helpers
- Key files: `src/cli/root.go`, `src/cli/flows.go`, `src/cli/executions.go`, `src/cli/namespaces.go`, `src/cli/client.go`, `src/cli/helpers.go`, `src/cli/auth.go`, `src/cli/config.go`

**src/cli/testdata:**
- Purpose: Test fixtures for CLI commands
- Contains: YAML flow files and fixture directories
- Key files: `src/cli/testdata/flow.yaml`, `src/cli/testdata/deploy_folder_test/flow1.yaml`

**scripts:**
- Purpose: Ad-hoc debugging tools
- Contains: Standalone Go programs
- Key files: `scripts/debug_update.go`

**.github/workflows:**
- Purpose: CI automation
- Contains: Test workflow
- Key files: `.github/workflows/tests.yml`

**.opencode:**
- Purpose: Local agent metadata and skills
- Contains: Skill docs and tool config
- Key files: `.opencode/skills/add-cli-command/SKILL.md`

## Key File Locations

**Entry Points:**
- `main.go`: CLI entry point calls `cli.Execute()`
- `src/cli/root.go`: Root Cobra command and global flag setup

**Configuration:**
- `go.mod`: Go module and dependencies
- `src/cli/root.go`: Viper configuration setup and flag bindings
- `src/cli/auth.go`: Auth context persistence in `~/.kestra/config.yaml`

**Core Logic:**
- `src/cli/flows.go`: Flow list/get/deploy/validate commands
- `src/cli/executions.go`: Execution run/get/kill commands
- `src/cli/namespaces.go`: Namespace list command
- `src/cli/client.go`: SDK client construction and error formatting
- `src/cli/helpers.go`: Output formatting utilities

**Testing:**
- `src/cli/flows_test.go`: Flow command/unit helper tests
- `src/cli/executions_test.go`: Execution command/unit helper tests
- `src/cli/namespaces_test.go`: Namespace command tests
- `src/cli/testdata/`: Fixture files used by tests

## Naming Conventions

**Files:**
- Lowercase Go files per command or concern: `flows.go`, `executions.go`, `client.go`
- Tests use `_test.go` suffix: `flows_test.go`, `executions_test.go`

**Directories:**
- Lowercase names: `src/cli`, `src/cli/testdata`, `scripts`

## Where to Add New Code

**New Feature:**
- Primary code: `src/cli/<domain>.go`
- Tests: `src/cli/<domain>_test.go`

**New Component/Module:**
- Implementation: `src/cli/<name>.go` and register in `src/cli/root.go`

**Utilities:**
- Shared helpers: `src/cli/helpers.go` or a new `src/cli/<helper>.go`

## Special Directories

**src/cli/testdata:**
- Purpose: Static fixtures for CLI tests
- Generated: No
- Committed: Yes

**scripts:**
- Purpose: Local debugging and experimentation
- Generated: No
- Committed: Yes

**.github/workflows:**
- Purpose: CI pipeline definitions
- Generated: No
- Committed: Yes

---

*Structure analysis: 2026-01-30*
