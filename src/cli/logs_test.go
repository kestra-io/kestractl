package cli

import (
	"errors"
	"strings"
	"testing"
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
