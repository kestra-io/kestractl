# Codebase Concerns

**Analysis Date:** 2026-01-30

## Tech Debt

**SDK Workarounds in Execution Paths:**
- Issue: Execution APIs rely on parsing error bodies as success due to SDK type mismatch; this is a stopgap that can mask real errors.
- Files: `src/cli/client.go`, `src/cli/executions.go`
- Impact: Misclassified failures can lead to incorrect CLI output and follow-on automation issues.
- Fix approach: Remove `tryParseExecutionFromError` once SDK models align; add explicit response validation and tests around error handling.

**Monolithic Flows Command File:**
- Issue: Single file contains list/get/deploy/validate logic, parsing helpers, and formatting utilities.
- Files: `src/cli/flows.go`
- Impact: Harder to reason about changes and increases regression risk in unrelated flows features.
- Fix approach: Split into focused files (list/get/deploy/validate/formatting) and keep shared helpers in a dedicated module.

**Silent Recovery from Invalid Config YAML:**
- Issue: Invalid config file is swallowed and replaced with empty configuration, hiding corruption.
- Files: `src/cli/auth.go`
- Impact: Users can lose contexts or silently operate with defaults, making auth failures hard to diagnose.
- Fix approach: Surface a warning/error when YAML is invalid and require an explicit reset path.

## Known Bugs

**Namespace/Flow Filters Not Implemented:**
- Symptoms: `kestra executions kill-running --namespace ...` or `--flow-id ...` returns an error instead of filtering.
- Files: `src/cli/executions.go`
- Trigger: Use `--namespace` or `--flow-id` flags.
- Workaround: None; only unfiltered kill works.

**Namespaces List Limited to First Page:**
- Symptoms: `kestra namespaces list` omits results beyond the first 100 namespaces.
- Files: `src/cli/namespaces.go`
- Trigger: Instances with more than 100 namespaces.
- Workaround: None; requires pagination support.

## Security Considerations

**Hardcoded Access Token in Debug Script:**
- Risk: Token exposure in repo or logs if the script is committed or shared.
- Files: `scripts/debug_update.go`
- Current mitigation: None; token is literal in source.
- Recommendations: Remove token from source, load from env, and add to `.gitignore` if kept locally.

**Verbose HTTP Debug Logging Can Leak Secrets:**
- Risk: SDK debug logging can print request headers and payloads containing tokens or passwords.
- Files: `src/cli/root.go`, `src/cli/client.go`
- Current mitigation: Token/password redaction only in flag echo; SDK debug logging not redacted.
- Recommendations: Add explicit redaction or warn to avoid `--verbose` in shared terminals/CI.

## Performance Bottlenecks

**Large Flow Validations Load Entire Directory into Memory:**
- Problem: `runFlowsValidate` reads every file and concatenates into a single request body.
- Files: `src/cli/flows.go`
- Cause: `contents` slice + `strings.Join` for all files.
- Improvement path: Stream file contents or validate in batches to reduce memory spikes.

**Sequential Deployment for Large Directories:**
- Problem: Deploys are strictly sequential with one API call per file.
- Files: `src/cli/flows.go`
- Cause: `runFlowsDeploy` iterates file list in order without concurrency.
- Improvement path: Add controlled parallelism with rate limits and ordered output aggregation.

## Fragile Areas

**YAML Rewrites Drop Structure/Comments:**
- Files: `src/cli/flows.go`
- Why fragile: `replaceNamespaceInYAML` parses into a map and marshals back, losing comments, anchors, and ordering.
- Safe modification: Use a YAML node-based edit to preserve structure when modifying namespace.
- Test coverage: Limited to namespace replacement without structure preservation.

**Multi-Document YAML Handling:**
- Files: `src/cli/flows.go`
- Why fragile: `parseFlowYAML` uses a single-document unmarshal and can mis-handle multi-doc files; deploy then fails before API call.
- Safe modification: Detect multi-document YAML and either support it or provide a clear error.
- Test coverage: No tests for multi-document YAML inputs.

## Scaling Limits

**Namespace Pagination Cap:**
- Current capacity: 100 namespaces per call.
- Limit: Results truncate beyond page 1.
- Scaling path: Implement pagination loop with `Page`/`Size` in `src/cli/namespaces.go`.

## Dependencies at Risk

**Kestra Go SDK Type Mismatch Issues:**
- Risk: API responses sometimes fail to deserialize into SDK models.
- Impact: CLI relies on error-body parsing to proceed.
- Migration plan: Track SDK fixes and remove `tryParseExecutionFromError` in `src/cli/client.go` when models stabilize.

## Missing Critical Features

**Config Validation and Corruption Recovery:**
- Problem: Invalid YAML config is silently replaced with empty contexts.
- Blocks: Reliable auth context management for users who upgrade or manually edit config.

## Test Coverage Gaps

**Uncovered Auth and Client Paths:**
- What's not tested: Config resolution precedence, error formatting, and auth persistence/permissions.
- Files: `src/cli/client.go`, `src/cli/auth.go`, `src/cli/root.go`
- Risk: Regressions in authentication and error messaging go unnoticed.
- Priority: High

**API Result Handling and Pagination:**
- What's not tested: Pagination behavior and SDK error fallbacks.
- Files: `src/cli/namespaces.go`, `src/cli/executions.go`, `src/cli/client.go`
- Risk: Data truncation or silent failures in production usage.
- Priority: Medium

---

*Concerns audit: 2026-01-30*
