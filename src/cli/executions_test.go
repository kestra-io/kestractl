package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	kestra "github.com/kestra-io/client-sdk/go-sdk/kestra_api_client"
)

func TestExecutionsRunCommand_NoArgs(t *testing.T) {
	cmd := newExecutionsRunCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 2 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestExecutionsRunCommand_OneArg(t *testing.T) {
	cmd := newExecutionsRunCommand()
	_, err := executeCommand(cmd, "namespace")
	if err == nil {
		t.Fatal("expected error when only 1 arg provided")
	}
	if !strings.Contains(err.Error(), "accepts 2 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestExecutionsGetCommand_NoArgs(t *testing.T) {
	cmd := newExecutionsGetCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestExecutionsListCommand_RejectsArgs(t *testing.T) {
	cmd := newExecutionsListCommand()
	_, err := executeCommand(cmd, "unexpected")
	if err == nil {
		t.Fatal("expected error when a positional arg is provided")
	}
	if !strings.Contains(err.Error(), "unknown command") && !strings.Contains(err.Error(), "accepts 0 arg") {
		t.Fatalf("expected no-args error, got: %v", err)
	}
}

func TestExecutionsListCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newExecutionsListCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestExecutionsKillCommand_NoArgs(t *testing.T) {
	cmd := newExecutionsKillCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestExecutionsKillCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newExecutionsKillCommand()
	_, err := executeCommand(cmd, "exec-123")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestExecutionsRestartCommand_NoArgs(t *testing.T) {
	cmd := newExecutionsRestartCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestExecutionsRestartCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newExecutionsRestartCommand()
	_, err := executeCommand(cmd, "exec-123")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestExecutionsResumeCommand_NoArgs(t *testing.T) {
	cmd := newExecutionsResumeCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestExecutionsResumeCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newExecutionsResumeCommand()
	_, err := executeCommand(cmd, "exec-123")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestExecutionsPauseCommand_NoArgs(t *testing.T) {
	cmd := newExecutionsPauseCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestExecutionsPauseCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newExecutionsPauseCommand()
	_, err := executeCommand(cmd, "exec-123")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestExecutionsForceRunCommand_NoArgs(t *testing.T) {
	cmd := newExecutionsForceRunCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestExecutionsForceRunCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newExecutionsForceRunCommand()
	_, err := executeCommand(cmd, "exec-123")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestExecutionsUnqueueCommand_NoArgs(t *testing.T) {
	cmd := newExecutionsUnqueueCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestExecutionsUnqueueCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newExecutionsUnqueueCommand()
	_, err := executeCommand(cmd, "exec-123")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestExecutionsDeleteCommand_NoArgs(t *testing.T) {
	cmd := newExecutionsDeleteCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestExecutionsDeleteCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newExecutionsDeleteCommand()
	_, err := executeCommand(cmd, "exec-123")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestExecutionsReplayCommand_NoArgs(t *testing.T) {
	cmd := newExecutionsReplayCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestExecutionsReplayCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newExecutionsReplayCommand()
	_, err := executeCommand(cmd, "exec-123")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestExecutionToMap(t *testing.T) {
	exec := kestra.NewExecutionWithDefaults()
	exec.SetId("exec-1")
	exec.SetFlowId("my-flow")
	exec.SetNamespace("my.ns")
	exec.SetFlowRevision(2)
	st := kestra.NewStateWithDefaults()
	st.SetCurrent(kestra.STATETYPE_SUCCESS)
	exec.SetState(*st)

	m := executionToMap(exec)
	if m["id"] != "exec-1" {
		t.Errorf("expected id exec-1, got %v", m["id"])
	}
	if m["flowId"] != "my-flow" {
		t.Errorf("expected flowId my-flow, got %v", m["flowId"])
	}
	if m["namespace"] != "my.ns" {
		t.Errorf("expected namespace my.ns, got %v", m["namespace"])
	}
	state, ok := m["state"].(map[string]any)
	if !ok {
		t.Fatalf("expected state map, got %T", m["state"])
	}
	if state["current"] != kestra.STATETYPE_SUCCESS {
		t.Errorf("expected SUCCESS state, got %v", state["current"])
	}
}

func TestBuildExecutionFilters(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		flowID    string
		state     string
		wantLen   int
		wantField map[kestra.QueryFilterField]string
	}{
		{name: "empty", wantLen: 0, wantField: map[kestra.QueryFilterField]string{}},
		{
			name:      "namespace only",
			namespace: "my.ns",
			wantLen:   1,
			wantField: map[kestra.QueryFilterField]string{kestra.QUERYFILTERFIELD_NAMESPACE: "my.ns"},
		},
		{
			name:      "all filters",
			namespace: "my.ns",
			flowID:    "my-flow",
			state:     "failed",
			wantLen:   3,
			wantField: map[kestra.QueryFilterField]string{
				kestra.QUERYFILTERFIELD_NAMESPACE: "my.ns",
				kestra.QUERYFILTERFIELD_FLOW_ID:   "my-flow",
				kestra.QUERYFILTERFIELD_STATE:     "FAILED", // upper-cased
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filters := buildExecutionFilters(tt.namespace, tt.flowID, tt.state)
			if len(filters) != tt.wantLen {
				t.Fatalf("expected %d filters, got %d", tt.wantLen, len(filters))
			}
			for _, f := range filters {
				if f.GetOperation() != kestra.QUERYFILTEROP_EQUALS {
					t.Errorf("expected EQUALS operation, got %v", f.GetOperation())
				}
				want, ok := tt.wantField[f.GetField()]
				if !ok {
					t.Errorf("unexpected filter field %v", f.GetField())
					continue
				}
				if got, _ := f.Value.(string); got != want {
					t.Errorf("field %v: expected value %q, got %q", f.GetField(), want, got)
				}
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		input    any
		expected string
	}{
		{input: "PT5S", expected: "5.00s"},
		{input: "PT5.123S", expected: "5.12s"},
		{input: "PT0.5S", expected: "0.50s"},
		{input: float64(5000), expected: "5.00s"},
		{input: float64(1234), expected: "1.23s"},
		{input: "not-iso", expected: "not-iso"},
		{input: "plain string", expected: "plain string"},
		{input: 42, expected: "42"},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := formatDuration(tt.input)
			if result != tt.expected {
				t.Errorf("formatDuration(%v) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseISO8601Duration(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		wantErr  bool
	}{
		{input: "PT5S", expected: "5.00s"},
		{input: "PT5.123S", expected: "5.12s"},
		{input: "PT0.5S", expected: "0.50s"},
		{input: "PT123S", expected: "123.00s"},
		{input: "PT0.001S", expected: "0.00s"},
		{input: "invalid", wantErr: true},
		{input: "PT", wantErr: true},
		{input: "PTS", wantErr: true},
		{input: "5S", wantErr: true},
		{input: "PT5M", wantErr: true}, // Minutes not supported
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := parseISO8601Duration(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseISO8601Duration(%s) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("parseISO8601Duration(%s) unexpected error: %v", tt.input, err)
				return
			}
			if result != tt.expected {
				t.Errorf("parseISO8601Duration(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestPrintExecutionState(t *testing.T) {
	// Test with valid state
	execution := map[string]any{
		"state": map[string]any{
			"current":   "SUCCESS",
			"startDate": "2024-01-15T10:00:00Z",
			"endDate":   "2024-01-15T10:00:05Z",
			"duration":  "PT5S",
		},
	}

	// This should not panic
	var buf bytes.Buffer
	printExecutionState(&buf, execution, true)
	output := buf.String()

	if !strings.Contains(output, "State: SUCCESS") {
		t.Errorf("expected state output, got: %s", output)
	}
}

func TestPrintExecutionState_Unknown(t *testing.T) {
	// Test with missing state
	execution := map[string]any{}

	var buf bytes.Buffer
	printExecutionState(&buf, execution, false)
	output := buf.String()

	if !strings.Contains(output, "State: unknown") {
		t.Errorf("expected 'unknown' state, got: %s", output)
	}
}

func TestExecutionsSetLabelsCommand_NotEnoughArgs(t *testing.T) {
	cmd := newExecutionsSetLabelsCommand()
	_, err := executeCommand(cmd, "exec-123")
	if err == nil {
		t.Fatal("expected error when not enough args provided")
	}
	if !strings.Contains(err.Error(), "requires at least 2 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestExecutionsSetLabelsCommand_InvalidLabel(t *testing.T) {
	cmd := newExecutionsSetLabelsCommand()
	_, err := executeCommand(cmd, "exec-123", "noequalsign")
	if err == nil {
		t.Fatal("expected error for malformed label")
	}
	if !strings.Contains(err.Error(), "invalid label") {
		t.Fatalf("expected invalid label error, got: %v", err)
	}
}

func TestExecutionsSetLabelsCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newExecutionsSetLabelsCommand()
	_, err := executeCommand(cmd, "exec-123", "env=prod")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestParseLabels(t *testing.T) {
	t.Run("valid pairs", func(t *testing.T) {
		labels, err := parseLabels([]string{"env=prod", "team=data"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(labels) != 2 {
			t.Fatalf("expected 2 labels, got %d", len(labels))
		}
		if labels[0].GetKey() != "env" || labels[0].GetValue() != "prod" {
			t.Errorf("unexpected first label: %+v", labels[0])
		}
	})

	t.Run("empty value allowed", func(t *testing.T) {
		labels, err := parseLabels([]string{"key="})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if labels[0].GetKey() != "key" || labels[0].GetValue() != "" {
			t.Errorf("unexpected label: %+v", labels[0])
		}
	})

	t.Run("missing separator", func(t *testing.T) {
		if _, err := parseLabels([]string{"bad"}); err == nil {
			t.Fatal("expected error for missing separator")
		}
	})

	t.Run("empty key", func(t *testing.T) {
		if _, err := parseLabels([]string{"=value"}); err == nil {
			t.Fatal("expected error for empty key")
		}
	})
}

func TestExecutionsFlowGraphCommand_NoArgs(t *testing.T) {
	cmd := newExecutionsFlowGraphCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestExecutionsFlowGraphCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newExecutionsFlowGraphCommand()
	_, err := executeCommand(cmd, "exec-123")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestEdgeRelationValue(t *testing.T) {
	t.Run("nil relation value", func(t *testing.T) {
		e := *kestra.NewFlowGraphEdge()
		if got := edgeRelationValue(e); got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})

	t.Run("populated relation value", func(t *testing.T) {
		e := *kestra.NewFlowGraphEdge()
		rel := kestra.NewRelation1()
		v := "ERROR"
		rel.Value = &v
		e.SetRelation(*rel)
		if got := edgeRelationValue(e); got != "ERROR" {
			t.Errorf("expected ERROR, got %q", got)
		}
	})
}

func TestExecutionsLatestCommand_NoFlows(t *testing.T) {
	cmd := newExecutionsLatestCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no --flow provided")
	}
	if !strings.Contains(err.Error(), "at least one --flow") {
		t.Fatalf("expected flow-required error, got: %v", err)
	}
}

func TestExecutionsLatestCommand_InvalidFlow(t *testing.T) {
	cmd := newExecutionsLatestCommand()
	_, err := executeCommand(cmd, "--flow", "noseparator")
	if err == nil {
		t.Fatal("expected error for malformed flow reference")
	}
	if !strings.Contains(err.Error(), "invalid flow reference") {
		t.Fatalf("expected invalid flow reference error, got: %v", err)
	}
}

func TestExecutionsLatestCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newExecutionsLatestCommand()
	_, err := executeCommand(cmd, "--flow", "my.ns:my-flow")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestParseFlowRefs(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		filters, err := parseFlowRefs([]string{"company.team:daily", "company.team:hourly"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(filters) != 2 {
			t.Fatalf("expected 2 filters, got %d", len(filters))
		}
		if filters[0].GetNamespace() != "company.team" || filters[0].GetId() != "daily" {
			t.Errorf("unexpected first filter: %+v", filters[0])
		}
	})

	t.Run("missing separator", func(t *testing.T) {
		if _, err := parseFlowRefs([]string{"bad"}); err == nil {
			t.Fatal("expected error for missing separator")
		}
	})

	t.Run("empty id", func(t *testing.T) {
		if _, err := parseFlowRefs([]string{"ns:"}); err == nil {
			t.Fatal("expected error for empty id")
		}
	})
}

func TestExecutionsChangeStatusCommand_NoArgs(t *testing.T) {
	cmd := newExecutionsChangeStatusCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 2 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestExecutionsChangeStatusCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newExecutionsChangeStatusCommand()
	_, err := executeCommand(cmd, "exec-123", "SUCCESS")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestExecutionsSearchByFlowCommand_MissingFlags(t *testing.T) {
	cmd := newExecutionsSearchByFlowCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error for missing required flags")
	}
	if !strings.Contains(err.Error(), "namespace") && !strings.Contains(err.Error(), "required") {
		t.Fatalf("expected missing flag error, got: %v", err)
	}
}

func TestExecutionsSearchByFlowCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newExecutionsSearchByFlowCommand()
	_, err := executeCommand(cmd, "--namespace", "my.ns", "--flow-id", "my-flow")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestExecutionsUpdateTaskRunCommand_NoArgs(t *testing.T) {
	cmd := newExecutionsUpdateTaskRunCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 3 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestExecutionsUpdateTaskRunCommand_NotEnoughArgs(t *testing.T) {
	cmd := newExecutionsUpdateTaskRunCommand()
	_, err := executeCommand(cmd, "exec-123", "taskrun-456")
	if err == nil {
		t.Fatal("expected error when only 2 args provided")
	}
	if !strings.Contains(err.Error(), "accepts 3 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestExecutionsUpdateTaskRunCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newExecutionsUpdateTaskRunCommand()
	_, err := executeCommand(cmd, "exec-123", "taskrun-456", "SUCCESS")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestExecutionsBulkKillCommand_NoArgs(t *testing.T) {
	cmd := newExecutionsBulkKillCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "requires at least 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestExecutionsBulkKillCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newExecutionsBulkKillCommand()
	_, err := executeCommand(cmd, "id1", "id2")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestExecutionsBulkDeleteCommand_NoArgs(t *testing.T) {
	cmd := newExecutionsBulkDeleteCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
}

func TestExecutionsBulkRestartCommand_NoArgs(t *testing.T) {
	cmd := newExecutionsBulkRestartCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
}

func TestExecutionsBulkReplayCommand_NoArgs(t *testing.T) {
	cmd := newExecutionsBulkReplayCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
}

func TestExecutionsBulkPauseCommand_NoArgs(t *testing.T) {
	cmd := newExecutionsBulkPauseCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
}

func TestExecutionsBulkResumeCommand_NoArgs(t *testing.T) {
	cmd := newExecutionsBulkResumeCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
}

func TestExecutionsBulkForceRunCommand_NoArgs(t *testing.T) {
	cmd := newExecutionsBulkForceRunCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
}

func TestExecutionsBulkSetLabelsCommand_NoArgs(t *testing.T) {
	cmd := newExecutionsBulkSetLabelsCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
}

func TestExecutionsBulkSetLabelsCommand_NoIds(t *testing.T) {
	cmd := newExecutionsBulkSetLabelsCommand()
	_, err := executeCommand(cmd, "env=prod")
	if err == nil {
		t.Fatal("expected error when no --ids provided")
	}
	if !strings.Contains(err.Error(), "ids") {
		t.Fatalf("expected ids error, got: %v", err)
	}
}

func TestExecutionsBulkUnqueueCommand_NoArgs(t *testing.T) {
	cmd := newExecutionsBulkUnqueueCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
}

func TestExecutionsBulkUnqueueCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newExecutionsBulkUnqueueCommand()
	_, err := executeCommand(cmd, "id1", "id2")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestExecutionsReplayByQueryCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newExecutionsReplayByQueryCommand()
	_, err := executeCommand(cmd, "--namespace", "qa.test")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestExecutionsKillByQueryCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newExecutionsKillByQueryCommand()
	_, err := executeCommand(cmd, "--namespace", "qa.test")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestExecutionsPauseByQueryCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newExecutionsPauseByQueryCommand()
	_, err := executeCommand(cmd, "--namespace", "qa.test")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestExecutionsResumeByQueryCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newExecutionsResumeByQueryCommand()
	_, err := executeCommand(cmd, "--namespace", "qa.test")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestExecutionsRestartByQueryCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newExecutionsRestartByQueryCommand()
	_, err := executeCommand(cmd, "--namespace", "qa.test")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestExecutionsForceRunByQueryCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newExecutionsForceRunByQueryCommand()
	_, err := executeCommand(cmd, "--namespace", "qa.test")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}
