package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	apiclient "github.com/kestra-io/kestra-cli/src/api_client"
)

func newKestraClient() *apiclient.KestraClient {
	return apiclient.NewKestraClient(nil)
}

func temporaryContext(host, tenant, token string) *apiclient.AuthContext {
	if host == "" && token == "" {
		return nil
	}

	if host == "" {
		host = "http://localhost:8080"
	}
	if tenant == "" {
		tenant = "main"
	}

	return &apiclient.AuthContext{
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
