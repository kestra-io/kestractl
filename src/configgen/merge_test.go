package configgen

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDeepMerge_FlatMaps(t *testing.T) {
	dst := map[string]any{"a": "1", "b": "2"}
	src := map[string]any{"b": "overwritten", "c": "3"}
	result := deepMerge(dst, src)

	if result["a"] != "1" {
		t.Errorf("expected a=1, got %v", result["a"])
	}
	if result["b"] != "overwritten" {
		t.Errorf("expected b=overwritten, got %v", result["b"])
	}
	if result["c"] != "3" {
		t.Errorf("expected c=3, got %v", result["c"])
	}
	// originals untouched
	if dst["b"] != "2" {
		t.Errorf("dst was mutated")
	}
}

func TestDeepMerge_NestedMaps(t *testing.T) {
	dst := map[string]any{
		"kestra": map[string]any{
			"queue":   map[string]any{"type": "kafka"},
			"storage": map[string]any{"type": "local"},
		},
	}
	src := map[string]any{
		"kestra": map[string]any{
			"repository": map[string]any{"type": "elasticsearch"},
		},
	}
	result := deepMerge(dst, src)

	kestra := result["kestra"].(map[string]any)
	if kestra["queue"].(map[string]any)["type"] != "kafka" {
		t.Error("queue type lost during merge")
	}
	if kestra["storage"].(map[string]any)["type"] != "local" {
		t.Error("storage type lost during merge")
	}
	if kestra["repository"].(map[string]any)["type"] != "elasticsearch" {
		t.Error("repository type not merged")
	}
}

func TestDeepMerge_SrcOverwritesDst(t *testing.T) {
	dst := map[string]any{
		"kestra": map[string]any{
			"queue": map[string]any{"type": "h2"},
		},
	}
	src := map[string]any{
		"kestra": map[string]any{
			"queue": map[string]any{"type": "kafka"},
		},
	}
	result := deepMerge(dst, src)
	queueType := result["kestra"].(map[string]any)["queue"].(map[string]any)["type"]
	if queueType != "kafka" {
		t.Errorf("expected kafka, got %v", queueType)
	}
}

func TestReplacePlaceholders_Nested(t *testing.T) {
	m := map[string]any{
		"kestra": map[string]any{
			"url": "__KESTRA_URL__",
			"encryption": map[string]any{
				"secret-key": "__ENCRYPTION_KEY__",
			},
		},
	}
	replacements := map[string]string{
		"__KESTRA_URL__":     "http://localhost:8080/",
		"__ENCRYPTION_KEY__": "test-key-12345",
	}
	result := replacePlaceholders(m, replacements)

	kestra := result["kestra"].(map[string]any)
	if kestra["url"] != "http://localhost:8080/" {
		t.Errorf("url not replaced: %v", kestra["url"])
	}
	enc := kestra["encryption"].(map[string]any)
	if enc["secret-key"] != "test-key-12345" {
		t.Errorf("encryption key not replaced: %v", enc["secret-key"])
	}
}

func TestReplacePlaceholders_SkipsNonStrings(t *testing.T) {
	m := map[string]any{
		"port":    8080,
		"enabled": true,
		"name":    "__NAME__",
	}
	result := replacePlaceholders(m, map[string]string{"__NAME__": "test"})

	if result["port"] != 8080 {
		t.Error("int was modified")
	}
	if result["enabled"] != true {
		t.Error("bool was modified")
	}
	if result["name"] != "test" {
		t.Error("string was not replaced")
	}
}

func TestReplacePlaceholders_Slices(t *testing.T) {
	m := map[string]any{
		"permissions": map[string]any{
			"FLOW": []any{"CREATE", "READ", "__ACTION__"},
		},
	}
	result := replacePlaceholders(m, map[string]string{"__ACTION__": "DELETE"})
	perms := result["permissions"].(map[string]any)
	flow := perms["FLOW"].([]any)
	if flow[2] != "DELETE" {
		t.Errorf("slice element not replaced: %v", flow[2])
	}
}

func TestFullSnippetMerge(t *testing.T) {
	baseYAML := `
kestra:
  tasks:
    tmp-dir:
      path: __TMP_DIR_PATH__
  url: __KESTRA_URL__
`
	queueYAML := `
kestra:
  queue:
    type: kafka
`
	repoYAML := `
kestra:
  repository:
    type: elasticsearch
`
	snippets := []string{baseYAML, queueYAML, repoYAML}
	merged := map[string]any{}
	for _, s := range snippets {
		var m map[string]any
		if err := yaml.Unmarshal([]byte(s), &m); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		merged = deepMerge(merged, m)
	}

	result := replacePlaceholders(merged, map[string]string{
		"__TMP_DIR_PATH__": "/tmp/kestra-wd/tmp",
		"__KESTRA_URL__":   "http://localhost:8080/",
	})

	// Marshal and verify structure
	out, err := yaml.Marshal(result)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	// Parse back to verify
	var check map[string]any
	if err := yaml.Unmarshal(out, &check); err != nil {
		t.Fatalf("re-unmarshal failed: %v", err)
	}

	kestra := check["kestra"].(map[string]any)
	if kestra["queue"].(map[string]any)["type"] != "kafka" {
		t.Error("queue type missing after full round-trip")
	}
	if kestra["repository"].(map[string]any)["type"] != "elasticsearch" {
		t.Error("repository type missing after full round-trip")
	}
	if kestra["url"] != "http://localhost:8080/" {
		t.Error("url placeholder not replaced after full round-trip")
	}
}
