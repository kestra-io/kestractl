package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	kestra "github.com/kestra-io/client-sdk/go-sdk/v2/kestra_api_client"
)

// decodeJSONPreservingNumbers decodes JSON into out with json.Decoder.UseNumber,
// so numbers land as json.Number (their original digits) instead of float64.
// Anything above 2^53 — a large ID, epoch nanos — otherwise loses precision the
// moment it is decoded, and re-renders wrong (#121). json.Number re-encodes as a
// bare JSON number, so both the table and the --output json paths stay exact.
// Trailing content after the first value is rejected, which plain
// json.Unmarshal does for free.
func decodeJSONPreservingNumbers(data []byte, out any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	if decoder.More() {
		return fmt.Errorf("unexpected trailing data after JSON value")
	}
	return nil
}

// rawGet issues a GET against the Kestra API and returns the response body
// unparsed, reusing the SDK's configured host, HTTP client, default headers and
// context-based authentication. It exists because the generated client decodes
// free-form values (KV values, namespace variables) into map[string]interface{}
// with plain json.Unmarshal, which loses integer precision above 2^53 before
// kestractl ever sees the payload; reading the bytes here lets the caller decode
// them with decodeJSONPreservingNumbers instead (#121).
//
// path must be the full API path, starting with a slash and already escaped.
func rawGet(client *Client, path string) ([]byte, error) {
	cfg := client.API.GetConfig()

	base := ""
	if len(cfg.Servers) > 0 {
		base = cfg.Servers[0].URL
	}
	if base == "" {
		base = cfg.Scheme + "://" + cfg.Host
	}
	base = strings.TrimRight(base, "/")

	req, err := http.NewRequestWithContext(client.Ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	// Reuse the auth the SDK stored on the context.
	if auth, ok := client.Ctx.Value(kestra.ContextBasicAuth).(kestra.BasicAuth); ok {
		req.SetBasicAuth(auth.UserName, auth.Password)
	}
	if token, ok := client.Ctx.Value(kestra.ContextAccessToken).(string); ok {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for h, v := range cfg.DefaultHeader {
		req.Header.Set(h, v)
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response from %s: %w", path, err)
	}
	if resp.StatusCode >= 400 {
		return nil, formatErrorBody(body, resp.Status)
	}

	return body, nil
}
