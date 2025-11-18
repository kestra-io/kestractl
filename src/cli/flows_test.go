package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apiclient "github.com/kestra-io/kestra-cli/src/api_client"
	"github.com/spf13/cobra"
)

type fakeFlowsService struct {
	listFn   func(namespace, tenant string, ctx *apiclient.AuthContext) ([]map[string]any, error)
	getFn    func(namespace, flowID, tenant string, ctx *apiclient.AuthContext) (map[string]any, error)
	createFn func(yamlContent string, tenant string, ctx *apiclient.AuthContext, override bool) (map[string]any, error)
}

func (f *fakeFlowsService) ListFlows(namespace, tenant string, ctx *apiclient.AuthContext) ([]map[string]any, error) {
	if f.listFn == nil {
		return nil, errors.New("list not implemented")
	}
	return f.listFn(namespace, tenant, ctx)
}

func (f *fakeFlowsService) GetFlow(namespace, flowID, tenant string, ctx *apiclient.AuthContext) (map[string]any, error) {
	if f.getFn == nil {
		return nil, errors.New("get not implemented")
	}
	return f.getFn(namespace, flowID, tenant, ctx)
}

func (f *fakeFlowsService) CreateFlow(yamlContent string, tenant string, ctx *apiclient.AuthContext, override bool) (map[string]any, error) {
	if f.createFn == nil {
		return nil, errors.New("create not implemented")
	}
	return f.createFn(yamlContent, tenant, ctx, override)
}

func TestFlowsDeployCommand_Success(t *testing.T) {
	fixturePath := filepath.Join("testdata", "flow.yaml")

	fake := &fakeFlowsService{
		createFn: func(yamlContent string, tenant string, ctx *apiclient.AuthContext, override bool) (map[string]any, error) {
			if !strings.Contains(yamlContent, "namespace: test.namespace") {
				t.Fatalf("expected YAML content to contain namespace, got:\n%s", yamlContent)
			}
			if override {
				t.Fatalf("override should default to false")
			}
			return map[string]any{
				"id":        "test-flow",
				"namespace": "test.namespace",
				"revision":  "1",
			}, nil
		},
	}

	cmd := newFlowsDeployCommand(fake)

	output, err := executeCommand(cmd, fixturePath)
	if err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}

	if !strings.Contains(output, "Flow deployed successfully!") {
		t.Fatalf("expected success message, got: %s", output)
	}

	if !strings.Contains(output, "Flow ID: test-flow") {
		t.Fatalf("expected Flow ID in output, got: %s", output)
	}
}

func TestFlowsDeployCommand_ServiceError(t *testing.T) {
	fixturePath := filepath.Join("testdata", "flow.yaml")
	expectedErr := errors.New("boom")

	fake := &fakeFlowsService{
		createFn: func(yamlContent string, tenant string, ctx *apiclient.AuthContext, override bool) (map[string]any, error) {
			return nil, expectedErr
		},
	}

	cmd := newFlowsDeployCommand(fake)

	_, err := executeCommand(cmd, fixturePath)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
}

func executeCommand(cmd *cobra.Command, args ...string) (string, error) {
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)

	stdout, err := captureStdout(func() error {
		return cmd.Execute()
	})
	return buf.String() + stdout, err
}

func captureStdout(fn func() error) (string, error) {
	originalStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	defer func() {
		os.Stdout = originalStdout
	}()

	os.Stdout = w

	var buf bytes.Buffer
	copyDone := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(&buf, r)
		r.Close()
		copyDone <- copyErr
	}()

	closed := false
	defer func() {
		if !closed {
			w.Close()
		}
	}()

	errFn := fn()
	if !closed {
		w.Close()
		closed = true
	}

	copyErr := <-copyDone

	if errFn != nil {
		return buf.String(), errFn
	}
	return buf.String(), copyErr
}
