package cli

import (
	"strings"
	"testing"

	kestra "github.com/kestra-io/client-sdk/go-sdk/v2/kestra_api_client"
)

func TestParseQueryFilters_Namespace(t *testing.T) {
	filters, err := parseQueryFilters("my.ns", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(filters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(filters))
	}
	if filters[0].Field == nil || *filters[0].Field != kestra.QUERYFILTERFIELD_NAMESPACE {
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
	if *filters[0].Field != kestra.QUERYFILTERFIELD_STATE || string(*filters[0].Operation) != "EQUALS" || filters[0].Value != "SUCCESS" {
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

func TestParseQueryFilters_FieldNames(t *testing.T) {
	cases := map[string]kestra.QueryFilterField{
		"NAMESPACE": "namespace",
		"FLOW_ID":   "flowId",
		"flow_id":   "flowId",
		"STATE":     "state",
		"QUERY":     "q",
		"MIN_LEVEL": "level",
	}
	for name, want := range cases {
		filters, err := parseQueryFilters("", []string{name + ":EQUALS:x"})
		if err != nil {
			t.Errorf("%s: unexpected error: %v", name, err)
			continue
		}
		if got := *filters[0].Field; got != want {
			t.Errorf("%s: field = %q, want %q", name, got, want)
		}
	}
}

func TestQueryFilterFields_CoverSDKEnum(t *testing.T) {
	mapped := map[kestra.QueryFilterField]bool{}
	for _, f := range queryFilterFields {
		mapped[f] = true
	}
	for _, f := range kestra.AllowedQueryFilterFieldEnumValues {
		if !mapped[f] {
			t.Errorf("SDK field %q has no --filter name in queryFilterFields", f)
		}
	}
}

func TestQueryFiltersToSearchFilters(t *testing.T) {
	qf, err := parseQueryFilters("my.ns", []string{"FLOW_ID:EQUALS:my-flow", "MIN_LEVEL:EQUALS:WARN"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sf := queryFiltersToSearchFilters(qf)
	want := []string{"namespace", "flowId", "level"}
	if len(sf) != len(want) {
		t.Fatalf("expected %d search filters, got %d", len(want), len(sf))
	}
	for i, field := range want {
		if string(sf[i].Field) != field {
			t.Errorf("search filter %d: field = %q, want %q", i, sf[i].Field, field)
		}
	}
}
