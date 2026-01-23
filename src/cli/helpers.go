package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
)

func temporaryContext() *AuthContext {
	if globalFlags.Host == "" && globalFlags.Token == "" {
		return nil
	}

	host := globalFlags.Host
	tenant := globalFlags.Tenant
	token := globalFlags.Token

	if host == "" {
		host = "http://localhost:8080"
	}
	if tenant == "" {
		tenant = "main"
	}

	return &AuthContext{
		Name:       "temp",
		Host:       host,
		Tenant:     tenant,
		AuthMethod: "token",
		Token:      token,
	}
}

func printJSON(value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func tabWriter() *tabwriter.Writer {
	return tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
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
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(data)
	}
}

func formatList(items []string) string {
	return strings.Join(items, ", ")
}

// validateOutputFormat validates the output format from global flags
func validateOutputFormat() error {
	// Default to table if not set
	if globalFlags.Output == "" {
		globalFlags.Output = "table"
	}

	output := strings.ToLower(globalFlags.Output)
	if output != "table" && output != "json" {
		return fmt.Errorf("output must be 'table' or 'json', got '%s'", globalFlags.Output)
	}
	globalFlags.Output = output
	return nil
}
