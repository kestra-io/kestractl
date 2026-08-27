package cli

import (
	"fmt"
	"strings"

	kestra "github.com/kestra-io/client-sdk/go-sdk/v2/kestra_api_client"
	"github.com/spf13/cobra"
)

// byQueryFilterFlags holds the selection flags shared by every *-by-query
// command. The server-side bulk endpoints operate on the set of resources
// matched by these filters and reject a request that carries no selection, so
// at least one filter must be provided.
type byQueryFilterFlags struct {
	namespace string
	flowID    string
	filters   []string
}

// addByQueryFilterFlags registers the shared selection flags on a *-by-query
// command.
func addByQueryFilterFlags(cmd *cobra.Command, f *byQueryFilterFlags) {
	cmd.Flags().StringVar(&f.namespace, "namespace", "", "Only match resources in this namespace")
	cmd.Flags().StringVar(&f.flowID, "flow", "", "Only match resources for this flow ID")
	cmd.Flags().StringArrayVar(&f.filters, "filter", nil,
		"Additional match filter as FIELD:OPERATION:VALUE (e.g. STATE:EQUALS:SUCCESS); repeatable")
}

// extraFilters merges the convenience flags into the raw FIELD:OPERATION:VALUE
// list understood by parseQueryFilters.
func (f *byQueryFilterFlags) extraFilters() []string {
	specs := append([]string{}, f.filters...)
	if strings.TrimSpace(f.flowID) != "" {
		specs = append(specs, "FLOW_ID:EQUALS:"+f.flowID)
	}
	return specs
}

// resolve builds the SDK QueryFilter slice from the flags. It returns an error
// when no selection was provided, because the server rejects unfiltered bulk
// operations.
func (f *byQueryFilterFlags) resolve() ([]kestra.QueryFilter, error) {
	filters, err := parseQueryFilters(f.namespace, f.extraFilters())
	if err != nil {
		return nil, err
	}
	if len(filters) == 0 {
		return nil, fmt.Errorf("at least one selection filter is required: use --namespace or --filter FIELD:OPERATION:VALUE")
	}
	return filters, nil
}

// resolveOptional builds the SDK QueryFilter slice without requiring a
// selection. It returns nil when no flags were provided, which leaves the
// operation tenant-wide (the endpoint's default). Use this for endpoints that
// accept an unfiltered bulk request.
func (f *byQueryFilterFlags) resolveOptional() ([]kestra.QueryFilter, error) {
	filters, err := parseQueryFilters(f.namespace, f.extraFilters())
	if err != nil {
		return nil, err
	}
	if len(filters) == 0 {
		return nil, nil
	}
	return filters, nil
}

// parseQueryFilters builds the SDK QueryFilter slice used by the *-by-query
// endpoints. namespace, when non-empty, adds an equality filter on NAMESPACE.
// Each entry in raw must be "FIELD:OPERATION:VALUE" (e.g. "STATE:EQUALS:SUCCESS").
func parseQueryFilters(namespace string, raw []string) ([]kestra.QueryFilter, error) {
	filters := make([]kestra.QueryFilter, 0, len(raw)+1)

	if strings.TrimSpace(namespace) != "" {
		qf, err := newQueryFilter("NAMESPACE", "EQUALS", namespace)
		if err != nil {
			return nil, err
		}
		filters = append(filters, *qf)
	}

	for _, spec := range raw {
		parts := strings.SplitN(spec, ":", 3)
		if len(parts) != 3 {
			return nil, fmt.Errorf("invalid --filter %q: expected FIELD:OPERATION:VALUE", spec)
		}
		qf, err := newQueryFilter(parts[0], parts[1], parts[2])
		if err != nil {
			return nil, err
		}
		filters = append(filters, *qf)
	}

	return filters, nil
}

// queryFiltersToSearchFilters converts the generated QueryFilter values into
// the hand-written SearchFilter values used by request-body endpoints (e.g.
// triggers delete-by-query). The field name is camel-cased to the server's
// query-parameter form, matching the SDK's own conversion.
func queryFiltersToSearchFilters(filters []kestra.QueryFilter) []kestra.SearchFilter {
	out := make([]kestra.SearchFilter, 0, len(filters))
	for _, f := range filters {
		field := ""
		if f.Field != nil {
			field = camelFilterField(string(*f.Field))
		}
		op := ""
		if f.Operation != nil {
			op = string(*f.Operation)
		}
		out = append(out, kestra.SearchFilter{
			Field:     kestra.SearchFilterField(field),
			Operation: kestra.SearchFilterOp(op),
			Value:     f.Value,
		})
	}
	return out
}

// camelFilterField mirrors the SDK's field-name conversion: QUERY becomes "q",
// otherwise the value is camel-cased (NAMESPACE -> namespace, FLOW_ID -> flowId).
func camelFilterField(s string) string {
	if strings.EqualFold(s, "query") {
		return "q"
	}
	parts := strings.FieldsFunc(strings.TrimSpace(s), func(r rune) bool {
		return !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
	if len(parts) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(strings.ToLower(parts[0]))
	for _, p := range parts[1:] {
		p = strings.ToLower(p)
		if p == "" {
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]))
		if len(p) > 1 {
			b.WriteString(p[1:])
		}
	}
	return b.String()
}

// newQueryFilter validates the field and operation against the SDK enums and
// returns a populated QueryFilter.
func newQueryFilter(field, op, value string) (*kestra.QueryFilter, error) {
	fieldEnum, err := kestra.NewQueryFilterFieldFromValue(strings.ToUpper(strings.TrimSpace(field)))
	if err != nil {
		return nil, fmt.Errorf("invalid filter field %q: %w", field, err)
	}
	opEnum, err := kestra.NewQueryFilterOpFromValue(strings.ToUpper(strings.TrimSpace(op)))
	if err != nil {
		return nil, fmt.Errorf("invalid filter operation %q: %w", op, err)
	}

	qf := kestra.NewQueryFilter()
	qf.Field = fieldEnum
	qf.Operation = opEnum
	qf.Value = value
	return qf, nil
}
