# External Integrations

**Analysis Date:** 2026-01-30

## APIs & External Services

**Kestra API:**
- Kestra - Flow/execution/namespace operations via SDK calls in `src/cli/flows.go`, `src/cli/executions.go`, `src/cli/namespaces.go`
  - SDK/Client: `github.com/kestra-io/client-sdk/go-sdk` in `go.mod`, configured in `src/cli/client.go`
  - Auth: `KESTRA_TOKEN` or `KESTRA_USERNAME`/`KESTRA_PASSWORD` env vars in `src/cli/root.go`; persisted contexts in `src/cli/auth.go`

## Data Storage

**Databases:**
- Not detected (no database clients in `go.mod`)

**File Storage:**
- Local filesystem only for CLI config at `~/.kestra/config.yaml` in `src/cli/auth.go`

**Caching:**
- None detected (no cache libraries in `go.mod`)

## Authentication & Identity

**Auth Provider:**
- Kestra API auth (token or basic auth) implemented in `src/cli/client.go` and configured via `src/cli/root.go`

## Monitoring & Observability

**Error Tracking:**
- None detected (no error tracking SDKs in `go.mod`)

**Logs:**
- Stdout/stderr output via `fmt` and `tabwriter` in `src/cli/helpers.go`

## CI/CD & Deployment

**Hosting:**
- Not applicable (CLI binary built locally) per `README.md`

**CI Pipeline:**
- GitHub Actions running `go test ./...` in `.github/workflows/tests.yml`

## Environment Configuration

**Required env vars:**
- `KESTRA_HOST`, `KESTRA_TENANT`, and `KESTRA_TOKEN` (or `KESTRA_USERNAME` + `KESTRA_PASSWORD`) in `src/cli/root.go`

**Secrets location:**
- Stored in `~/.kestra/config.yaml` with 0600 permissions in `src/cli/auth.go`

## Webhooks & Callbacks

**Incoming:**
- None detected (no webhook handlers in `src/cli/*.go`)

**Outgoing:**
- Kestra API requests via SDK client in `src/cli/client.go`

---

*Integration audit: 2026-01-30*
