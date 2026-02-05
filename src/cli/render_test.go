package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"text/tabwriter"
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
