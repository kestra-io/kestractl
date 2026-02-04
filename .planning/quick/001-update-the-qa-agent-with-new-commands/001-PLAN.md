---
phase: quick-001-update-the-qa-agent-with-new-commands
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - .opencode/agents/qa.md
autonomous: true
must_haves:
  truths:
    - "QA agent covers all README.md CLI commands or explicitly skips with guardrails"
    - "Commands that require local config writes use a temporary config path"
    - "QA output calls out skipped commands with reasons"
  artifacts:
    - path: ".opencode/agents/qa.md"
      provides: "Updated QA command list and constraints"
  key_links:
    - from: ".opencode/agents/qa.md"
      to: "README.md"
      via: "command list alignment"
      pattern: "kestra (config|flows|executions|namespaces)"
---

<objective>
Update the QA agent prompt so it covers the current README command set with safe execution guardrails.

Purpose: Keep QA runs aligned with documented CLI surface area.
Output: Updated `.opencode/agents/qa.md` command list and constraints.
</objective>

<execution_context>
@/Users/k_/.config/opencode/get-shit-done/workflows/execute-plan.md
@/Users/k_/.config/opencode/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/STATE.md
@.planning/PROJECT.md
@README.md
@.opencode/agents/qa.md
</context>

<tasks>

<task type="auto">
  <name>Task 1: Reconcile QA command coverage with README</name>
  <files>.opencode/agents/qa.md</files>
  <action>
    Review README.md command sections (Config Management, Flows, Executions, Namespaces, Output Formats).
    Update the QA workflow list to include any documented commands not currently covered, prioritizing:
    - `kestra config add`, `kestra config use`, `kestra config remove` (use a temporary config path like `/tmp/kestra-qa-config.yaml` and clean up if created).
    - Output format checks (e.g., `--output json`) for one representative list command.
    - Any new flags or aliases explicitly shown in README.md that are safe to execute.
    Keep existing flow/namespace/execution commands intact and only add new items or refine existing ones.
  </action>
  <verify>Diff `.opencode/agents/qa.md` to confirm all README sections have coverage or explicit skip notes.</verify>
  <done>QA agent command list mirrors README.md commands with safe, runnable variants.</done>
</task>

<task type="auto">
  <name>Task 2: Tighten constraints and skip logic for risky commands</name>
  <files>.opencode/agents/qa.md</files>
  <action>
    Update constraints to allow only a temporary config file for `config add/use/remove`, and require cleanup after use.
    For any command that could be destructive or depends on server state (e.g., deploy/kill), ensure the QA agent uses existing guardrails or adds explicit skip conditions with clear reporting.
    Make sure the reporting step calls out skipped commands and the reason (missing file, env var not set, or guardrail).
  </action>
  <verify>QA prompt includes explicit temp-config guidance and skip/report language for risky commands.</verify>
  <done>QA agent has clear, enforceable guardrails for new commands without modifying user config.</done>
</task>

</tasks>

<verification>
Ensure `.opencode/agents/qa.md` includes all README command categories and notes any intentional skips.
</verification>

<success_criteria>
- QA agent workflow covers config, flows, executions, namespaces, and output format commands.
- Any commands requiring local config writes use `/tmp/kestra-qa-config.yaml` and are cleaned up.
- Skipped commands are explicitly reported with reasons in the QA output.
</success_criteria>

<output>
After completion, create `.planning/quick/001-update-the-qa-agent-with-new-commands/001-SUMMARY.md`
</output>
