# AGENTS

This file guides automated agents working in this repo.

## References
- README.md
- CONTRIBUTING.md
- e2e_tests/README.md
- run-e2e-tests.sh

## Workspace notes
- Go module: `github.com/kestra-io/kestractl`
- Go version: `1.25` (see `go.mod`)
- No Cursor or Copilot rule files found in `.cursor/rules/`, `.cursorrules`, or `.github/copilot-instructions.md`.

## Build, lint, test
- Download deps: `go mod download`
- Build CLI (all packages): `go build ./...`
- Build binary: `go build -o kestractl`
- Run unit tests (CLI packages): `go test ./src/...`
- Run all Go tests: `go test ./...`
- Verbose tests: `go test -v ./src/cli/...`
- Single test by name: `go test -v ./src/cli/... -run TestName`
- Single package tests: `go test ./src/cli -run TestName`
- Format (Go standard): `gofmt -w .`
- Lint: no repo-specific linter configured; use `go vet ./...` if needed.

## E2E tests
- See `e2e_tests/README.md` for details.
- Requires a running Kestra EE instance with `e2e_tests/docker-setup`.
- Run from repo root: `sh -c run-e2e-tests.sh`
- Run from `e2e_tests/`: `go test ./...`
- The script reads `COMPATIBLE_KESTRA_VERSION.properties` when no arg is provided.

## Project layout
- `main.go`: entrypoint, calls CLI execute.
- `src/cli/`: Cobra commands, client wrapper, render helpers, tests.
- `e2e_tests/`: separate Go module for end-to-end tests.
- `install-scripts/`: install helper script used in README.

## Command architecture
- Cobra commands handle flags/args/usage text.
- Business logic lives in `run*` functions for testability.
- Client creation goes through `NewClient()` which resolves config.
- Output supports `table` and `json` formats; normalize output early.

## Code style
### Formatting
- Use `gofmt` for all Go files; do not hand-align with spaces.
- Keep line length reasonable but follow Go idioms over wrapping.

### Imports
- Let `gofmt` sort and group imports.
- Standard library imports first, then third-party, then local module.
- Avoid blank lines unless separating groups per `gofmt`.

### Naming
- Use Go idioms: `camelCase` for locals, `PascalCase` for exported.
- Command constructors: `newXCommand()` returning `*cobra.Command`.
- Business logic helpers: `runX(...)`.
- Tests: `TestXxx` with descriptive suffix.

### Types and interfaces
- Prefer concrete types; use `interface{}`/`any` only when needed.
- For output rendering, use `Renderer` helpers where possible.
- Keep structs small and focused; avoid global state except config flags.

### Error handling
- Return errors instead of panicking.
- Wrap errors with context using `fmt.Errorf("...: %w", err)`.
- Error messages: lowercase, no trailing period.
- Use `formatSDKError` for SDK/API error normalization.

### Flags and config
- Global flags are defined in `src/cli/root.go` and bound to Viper.
- Config precedence: flags > env vars > config file > defaults.
- Config file default: `~/.kestractl/config.yaml`.
- Environment variables are prefixed with `KESTRACTL_`.

### Output
- Respect `globalFlags.Output` (table/json).
- For table output, write headers in uppercase.
- Use `tabwriter` via `Renderer.Render(...)`.
- For JSON output, use indented JSON (`RenderJSON`).

### CLI help text
- Provide `Use`, `Short`, `Long`, `Example`, `Aliases`, and `Args`.
- Use `cobra.ExactArgs` or similar validation.
- Examples should be real, copy/paste-able commands.

### Telemetry
- `initializeTelemetry()` runs in `PersistentPreRunE`.
- Do not block command execution on telemetry.

## Testing conventions
- Unit tests live beside commands in `src/cli/*_test.go`.
- Focus on argument validation and pure function behavior.
- Use helper `executeCommand` patterns in tests (see existing tests).
- For integration behavior, prefer e2e tests under `e2e_tests/`.

## Adding a new command (checklist)
- Create `src/cli/{resource}.go`.
- Add `newResourceCommand()` and subcommands.
- Validate output format with `validateOutputFormat()`.
- Create client via `NewClient()`.
- Move business logic into `runResourceX(...)`.
- Support both table and JSON output.
- Register the command in `src/cli/root.go`.
- Add tests in `src/cli/{resource}_test.go`.

## Common pitfalls
- Forgetting to normalize output format before rendering.
- Writing direct SDK errors without `formatSDKError`.
- Not binding flags to Viper (breaks precedence).
- Using `os.Exit` in command handlers; prefer returning errors.

## Suggested local workflows
- Quick unit tests: `go test ./src/...`
- Single test while iterating: `go test -v ./src/cli/... -run TestX`
- Build and run: `go build -o kestractl && ./kestractl --help`
- Full e2e: `sh -c run-e2e-tests.sh`

## Files to consult for changes
- `README.md` for user-facing docs and install steps.
- `CONTRIBUTING.md` for command patterns and examples.
- `src/cli/root.go` for global flags and config handling.
- `src/cli/render.go` for output rendering helpers.
- `src/cli/client.go` for SDK client setup and error formatting.

## Security and secrets
- Never log credentials; `--verbose` masks token/password.
- Do not commit real tokens; use placeholders in docs/tests.

## Non-go files
- Shell scripts (`run-e2e-tests.sh`) should be POSIX-ish and safe.
- `docker-compose-ci.yml` is used by e2e script; keep in sync.

## When unsure
- Follow existing patterns in `src/cli/`.
- Prefer minimal changes and direct SDK calls via `Client`.
- Keep CLI behavior predictable and documented.

## Agent behavior
- Avoid touching `.git` or `.idea` files.
- Do not add new dependencies without a clear need.
- Do not run destructive git commands.

## End
- Keep this file updated when workflows or conventions change.
- Aim for clarity over exhaustiveness.
