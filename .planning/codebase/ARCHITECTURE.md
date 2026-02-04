# Architecture

**Analysis Date:** 2026-01-30

## Pattern Overview

**Overall:** Cobra-based CLI with command modules and shared SDK client

**Key Characteristics:**
- Command tree defined in `src/cli/root.go` and composed from per-domain command files
- Shared API client and auth config in `src/cli/client.go` and `src/cli/auth.go`
- Output formatting and table/JSON helpers centralized in `src/cli/helpers.go`

## Layers

**Entry + Command Wiring:**
- Purpose: CLI bootstrapping and global flag/config initialization
- Location: `main.go`, `src/cli/root.go`
- Contains: `main()`, `Execute()`, root Cobra command, global flags, Viper init
- Depends on: `src/cli/client.go`, `src/cli/config.go`, `src/cli/flows.go`, `src/cli/executions.go`, `src/cli/namespaces.go`
- Used by: CLI binary entry at `main.go`

**Command Handlers:**
- Purpose: Implement subcommands and orchestration logic
- Location: `src/cli/flows.go`, `src/cli/executions.go`, `src/cli/namespaces.go`, `src/cli/config.go`
- Contains: Cobra command constructors plus `run...` handler functions
- Depends on: `src/cli/client.go`, `src/cli/helpers.go`, `src/cli/auth.go`
- Used by: Root command in `src/cli/root.go`

**Client + Auth + Config:**
- Purpose: Build SDK client, resolve configuration, persist contexts
- Location: `src/cli/client.go`, `src/cli/auth.go`, `src/cli/root.go`
- Contains: `Client` wrapper, `NewClient()`, `AuthManager`, Viper config loading
- Depends on: Kestra SDK (`github.com/kestra-io/client-sdk/...`), Viper, YAML
- Used by: All command handlers in `src/cli/flows.go`, `src/cli/executions.go`, `src/cli/namespaces.go`, `src/cli/config.go`

**Formatting + Utilities:**
- Purpose: JSON output and table formatting utilities, output validation
- Location: `src/cli/helpers.go`
- Contains: `printJSON`, `tabWriter`, `validateOutputFormat`, string helpers
- Depends on: standard library only
- Used by: Command handlers in `src/cli/flows.go`, `src/cli/executions.go`, `src/cli/namespaces.go`

## Data Flow

**Command Execution Flow:**

1. CLI entry in `main.go` calls `cli.Execute()` (`src/cli/root.go`).
2. Root command loads config (Viper) and populates `globalFlags` (`src/cli/root.go`).
3. Subcommand `RunE` validates output format and creates client via `NewClient()` (`src/cli/helpers.go`, `src/cli/client.go`).
4. Handler calls Kestra SDK via `Client.API` (e.g., `FlowsAPI`, `ExecutionsAPI`) (`src/cli/flows.go`, `src/cli/executions.go`, `src/cli/namespaces.go`).
5. Results formatted to JSON or table output (`src/cli/helpers.go`).

**Flows Deploy/Validate Flow:**

1. File/dir scan and YAML read in `runFlowsDeploy()` / `runFlowsValidate()` (`src/cli/flows.go`).
2. Parse or modify YAML via `parseFlowYAML()` and `replaceNamespaceInYAML()` (`src/cli/flows.go`).
3. SDK call: create/update/validate through `FlowsAPI` (`src/cli/flows.go`).
4. Aggregate per-file results and format output (`src/cli/flows.go`, `src/cli/helpers.go`).

**State Management:**
- Global CLI state stored in `globalFlags` (output/host/auth) in `src/cli/root.go`.
- Configuration precedence handled by Viper (flags > env > config) in `src/cli/root.go` and `src/cli/client.go`.

## Key Abstractions

**Client:**
- Purpose: Wrap SDK client with auth context and tenant
- Examples: `Client` and `NewClient()` in `src/cli/client.go`
- Pattern: Factory (`newClientFunc`) for test injection in `src/cli/client.go`

**AuthManager:**
- Purpose: Persist and resolve auth contexts in YAML config
- Examples: `AuthManager` in `src/cli/auth.go`
- Pattern: File-backed repository (read/modify/write) in `src/cli/auth.go`

**Command Result Structs:**
- Purpose: Stable JSON/table output shapes
- Examples: `DeployResult`, `ValidateResult` in `src/cli/flows.go`
- Pattern: DTO structs for output formatting

## Entry Points

**CLI Binary:**
- Location: `main.go`
- Triggers: Executing the `kestra` CLI binary
- Responsibilities: Bootstraps command execution and exit handling

**Root Command:**
- Location: `src/cli/root.go`
- Triggers: `cli.Execute()` from `main.go`
- Responsibilities: Define global flags, init config, register subcommands

**Debug Script:**
- Location: `scripts/debug_update.go`
- Triggers: `go run scripts/debug_update.go`
- Responsibilities: Manual SDK debugging (update flow ordering and timing)

## Error Handling

**Strategy:** Return errors to Cobra `RunE` and map SDK errors to user-friendly messages

**Patterns:**
- SDK error parsing via `formatSDKError()` in `src/cli/client.go`
- Input validation with `cobra.ExactArgs` and custom checks in `src/cli/flows.go` and `src/cli/executions.go`

## Cross-Cutting Concerns

**Logging:** `fmt.Printf` only in verbose mode and command output (`src/cli/root.go`, `src/cli/executions.go`)
**Validation:** CLI flag/output validation in `src/cli/helpers.go`, YAML parsing in `src/cli/flows.go`
**Authentication:** Token/basic auth resolved in `src/cli/client.go`, persisted in `src/cli/auth.go`

---

*Architecture analysis: 2026-01-30*
