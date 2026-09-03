package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTenantsCommand_Structure(t *testing.T) {
	cmd := newTenantsCommand()

	if cmd.Use != "tenants" {
		t.Fatalf("expected use 'tenants', got '%s'", cmd.Use)
	}

	expected := map[string]bool{"list": false, "get <tenant_id>": false, "create <tenant_id>": false, "update <tenant_id>": false, "delete <tenant_id>": false}
	for _, sub := range cmd.Commands() {
		if _, ok := expected[sub.Use]; ok {
			expected[sub.Use] = true
		}
	}
	for use, found := range expected {
		if !found {
			t.Fatalf("expected '%s' subcommand", use)
		}
	}
}

func TestTenantsListCommand_Help(t *testing.T) {
	cmd := newTenantsListCommand()
	output, err := executeCommand(cmd, "--help")
	if err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}
	if !strings.Contains(output, "List all tenants") {
		t.Fatalf("expected help text, got: %s", output)
	}
}

func TestTenantsGetCommand_NoArgs(t *testing.T) {
	cmd := newTenantsGetCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestTenantsCreateCommand_NoArgs(t *testing.T) {
	cmd := newTenantsCreateCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestTenantsUpdateCommand_NoArgs(t *testing.T) {
	cmd := newTenantsUpdateCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestTenantsDeleteCommand_NoArgs(t *testing.T) {
	cmd := newTenantsDeleteCommand()
	_, err := executeCommand(cmd)
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Fatalf("expected args error, got: %v", err)
	}
}

func TestTenantsGetCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newTenantsGetCommand()
	_, err := executeCommand(cmd, "my-tenant")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestTenantsCreateCommand_ClientError(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	original := newClientFunc
	newClientFunc = func() (*Client, error) {
		return nil, errors.New("client error")
	}
	defer func() { newClientFunc = original }()

	cmd := newTenantsCreateCommand()
	_, err := executeCommand(cmd, "my-tenant")
	if err == nil {
		t.Fatal("expected client error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestTenantsCreateCommand_InvalidQuota(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	cmd := newTenantsCreateCommand()
	_, err := executeCommand(cmd, "my-tenant", "--quota", "invalid")
	if err == nil {
		t.Fatal("expected error for invalid --quota")
	}
	if !strings.Contains(err.Error(), "expected format duration=") {
		t.Fatalf("expected format error, got: %v", err)
	}
}

func TestTenantsUpdateCommand_BehaviorWithoutLimit(t *testing.T) {
	origOutput := globalFlags.Output
	globalFlags.Output = "table"
	defer func() { globalFlags.Output = origOutput }()

	cmd := newTenantsUpdateCommand()
	_, err := executeCommand(cmd, "my-tenant", "--concurrency-behavior", "FAIL")
	if err == nil {
		t.Fatal("expected error for --concurrency-behavior without --concurrency-limit")
	}
	if !strings.Contains(err.Error(), "--concurrency-behavior requires --concurrency-limit") {
		t.Fatalf("expected flag dependency error, got: %v", err)
	}
}

func TestParseConcurrencyFlags_NoneProvided(t *testing.T) {
	concurrency, err := parseConcurrencyFlags(0, false, "QUEUE", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if concurrency != nil {
		t.Fatalf("expected nil concurrency, got: %+v", concurrency)
	}
}

func TestParseConcurrencyFlags_LimitOnlyDefaultsToQueue(t *testing.T) {
	concurrency, err := parseConcurrencyFlags(10, true, "QUEUE", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if concurrency.GetLimit() != 10 || string(concurrency.GetBehavior()) != "QUEUE" {
		t.Fatalf("unexpected concurrency: %+v", concurrency)
	}
}

func TestParseConcurrencyFlags_LowercaseBehavior(t *testing.T) {
	concurrency, err := parseConcurrencyFlags(5, true, "fail", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(concurrency.GetBehavior()) != "FAIL" {
		t.Fatalf("unexpected behavior: %s", concurrency.GetBehavior())
	}
}

func TestParseConcurrencyFlags_InvalidBehavior(t *testing.T) {
	_, err := parseConcurrencyFlags(5, true, "EXPLODE", true)
	if err == nil {
		t.Fatal("expected error for invalid behavior")
	}
	if !strings.Contains(err.Error(), "expected QUEUE, CANCEL or FAIL") {
		t.Fatalf("expected behavior error, got: %v", err)
	}
}

func TestParseQuotaFlags_NoneProvided(t *testing.T) {
	quotas, err := parseQuotaFlags(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if quotas != nil {
		t.Fatalf("expected nil quotas, got: %+v", quotas)
	}
}

func TestParseQuotaFlags_Multiple(t *testing.T) {
	quotas, err := parseQuotaFlags([]string{
		"duration=PT1H,limit=100,behavior=FAIL",
		"behavior=cancel,duration=P1D,limit=1000",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(quotas) != 2 {
		t.Fatalf("expected 2 quotas, got: %d", len(quotas))
	}
	if quotas[0].Duration != "PT1H" || quotas[0].Limit != 100 || string(quotas[0].Behavior) != "FAIL" {
		t.Fatalf("unexpected first quota: %+v", quotas[0])
	}
	if quotas[1].Duration != "P1D" || quotas[1].Limit != 1000 || string(quotas[1].Behavior) != "CANCEL" {
		t.Fatalf("unexpected second quota: %+v", quotas[1])
	}
}

func TestParseQuotaFlags_MissingKey(t *testing.T) {
	_, err := parseQuotaFlags([]string{"duration=PT1H,limit=100"})
	if err == nil {
		t.Fatal("expected error for missing behavior")
	}
	if !strings.Contains(err.Error(), "duration, limit and behavior are all required") {
		t.Fatalf("expected missing-key error, got: %v", err)
	}
}

func TestParseQuotaFlags_UnknownKey(t *testing.T) {
	_, err := parseQuotaFlags([]string{"duration=PT1H,limit=100,behavior=FAIL,bogus=1"})
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
	if !strings.Contains(err.Error(), `unknown key "bogus"`) {
		t.Fatalf("expected unknown-key error, got: %v", err)
	}
}

func TestParseQuotaFlags_InvalidLimit(t *testing.T) {
	_, err := parseQuotaFlags([]string{"duration=PT1H,limit=ten,behavior=FAIL"})
	if err == nil {
		t.Fatal("expected error for non-integer limit")
	}
	if !strings.Contains(err.Error(), "limit must be an integer") {
		t.Fatalf("expected limit error, got: %v", err)
	}
}

func TestParseQuotaFlags_InvalidBehavior(t *testing.T) {
	_, err := parseQuotaFlags([]string{"duration=PT1H,limit=100,behavior=QUEUE"})
	if err == nil {
		t.Fatal("expected error for invalid quota behavior")
	}
	if !strings.Contains(err.Error(), "behavior must be FAIL or CANCEL") {
		t.Fatalf("expected behavior error, got: %v", err)
	}
}

const tenantWithLimitsJSON = `{"id":"my-tenant","name":"My Tenant","deleted":false,"concurrency":{"limit":10,"behavior":"QUEUE"},"quotas":[{"duration":"PT1H","limit":100,"behavior":"FAIL"}]}`

func TestRunTenantsList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/tenants/search") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[` + tenantWithLimitsJSON + `],"total":1}`))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runTenantsList(newTestClient(t, server.URL), 1, 100, nil, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runTenantsList error: %v", err)
	}
	if !strings.Contains(buf.String(), "my-tenant") || !strings.Contains(buf.String(), "Total tenants: 1") {
		t.Fatalf("unexpected output:\n%s", buf.String())
	}
}

func TestRunTenantsGet_RendersConcurrencyAndQuotas(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/tenants/my-tenant") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(tenantWithLimitsJSON))
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runTenantsGet(newTestClient(t, server.URL), "my-tenant", newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runTenantsGet error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "limit=10 behavior=QUEUE") {
		t.Fatalf("expected concurrency in output, got:\n%s", output)
	}
	if !strings.Contains(output, "PT1H") || !strings.Contains(output, "FAIL") {
		t.Fatalf("expected quota in output, got:\n%s", output)
	}
}

func TestRunTenantsCreate_SendsConcurrencyAndQuotas(t *testing.T) {
	var gotBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/configs" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"version":"2.0.0"}`))
			return
		}
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/tenants") {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(tenantWithLimitsJSON))
	}))
	t.Cleanup(server.Close)

	concurrency, err := parseConcurrencyFlags(10, true, "QUEUE", true)
	if err != nil {
		t.Fatal(err)
	}
	quotas, err := parseQuotaFlags([]string{"duration=PT1H,limit=100,behavior=FAIL"})
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	err = runTenantsCreate(newTestClient(t, server.URL), "my-tenant", "", concurrency, quotas, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runTenantsCreate error: %v", err)
	}

	if gotBody["name"] != "my-tenant" {
		t.Fatalf("expected name to default to the tenant ID, got: %v", gotBody)
	}
	sentConcurrency, ok := gotBody["concurrency"].(map[string]interface{})
	if !ok || sentConcurrency["limit"] != float64(10) || sentConcurrency["behavior"] != "QUEUE" {
		t.Fatalf("expected concurrency in request body, got: %v", gotBody)
	}
	sentQuotas, ok := gotBody["quotas"].([]interface{})
	if !ok || len(sentQuotas) != 1 {
		t.Fatalf("expected quotas in request body, got: %v", gotBody)
	}
}

func TestRunTenantsUpdate_PreservesUnsetFields(t *testing.T) {
	var gotBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(tenantWithLimitsJSON))
		case http.MethodPut:
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_, _ = w.Write([]byte(tenantWithLimitsJSON))
		}
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runTenantsUpdate(newTestClient(t, server.URL), "my-tenant", "Renamed", true, nil, nil, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runTenantsUpdate error: %v", err)
	}

	if gotBody["name"] != "Renamed" {
		t.Fatalf("expected name to be updated, got: %v", gotBody)
	}
	sentConcurrency, ok := gotBody["concurrency"].(map[string]interface{})
	if !ok || sentConcurrency["limit"] != float64(10) {
		t.Fatalf("expected pre-existing concurrency to be preserved, got: %v", gotBody)
	}
	sentQuotas, ok := gotBody["quotas"].([]interface{})
	if !ok || len(sentQuotas) != 1 {
		t.Fatalf("expected pre-existing quotas to be preserved, got: %v", gotBody)
	}
}

func TestRunTenantsUpdate_ReplacesQuotas(t *testing.T) {
	var gotBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(tenantWithLimitsJSON))
		case http.MethodPut:
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_, _ = w.Write([]byte(tenantWithLimitsJSON))
		}
	}))
	t.Cleanup(server.Close)

	quotas, err := parseQuotaFlags([]string{"duration=P1D,limit=1000,behavior=CANCEL"})
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	err = runTenantsUpdate(newTestClient(t, server.URL), "my-tenant", "", false, nil, quotas, newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runTenantsUpdate error: %v", err)
	}

	sentQuotas, ok := gotBody["quotas"].([]interface{})
	if !ok || len(sentQuotas) != 1 {
		t.Fatalf("expected quotas in request body, got: %v", gotBody)
	}
	sentQuota, _ := sentQuotas[0].(map[string]interface{})
	if sentQuota["duration"] != "P1D" || sentQuota["behavior"] != "CANCEL" {
		t.Fatalf("expected quotas to be replaced, got: %v", gotBody)
	}
}

func TestRunTenantsDelete_SkipConfirm(t *testing.T) {
	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runTenantsDelete(newTestClient(t, server.URL), "my-tenant", true, strings.NewReader(""), newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runTenantsDelete error: %v", err)
	}
	if gotMethod != http.MethodDelete || !strings.HasSuffix(gotPath, "/tenants/my-tenant") {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}
	if !strings.Contains(buf.String(), "deleted") {
		t.Fatalf("unexpected output:\n%s", buf.String())
	}
}

func TestRunTenantsDelete_Cancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("no request expected when the deletion is cancelled")
	}))
	t.Cleanup(server.Close)

	var buf bytes.Buffer
	err := runTenantsDelete(newTestClient(t, server.URL), "my-tenant", false, strings.NewReader("n\n"), newTableRenderer(&buf))
	if err != nil {
		t.Fatalf("runTenantsDelete error: %v", err)
	}
	if !strings.Contains(buf.String(), "Cancelled") {
		t.Fatalf("expected cancellation output, got:\n%s", buf.String())
	}
}
