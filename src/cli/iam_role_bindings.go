package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"text/tabwriter"

	kestra "github.com/kestra-io/client-sdk/go-sdk/kestra_api_client"
	"github.com/spf13/cobra"
)

type iamRoleBindingOptions struct {
	Role  string
	User  string
	Group string
}

type iamRoleBindingTarget struct {
	Type string
	ID   string
	Name string
}

func newIamRolesAttachCommand() *cobra.Command {
	var opts iamRoleBindingOptions

	cmd := &cobra.Command{
		Use:   "attach",
		Short: "Attach an IAM role to a user or group.",
		Example: `  # Attach a role to a user
	  kestra iam roles attach --role ops --user usr_123

	  # Attach a role to a group
	  kestra iam roles attach --role ops --group grp_456`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutputFormat(); err != nil {
				return err
			}

			client, err := NewClient()
			if err != nil {
				return err
			}

			if err := validateIamRoleBindingFlags(opts); err != nil {
				return err
			}

			return runIamRolesBindingsAttach(client, opts)
		},
	}

	cmd.Flags().StringVar(&opts.Role, "role", "", "Role ID or name to attach (required)")
	cmd.Flags().StringVar(&opts.User, "user", "", "User ID, username, or display name to attach")
	cmd.Flags().StringVar(&opts.Group, "group", "", "Group ID or name to attach")

	_ = cmd.MarkFlagRequired("role")

	return cmd
}

func newIamRolesDetachCommand() *cobra.Command {
	var opts iamRoleBindingOptions

	cmd := &cobra.Command{
		Use:   "detach",
		Short: "Detach an IAM role from a user or group.",
		Example: `  # Detach a role from a user
	  kestra iam roles detach --role ops --user usr_123

	  # Detach a role from a group
	  kestra iam roles detach --role ops --group grp_456`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutputFormat(); err != nil {
				return err
			}

			client, err := NewClient()
			if err != nil {
				return err
			}

			if err := validateIamRoleBindingFlags(opts); err != nil {
				return err
			}

			return runIamRolesBindingsDetach(client, opts)
		},
	}

	cmd.Flags().StringVar(&opts.Role, "role", "", "Role ID or name to detach (required)")
	cmd.Flags().StringVar(&opts.User, "user", "", "User ID, username, or display name to detach")
	cmd.Flags().StringVar(&opts.Group, "group", "", "Group ID or name to detach")

	_ = cmd.MarkFlagRequired("role")

	return cmd
}

func validateIamRoleBindingFlags(opts iamRoleBindingOptions) error {
	role := strings.TrimSpace(opts.Role)
	user := strings.TrimSpace(opts.User)
	group := strings.TrimSpace(opts.Group)

	if role == "" {
		return fmt.Errorf("role is required")
	}
	if user == "" && group == "" {
		return fmt.Errorf("either --user or --group is required")
	}
	if user != "" && group != "" {
		return fmt.Errorf("only one of --user or --group can be set")
	}

	return nil
}

func runIamRolesBindingsAttach(client *Client, opts iamRoleBindingOptions) error {
	role, err := resolveIamRoleIdentifier(client, opts.Role)
	if err != nil {
		return err
	}

	target, bindingType, err := resolveIamBindingTarget(client, opts)
	if err != nil {
		return err
	}

	request := kestra.NewIAMBindingControllerApiCreateBindingRequest(bindingType, target.ID, role.ID)
	_, err = createIamBinding(client, request)
	if err != nil {
		return err
	}

	return printIamRoleBindingResult("attach", role, target)
}

func runIamRolesBindingsDetach(client *Client, opts iamRoleBindingOptions) error {
	role, err := resolveIamRoleIdentifier(client, opts.Role)
	if err != nil {
		return err
	}

	target, bindingType, err := resolveIamBindingTarget(client, opts)
	if err != nil {
		return err
	}

	bindings, err := searchIamBindings(client, bindingType, target.ID)
	if err != nil {
		return err
	}

	matching := filterBindingsByRole(bindings, role.ID)
	if len(matching) == 0 {
		return fmt.Errorf("binding not found for %s %s and role %s", target.Type, target.ID, role.ID)
	}
	if len(matching) > 1 {
		return fmt.Errorf("multiple bindings matched for %s %s and role %s: %s", target.Type, target.ID, role.ID, strings.Join(matching, ", "))
	}

	if err := deleteIamBinding(client, matching[0]); err != nil {
		return err
	}

	return printIamRoleBindingResult("detach", role, target)
}

func resolveIamBindingTarget(client *Client, opts iamRoleBindingOptions) (*iamRoleBindingTarget, kestra.BindingType, error) {
	if strings.TrimSpace(opts.User) != "" {
		user, err := resolveIamUserIdentifier(client, opts.User)
		if err != nil {
			return nil, "", err
		}
		return &iamRoleBindingTarget{Type: "user", ID: user.ID, Name: user.Name}, kestra.BINDINGTYPE_USER, nil
	}

	group, err := resolveIamGroupIdentifier(client, opts.Group)
	if err != nil {
		return nil, "", err
	}
	return &iamRoleBindingTarget{Type: "group", ID: group.ID, Name: group.Name}, kestra.BINDINGTYPE_GROUP, nil
}

func printIamRoleBindingResult(action string, role *iamResolvedIdentifier, target *iamRoleBindingTarget) error {
	result := map[string]any{
		"action": strings.ToLower(action),
		"role": map[string]any{
			"id":   role.ID,
			"name": role.Name,
		},
		"target": map[string]any{
			"type": target.Type,
			"id":   target.ID,
			"name": target.Name,
		},
	}

	renderer, err := NewRendererFromFlags(nil)
	if err != nil {
		return err
	}
	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ACTION\tTARGET_TYPE\tTARGET_ID\tTARGET_NAME\tROLE_ID\tROLE_NAME")
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			strings.ToUpper(action),
			strings.ToUpper(target.Type),
			withFallback(target.ID),
			withFallback(target.Name),
			withFallback(role.ID),
			withFallback(role.Name),
		)
		return nil
	})
}

func createIamBinding(client *Client, request *kestra.IAMBindingControllerApiCreateBindingRequest) (*kestra.IAMBindingControllerApiBindingDetail, error) {
	baseURL, err := bindingBaseURL(client)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/api/v1/%s/bindings", baseURL, url.PathEscape(client.Tenant))
	var response kestra.IAMBindingControllerApiBindingDetail
	if err := doBindingRequest(client, http.MethodPost, endpoint, request, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

func searchIamBindings(client *Client, bindingType kestra.BindingType, externalID string) ([]kestra.IAMBindingControllerApiBindingDetail, error) {
	baseURL, err := bindingBaseURL(client)
	if err != nil {
		return nil, err
	}

	query := url.Values{}
	query.Set("page", "1")
	query.Set("size", "1000")
	query.Set("type", string(bindingType))
	query.Set("id", externalID)

	endpoint := fmt.Sprintf("%s/api/v1/%s/bindings/search?%s", baseURL, url.PathEscape(client.Tenant), query.Encode())
	var response kestra.PagedResultsIAMBindingControllerApiBindingDetail
	if err := doBindingRequest(client, http.MethodGet, endpoint, nil, &response); err != nil {
		return nil, err
	}

	return response.GetResults(), nil
}

func deleteIamBinding(client *Client, bindingID string) error {
	baseURL, err := bindingBaseURL(client)
	if err != nil {
		return err
	}

	endpoint := fmt.Sprintf("%s/api/v1/%s/bindings/%s", baseURL, url.PathEscape(client.Tenant), url.PathEscape(bindingID))
	return doBindingRequest(client, http.MethodDelete, endpoint, nil, nil)
}

func filterBindingsByRole(bindings []kestra.IAMBindingControllerApiBindingDetail, roleID string) []string {
	ids := make([]string, 0)
	for _, binding := range bindings {
		role, ok := binding.GetRoleOk()
		if !ok || role == nil {
			continue
		}
		if role.GetId() != roleID {
			continue
		}
		bindingID := binding.GetId()
		if bindingID != "" {
			ids = append(ids, bindingID)
		}
	}

	return ids
}

func bindingBaseURL(client *Client) (string, error) {
	cfg := client.API.GetConfig()
	baseURL, err := cfg.ServerURLWithContext(client.Ctx, "BindingsAPIService")
	if err != nil {
		return "", err
	}
	return strings.TrimRight(baseURL, "/"), nil
}

func doBindingRequest(client *Client, method string, endpoint string, payload any, out any) error {
	cfg := client.API.GetConfig()
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return err
	}

	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	for header, value := range cfg.DefaultHeader {
		req.Header.Add(header, value)
	}
	if cfg.UserAgent != "" {
		req.Header.Add("User-Agent", cfg.UserAgent)
	}

	applyBindingAuth(req, client.Ctx)

	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode >= 300 {
		return formatBindingError(resp, respBody)
	}

	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return err
		}
	}

	return nil
}

func applyBindingAuth(req *http.Request, ctx context.Context) {
	if ctx == nil {
		return
	}
	if token, ok := ctx.Value(kestra.ContextAccessToken).(string); ok && strings.TrimSpace(token) != "" {
		req.Header.Add("Authorization", "Bearer "+token)
		return
	}
	if basicAuth, ok := ctx.Value(kestra.ContextBasicAuth).(kestra.BasicAuth); ok {
		req.SetBasicAuth(basicAuth.UserName, basicAuth.Password)
	}
}

func formatBindingError(resp *http.Response, body []byte) error {
	if len(body) > 0 {
		var jsonErr map[string]any
		if json.Unmarshal(body, &jsonErr) == nil {
			if msg, ok := jsonErr["message"].(string); ok && msg != "" {
				return fmt.Errorf("API error: %s", msg)
			}
			if msg, ok := jsonErr["error"].(string); ok && msg != "" {
				return fmt.Errorf("API error: %s", msg)
			}
		}
	}

	bodyStr := strings.TrimSpace(string(body))
	if strings.HasPrefix(bodyStr, "<") {
		if resp != nil {
			return fmt.Errorf("API request failed: %s (received HTML response instead of JSON)", resp.Status)
		}
		return fmt.Errorf("API request failed: received HTML response instead of JSON")
	}

	if resp != nil {
		return fmt.Errorf("API error: %s", resp.Status)
	}

	return fmt.Errorf("API request failed")
}
