package buildkite

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	workflowTestSuiteUUID = "123e4567-e89b-42d3-a456-426614174000"
	workflowTestAddress   = "buildkite_test_suite_workflow.test"
	workflowTestMonitor   = `type = "transition_count"
window_size = 73
alarm_threshold = 0.127
recover_threshold = 0`
	workflowTestBody = `{
		"monitor":{"type":"transition_count","window_size":73,"alarm_threshold":0.127,"recover_threshold":0},
		"filters":[{"key":"scm.branch","operator":"is","value":"main"},{"key":"env","operator":"is_not","value":"staging"}],
		"alarm_actions":[{"type":"add_label","label":"flaky"},{"type":"send_webhook_notification","endpoint":"https://example.com/secret"}],
		"recover_actions":[{"type":"remove_label","label":"flaky"}]
	}`
	workflowTestCollections = `
filters = [
  { key = "scm.branch", operator = "is", value = "main" },
  { key = "env", operator = "is_not", value = "staging" }
]
alarm_actions = [
  { type = "add_label", label = "flaky" },
  { type = "send_webhook_notification", endpoint = "https://example.com/secret" }
]
recover_actions = [{ type = "remove_label", label = "flaky" }]
`
)

// The response is built from an independent API fixture, not echoed from the request. Reversing
// each array also makes an accidentally list-valued resource fail its post-apply consistency check.
type workflowTestAPI struct {
	t           *testing.T
	server      *httptest.Server
	mu          sync.Mutex
	want        map[string]any
	workflows   map[string]map[string]any
	creates     int
	status      int
	maskWebhook bool
}

func newWorkflowTestAPI(t *testing.T, body string) *workflowTestAPI {
	t.Helper()
	api := &workflowTestAPI{t: t, workflows: map[string]map[string]any{}}
	if err := json.Unmarshal([]byte(body), &api.want); err != nil {
		t.Fatal(err)
	}
	api.server = httptest.NewServer(http.HandlerFunc(api.handle))
	t.Cleanup(api.server.Close)
	return api
}

func (a *workflowTestAPI) handle(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if r.Header.Get("Authorization") != "Bearer test" {
		a.t.Error("request did not use the configured test token")
	}
	if r.Method == http.MethodGet && r.URL.Path == "/v2/analytics/organizations/test/suites/application" {
		_, _ = fmt.Fprintf(w, `{"id":%q,"graphql_id":"suite-graphql-id","slug":"application"}`, workflowTestSuiteUUID)
		return
	}
	collection := "/v2/analytics/organizations/test/suites/" + workflowTestSuiteUUID + "/workflows"
	creating := r.Method == http.MethodPost && r.URL.Path == collection
	updating := r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, collection+"/")
	if creating || updating {
		id := strings.TrimPrefix(r.URL.Path, collection+"/")
		if updating {
			if _, exists := a.workflows[id]; !exists {
				w.WriteHeader(http.StatusNotFound)
				return
			}
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			a.t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		// Set iteration order is immaterial; compare each list by its contents.
		for _, key := range []string{"filters", "alarm_actions", "recover_actions"} {
			actual, ok := body[key].([]any)
			expected := a.want[key].([]any)
			if !ok || len(actual) != len(expected) {
				a.t.Errorf("%s: got %v, want %v", key, body[key], expected)
				continue
			}
			for _, item := range expected {
				if !slices.ContainsFunc(actual, func(v any) bool { return reflect.DeepEqual(v, item) }) {
					a.t.Errorf("%s missing expected item %v", key, item)
				}
			}
		}
		if !reflect.DeepEqual(body["monitor"], a.want["monitor"]) || len(body) != 4 {
			a.t.Errorf("unexpected workflow payload: %#v", body)
		}
		if a.status != 0 {
			w.WriteHeader(a.status)
			_, _ = w.Write([]byte(`{"message":"Validation Failed","errors":[{"field":"/alarm_actions/0/label","code":"invalid"}]}`))
			return
		}
		if creating {
			a.creates++
			id = fmt.Sprintf("123e4567-e89b-42d3-a456-%012d", a.creates)
		}
		encoded, _ := json.Marshal(a.want)
		var response map[string]any
		if err := json.Unmarshal(encoded, &response); err != nil {
			a.t.Error(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		for _, key := range []string{"filters", "alarm_actions", "recover_actions"} {
			slices.Reverse(response[key].([]any))
		}
		response["id"] = id
		response["url"] = "https://api.buildkite.com" + collection + "/" + id
		response["web_url"] = "https://buildkite.com/workflows/" + id + "/events"
		a.workflows[id] = response
		if creating {
			w.WriteHeader(http.StatusCreated)
		}
		encoded, _ = json.Marshal(response)
		if a.maskWebhook {
			encoded = []byte(strings.ReplaceAll(string(encoded), "https://example.com/secret", "https://example.com/[REDACTED]"))
		}
		_, _ = w.Write(encoded)
		return
	}
	if !strings.HasPrefix(r.URL.Path, collection+"/") {
		a.t.Errorf("unexpected workflow request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, collection+"/")
	if a.status != 0 {
		w.WriteHeader(a.status)
		_, _ = w.Write([]byte(`{"message":"permission denied; not found is not a status code"}`))
		return
	}
	response, exists := a.workflows[id]
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		encoded, _ := json.Marshal(response)
		if a.maskWebhook {
			encoded = []byte(strings.ReplaceAll(string(encoded), "https://example.com/secret", "https://example.com/[REDACTED]"))
		}
		_, _ = w.Write(encoded)
	case http.MethodDelete:
		delete(a.workflows, id)
		w.WriteHeader(http.StatusNoContent)
	default:
		a.t.Errorf("unexpected method: %s", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func workflowTestConfig(apiURL, suite, monitor, collections string) string {
	return fmt.Sprintf(`
provider "buildkite" {
  organization = "test"
  api_token = "test"
  rest_url = %q
  max_retries = 0
}
resource "buildkite_test_suite_workflow" "test" {
  test_suite_uuid = %q
  monitor = {
    %s
  }
  %s
}
`, apiURL, suite, monitor, collections)
}

func TestTestSuiteWorkflowLifecycle(t *testing.T) {
	api := newWorkflowTestAPI(t, workflowTestBody)
	// The branch value is unknown during planning, just as it can be when referencing another resource.
	collections := strings.Replace(workflowTestCollections, `value = "main"`, `value = terraform_data.branch.output`, 1)
	config := workflowTestConfig(api.server.URL, workflowTestSuiteUUID, workflowTestMonitor, collections) + `
resource "terraform_data" "branch" {
  input = "main"
}`
	checks := resource.ComposeAggregateTestCheckFunc(
		resource.TestCheckResourceAttr(workflowTestAddress, "id", "123e4567-e89b-42d3-a456-000000000001"),
		resource.TestCheckResourceAttr(workflowTestAddress, "test_suite_uuid", workflowTestSuiteUUID),
		resource.TestCheckResourceAttr(workflowTestAddress, "monitor.window_size", "73"),
		resource.TestCheckResourceAttr(workflowTestAddress, "monitor.alarm_threshold", "0.127"),
		resource.TestCheckResourceAttr(workflowTestAddress, "monitor.recover_threshold", "0"),
		resource.TestCheckResourceAttr(workflowTestAddress, "filters.#", "2"),
		resource.TestCheckResourceAttr(workflowTestAddress, "alarm_actions.#", "2"),
		resource.TestCheckTypeSetElemNestedAttrs(workflowTestAddress, "alarm_actions.*", map[string]string{"type": "send_webhook_notification", "endpoint": "https://example.com/secret"}),
		resource.TestCheckResourceAttrSet(workflowTestAddress, "url"),
		resource.TestCheckResourceAttrSet(workflowTestAddress, "web_url"),
	)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy: func(*terraform.State) error {
			api.mu.Lock()
			defer api.mu.Unlock()
			if len(api.workflows) != 0 || api.creates != 1 {
				return fmt.Errorf("expected one creation and no remaining workflows: creates=%d remaining=%d", api.creates, len(api.workflows))
			}
			return nil
		},
		Steps: []resource.TestStep{
			{Config: config, Check: checks},
			{
				ResourceName: workflowTestAddress, ImportState: true,
				ImportStateId: "application/123e4567-e89b-42d3-a456-000000000099",
				ExpectError:   regexp.MustCompile("Cannot import non-existent remote object"),
			},
			{
				ResourceName: workflowTestAddress, ImportState: true,
				ImportStateId: "application/123e4567-e89b-42d3-a456-000000000001", ImportStateVerify: true,
			},
			{Config: config, PlanOnly: true},
			{PreConfig: func() {
				api.mu.Lock()
				defer api.mu.Unlock()
				for _, response := range api.workflows {
					response["url"] = "https://api.buildkite.com/renamed-suite/workflow"
					response["web_url"] = "https://buildkite.com/renamed-suite/workflow/events"
				}
			}, RefreshState: true, Check: resource.TestCheckResourceAttr(workflowTestAddress, "web_url", "https://buildkite.com/renamed-suite/workflow/events")},
		},
	})
}

func TestTestSuiteWorkflowImportErrors(t *testing.T) {
	const id = "123e4567-e89b-42d3-a456-000000000001"
	tests := []struct {
		name, importID, body, message string
		status                        int
	}{
		{name: "missing separator", importID: id, message: "Expected suite-slug/workflow-uuid"},
		{name: "missing slug", importID: "/" + id, message: "Expected suite-slug/workflow-uuid"},
		{name: "missing uuid", importID: "application/", message: "Expected suite-slug/workflow-uuid"},
		{name: "invalid uuid", importID: "application/not-a-uuid", message: "Expected suite-slug/workflow-uuid"},
		{name: "extra component", importID: "application/" + id + "/extra", message: "Expected suite-slug/workflow-uuid"},
		{name: "url", importID: "https://buildkite.com/application/" + id, message: "Expected suite-slug/workflow-uuid"},
		{name: "path traversal", importID: "../" + id, message: "Expected suite-slug/workflow-uuid"},
		{name: "query", importID: "application?other/" + id, message: "Expected suite-slug/workflow-uuid"},
		{name: "suite not found", status: http.StatusNotFound, message: "Could not read test suite"},
		{name: "suite forbidden", status: http.StatusForbidden, message: "Could not read test suite"},
		{name: "suite server error", status: http.StatusInternalServerError, message: "Could not read test suite"},
		{name: "missing suite uuid", status: http.StatusOK, body: `{"graphql_id":"suite-graphql-id"}`, message: "did not return a valid suite UUID"},
		{name: "invalid suite uuid", status: http.StatusOK, body: `{"id":"suite-graphql-id"}`, message: "did not return a valid suite UUID"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if tc.status == 0 {
					t.Error("invalid import ID must not make API requests")
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if req.Method != http.MethodGet || req.URL.Path != "/v2/analytics/organizations/test/suites/application" {
					t.Errorf("unexpected request: %s %s", req.Method, req.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			ctx := context.Background()
			r := &testSuiteWorkflowResource{client: NewClient(&clientConfig{org: "test", apiToken: "test", restURL: server.URL})}
			resp := fwresource.ImportStateResponse{State: tfsdk.State{Schema: resourceSchema(ctx, t, r)}}
			importID := tc.importID
			if importID == "" {
				importID = "application/" + id
			}
			r.ImportState(ctx, fwresource.ImportStateRequest{ID: importID}, &resp)
			if !resp.Diagnostics.HasError() || !diagnosticsContain(resp.Diagnostics, tc.message) || !resp.State.Raw.IsNull() {
				t.Fatalf("failed import must report %q without writing state: %v, state: %s", tc.message, resp.Diagnostics, resp.State.Raw)
			}
		})
	}
}

func TestTestSuiteWorkflowTypes(t *testing.T) {
	tests := map[string]struct {
		monitor, collections, body string
		checks                     resource.TestCheckFunc
	}{
		"probabilistic_flakiness": {
			monitor: `type = "probabilistic_flakiness"
alarm_threshold = 0.025
recover_threshold = 0.0031`,
			collections: `alarm_actions = [
{type="send_slack_notification", slack_workspace_id="123e4567-e89b-42d3-a456-426614174001", channel="#test", message_template="{{ test.name }} flaky"},
{type="create_linear_issue", linear_team_id="123e4567-e89b-42d3-a456-426614174002", title_template="Flaky {{ test.name }}", description_template="Check test"},
{type="change_state", state="muted"}
]`,
			body: `{"monitor":{"type":"probabilistic_flakiness","alarm_threshold":0.025,"recover_threshold":0.0031},"filters":[],"recover_actions":[],"alarm_actions":[
{"type":"send_slack_notification","slack_workspace_id":"123e4567-e89b-42d3-a456-426614174001","channel":"#test","message_template":"{{ test.name }} flaky"},
{"type":"create_linear_issue","linear_team_id":"123e4567-e89b-42d3-a456-426614174002","title_template":"Flaky {{ test.name }}","description_template":"Check test"},
{"type":"change_state","state":"muted"}]}`,
			checks: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(workflowTestAddress, "monitor.recover_threshold", "0.0031"),
				resource.TestCheckTypeSetElemNestedAttrs(workflowTestAddress, "alarm_actions.*", map[string]string{"type": "change_state", "state": "muted"}),
				resource.TestCheckTypeSetElemNestedAttrs(workflowTestAddress, "alarm_actions.*", map[string]string{"type": "send_slack_notification", "channel": "#test", "message_template": "{{ test.name }} flaky"}),
				resource.TestCheckTypeSetElemNestedAttrs(workflowTestAddress, "alarm_actions.*", map[string]string{"type": "create_linear_issue", "title_template": "Flaky {{ test.name }}", "description_template": "Check test"}),
			),
		},
		"duration_threshold": {
			monitor: `type = "duration_threshold"
aggregation = "p50"
evaluation_window = 17
alarm_threshold_ms = 12345
recover_threshold_ms = 0`,
			body: `{"monitor":{"type":"duration_threshold","aggregation":"p50","evaluation_window":17,"alarm_threshold_ms":12345,"recover_threshold_ms":0},"filters":[],"alarm_actions":[],"recover_actions":[]}`,
			checks: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(workflowTestAddress, "monitor.alarm_threshold_ms", "12345"),
				resource.TestCheckResourceAttr(workflowTestAddress, "monitor.recover_threshold_ms", "0"),
				resource.TestCheckResourceAttr(workflowTestAddress, "monitor.evaluation_window", "17"),
			),
		},
		"passed_on_retry": {
			monitor: `type = "passed_on_retry"`,
			body:    `{"monitor":{"type":"passed_on_retry"},"filters":[],"alarm_actions":[],"recover_actions":[]}`,
		},
		"new_test": {
			monitor: `type = "new_test"`,
			body:    `{"monitor":{"type":"new_test"},"filters":[],"alarm_actions":[],"recover_actions":[]}`,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			api := newWorkflowTestAPI(t, tc.body)
			checks := []resource.TestCheckFunc{
				resource.TestCheckResourceAttr(workflowTestAddress, "monitor.type", name),
				resource.TestCheckResourceAttr(workflowTestAddress, "filters.#", "0"),
				resource.TestCheckResourceAttr(workflowTestAddress, "recover_actions.#", "0"),
			}
			if tc.checks != nil {
				checks = append(checks, tc.checks)
			}
			config := workflowTestConfig(api.server.URL, workflowTestSuiteUUID, tc.monitor, tc.collections)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				Steps: []resource.TestStep{
					{Config: config, Check: resource.ComposeAggregateTestCheckFunc(checks...)},
					{Config: config, PlanOnly: true},
				},
			})
		})
	}
}

func TestTestSuiteWorkflowReplacement(t *testing.T) {
	api := newWorkflowTestAPI(t, workflowTestBody)
	config := workflowTestConfig(api.server.URL, workflowTestSuiteUUID, workflowTestMonitor, workflowTestCollections)
	steps := []resource.TestStep{{Config: config}}
	for _, replacement := range []string{
		strings.Replace(config, workflowTestSuiteUUID, "123e4567-e89b-42d3-a456-426614174099", 1),
		strings.Replace(config, workflowTestMonitor, `type = "passed_on_retry"`, 1),
	} {
		steps = append(steps, resource.TestStep{
			Config: replacement, PlanOnly: true, ExpectNonEmptyPlan: true,
			ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectResourceAction(workflowTestAddress, plancheck.ResourceActionDestroyBeforeCreate)}},
		})
	}
	steps = append(steps, resource.TestStep{
		PreConfig: func() {
			api.mu.Lock()
			defer api.mu.Unlock()
			api.want["monitor"] = map[string]any{"type": "passed_on_retry"}
		},
		Config: strings.Replace(config, workflowTestMonitor, `type = "passed_on_retry"`, 1),
		Check: resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr(workflowTestAddress, "id", "123e4567-e89b-42d3-a456-000000000002"),
			resource.TestCheckResourceAttr(workflowTestAddress, "monitor.type", "passed_on_retry"),
		),
	})
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: protoV6ProviderFactories(), Steps: steps})
}

func TestTestSuiteWorkflowUpdate(t *testing.T) {
	api := newWorkflowTestAPI(t, workflowTestBody)
	config := workflowTestConfig(api.server.URL, workflowTestSuiteUUID, workflowTestMonitor, workflowTestCollections)
	updated := workflowTestConfig(api.server.URL, workflowTestSuiteUUID, `type = "transition_count"
window_size = 91
alarm_threshold = 0.25
recover_threshold = 0.012`, `
filters = [{key="scm.branch",operator="starts_with",value="release/"}]
alarm_actions = [{type="send_webhook_notification",endpoint="https://example.com/changed"}]
recover_actions = [{type="change_state",state="enabled"}]
`)
	cleared := workflowTestConfig(api.server.URL, workflowTestSuiteUUID, `type = "transition_count"
window_size = 91
alarm_threshold = 0.25
recover_threshold = 0.012`, `filters = [{key="scm.branch",operator="starts_with",value="release/"}]`)
	idCheck := resource.TestCheckResourceAttr(workflowTestAddress, "id", "123e4567-e89b-42d3-a456-000000000001")
	updateCheck := resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
		plancheck.ExpectResourceAction(workflowTestAddress, plancheck.ResourceActionUpdate),
	}}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{Config: config},
			{PreConfig: func() {
				api.mu.Lock()
				defer api.mu.Unlock()
				if err := json.Unmarshal([]byte(`{
"monitor":{"type":"transition_count","window_size":91,"alarm_threshold":0.25,"recover_threshold":0.012},
"filters":[{"key":"scm.branch","operator":"starts_with","value":"release/"}],
"alarm_actions":[{"type":"send_webhook_notification","endpoint":"https://example.com/changed"}],
"recover_actions":[{"type":"change_state","state":"enabled"}]}`), &api.want); err != nil {
					t.Fatal(err)
				}
			}, Config: updated, ConfigPlanChecks: updateCheck, Check: resource.ComposeAggregateTestCheckFunc(
				idCheck,
				resource.TestCheckResourceAttr(workflowTestAddress, "monitor.window_size", "91"),
				resource.TestCheckResourceAttr(workflowTestAddress, "monitor.alarm_threshold", "0.25"),
				resource.TestCheckResourceAttr(workflowTestAddress, "monitor.recover_threshold", "0.012"),
				resource.TestCheckResourceAttr(workflowTestAddress, "filters.#", "1"),
				resource.TestCheckTypeSetElemNestedAttrs(workflowTestAddress, "filters.*", map[string]string{"operator": "starts_with", "value": "release/"}),
				resource.TestCheckResourceAttr(workflowTestAddress, "alarm_actions.#", "1"),
				resource.TestCheckTypeSetElemNestedAttrs(workflowTestAddress, "alarm_actions.*", map[string]string{"endpoint": "https://example.com/changed"}),
				resource.TestCheckTypeSetElemNestedAttrs(workflowTestAddress, "recover_actions.*", map[string]string{"state": "enabled"}),
			)},
			{PreConfig: func() {
				api.mu.Lock()
				defer api.mu.Unlock()
				api.want["alarm_actions"], api.want["recover_actions"] = []any{}, []any{}
			}, Config: cleared, ConfigPlanChecks: updateCheck, Check: resource.ComposeAggregateTestCheckFunc(
				idCheck,
				resource.TestCheckResourceAttr(workflowTestAddress, "alarm_actions.#", "0"),
				resource.TestCheckResourceAttr(workflowTestAddress, "recover_actions.#", "0"),
			)},
			{Config: cleared, PlanOnly: true},
		},
	})
}

func TestTestSuiteWorkflowValidation(t *testing.T) {
	tests := []struct{ monitor, collections, message string }{
		{`type="transition_count"`, "", "requires window_size"},
		{workflowTestMonitor, "", "requires a nonblank scm.branch"},
		{`type="passed_on_retry"\nwindow_size=10`, "", "does not support window_size"},
		{`type="new_test"`, `recover_actions=[{type="add_label",label="flaky"}]`, "does not support recovery"},
		{strings.Replace(workflowTestMonitor, "recover_threshold = 0", "recover_threshold = 0.127", 1), workflowTestCollections, "must be less than alarm_threshold"},
		{strings.Replace(workflowTestMonitor, "0.127", "0.1271", 1), workflowTestCollections, "at most three decimal places"},
		{`type="probabilistic_flakiness"\nalarm_threshold=0.024\nrecover_threshold=0`, "", "at least 0.025"},
		{`type="duration_threshold"\naggregation="p50"\nevaluation_window=1\nalarm_threshold_ms=10\nrecover_threshold_ms=11`, "", "must be less than alarm_threshold_ms"},
		{workflowTestMonitor, `filters=[{key="scm.branch",operator="is",value=" main "}]`, "cannot have leading or trailing whitespace"},
		{`type="passed_on_retry"`, `alarm_actions=[{type="send_slack_notification",channel="#test"}]`, "requires slack_workspace_id"},
		{`type="passed_on_retry"`, `alarm_actions=[{type="add_label",label="flaky",endpoint="https://example.com"}]`, "does not support endpoint"},
		{`type="passed_on_retry"`, `filters=[{key="mut.state",operator="is",value="muted"}]`, "cannot be mutable tags"},
		{`type="passed_on_retry"`, `filters=[null]`, "cannot contain null"},
		{`type="passed_on_retry"`, `alarm_actions=[null]`, "cannot contain null"},
		{`type="passed_on_retry"`, `alarm_actions=[{type="send_slack_notification",slack_workspace_id="123E4567-E89B-42D3-A456-426614174001",channel="#test",message_template="Flaky test"}]`, `must be a lowercase\s+UUID`},
		{`type="passed_on_retry"`, `recover_actions=[{type="send_slack_notification",slack_workspace_id="123e4567-E89b-42d3-a456-426614174001",channel="#test",message_template="Recovered test"}]`, `must be a lowercase\s+UUID`},
		{`type="passed_on_retry"`, `alarm_actions=[{type="create_linear_issue",linear_team_id="123E4567-E89B-42D3-A456-426614174002",title_template="Flaky test",description_template="Check test"}]`, `must be a lowercase\s+UUID`},
		{`type="passed_on_retry"`, `recover_actions=[{type="create_linear_issue",linear_team_id="123e4567-E89b-42d3-a456-426614174002",title_template="Recovered test",description_template="Check test"}]`, `must be a lowercase\s+UUID`},
	}
	for _, tc := range tests {
		t.Run(tc.message, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				Steps: []resource.TestStep{{
					Config:   workflowTestConfig("http://127.0.0.1:1", workflowTestSuiteUUID, strings.ReplaceAll(tc.monitor, `\n`, "\n"), tc.collections),
					PlanOnly: true, ExpectError: regexp.MustCompile(tc.message),
				}},
			})
		})
	}
}

// Direct CRUD checks keep failures observable without the Terraform harness trying to refresh or
// clean up with the deliberately denied/masked credentials.
func TestTestSuiteWorkflowFailureState(t *testing.T) {
	ctx := context.Background()
	api := newWorkflowTestAPI(t, workflowTestBody)
	r := &testSuiteWorkflowResource{client: NewClient(&clientConfig{org: "test", apiToken: "test", restURL: api.server.URL})}
	schema := resourceSchema(ctx, t, r)
	var response testSuiteWorkflowResponse
	if err := json.Unmarshal([]byte(workflowTestBody), &response); err != nil {
		t.Fatal(err)
	}
	state := testSuiteWorkflowModel{ID: types.StringUnknown(), TestSuiteUUID: types.StringValue(workflowTestSuiteUUID)}
	if diags := state.applyWorkflowResponse(response); diags.HasError() {
		t.Fatal(diags)
	}
	plan := tfsdk.Plan{Schema: schema}
	if diags := plan.Set(ctx, state); diags.HasError() {
		t.Fatal(diags)
	}
	create := fwresource.CreateResponse{State: tfsdk.State{Schema: schema}}
	r.Create(ctx, fwresource.CreateRequest{Plan: plan}, &create)
	if create.Diagnostics.HasError() {
		t.Fatal(create.Diagnostics)
	}
	updatePlan := tfsdk.Plan{Schema: schema, Raw: create.State.Raw}
	// Computed URLs are unknown in an update plan; failures must retain their prior values.
	if diags := updatePlan.SetAttribute(ctx, path.Root("web_url"), types.StringUnknown()); diags.HasError() {
		t.Fatal(diags)
	}

	for _, status := range []int{http.StatusForbidden, http.StatusInternalServerError} {
		api.mu.Lock()
		api.status = status
		api.mu.Unlock()
		read := fwresource.ReadResponse{State: create.State}
		r.Read(ctx, fwresource.ReadRequest{State: create.State}, &read)
		if !read.Diagnostics.HasError() || !read.State.Raw.Equal(create.State.Raw) {
			t.Fatalf("HTTP %d must report an error and retain state: %v", status, read.Diagnostics)
		}
		updated := fwresource.UpdateResponse{State: create.State}
		r.Update(ctx, fwresource.UpdateRequest{Plan: updatePlan, State: create.State}, &updated)
		if !updated.Diagnostics.HasError() || !updated.State.Raw.Equal(create.State.Raw) {
			t.Fatalf("HTTP %d update must report an error and retain state: %v", status, updated.Diagnostics)
		}
		deleted := fwresource.DeleteResponse{State: create.State}
		r.Delete(ctx, fwresource.DeleteRequest{State: create.State}, &deleted)
		if !deleted.Diagnostics.HasError() {
			t.Fatalf("HTTP %d deletion must fail", status)
		}
	}
	api.mu.Lock()
	api.status, api.maskWebhook = 0, true
	api.mu.Unlock()
	read := fwresource.ReadResponse{State: create.State}
	r.Read(ctx, fwresource.ReadRequest{State: create.State}, &read)
	if !diagnosticsContain(read.Diagnostics, "masked webhook") || !read.State.Raw.Equal(create.State.Raw) {
		t.Fatalf("masked endpoint must fail without changing state: %v", read.Diagnostics)
	}
	updated := fwresource.UpdateResponse{State: create.State}
	r.Update(ctx, fwresource.UpdateRequest{Plan: updatePlan, State: create.State}, &updated)
	if !diagnosticsContain(updated.Diagnostics, "masked webhook") || !updated.State.Raw.Equal(create.State.Raw) {
		t.Fatalf("masked update must fail without losing prior state: %v", updated.Diagnostics)
	}
	maskedCreate := fwresource.CreateResponse{State: tfsdk.State{Schema: schema}}
	r.Create(ctx, fwresource.CreateRequest{Plan: plan}, &maskedCreate)
	var tracked testSuiteWorkflowModel
	if diags := maskedCreate.State.Get(ctx, &tracked); diags.HasError() {
		t.Fatal(diags)
	}
	if !diagnosticsContain(maskedCreate.Diagnostics, "masked webhook") || tracked.ID.ValueString() != "123e4567-e89b-42d3-a456-000000000002" || tracked.TestSuiteUUID.ValueString() != workflowTestSuiteUUID {
		t.Fatalf("create conversion failure must retain the new workflow's identity: %v", maskedCreate.Diagnostics)
	}
	api.mu.Lock()
	api.maskWebhook = false
	api.status = http.StatusUnprocessableEntity
	api.mu.Unlock()
	failed := fwresource.CreateResponse{State: tfsdk.State{Schema: schema}}
	r.Create(ctx, fwresource.CreateRequest{Plan: plan}, &failed)
	if !diagnosticsContain(failed.Diagnostics, "/alarm_actions/0/label") {
		t.Fatalf("API validation pointer missing: %v", failed.Diagnostics)
	}
	updated = fwresource.UpdateResponse{State: create.State}
	r.Update(ctx, fwresource.UpdateRequest{Plan: updatePlan, State: create.State}, &updated)
	if !diagnosticsContain(updated.Diagnostics, "/alarm_actions/0/label") || !updated.State.Raw.Equal(create.State.Raw) {
		t.Fatalf("invalid update must report the API validation pointer and retain state: %v", updated.Diagnostics)
	}
	api.mu.Lock()
	api.status = 0
	for _, response := range api.workflows {
		response["alarm_actions"] = []any{map[string]any{"type": "trigger_pipeline"}}
	}
	api.mu.Unlock()
	read = fwresource.ReadResponse{State: create.State}
	r.Read(ctx, fwresource.ReadRequest{State: create.State}, &read)
	if !diagnosticsContain(read.Diagnostics, "unsupported workflow type") || !read.State.Raw.Equal(create.State.Raw) {
		t.Fatalf("unsupported actions must fail without silently dropping them: %v", read.Diagnostics)
	}
	api.mu.Lock()
	api.workflows = map[string]map[string]any{}
	api.mu.Unlock()
	read = fwresource.ReadResponse{State: create.State}
	r.Read(ctx, fwresource.ReadRequest{State: create.State}, &read)
	if read.Diagnostics.HasError() || !read.State.Raw.IsNull() {
		t.Fatalf("missing workflow must be removed: %v", read.Diagnostics)
	}
	deleted := fwresource.DeleteResponse{State: create.State}
	r.Delete(ctx, fwresource.DeleteRequest{State: create.State}, &deleted)
	if deleted.Diagnostics.HasError() {
		t.Fatalf("deleting a missing workflow must succeed: %v", deleted.Diagnostics)
	}
}
