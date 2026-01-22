package cli

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeNamespacesService struct {
	listFn func(ctx context.Context, tenant string, query string, page, size int) ([]any, error)
}

func (f *fakeNamespacesService) ListNamespaces(ctx context.Context, tenant string, query string, page, size int) ([]any, error) {
	if f.listFn == nil {
		return nil, errors.New("list not implemented")
	}
	return f.listFn(ctx, tenant, query, page, size)
}

func TestNamespacesListCommand_Success(t *testing.T) {
	fake := &fakeNamespacesService{
		listFn: func(ctx context.Context, tenant string, query string, page, size int) ([]any, error) {
			if page != 1 {
				t.Fatalf("expected page 1, got %d", page)
			}
			if size != 100 {
				t.Fatalf("expected size 100, got %d", size)
			}
			return []any{
				"namespace1",
				"namespace2",
				"test.namespace",
			}, nil
		},
	}

	cmd := newNamespacesListCommand(fake)

	output, err := executeCommand(cmd)
	if err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}

	if !strings.Contains(output, "Total namespaces: 3") {
		t.Fatalf("expected namespace count in output, got: %s", output)
	}

	if !strings.Contains(output, "namespace1") {
		t.Fatalf("expected namespace1 in output, got: %s", output)
	}
}

func TestNamespacesListCommand_WithQuery(t *testing.T) {
	fake := &fakeNamespacesService{
		listFn: func(ctx context.Context, tenant string, query string, page, size int) ([]any, error) {
			if query != "test" {
				t.Fatalf("expected query 'test', got '%s'", query)
			}
			return []any{
				"test.namespace",
			}, nil
		},
	}

	cmd := newNamespacesListCommand(fake)

	output, err := executeCommand(cmd, "--query", "test")
	if err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}

	if !strings.Contains(output, "Total namespaces: 1") {
		t.Fatalf("expected namespace count in output, got: %s", output)
	}
}

func TestNamespacesListCommand_ServiceError(t *testing.T) {
	expectedErr := errors.New("list failed")

	fake := &fakeNamespacesService{
		listFn: func(ctx context.Context, tenant string, query string, page, size int) ([]any, error) {
			return nil, expectedErr
		},
	}

	cmd := newNamespacesListCommand(fake)

	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
}
