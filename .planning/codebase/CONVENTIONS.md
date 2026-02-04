# Coding Conventions

**Analysis Date:** 2026-01-30

## Naming Patterns

**Files:**
- Use lowercase file names; test files use `_test.go` suffix in `src/cli/flows.go` and `src/cli/flows_test.go`.

**Functions:**
- Exported functions use PascalCase (e.g., `NewRootCommand`, `Execute`) in `src/cli/root.go` and `src/cli/client.go`.
- Unexported helpers use lowerCamelCase (e.g., `newFlowsCommand`, `runFlowsList`) in `src/cli/flows.go`.

**Variables:**
- Local variables use lowerCamelCase (e.g., `namespaceOverride`, `failFast`) in `src/cli/flows.go`.
- Constants use PascalCase with semantic prefixes (e.g., `FlagHost`, `cliVersion`) in `src/cli/root.go`.

**Types:**
- Exported structs use PascalCase (e.g., `DeployResult`, `AuthManager`) in `src/cli/flows.go` and `src/cli/auth.go`.
- Internal structs use lowerCamelCase (e.g., `authConfig`) in `src/cli/auth.go`.

## Code Style

**Formatting:**
- Use gofmt defaults (tabs, aligned imports) as seen in `src/cli/flows.go` and `src/cli/executions.go`.

**Linting:**
- Not detected (no `.golangci.*` config in repo root `.`).

## Import Organization

**Order:**
1. Standard library imports first in `src/cli/flows.go` and `src/cli/executions.go`.
2. Blank line separator between stdlib and external imports in `src/cli/flows.go`.
3. Third-party imports (e.g., `github.com/spf13/cobra`) in `src/cli/flows.go`.

**Path Aliases:**
- Use explicit aliases when SDK package names are verbose (e.g., `kestra` alias) in `src/cli/flows.go` and `src/cli/client.go`.

## Error Handling

**Patterns:**
- Return early with `errors.New(...)` for validation errors in `src/cli/config.go` and `src/cli/executions.go`.
- Wrap with context using `fmt.Errorf("...: %w", err)` in `src/cli/flows.go` and `src/cli/auth.go`.
- Use `errors.As` to detect typed errors in `src/cli/root.go`.

## Logging

**Framework:**
- Use `fmt.Println`/`fmt.Printf` for user-facing output in `src/cli/executions.go` and `src/cli/config.go`.
- Use tabular output via `tabwriter` in `src/cli/helpers.go` and `src/cli/namespaces.go`.

**Patterns:**
- Table headers are uppercase and tab-separated in `src/cli/flows.go` and `src/cli/namespaces.go`.

## Comments

**When to Comment:**
- Add doc comments for exported types and key functions in `src/cli/flows.go` and `src/cli/client.go`.
- Use inline comments to clarify non-obvious steps (e.g., configuration precedence, SDK quirks) in `src/cli/root.go` and `src/cli/executions.go`.

**JSDoc/TSDoc:**
- Not applicable; use Go doc comments instead in `src/cli/auth.go` and `src/cli/client.go`.

## Function Design

**Size:**
- Keep Cobra command builders thin and delegate logic to `run*` helpers per `CONTRIBUTING.md` and `src/cli/flows.go`.

**Parameters:**
- `run*` functions accept a `*Client` plus input args/flags (e.g., `runFlowsList(client, namespace)`) in `src/cli/flows.go`.

**Return Values:**
- Functions return `error` and use `nil` to indicate success in `src/cli/flows.go` and `src/cli/namespaces.go`.

## Module Design

**Exports:**
- Package entrypoints are exported (`Execute`, `NewRootCommand`, `NewClient`) in `src/cli/root.go` and `src/cli/client.go`.

**Barrel Files:**
- Not applicable; Go packages use direct imports across files in `src/cli/*.go`.

---

*Convention analysis: 2026-01-30*
