package cli

import (
	"errors"
	"strings"
	"testing"

	apiclient "github.com/kestra-io/kestra-cli/src/api_client"
)

type fakeExecutionsService struct {
	killFn         func(state []string, namespace, flowID, tenant string, ctx *apiclient.AuthContext) (map[string]any, error)
	triggerFn      func(namespace, flowID string, wait bool, inputs map[string]any, tenant string, ctx *apiclient.AuthContext) (map[string]any, error)
	getExecutionFn func(executionID, tenant string, ctx *apiclient.AuthContext) (map[string]any, error)
}

func (f *fakeExecutionsService) KillByQuery(state []string, namespace, flowID, tenant string, ctx *apiclient.AuthContext) (map[string]any, error) {
	if f.killFn == nil {
		return nil, errors.New("kill not implemented")
	}
	return f.killFn(state, namespace, flowID, tenant, ctx)
}

func (f *fakeExecutionsService) TriggerExecution(namespace, flowID string, wait bool, inputs map[string]any, tenant string, ctx *apiclient.AuthContext) (map[string]any, error) {
	if f.triggerFn == nil {
		return nil, errors.New("trigger not implemented")
	}
	return f.triggerFn(namespace, flowID, wait, inputs, tenant, ctx)
}

func (f *fakeExecutionsService) GetExecution(executionID, tenant string, ctx *apiclient.AuthContext) (map[string]any, error) {
	if f.getExecutionFn == nil {
		return nil, errors.New("get execution not implemented")
	}
	return f.getExecutionFn(executionID, tenant, ctx)
}

func TestExecutionsRunCommand_Success(t *testing.T) {
	fake := &fakeExecutionsService{
		triggerFn: func(namespace, flowID string, wait bool, inputs map[string]any, tenant string, ctx *apiclient.AuthContext) (map[string]any, error) {
			if namespace != "test.namespace" {
				t.Fatalf("expected namespace 'test.namespace', got '%s'", namespace)
			}
			if flowID != "test-flow" {
				t.Fatalf("expected flowID 'test-flow', got '%s'", flowID)
			}
			if wait {
				t.Fatalf("wait should default to false")
			}
			return map[string]any{
				"id":        "exec-123",
				"flowId":    "test-flow",
				"namespace": "test.namespace",
				"state": map[string]any{
					"current": "CREATED",
				},
			}, nil
		},
	}

	cmd := newExecutionsRunCommand(fake)

	output, err := executeCommand(cmd, "test.namespace", "test-flow")
	if err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}

	if !strings.Contains(output, "Execution triggered successfully!") {
		t.Fatalf("expected success message, got: %s", output)
	}

	if !strings.Contains(output, "Execution ID: exec-123") {
		t.Fatalf("expected Execution ID in output, got: %s", output)
	}
}

func TestExecutionsRunCommand_ServiceError(t *testing.T) {
	expectedErr := errors.New("trigger failed")

	fake := &fakeExecutionsService{
		triggerFn: func(namespace, flowID string, wait bool, inputs map[string]any, tenant string, ctx *apiclient.AuthContext) (map[string]any, error) {
			return nil, expectedErr
		},
	}

	cmd := newExecutionsRunCommand(fake)

	_, err := executeCommand(cmd, "test.namespace", "test-flow")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
}

func TestExecutionsGetCommand_Success(t *testing.T) {
	fake := &fakeExecutionsService{
		getExecutionFn: func(executionID, tenant string, ctx *apiclient.AuthContext) (map[string]any, error) {
			if executionID != "exec-123" {
				t.Fatalf("expected executionID 'exec-123', got '%s'", executionID)
			}
			return map[string]any{
				"id":           "exec-123",
				"flowId":       "test-flow",
				"namespace":    "test.namespace",
				"flowRevision": "1",
				"state": map[string]any{
					"current": "SUCCESS",
				},
				"url": "http://localhost:8080/ui/main/executions/test.namespace/test-flow/exec-123",
			}, nil
		},
	}

	cmd := newExecutionsGetCommand(fake)

	output, err := executeCommand(cmd, "exec-123")
	if err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}

	if !strings.Contains(output, "Execution Details") {
		t.Fatalf("expected 'Execution Details' in output, got: %s", output)
	}

	if !strings.Contains(output, "Execution ID: exec-123") {
		t.Fatalf("expected Execution ID in output, got: %s", output)
	}
}

func TestExecutionsKillCommand_Success(t *testing.T) {
	fake := &fakeExecutionsService{
		killFn: func(state []string, namespace, flowID, tenant string, ctx *apiclient.AuthContext) (map[string]any, error) {
			if len(state) != 1 || state[0] != "RUNNING" {
				t.Fatalf("expected state ['RUNNING'], got %v", state)
			}
			return map[string]any{
				"count": float64(5),
			}, nil
		},
	}

	cmd := newExecutionsKillCommand(fake)

	output, err := executeCommand(cmd)
	if err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}

	if !strings.Contains(output, "Kill request sent successfully!") {
		t.Fatalf("expected success message, got: %s", output)
	}

	if !strings.Contains(output, "Executions killed: 5") {
		t.Fatalf("expected kill count in output, got: %s", output)
	}
}

