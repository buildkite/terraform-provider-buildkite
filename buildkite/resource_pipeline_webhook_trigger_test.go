package buildkite

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

const triggerTestAddress = "buildkite_pipeline_webhook_trigger.test"

// The server returns the credential only on POST and resolves unset build fields
// to defaults, matching the public API rather than echoing the Terraform configuration.
type webhookTriggerRecord struct {
	id          string
	pipeline    string
	label       string
	enabled     bool
	branch      any
	commit      any
	message     any
	environment map[string]any
	filter      any
	exists      bool
}

type webhookTriggerTestAPI struct {
	*webhookTriggerRecord
	t                  *testing.T
	server             *httptest.Server
	mu                 sync.Mutex
	records            map[string]*webhookTriggerRecord
	pipelines          map[string]string
	creates            int
	deletes            int
	lastPayload        map[string]any
	readStatus         int
	writeStatus        int
	omitURL            bool
	triggerType        string
	deleteNotFound     bool
	parentStatus       int
	parentOrganization string
	events             []string
}

func newWebhookTriggerTestAPI(t *testing.T) *webhookTriggerTestAPI {
	api := &webhookTriggerTestAPI{t: t, triggerType: "webhook", webhookTriggerRecord: &webhookTriggerRecord{}, records: map[string]*webhookTriggerRecord{}, pipelines: map[string]string{"receiver-id": "receiver", "other-id": "other"}, parentOrganization: "example"}
	api.server = httptest.NewServer(http.HandlerFunc(api.handle))
	t.Cleanup(api.server.Close)
	return api
}

func (api *webhookTriggerTestAPI) handle(w http.ResponseWriter, r *http.Request) {
	api.mu.Lock()
	defer api.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/graphql" {
		api.graphql(w, r)
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 6 || parts[0] != "v2" || parts[1] != "organizations" || parts[2] != "example" || parts[3] != "pipelines" || parts[5] != "triggers" {
		api.t.Errorf("unexpected API path: %s", r.URL.Path)
		http.Error(w, "unexpected path", http.StatusBadRequest)
		return
	}
	if r.Header.Get("Authorization") != "Bearer test-token" {
		api.t.Error("missing API authentication")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if r.Method == http.MethodGet && api.readStatus != 0 {
		w.WriteHeader(api.readStatus)
		_, _ = w.Write([]byte(`{"message":"read failed"}`))
		return
	}
	if r.Method != http.MethodGet && api.writeStatus != 0 {
		w.WriteHeader(api.writeStatus)
		_, _ = w.Write([]byte(`{"message":"write failed"}`))
		return
	}
	if r.Method != http.MethodPost {
		if len(parts) != 7 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		record := api.records[parts[6]]
		if record == nil || !record.exists || parts[4] != record.pipeline {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		previous := api.webhookTriggerRecord
		api.webhookTriggerRecord = record
		defer func() { api.webhookTriggerRecord = previous }()
	}

	switch r.Method {
	case http.MethodPost, http.MethodPatch:
		if r.Header.Get("Content-Type") != "application/json" {
			api.t.Error("missing JSON content type")
			w.WriteHeader(http.StatusUnsupportedMediaType)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			api.t.Errorf("decode payload: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		for key := range body {
			if key != "type" && key != "label" && key != "enabled" && key != "build" && key != "filter" {
				api.t.Errorf("unexpected field %s", key)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}
		if r.Method == http.MethodPost {
			if len(parts) != 6 || body["type"] != "webhook" {
				api.t.Error("invalid create request")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			api.webhookTriggerRecord = &webhookTriggerRecord{}
			api.creates++
			api.events = append(api.events, "create")
			api.id = fmt.Sprintf("123e4567-e89b-42d3-a456-%012d", api.creates)
			api.pipeline, api.exists = parts[4], true
			api.records[api.id] = api.webhookTriggerRecord
			api.branch, api.commit, api.message = "main", "HEAD", "Webhook build"
			api.environment = map[string]any{}
			api.filter = nil
		} else if _, ok := body["type"]; ok {
			api.t.Error("PATCH must not send immutable type")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		api.lastPayload = body
		if label, ok := body["label"].(string); ok {
			api.label = label
		}
		if enabled, ok := body["enabled"].(bool); ok {
			api.enabled = enabled
		}
		if filter, ok := body["filter"]; ok {
			api.filter = filter
		}
		if build, ok := body["build"].(map[string]any); ok {
			for key, value := range build {
				switch key {
				case "branch":
					if value == nil {
						value = "main"
					}
					api.branch = value
				case "commit":
					if value == nil {
						value = "HEAD"
					}
					api.commit = value
				case "message":
					if value == nil {
						value = "Webhook build"
					}
					api.message = value
				case "environment":
					api.environment = value.(map[string]any)
				default:
					api.t.Errorf("unexpected build field %s", key)
				}
			}
		}
	case http.MethodDelete:
		api.exists = false
		api.deletes++
		api.events = append(api.events, "delete:"+api.id)
		if api.deleteNotFound {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	case http.MethodGet:
	default:
		api.t.Errorf("unexpected method: %s", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	names := []string{}
	for name := range api.environment {
		names = append(names, name)
	}
	body := map[string]any{
		"id": api.id, "type": api.triggerType, "label": api.label, "enabled": api.enabled,
		"build":  map[string]any{"branch": api.branch, "commit": api.commit, "message": api.message, "environment_variables": names},
		"filter": api.filter, "verification": nil,
		"endpoint_url_hint": "https://webhook.buildkite.com/deliver/bktr_XXXX",
	}
	if r.Method == http.MethodPost {
		if !api.omitURL {
			body["endpoint_url"] = fmt.Sprintf("https://example.test/trigger-%d", api.creates)
		}
		w.WriteHeader(http.StatusCreated)
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		api.t.Errorf("encode response: %v", err)
	}
}

func (api *webhookTriggerTestAPI) config(extra string) string {
	return fmt.Sprintf(`
 provider "buildkite" {
  organization = "example"
  api_token = "test-token"
  rest_url = %[1]q
 graphql_url = "%[1]s/graphql"
  max_retries = 0
 }
 resource "buildkite_pipeline_webhook_trigger" "test" {
  pipeline_id = "receiver-id"
  label = "Example trigger"
  %[2]s
 }
 `, api.server.URL, extra)
}

func (api *webhookTriggerTestAPI) change(fn func()) func() {
	return func() { api.mu.Lock(); defer api.mu.Unlock(); fn() }
}

func (api *webhookTriggerTestAPI) checkPayload(want map[string]any) resource.TestCheckFunc {
	return func(*terraform.State) error {
		api.mu.Lock()
		defer api.mu.Unlock()
		if diff := cmp.Diff(want, api.lastPayload); diff != "" {
			return fmt.Errorf("payload mismatch (-want +got):\n%s", diff)
		}
		return nil
	}
}

func (api *webhookTriggerTestAPI) checkDestroy(*terraform.State) error {
	api.mu.Lock()
	defer api.mu.Unlock()
	for _, record := range api.records {
		if record.exists {
			return fmt.Errorf("trigger still exists")
		}
	}
	return nil
}

func TestPipelineWebhookTriggerLifecycle(t *testing.T) {
	api := newWebhookTriggerTestAPI(t)
	config := api.config(`
 enabled = false
 filter = "webhook.payload.action == \"deploy\""
 build = {
  branch = "release"
  commit = "abc123"
  message = "Deploy release"
  environment = { TARGET = "staging" }
 }
 `)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(), CheckDestroy: api.checkDestroy,
		Steps: []resource.TestStep{
			{Config: config, ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectSensitiveValue(triggerTestAddress, tfjsonpath.New("endpoint_url")), plancheck.ExpectSensitiveValue(triggerTestAddress, tfjsonpath.New("build").AtMapKey("environment"))}}, Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(triggerTestAddress, "endpoint_url", "https://example.test/trigger-1"),
				resource.TestCheckResourceAttr(triggerTestAddress, "enabled", "false"),
				resource.TestCheckResourceAttr(triggerTestAddress, "build.environment.TARGET", "staging"),
				api.checkPayload(map[string]any{"type": "webhook", "label": "Example trigger", "enabled": false,
					"filter": map[string]any{"expression": "webhook.payload.action == \"deploy\""},
					"build":  map[string]any{"branch": "release", "commit": "abc123", "message": "Deploy release", "environment": map[string]any{"TARGET": "staging"}}}),
			)},
			{Config: strings.Replace(config, "enabled = false", "enabled = true", 1), Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(triggerTestAddress, "endpoint_url", "https://example.test/trigger-1"),
				resource.TestCheckResourceAttr(triggerTestAddress, "enabled", "true"),
			)},
			// Clear the filter and the whole build block. The API needs explicit nulls and an empty environment.
			{Config: api.config(""), Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(triggerTestAddress, "endpoint_url", "https://example.test/trigger-1"),
				resource.TestCheckResourceAttr(triggerTestAddress, "effective_build.branch", "main"),
				resource.TestCheckResourceAttr(triggerTestAddress, "effective_build.commit", "HEAD"),
				resource.TestCheckResourceAttr(triggerTestAddress, "effective_build.message", "Webhook build"),
				resource.TestCheckNoResourceAttr(triggerTestAddress, "filter"),
				api.checkPayload(map[string]any{"label": "Example trigger", "enabled": true, "filter": nil,
					"build": map[string]any{"branch": nil, "commit": nil, "message": nil, "environment": map[string]any{}}}),
			)},
			{Config: api.config(""), PlanOnly: true, ExpectNonEmptyPlan: false},
			{ResourceName: triggerTestAddress, ImportState: true, ImportStateId: "receiver/123e4567-e89b-42d3-a456-000000000001", ImportStateVerify: true, ImportStateVerifyIgnore: []string{"endpoint_url", "build"}, ImportStateCheck: func(states []*terraform.InstanceState) error {
				if len(states) != 1 || states[0].Attributes["endpoint_url"] != "" {
					return fmt.Errorf("import must not invent or recover a credential")
				}
				return nil
			}},
		},
	})
}

func TestPipelineWebhookTriggerDriftAndDeletion(t *testing.T) {
	api := newWebhookTriggerTestAPI(t)
	config := api.config(`build = { branch = "release" }`)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(), CheckDestroy: api.checkDestroy,
		Steps: []resource.TestStep{
			{Config: config},
			{PreConfig: api.change(func() { api.branch = "other"; api.enabled = false }), Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(triggerTestAddress, plancheck.ResourceActionUpdate)}},
				Check:            resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr(triggerTestAddress, "enabled", "true"), resource.TestCheckResourceAttr(triggerTestAddress, "build.branch", "release"), resource.TestCheckResourceAttr(triggerTestAddress, "endpoint_url", "https://example.test/trigger-1"))},
			{PreConfig: api.change(func() { api.exists = false }), Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(triggerTestAddress, plancheck.ResourceActionCreate)}},
				Check:            resource.TestCheckResourceAttr(triggerTestAddress, "endpoint_url", "https://example.test/trigger-2")},
			{Config: strings.Replace(config, `pipeline_id = "receiver-id"`, `pipeline_id = "other-id"`, 1),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(triggerTestAddress, plancheck.ResourceActionDestroyBeforeCreate)}},
				Check:            resource.TestCheckResourceAttr(triggerTestAddress, "endpoint_url", "https://example.test/trigger-3")},
			// The object can disappear between refresh and DELETE.
			{PreConfig: api.change(func() { api.deleteNotFound = true }), Config: strings.Replace(config, `pipeline_id = "receiver-id"`, `pipeline_id = "other-id"`, 1), Destroy: true},
		},
	})
}

func TestPipelineWebhookTriggerReadErrors(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			api := newWebhookTriggerTestAPI(t)
			resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: protoV6ProviderFactories(), CheckDestroy: api.checkDestroy, Steps: []resource.TestStep{
				{Config: api.config("")},
				{PreConfig: api.change(func() { api.readStatus = status }), Config: api.config(""), PlanOnly: true, ExpectError: regexp.MustCompile("Unable to read pipeline webhook trigger")},
				{PreConfig: api.change(func() { api.readStatus = 0 }), Config: api.config(""), Check: func(*terraform.State) error {
					api.mu.Lock()
					defer api.mu.Unlock()
					if api.creates != 1 {
						return fmt.Errorf("read error caused recreation")
					}
					return nil
				}},
			}})
		})
	}
}

func TestPipelineWebhookTriggerWriteErrors(t *testing.T) {
	api := newWebhookTriggerTestAPI(t)
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: protoV6ProviderFactories(), CheckDestroy: api.checkDestroy, Steps: []resource.TestStep{
		{PreConfig: api.change(func() { api.writeStatus = http.StatusUnprocessableEntity }), Config: api.config(""), ExpectError: regexp.MustCompile("Unable to create pipeline webhook trigger")},
		{PreConfig: api.change(func() { api.writeStatus = 0 }), Config: api.config("")},
		{PreConfig: api.change(func() { api.writeStatus = http.StatusForbidden }), Config: api.config("enabled = false"), ExpectError: regexp.MustCompile("Unable to update pipeline webhook trigger")},
		{Config: api.config(""), Destroy: true, ExpectError: regexp.MustCompile("Unable to delete pipeline webhook trigger")},
		{PreConfig: api.change(func() { api.writeStatus = 0 }), Config: api.config(""), Check: resource.TestCheckResourceAttr(triggerTestAddress, "endpoint_url", "https://example.test/trigger-1")},
	}})
}

func TestPipelineWebhookTriggerMissingCreationURL(t *testing.T) {
	api := newWebhookTriggerTestAPI(t)
	api.omitURL = true
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: protoV6ProviderFactories(), CheckDestroy: api.checkDestroy, Steps: []resource.TestStep{
		{Config: api.config(""), ExpectError: regexp.MustCompile("Missing pipeline trigger endpoint URL")},
	}})
}

func TestPipelineWebhookTriggerUnsupportedImport(t *testing.T) {
	api := newWebhookTriggerTestAPI(t)
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: protoV6ProviderFactories(), Steps: []resource.TestStep{
		{Config: api.config("")},
		{PreConfig: api.change(func() { api.triggerType = "github" }), Config: api.config(""), ResourceName: triggerTestAddress, ImportState: true, ImportStateId: "receiver/123e4567-e89b-42d3-a456-000000000001", ExpectError: regexp.MustCompile("Unsupported pipeline trigger type")},
		{PreConfig: api.change(func() { api.triggerType = "webhook" }), Config: api.config("")},
	}})
}

func TestPipelineWebhookTriggerInvalidImportID(t *testing.T) {
	for _, id := range []string{"", "receiver", "receiver/not-a-uuid", "/123e4567-e89b-42d3-a456-426614174000", "org/receiver/123e4567-e89b-42d3-a456-426614174000"} {
		t.Run(id, func(t *testing.T) {
			r := pipelineWebhookTriggerResource{}
			var response frameworkresource.ImportStateResponse
			r.ImportState(t.Context(), frameworkresource.ImportStateRequest{ID: id}, &response)
			if !response.Diagnostics.HasError() {
				t.Fatal("invalid import ID was accepted")
			}
		})
	}
}

func TestAccBuildkitePipelineWebhookTrigger(t *testing.T) {
	name := "terraform-trigger-" + acctest.RandString(12)
	config := func(enabled bool, branch string) string {
		return fmt.Sprintf(`
   resource "buildkite_pipeline" "receiver" {
    name = %q
    repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
   }
   resource "buildkite_pipeline_webhook_trigger" "test" {
    pipeline_id = buildkite_pipeline.receiver.id
    label = "Terraform acceptance test"
    enabled = %t
    build = { branch = %q }
   }
  `, name, enabled, branch)
	}
	var endpoint string
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckPipelineDestroyFunc,
		Steps: []resource.TestStep{
			{Config: config(false, "main"), Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttrSet(triggerTestAddress, "id"),
				resource.TestCheckResourceAttrSet(triggerTestAddress, "endpoint_url"),
				resource.TestCheckResourceAttr(triggerTestAddress, "enabled", "false"),
				func(state *terraform.State) error {
					endpoint = state.RootModule().Resources[triggerTestAddress].Primary.Attributes["endpoint_url"]
					return nil
				},
			)},
			{Config: config(true, "release"), Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(triggerTestAddress, "enabled", "true"),
				resource.TestCheckResourceAttr(triggerTestAddress, "build.branch", "release"),
				func(state *terraform.State) error {
					if state.RootModule().Resources[triggerTestAddress].Primary.Attributes["endpoint_url"] != endpoint {
						return fmt.Errorf("endpoint URL changed during update")
					}
					return nil
				},
			)},
			{ResourceName: triggerTestAddress, ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"endpoint_url", "build.commit", "build.message"}, ImportStateIdFunc: func(state *terraform.State) (string, error) {
				attrs := state.RootModule().Resources[triggerTestAddress].Primary.Attributes
				return state.RootModule().Resources["buildkite_pipeline.receiver"].Primary.Attributes["slug"] + "/" + attrs["id"], nil
			}},
		},
	})
}

func TestPipelineWebhookTriggerUpdatePreservesUnmanagedBuildFields(t *testing.T) {
	api := newWebhookTriggerTestAPI(t)
	config := api.config(`build = { environment = { TARGET = "staging" } }`)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             api.checkDestroy,
		Steps: []resource.TestStep{
			{Config: config},
			// An imported trigger can have build overrides that this configuration does not manage.
			{PreConfig: api.change(func() { api.branch = "release" }),
				Config: strings.Replace(config, `label = "Example trigger"`, `label = "Updated label"`, 1),
				Check: func(state *terraform.State) error {
					api.mu.Lock()
					defer api.mu.Unlock()
					if state.RootModule().Resources[triggerTestAddress].Primary.Attributes["effective_build.branch"] != "release" {
						return fmt.Errorf("effective build state did not reflect the external branch override")
					}
					if api.branch != "release" {
						return fmt.Errorf("unmanaged branch override was reset")
					}
					return nil
				}},
		},
	})
}

func (api *webhookTriggerTestAPI) graphql(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer test-token" {
		api.t.Error("missing GraphQL authentication")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if api.parentStatus != 0 {
		w.WriteHeader(api.parentStatus)
		return
	}
	var request struct {
		OperationName string            `json:"operationName"`
		Variables     map[string]string `json:"variables"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		api.t.Errorf("decode GraphQL request: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var data map[string]any
	switch request.OperationName {
	case "getPipelineTriggerParent":
		var node any
		if slug, exists := api.pipelines[request.Variables["id"]]; exists {
			node = map[string]any{"__typename": "Pipeline", "id": request.Variables["id"], "slug": slug, "organization": map[string]any{"slug": api.parentOrganization}}
		}
		data = map[string]any{"node": node}
	case "getPipelineId":
		var pipeline any
		for id, slug := range api.pipelines {
			if request.Variables["slug"] == "example/"+slug {
				pipeline = map[string]any{"id": id}
			}
		}
		data = map[string]any{"pipeline": pipeline}
	default:
		api.t.Errorf("unexpected GraphQL operation: %s", request.OperationName)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := json.NewEncoder(w).Encode(map[string]any{"data": data}); err != nil {
		api.t.Error(err)
	}
}

func TestPipelineWebhookTriggerParentRename(t *testing.T) {
	api := newWebhookTriggerTestAPI(t)
	config := api.config(`build = { branch = "release" }`)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(), CheckDestroy: api.checkDestroy,
		Steps: []resource.TestStep{
			{Config: config},
			{PreConfig: api.change(func() { api.pipelines["receiver-id"] = "renamed"; api.pipeline = "renamed" }), Config: config, PlanOnly: true, ExpectNonEmptyPlan: false},
			{Config: api.config(`enabled = false
    build = { branch = "release" }`),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(triggerTestAddress, plancheck.ResourceActionUpdate)}},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(triggerTestAddress, "pipeline_id", "receiver-id"),
					resource.TestCheckResourceAttr(triggerTestAddress, "endpoint_url", "https://example.test/trigger-1"),
				)},
			{Config: config, Destroy: true},
		},
	})
}

func TestPipelineWebhookTriggerImportBuildSettings(t *testing.T) {
	api := newWebhookTriggerTestAPI(t)
	config := api.config(`build = {
  branch = "release"
  commit = "abc123"
  message = "Deploy release"
  environment = { TARGET = "staging" }
 }`)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(), CheckDestroy: api.checkDestroy,
		Steps: []resource.TestStep{
			{Config: config},
			{ResourceName: triggerTestAddress, ImportState: true,
				ImportStateId:     "receiver/123e4567-e89b-42d3-a456-000000000001",
				ImportStateVerify: true, ImportStateVerifyIgnore: []string{"endpoint_url", "build.environment"},
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected one imported trigger")
					}
					for key, want := range map[string]string{"pipeline_id": "receiver-id", "build.branch": "release", "build.commit": "abc123", "build.message": "Deploy release"} {
						if got := states[0].Attributes[key]; got != want {
							return fmt.Errorf("imported %s = %q; want %q", key, got, want)
						}
					}
					if states[0].Attributes["build.environment.TARGET"] != "" {
						return fmt.Errorf("import recovered a hidden environment value")
					}
					return nil
				},
			},
		},
	})
}

func TestPipelineWebhookTriggerRemoveIndividualOverrides(t *testing.T) {
	api := newWebhookTriggerTestAPI(t)
	config := api.config(`build = {
  branch = "release"
  commit = "abc123"
  message = "Deploy release"
  environment = { TARGET = "staging" }
 }`)
	removed := api.config(`build = { environment = { TARGET = "staging" } }`)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(), CheckDestroy: api.checkDestroy,
		Steps: []resource.TestStep{
			{Config: config},
			{Config: removed, Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(triggerTestAddress, "effective_build.branch", "main"),
				resource.TestCheckResourceAttr(triggerTestAddress, "effective_build.commit", "HEAD"),
				resource.TestCheckResourceAttr(triggerTestAddress, "effective_build.message", "Webhook build"),
				api.checkPayload(map[string]any{"label": "Example trigger", "enabled": true,
					"build": map[string]any{"branch": nil, "commit": nil, "message": nil, "environment": map[string]any{"TARGET": "staging"}}}),
			)},
			{Config: removed, PlanOnly: true, ExpectNonEmptyPlan: false},
			{Config: api.config(`build = {}`), Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckNoResourceAttr(triggerTestAddress, "build.environment"),
				api.checkPayload(map[string]any{"label": "Example trigger", "enabled": true, "build": map[string]any{"environment": map[string]any{}}}),
			)},
			{Config: api.config(`build = {}`), PlanOnly: true, ExpectNonEmptyPlan: false},
		},
	})
}

func TestPipelineWebhookTriggerCreateBeforeDestroy(t *testing.T) {
	api := newWebhookTriggerTestAPI(t)
	config := api.config(`lifecycle { create_before_destroy = true }`) + `
 resource "terraform_data" "sender" {
  input = buildkite_pipeline_webhook_trigger.test.endpoint_url
 }
 `
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(), CheckDestroy: api.checkDestroy,
		Steps: []resource.TestStep{
			{Config: config},
			{Config: strings.Replace(config, `pipeline_id = "receiver-id"`, `pipeline_id = "other-id"`, 1),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(triggerTestAddress, plancheck.ResourceActionCreateBeforeDestroy)}},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(triggerTestAddress, "endpoint_url", "https://example.test/trigger-2"),
					resource.TestCheckResourceAttr("terraform_data.sender", "input", "https://example.test/trigger-2"),
					func(*terraform.State) error {
						api.mu.Lock()
						defer api.mu.Unlock()
						if diff := cmp.Diff([]string{"create", "create", "delete:123e4567-e89b-42d3-a456-000000000001"}, api.events); diff != "" {
							return fmt.Errorf("wrong replacement order: %s", diff)
						}
						return nil
					},
				),
			},
		},
	})
}

func TestPipelineWebhookTriggerParentLookupErrors(t *testing.T) {
	api := newWebhookTriggerTestAPI(t)
	config := api.config("")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(), CheckDestroy: api.checkDestroy,
		Steps: []resource.TestStep{
			{Config: config},
			{PreConfig: api.change(func() { api.parentStatus = http.StatusForbidden }), Config: config, PlanOnly: true, ExpectError: regexp.MustCompile("Unable to resolve receiving pipeline")},
			{PreConfig: api.change(func() { api.parentStatus = 0; api.parentOrganization = "different" }), Config: config, PlanOnly: true, ExpectError: regexp.MustCompile("different organization")},
			{PreConfig: api.change(func() { api.parentOrganization = "example" }), Config: config, PlanOnly: true, ExpectNonEmptyPlan: false},
		},
	})
}

func TestPipelineWebhookTriggerUpdateAfterImport(t *testing.T) {
	api := newWebhookTriggerTestAPI(t)
	api.webhookTriggerRecord = &webhookTriggerRecord{
		id: "123e4567-e89b-42d3-a456-000000000001", pipeline: "receiver", exists: true,
		label: "Existing trigger", enabled: true, branch: "release", commit: "abc123", message: "Deploy release",
		environment: map[string]any{"TARGET": "staging"},
	}
	api.records[api.id] = api.webhookTriggerRecord
	config := api.config(`build = { branch = "release", commit = "abc123", message = "Deploy release" }`) + `
 import {
  to = buildkite_pipeline_webhook_trigger.test
  id = "receiver/123e4567-e89b-42d3-a456-000000000001"
 }
 `
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(), CheckDestroy: api.checkDestroy,
		Steps: []resource.TestStep{
			{Config: config, Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(triggerTestAddress, "build.branch", "release"),
				resource.TestCheckNoResourceAttr(triggerTestAddress, "endpoint_url"),
				api.checkPayload(map[string]any{"label": "Example trigger", "enabled": true, "build": map[string]any{"branch": "release", "commit": "abc123", "message": "Deploy release"}}),
				func(*terraform.State) error {
					api.mu.Lock()
					defer api.mu.Unlock()
					if api.environment["TARGET"] != "staging" {
						return fmt.Errorf("import update removed unknown environment values")
					}
					return nil
				},
			)},
			{Config: config, PlanOnly: true, ExpectNonEmptyPlan: false},
			{Config: api.config(""), Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(triggerTestAddress, "effective_build.branch", "main"),
				resource.TestCheckNoResourceAttr(triggerTestAddress, "build"),
				api.checkPayload(map[string]any{"label": "Example trigger", "enabled": true, "build": map[string]any{"branch": nil, "commit": nil, "message": nil}}),
			)},
		},
	})
}

func TestPipelineWebhookTriggerAdoptObservedOverride(t *testing.T) {
	api := newWebhookTriggerTestAPI(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(), CheckDestroy: api.checkDestroy,
		Steps: []resource.TestStep{
			{Config: api.config("")},
			{PreConfig: api.change(func() { api.branch = "release" }), Config: api.config(`build = { branch = "release" }`)},
			{Config: api.config(""), Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(triggerTestAddress, "effective_build.branch", "main"),
				api.checkPayload(map[string]any{"label": "Example trigger", "enabled": true, "build": map[string]any{"branch": nil}}),
			)},
		},
	})
}
