package cli

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// nestedFlowSource exercises the walker end to end: nested flowables, the
// error/finally/afterExecution containers, trigger conditions and
// preconditions, an SLA condition and pluginDefaults.
const nestedFlowSource = `
id: nested-flow
namespace: prod.team.data
disabled: true
inputs:
  - id: target
    type: STRING
tasks:
  - id: branch
    type: io.kestra.plugin.core.flow.If
    condition: "{{ inputs.target == 'a' }}"
    then:
      - id: each
        type: io.kestra.core.tasks.flows.EachSequential
        tasks:
          - id: log
            type: io.kestra.plugin.core.log.Log
    else:
      - id: switch
        type: io.kestra.plugin.core.flow.Switch
        cases:
          a:
            - id: call
              type: io.kestra.plugin.core.flow.Subflow
  - id: loop
    type: io.kestra.plugin.core.flow.ForEachItem
    tasks:
      - id: purge
        type: io.kestra.plugin.fs.local.Delete
errors:
  - id: on-error
    type: io.kestra.plugin.core.log.Log
finally:
  - id: cleanup
    type: io.kestra.plugin.core.log.Log
afterExecution:
  - id: after
    type: io.kestra.plugin.core.log.Log
triggers:
  - id: schedule
    type: io.kestra.plugin.core.trigger.Schedule
    cron: "0 * * * *"
    conditions:
      - type: io.kestra.plugin.core.condition.DayWeekInMonth
      - type: io.kestra.plugin.core.condition.WeekendCondition
  - id: on-flow
    type: io.kestra.plugin.core.trigger.Flow
    preconditions:
      flows:
        - namespace: prod.team.data
checks:
  - id: freshness
    condition: "{{ trigger.date }}"
sla:
  - id: max-duration
    type: MAX_DURATION
    condition: "{{ execution.duration > 60 }}"
pluginDefaults:
  - type: io.kestra.plugin.core.log.Log
    forced: true
    values:
      level: INFO
  - type: io.kestra.plugin.jdbc.postgresql.Query
    values:
      url: jdbc:postgresql://db/app
`

func mustAnalyze(t *testing.T, source string) flowAnalysis {
	t.Helper()
	analysis, err := analyzeFlowSource(source)
	if err != nil {
		t.Fatalf("analyzeFlowSource returned an error: %v", err)
	}
	return analysis
}

func TestAnalyzeFlowSource_Walker(t *testing.T) {
	analysis := mustAnalyze(t, nestedFlowSource)

	if analysis.Namespace != "prod.team.data" || analysis.FlowID != "nested-flow" {
		t.Fatalf("unexpected flow identity: %q/%q", analysis.Namespace, analysis.FlowID)
	}
	if !analysis.Disabled || !analysis.HasInputs || !analysis.HasTriggers {
		t.Fatalf("unexpected flags: %+v", analysis)
	}

	wantTasks := map[string]int64{
		"io.kestra.plugin.core.flow.If":             1,
		"io.kestra.core.tasks.flows.EachSequential": 1,
		"io.kestra.plugin.core.flow.Switch":         1,
		"io.kestra.plugin.core.flow.Subflow":        1,
		"io.kestra.plugin.core.flow.ForEachItem":    1,
		"io.kestra.plugin.fs.local.Delete":          1,
		// error, finally, afterExecution and the nested EachSequential task.
		"io.kestra.plugin.core.log.Log": 4,
	}
	for typeName, want := range wantTasks {
		if got := analysis.TaskTypes[typeName]; got != want {
			t.Errorf("task type %s: got %d, want %d", typeName, got, want)
		}
	}
	if len(analysis.TaskTypes) != len(wantTasks) {
		t.Errorf("unexpected task types collected: %v", analysis.TaskTypes)
	}
	if analysis.TaskTypes["STRING"] != 0 {
		t.Error("input declarations must not be counted as tasks")
	}

	wantTriggers := map[string]int64{
		"io.kestra.plugin.core.trigger.Schedule": 1,
		"io.kestra.plugin.core.trigger.Flow":     1,
	}
	for typeName, want := range wantTriggers {
		if got := analysis.TriggerTypes[typeName]; got != want {
			t.Errorf("trigger type %s: got %d, want %d", typeName, got, want)
		}
	}
	if len(analysis.TriggerTypes) != len(wantTriggers) {
		t.Errorf("unexpected trigger types collected: %v", analysis.TriggerTypes)
	}
	if analysis.TaskTypes["io.kestra.plugin.core.condition.DayWeekInMonth"] != 0 {
		t.Error("trigger conditions must not be counted as tasks")
	}

	if analysis.RemovedTasks["EachSequential"] != 1 || analysis.RemovedTasks["ForEachItem"] != 1 {
		t.Errorf("unexpected removed tasks: %v", analysis.RemovedTasks)
	}
	if analysis.SubflowTasks != 1 {
		t.Errorf("subflow tasks: got %d, want 1", analysis.SubflowTasks)
	}
	if analysis.FsLocalDelete != 1 {
		t.Errorf("fs.local.Delete: got %d, want 1", analysis.FsLocalDelete)
	}
	// Two `conditions` entries plus one `preconditions` entry.
	if analysis.TriggerConditions != 3 {
		t.Errorf("trigger conditions: got %d, want 3", analysis.TriggerConditions)
	}
	// The check and the SLA carry a `condition`; the If task's own `condition`
	// survives in 2.0 and must not be counted.
	if analysis.ConditionProperty != 2 {
		t.Errorf("condition property: got %d, want 2", analysis.ConditionProperty)
	}
	if analysis.PluginDefaultsEntries != 2 || analysis.PluginDefaultsForced != 1 {
		t.Errorf("plugin defaults: %d entries, %d forced", analysis.PluginDefaultsEntries, analysis.PluginDefaultsForced)
	}
	if analysis.PluginDefaultsTypes["io.kestra.plugin.jdbc.postgresql.Query"] != 1 {
		t.Errorf("unexpected plugin default types: %v", analysis.PluginDefaultsTypes)
	}
}

func TestAnalyzeFlowSource_Signals(t *testing.T) {
	cases := []struct {
		name   string
		source string
		check  func(t *testing.T, a flowAnalysis)
	}{
		{
			name: "modern subflow spelling",
			source: `
id: f
namespace: ns
tasks:
  - id: call
    type: io.kestra.plugin.core.flow.Subflow
`,
			check: func(t *testing.T, a flowAnalysis) {
				if a.SubflowTasks != 1 {
					t.Errorf("subflow tasks: got %d, want 1", a.SubflowTasks)
				}
			},
		},
		{
			name: "legacy subflow spelling",
			source: `
id: f
namespace: ns
tasks:
  - id: call
    type: io.kestra.core.tasks.flows.Flow
`,
			check: func(t *testing.T, a flowAnalysis) {
				if a.SubflowTasks != 1 {
					t.Errorf("subflow tasks: got %d, want 1", a.SubflowTasks)
				}
			},
		},
		{
			name: "legacy each parallel is a removed task",
			source: `
id: f
namespace: ns
tasks:
  - id: fan
    type: io.kestra.core.tasks.flows.EachParallel
    tasks:
      - id: inner
        type: io.kestra.plugin.core.log.Log
`,
			check: func(t *testing.T, a flowAnalysis) {
				if a.RemovedTasks["EachParallel"] != 1 {
					t.Errorf("unexpected removed tasks: %v", a.RemovedTasks)
				}
				if a.TaskTypes["io.kestra.plugin.core.log.Log"] != 1 {
					t.Error("nested task under a removed flowable must still be counted")
				}
			},
		},
		{
			name: "pebble json positives",
			source: `
id: f
namespace: ns
tasks:
  - id: log
    type: io.kestra.plugin.core.log.Log
    message: "{{ json(inputs.raw) }} {{ json (inputs.other) }}"
`,
			check: func(t *testing.T, a flowAnalysis) {
				if a.PebbleJSON != 2 {
					t.Errorf("pebble json: got %d, want 2", a.PebbleJSON)
				}
			},
		},
		{
			name: "pebble json negatives",
			source: `
id: f
namespace: ns
tasks:
  - id: log
    type: io.kestra.plugin.core.log.Log
    message: "{{ fromJson(x) }} {{ toJson(y) }} {{ vars.payload.json(z) }}"
`,
			check: func(t *testing.T, a flowAnalysis) {
				if a.PebbleJSON != 0 {
					t.Errorf("pebble json: got %d, want 0", a.PebbleJSON)
				}
			},
		},
		{
			name: "input declarations are not tasks",
			source: `
id: f
namespace: ns
inputs:
  - id: name
    type: STRING
  - id: count
    type: INT
tasks:
  - id: log
    type: io.kestra.plugin.core.log.Log
    format: STRING
`,
			check: func(t *testing.T, a flowAnalysis) {
				if len(a.TaskTypes) != 1 || a.TaskTypes["io.kestra.plugin.core.log.Log"] != 1 {
					t.Errorf("unexpected task types: %v", a.TaskTypes)
				}
				if !a.HasInputs {
					t.Error("expected the flow to be flagged as having inputs")
				}
			},
		},
		{
			name: "dotless types inside task properties are ignored",
			source: `
id: f
namespace: ns
tasks:
  - id: query
    type: io.kestra.plugin.jdbc.postgresql.Query
    fetchType: FETCH
    columns:
      - name: id
        type: INTEGER
`,
			check: func(t *testing.T, a flowAnalysis) {
				if len(a.TaskTypes) != 1 {
					t.Errorf("unexpected task types: %v", a.TaskTypes)
				}
			},
		},
		{
			name: "taskDefaults alias is out of scope",
			source: `
id: f
namespace: ns
taskDefaults:
  - type: io.kestra.plugin.core.log.Log
    forced: true
    values:
      level: INFO
tasks:
  - id: log
    type: io.kestra.plugin.core.log.Log
`,
			check: func(t *testing.T, a flowAnalysis) {
				if a.PluginDefaultsEntries != 0 {
					t.Errorf("taskDefaults must not be counted, got %d entries", a.PluginDefaultsEntries)
				}
			},
		},
		{
			name: "task condition properties are not counted",
			source: `
id: f
namespace: ns
tasks:
  - id: branch
    type: io.kestra.plugin.core.flow.If
    condition: "{{ inputs.flag }}"
    then:
      - id: log
        type: io.kestra.plugin.core.log.Log
        condition: "{{ inputs.other }}"
`,
			check: func(t *testing.T, a flowAnalysis) {
				if a.ConditionProperty != 0 {
					t.Errorf("condition property: got %d, want 0 (the If task keeps `condition` in 2.0)", a.ConditionProperty)
				}
			},
		},
		{
			name: "flow-level checks and triggers carry the condition signal",
			source: `
id: f
namespace: ns
checks:
  - id: freshness
    condition: "{{ trigger.date }}"
triggers:
  - id: schedule
    type: io.kestra.plugin.core.trigger.Schedule
    cron: "0 * * * *"
    condition: "{{ flow.namespace }}"
tasks:
  - id: log
    type: io.kestra.plugin.core.log.Log
`,
			check: func(t *testing.T, a flowAnalysis) {
				if a.ConditionProperty != 2 {
					t.Errorf("condition property: got %d, want 2", a.ConditionProperty)
				}
			},
		},
		{
			name: "trigger conditions are counted per entry",
			source: `
id: f
namespace: ns
triggers:
  - id: schedule
    type: io.kestra.plugin.core.trigger.Schedule
    cron: "0 * * * *"
    conditions:
      - type: io.kestra.plugin.core.condition.DayWeekInMonth
      - type: io.kestra.plugin.core.condition.WeekendCondition
      - type: io.kestra.plugin.core.condition.DateTimeBetween
`,
			check: func(t *testing.T, a flowAnalysis) {
				if a.TriggerConditions != 3 {
					t.Errorf("trigger conditions: got %d, want 3", a.TriggerConditions)
				}
			},
		},
		{
			name: "triggers without conditions raise no signal",
			source: `
id: f
namespace: ns
triggers:
  - id: schedule
    type: io.kestra.plugin.core.trigger.Schedule
    cron: "0 * * * *"
`,
			check: func(t *testing.T, a flowAnalysis) {
				if a.TriggerConditions != 0 || a.ConditionProperty != 0 {
					t.Errorf("unexpected condition signals: %+v", a)
				}
				if !a.HasTriggers {
					t.Error("expected the flow to be flagged as having triggers")
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.check(t, mustAnalyze(t, tc.source))
		})
	}
}

func TestAnalyzeFlowSource_ParseFailureKeepsTextSignals(t *testing.T) {
	analysis, err := analyzeFlowSource("id: broken\n\ttabs: are: not: yaml\n{{ json(x) }}")
	if err == nil {
		t.Fatal("expected a parse error")
	}
	if !analysis.ParseFailed {
		t.Error("expected ParseFailed to be set")
	}
	if analysis.PebbleJSON != 1 {
		t.Errorf("pebble json: got %d, want 1", analysis.PebbleJSON)
	}
}

func TestPluginFamily(t *testing.T) {
	cases := map[string]string{
		"io.kestra.plugin.core.log.Log":             "io.kestra.plugin.core",
		"io.kestra.plugin.jdbc.postgresql.Query":    "io.kestra.plugin.jdbc",
		"io.kestra.core.tasks.flows.EachSequential": "io.kestra.plugin.core",
		"com.acme.internal.tasks.DoSomething":       "com.acme.internal.tasks",
		"io.kestra.plugin.fs.local.Delete":          "io.kestra.plugin.fs",
		"io.kestra.plugin.ee.jdbc.snowflake.Query":  "io.kestra.plugin.ee.jdbc",
		"io.kestra.plugin.ee.kafka.Produce":         "io.kestra.plugin.ee.kafka",
	}

	for typeName, want := range cases {
		if got := pluginFamily(typeName); got != want {
			t.Errorf("pluginFamily(%q): got %q, want %q", typeName, got, want)
		}
	}
}

func TestAnonymizer_StableAndKindSeparated(t *testing.T) {
	first := newAnonymizer(true)
	second := newAnonymizer(true)

	if first.namespace("prod.team") != second.namespace("prod.team") {
		t.Error("expected the same label across anonymizer instances")
	}
	if !strings.HasPrefix(first.namespace("prod.team"), "ns-") {
		t.Error("expected the ns- prefix")
	}
	if first.flow("prod.team") == first.namespace("prod.team") {
		t.Error("expected different kinds to hash differently")
	}
	if strings.Contains(first.namespace("prod.team"), "prod") {
		t.Error("the namespace name must not survive anonymization")
	}
	if first.flowRef("main", "prod.team", "my-flow") != first.tenant("main")+"/"+first.namespace("prod.team")+"/"+first.flow("my-flow") {
		t.Error("unexpected flow reference format")
	}
}

func TestAnonymizer_CollisionLengthensLabel(t *testing.T) {
	anon := newAnonymizer(true)

	label := anon.namespace("alpha")
	// Simulate another identifier having claimed the short label already.
	delete(anon.cache, "namespace:alpha")
	anon.used[label] = "namespace:squatter"

	relabeled := anon.namespace("alpha")
	if relabeled == label {
		t.Fatal("expected the colliding label to be lengthened")
	}
	if len(relabeled) <= len(label) || !strings.HasPrefix(relabeled, "ns-") {
		t.Fatalf("unexpected relabeled value %q", relabeled)
	}
}

func TestAnonymizer_IdentityWhenDisabled(t *testing.T) {
	anon := newAnonymizer(false)

	if anon.namespace("prod.team") != "prod.team" || anon.tenant("main") != "main" {
		t.Error("expected identity mapping when anonymization is disabled")
	}
	if anon.flowRef("main", "prod.team", "my-flow") != "main/prod.team/my-flow" {
		t.Error("unexpected flow reference when anonymization is disabled")
	}
}

// flowFor builds a minimal analyzed flow for aggregation tests.
func flowFor(namespace, id string, mutate func(a *flowAnalysis)) flowAnalysis {
	analysis := flowAnalysis{
		Namespace:           namespace,
		FlowID:              id,
		TaskTypes:           map[string]int64{},
		TriggerTypes:        map[string]int64{},
		PluginDefaultsTypes: map[string]int64{},
		RemovedTasks:        map[string]int64{},
	}
	if mutate != nil {
		mutate(&analysis)
	}
	return analysis
}

func testReport(t *testing.T, anonymize bool, scans []tenantScan) *usageReport {
	t.Helper()
	report := aggregateReport(scans, newAnonymizer(anonymize), time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC))
	report.Scope = usageReportScope
	return report
}

func TestAggregateReport_MergesTenantsAndOrders(t *testing.T) {
	scans := []tenantScan{
		{
			Tenant: "main",
			Flows: []flowAnalysis{
				flowFor("ns.a", "f1", func(a *flowAnalysis) {
					a.HasTriggers = true
					a.TaskTypes["io.kestra.plugin.core.log.Log"] = 3
					a.TriggerTypes["io.kestra.plugin.core.trigger.Schedule"] = 1
					a.RemovedTasks["ForEach"] = 2
					a.PluginDefaultsEntries = 1
					a.PluginDefaultsForced = 1
					a.PluginDefaultsTypes["io.kestra.plugin.core.log.Log"] = 1
				}),
				flowFor("ns.a", "f2", func(a *flowAnalysis) {
					a.Disabled = true
					a.TaskTypes["io.kestra.plugin.jdbc.postgresql.Query"] = 1
					a.SubflowTasks = 2
				}),
			},
			Deprecated: []deprecatedFlow{{
				Namespace: "ns.a",
				FlowID:    "f1",
				TaskCount: 2,
				Tasks: []deprecatedTask{
					{TaskType: "io.kestra.core.tasks.flows.EachSequential", Replacement: "io.kestra.plugin.core.flow.ForEach"},
					{TaskType: "io.kestra.core.tasks.flows.EachSequential", Replacement: "io.kestra.plugin.core.flow.ForEach"},
				},
			}},
			DeprecatedAvailable: true,
		},
		{
			Tenant: "other",
			Flows: []flowAnalysis{
				flowFor("ns.b", "f3", func(a *flowAnalysis) {
					a.HasInputs = true
					a.TaskTypes["io.kestra.plugin.core.log.Log"] = 1
					a.PebbleJSON = 4
				}),
				// A flow whose YAML did not parse keeps its text signals but
				// contributes no identity.
				flowFor("", "", func(a *flowAnalysis) {
					a.ParseFailed = true
					a.PebbleJSON = 1
				}),
			},
			Notes: []string{"used the fallback"},
		},
	}

	report := testReport(t, true, scans)

	if report.Totals.TenantsScanned != 2 || report.Totals.Count != 3 {
		t.Fatalf("unexpected totals: %+v", report.Totals)
	}
	if report.Totals.NamespacesCount != 2 {
		t.Errorf("namespaces: got %d, want 2", report.Totals.NamespacesCount)
	}
	if report.Totals.DisabledFlows != 1 || report.Totals.FlowsWithInputs != 1 || report.Totals.FlowsWithTriggers != 1 {
		t.Errorf("unexpected flow flags in totals: %+v", report.Totals)
	}
	if report.Totals.TaskTypeCount["io.kestra.plugin.core.log.Log"] != 4 {
		t.Errorf("task type count not merged across tenants: %v", report.Totals.TaskTypeCount)
	}
	if report.Totals.TaskTypeFlowCount["io.kestra.plugin.core.log.Log"] != 2 {
		t.Errorf("task type flow count: %v", report.Totals.TaskTypeFlowCount)
	}
	if report.Totals.PluginFamilyCount["io.kestra.plugin.core"] != 5 {
		t.Errorf("plugin families: %v", report.Totals.PluginFamilyCount)
	}
	if report.Totals.SubflowTaskCount != 2 || report.Totals.FlowsUsingSubflow != 1 {
		t.Errorf("unexpected subflow totals: %+v", report.Totals)
	}

	if report.Signals.PebbleJsonFunction.Occurrences != 5 || report.Signals.PebbleJsonFunction.Flows != 1 {
		t.Errorf("unexpected pebble signal: %+v", report.Signals.PebbleJsonFunction)
	}
	if report.Signals.PluginDefaults.Entries != 1 || report.Signals.PluginDefaults.ForcedEntries != 1 {
		t.Errorf("unexpected plugin defaults signal: %+v", report.Signals.PluginDefaults)
	}
	// Every removed task must have a row, even the unused ones.
	for _, class := range removedTaskClasses {
		if _, ok := report.Signals.RemovedTasks[class]; !ok {
			t.Errorf("missing removed task row for %s", class)
		}
	}
	if report.Signals.RemovedTasks["ForEach"].Occurrences != 2 {
		t.Errorf("unexpected ForEach signal: %+v", report.Signals.RemovedTasks["ForEach"])
	}
	if !report.Signals.ServerDeprecationsAvailable || len(report.Signals.ServerDeprecations) != 1 {
		t.Errorf("unexpected server deprecations: %+v", report.Signals.ServerDeprecations)
	}
	if len(report.Signals.DeprecatedTaskTypes) != 1 {
		t.Fatalf("unexpected deprecated task types: %+v", report.Signals.DeprecatedTaskTypes)
	}
	deprecated := report.Signals.DeprecatedTaskTypes[0]
	if deprecated.TaskType != "io.kestra.core.tasks.flows.EachSequential" || deprecated.Count != 2 {
		t.Errorf("unexpected deprecated task type: %+v", deprecated)
	}
	if deprecated.Replacement != "io.kestra.plugin.core.flow.ForEach" {
		t.Errorf("unexpected replacement: %q", deprecated.Replacement)
	}

	if len(report.Tenants) != 2 || report.Tenants[0].ParseFails != 0 || report.Tenants[1].ParseFails != 1 {
		t.Errorf("unexpected per-tenant reports: %+v", report.Tenants)
	}
	if len(report.Tenants[0].Namespaces) != 1 || report.Tenants[0].Namespaces[0].FlowCount != 2 {
		t.Errorf("unexpected namespace breakdown: %+v", report.Tenants[0].Namespaces)
	}

	joinedNotes := strings.Join(report.Notes, "\n")
	if !strings.Contains(joinedNotes, "used the fallback") || !strings.Contains(joinedNotes, "could not be parsed") {
		t.Errorf("unexpected notes: %v", report.Notes)
	}

	// Ordering must be deterministic: descending count, then name.
	entries := sortedCounts(report.Totals.TaskTypeCount)
	if entries[0].Name != "io.kestra.plugin.core.log.Log" || entries[1].Name != "io.kestra.plugin.jdbc.postgresql.Query" {
		t.Errorf("unexpected count ordering: %+v", entries)
	}
}

func TestAggregateReport_CapsFlowReferences(t *testing.T) {
	scan := tenantScan{Tenant: "main"}
	for i := 0; i < maxFlowRefs+10; i++ {
		scan.Flows = append(scan.Flows, flowFor("ns", fmt.Sprintf("flow-%02d", i), func(a *flowAnalysis) {
			a.RemovedTasks["ForEach"] = 1
		}))
	}

	report := testReport(t, false, []tenantScan{scan})
	signal := report.Signals.RemovedTasks["ForEach"]

	if signal.Flows != maxFlowRefs+10 {
		t.Errorf("flows: got %d, want %d", signal.Flows, maxFlowRefs+10)
	}
	if len(signal.FlowRefs) != maxFlowRefs {
		t.Fatalf("flow refs: got %d, want %d", len(signal.FlowRefs), maxFlowRefs)
	}
	for i := 1; i < len(signal.FlowRefs); i++ {
		if signal.FlowRefs[i-1] > signal.FlowRefs[i] {
			t.Fatal("expected the flow references to be sorted")
		}
	}
}

func TestRenderUsageReportMarkdown(t *testing.T) {
	analysis := mustAnalyze(t, nestedFlowSource)
	report := testReport(t, true, []tenantScan{{Tenant: "main", Flows: []flowAnalysis{analysis}}})

	var buf bytes.Buffer
	if err := renderUsageReportMarkdown(report, &buf); err != nil {
		t.Fatalf("renderUsageReportMarkdown returned an error: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		"# Kestra usage report",
		"## Migration signals",
		"## pluginDefaults detail",
		"## Affected flows",
		"## Inventory",
		"## Scan notes",
		"| Task ForEachItem | 1 | 1 |",
		"| Task ForEach | 0 | 0 |",
		"| Trigger conditions/preconditions | 3 | 1 |",
		"io.kestra.plugin.core.trigger.Schedule",
		"Scope: single-tenant",
		"- Kestra version: unknown",
		"The deprecation endpoint could not be read",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown is missing %q", want)
		}
	}
	if !strings.Contains(out, "Anonymization: on") {
		t.Error("expected the anonymization note")
	}
}

func TestRenderUsageReportMarkdown_ServerDeprecations(t *testing.T) {
	scan := tenantScan{
		Tenant:              "main",
		Flows:               []flowAnalysis{flowFor("ns.a", "f1", nil)},
		DeprecatedAvailable: true,
		Deprecated: []deprecatedFlow{
			{
				Namespace: "ns.a", FlowID: "f1", TaskCount: 2,
				Tasks: []deprecatedTask{
					{TaskType: "io.kestra.core.tasks.flows.EachSequential", Replacement: "io.kestra.plugin.core.flow.ForEach"},
					{TaskType: "io.kestra.plugin.core.flow.Worker"},
				},
			},
			{
				Namespace: "ns.a", FlowID: "f2", TaskCount: 1,
				Tasks: []deprecatedTask{
					{TaskType: "io.kestra.core.tasks.flows.EachSequential", Replacement: "io.kestra.plugin.core.flow.ForEach"},
				},
			},
		},
	}
	report := testReport(t, true, []tenantScan{scan})

	var buf bytes.Buffer
	if err := renderUsageReportMarkdown(report, &buf); err != nil {
		t.Fatalf("renderUsageReportMarkdown returned an error: %v", err)
	}
	out := buf.String()

	// Occurrences is the deprecated-task total, Flows the number of flows.
	if !strings.Contains(out, "| Server-reported deprecations | 3 | 2 |") {
		t.Error("unexpected server deprecation signal row")
	}
	for _, want := range []string{
		"## Deprecated task types (server-reported)",
		"| `io.kestra.core.tasks.flows.EachSequential` | `io.kestra.plugin.core.flow.ForEach` | 2 |",
		"| `io.kestra.plugin.core.flow.Worker` | - | 1 |",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown is missing %q", want)
		}
	}
	// Task ids are user identifiers and must never be rendered.
	if strings.Contains(out, "taskId") {
		t.Error("the report must not carry task ids")
	}
}

func TestUsageReportJSONRoundTrip(t *testing.T) {
	analysis := mustAnalyze(t, nestedFlowSource)
	report := testReport(t, true, []tenantScan{{Tenant: "main", Flows: []flowAnalysis{analysis}}})

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("failed to marshal the report: %v", err)
	}

	var decoded usageReport
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal the report: %v", err)
	}
	if decoded.Scope != usageReportScope || !decoded.Anonymized {
		t.Errorf("unexpected decoded header: %+v", decoded)
	}
	if decoded.Totals.Count != 1 || decoded.Signals.RemovedTasks["ForEachItem"].Occurrences != 1 {
		t.Errorf("unexpected decoded body: %+v", decoded)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal into a map: %v", err)
	}
	for _, key := range []string{"generated_at", "kestractl_version", "anonymized", "scope", "totals", "signals", "tenants"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing JSON key %q", key)
		}
	}
}

const secretFlowSource = `
id: leaky-flow
namespace: prod.secrets
inputs:
  - id: token
    type: STRING
    defaults: SENTINEL-SECRET-VALUE
variables:
  password: SENTINEL-SECRET-VALUE
tasks:
  - id: query
    type: io.kestra.plugin.jdbc.postgresql.Query
    url: jdbc:postgresql://db/app?password=SENTINEL-SECRET-VALUE
    sql: SELECT * FROM customers WHERE email = 'SENTINEL-SECRET-VALUE'
  - id: each
    type: io.kestra.plugin.core.flow.ForEach
    values: SENTINEL-SECRET-VALUE
    tasks:
      - id: log
        type: io.kestra.plugin.core.log.Log
        message: SENTINEL-SECRET-VALUE
pluginDefaults:
  - type: io.kestra.plugin.core.log.Log
    values:
      level: SENTINEL-SECRET-VALUE
`

func TestUsageReport_DoesNotLeakFlowValues(t *testing.T) {
	const sentinel = "SENTINEL-SECRET-VALUE"

	for _, anonymize := range []bool{true, false} {
		t.Run(fmt.Sprintf("anonymize=%t", anonymize), func(t *testing.T) {
			analysis := mustAnalyze(t, secretFlowSource)
			report := testReport(t, anonymize, []tenantScan{{Tenant: "main", Flows: []flowAnalysis{analysis}}})

			var markdown bytes.Buffer
			if err := renderUsageReportMarkdown(report, &markdown); err != nil {
				t.Fatalf("renderUsageReportMarkdown returned an error: %v", err)
			}
			data, err := json.Marshal(report)
			if err != nil {
				t.Fatalf("failed to marshal the report: %v", err)
			}

			if strings.Contains(markdown.String(), sentinel) {
				t.Error("the markdown report leaked a flow property value")
			}
			if strings.Contains(string(data), sentinel) {
				t.Error("the JSON report leaked a flow property value")
			}

			// Sanity check: the signal itself is still reported.
			if report.Signals.RemovedTasks["ForEach"].Occurrences != 1 {
				t.Error("expected the ForEach signal to be counted")
			}

			names := strings.Contains(markdown.String(), "prod.secrets") || strings.Contains(markdown.String(), "leaky-flow")
			if anonymize && names {
				t.Error("anonymized report must not contain real names")
			}
			if !anonymize && !names {
				t.Error("non-anonymized report should contain the real names")
			}
		})
	}
}

// buildFlowZip assembles an in-memory export archive from name/content pairs.
func buildFlowZip(t *testing.T, entries map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for name, content := range entries {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatalf("failed to create the zip entry %q: %v", name, err)
		}
		if _, err := file.Write([]byte(content)); err != nil {
			t.Fatalf("failed to write the zip entry %q: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close the archive: %v", err)
	}
	return buf.Bytes()
}

func TestFlowsFromZip(t *testing.T) {
	archive := buildFlowZip(t, map[string]string{
		"ns/first.yml":   "id: first\nnamespace: ns\n",
		"ns/second.yaml": "id: second\nnamespace: ns\n",
		"ns/readme.txt":  "not a flow",
	})

	sources, skipped, err := flowsFromZip(archive)
	if err != nil {
		t.Fatalf("flowsFromZip returned an error: %v", err)
	}
	if len(sources) != 2 {
		t.Fatalf("sources: got %d, want 2", len(sources))
	}
	if skipped != 0 {
		t.Errorf("skipped: got %d, want 0", skipped)
	}
	for _, source := range sources {
		if !strings.Contains(source, "namespace: ns") {
			t.Errorf("unexpected source content: %q", source)
		}
	}
}

func TestFlowsFromZip_InvalidArchive(t *testing.T) {
	if _, _, err := flowsFromZip([]byte("not a zip file")); err == nil {
		t.Fatal("expected an error for an invalid archive")
	}
}
