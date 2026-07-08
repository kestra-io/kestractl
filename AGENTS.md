# AGENTS.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Build
go build -o kestractl

# Unit tests (fast, no external deps)
go test ./src/...
go test -v ./src/cli/... -run TestName

# E2E tests (requires Docker + running Kestra EE instance)
sh run-e2e-tests.sh [version]
```

## Architecture

`main.go` → `src/cli/root.go` (Cobra root) → individual command files → `client.go` → Kestra Go SDK → Kestra API.

All source lives in `src/cli/`. Each resource domain has `<resource>.go` and `<resource>_test.go`.

**Config resolution (highest → lowest priority):** CLI flags → `KESTRACTL_*` env vars → `~/.kestractl/config.yaml` → defaults. Viper handles this in `root.go`.

**Command pattern:** `newXCommand()` wires Cobra boilerplate and calls `runX(...)` — a pure function containing all business logic. Tests target `runX()` directly, not the command.

**Client:** `NewClient()` in `client.go` resolves the active auth context and wraps the Kestra SDK's `APIClient`. `formatSDKError()` extracts meaningful messages from `GenericOpenAPIError`. Some SDK endpoints return type mismatches on success — `tryParseXFromError()` helpers work around this by parsing raw JSON from "error" responses.

**Output:** `render.go` provides a `Renderer` that routes to table (`tabwriter`) or JSON based on `--output`. All commands support both. Table headers are UPPERCASE.

**Auth contexts:** Stored in `~/.kestractl/config.yaml` (permissions `0o600`). `auth.go` manages read/write. Multiple named contexts supported; `config use <name>` switches the active one.

**Telemetry:** `telemetry.go` fires a PostHog event per command in `PersistentPreRunE`. It does not block execution. Disable with `KESTRACTL_TELEMETRY_DISABLED=true`.

## Adding a Command

See `CONTRIBUTING.md` for the full checklist. Key points:
1. Add `newXCommand()` + `runX()` in a new `<resource>.go` file
2. Call `validateOutputFormat()` early; support both table and JSON output
3. Register in `root.go`
4. Test pure `runX()` logic in `<resource>_test.go`

**Common pitfalls:**
- Forgetting `validateOutputFormat()` before rendering
- Writing raw SDK errors without `formatSDKError`
- Not binding flags to Viper (breaks config precedence)
- Using `os.Exit` in command handlers — return errors instead

**Security:** Never log credentials. `--verbose` must mask token/password values.

## E2E Tests

`e2e_tests/` is a **separate Go module**. It builds the `kestractl` binary, then uses `os/exec` to run real CLI commands against a live Kestra EE instance. Compatible versions are listed in `COMPATIBLE_KESTRA_VERSION.properties`. Docker setup lives in `e2e_tests/docker-setup/`.

## Code Review & QA

In addition to `Common pitfalls` above, check for:
- **Backward compatibility:** flags, output fields, and SDK method signatures are additive; a breaking rename needs an explicit call-out, not a silent change
- **Command injection:** no `os/exec` call built from unsanitized user input
- **SSRF:** outbound requests to a user-supplied host/URL (`--server`, webhook-style flags) are validated, not blindly followed
- **Resource leaks:** HTTP clients and file handles are closed; no goroutine started without a way to stop it
- **Idempotency:** destructive commands (`delete`, `reset`) require explicit confirmation or a `--force`/`-y` flag
- **SDK version pinning:** see `Release` below — a floating/`-SNAPSHOT`/`develop`-tracking Go SDK version is a review blocker for anything targeting `main`

**A running Kestra instance is required for QA.** Beyond unit tests, exercising a command for real needs a live Kestra instance — spin one up locally (see `e2e_tests/docker-setup/`) or point `--server`/config at one. Don't conclude a command is broken from unit tests alone.

**CE vs EE:** some features only exist in Kestra **EE** (Enterprise Edition), not CE — tenants, RBAC/roles, SSO, and anything under `kestra.security.*` config. Before filing a bug or reviewing a fix for one of these, confirm which edition the test instance is running.

**Known EE gotcha:** a superadmin configured with only `kestra.security.super-admin.username`/`password` never gets a tenant or `ADMIN` role bound, so every API call 403s even though login succeeds. Fix is setting `kestra.security.super-admin.tenant-admin-access: [<tenant-id>]` (e.g. `[main]`) too, which triggers tenant auto-creation and binds the built-in `ADMIN` role on startup. Check for this config before assuming a kestractl auth/permission bug.

## Release

Releases are created by creating a new Git tag. See `.github/workflows/release.yml` for more details. When releasing from `main` branch, be sure that Go SDK version in `go.mod` is a fixed and short one. For example, `github.com/kestra-io/client-sdk/go-sdk v1.1.0` is valid for a release but `github.com/kestra-io/client-sdk/go-sdk v1.1.1-0.20260702143038-8c3851bea2e1` is not valid for a release.
