package cli

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// This file holds the pure analysis core of `flows usage-report`: report
// structs, the flow-source walker, the anonymizer, aggregation and markdown
// rendering. It deliberately depends on no SDK request type so that it can be
// ported to the v2 branch unchanged.

// maxFlowRefs caps how many flow references a single signal lists. A migration
// planner needs a representative sample, not an exhaustive dump.
const maxFlowRefs = 50

// maxWalkDepth bounds the recursive walk of a flow source. Real flows nest a
// handful of levels; the bound only guards against pathological input.
const maxWalkDepth = 64

// usageReport is the serialized report. Field names mirror the SDK's FlowUsage
// shape where they overlap; JSON tags are snake_case.
type usageReport struct {
	GeneratedAt      string           `json:"generated_at"`
	KestractlVersion string           `json:"kestractl_version"`
	KestraVersion    string           `json:"kestra_version"`
	Anonymized       bool             `json:"anonymized"`
	Scope            string           `json:"scope"`
	Totals           usageTotals      `json:"totals"`
	Signals          migrationSignals `json:"signals"`
	Tenants          []tenantReport   `json:"tenants"`
	Notes            []string         `json:"notes,omitempty"`
}

// usageTotals is the flow inventory, aggregated across every scanned tenant.
type usageTotals struct {
	TenantsScanned       int              `json:"tenants_scanned"`
	Count                int              `json:"count"`
	NamespacesCount      int              `json:"namespaces_count"`
	DisabledFlows        int              `json:"disabled_flows"`
	FlowsWithInputs      int              `json:"flows_with_inputs"`
	FlowsWithTriggers    int              `json:"flows_with_triggers"`
	TaskTypeCount        map[string]int64 `json:"task_type_count"`
	TaskTypeFlowCount    map[string]int64 `json:"task_type_flow_count"`
	TriggerTypeCount     map[string]int64 `json:"trigger_type_count"`
	TriggerTypeFlowCount map[string]int64 `json:"trigger_type_flow_count"`
	PluginFamilyCount    map[string]int64 `json:"plugin_family_count"`
	SubflowTaskCount     int64            `json:"subflow_task_count"`
	FlowsUsingSubflow    int              `json:"flows_using_subflow"`
}

// signalCount is one migration signal: how often it occurs, in how many flows,
// and a capped sample of those flows.
type signalCount struct {
	Occurrences int64    `json:"occurrences"`
	Flows       int      `json:"flows"`
	FlowRefs    []string `json:"flow_refs,omitempty"`
}

// pluginDefaultsSignal details the pluginDefaults usage, which v2 removes.
type pluginDefaultsSignal struct {
	Flows         int              `json:"flows"`
	Entries       int64            `json:"entries"`
	ForcedEntries int64            `json:"forced_entries"`
	TypeCount     map[string]int64 `json:"type_count,omitempty"`
	FlowRefs      []string         `json:"flow_refs,omitempty"`
}

// serverDeprecation is one entry of the server-side deprecation cross-check
// (`flows list-deprecated`), which complements the client-side signals.
type serverDeprecation struct {
	FlowRef         string `json:"flow_ref"`
	DeprecatedTasks int    `json:"deprecated_tasks"`
}

// deprecatedTaskType aggregates the server-reported deprecations by plugin
// type. Only type names travel here — the task ids the server returns are user
// identifiers and are dropped on the way in.
type deprecatedTaskType struct {
	TaskType    string `json:"task_type"`
	Replacement string `json:"replacement,omitempty"`
	Count       int64  `json:"count"`
}

// migrationSignals holds every signal keyed to a known Kestra 2.0 breaking
// change.
type migrationSignals struct {
	PluginDefaults              pluginDefaultsSignal   `json:"plugin_defaults"`
	RemovedTasks                map[string]signalCount `json:"removed_tasks"`
	TriggerConditions           signalCount            `json:"trigger_conditions"`
	ConditionProperty           signalCount            `json:"condition_property"`
	PebbleJsonFunction          signalCount            `json:"pebble_json_function"`
	FsLocalDelete               signalCount            `json:"fs_local_delete"`
	ServerDeprecationsAvailable bool                   `json:"server_deprecations_available"`
	ServerDeprecations          []serverDeprecation    `json:"server_deprecations,omitempty"`
	DeprecatedTaskTypes         []deprecatedTaskType   `json:"deprecated_task_types,omitempty"`
}

// tenantReport is the per-tenant breakdown.
type tenantReport struct {
	Tenant     string            `json:"tenant"`
	Totals     usageTotals       `json:"totals"`
	Namespaces []namespaceReport `json:"namespaces"`
	ParseFails int               `json:"parse_fails"`
	Notes      []string          `json:"notes,omitempty"`
	Errors     []string          `json:"errors,omitempty"`
}

// namespaceReport is the per-namespace breakdown inside a tenant.
type namespaceReport struct {
	Namespace         string `json:"namespace"`
	FlowCount         int    `json:"flow_count"`
	DisabledFlows     int    `json:"disabled_flows"`
	FlowsWithTriggers int    `json:"flows_with_triggers"`
}

// deprecatedFlow is the server-reported deprecation for one flow, reduced to
// the fields the report needs.
type deprecatedFlow struct {
	Namespace string
	FlowID    string
	TaskCount int
	Tasks     []deprecatedTask
}

// deprecatedTask is one deprecated task the server reported, kept as plugin
// type names only.
type deprecatedTask struct {
	TaskType    string
	Replacement string
}

// tenantScan is everything one tenant contributed to the report: analyzed
// flows plus the notes and errors collected while fetching them.
type tenantScan struct {
	Tenant              string
	Flows               []flowAnalysis
	Deprecated          []deprecatedFlow
	DeprecatedAvailable bool
	Notes               []string
	Errors              []string
}

// flowAnalysis is the intermediate per-flow result. It is never serialized,
// and — by construction — carries only the flow's identity, booleans, counters
// and dotted plugin `type` strings. No YAML property value can reach it, which
// is what makes the anonymized report safe to share.
type flowAnalysis struct {
	Namespace   string
	FlowID      string
	Disabled    bool
	HasInputs   bool
	HasTriggers bool
	ParseFailed bool

	// TaskTypes/TriggerTypes map a dotted plugin type to its occurrence count.
	TaskTypes    map[string]int64
	TriggerTypes map[string]int64

	PluginDefaultsEntries int64
	PluginDefaultsForced  int64
	PluginDefaultsTypes   map[string]int64

	RemovedTasks      map[string]int64
	SubflowTasks      int64
	TriggerConditions int64
	ConditionProperty int64
	PebbleJSON        int64
	FsLocalDelete     int64
}

// taskContainerKeys are the flow-level keys walked in task context. inputs,
// outputs, labels, variables and description are deliberately absent: their
// `type: STRING` entries would otherwise pollute the task-type counts.
var taskContainerKeys = []string{"tasks", "errors", "finally", "afterExecution", "listeners"}

// flowTaskPrefixes are the two package spellings of Kestra's core flow tasks —
// the modern one and the pre-0.16 legacy one. Both must be recognized.
var flowTaskPrefixes = []string{"io.kestra.plugin.core.flow.", "io.kestra.core.tasks.flows."}

// removedTaskClasses are the core flow tasks removed or reworked in Kestra 2.0.
var removedTaskClasses = []string{"ForEach", "ForEachItem", "EachSequential", "EachParallel", "Dag", "Template", "Worker"}

// subflowTaskClasses are the two class names used to call a subflow.
var subflowTaskClasses = []string{"Subflow", "Flow"}

// fsLocalDeleteType changed its `recursive` default in Kestra 2.0.
const fsLocalDeleteType = "io.kestra.plugin.fs.local.Delete"

// pebbleJSONPattern matches Pebble's removed `json()` function while excluding
// `fromJson(`, `toJson(` and method-style `.json(`. It is a text heuristic —
// it also runs on sources whose YAML failed to parse — so it can over-count on
// flows that mention `json (` in prose.
var pebbleJSONPattern = regexp.MustCompile(`(^|[^\w.])json\s*\(`)

// analyzeFlowSource walks one flow's YAML source and returns the signals it
// carries. On a YAML parse failure it still returns a usable analysis with
// ParseFailed set and the raw-text signals filled in, alongside the error.
func analyzeFlowSource(raw string) (flowAnalysis, error) {
	analysis := flowAnalysis{
		TaskTypes:           map[string]int64{},
		TriggerTypes:        map[string]int64{},
		PluginDefaultsTypes: map[string]int64{},
		RemovedTasks:        map[string]int64{},
	}

	// Raw-text signals first: they must survive a YAML parse failure.
	analysis.PebbleJSON = int64(len(pebbleJSONPattern.FindAllStringIndex(raw, -1)))

	var root map[string]any
	if err := yaml.Unmarshal([]byte(raw), &root); err != nil {
		analysis.ParseFailed = true
		return analysis, err
	}
	if root == nil {
		analysis.ParseFailed = true
		return analysis, fmt.Errorf("flow source is empty")
	}

	analysis.Namespace, _ = root["namespace"].(string)
	analysis.FlowID, _ = root["id"].(string)
	if disabled, ok := root["disabled"].(bool); ok {
		analysis.Disabled = disabled
	}
	if _, ok := root["inputs"]; ok {
		analysis.HasInputs = true
	}

	for _, key := range taskContainerKeys {
		analysis.walk(root[key], false, 0)
	}

	if triggers, ok := root["triggers"]; ok {
		analysis.HasTriggers = hasAnyEntry(triggers)
		analysis.walk(triggers, true, 0)
	}

	analysis.collectPluginDefaults(root["pluginDefaults"])
	// The `condition` -> `when` rename covers flow-level checks, SLAs and
	// triggers, not task properties.
	analysis.collectConditionEntries(root["sla"])
	analysis.collectConditionEntries(root["checks"])

	return analysis, nil
}

// walk recursively records plugin types below node. It enters a map only to
// read its `type`/`condition` keys and to keep descending; property *values*
// are never recorded, which is what keeps flowAnalysis leak-free.
func (a *flowAnalysis) walk(node any, trigger bool, depth int) {
	if node == nil || depth > maxWalkDepth {
		return
	}

	switch value := node.(type) {
	case []any:
		for _, item := range value {
			a.walk(item, trigger, depth+1)
		}
	case map[string]any:
		typeName, hasType := dottedType(value)
		if hasType {
			a.recordType(typeName, trigger)
		}
		if trigger && hasType {
			// A trigger's `condition` is renamed to `when` in 2.0. Task-level
			// `condition` properties are NOT in scope: the If task, for one,
			// keeps its `condition` in 2.0.
			if _, ok := value["condition"]; ok {
				a.ConditionProperty++
			}
			a.TriggerConditions += entryCount(value["conditions"]) + entryCount(value["preconditions"])
		}
		for key, child := range value {
			// Nested tasks live under keys such as tasks/errors/then/else/
			// cases/branches; descending generically catches every flowable
			// without hardcoding each one. Trigger context does not nest.
			if key == "type" || key == "conditions" || key == "preconditions" {
				continue
			}
			a.walk(child, false, depth+1)
		}
	}
}

// recordType counts one plugin type and its migration signals.
func (a *flowAnalysis) recordType(typeName string, trigger bool) {
	if trigger {
		a.TriggerTypes[typeName]++
		return
	}

	a.TaskTypes[typeName]++
	if class, ok := coreFlowClass(typeName, removedTaskClasses); ok {
		a.RemovedTasks[class]++
	}
	if _, ok := coreFlowClass(typeName, subflowTaskClasses); ok {
		a.SubflowTasks++
	}
	if typeName == fsLocalDeleteType {
		a.FsLocalDelete++
	}
}

// collectPluginDefaults counts the flow-level pluginDefaults entries. Kestra
// 1.3 spells the key `pluginDefaults`; the pre-0.18 `taskDefaults` alias is out
// of scope.
func (a *flowAnalysis) collectPluginDefaults(node any) {
	entries, ok := node.([]any)
	if !ok {
		return
	}

	for _, item := range entries {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		a.PluginDefaultsEntries++
		if typeName, ok := dottedType(entry); ok {
			a.PluginDefaultsTypes[typeName]++
		}
		if forced, ok := entry["forced"].(bool); ok && forced {
			a.PluginDefaultsForced++
		}
	}
}

// collectConditionEntries counts the `condition` property on a list of
// flow-level entries — SLAs and checks — which v2 replaces with `when`.
func (a *flowAnalysis) collectConditionEntries(node any) {
	entries, ok := node.([]any)
	if !ok {
		return
	}

	for _, item := range entries {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if _, ok := entry["condition"]; ok {
			a.ConditionProperty++
		}
	}
}

// dottedType returns the map's `type` value only when it looks like a plugin
// class name, i.e. contains a dot. This is the guard that keeps input
// declarations (`type: STRING`) out of the task counts.
func dottedType(m map[string]any) (string, bool) {
	typeName, ok := m["type"].(string)
	if !ok {
		return "", false
	}
	typeName = strings.TrimSpace(typeName)
	if !strings.Contains(typeName, ".") {
		return "", false
	}
	return typeName, true
}

// coreFlowClass reports whether typeName is one of the given core flow classes,
// under either the modern or the legacy package spelling.
func coreFlowClass(typeName string, classes []string) (string, bool) {
	for _, prefix := range flowTaskPrefixes {
		if !strings.HasPrefix(typeName, prefix) {
			continue
		}
		class := strings.TrimPrefix(typeName, prefix)
		for _, candidate := range classes {
			if class == candidate {
				return class, true
			}
		}
	}
	return "", false
}

// entryCount returns the number of entries in a list or map node, and 0 for
// anything else.
func entryCount(node any) int64 {
	switch value := node.(type) {
	case []any:
		return int64(len(value))
	case map[string]any:
		return int64(len(value))
	default:
		return 0
	}
}

// hasAnyEntry reports whether node is a non-empty list or map.
func hasAnyEntry(node any) bool {
	switch value := node.(type) {
	case []any:
		return len(value) > 0
	case map[string]any:
		return len(value) > 0
	default:
		return false
	}
}

// pluginFamily reduces a plugin type to the family it belongs to: the class
// segment is dropped, official plugins collapse to their first four segments
// (io.kestra.plugin.jdbc), legacy core tasks map onto io.kestra.plugin.core and
// custom plugins keep their full package.
func pluginFamily(typeName string) string {
	segments := strings.Split(typeName, ".")
	if len(segments) < 2 {
		return typeName
	}
	pkg := segments[:len(segments)-1]

	if strings.HasPrefix(typeName, "io.kestra.core.tasks.") {
		return "io.kestra.plugin.core"
	}
	// EE plugins all live under io.kestra.plugin.ee, so they need one segment
	// more than the open-source ones to stay distinguishable
	// (io.kestra.plugin.ee.jdbc rather than a single io.kestra.plugin.ee).
	switch {
	case strings.HasPrefix(typeName, "io.kestra.plugin.ee."):
		if len(pkg) > 5 {
			pkg = pkg[:5]
		}
	case strings.HasPrefix(typeName, "io.kestra.plugin."):
		if len(pkg) > 4 {
			pkg = pkg[:4]
		}
	}
	return strings.Join(pkg, ".")
}

// anonymizer maps tenant, namespace and flow identifiers to stable short
// hashes. The hash is unsalted so the same identifier yields the same label on
// every run and every machine: reports stay diffable and a follow-up
// conversation can refer to "ns-3f9a12" without ever revealing the name.
type anonymizer struct {
	enabled bool
	cache   map[string]string
	used    map[string]string
}

func newAnonymizer(enabled bool) *anonymizer {
	return &anonymizer{
		enabled: enabled,
		cache:   map[string]string{},
		used:    map[string]string{},
	}
}

func (a *anonymizer) tenant(id string) string    { return a.label("tenant", "t-", id) }
func (a *anonymizer) namespace(id string) string { return a.label("namespace", "ns-", id) }
func (a *anonymizer) flow(id string) string      { return a.label("flow", "f-", id) }

// label returns the stable pseudonym for id within kind. Namespaces are hashed
// whole — segments are not preserved, since a namespace path is as identifying
// as the leaf.
func (a *anonymizer) label(kind, prefix, id string) string {
	if !a.enabled {
		return id
	}

	key := kind + ":" + id
	if label, ok := a.cache[key]; ok {
		return label
	}

	sum := sha256.Sum256([]byte(key))
	digest := hex.EncodeToString(sum[:])
	label := prefix + digest[:6]
	// Lengthen on collision so two different identifiers never share a label.
	for length := 8; length <= len(digest); length += 2 {
		if owner, taken := a.used[label]; !taken || owner == key {
			break
		}
		label = prefix + digest[:length]
	}

	a.cache[key] = label
	a.used[label] = key
	return label
}

// flowRef builds the "t-xx/ns-yy/f-zz" reference used throughout the report.
func (a *anonymizer) flowRef(tenant, namespace, flowID string) string {
	return a.tenant(tenant) + "/" + a.namespace(namespace) + "/" + a.flow(flowID)
}

// signalAccumulator collects one signal's occurrences and affected flows while
// aggregating, capping the reference sample at maxFlowRefs.
type signalAccumulator struct {
	occurrences int64
	flows       int
	refs        []string
}

func (s *signalAccumulator) add(occurrences int64, ref string) {
	if occurrences <= 0 {
		return
	}
	s.occurrences += occurrences
	if ref == "" {
		return
	}
	s.flows++
	if len(s.refs) < maxFlowRefs {
		s.refs = append(s.refs, ref)
	}
}

func (s *signalAccumulator) result() signalCount {
	sort.Strings(s.refs)
	return signalCount{Occurrences: s.occurrences, Flows: s.flows, FlowRefs: s.refs}
}

// aggregateReport merges the per-tenant scans into the final report. The
// caller fills in Scope and any scope-level notes.
func aggregateReport(scans []tenantScan, anon *anonymizer, generatedAt time.Time) *usageReport {
	report := &usageReport{
		GeneratedAt:      generatedAt.UTC().Format(time.RFC3339),
		KestractlVersion: version,
		Anonymized:       anon.enabled,
		Totals:           newUsageTotals(),
		Signals: migrationSignals{
			RemovedTasks: map[string]signalCount{},
			PluginDefaults: pluginDefaultsSignal{
				TypeCount: map[string]int64{},
			},
		},
		Tenants: make([]tenantReport, 0, len(scans)),
	}

	var (
		defaults          signalAccumulator
		triggerConditions signalAccumulator
		conditionProperty signalAccumulator
		pebbleJSON        signalAccumulator
		fsLocalDelete     signalAccumulator
		removed           = map[string]*signalAccumulator{}
		namespaceKeys     = map[string]struct{}{}
		deprecatedTypes   = map[deprecatedTask]int64{}
	)

	for _, scan := range scans {
		tenant := tenantReport{
			Tenant:     anon.tenant(scan.Tenant),
			Totals:     newUsageTotals(),
			Namespaces: []namespaceReport{},
			Notes:      scan.Notes,
			Errors:     scan.Errors,
		}
		namespaces := map[string]*namespaceReport{}

		for _, flow := range scan.Flows {
			// A flow whose YAML did not parse has no identity: count it as a
			// parse failure and keep only its raw-text signals.
			ref := ""
			if !flow.ParseFailed {
				ref = anon.flowRef(scan.Tenant, flow.Namespace, flow.FlowID)
				tenant.Totals.Count++
				namespaceKeys[scan.Tenant+"/"+flow.Namespace] = struct{}{}

				ns, ok := namespaces[flow.Namespace]
				if !ok {
					ns = &namespaceReport{Namespace: anon.namespace(flow.Namespace)}
					namespaces[flow.Namespace] = ns
				}
				ns.FlowCount++
				if flow.Disabled {
					ns.DisabledFlows++
					tenant.Totals.DisabledFlows++
				}
				if flow.HasInputs {
					tenant.Totals.FlowsWithInputs++
				}
				if flow.HasTriggers {
					ns.FlowsWithTriggers++
					tenant.Totals.FlowsWithTriggers++
				}
				if flow.SubflowTasks > 0 {
					tenant.Totals.FlowsUsingSubflow++
				}
				tenant.Totals.SubflowTaskCount += flow.SubflowTasks

				for typeName, count := range flow.TaskTypes {
					tenant.Totals.TaskTypeCount[typeName] += count
					tenant.Totals.TaskTypeFlowCount[typeName]++
					tenant.Totals.PluginFamilyCount[pluginFamily(typeName)] += count
				}
				for typeName, count := range flow.TriggerTypes {
					tenant.Totals.TriggerTypeCount[typeName] += count
					tenant.Totals.TriggerTypeFlowCount[typeName]++
					tenant.Totals.PluginFamilyCount[pluginFamily(typeName)] += count
				}
			} else {
				tenant.ParseFails++
			}

			if flow.PluginDefaultsEntries > 0 {
				defaults.add(flow.PluginDefaultsEntries, ref)
				report.Signals.PluginDefaults.ForcedEntries += flow.PluginDefaultsForced
				for typeName, count := range flow.PluginDefaultsTypes {
					report.Signals.PluginDefaults.TypeCount[typeName] += count
				}
			}
			for class, count := range flow.RemovedTasks {
				acc, ok := removed[class]
				if !ok {
					acc = &signalAccumulator{}
					removed[class] = acc
				}
				acc.add(count, ref)
			}
			triggerConditions.add(flow.TriggerConditions, ref)
			conditionProperty.add(flow.ConditionProperty, ref)
			pebbleJSON.add(flow.PebbleJSON, ref)
			fsLocalDelete.add(flow.FsLocalDelete, ref)
		}

		tenant.Totals.TenantsScanned = 1
		tenant.Totals.NamespacesCount = len(namespaces)
		tenant.Namespaces = sortedNamespaceReports(namespaces)

		if scan.DeprecatedAvailable {
			report.Signals.ServerDeprecationsAvailable = true
			for _, dep := range scan.Deprecated {
				report.Signals.ServerDeprecations = append(report.Signals.ServerDeprecations, serverDeprecation{
					FlowRef:         anon.flowRef(scan.Tenant, dep.Namespace, dep.FlowID),
					DeprecatedTasks: dep.TaskCount,
				})
				for _, task := range dep.Tasks {
					deprecatedTypes[task]++
				}
			}
		}

		mergeTotals(&report.Totals, tenant.Totals)
		report.Tenants = append(report.Tenants, tenant)

		for _, note := range scan.Notes {
			report.Notes = append(report.Notes, tenant.Tenant+": "+note)
		}
		for _, failure := range scan.Errors {
			report.Notes = append(report.Notes, tenant.Tenant+": "+failure)
		}
		if tenant.ParseFails > 0 {
			report.Notes = append(report.Notes, fmt.Sprintf("%s: %d flow source(s) could not be parsed as YAML; only text-based signals were counted for them",
				tenant.Tenant, tenant.ParseFails))
		}
	}

	report.Totals.TenantsScanned = len(scans)
	report.Totals.NamespacesCount = len(namespaceKeys)

	report.Signals.PluginDefaults.Flows = defaults.flows
	report.Signals.PluginDefaults.Entries = defaults.occurrences
	sort.Strings(defaults.refs)
	report.Signals.PluginDefaults.FlowRefs = defaults.refs
	report.Signals.TriggerConditions = triggerConditions.result()
	report.Signals.ConditionProperty = conditionProperty.result()
	report.Signals.PebbleJsonFunction = pebbleJSON.result()
	report.Signals.FsLocalDelete = fsLocalDelete.result()

	// Every removed task gets a row, including the ones nobody uses: "0 uses
	// of ForEach" is itself an answer for the migration plan.
	for _, class := range removedTaskClasses {
		if acc, ok := removed[class]; ok {
			report.Signals.RemovedTasks[class] = acc.result()
		} else {
			report.Signals.RemovedTasks[class] = signalCount{}
		}
	}

	for task, count := range deprecatedTypes {
		report.Signals.DeprecatedTaskTypes = append(report.Signals.DeprecatedTaskTypes, deprecatedTaskType{
			TaskType:    task.TaskType,
			Replacement: task.Replacement,
			Count:       count,
		})
	}
	sort.Slice(report.Signals.DeprecatedTaskTypes, func(i, j int) bool {
		left, right := report.Signals.DeprecatedTaskTypes[i], report.Signals.DeprecatedTaskTypes[j]
		if left.Count != right.Count {
			return left.Count > right.Count
		}
		if left.TaskType != right.TaskType {
			return left.TaskType < right.TaskType
		}
		return left.Replacement < right.Replacement
	})

	sort.Slice(report.Signals.ServerDeprecations, func(i, j int) bool {
		return report.Signals.ServerDeprecations[i].FlowRef < report.Signals.ServerDeprecations[j].FlowRef
	})

	return report
}

func newUsageTotals() usageTotals {
	return usageTotals{
		TaskTypeCount:        map[string]int64{},
		TaskTypeFlowCount:    map[string]int64{},
		TriggerTypeCount:     map[string]int64{},
		TriggerTypeFlowCount: map[string]int64{},
		PluginFamilyCount:    map[string]int64{},
	}
}

// mergeTotals folds one tenant's totals into the report-wide totals. The
// TenantsScanned and NamespacesCount fields are set by the caller, since they
// are counted across tenants.
func mergeTotals(into *usageTotals, from usageTotals) {
	into.Count += from.Count
	into.DisabledFlows += from.DisabledFlows
	into.FlowsWithInputs += from.FlowsWithInputs
	into.FlowsWithTriggers += from.FlowsWithTriggers
	into.SubflowTaskCount += from.SubflowTaskCount
	into.FlowsUsingSubflow += from.FlowsUsingSubflow
	mergeCounts(into.TaskTypeCount, from.TaskTypeCount)
	mergeCounts(into.TaskTypeFlowCount, from.TaskTypeFlowCount)
	mergeCounts(into.TriggerTypeCount, from.TriggerTypeCount)
	mergeCounts(into.TriggerTypeFlowCount, from.TriggerTypeFlowCount)
	mergeCounts(into.PluginFamilyCount, from.PluginFamilyCount)
}

func mergeCounts(into, from map[string]int64) {
	for key, count := range from {
		into[key] += count
	}
}

func sortedNamespaceReports(namespaces map[string]*namespaceReport) []namespaceReport {
	result := make([]namespaceReport, 0, len(namespaces))
	for _, ns := range namespaces {
		result = append(result, *ns)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].FlowCount != result[j].FlowCount {
			return result[i].FlowCount > result[j].FlowCount
		}
		return result[i].Namespace < result[j].Namespace
	})
	return result
}

// countEntry is one row of a count map, ready to render.
type countEntry struct {
	Name  string
	Count int64
}

// sortedCounts orders a count map by descending count, then by name, so the
// rendered report is deterministic.
func sortedCounts(counts map[string]int64) []countEntry {
	entries := make([]countEntry, 0, len(counts))
	for name, count := range counts {
		entries = append(entries, countEntry{Name: name, Count: count})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Count != entries[j].Count {
			return entries[i].Count > entries[j].Count
		}
		return entries[i].Name < entries[j].Name
	})
	return entries
}

// renderUsageReportMarkdown writes the report as plain GitHub-flavoured
// markdown, meant to be copy-pasted into an issue or a migration document.
func renderUsageReportMarkdown(report *usageReport, w io.Writer) error {
	out := &markdownWriter{w: w}

	out.printf("# Kestra usage report\n\n")
	out.printf("- Generated at: %s\n", report.GeneratedAt)
	out.printf("- kestractl version: %s\n", report.KestractlVersion)
	kestraVersion := report.KestraVersion
	if kestraVersion == "" {
		kestraVersion = "unknown"
	}
	out.printf("- Kestra version: %s\n", kestraVersion)
	out.printf("- Scope: %s\n", report.Scope)
	out.printf("- Tenants scanned: %d\n", report.Totals.TenantsScanned)
	if report.Anonymized {
		out.printf("- Anonymization: on — tenant, namespace and flow names are replaced by stable hashes (`t-`/`ns-`/`f-`). Plugin types and counts are unchanged. Re-run with `--anonymize=false` to see real names.\n")
	} else {
		out.printf("- Anonymization: off — this report contains real tenant, namespace and flow names.\n")
	}
	out.printf("\nThis report is built from flow sources only: no execution, log or database data is read. ")
	out.printf("Namespace-level plugin defaults are not part of a flow source and are therefore not visible here.\n\n")

	renderSignalsSection(out, report)
	renderPluginDefaultsSection(out, report)
	renderDeprecatedTaskTypesSection(out, report)
	renderAffectedFlowsSection(out, report)
	renderInventorySection(out, report)
	renderNotesSection(out, report)

	return out.err
}

func renderSignalsSection(out *markdownWriter, report *usageReport) {
	signals := report.Signals

	out.printf("## Migration signals\n\n")
	out.printf("| Signal | Occurrences | Flows | Kestra 2.0 impact |\n")
	out.printf("| --- | ---: | ---: | --- |\n")
	out.printf("| pluginDefaults | %d | %d | Removed in 2.0 — move defaults into each task |\n",
		signals.PluginDefaults.Entries, signals.PluginDefaults.Flows)
	for _, class := range removedTaskClasses {
		signal := signals.RemovedTasks[class]
		out.printf("| Task %s | %d | %d | Removed or reworked in 2.0 |\n", class, signal.Occurrences, signal.Flows)
	}
	out.printf("| Trigger conditions/preconditions | %d | %d | Replaced by `when` |\n",
		signals.TriggerConditions.Occurrences, signals.TriggerConditions.Flows)
	out.printf("| `condition` property | %d | %d | Replaced by `when` |\n",
		signals.ConditionProperty.Occurrences, signals.ConditionProperty.Flows)
	out.printf("| Pebble `json()` | %d | %d | Removed — use `fromJson()`/`toJson()` (text heuristic) |\n",
		signals.PebbleJsonFunction.Occurrences, signals.PebbleJsonFunction.Flows)
	out.printf("| `fs.local.Delete` | %d | %d | `recursive` default changed in 2.0 |\n",
		signals.FsLocalDelete.Occurrences, signals.FsLocalDelete.Flows)
	if signals.ServerDeprecationsAvailable {
		deprecatedTasks := int64(0)
		for _, dep := range signals.ServerDeprecations {
			deprecatedTasks += int64(dep.DeprecatedTasks)
		}
		out.printf("| Server-reported deprecations | %d | %d | Reported by the Kestra instance itself |\n",
			deprecatedTasks, len(signals.ServerDeprecations))
	} else {
		out.printf("| Server-reported deprecations | n/a | n/a | The deprecation endpoint could not be read |\n")
	}
	out.printf("\n")
}

func renderPluginDefaultsSection(out *markdownWriter, report *usageReport) {
	defaults := report.Signals.PluginDefaults

	out.printf("## pluginDefaults detail\n\n")
	out.printf("- Flows declaring pluginDefaults: %d\n", defaults.Flows)
	out.printf("- Total entries: %d (of which forced: %d)\n\n", defaults.Entries, defaults.ForcedEntries)
	if len(defaults.TypeCount) == 0 {
		out.printf("No pluginDefaults entries found.\n\n")
		return
	}

	out.printf("| Default type | Entries |\n| --- | ---: |\n")
	for _, entry := range sortedCounts(defaults.TypeCount) {
		out.printf("| `%s` | %d |\n", entry.Name, entry.Count)
	}
	out.printf("\n")
}

// renderDeprecatedTaskTypesSection lists the deprecations the instance itself
// reported, aggregated by plugin type. Task ids are deliberately not part of
// the report.
func renderDeprecatedTaskTypesSection(out *markdownWriter, report *usageReport) {
	if !report.Signals.ServerDeprecationsAvailable {
		return
	}

	out.printf("## Deprecated task types (server-reported)\n\n")
	if len(report.Signals.DeprecatedTaskTypes) == 0 {
		out.printf("The instance reported no deprecated task.\n\n")
		return
	}

	out.printf("| Task type | Replacement | Count |\n| --- | --- | ---: |\n")
	for _, entry := range report.Signals.DeprecatedTaskTypes {
		replacement := "-"
		if entry.Replacement != "" {
			replacement = "`" + entry.Replacement + "`"
		}
		out.printf("| `%s` | %s | %d |\n", entry.TaskType, replacement, entry.Count)
	}
	out.printf("\n")
}

func renderAffectedFlowsSection(out *markdownWriter, report *usageReport) {
	out.printf("## Affected flows\n\n")

	rendered := false
	render := func(title string, refs []string) {
		if len(refs) == 0 {
			return
		}
		rendered = true
		out.printf("### %s\n\n", title)
		for _, ref := range refs {
			out.printf("- `%s`\n", ref)
		}
		if len(refs) == maxFlowRefs {
			out.printf("\n_Truncated at %d flows._\n", maxFlowRefs)
		}
		out.printf("\n")
	}

	render("pluginDefaults", report.Signals.PluginDefaults.FlowRefs)
	for _, class := range removedTaskClasses {
		render("Task "+class, report.Signals.RemovedTasks[class].FlowRefs)
	}
	render("Trigger conditions/preconditions", report.Signals.TriggerConditions.FlowRefs)
	render("`condition` property", report.Signals.ConditionProperty.FlowRefs)
	render("Pebble `json()`", report.Signals.PebbleJsonFunction.FlowRefs)
	render("`fs.local.Delete`", report.Signals.FsLocalDelete.FlowRefs)

	if len(report.Signals.ServerDeprecations) > 0 {
		rendered = true
		out.printf("### Server-reported deprecations\n\n")
		for _, dep := range report.Signals.ServerDeprecations {
			out.printf("- `%s` (%d deprecated task(s))\n", dep.FlowRef, dep.DeprecatedTasks)
		}
		out.printf("\n")
	}

	if !rendered {
		out.printf("No flow matched any migration signal.\n\n")
	}
}

func renderInventorySection(out *markdownWriter, report *usageReport) {
	totals := report.Totals

	out.printf("## Inventory\n\n")
	out.printf("| Metric | Value |\n| --- | ---: |\n")
	out.printf("| Tenants scanned | %d |\n", totals.TenantsScanned)
	out.printf("| Flows | %d |\n", totals.Count)
	out.printf("| Namespaces | %d |\n", totals.NamespacesCount)
	out.printf("| Disabled flows | %d |\n", totals.DisabledFlows)
	out.printf("| Flows with inputs | %d |\n", totals.FlowsWithInputs)
	out.printf("| Flows with triggers | %d |\n", totals.FlowsWithTriggers)
	out.printf("| Subflow tasks | %d |\n", totals.SubflowTaskCount)
	out.printf("| Flows calling a subflow | %d |\n", totals.FlowsUsingSubflow)
	out.printf("\n")

	out.printf("### Flows per namespace\n\n")
	out.printf("| Tenant | Namespace | Flows | Disabled | With triggers |\n| --- | --- | ---: | ---: | ---: |\n")
	for _, tenant := range report.Tenants {
		for _, ns := range tenant.Namespaces {
			out.printf("| `%s` | `%s` | %d | %d | %d |\n",
				tenant.Tenant, ns.Namespace, ns.FlowCount, ns.DisabledFlows, ns.FlowsWithTriggers)
		}
	}
	out.printf("\n")

	renderCountTable(out, "Task types", "Task type", totals.TaskTypeCount, totals.TaskTypeFlowCount)
	renderCountTable(out, "Trigger types", "Trigger type", totals.TriggerTypeCount, totals.TriggerTypeFlowCount)

	out.printf("### Plugin families\n\n")
	if len(totals.PluginFamilyCount) == 0 {
		out.printf("No plugin usage found.\n\n")
		return
	}
	out.printf("| Plugin family | Uses |\n| --- | ---: |\n")
	for _, entry := range sortedCounts(totals.PluginFamilyCount) {
		out.printf("| `%s` | %d |\n", entry.Name, entry.Count)
	}
	out.printf("\n")
}

// renderCountTable renders one "type → uses / flows" inventory table.
func renderCountTable(out *markdownWriter, title, column string, counts, flowCounts map[string]int64) {
	out.printf("### %s\n\n", title)
	if len(counts) == 0 {
		out.printf("None found.\n\n")
		return
	}

	out.printf("| %s | Uses | Flows |\n| --- | ---: | ---: |\n", column)
	for _, entry := range sortedCounts(counts) {
		out.printf("| `%s` | %d | %d |\n", entry.Name, entry.Count, flowCounts[entry.Name])
	}
	out.printf("\n")
}

func renderNotesSection(out *markdownWriter, report *usageReport) {
	out.printf("## Scan notes\n\n")
	if len(report.Notes) == 0 {
		out.printf("- The scan completed without fallbacks or errors.\n")
	}
	for _, note := range report.Notes {
		out.printf("- %s\n", note)
	}
	out.printf("- Namespace-level plugin defaults are stored outside flow sources and are not covered by this report.\n")
	out.printf("- The Pebble `json()` count is a text heuristic over the flow source.\n")
}

// markdownWriter accumulates the first write error so the renderer does not
// have to check every single Fprintf.
type markdownWriter struct {
	w   io.Writer
	err error
}

func (m *markdownWriter) printf(format string, args ...any) {
	if m.err != nil {
		return
	}
	_, m.err = fmt.Fprintf(m.w, format, args...)
}

// maxZipEntrySize bounds a single flow source read out of an export archive.
// A flow YAML is a few kilobytes; anything larger is not a flow and is skipped
// rather than buffered.
const maxZipEntrySize = 10 << 20 // 10 MiB

// flowsFromZip reads the YAML sources out of a flow export archive entirely in
// memory — the archive never touches disk. It returns the sources plus the
// number of entries that were skipped (oversized or unreadable).
func flowsFromZip(data []byte) ([]string, int, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read the flow export archive: %w", err)
	}

	sources := make([]string, 0, len(reader.File))
	skipped := 0

	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() || !isYAMLEntry(entry.Name) {
			continue
		}
		if entry.UncompressedSize64 > maxZipEntrySize {
			skipped++
			continue
		}

		source, err := readZipEntry(entry)
		if err != nil {
			skipped++
			continue
		}
		sources = append(sources, source)
	}

	return sources, skipped, nil
}

func readZipEntry(entry *zip.File) (string, error) {
	file, err := entry.Open()
	if err != nil {
		return "", err
	}
	defer file.Close()

	content, err := io.ReadAll(io.LimitReader(file, maxZipEntrySize))
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func isYAMLEntry(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".yml") || strings.HasSuffix(lower, ".yaml")
}
