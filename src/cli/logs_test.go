package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	kestra "github.com/kestra-io/client-sdk/go-sdk/v2/kestra_api_client"
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

func TestLogsDownloadCommand_NoArgs(t *testing.T) {
	cmd := newLogsDownloadCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestLogsDownloadCommand_ClientError(t *testing.T) {
	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newLogsDownloadCommand()
	_, err := executeCommand(cmd, "exec-123")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestBuildLogSearchFilters(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		filters := buildLogSearchFilters(nil, "", "", "", "", "")
		if len(filters) != 0 {
			t.Fatalf("expected no filters, got %d", len(filters))
		}
	})

	t.Run("all filters with upper-cased level", func(t *testing.T) {
		filters := buildLogSearchFilters(nil, "boom", "my.ns", "my-flow", "trg-1", "warn")
		if len(filters) != 5 {
			t.Fatalf("expected 5 filters, got %d", len(filters))
		}
		got := map[kestra.SearchFilterField]any{}
		for _, f := range filters {
			wantOp := kestra.OpEquals
			if f.Field == kestra.FilterMinLevel {
				// LEVEL is a minimum, and 2.0 rejects EQUALS on it.
				wantOp = kestra.OpGreaterThanOrEqualTo
			}
			if f.Operation != wantOp {
				t.Errorf("field %v: expected op %v, got %v", f.Field, wantOp, f.Operation)
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

// Regression test for #122: the LEVEL column and the JSON "level" field must
// not carry the JSON quotes of the SDK's Level enum.
func TestRunLogsList_LevelHasNoQuotes(t *testing.T) {
	body := `[{"timestamp":"2026-09-03T12:54:31.000Z","level":"INFO","namespace":"bug122","flowId":"hello122","taskId":"hello","message":"hello from bug122"}]`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	t.Run("json", func(t *testing.T) {
		var buf bytes.Buffer
		if err := runLogsList(newTestClient(t, server.URL), "exec-1", logFilter{}, newJSONRenderer(&buf)); err != nil {
			t.Fatalf("runLogsList error: %v", err)
		}
		var got []map[string]any
		if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
			t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
		}
		if len(got) != 1 {
			t.Fatalf("expected 1 entry, got %d: %s", len(got), buf.String())
		}
		if got[0]["level"] != "INFO" {
			t.Errorf(`json level = %#v, want "INFO"`, got[0]["level"])
		}
	})

	t.Run("table", func(t *testing.T) {
		var buf bytes.Buffer
		if err := runLogsList(newTestClient(t, server.URL), "exec-1", logFilter{}, newTableRenderer(&buf)); err != nil {
			t.Fatalf("runLogsList error: %v", err)
		}
		out := buf.String()
		if strings.Contains(out, `"INFO"`) {
			t.Errorf("table output contains quoted level:\n%s", out)
		}
		if !strings.Contains(out, "INFO") {
			t.Errorf("table output missing level:\n%s", out)
		}
	})
}

// The two server lines accept different operations on the level field, and
// each answers the other's with a 400 (#132):
//
//   - 2.0: "Operation EQUALS is not supported for field LEVEL. Supported
//     operations are GREATER_THAN_OR_EQUAL_TO, LESS_THAN_OR_EQUAL_TO, IN, NOT_IN"
//   - 1.3: "Operation GREATER_THAN_OR_EQUAL_TO is not supported for field
//     MIN_LEVEL. Supported operations are EQUALS, NOT_EQUALS"
func TestRunLogsSearch_MinLevelOperationPerServerEra(t *testing.T) {
	for _, tt := range []struct {
		name    string
		version string
		wantOp  string
		badOp   string
	}{
		{name: "kestra 2.0", version: "2.0.0-rc13", wantOp: "GREATER_THAN_OR_EQUAL_TO", badOp: "EQUALS"},
		{name: "kestra 1.3", version: "1.3.35", wantOp: "EQUALS", badOp: "GREATER_THAN_OR_EQUAL_TO"},
		// A develop build gets the 2.0 shape, like the rest of the compat layer.
		{name: "unknown version", version: "develop", wantOp: "GREATER_THAN_OR_EQUAL_TO", badOp: "EQUALS"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var rawQuery string
			server := httptest.NewServer(versionHandler(tt.version, func(w http.ResponseWriter, r *http.Request) {
				rawQuery = r.URL.RawQuery
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"total":0,"results":[]}`))
			}))
			t.Cleanup(server.Close)

			var buf bytes.Buffer
			client := newTestClient(t, server.URL)
			filters := buildLogSearchFilters(client, "intentional", "qa.ns", "", "", "ERROR")
			if err := runLogsSearch(client, filters, 1, 50, nil, newTableRenderer(&buf)); err != nil {
				t.Fatalf("runLogsSearch error: %v", err)
			}

			decoded, err := url.QueryUnescape(rawQuery)
			if err != nil {
				t.Fatalf("unescape query %q: %v", rawQuery, err)
			}
			if want := "filters[level][" + tt.wantOp + "]=ERROR"; !strings.Contains(decoded, want) {
				t.Errorf("expected %q, got query: %s", want, decoded)
			}
			if bad := "filters[level][" + tt.badOp + "]"; strings.Contains(decoded, bad) {
				t.Errorf("level must not use %q on this server, got query: %s", bad, decoded)
			}
			if !strings.Contains(decoded, "filters[namespace][EQUALS]=qa.ns") {
				t.Errorf("expected namespace EQUALS filter, got query: %s", decoded)
			}
		})
	}
}
