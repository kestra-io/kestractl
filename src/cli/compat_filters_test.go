package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	kestra "github.com/kestra-io/client-sdk/go-sdk/v2/kestra_api_client"
)

// TestByQueryCommands_SendMultiWordFilterFieldsInServerCase covers
// kestra-io/client-sdk#475: the generated *ByQuery endpoints of Go SDK v2.0.2+
// send FLOW_ID as `flowid`, which Kestra rejects with a 422.
func TestByQueryCommands_SendMultiWordFilterFieldsInServerCase(t *testing.T) {
	labels := []kestra.Label{{Key: "env", Value: "prod"}}
	commands := []struct {
		name string
		run  func(*Client, []kestra.QueryFilter, *Renderer) error
	}{
		{"flows disable-by-query", func(c *Client, f []kestra.QueryFilter, r *Renderer) error {
			return runFlowsByQueryOp(c, "disable", f, r)
		}},
		{"executions kill-by-query", func(c *Client, f []kestra.QueryFilter, r *Renderer) error {
			return runExecutionsByQueryOp(c, "kill", f, r)
		}},
		{"executions set-labels-by-query", func(c *Client, f []kestra.QueryFilter, r *Renderer) error {
			return runExecutionsSetLabelsByQuery(c, labels, f, r)
		}},
	}

	for _, tt := range commands {
		t.Run(tt.name, func(t *testing.T) {
			var got url.Values
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.URL.Query()
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"count":1}`))
			}))
			t.Cleanup(server.Close)

			flags := byQueryFilterFlags{namespace: "qa.a", flowID: "f1"}
			filters, err := flags.resolve()
			if err != nil {
				t.Fatal(err)
			}
			renderer, err := NewRenderer("json", io.Discard)
			if err != nil {
				t.Fatal(err)
			}
			if err := tt.run(newTestClient(t, server.URL), filters, renderer); err != nil {
				t.Fatalf("%s: %v", tt.name, err)
			}

			want := url.Values{
				"filters[namespace][EQUALS]": {"qa.a"},
				"filters[flowId][EQUALS]":    {"f1"},
			}
			for key, values := range want {
				if strings.Join(got[key], ",") != strings.Join(values, ",") {
					t.Errorf("%s = %q, want %q (query: %v)", key, got[key], values, got)
				}
			}
			if _, ok := got["filters[flowid][EQUALS]"]; ok {
				t.Errorf("lower-cased field sent: %v", got)
			}
		})
	}
}

func TestFilterKeyWithWireField(t *testing.T) {
	tests := []struct {
		key, want string
	}{
		{"filters[flowid][EQUALS]", "filters[flowId][EQUALS]"},
		{"filters[triggerexecutionid][EQUALS]", "filters[triggerExecutionId][EQUALS]"},
		{"filters[and][0][flowid][EQUALS]", "filters[and][0][flowId][EQUALS]"},
		{"filters[or][1][and][0][startdate][GREATER_THAN_OR_EQUAL_TO]", "filters[or][1][and][0][startDate][GREATER_THAN_OR_EQUAL_TO]"},
		// Already in the server's case, or single-word: nothing to restore.
		{"filters[flowId][EQUALS]", "filters[flowId][EQUALS]"},
		{"filters[namespace][EQUALS]", "filters[namespace][EQUALS]"},
		{"filters[q][EQUALS]", "filters[q][EQUALS]"},
		// A label key is user data, not a field, even when it reads like one.
		{"filters[labels][EQUALS][flowid]", "filters[labels][EQUALS][flowid]"},
		{"flowid", "flowid"},
		{"page", "page"},
	}
	for _, tt := range tests {
		got, changed := filterKeyWithWireField(tt.key)
		if got != tt.want || changed != (tt.key != tt.want) {
			t.Errorf("filterKeyWithWireField(%q) = %q, %v; want %q", tt.key, got, changed, tt.want)
		}
	}
}

func TestCompatTransport_RestoresFilterFieldCaseOnly(t *testing.T) {
	var gotRawQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)

	const rawQuery = "page=1&filters[flowid][EQUALS]=my%20flow&filters%5Btriggerid%5D%5BEQUALS%5D=t1&filters[labels][EQUALS][flowid]=x"
	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/main/flows/disable/by-query?"+rawQuery, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := newTestClient(t, server.URL).API.GetConfig().HTTPClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	const want = "page=1&filters[flowId][EQUALS]=my%20flow&filters[triggerId][EQUALS]=t1&filters[labels][EQUALS][flowid]=x"
	if gotRawQuery != want {
		t.Errorf("server got query\n  %s\nwant\n  %s", gotRawQuery, want)
	}
	if req.URL.RawQuery != rawQuery {
		t.Errorf("caller request was mutated: %q", req.URL.RawQuery)
	}
}
