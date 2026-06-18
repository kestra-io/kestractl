package cli

import (
	"strings"
	"testing"
)

func TestParseQueryFilters_Namespace(t *testing.T) {
	filters, err := parseQueryFilters("my.ns", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(filters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(filters))
	}
	if filters[0].Field == nil || string(*filters[0].Field) != "NAMESPACE" {
		t.Fatalf("expected NAMESPACE field, got %v", filters[0].Field)
	}
	if filters[0].Operation == nil || string(*filters[0].Operation) != "EQUALS" {
		t.Fatalf("expected EQUALS operation, got %v", filters[0].Operation)
	}
	if filters[0].Value != "my.ns" {
		t.Fatalf("expected value my.ns, got %v", filters[0].Value)
	}
}

func TestParseQueryFilters_RawSpec(t *testing.T) {
	filters, err := parseQueryFilters("", []string{"STATE:EQUALS:SUCCESS"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(filters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(filters))
	}
	if string(*filters[0].Field) != "STATE" || string(*filters[0].Operation) != "EQUALS" || filters[0].Value != "SUCCESS" {
		t.Fatalf("unexpected filter: %+v", filters[0])
	}
}

func TestParseQueryFilters_NamespaceAndRawCombine(t *testing.T) {
	filters, err := parseQueryFilters("my.ns", []string{"FLOW_ID:EQUALS:my-flow"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(filters) != 2 {
		t.Fatalf("expected 2 filters, got %d", len(filters))
	}
}

func TestParseQueryFilters_InvalidSpec(t *testing.T) {
	_, err := parseQueryFilters("", []string{"STATE:SUCCESS"})
	if err == nil {
		t.Fatal("expected error for malformed filter spec")
	}
	if !strings.Contains(err.Error(), "FIELD:OPERATION:VALUE") {
		t.Fatalf("expected format hint, got: %v", err)
	}
}

func TestParseQueryFilters_InvalidField(t *testing.T) {
	_, err := parseQueryFilters("", []string{"BOGUS:EQUALS:x"})
	if err == nil {
		t.Fatal("expected error for invalid field")
	}
	if !strings.Contains(err.Error(), "invalid filter field") {
		t.Fatalf("expected field error, got: %v", err)
	}
}

func TestParseQueryFilters_InvalidOperation(t *testing.T) {
	_, err := parseQueryFilters("", []string{"STATE:BOGUS:x"})
	if err == nil {
		t.Fatal("expected error for invalid operation")
	}
	if !strings.Contains(err.Error(), "invalid filter operation") {
		t.Fatalf("expected operation error, got: %v", err)
	}
}

func TestByQueryFilterFlags_ResolveRequiresSelection(t *testing.T) {
	f := &byQueryFilterFlags{}
	_, err := f.resolve()
	if err == nil {
		t.Fatal("expected error when no selection provided")
	}
	if !strings.Contains(err.Error(), "at least one selection filter is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestByQueryFilterFlags_ResolveOptionalAllowsEmpty(t *testing.T) {
	f := &byQueryFilterFlags{}
	filters, err := f.resolveOptional()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filters != nil {
		t.Fatalf("expected nil filters, got %v", filters)
	}
}

func TestQueryFiltersToSearchFilters(t *testing.T) {
	qf, err := parseQueryFilters("my.ns", []string{"FLOW_ID:EQUALS:my-flow"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sf := queryFiltersToSearchFilters(qf)
	if len(sf) != 2 {
		t.Fatalf("expected 2 search filters, got %d", len(sf))
	}
	if string(sf[0].Field) != "namespace" {
		t.Fatalf("expected camel-cased namespace field, got %q", sf[0].Field)
	}
	if string(sf[1].Field) != "flowId" {
		t.Fatalf("expected camel-cased flowId field, got %q", sf[1].Field)
	}
}

func TestCamelFilterField(t *testing.T) {
	cases := map[string]string{
		"NAMESPACE": "namespace",
		"FLOW_ID":   "flowId",
		"QUERY":     "q",
		"STATE":     "state",
	}
	for in, want := range cases {
		if got := camelFilterField(in); got != want {
			t.Errorf("camelFilterField(%q) = %q, want %q", in, got, want)
		}
	}
}
