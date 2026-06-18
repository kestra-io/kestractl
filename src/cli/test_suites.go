package cli

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	kestra "github.com/kestra-io/client-sdk/go-sdk/kestra_api_client"
	"github.com/spf13/cobra"
)

func newTestSuitesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test-suites",
		Short: "Manage test suites (list, get, delete, run)",
	}

	cmd.AddCommand(newTestSuitesListCommand())
	cmd.AddCommand(newTestSuitesGetCommand())
	cmd.AddCommand(newTestSuitesDeleteCommand())
	cmd.AddCommand(newTestSuitesRunCommand())
	cmd.AddCommand(newTestSuitesCreateCommand())
	cmd.AddCommand(newTestSuitesUpdateCommand())
	cmd.AddCommand(newTestSuitesDeleteBulkCommand())
	cmd.AddCommand(newTestSuitesDisableBulkCommand())
	cmd.AddCommand(newTestSuitesEnableBulkCommand())
	cmd.AddCommand(newTestSuitesValidateCommand())
	cmd.AddCommand(newTestSuitesSearchResultsCommand())
	cmd.AddCommand(newTestSuitesLastResultCommand())
	cmd.AddCommand(newTestSuitesRunByQueryCommand())
	cmd.AddCommand(newTestSuitesGetResultCommand())

	return cmd
}

func newTestSuitesListCommand() *cobra.Command {
	var namespace, flowID string
	var page, size int32

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List test suites.",
		Long: `List test suites, optionally filtered by namespace or flow ID.

Results are paginated. Use --page and --size to navigate.`,
		Example: `  kestractl test-suites list
  kestractl test-suites list --namespace my.ns --flow-id my-flow
  kestractl test-suites list --output json`,
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runTestSuitesList(client, namespace, flowID, page, size, renderer)
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Filter by namespace")
	cmd.Flags().StringVar(&flowID, "flow-id", "", "Filter by flow ID")
	cmd.Flags().Int32Var(&page, "page", 1, "Page number")
	cmd.Flags().Int32Var(&size, "size", 50, "Page size")
	return cmd
}

func runTestSuitesList(client *Client, namespace, flowID string, page, size int32, renderer *Renderer) error {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 50
	}

	req := client.API.TestSuitesAPI.
		SearchTestSuites(client.Ctx, client.Tenant).
		Page(page).
		Size(size)
	if namespace != "" {
		req = req.Namespace(namespace)
	}
	if flowID != "" {
		req = req.FlowId(flowID)
	}

	resp, _, err := req.Execute()
	if err != nil {
		return formatSDKError(err)
	}

	suites := resp.GetResults()
	result := make([]map[string]any, len(suites))
	for i, s := range suites {
		result[i] = map[string]any{
			"id":          s.GetId(),
			"namespace":   s.GetNamespace(),
			"flowId":      s.GetFlowId(),
			"description": s.GetDescription(),
			"disabled":    s.GetDisabled(),
		}
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ID\tNAMESPACE\tFLOW\tDESCRIPTION\tDISABLED")
		for _, row := range result {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%v\n",
				stringify(row["id"]),
				stringify(row["namespace"]),
				stringify(row["flowId"]),
				stringify(row["description"]),
				row["disabled"],
			)
		}
		fmt.Fprintf(w, "\nShowing %d test suite(s) (page %d, total %d)\n", len(result), page, resp.GetTotal())
		return nil
	})
}

func newTestSuitesGetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <namespace> <id>",
		Short: "Get a test suite by namespace and ID.",
		Example: `  kestractl test-suites get my.namespace my-test-suite
  kestractl test-suites get my.namespace my-test-suite --output json`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runTestSuitesGet(client, args[0], args[1], renderer)
		},
	}
	return cmd
}

func runTestSuitesGet(client *Client, namespace, id string, renderer *Renderer) error {
	suite, _, err := client.API.TestSuitesAPI.
		TestSuite(client.Ctx, namespace, id, client.Tenant).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	result := map[string]any{
		"id":          suite.GetId(),
		"namespace":   suite.GetNamespace(),
		"flowId":      suite.GetFlowId(),
		"description": suite.GetDescription(),
		"disabled":    suite.GetDisabled(),
		"testCases":   len(suite.GetTestCases()),
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "ID\t%s\n", suite.GetId())
		fmt.Fprintf(w, "NAMESPACE\t%s\n", suite.GetNamespace())
		fmt.Fprintf(w, "FLOW\t%s\n", suite.GetFlowId())
		if desc := suite.GetDescription(); desc != "" {
			fmt.Fprintf(w, "DESCRIPTION\t%s\n", desc)
		}
		fmt.Fprintf(w, "DISABLED\t%v\n", suite.GetDisabled())
		fmt.Fprintf(w, "TEST CASES\t%d\n", len(suite.GetTestCases()))
		return nil
	})
}

func newTestSuitesDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <namespace> <id>",
		Short: "Delete a test suite.",
		Example: `  kestractl test-suites delete my.namespace my-test-suite`,
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runTestSuitesDelete(client, args[0], args[1], renderer)
		},
	}
	return cmd
}

func runTestSuitesDelete(client *Client, namespace, id string, renderer *Renderer) error {
	_, _, err := client.API.TestSuitesAPI.
		DeleteTestSuite(client.Ctx, namespace, id, client.Tenant).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	result := map[string]any{"namespace": namespace, "id": id, "deleted": true}
	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Test suite '%s/%s' deleted.\n", namespace, id)
		return nil
	})
}

func newTestSuitesRunCommand() *cobra.Command {
	var testCases []string

	cmd := &cobra.Command{
		Use:   "run <namespace> <id>",
		Short: "Run a test suite.",
		Example: `  kestractl test-suites run my.namespace my-test-suite
  kestractl test-suites run my.namespace my-test-suite --test-case case1 --test-case case2
  kestractl test-suites run my.namespace my-test-suite --output json`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runTestSuitesRun(client, args[0], args[1], testCases, renderer)
		},
	}

	cmd.Flags().StringArrayVar(&testCases, "test-case", nil, "Run only specific test cases (repeatable)")
	return cmd
}

func newTestSuitesCreateCommand() *cobra.Command {
	var file string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a test suite from a YAML file.",
		Example: `  kestractl test-suites create --file my-test-suite.yaml
  kestractl test-suites create --file my-test-suite.yaml --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runTestSuitesCreate(client, file, renderer)
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "", "Path to YAML file containing the test suite definition (required)")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func runTestSuitesCreate(client *Client, file string, renderer *Renderer) error {
	content, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("failed to read file '%s': %w", file, err)
	}

	suite, _, err := client.API.TestSuitesAPI.
		CreateTestSuite(client.Ctx, client.Tenant).
		Body(string(content)).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	result := map[string]any{
		"id":        suite.GetId(),
		"namespace": suite.GetNamespace(),
		"flowId":    suite.GetFlowId(),
	}
	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Test suite '%s' created in namespace '%s'.\n", suite.GetId(), suite.GetNamespace())
		return nil
	})
}

func newTestSuitesUpdateCommand() *cobra.Command {
	var file string

	cmd := &cobra.Command{
		Use:   "update <namespace> <id>",
		Short: "Update a test suite from a YAML file.",
		Example: `  kestractl test-suites update my.namespace my-test-suite --file updated.yaml
  kestractl test-suites update my.namespace my-test-suite --file updated.yaml --output json`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runTestSuitesUpdate(client, args[0], args[1], file, renderer)
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "", "Path to YAML file containing the updated test suite definition (required)")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func runTestSuitesUpdate(client *Client, namespace, id, file string, renderer *Renderer) error {
	content, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("failed to read file '%s': %w", file, err)
	}

	suite, _, err := client.API.TestSuitesAPI.
		UpdateTestSuite(client.Ctx, namespace, id, client.Tenant).
		Body(string(content)).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	result := map[string]any{
		"id":        suite.GetId(),
		"namespace": suite.GetNamespace(),
		"flowId":    suite.GetFlowId(),
	}
	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Test suite '%s' in namespace '%s' updated.\n", suite.GetId(), suite.GetNamespace())
		return nil
	})
}

func runTestSuitesRun(client *Client, namespace, id string, testCases []string, renderer *Renderer) error {
	runReq := kestra.NewTestSuiteControllerRunRequest()
	if len(testCases) > 0 {
		runReq.SetTestCases(testCases)
	}

	result, _, err := client.API.TestSuitesAPI.
		RunTestSuite(client.Ctx, namespace, id, client.Tenant).
		TestSuiteControllerRunRequest(*runReq).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	passed, failed := 0, 0
	for _, r := range result.GetResults() {
		if r.GetState() == kestra.TESTSTATE_SUCCESS {
			passed++
		} else {
			failed++
		}
	}

	jsonResult := map[string]any{
		"id":          result.GetId(),
		"testSuiteId": result.GetTestSuiteId(),
		"namespace":   result.GetNamespace(),
		"flowId":      result.GetFlowId(),
		"state":       string(result.GetState()),
		"passed":      passed,
		"failed":      failed,
	}
	if !result.GetStartDate().IsZero() {
		jsonResult["startDate"] = result.GetStartDate().Format(time.RFC3339)
	}

	return renderer.Render(jsonResult, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "SUITE\t%s\n", result.GetTestSuiteId())
		fmt.Fprintf(w, "NAMESPACE\t%s\n", result.GetNamespace())
		fmt.Fprintf(w, "FLOW\t%s\n", result.GetFlowId())
		fmt.Fprintf(w, "STATE\t%s\n", string(result.GetState()))
		fmt.Fprintf(w, "PASSED\t%d\n", passed)
		fmt.Fprintf(w, "FAILED\t%d\n", failed)
		return nil
	})
}

func newTestSuitesValidateCommand() *cobra.Command {
	var filePath string

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate a test suite YAML definition.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutputFormat(); err != nil {
				return err
			}
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			if filePath == "" {
				return fmt.Errorf("--file is required")
			}
			content, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read file: %w", err)
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runTestSuitesValidate(client, string(content), renderer)
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Path to the test suite YAML file (required)")

	return cmd
}

func runTestSuitesValidate(client *Client, body string, renderer *Renderer) error {
	result, _, err := client.API.TestSuitesAPI.
		ValidateTestSuite(client.Ctx, client.Tenant).
		Body(body).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	constraints := result.GetConstraints()
	warnings := result.GetWarnings()
	infos := result.GetInfos()

	jsonResult := map[string]any{
		"namespace":   result.GetNamespace(),
		"flow":        result.GetFlow(),
		"constraints": constraints,
		"warnings":    warnings,
		"infos":       infos,
		"outdated":    result.GetOutdated(),
	}

	return renderer.Render(jsonResult, func(w *tabwriter.Writer) error {
		if constraints != "" {
			fmt.Fprintf(w, "VIOLATIONS\t%s\n", constraints)
		}
		if len(warnings) > 0 {
			fmt.Fprintf(w, "WARNINGS\t%d\n", len(warnings))
		}
		if len(infos) > 0 {
			fmt.Fprintf(w, "INFOS\t%d\n", len(infos))
		}
		if constraints == "" {
			fmt.Fprintln(w, "Validation passed.")
		}
		return nil
	})
}

func parseTestSuiteIds(args []string) ([]kestra.TestSuiteControllerTestSuiteApiId, error) {
	ids := make([]kestra.TestSuiteControllerTestSuiteApiId, 0, len(args))
	for _, arg := range args {
		parts := strings.SplitN(arg, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid format %q: expected <namespace>/<id>", arg)
		}
		id := kestra.NewTestSuiteControllerTestSuiteApiId(parts[0], parts[1])
		ids = append(ids, *id)
	}
	return ids, nil
}

func newTestSuitesBulkOp(use, short, op string) *cobra.Command {
	return &cobra.Command{
		Use:     fmt.Sprintf("%s <namespace/id>...", use),
		Short:   short,
		Example: fmt.Sprintf("  kestractl test-suites %s my.namespace/my-suite", use),
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			ids, err := parseTestSuiteIds(args)
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runTestSuitesBulkOp(client, ids, op, renderer)
		},
	}
}

func newTestSuitesDeleteBulkCommand() *cobra.Command {
	return newTestSuitesBulkOp("delete-bulk", "Delete multiple test suites.", "delete")
}

func newTestSuitesDisableBulkCommand() *cobra.Command {
	return newTestSuitesBulkOp("disable-bulk", "Disable multiple test suites.", "disable")
}

func newTestSuitesEnableBulkCommand() *cobra.Command {
	return newTestSuitesBulkOp("enable-bulk", "Enable multiple test suites.", "enable")
}

func runTestSuitesBulkOp(client *Client, ids []kestra.TestSuiteControllerTestSuiteApiId, op string, renderer *Renderer) error {
	req := kestra.NewTestSuiteControllerTestSuiteBulkRequest(ids)

	var resp *kestra.BulkResponse
	var err error

	switch op {
	case "delete":
		resp, _, err = client.API.TestSuitesAPI.
			DeleteTestSuitesByIds(client.Ctx, client.Tenant).
			TestSuiteControllerTestSuiteBulkRequest(*req).Execute()
	case "disable":
		resp, _, err = client.API.TestSuitesAPI.
			DisableTestSuitesByIds(client.Ctx, client.Tenant).
			TestSuiteControllerTestSuiteBulkRequest(*req).Execute()
	default: // enable
		resp, _, err = client.API.TestSuitesAPI.
			EnableTestSuitesByIds(client.Ctx, client.Tenant).
			TestSuiteControllerTestSuiteBulkRequest(*req).Execute()
	}

	if err != nil {
		return formatSDKError(err)
	}

	count := int32(0)
	if resp != nil {
		count = resp.GetCount()
	}

	result := map[string]any{"operation": op, "count": count}
	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Bulk %s: %d test suite(s) affected.\n", op, count)
		return nil
	})
}

func newTestSuitesSearchResultsCommand() *cobra.Command {
	var (
		page        int32
		size        int32
		sort        []string
		testSuiteID string
		namespace   string
		flowID      string
	)

	cmd := &cobra.Command{
		Use:   "search-results",
		Short: "Search test suite run results.",
		Long:  "Search for test suite run results with optional filters.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutputFormat(); err != nil {
				return err
			}
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runTestSuitesSearchResults(client, page, size, sort, testSuiteID, namespace, flowID, renderer)
		},
	}

	cmd.Flags().Int32Var(&page, "page", 1, "Page number")
	cmd.Flags().Int32Var(&size, "size", 100, "Page size")
	cmd.Flags().StringArrayVar(&sort, "sort", nil, "Sort expression (repeatable)")
	cmd.Flags().StringVar(&testSuiteID, "test-suite-id", "", "Filter by test suite ID")
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Filter by namespace")
	cmd.Flags().StringVar(&flowID, "flow-id", "", "Filter by flow ID")

	return cmd
}

func runTestSuitesSearchResults(client *Client, page, size int32, sort []string, testSuiteID, namespace, flowID string, renderer *Renderer) error {
	req := client.API.TestSuitesAPI.
		SearchTestSuitesResults(client.Ctx, client.Tenant).
		Page(page).
		Size(size)
	if len(sort) > 0 {
		req = req.Sort(sort)
	}
	if testSuiteID != "" {
		req = req.TestSuiteId(testSuiteID)
	}
	if namespace != "" {
		req = req.Namespace(namespace)
	}
	if flowID != "" {
		req = req.FlowId(flowID)
	}

	resp, _, err := req.Execute()
	if err != nil {
		return formatSDKError(err)
	}

	results := resp.GetResults()

	return renderer.Render(results, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ID\tTEST SUITE ID\tNAMESPACE\tFLOW ID\tSTATE")
		for _, r := range results {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%v\n",
				r.GetId(), r.GetTestSuiteId(), r.GetNamespace(), r.GetFlowId(), r.GetState())
		}
		fmt.Fprintf(w, "\nTotal: %d\n", resp.GetTotal())
		return nil
	})
}

func newTestSuitesLastResultCommand() *cobra.Command {
	var ids []string

	cmd := &cobra.Command{
		Use:   "last-result",
		Short: "Get last test suite run results for given IDs.",
		Long:  "Retrieve the last run results for a set of test suites specified by namespace/id pairs.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutputFormat(); err != nil {
				return err
			}
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runTestSuitesLastResult(client, ids, renderer)
		},
	}

	cmd.Flags().StringArrayVar(&ids, "id", nil, "Test suite id as namespace/id (repeatable)")

	return cmd
}

func runTestSuitesLastResult(client *Client, rawIDs []string, renderer *Renderer) error {
	suiteIDs, err := parseTestSuiteIds(rawIDs)
	if err != nil {
		return err
	}

	body := kestra.NewTestSuiteControllerSearchTestsLastResult()
	if len(suiteIDs) > 0 {
		body.SetTestSuiteIds(suiteIDs)
	}

	resp, _, err := client.API.TestSuitesAPI.
		TestsLastResult(client.Ctx, client.Tenant).
		TestSuiteControllerSearchTestsLastResult(*body).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	results := resp.GetResults()

	return renderer.Render(results, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "ID\tTEST SUITE ID\tNAMESPACE\tFLOW ID\tSTATE")
		for _, r := range results {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%v\n",
				r.GetId(), r.GetTestSuiteId(), r.GetNamespace(), r.GetFlowId(), r.GetState())
		}
		fmt.Fprintf(w, "\nShowing %d result(s)\n", len(results))
		return nil
	})
}

func newTestSuitesRunByQueryCommand() *cobra.Command {
	var (
		namespace             string
		flowID                string
		includeChildNamespace bool
	)

	cmd := &cobra.Command{
		Use:   "run-by-query",
		Short: "Run test suites matching a query.",
		Long:  "Run multiple test suites that match the given namespace/flow filters.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutputFormat(); err != nil {
				return err
			}
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runTestSuitesRunByQuery(client, namespace, flowID, includeChildNamespace, renderer)
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Namespace filter")
	cmd.Flags().StringVar(&flowID, "flow-id", "", "Flow ID filter")
	cmd.Flags().BoolVar(&includeChildNamespace, "include-child-namespaces", false, "Include child namespaces")

	return cmd
}

func runTestSuitesRunByQuery(client *Client, namespace, flowID string, includeChildNamespaces bool, renderer *Renderer) error {
	body := kestra.NewTestSuiteServiceRunByQueryRequest(includeChildNamespaces)
	if namespace != "" {
		body.SetNamespace(namespace)
	}
	if flowID != "" {
		body.SetFlowId(flowID)
	}

	resp, _, err := client.API.TestSuitesAPI.
		RunTestSuitesByQuery(client.Ctx, client.Tenant).
		TestSuiteServiceRunByQueryRequest(*body).
		Execute()
	if err != nil {
		return formatSDKError(err)
	}

	count := resp.GetNumberOfTestSuitesToBeRun()
	caseCount := resp.GetNumberOfTestCasesToBeRun()

	return renderer.Render(map[string]any{
		"numberOfTestSuitesToBeRun": count,
		"numberOfTestCasesToBeRun":  caseCount,
	}, func(w *tabwriter.Writer) error {
		fmt.Fprintf(w, "Queued %d test suite(s) with %d test case(s) to run.\n", count, caseCount)
		return nil
	})
}

func newTestSuitesGetResultCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get-result <result_id>",
		Short: "Get a specific test suite run result.",
		Long:  "Retrieve a test suite run result by its result ID.",
		Example: `  kestractl test-suites get-result 01JXA1B2C3D4E5F6G7H8I9J0K1
  kestractl test-suites get-result 01JXA1B2C3D4E5F6G7H8I9J0K1 --output json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			client, err := newClientFunc()
			if err != nil {
				return err
			}
			return runTestSuitesGetResult(client, args[0], renderer)
		},
	}
	return cmd
}

func runTestSuitesGetResult(client *Client, id string, renderer *Renderer) error {
	result, err := client.Kestra.TestSuites().TestResult(client.Ctx, id, client.Tenant)
	if err != nil {
		return formatSDKError(err)
	}

	return renderer.Render(result, func(w *tabwriter.Writer) error {
		fmt.Fprintln(w, "Test Suite Run Result")
		fmt.Fprintln(w)
		fmt.Fprintf(w, "ID:\t%s\n", result.GetId())
		fmt.Fprintf(w, "Test Suite ID:\t%s\n", result.GetTestSuiteId())
		fmt.Fprintf(w, "Namespace:\t%s\n", result.GetNamespace())
		fmt.Fprintf(w, "Flow ID:\t%s\n", result.GetFlowId())
		fmt.Fprintf(w, "State:\t%v\n", result.GetState())
		if sd := result.GetStartDate(); !sd.IsZero() {
			fmt.Fprintf(w, "Start Date:\t%s\n", sd.Format("2006-01-02T15:04:05Z"))
		}
		if ed := result.GetEndDate(); !ed.IsZero() {
			fmt.Fprintf(w, "End Date:\t%s\n", ed.Format("2006-01-02T15:04:05Z"))
		}
		return nil
	})
}
