package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"text/tabwriter"

	kestra "github.com/kestra-io/client-sdk/go-sdk/v2/kestra_api_client"
)

func TestNormalizeOutputFormat(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{input: "table", want: "table", wantErr: false},
		{input: "json", want: "json", wantErr: false},
		{input: "TABLE", want: "table", wantErr: false},
		{input: "JSON", want: "json", wantErr: false},
		{input: "", want: "table", wantErr: false},
		{input: "xml", wantErr: true},
		{input: "yaml", wantErr: true},
	}

	for _, tt := range tests {
		got, err := normalizeOutputFormat(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Fatalf("expected error for %q", tt.input)
			}
			continue
		}
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", tt.input, err)
		}
		if got != tt.want {
			t.Fatalf("normalizeOutputFormat(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestValidateOutputFormat(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{input: "table", wantErr: false},
		{input: "json", wantErr: false},
		{input: "TABLE", wantErr: false},
		{input: "JSON", wantErr: false},
		{input: "", wantErr: false},
		{input: "xml", wantErr: true},
		{input: "yaml", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			globalFlags.Output = tt.input
			err := validateOutputFormat()
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestRenderer_RenderJSON(t *testing.T) {
	var buf bytes.Buffer
	renderer, err := NewRenderer(OutputJSON, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	called := false
	err = renderer.Render(map[string]any{"id": "flow"}, func(w *tabwriter.Writer) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected render error: %v", err)
	}
	if called {
		t.Fatal("expected table renderer not to be called")
	}

	output := buf.String()
	if !strings.Contains(output, "\"id\": \"flow\"") {
		t.Fatalf("expected JSON output, got: %s", output)
	}
}

func TestRenderer_RenderTable(t *testing.T) {
	var buf bytes.Buffer
	renderer, err := NewRenderer(OutputTable, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	called := false
	err = renderer.Render(map[string]any{"id": "flow"}, func(w *tabwriter.Writer) error {
		called = true
		fmt.Fprintln(w, "ID\tName")
		fmt.Fprintln(w, "1\tflow")
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected render error: %v", err)
	}
	if !called {
		t.Fatal("expected table renderer to be called")
	}

	output := buf.String()
	if !strings.Contains(output, "ID") || !strings.Contains(output, "flow") {
		t.Fatalf("expected table output, got: %s", output)
	}
	if strings.Contains(output, "\"id\"") {
		t.Fatalf("did not expect JSON output, got: %s", output)
	}
}

// bugStringer is a named string type that also implements fmt.Stringer, to
// prove the Stringer branch still wins over the plain-string-kind fallback.
type bugStringer string

func (b bugStringer) String() string { return "stringer:" + string(b) }

func TestStringify(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  string
	}{
		{name: "nil", input: nil, want: ""},
		{name: "plain string", input: "INFO", want: "INFO"},
		{name: "sdk Level enum", input: kestra.LEVEL_INFO, want: "INFO"},
		{name: "sdk Level enum error", input: kestra.LEVEL_ERROR, want: "ERROR"},
		{name: "sdk StateType enum", input: kestra.STATETYPE_SUCCESS, want: "SUCCESS"},
		{name: "sdk Relation enum", input: kestra.RELATION_USED_BY, want: "USED_BY"},
		{name: "pointer to enum", input: kestra.LEVEL_WARN.Ptr(), want: "WARN"},
		{name: "stringer wins over string kind", input: bugStringer("x"), want: "stringer:x"},
		{name: "int stays json", input: 42, want: "42"},
		{name: "bool stays json", input: true, want: "true"},
		{name: "byte slice stays json", input: []byte("hi"), want: `"aGk="`},
		{name: "json.RawMessage stays json", input: json.RawMessage(`{"a":1}`), want: `{"a":1}`},
		{name: "map stays json", input: map[string]any{"a": 1}, want: `{"a":1}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := stringify(tc.input); got != tc.want {
				t.Errorf("stringify(%#v) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestToPrettyString_EnumHasNoQuotes(t *testing.T) {
	if got := toPrettyString(kestra.STATETYPE_FAILED); got != "FAILED" {
		t.Errorf("toPrettyString(STATETYPE_FAILED) = %q, want %q", got, "FAILED")
	}
	// Non-string values keep their existing pretty-printed JSON form.
	wantPretty := "{\n  \"a\": 1\n}"
	if got := toPrettyString(json.RawMessage(`{"a":1}`)); got != wantPretty {
		t.Errorf("toPrettyString(RawMessage) = %q, want %q", got, wantPretty)
	}
}
