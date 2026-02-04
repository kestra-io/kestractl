# Testing Patterns

**Analysis Date:** 2026-01-30

## Test Framework

**Runner:**
- Go standard testing (`testing` package) used in `src/cli/flows_test.go` and `src/cli/executions_test.go`.
- Config: Not detected (no test config files; CI runs `go test ./...` in `.github/workflows/tests.yml`).

**Assertion Library:**
- Standard library assertions via `testing.T` with manual checks in `src/cli/flows_test.go` and `src/cli/namespaces_test.go`.

**Run Commands:**
```bash
go test ./...              # Run all tests (documented in CONTRIBUTING.md)
go test -v ./src/cli/...    # Verbose (documented in CONTRIBUTING.md)
go test -v ./src/cli/... -run TestFlowsListCommand_NoArgs  # Run specific test (documented in CONTRIBUTING.md)
```

## Test File Organization

**Location:**
- Co-located tests alongside source in `src/cli/flows_test.go`, `src/cli/executions_test.go`, and `src/cli/namespaces_test.go`.

**Naming:**
- Use `_test.go` suffix, matching the source file base name (e.g., `src/cli/flows.go` and `src/cli/flows_test.go`).

**Structure:** (from `src/cli/flows_test.go` and `src/cli/executions_test.go`)
```
src/cli/
  flows.go
  flows_test.go
  executions.go
  executions_test.go
  namespaces.go
  namespaces_test.go
  testdata/
```

## Test Structure

**Suite Organization:** (example from `src/cli/flows_test.go`)
```go
tests := []struct {
    name        string
    yaml        string
    wantNS      string
    wantID      string
    wantErr     bool
    errContains string
}{}

for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        ns, id, err := parseFlowYAML(tt.yaml)
        // assertions...
    })
}
```

**Patterns:**
- Table-driven tests with `t.Run` subtests in `src/cli/flows_test.go` and `src/cli/executions_test.go`.
- Assert error presence and message substrings with `strings.Contains` in `src/cli/flows_test.go` and `src/cli/namespaces_test.go`.

## Mocking

**Framework:**
- No mocking framework; use function variable override for dependencies in `src/cli/client.go` and `src/cli/flows_test.go`.

**Patterns:** (from `src/cli/flows_test.go`)
```go
original := newClientFunc
newClientFunc = func() (*Client, error) {
    return &Client{Tenant: "main"}, nil
}
defer func() { newClientFunc = original }()
```

**What to Mock:**
- Override client creation to avoid config/auth requirements in `src/cli/flows_test.go` and `src/cli/executions_test.go`.

**What NOT to Mock:**
- Pure functions are tested directly (e.g., `parseFlowYAML`) in `src/cli/flows_test.go`.

## Fixtures and Factories

**Test Data:** (from `src/cli/flows_test.go`)
```go
testDir := "testdata/deploy_folder_test"
files, err := collectFlowFiles(testDir)
```

**Location:**
- Filesystem fixtures stored in `src/cli/testdata/` (e.g., `src/cli/testdata/deploy_folder_test/flow1.yaml`).

## Coverage

**Requirements:**
- None enforced (CI runs `go test ./...` without coverage in `.github/workflows/tests.yml`).

**View Coverage:**
```bash
# Not documented in repo configs; no coverage command in CONTRIBUTING.md or .github/workflows/tests.yml
```

## Test Types

**Unit Tests:**
- CLI argument validation, helpers, and pure functions in `src/cli/flows_test.go` and `src/cli/executions_test.go`.

**Integration Tests:**
- Not used (no tests invoking real SDK/API calls; client is stubbed in `src/cli/flows_test.go`).

**E2E Tests:**
- Not used (no E2E framework or directories; only unit tests in `src/cli/*_test.go`).

## Common Patterns

**Async Testing:** (from `src/cli/executions_test.go`)
```go
output, _ := captureStdout(func() error {
    printExecutionState(execution, true)
    return nil
})
```

**Error Testing:** (from `src/cli/flows_test.go`)
```go
_, err := executeCommand(cmd)
if err == nil {
    t.Fatal("expected error when no args provided")
}
```

---

*Testing analysis: 2026-01-30*
