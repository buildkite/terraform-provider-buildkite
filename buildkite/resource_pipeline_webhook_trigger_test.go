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
type webhookTriggerTestAPI struct {
	t              *testing.T
	server         *httptest.Server
	mu             sync.Mutex
	id             string
	pipeline       string
	label          string
	enabled        bool
	branch         any
	commit         any
	message        any
	environment    map[string]any
	filter         any
	exists         bool
	creates        int
	deletes        int
	lastPayload    map[string]any
	readStatus     int
	writeStatus    int
	omitURL        bool
	triggerType    string
	deleteNotFound bool
}

func newWebhookTriggerTestAPI(t *testing.T) *webhookTriggerTestAPI {
	api := &webhookTriggerTestAPI{t: t, triggerType: "webhook"}
	api.server = httptest.NewServer(http.HandlerFunc(api.handle))
	t.Cleanup(api.server.Close)
	return api
}

func (api *webhookTriggerTestAPI) handle(w http.ResponseWriter, r *http.Request) {
	api.mu.Lock()
	defer api.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
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
	if r.Method != http.MethodPost && (!api.exists || len(parts) != 7 || parts[6] != api.id || parts[4] != api.pipeline) {
		w.WriteHeader(http.StatusNotFound)
		return
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
			if len(parts) != 6 || api.exists || body["type"] != "webhook" {
				api.t.Error("invalid create request")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			api.creates++
			api.id = fmt.Sprintf("123e4567-e89b-42d3-a456-%012d", api.creates)
			api.pipeline, api.exists = parts[4], true
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
  rest_url = %q
  max_retries = 0
 }
 resource "buildkite_pipeline_webhook_trigger" "test" {
  pipeline_slug = "receiver"
  label = "Example trigger"
  %s
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
	if api.exists {
		return fmt.Errorf("trigger still exists")
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
				resource.TestCheckNoResourceAttr(triggerTestAddress, "build"),
				resource.TestCheckNoResourceAttr(triggerTestAddress, "filter"),
				api.checkPayload(map[string]any{"label": "Example trigger", "enabled": true, "filter": nil,
					"build": map[string]any{"branch": nil, "commit": nil, "message": nil, "environment": map[string]any{}}}),
			)},
			{Config: api.config(""), PlanOnly: true, ExpectNonEmptyPlan: false},
			{ResourceName: triggerTestAddress, ImportState: true, ImportStateId: "receiver/123e4567-e89b-42d3-a456-000000000001", ImportStateVerify: true, ImportStateVerifyIgnore: []string{"endpoint_url"}, ImportStateCheck: func(states []*terraform.InstanceState) error {
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
			{Config: strings.Replace(config, `pipeline_slug = "receiver"`, `pipeline_slug = "other"`, 1),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(triggerTestAddress, plancheck.ResourceActionDestroyBeforeCreate)}},
				Check:            resource.TestCheckResourceAttr(triggerTestAddress, "endpoint_url", "https://example.test/trigger-3")},
			// The object can disappear between refresh and DELETE.
			{PreConfig: api.change(func() { api.deleteNotFound = true }), Config: strings.Replace(config, `pipeline_slug = "receiver"`, `pipeline_slug = "other"`, 1), Destroy: true},
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
    pipeline_slug = buildkite_pipeline.receiver.slug
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
			{ResourceName: triggerTestAddress, ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"endpoint_url", "build"}, ImportStateIdFunc: func(state *terraform.State) (string, error) {
				attrs := state.RootModule().Resources[triggerTestAddress].Primary.Attributes
				return attrs["pipeline_slug"] + "/" + attrs["id"], nil
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
				Check: func(*terraform.State) error {
					api.mu.Lock()
					defer api.mu.Unlock()
					if api.branch != "release" {
						return fmt.Errorf("unmanaged branch override was reset")
					}
					return nil
				}},
		},
	})
}
