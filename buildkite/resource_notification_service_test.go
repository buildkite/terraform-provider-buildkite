package buildkite

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const notificationServiceTestID = "123e4567-e89b-42d3-a456-426614174000"

func TestNotificationServiceUpdatePayloadClearsOptionalValues(t *testing.T) {
	t.Parallel()

	scopeUUIDs := types.SetValueMust(types.StringType, []attr.Value{types.StringValue(notificationServiceTestID)})
	state := notificationServiceResourceModel{
		BranchConfiguration: types.StringValue("main"),
		ScopeUUIDs:          scopeUUIDs,
		ProviderType:        types.StringValue(notificationServiceProviderSlackWorkspace),
	}
	plan := state
	plan.BranchConfiguration = types.StringNull()
	plan.ScopeUUIDs = types.SetNull(types.StringType)

	got, diags := plan.updatePayload(t.Context(), state)
	if diags.HasError() {
		t.Fatalf("updatePayload() diagnostics = %v", diags)
	}
	want := map[string]any{
		"branch_configuration": nil,
		"scope_uuids":          nil,
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("updatePayload() mismatch (-got +want):\n%s", diff)
	}

	response := notificationServiceTestResponse(notificationServiceProviderSlackWorkspace)
	if applyDiags := plan.applyAPIResponse(t.Context(), &response, state); applyDiags.HasError() {
		t.Fatalf("applyAPIResponse() diagnostics = %v", applyDiags)
	}
	if !plan.BranchConfiguration.IsNull() {
		t.Errorf("BranchConfiguration = %v, want null", plan.BranchConfiguration)
	}
	if !plan.ScopeUUIDs.IsNull() {
		t.Errorf("ScopeUUIDs = %v, want null", plan.ScopeUUIDs)
	}
}

func TestNotificationServiceRESTLifecycle(t *testing.T) {
	api := newNotificationServiceTestAPI(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy: func(*terraform.State) error {
			if !api.isDeleted() {
				return fmt.Errorf("notification service was not deleted")
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: notificationServiceUnitTestConfig(api.server.URL, "initial", false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("buildkite_notification_service.test", "id", notificationServiceTestID),
					resource.TestCheckResourceAttr("buildkite_notification_service.test", "enabled", "false"),
					resource.TestCheckResourceAttr("buildkite_notification_service.test", "webhook.token", "terraform-secret"),
					func(*terraform.State) error {
						return api.verifyCreateDisableSequence()
					},
				),
			},
			{
				Config: notificationServiceUnitTestConfig(api.server.URL, "updated", true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("buildkite_notification_service.test", "description", "updated"),
					resource.TestCheckResourceAttr("buildkite_notification_service.test", "enabled", "true"),
					func(*terraform.State) error {
						return api.verifyPartialPatch()
					},
				),
			},
			{
				ResourceName:      "buildkite_notification_service.test",
				ImportState:       true,
				ImportStateId:     notificationServiceTestID,
				ImportStateVerify: true,
			},
		},
	})
}

func TestNotificationServiceCreateTracksResourceBeforeDisableError(t *testing.T) {
	api := newNotificationServiceTestAPI(t)
	api.failDisable = true

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy: func(*terraform.State) error {
			if !api.isDeleted() {
				return fmt.Errorf("notification service was not tracked and deleted after disable failed")
			}
			return nil
		},
		Steps: []resource.TestStep{{
			Config:      notificationServiceUnitTestConfig(api.server.URL, "initial", false),
			ExpectError: regexp.MustCompile("Unable to disable notification service"),
		}},
	})
}

func TestNotificationServiceOAuthImportUpdatesCommonFields(t *testing.T) {
	api := newNotificationServiceTestAPI(t)
	api.setProvider(notificationServiceProviderSlackWorkspace)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:             notificationServiceOAuthUnitTestConfig(api.server.URL, ""),
				ResourceName:       "buildkite_notification_service.test",
				ImportState:        true,
				ImportStateId:      notificationServiceTestID,
				ImportStatePersist: true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: notificationServiceOAuthUnitTestConfig(api.server.URL, "managed after import"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("buildkite_notification_service.test", "provider_type", notificationServiceProviderSlackWorkspace),
					resource.TestCheckResourceAttr("buildkite_notification_service.test", "description", "managed after import"),
				),
			},
		},
	})
}

func TestNotificationServiceReadRemovesMissingResource(t *testing.T) {
	api := newNotificationServiceTestAPI(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: notificationServiceUnitTestConfig(api.server.URL, "initial", true),
			},
			{
				PreConfig: func() {
					api.remove()
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
				Check: func(state *terraform.State) error {
					if _, ok := state.RootModule().Resources["buildkite_notification_service.test"]; ok {
						return fmt.Errorf("missing notification service was not removed from state")
					}
					return nil
				},
			},
		},
	})
}

func TestNotificationServiceConfigurationValidation(t *testing.T) {
	tests := map[string]struct {
		resourceConfig string
		wantError      string
	}{
		"missing required setting": {
			resourceConfig: `
				provider_type = "webhook"
				webhook = {}
			`,
			wantError: "webhook.url is required",
		},
		"Slack Workspace creation": {
			resourceConfig: `provider_type = "slack_workspace"`,
			wantError:      "OAuth-managed notification service cannot be created",
		},
		"legacy Slack": {
			resourceConfig: `provider_type = "slack"`,
			wantError:      "Attribute provider_type value must be one of",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			config := fmt.Sprintf(`
				provider "buildkite" {
					organization = "test"
					api_token = "test"
					rest_url = "http://127.0.0.1"
					max_retries = 0
				}

				resource "buildkite_notification_service" "test" {
					%s
				}
			`, test.resourceConfig)

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				Steps: []resource.TestStep{{
					Config:      config,
					PlanOnly:    true,
					ExpectError: regexp.MustCompile(test.wantError),
				}},
			})
		})
	}
}

func TestAccBuildkiteNotificationService(t *testing.T) {
	random := acctest.RandString(10)
	config := func(description string, enabled bool) string {
		return fmt.Sprintf(`
			resource "buildkite_notification_service" "test" {
				provider_type = "webhook"
				description = %q
				enabled = %t

				build_states = {
					build_failed = true
					build_fixed = true
				}

				webhook = {
					url = "https://example.com/buildkite-webhook-%s"
					events = ["build.finished"]
				}
			}
		`, description, enabled, random)
	}

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckNotificationServiceDestroy,
		Steps: []resource.TestStep{
			{
				Config: config("Terraform acceptance test "+random, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("buildkite_notification_service.test", "provider_type", notificationServiceProviderWebhook),
					resource.TestCheckResourceAttr("buildkite_notification_service.test", "enabled", "false"),
					resource.TestCheckResourceAttr("buildkite_notification_service.test", "build_states.build_failed", "true"),
					resource.TestCheckResourceAttr("buildkite_notification_service.test", "build_states.build_fixed", "true"),
					resource.TestCheckResourceAttrSet("buildkite_notification_service.test", "id"),
				),
			},
			{
				Config: config("Updated Terraform acceptance test "+random, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("buildkite_notification_service.test", "description", "Updated Terraform acceptance test "+random),
					resource.TestCheckResourceAttr("buildkite_notification_service.test", "enabled", "true"),
				),
			},
			{
				ResourceName:      "buildkite_notification_service.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(state *terraform.State) (string, error) {
					resourceState, ok := state.RootModule().Resources["buildkite_notification_service.test"]
					if !ok {
						return "", fmt.Errorf("notification service not found in state")
					}
					return resourceState.Primary.Attributes["id"], nil
				},
			},
		},
	})
}

func testAccCheckNotificationServiceDestroy(state *terraform.State) error {
	client := getTestClient()
	for _, resourceState := range state.RootModule().Resources {
		if resourceState.Type != "buildkite_notification_service" {
			continue
		}
		resource := &notificationServiceResource{client: client}
		_, err := resource.get(context.Background(), resourceState.Primary.Attributes["id"])
		if err == nil {
			return fmt.Errorf("notification service still exists")
		}
		if !isAPIStatus(err, http.StatusNotFound) {
			return fmt.Errorf("checking notification service deletion: %w", err)
		}
	}
	return nil
}

func notificationServiceUnitTestConfig(restURL, description string, enabled bool) string {
	return fmt.Sprintf(`
		provider "buildkite" {
			organization = "test"
			api_token = "test"
			rest_url = %q
			max_retries = 0
		}

		resource "buildkite_notification_service" "test" {
			provider_type = "webhook"
			description = %q
			enabled = %t

			build_states = {
				build_failed = true
			}

			webhook = {
				url = "https://example.test/hook"
				token = "terraform-secret"
				events = ["build.finished"]
			}
		}
	`, restURL, description, enabled)
}

func notificationServiceOAuthUnitTestConfig(restURL, description string) string {
	return fmt.Sprintf(`
		provider "buildkite" {
			organization = "test"
			api_token = "test"
			rest_url = %q
			max_retries = 0
		}

		resource "buildkite_notification_service" "test" {
			provider_type = "slack_workspace"
			description = %q
		}
	`, restURL, description)
}

type notificationServiceTestRequest struct {
	Method      string
	Path        string
	ContentType string
	Body        map[string]any
}

type notificationServiceTestAPI struct {
	t            *testing.T
	server       *httptest.Server
	mu           sync.Mutex
	deleted      bool
	description  string
	enabled      bool
	buildFailed  bool
	failDisable  bool
	providerType string
	requests     []notificationServiceTestRequest
}

func newNotificationServiceTestAPI(t *testing.T) *notificationServiceTestAPI {
	t.Helper()
	api := &notificationServiceTestAPI{
		t:            t,
		enabled:      true,
		providerType: notificationServiceProviderWebhook,
	}
	api.server = httptest.NewServer(http.HandlerFunc(api.handle))
	t.Cleanup(api.server.Close)
	return api
}

func (api *notificationServiceTestAPI) handle(w http.ResponseWriter, req *http.Request) {
	api.mu.Lock()
	defer api.mu.Unlock()

	var body map[string]any
	if req.Body != nil && req.ContentLength != 0 {
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			api.t.Errorf("decode %s request: %v", req.Method, err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}
	api.requests = append(api.requests, notificationServiceTestRequest{
		Method:      req.Method,
		Path:        req.URL.Path,
		ContentType: req.Header.Get("Content-Type"),
		Body:        body,
	})

	collectionPath := "/v2/organizations/test/services"
	resourcePath := collectionPath + "/" + notificationServiceTestID
	switch {
	case req.Method == http.MethodPost && req.URL.Path == collectionPath:
		api.deleted = false
		api.enabled = true
		api.description, _ = body["description"].(string)
		if buildStates, ok := body["build_states"].(map[string]any); ok {
			api.buildFailed, _ = buildStates["build_failed"].(bool)
		}
		api.writeResponse(w, http.StatusCreated)
	case req.Method == http.MethodGet && req.URL.Path == resourcePath:
		if api.deleted {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		api.writeResponse(w, http.StatusOK)
	case req.Method == http.MethodPatch && req.URL.Path == resourcePath:
		if description, ok := body["description"].(string); ok {
			api.description = description
		}
		api.writeResponse(w, http.StatusOK)
	case req.Method == http.MethodPut && req.URL.Path == resourcePath+"/disable":
		if api.failDisable {
			http.Error(w, `{"message":"unable to disable"}`, http.StatusUnprocessableEntity)
			return
		}
		api.enabled = false
		api.writeResponse(w, http.StatusOK)
	case req.Method == http.MethodPut && req.URL.Path == resourcePath+"/enable":
		api.enabled = true
		api.writeResponse(w, http.StatusOK)
	case req.Method == http.MethodDelete && req.URL.Path == resourcePath:
		api.deleted = true
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, `{"message":"unexpected request"}`, http.StatusNotFound)
	}
}

func (api *notificationServiceTestAPI) writeResponse(w http.ResponseWriter, status int) {
	response := notificationServiceTestResponse(api.providerType)
	if api.description != "" {
		response.Description = &api.description
	}
	response.Enabled = api.enabled
	response.BuildStates.BuildFailed = api.buildFailed
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		api.t.Errorf("encode response: %v", err)
	}
}

func (api *notificationServiceTestAPI) verifyPartialPatch() error {
	api.mu.Lock()
	defer api.mu.Unlock()
	for index := len(api.requests) - 1; index >= 0; index-- {
		request := api.requests[index]
		if request.Method != http.MethodPatch {
			continue
		}
		if request.ContentType != "application/json" {
			return fmt.Errorf("PATCH Content-Type = %q, want application/json", request.ContentType)
		}
		want := map[string]any{"description": "updated"}
		if diff := cmp.Diff(request.Body, want); diff != "" {
			return fmt.Errorf("PATCH body mismatch (-got +want):\n%s", diff)
		}
		return nil
	}
	return fmt.Errorf("no PATCH request was received")
}

func (api *notificationServiceTestAPI) verifyCreateDisableSequence() error {
	api.mu.Lock()
	defer api.mu.Unlock()
	var createIndex, disableIndex = -1, -1
	for index, request := range api.requests {
		if request.Method == http.MethodPost {
			createIndex = index
		}
		if request.Method == http.MethodPut && request.Path == "/v2/organizations/test/services/"+notificationServiceTestID+"/disable" {
			disableIndex = index
		}
	}
	if createIndex == -1 || disableIndex == -1 {
		return fmt.Errorf("create/disable requests not found: %#v", api.requests)
	}
	if createIndex > disableIndex {
		return fmt.Errorf("disable request preceded create request")
	}
	return nil
}

func (api *notificationServiceTestAPI) remove() {
	api.mu.Lock()
	defer api.mu.Unlock()
	api.deleted = true
}

func (api *notificationServiceTestAPI) setProvider(providerType string) {
	api.mu.Lock()
	defer api.mu.Unlock()
	api.providerType = providerType
}

func (api *notificationServiceTestAPI) isDeleted() bool {
	api.mu.Lock()
	defer api.mu.Unlock()
	return api.deleted
}

func notificationServiceTestResponse(providerType string) notificationServiceAPIResponse {
	response := notificationServiceAPIResponse{
		ID:                  notificationServiceTestID,
		Enabled:             true,
		Scope:               "all",
		ScopeUUIDs:          []string{},
		BranchConfiguration: "",
		Settings:            json.RawMessage(`{}`),
		CreatedAt:           "2026-08-12T00:00:00Z",
	}
	response.Provider.ID = providerType
	switch providerType {
	case notificationServiceProviderWebhook:
		response.Settings = json.RawMessage(`{
			"url":"https://example.test/hook",
			"token":"terraform-secret",
			"token_mode":"token",
			"version":3,
			"events":["build.finished"],
			"tls_verify":true
		}`)
	}
	return response
}
