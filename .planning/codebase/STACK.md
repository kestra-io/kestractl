# Technology Stack

**Analysis Date:** 2026-01-30

## Languages

**Primary:**
- Go 1.25 - CLI source in `main.go` and `src/cli/*.go`, version in `go.mod`

**Secondary:**
- YAML - flow definitions and config files in `src/cli/flows.go`, `src/cli/auth.go`, examples in `README.md`

## Runtime

**Environment:**
- Go toolchain (go 1.25 in `go.mod`; CI uses 1.21 in `.github/workflows/tests.yml`)

**Package Manager:**
- Go modules - `go.mod`
- Lockfile: present (`go.sum`)

## Frameworks

**Core:**
- Cobra v1.8.0 - CLI command framework in `src/cli/*.go`, version in `go.mod`
- Viper v1.21.0 - config/env/flag resolution in `src/cli/root.go`, version in `go.mod`

**Testing:**
- Go test (standard library) - tests in `src/cli/*_test.go`, CI run in `.github/workflows/tests.yml`

**Build/Dev:**
- Go toolchain (`go build`) in `README.md`

## Key Dependencies

**Critical:**
- `github.com/kestra-io/client-sdk/go-sdk` v0.0.0-20260123183957-5a64f79c211e - Kestra API client in `src/cli/client.go`, used by `src/cli/flows.go`, `src/cli/executions.go`, `src/cli/namespaces.go`
- `gopkg.in/yaml.v3` v3.0.1 - YAML parsing for flow/config data in `src/cli/flows.go`, `src/cli/auth.go`

**Infrastructure:**
- `github.com/spf13/cobra` v1.8.0 - command wiring in `src/cli/*.go`
- `github.com/spf13/viper` v1.21.0 - config/env handling in `src/cli/root.go`

## Configuration

**Environment:**
- Viper loads env vars with the `KESTRA_` prefix and flag > env > config precedence in `src/cli/root.go`
- Config file stored at `~/.kestra/config.yaml` and managed in `src/cli/auth.go` (read path in `src/cli/root.go`)

**Build:**
- Go module definitions in `go.mod` and checksums in `go.sum`

## Platform Requirements

**Development:**
- Go toolchain 1.25 per `go.mod` (README documents 1.21+ in `README.md`)
- Access to a Kestra API host and auth credentials in `README.md` and `src/cli/client.go`

**Production:**
- CLI binary built via `go build` and executed on user machines/CI to call the Kestra API from `README.md` and `src/cli/client.go`

---

*Stack analysis: 2026-01-30*
