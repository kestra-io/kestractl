package cli

import (
	"fmt"
	"sort"
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
// triggers delete-by-query). QueryFilterField values are already the server's
// wire names, so the field is passed through unchanged.
func queryFiltersToSearchFilters(filters []kestra.QueryFilter) []kestra.SearchFilter {
	out := make([]kestra.SearchFilter, 0, len(filters))
	for _, f := range filters {
		field := ""
		if f.Field != nil {
			field = string(*f.Field)
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

// queryFilterFields maps the FIELD names accepted by --filter to the SDK
// enum. The names are the server's QueryFilter.Field constants (FLOW_ID), while
// the SDK values are their wire names (flowId). Most are the camel-cased name,
// but QUERY is "q" and MIN_LEVEL is "level", so the mapping is spelled out.
var queryFilterFields = map[string]kestra.QueryFilterField{
	"QUERY":                kestra.QUERYFILTERFIELD_QUERY,
	"SCOPE":                kestra.QUERYFILTERFIELD_SCOPE,
	"NAMESPACE":            kestra.QUERYFILTERFIELD_NAMESPACE,
	"KIND":                 kestra.QUERYFILTERFIELD_KIND,
	"LABELS":               kestra.QUERYFILTERFIELD_LABELS,
	"METADATA":             kestra.QUERYFILTERFIELD_METADATA,
	"FLOW_ID":              kestra.QUERYFILTERFIELD_FLOW_ID,
	"FLOW_REVISION":        kestra.QUERYFILTERFIELD_FLOW_REVISION,
	"ID":                   kestra.QUERYFILTERFIELD_ID,
	"ASSET_ID":             kestra.QUERYFILTERFIELD_ASSET_ID,
	"TYPE":                 kestra.QUERYFILTERFIELD_TYPE,
	"CREATED":              kestra.QUERYFILTERFIELD_CREATED,
	"UPDATED":              kestra.QUERYFILTERFIELD_UPDATED,
	"START_DATE":           kestra.QUERYFILTERFIELD_START_DATE,
	"END_DATE":             kestra.QUERYFILTERFIELD_END_DATE,
	"STATE":                kestra.QUERYFILTERFIELD_STATE,
	"TIME_RANGE":           kestra.QUERYFILTERFIELD_TIME_RANGE,
	"TRIGGER_EXECUTION_ID": kestra.QUERYFILTERFIELD_TRIGGER_EXECUTION_ID,
	"TRIGGER_ID":           kestra.QUERYFILTERFIELD_TRIGGER_ID,
	"TRIGGER_STATE":        kestra.QUERYFILTERFIELD_TRIGGER_STATE,
	"EXECUTION_ID":         kestra.QUERYFILTERFIELD_EXECUTION_ID,
	"TASK_ID":              kestra.QUERYFILTERFIELD_TASK_ID,
	"TASK_RUN_ID":          kestra.QUERYFILTERFIELD_TASK_RUN_ID,
	"CHILD_FILTER":         kestra.QUERYFILTERFIELD_CHILD_FILTER,
	"WORKER_ID":            kestra.QUERYFILTERFIELD_WORKER_ID,
	"EXISTING_ONLY":        kestra.QUERYFILTERFIELD_EXISTING_ONLY,
	"MIN_LEVEL":            kestra.QUERYFILTERFIELD_MIN_LEVEL,
	"PATH":                 kestra.QUERYFILTERFIELD_PATH,
	"PARENT_PATH":          kestra.QUERYFILTERFIELD_PARENT_PATH,
	"VERSION":              kestra.QUERYFILTERFIELD_VERSION,
}

// newQueryFilter validates the field and operation against the SDK enums and
// returns a populated QueryFilter.
func newQueryFilter(field, op, value string) (*kestra.QueryFilter, error) {
	fieldEnum, ok := queryFilterFields[strings.ToUpper(strings.TrimSpace(field))]
	if !ok {
		return nil, fmt.Errorf("invalid filter field %q: valid fields are %s", field, strings.Join(queryFilterFieldNames(), ", "))
	}
	opEnum, err := kestra.NewQueryFilterOpFromValue(strings.ToUpper(strings.TrimSpace(op)))
	if err != nil {
		return nil, fmt.Errorf("invalid filter operation %q: %w", op, err)
	}

	qf := kestra.NewQueryFilter()
	qf.SetField(fieldEnum)
	qf.Operation = opEnum
	qf.Value = value
	return qf, nil
}

// queryFilterFieldNames returns the accepted --filter field names, sorted.
func queryFilterFieldNames() []string {
	names := make([]string, 0, len(queryFilterFields))
	for name := range queryFilterFields {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
