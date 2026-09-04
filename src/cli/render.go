package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"text/tabwriter"
)

const (
	OutputTable = "table"
	OutputJSON  = "json"
)

type Renderer struct {
	format string
	out    io.Writer
}

func NewRenderer(output string, out io.Writer) (*Renderer, error) {
	normalized, err := normalizeOutputFormat(output)
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = os.Stdout
	}
	return &Renderer{format: normalized, out: out}, nil
}

func NewRendererFromFlags(out io.Writer) (*Renderer, error) {
	if err := validateOutputFormat(); err != nil {
		return nil, err
	}
	return NewRenderer(globalFlags.Output, out)
}

func (r *Renderer) Format() string {
	return r.format
}

func (r *Renderer) IsJSON() bool {
	return r.format == OutputJSON
}

func (r *Renderer) Writer() io.Writer {
	return r.out
}

func (r *Renderer) Render(value any, renderTable func(w *tabwriter.Writer) error) error {
	if r.IsJSON() {
		return r.RenderJSON(value)
	}

	writer := tabwriter.NewWriter(r.out, 0, 4, 2, ' ', 0)
	if err := renderTable(writer); err != nil {
		return err
	}
	return writer.Flush()
}

func (r *Renderer) RenderJSON(value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(r.out, string(data))
	return err
}

// underlyingString reports the text of any value whose underlying kind is
// string, dereferencing pointers along the way.
//
// The Kestra SDK generates its enums as named string types (Level, StateType,
// Relation, ...) with no String() method, so a plain `case string` type switch
// does not match them and they would otherwise fall through to json.Marshal
// and be rendered with their JSON quotes — e.g. `"INFO"` instead of INFO.
// See https://github.com/kestra-io/kestractl/issues/122.
func underlyingString(value any) (string, bool) {
	rv := reflect.ValueOf(value)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return "", false
		}
		rv = rv.Elem()
	}
	if rv.Kind() == reflect.String {
		return rv.String(), true
	}
	return "", false
}

func stringify(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		if s, ok := underlyingString(v); ok {
			return s
		}
		data, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(data)
	}
}

func toPrettyString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		if s, ok := underlyingString(v); ok {
			return s
		}
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(data)
	}
}

func normalizeOutputFormat(output string) (string, error) {
	if output == "" {
		output = OutputTable
	}

	normalized := strings.ToLower(output)
	if normalized != OutputTable && normalized != OutputJSON {
		return "", fmt.Errorf("output must be 'table' or 'json', got '%s'", output)
	}
	return normalized, nil
}

// validateOutputFormat validates the output format from global flags.
func validateOutputFormat() error {
	normalized, err := normalizeOutputFormat(globalFlags.Output)
	if err != nil {
		return err
	}
	globalFlags.Output = normalized
	return nil
}

// joinEnumValues renders a string-backed enum's allowed values for error
// messages, e.g. "PENDING, ACCEPTED, EXPIRED".
func joinEnumValues[T ~string](values []T) string {
	parts := make([]string, 0, len(values))
	for _, v := range values {
		parts = append(parts, string(v))
	}
	return strings.Join(parts, ", ")
}
