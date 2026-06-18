package cli

import (
	"errors"
	"strings"
	"testing"

	kestra "github.com/kestra-io/client-sdk/go-sdk/kestra_api_client"
)

func TestLogsListCommand_NoArgs(t *testing.T) {
	cmd := newLogsListCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestLogsListCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newLogsListCommand()
	_, err := executeCommand(cmd, "exec-123")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestLogsSearchCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newLogsSearchCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestLogsDeleteCommand_NoArgs(t *testing.T) {
	cmd := newLogsDeleteCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestLogsDeleteCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newLogsDeleteCommand()
	_, err := executeCommand(cmd, "exec-123")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestLogsDeleteFlowCommand_NoArgs(t *testing.T) {
	cmd := newLogsDeleteFlowCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 2 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestLogsDeleteFlowCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newLogsDeleteFlowCommand()
	_, err := executeCommand(cmd, "my.ns", "my-flow")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestBuildLogSearchFilters(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		filters := buildLogSearchFilters("", "", "", "", "")
		if len(filters) != 0 {
			t.Fatalf("expected no filters, got %d", len(filters))
		}
	})

	t.Run("all filters with upper-cased level", func(t *testing.T) {
		filters := buildLogSearchFilters("boom", "my.ns", "my-flow", "trg-1", "warn")
		if len(filters) != 5 {
			t.Fatalf("expected 5 filters, got %d", len(filters))
		}
		got := map[kestra.SearchFilterField]any{}
		for _, f := range filters {
			if f.Operation != kestra.OpEquals {
				t.Errorf("expected EQUALS op, got %v", f.Operation)
			}
			got[f.Field] = f.Value
		}
		if got[kestra.FilterQuery] != "boom" {
			t.Errorf("query: got %v", got[kestra.FilterQuery])
		}
		if got[kestra.FilterNamespace] != "my.ns" {
			t.Errorf("namespace: got %v", got[kestra.FilterNamespace])
		}
		if got[kestra.FilterMinLevel] != "WARN" {
			t.Errorf("expected upper-cased WARN, got %v", got[kestra.FilterMinLevel])
		}
	})
}

func TestLogFilterOptions(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		f := logFilterOptions("", "", "", false, 0)
		if f.minLevel != nil || f.taskRunID != nil || f.taskID != nil || f.attempt != nil {
			t.Fatalf("expected all-nil filter, got %+v", f)
		}
	})

	t.Run("populated", func(t *testing.T) {
		f := logFilterOptions("WARN", "tr-1", "task-1", true, 2)
		if f.minLevel == nil || *f.minLevel != "WARN" {
			t.Errorf("expected minLevel WARN, got %v", f.minLevel)
		}
		if f.taskRunID == nil || *f.taskRunID != "tr-1" {
			t.Errorf("expected taskRunID tr-1, got %v", f.taskRunID)
		}
		if f.taskID == nil || *f.taskID != "task-1" {
			t.Errorf("expected taskID task-1, got %v", f.taskID)
		}
		if f.attempt == nil || *f.attempt != 2 {
			t.Errorf("expected attempt 2, got %v", f.attempt)
		}
	})

	t.Run("attempt zero not set", func(t *testing.T) {
		f := logFilterOptions("", "", "", false, 0)
		if f.attempt != nil {
			t.Errorf("expected attempt nil when not changed, got %v", *f.attempt)
		}
	})
}
