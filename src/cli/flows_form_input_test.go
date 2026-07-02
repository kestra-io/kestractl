package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Regression coverage for the originally reported bug: `flows list`/`ls` erroring
// with "FORM is not a valid Type" whenever a returned flow has a FORM input, because
// the bundled go-sdk's Type enum used to be closed and didn't include FORM.
//
// On current main this already passes without any SDK change: PR #83's generic
// GenericOpenAPIError raw-body recovery (tryParseFlowListFromError / tryParseFlowSearchFromError)
// kicks in on any decode failure, not just the array-format-labels bug it was written
// for, and neither recovery path nor parsedFlow reads the "type"/"inputs" fields anyway.
// These tests document and lock in that behavior; they do not exercise the go-sdk fix
// itself (see go-sdk/kestra_api_client/model_type_test.go in client-sdk for that).
const flowWithFormInputJSON = `{"id":"f1","namespace":"my.namespace","disabled":false,"deleted":false,"tasks":[{"id":"t","type":"io.kestra.plugin.core.log.Log"}],"inputs":[{"id":"myform","type":"FORM"}]}`

func TestListAllFlows_ToleratesFormInput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[` + flowWithFormInputJSON + `],"total":1}`))
	}))
	t.Cleanup(server.Close)

	flows, err := listAllFlows(newTestClient(t, server.URL))
	if err != nil {
		t.Fatalf("listAllFlows should tolerate a FORM input, got error: %v", err)
	}
	if len(flows) != 1 || flows[0].ID != "f1" {
		t.Fatalf("expected flow f1 to be listed, got: %+v", flows)
	}
}

func TestRunFlowsList_Namespace_ToleratesFormInput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[` + flowWithFormInputJSON + `]`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runFlowsList(newTestClient(t, server.URL), "my.namespace", newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runFlowsList should tolerate a FORM input, got error: %v", err)
	}
	if !strings.Contains(buf.String(), "f1") {
		t.Fatalf("expected flow f1 in output, got:\n%s", buf.String())
	}
}
