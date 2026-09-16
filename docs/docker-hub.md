<!--
  Source text for the Docker Hub *Overview* of kestra/kestractl.

  Nothing syncs this file to Docker Hub. After editing it, paste the whole file
  into the Overview field at https://hub.docker.com/repository/docker/kestra/kestractl/general
  (repository admin rights required). Keep every link absolute -- relative paths
  404 there -- and the file under 25,000 characters, Docker Hub's cap.

  This comment does not render, so it is safe to paste along with the rest.
-->

# kestractl

The official command-line interface for [Kestra](https://kestra.io) — manage flows,
executions, triggers, namespaces, namespace files, the key-value store, apps,
dashboards, test suites and IAM (users, groups, roles, service accounts, bindings,
invitations) from a terminal or a CI job.

Source, full command reference and issue tracker:
**[github.com/kestra-io/kestractl](https://github.com/kestra-io/kestractl)**

```bash
docker run --rm kestra/kestractl:latest --help
```

## Supported tags

Every release publishes a multi-arch manifest (`linux/amd64` + `linux/arm64`) in two
variants:

| Tag | Contents |
| --- | --- |
| `latest`, `X`, `X.Y`, `X.Y.Z` | Alpine based, with `bash`, `git` and `curl` — use this in CI jobs that run scripts |
| `latest-static`, `X-static`, `X.Y-static`, `X.Y.Z-static` | distroless, no shell — smallest surface, runs `kestractl` only |

`latest`, `X` and `X.Y` follow the newest stable release, so `:3` tracks the 3.x line and
`:3.1` the 3.1 patches. Pin the full `X.Y.Z` in CI if you want a release to stay put; a
prerelease, should one be published, only ever moves `X`.

The full list of published tags is on the **Tags** tab above.

The same images are published to GitHub Container Registry as
`ghcr.io/kestra-io/kestractl`, with identical tags and digests.

## Kestra compatibility

The current major targets Kestra 2.x and works against Kestra 1.3 for everyday commands
(flows, executions, namespace files, KV, namespaces); features that only exist in
Kestra 2.0 are refused with a clear error on a 1.x server. Every supported version is
exercised by the end-to-end test matrix on each run — the authoritative list lives in
[`COMPATIBLE_KESTRA_VERSION.properties`](https://github.com/kestra-io/kestractl/blob/main/COMPATIBLE_KESTRA_VERSION.properties).

For the full Kestra 1.x feature set (namespace plugin defaults, superadmin management),
use the `1` tag line instead: `kestra/kestractl:1`.

## Configuration

Either pass `KESTRACTL_*` environment variables:

```bash
docker run --rm \
  -e KESTRACTL_HOST=https://kestra.example.com \
  -e KESTRACTL_TENANT=main \
  -e KESTRACTL_USERNAME=admin@example.com \
  -e KESTRACTL_PASSWORD='...' \
  kestra/kestractl:latest flows list
```

or mount a config file. The images run as a non-root user, so mount it at that user's
home: `/home/kestractl/.kestractl` (Alpine) or `/home/nonroot/.kestractl` (`-static`).

```bash
docker run --rm \
  -v ~/.kestractl:/home/kestractl/.kestractl:ro \
  -v "$PWD/flows:/flows:ro" \
  kestra/kestractl:latest flows deploy /flows
```

Settings resolve highest-to-lowest as command-line flags → `KESTRACTL_*` environment
variables → `~/.kestractl/config.yaml` → defaults, so a mounted config can be overridden
per job without editing it.

Common variables: `KESTRACTL_HOST`, `KESTRACTL_TENANT`, `KESTRACTL_TOKEN`,
`KESTRACTL_USERNAME`, `KESTRACTL_PASSWORD`, `KESTRACTL_OUTPUT` (`table` or `json`),
`KESTRACTL_HEADER`.

## Deploying flows from CI

```yaml
deploy-flows:
  image: kestra/kestractl:3
  variables:
    KESTRACTL_HOST: https://kestra.example.com
    KESTRACTL_TENANT: main
  script:
    - kestractl flows validate ./flows
    - kestractl flows deploy ./flows
```

`KESTRACTL_TOKEN`, `KESTRACTL_USERNAME` and `KESTRACTL_PASSWORD` belong in your CI's
masked/protected variables — never in the job definition.

Note: GitLab CI and GitHub Actions container jobs override the image entrypoint and run
your `script` through a shell, so use the default (Alpine) tag there. The `-static` tag
has no shell and only works with `docker run` / `kubectl run` style invocations.

## Other install methods

Not every use needs a container — there is a convenience installer, plain binaries for
each OS/arch, and `go install`. See the
[README](https://github.com/kestra-io/kestractl#installation).

## Telemetry and update checks

kestractl sends anonymous usage telemetry and checks for a newer release at most once a
day; neither blocks a command. The version check is skipped automatically on CI. Disable
them with `KESTRACTL_TELEMETRY_DISABLED=true` and
`KESTRACTL_VERSION_CHECK_DISABLED=true`.

## License

Apache-2.0 — see
[github.com/kestra-io/kestractl](https://github.com/kestra-io/kestractl).
