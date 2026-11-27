package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunConfigGenerate(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "stack.json")
	if err := os.WriteFile(cfg, []byte(`{"edition":"oss","project_name":"x","queue":"h2"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	r, err := NewRenderer("table", &out)
	if err != nil {
		t.Fatal(err)
	}

	if err := runConfigGenerate(cfg, filepath.Join(dir, "out"), true, r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "valid") {
		t.Fatalf("dry run output: %q", out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "out")); err == nil {
		t.Fatal("dry run wrote files")
	}

	if err := runConfigGenerate(cfg, filepath.Join(dir, "out"), false, r); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"application.yml", "docker-compose.yml", ".env"} {
		if _, err := os.Stat(filepath.Join(dir, "out", f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}

	if err := runConfigGenerate(filepath.Join(dir, "nope.json"), "", false, r); err == nil {
		t.Error("expected error for missing file")
	}
}
