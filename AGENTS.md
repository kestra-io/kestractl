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

`COMPATIBLE_KESTRA_VERSION.properties` has two consumers, so it is the single source of truth for "which Kestra versions does this support":

- `.github/workflows/list-versions.yml` turns it into the e2e matrix.
- `.github/scripts/render-compat-badges.sh` renders the README's compatibility badges between the `<!-- compat-badges:start -->` / `end` markers. Numeric lines are grouped into one badge, newest first; anything else (`develop`) gets its own dimmed "tracked" badge. Run it after editing the properties file — `--check` runs in the `Tests` workflow and fails with the diff if the README drifted.

Keep the properties file in **ascending** order: the badge script renders newest-first by reversing it rather than version-sorting, deliberately, because no shell sort is semver-correct on prerelease suffixes (see the release traps above).

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

## Dependencies & Security Scanning

Five automated pieces, modelled on `kestra-io/kestra`'s setup and adapted to Go:

- **`.github/dependabot.yml`** — weekly updates (Wednesday 08:00 Europe/Paris, matching the other repos) for GitHub Actions, the root Go module, the `e2e_tests` Go module, and the Docker base images.
- **`dependabot-security-prefix.yml`** — retitles a Dependabot *security* PR to `fix(deps)` so it releases, and labels it `kind/security`.
- **`codeql-analysis.yml`** — CodeQL for `go` with the `security-and-quality` query pack, on PRs to `main` and weekly.
- **`vulnerabilities-check.yml`** — `govulncheck` on both Go modules (the Go analogue of kestra's OWASP `dependencyCheckAggregate`; it reports only vulnerabilities reachable from our code, so it needs no NVD API key), plus a daily Trivy scan of the published `kestra/kestractl:latest` and `:latest-static` images.
- **`dependency-submission.yml`** — submits the resolved `go mod graph` to GitHub's dependency graph on every push to `main`. Without it, Dependabot alerts only see `go.mod`'s direct requirements, so an advisory against a transitive module never fires.

Four things to know before touching this:

- **A security bump releases; a routine bump does not.** `main` auto-releases off the squashed merge subject, which GitHub takes from the PR title. Every ecosystem in `dependabot.yml` therefore uses `ci(deps)`, which scores no bump — a routine version update is not urgent and rides along with the next real change. Security updates are the exception, and Dependabot has one `commit-message.prefix` per ecosystem with no way to vary it by update type, so `dependabot-security-prefix.yml` promotes those to `fix(deps)` (→ patch) off the advisory metadata. The two files are coupled: change a prefix in `dependabot.yml` and the promotion step fails loudly on the next security PR rather than silently scoring it `none`.
- **That promotion workflow runs on `pull_request_target` and must never check out the PR.** A `pull_request` token is read-only on a Dependabot PR, so it cannot retitle; `pull_request_target` gets a writable token, which is only safe because the job reads advisory metadata and calls the API, never the branch's code.
- **Neither scanner is a job in `Tests`, deliberately.** `auto-tag.yml` releases on a green `Tests` run, so a CVE published against an already-released dependency, or a newly shipped CodeQL rule, would otherwise freeze the release line on work unrelated to the merge.
- **`govulncheck` runs on `stable` Go, not `go.mod`'s version.** govulncheck v1.8+ needs Go >= 1.26 to build at all. That means it scans the standard library of the current toolchain while releases are still built with the `go-version: "1.25"` pins in `tests.yml` and `release.yml` — bump those together, or the scan and the shipped binary disagree about which stdlib CVEs apply.

Already enabled repo-side and needing no file here: secret scanning, push protection, and Dependabot security updates.

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

**`main` is GA, so the table above applies literally.** The base tag is `v2.0.0`, so a `feat:` yields `v2.1.0` and a breaking change `v3.0.0`. The rc line is closed: `v2.0.0` was the last deliberate manual tag push.

`next-version.sh` still carries its prerelease branch — on a `vX.Y.Z-rcN` base it freezes the semver triple and only advances the rc counter, normalising `rcN` to `rc.N`. That is dormant on a GA base and kept for the next prerelease line; do not delete it or its tests when tidying.

Three traps are worth knowing before touching any of this:

- **The rc counter must keep its dot.** `-rcN` is a single alphanumeric semver identifier compared as a string, so `rc10` sorts *below* `rc9`; `-rc.N` is a numeric identifier and increments without bound. The legacy `v2.0.0-rc1`/`rc2` tags use the undotted form, and `next-version.sh` normalises to dotted on the next bump. Note `v2.0.0-rc.3` therefore sorts *below* those two legacy tags under strict semver — harmless here, because nothing resolves these tags by semver (`install.sh` reads `/releases` in creation order, the update notifier reads `/releases/latest`, which excludes prereleases). Verify claims like this with a real semver implementation: GNU `sort -V` is not semver-compliant and gets this case backwards.
- **Never find the base tag with `git tag --sort=-v:refname`.** Git's version sort mis-orders prerelease suffixes unless `versionsort.suffix` is configured. `next-version.sh` uses `git describe --tags --abbrev=0`, i.e. nearest by commit topology.
- **A tag pushed with the default `GITHUB_TOKEN` does not fire `on: push: tags`.** GitHub suppresses workflow events originating from `GITHUB_TOKEN`. That is why `release.yml` also has a `workflow_call` trigger and `auto-tag.yml` invokes it directly, rather than the repo carrying a PAT or GitHub App token with write access.

Each auto-tagged release is what a default `curl … | bash` install resolves to, because `install.sh`'s `VERSION=2` default picks the newest release of the major line **in creation order, prereleases included**. So publishing a prerelease on the v2 line would hand it to every default install — a reason not to reopen an rc line on `main` casually. Either way, that is why the release gate is the full `Tests` suite (unit, installer smoke, e2e matrix) and not a fast subset.

**GitHub's "latest" is a badge, not the install default.** GitHub has one repo-wide "latest" release, and `.goreleaser.yml` sets `release.make_latest: "true"` on `main` now that 2.0 is GA. `releases/v1` still carries `"true"` in its own config, which is harmless: the flag only takes effect when GoReleaser publishes a release, and that branch is legacy and not released any more. Since `install.sh` resolves the newest release of a major line itself (`DEFAULT_MAJOR=2`, with `VERSION=1` for the legacy line), that pointer does not decide what a default install gets — it drives the GitHub UI badge, and the in-CLI update notifier, which reads `/releases/latest`. One consequence of the flip is deliberate: `/releases/latest` is now a 2.x release, so **v1 users are notified about a major upgrade**. Note GitHub cannot mark a draft or prerelease as latest, so `make_latest: "true"` is inert on any prerelease `main` might publish.

### Container images

Every release also publishes a multi-arch (`linux/amd64` + `linux/arm64`) image, built by GoReleaser from the same tag (`dockers_v2` in `.goreleaser.yml`) and pushed to both `kestra/kestractl` and `ghcr.io/kestra-io/kestractl`, in two variants: Alpine (default tag) and distroless (`-static`).

Five things to know before touching this:

- **The rolling tags are copies, not rebuilds.** GoReleaser publishes only the exact-version manifest; `release.yml` then uses `regctl image copy` to point `X.Y`, `X` and `latest` at that same manifest. Rebuilding them would produce different digests per tag.
- **A prerelease moves the major-line tag but nothing else.** Should `main` ever publish an rc again, `:2` follows it — mirroring `install.sh`'s `VERSION=2` default, which also resolves the newest release of a major line, prereleases included — while `:X.Y` and `:latest` stay on the newest stable release. That keeps a prerelease from becoming what an untagged `docker pull` hands out.
- **`packages: write` has to be granted by the caller.** A reusable workflow inherits the caller's `GITHUB_TOKEN` and can only narrow it, so the grant in `release.yml` does nothing on the `auto-tag.yml` path unless `auto-tag.yml`'s `release` job grants it too. Getting this wrong fails only at the GHCR push, only on a real release.
- **A release that failed partway through is retried by hand, not re-tagged.** Actions -> Release -> Run workflow, with `tag` set to the tag that failed (leave it empty for a dry run). Every publish step is idempotent — buildx re-pushes the same digests, `createOrUpdateRelease` updates the existing release, `replace_existing_artifacts: true` lets the assets be re-uploaded, and `regctl image copy` re-points to the same manifest — so re-running converges instead of double-publishing. Do **not** reach for "Re-run all jobs" on Auto Tag instead: that re-runs the tagger, which finds the tag already present, reports `pushed=false`, and skips the release entirely.
- **But the retry only helps when the fix is outside the repo.** `workflow_dispatch` runs the workflow definition from the ref you dispatch on, while checking out the *tag's* tree — so `.goreleaser.yml`, the Dockerfiles and anything else the release reads come from the tag, frozen as of when it was cut. Retry when the cause is external state: registry credentials, a repository that had to be created, an outage, a rate limit. When the cause is in the config, the tag can never produce a good release no matter how many times you re-run it. Land the fix on `main` and let it cut a new tag; the broken tag stays behind with no release attached, which is untidy but harmless (nothing resolves tags — `install.sh` reads `/releases`, the update notifier reads `/releases/latest`). Delete it or leave it.

  This is worth internalising because the two cases look identical in the logs and the wrong choice wastes a release either way. A worked example, from the first time the images were published: `v2.1.0` failed on `access token has insufficient scopes` pushing to Docker Hub. That is external — the token lacked push rights and `kestra/kestractl` did not exist — so once both were fixed, re-running Release with `tag=v2.1.0` recovered it completely. Had the plan instead been to *disable* Docker Hub in `.goreleaser.yml`, retrying that same tag would have failed identically forever, because the tag's tree still had it enabled.

`DOCKER_LATEST` in `release.yml` decides whether this branch's releases claim the rolling `:latest` Docker tag, i.e. what an untagged `docker pull kestra/kestractl` hands out. It is `"true"` on `main` for the same reason `release.make_latest` is: `main` is the only released line and 2.0 is GA. Unlike GitHub's pointer this one is not inert on a prerelease, which is why the rc rule above is enforced in `release.yml` rather than left to the registry.
