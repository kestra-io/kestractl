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

**OSS vs EE:** some features only exist in Kestra **EE** (Enterprise Edition), not OSS — tenants, RBAC/roles, SSO, and anything under `kestra.security.*` config. Before filing a bug or reviewing a fix for one of these, confirm which edition the test instance is running.

**Known EE gotcha:** a superadmin configured with only `kestra.security.super-admin.username`/`password` never gets a tenant or `ADMIN` role bound, so every API call 403s even though login succeeds. Fix is setting `kestra.security.super-admin.tenant-admin-access: [<tenant-id>]` (e.g. `[main]`) too, which triggers tenant auto-creation and binds the built-in `ADMIN` role on startup. Check for this config before assuming a kestractl auth/permission bug.

## Branching & Releases

Two long-lived branches, two release lines:

- **`main`** — kestractl v2, targeting Kestra 2.x (Go SDK `go-sdk/v2`). Tags: `v2.x.y`.
- **`releases/v1`** — legacy v1 maintenance, targeting Kestra 1.x (Go SDK v1). Tags: `v1.x.y`. **v1 bugfixes branch from and PR into `releases/v1`, not `main`.**

Releases are created by pushing a Git tag from the matching branch, which runs `.github/workflows/release.yml` (GoReleaser). When releasing, be sure that Go SDK version in `go.mod` is a fixed and short one. For example, `github.com/kestra-io/client-sdk/go-sdk v1.1.0` is valid for a release but `github.com/kestra-io/client-sdk/go-sdk v1.1.1-0.20260702143038-8c3851bea2e1` is not valid for a release. `.github/scripts/check-sdk-pin.sh` enforces this and blocks the automated release path.

### `main` releases itself

`main` is auto-released. `.github/workflows/auto-tag.yml` runs after a **green `Tests` run on `main`** (via `workflow_run`, so it reuses that run rather than re-running the e2e matrix), computes the next tag with `.github/scripts/next-version.sh`, pushes it, and calls `release.yml`. `releases/v1` is untouched by this: its tags stay hand-cut.

Version policy, derived from the conventional-commit subjects since the last tag (highest bump wins):

| commit                                                    | bump  |
| --------------------------------------------------------- | ----- |
| `<type>!:`, or a `BREAKING CHANGE:` footer                 | major |
| `feat:`                                                   | minor |
| `fix:` `perf:` `build:` `refactor:` `chore(deps):`        | patch |
| `docs:` `test:` `ci:` `style:` bare `chore:`              | none  |

Two positional rules keep prose from moving the version, because AGENTS.md documents these markers verbatim and that invites pasting them into a commit body:

- `BREAKING CHANGE:` counts only as a **footer** — the last paragraph of the message. Quoted mid-body it is ignored.
- `[skip release]` counts only on the **head commit**, in its subject or alone on its own line. It suppresses that merge only: skipping pushes no tag, so the base tag does not advance, and a range-wide match would latch the release line off permanently after one marked merge.

**While `main` is pre-GA the semver triple is frozen and only the rc counter moves.** The base tag is `v2.0.0-rcN`, so any release-worthy merge yields `v2.0.0-rc.(N+1)` — a `feat:` does *not* jump to `v2.1.0`, and a breaking change does *not* jump to `v3.0.0`. Cutting `v2.0.0` GA is a deliberate manual tag push; after that the table above applies literally.

Three traps are worth knowing before touching any of this:

- **The rc counter must keep its dot.** `-rcN` is a single alphanumeric semver identifier compared as a string, so `rc10` sorts *below* `rc9`; `-rc.N` is a numeric identifier and increments without bound. The legacy `v2.0.0-rc1`/`rc2` tags use the undotted form, and `next-version.sh` normalises to dotted on the next bump. Note `v2.0.0-rc.3` therefore sorts *below* those two legacy tags under strict semver — harmless here, because nothing resolves these tags by semver (`install.sh` reads `/releases` in creation order, the update notifier reads `/releases/latest`, which excludes prereleases). Verify claims like this with a real semver implementation: GNU `sort -V` is not semver-compliant and gets this case backwards.
- **Never find the base tag with `git tag --sort=-v:refname`.** Git's version sort mis-orders prerelease suffixes unless `versionsort.suffix` is configured. `next-version.sh` uses `git describe --tags --abbrev=0`, i.e. nearest by commit topology.
- **A tag pushed with the default `GITHUB_TOKEN` does not fire `on: push: tags`.** GitHub suppresses workflow events originating from `GITHUB_TOKEN`. That is why `release.yml` also has a `workflow_call` trigger and `auto-tag.yml` invokes it directly, rather than the repo carrying a PAT or GitHub App token with write access.

Each auto-tagged rc is what a default `curl … | bash` install resolves to, because `install.sh`'s `VERSION=2` default picks the newest release of the major line, **prereleases included**. That is why the release gate is the full `Tests` suite (unit, installer smoke, e2e matrix) and not a fast subset.

**GitHub's "latest" is a badge, not the install default.** GitHub has one repo-wide "latest" release, and `.goreleaser.yml` pins `release.make_latest` per branch: `"true"` on `releases/v1`, `"false"` on `main` while 2.0 is pre-GA. Since `install.sh` resolves the newest release of a major line itself (`DEFAULT_MAJOR=2`, with `VERSION=1` for the legacy line), that pointer no longer decides what a default install gets — it drives the GitHub UI badge, and the in-CLI update notifier, which reads `/releases/latest`. Flip `main` to `"true"` (and `releases/v1` to `"false"`) when 2.0 goes GA, deliberately and not as a side effect of another change.
