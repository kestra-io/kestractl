package cli

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
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

func TestAnalyzeFlowSource_PebbleFunctions(t *testing.T) {
	cases := []struct {
		name          string
		source        string
		want          map[string]int64
		unknown       int64
		wantFilters   map[string]int64
		unknownFilter int64
	}{
		{
			name: "nested calls in one block",
			source: `
id: f
namespace: ns
tasks:
  - id: log
    type: io.kestra.plugin.core.log.Log
    message: "{{ render(kv('K')) }}"
`,
			want: map[string]int64{"render": 1, "kv": 1},
		},
		{
			name: "functions inside a tag block",
			source: `
id: f
namespace: ns
tasks:
  - id: log
    type: io.kestra.plugin.core.log.Log
    message: "{% if now() %}{{ uuid() }}{% endif %}"
`,
			want: map[string]int64{"now": 1, "uuid": 1},
		},
		{
			name:   "multiline expression",
			source: "id: f\nnamespace: ns\ntasks:\n  - id: log\n    type: io.kestra.plugin.core.log.Log\n    message: |\n      {{ secret(\n        'MY_KEY'\n      ) }}\n",
			want:   map[string]int64{"secret": 1},
		},
		{
			name: "function names are case-insensitive",
			source: `
id: f
namespace: ns
tasks:
  - id: log
    type: io.kestra.plugin.core.log.Log
    message: "{{ fromjson(inputs.raw) }} {{ FROMJSON(inputs.other) }}"
`,
			want: map[string]int64{"fromJson": 2},
		},
		{
			name: "method calls and text outside expression blocks are ignored",
			source: `
id: f
namespace: ns
tasks:
  - id: query
    type: io.kestra.plugin.jdbc.postgresql.Query
    sql: SELECT COUNT(*) FROM orders
    message: myfunc(1)
    other: "{{ payload.json(x) }}"
`,
			want: map[string]int64{},
		},
		{
			name: "unknown identifiers are counted without their name",
			source: `
id: f
namespace: ns
tasks:
  - id: log
    type: io.kestra.plugin.core.log.Log
    message: "{{ mycompanyhelper(now()) }}"
`,
			want:    map[string]int64{"now": 1},
			unknown: 1,
		},
		{
			name: "engine-registered functions are recognized",
			source: `
id: f
namespace: ns
tasks:
  - id: log
    type: io.kestra.plugin.core.log.Log
    message: "{{ subflow('ns', 'flow') }} {{ assets('id') }}"
`,
			want: map[string]int64{"subflow": 1, "assets": 1},
		},
		{
			name: "the deprecated json filter and the raw filter are recognized",
			source: `
id: f
namespace: ns
tasks:
  - id: log
    type: io.kestra.plugin.core.log.Log
    message: "{{ inputs.payload | json }} {{ inputs.text | raw }}"
`,
			want:        map[string]int64{},
			wantFilters: map[string]int64{"json": 1, "raw": 1},
		},
		{
			name: "filters with arguments are filters, not unknown functions",
			source: `
id: f
namespace: ns
tasks:
  - id: log
    type: io.kestra.plugin.core.log.Log
    message: "{{ now() | date('yyyy-MM-dd') }}"
`,
			want:        map[string]int64{"now": 1},
			wantFilters: map[string]int64{"date": 1},
		},
		{
			name: "bare and chained filters are counted",
			source: `
id: f
namespace: ns
tasks:
  - id: log
    type: io.kestra.plugin.core.log.Log
    message: "{{ inputs.name | trim | lower }}"
`,
			want:        map[string]int64{},
			wantFilters: map[string]int64{"trim": 1, "lower": 1},
		},
		{
			name: "a parenthesized filter is counted exactly once",
			source: `
id: f
namespace: ns
tasks:
  - id: log
    type: io.kestra.plugin.core.log.Log
    message: "{{ items | join(',') }}"
`,
			want:        map[string]int64{},
			wantFilters: map[string]int64{"join": 1},
		},
		{
			name: "filter names are case-insensitive",
			source: `
id: f
namespace: ns
tasks:
  - id: log
    type: io.kestra.plugin.core.log.Log
    message: "{{ x | DATE('x') }} {{ y | Upper }}"
`,
			want:        map[string]int64{},
			wantFilters: map[string]int64{"date": 1, "upper": 1},
		},
		{
			name: "a boolean or is not a filter",
			source: `
id: f
namespace: ns
tasks:
  - id: log
    type: io.kestra.plugin.core.log.Log
    message: "{{ a || b(1) }}"
`,
			want:    map[string]int64{},
			unknown: 1,
		},
		{
			name: "unknown filters go to their own anonymous bucket",
			source: `
id: f
namespace: ns
tasks:
  - id: log
    type: io.kestra.plugin.core.log.Log
    message: "{{ y | customFilter(2) }}"
`,
			want:          map[string]int64{},
			unknownFilter: 1,
		},
		{
			name: "foreign templating stays in the unknown function bucket",
			source: `
id: f
namespace: ns
tasks:
  - id: query
    type: io.kestra.plugin.jdbc.postgresql.Query
    sql: "SELECT * FROM {{ ref('my_model') }}"
`,
			want:    map[string]int64{},
			unknown: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			analysis := mustAnalyze(t, tc.source)

			if len(analysis.PebbleFunctions) != len(tc.want) {
				t.Fatalf("pebble functions: got %v, want %v", analysis.PebbleFunctions, tc.want)
			}
			for name, want := range tc.want {
				if got := analysis.PebbleFunctions[name]; got != want {
					t.Errorf("function %s: got %d, want %d", name, got, want)
				}
			}
			if analysis.PebbleUnknownFunctions != tc.unknown {
				t.Errorf("unknown functions: got %d, want %d", analysis.PebbleUnknownFunctions, tc.unknown)
			}
			if len(analysis.PebbleFilters) != len(tc.wantFilters) {
				t.Fatalf("pebble filters: got %v, want %v", analysis.PebbleFilters, tc.wantFilters)
			}
			for name, want := range tc.wantFilters {
				if got := analysis.PebbleFilters[name]; got != want {
					t.Errorf("filter %s: got %d, want %d", name, got, want)
				}
			}
			if analysis.PebbleUnknownFilters != tc.unknownFilter {
				t.Errorf("unknown filters: got %d, want %d", analysis.PebbleUnknownFilters, tc.unknownFilter)
			}
			for name := range analysis.PebbleFunctions {
				if _, ok := pebbleFunctions[strings.ToLower(name)]; !ok {
					t.Errorf("%q is not an allowlisted function name", name)
				}
			}
			for name := range analysis.PebbleFilters {
				if _, ok := pebbleFilters[strings.ToLower(name)]; !ok {
					t.Errorf("%q is not an allowlisted filter name", name)
				}
			}
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
	if analysis.PebbleFunctions["json"] != 1 {
		t.Errorf("pebble functions: got %v, want one json call", analysis.PebbleFunctions)
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
	if err := renderUsageReportMarkdown(report, &buf, true); err != nil {
		t.Fatalf("renderUsageReportMarkdown returned an error: %v", err)
	}
	out := collapseSpaces(buf.String())

	for _, want := range []string{
		"# Kestra usage report",
		"## Migration signals",
		"## pluginDefaults detail",
		"## Affected flows",
		"## Inventory",
		"## Scan notes",
		"## Task types",
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
	if err := renderUsageReportMarkdown(report, &buf, true); err != nil {
		t.Fatalf("renderUsageReportMarkdown returned an error: %v", err)
	}
	out := collapseSpaces(buf.String())

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

// spaceRuns matches the column padding of the aligned tables.
var spaceRuns = regexp.MustCompile(` {2,}`)

// collapseSpaces squeezes the alignment padding out of a rendered report so
// assertions can be written about table content rather than column widths.
func collapseSpaces(markdown string) string {
	return spaceRuns.ReplaceAllString(markdown, " ")
}

// tableLines returns the pipe-table rows that follow the given heading.
func tableLines(t *testing.T, markdown, heading string) []string {
	t.Helper()

	index := strings.Index(markdown, heading)
	if index < 0 {
		t.Fatalf("the report is missing %q", heading)
	}

	var rows []string
	for _, line := range strings.Split(markdown[index:], "\n") {
		if strings.HasPrefix(line, "|") {
			rows = append(rows, line)
			continue
		}
		if len(rows) > 0 {
			break
		}
	}
	if len(rows) < 3 {
		t.Fatalf("expected a table under %q, got %d row(s)", heading, len(rows))
	}
	return rows
}

func TestRenderUsageReportMarkdown_TablesAreColumnAligned(t *testing.T) {
	analysis := mustAnalyze(t, nestedFlowSource)
	report := testReport(t, true, []tenantScan{{Tenant: "main", Flows: []flowAnalysis{analysis}}})

	var buf bytes.Buffer
	if err := renderUsageReportMarkdown(report, &buf, true); err != nil {
		t.Fatalf("renderUsageReportMarkdown returned an error: %v", err)
	}
	out := buf.String()

	// The task-type table is pure ASCII, so byte length equals column width.
	rows := tableLines(t, out, "## Task types")
	width := len(rows[0])
	pipes := pipeColumns(rows[0])
	for _, row := range rows[1:] {
		if len(row) != width {
			t.Errorf("row %q is %d bytes wide, want %d", row, len(row), width)
		}
		if got := pipeColumns(row); !equalInts(got, pipes) {
			t.Errorf("row %q has pipes at %v, want %v", row, got, pipes)
		}
	}

	// The separator keeps its alignment colon and at least three dashes.
	if !strings.Contains(rows[1], "-:") || !strings.Contains(rows[1], "---") {
		t.Errorf("unexpected separator row %q", rows[1])
	}
	// Counts are padded on the left.
	if !regexp.MustCompile(`\|\s{2,}1 \|`).MatchString(out) {
		t.Error("expected right-aligned counts to be padded on the left")
	}
	// Alignment must never be done with ANSI escapes.
	if strings.Contains(out, "\x1b[") {
		t.Error("the report must not contain ANSI escape sequences")
	}
}

func pipeColumns(row string) []int {
	var columns []int
	for i, char := range row {
		if char == '|' {
			columns = append(columns, i)
		}
	}
	return columns
}

func equalInts(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func TestRenderUsageReportMarkdown_SectionOrder(t *testing.T) {
	analysis := mustAnalyze(t, nestedFlowSource)
	report := testReport(t, true, []tenantScan{{
		Tenant:              "main",
		Flows:               []flowAnalysis{analysis},
		DeprecatedAvailable: true,
		Deprecated: []deprecatedFlow{{
			Namespace: "prod.team.data", FlowID: "nested-flow", TaskCount: 1,
			Tasks: []deprecatedTask{{TaskType: "io.kestra.core.tasks.flows.EachSequential"}},
		}},
	}})

	var buf bytes.Buffer
	if err := renderUsageReportMarkdown(report, &buf, true); err != nil {
		t.Fatalf("renderUsageReportMarkdown returned an error: %v", err)
	}
	out := buf.String()

	// The inventory opens the report and the long task-type table closes it.
	sections := []string{
		"## Inventory",
		"## Migration signals",
		"## pluginDefaults detail",
		"## Deprecated task types (server-reported)",
		"## Affected flows",
		"## Trigger types",
		"## Plugin families",
		"## Pebble functions and filters",
		"## Scan notes",
		"## Task types",
	}
	previous := -1
	for _, section := range sections {
		index := strings.Index(out, section)
		if index < 0 {
			t.Fatalf("the report is missing %q", section)
		}
		if index <= previous {
			t.Errorf("%q is out of order (index %d, previous section ended at %d)", section, index, previous)
		}
		previous = index
	}

	// Only the per-namespace table is left nested under ## Inventory.
	if index := strings.Index(out, "### Flows per namespace"); index < 0 || index > strings.Index(out, "## Migration signals") {
		t.Error("### Flows per namespace must stay inside ## Inventory")
	}
	for _, unwanted := range []string{"### Trigger types", "### Plugin families", "### Task types"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("%q must be a top-level section now", unwanted)
		}
	}
}

// A report with nothing in it must still render every trailing section: the
// empty plugin-families case returns early from its own renderer only.
func TestRenderUsageReportMarkdown_EmptyReportKeepsTrailingSections(t *testing.T) {
	report := testReport(t, true, []tenantScan{{Tenant: "main"}})

	var buf bytes.Buffer
	if err := renderUsageReportMarkdown(report, &buf, false); err != nil {
		t.Fatalf("renderUsageReportMarkdown returned an error: %v", err)
	}
	out := collapseSpaces(buf.String())

	for _, want := range []string{
		"## Plugin families",
		"No plugin usage found.",
		"## Scan notes",
		"## Task types",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the empty report is missing %q", want)
		}
	}
}

func TestRenderUsageReportMarkdown_PebbleFunctionsSection(t *testing.T) {
	source := `
id: pebble-flow
namespace: ns
tasks:
  - id: log
    type: io.kestra.plugin.core.log.Log
    message: "{{ render(kv('K')) }} {{ mycompanyhelper(x) }} {{ y | date('yyyy') | trim }} {{ z | customFilter(1) }}"
`
	report := testReport(t, true, []tenantScan{{Tenant: "main", Flows: []flowAnalysis{mustAnalyze(t, source)}}})

	var buf bytes.Buffer
	if err := renderUsageReportMarkdown(report, &buf, false); err != nil {
		t.Fatalf("renderUsageReportMarkdown returned an error: %v", err)
	}
	out := collapseSpaces(buf.String())

	for _, want := range []string{
		"## Pebble functions and filters",
		"### Functions",
		"| Function | Uses | Flows |",
		"| `kv` | 1 | 1 |",
		"| `render` | 1 | 1 |",
		"| (unrecognized function-like calls) | 1 | - |",
		"### Filters",
		"| Filter | Uses | Flows |",
		"| `date` | 1 | 1 |",
		"| `trim` | 1 | 1 |",
		"| (unrecognized filters) | 1 | - |",
		"Pebble functions and filters are extracted from the `{{ }}` and `{% %}` expression blocks",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the report is missing %q", want)
		}
	}
	for _, unwanted := range []string{"mycompanyhelper", "customFilter"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("the report must not name %q", unwanted)
		}
	}

	// A report without any expression renders the empty case.
	empty := testReport(t, true, []tenantScan{{Tenant: "main"}})
	buf.Reset()
	if err := renderUsageReportMarkdown(empty, &buf, false); err != nil {
		t.Fatalf("renderUsageReportMarkdown returned an error: %v", err)
	}
	empty2 := collapseSpaces(buf.String())
	if !strings.Contains(empty2, "### Functions\n\nNone found.") || !strings.Contains(empty2, "### Filters\n\nNone found.") {
		t.Error("expected the empty Pebble functions and filters case")
	}
}

func TestRenderUsageReportMarkdown_DetailedGating(t *testing.T) {
	analysis := mustAnalyze(t, nestedFlowSource)
	report := testReport(t, true, []tenantScan{{Tenant: "main", Flows: []flowAnalysis{analysis}}})

	render := func(detailed bool) string {
		t.Helper()
		var buf bytes.Buffer
		if err := renderUsageReportMarkdown(report, &buf, detailed); err != nil {
			t.Fatalf("renderUsageReportMarkdown returned an error: %v", err)
		}
		return collapseSpaces(buf.String())
	}

	summary := render(false)
	for _, unwanted := range []string{"### Flows per namespace", "## Affected flows"} {
		if strings.Contains(summary, unwanted) {
			t.Errorf("the summary report must not contain %q", unwanted)
		}
	}
	if !strings.Contains(summary, "_Run with --detailed to list the affected flows._") {
		t.Error("expected the summary report to point at --detailed")
	}
	// Everything else is unchanged by the flag.
	for _, want := range []string{"## Migration signals", "| Task ForEachItem | 1 | 1 |", "| Namespaces | 1 |", "## Task types"} {
		if !strings.Contains(summary, want) {
			t.Errorf("the summary report is missing %q", want)
		}
	}

	detailed := render(true)
	for _, want := range []string{"### Flows per namespace", "## Affected flows"} {
		if !strings.Contains(detailed, want) {
			t.Errorf("the detailed report is missing %q", want)
		}
	}
	if strings.Contains(detailed, "_Run with --detailed") {
		t.Error("the detailed report must not point at --detailed")
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
	totals, ok := raw["totals"].(map[string]any)
	if !ok {
		t.Fatal("the totals object is missing from the JSON report")
	}
	for _, key := range []string{
		"pebble_function_count", "pebble_function_flow_count", "pebble_unknown_function_count",
		"pebble_filter_count", "pebble_filter_flow_count", "pebble_unknown_filter_count",
	} {
		if _, ok := totals[key]; !ok {
			t.Errorf("missing JSON totals key %q", key)
		}
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
        message: "SENTINEL-SECRET-VALUE {{ sentinelSecretMacro(now()) }} {{ x | sentinelSecretFilter('y') }}"
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
			if err := renderUsageReportMarkdown(report, &markdown, true); err != nil {
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

			// An undocumented function name may be a customer macro: it is
			// counted, never named.
			for _, name := range []string{"sentinelSecretMacro", "sentinelSecretFilter"} {
				if strings.Contains(markdown.String(), name) || strings.Contains(string(data), name) {
					t.Errorf("the report leaked the unrecognized Pebble name %q", name)
				}
			}
			if report.Totals.PebbleUnknownFilterCount != 1 {
				t.Errorf("unknown pebble filters: got %d, want 1", report.Totals.PebbleUnknownFilterCount)
			}
			if report.Totals.PebbleUnknownFunctionCount != 1 {
				t.Errorf("unknown pebble functions: got %d, want 1", report.Totals.PebbleUnknownFunctionCount)
			}
			if report.Totals.PebbleFunctionCount["now"] != 1 {
				t.Errorf("expected the documented call to be counted, got %v", report.Totals.PebbleFunctionCount)
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
