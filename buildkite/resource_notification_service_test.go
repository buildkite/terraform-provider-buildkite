package buildkite

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const notificationServiceTestID = "123e4567-e89b-42d3-a456-426614174000"

func TestNotificationServiceUpdatePayloadOnlyIncludesChanges(t *testing.T) {
	t.Parallel()

	state := notificationServiceResourceModel{
		Description:         types.StringValue("before"),
		BranchConfiguration: types.StringValue("main"),
		Enabled:             types.BoolValue(true),
		Scope:               types.StringValue("all"),
		ScopeUUIDs:          types.SetNull(types.StringType),
		ProviderType:        types.StringValue(notificationServiceProviderDatadogPipelineVisibility),
		DatadogPipelineVisibility: &notificationServiceDatadogPipelineVisibilityModel{
			APIKey:      types.StringValue("secret"),
			DatadogSite: types.StringValue("datadoghq.com"),
			DatadogTags: types.StringValue("team:platform"),
		},
	}
	plan := state
	plan.Description = types.StringValue("after")
	plan.DatadogPipelineVisibility = &notificationServiceDatadogPipelineVisibilityModel{
		APIKey:      types.StringUnknown(),
		DatadogSite: types.StringValue("datadoghq.eu"),
		DatadogTags: types.StringValue("team:platform"),
	}

	got, diags := plan.updatePayload(t.Context(), state)
	if diags.HasError() {
		t.Fatalf("updatePayload() diagnostics = %v", diags)
	}
	want := map[string]any{
		"description": "after",
		"settings": map[string]any{
			"datadog_site": "datadoghq.eu",
		},
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("updatePayload() mismatch (-got +want):\n%s", diff)
	}
}

func TestNotificationServiceScopeUUIDPayloadsUseLowercase(t *testing.T) {
	t.Parallel()

	upperUUID := strings.ToUpper(notificationServiceTestID)
	scopeUUIDs := types.SetValueMust(types.StringType, []attr.Value{types.StringValue(upperUUID)})
	plan := notificationServiceResourceModel{
		ProviderType: types.StringValue(notificationServiceProviderWebhook),
		ScopeUUIDs:   scopeUUIDs,
		Webhook:      &notificationServiceWebhookModel{},
	}

	createPayload, createDiags := plan.createPayload(t.Context())
	if createDiags.HasError() {
		t.Fatalf("createPayload() diagnostics = %v", createDiags)
	}
	if got, want := createPayload["scope_uuids"], []string{notificationServiceTestID}; !cmp.Equal(got, want) {
		t.Errorf("createPayload() scope_uuids mismatch (-got +want):\n%s", cmp.Diff(got, want))
	}

	state := plan
	state.ScopeUUIDs = types.SetNull(types.StringType)
	updatePayload, updateDiags := plan.updatePayload(t.Context(), state)
	if updateDiags.HasError() {
		t.Fatalf("updatePayload() diagnostics = %v", updateDiags)
	}
	if got, want := updatePayload["scope_uuids"], []string{notificationServiceTestID}; !cmp.Equal(got, want) {
		t.Errorf("updatePayload() scope_uuids mismatch (-got +want):\n%s", cmp.Diff(got, want))
	}
}

func TestNotificationServiceUpdatePayloadClearsOptionalValues(t *testing.T) {
	t.Parallel()

	scopeUUIDs := types.SetValueMust(types.StringType, []attr.Value{types.StringValue(notificationServiceTestID)})
	state := notificationServiceResourceModel{
		BranchConfiguration: types.StringValue("main"),
		ScopeUUIDs:          scopeUUIDs,
		ProviderType:        types.StringValue(notificationServiceProviderWebhook),
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
		"scope_uuids":          []string{},
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("updatePayload() mismatch (-got +want):\n%s", diff)
	}

	response := notificationServiceTestResponse(notificationServiceProviderWebhook)
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

func TestNotificationServiceApplyAPIResponsePreservesWriteOnlySettings(t *testing.T) {
	t.Parallel()

	response := notificationServiceTestResponse(notificationServiceProviderDatadogPipelineVisibility)
	response.Settings = json.RawMessage(`{"api_key":"XXXXXXXXcret","datadog_site":"datadoghq.eu","datadog_tags":"team:platform"}`)
	state := notificationServiceResourceModel{
		DatadogPipelineVisibility: &notificationServiceDatadogPipelineVisibilityModel{
			APIKey: types.StringValue("supersecret"),
		},
	}
	previous := state
	if diags := state.applyAPIResponse(t.Context(), &response, previous); diags.HasError() {
		t.Fatalf("applyAPIResponse() diagnostics = %v", diags)
	}
	if got, want := state.DatadogPipelineVisibility.APIKey.ValueString(), "supersecret"; got != want {
		t.Errorf("Datadog API key = %q, want %q", got, want)
	}

	response = notificationServiceTestResponse(notificationServiceProviderAWSEventBridge)
	response.Settings = json.RawMessage(`{"aws_region":"us-east-1","aws_account_id":"XXXXXXXX9012","event_source_name":"aws.partner/test","include_build_meta_data":null}`)
	state = notificationServiceResourceModel{
		AWSEventBridge: &notificationServiceAWSEventBridgeModel{AWSAccountID: types.StringValue("123456789012")},
	}
	previous = state
	if diags := state.applyAPIResponse(t.Context(), &response, previous); diags.HasError() {
		t.Fatalf("applyAPIResponse() diagnostics = %v", diags)
	}
	if got, want := state.AWSEventBridge.AWSAccountID.ValueString(), "123456789012"; got != want {
		t.Errorf("AWS account ID = %q, want %q", got, want)
	}

	response = notificationServiceTestResponse(notificationServiceProviderOpenTelemetryTracing)
	response.Settings = json.RawMessage(`{"endpoint":"https://otel.test","service_name":"buildkite","resource_attributes":{},"tracestate":{}}`)
	headers := types.MapValueMust(types.StringType, map[string]attr.Value{
		"authorization": types.StringValue("secret"),
	})
	state = notificationServiceResourceModel{
		OpenTelemetryTracing: &notificationServiceOpenTelemetryTracingModel{Headers: headers},
	}
	previous = state
	if diags := state.applyAPIResponse(t.Context(), &response, previous); diags.HasError() {
		t.Fatalf("applyAPIResponse() diagnostics = %v", diags)
	}
	if !state.OpenTelemetryTracing.Headers.Equal(headers) {
		t.Errorf("OpenTelemetry headers = %v, want %v", state.OpenTelemetryTracing.Headers, headers)
	}

	state = notificationServiceResourceModel{
		OpenTelemetryTracing: &notificationServiceOpenTelemetryTracingModel{
			Headers: types.MapUnknown(types.StringType),
		},
	}
	previous = state
	if diags := state.applyAPIResponse(t.Context(), &response, previous); diags.HasError() {
		t.Fatalf("applyAPIResponse() diagnostics = %v", diags)
	}
	if !state.OpenTelemetryTracing.Headers.IsNull() {
		t.Errorf("OpenTelemetry headers = %v, want null", state.OpenTelemetryTracing.Headers)
	}
}

func TestNotificationServiceApplyAPIResponseValidatesImportedAWSAccountID(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		mask      string
		accountID string
		wantError bool
	}{
		"matching suffix": {
			mask:      "XXXXXXXX9012",
			accountID: "123456789012",
		},
		"mismatched suffix": {
			mask:      "XXXXXXXX9012",
			accountID: "999999999999",
			wantError: true,
		},
		"fully masked": {
			mask:      "XXXXXXXXXXXX",
			accountID: "123456789012",
		},
		"unknown mask format": {
			mask:      "makarena-9012",
			accountID: "123456789012",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			response := notificationServiceTestResponse(notificationServiceProviderAWSEventBridge)
			settings, err := json.Marshal(map[string]any{
				"aws_region":              "us-east-1",
				"aws_account_id":          test.mask,
				"event_source_name":       "aws.partner/test",
				"include_build_meta_data": nil,
			})
			if err != nil {
				t.Fatalf("marshal settings: %v", err)
			}
			response.Settings = settings

			state := notificationServiceResourceModel{
				ProviderType: types.StringValue(notificationServiceProviderAWSEventBridge),
				AWSEventBridge: &notificationServiceAWSEventBridgeModel{
					AWSAccountID: types.StringValue(test.accountID),
				},
			}
			previous := notificationServiceResourceModel{
				ProviderType: types.StringValue(notificationServiceProviderAWSEventBridge),
				AWSEventBridge: &notificationServiceAWSEventBridgeModel{
					AWSAccountID: types.StringNull(),
				},
			}

			diags := state.applyAPIResponse(t.Context(), &response, previous)
			if got := diags.HasError(); got != test.wantError {
				t.Fatalf("applyAPIResponse() diagnostics HasError() = %t, want %t: %v", got, test.wantError, diags)
			}
			if got := state.AWSEventBridge.AWSAccountID.ValueString(); got != test.accountID {
				t.Errorf("AWS account ID = %q, want %q", got, test.accountID)
			}
		})
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
						return api.verifyPatch(map[string]any{"description": "updated"})
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

func TestNotificationServiceScopeLifecycle(t *testing.T) {
	api := newNotificationServiceTestAPI(t)
	upperUUID := strings.ToUpper(notificationServiceTestID)
	config := func(filters string) string {
		return fmt.Sprintf(`
			provider "buildkite" {
				organization = "test"
				api_token = "test"
				rest_url = %q
				max_retries = 0
			}

			resource "buildkite_notification_service" "test" {
				provider_type = "webhook"
				%s

				webhook = {
					url = "https://example.test/hook"
					events = ["build.finished"]
				}
			}
		`, api.server.URL, filters)
	}
	scopedConfig := config(fmt.Sprintf(`
		scope = "some_projects"
		scope_uuids = [%q]
	`, upperUUID))
	allConfig := config(`scope = "all"`)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: scopedConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("buildkite_notification_service.test", "scope", "some_projects"),
					resource.TestCheckTypeSetElemAttr("buildkite_notification_service.test", "scope_uuids.*", upperUUID),
				),
			},
			{
				Config: allConfig,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("buildkite_notification_service.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("buildkite_notification_service.test", "scope", "all"),
					func(*terraform.State) error {
						return api.verifyPatch(map[string]any{
							"scope":       "all",
							"scope_uuids": []any{},
						})
					},
				),
			},
			{
				Config: allConfig,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("buildkite_notification_service.test", plancheck.ResourceActionNoop),
					},
				},
			},
		},
	})
}

func TestNotificationServiceCannotRemoveSettings(t *testing.T) {
	api := newNotificationServiceTestAPI(t)
	configWithoutSettings := fmt.Sprintf(`
		provider "buildkite" {
			organization = "test"
			api_token = "test"
			rest_url = %q
			max_retries = 0
		}

		resource "buildkite_notification_service" "test" {
			provider_type = "webhook"
		}
	`, api.server.URL)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: notificationServiceUnitTestConfig(api.server.URL, "initial", true),
			},
			{
				Config:      configWithoutSettings,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile("Notification service settings are required"),
			},
		},
	})
}

func TestNotificationServiceEventBridgeLifecycle(t *testing.T) {
	api := newNotificationServiceTestAPI(t)
	api.setProvider(notificationServiceProviderAWSEventBridge)
	config := notificationServiceEventBridgeUnitTestConfig(api.server.URL, "us-east-1", "123456789012", "")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:             config,
				ResourceName:       "buildkite_notification_service.test",
				ImportState:        true,
				ImportStateId:      notificationServiceTestID,
				ImportStatePersist: true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("buildkite_notification_service.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.TestCheckResourceAttr("buildkite_notification_service.test", "aws_event_bridge.aws_account_id", "123456789012"),
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("buildkite_notification_service.test", plancheck.ResourceActionNoop),
					},
				},
			},
			{
				Config: notificationServiceEventBridgeUnitTestConfig(api.server.URL, "us-east-1", "123456789013", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("buildkite_notification_service.test", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.TestCheckResourceAttr("buildkite_notification_service.test", "aws_event_bridge.aws_account_id", "123456789013"),
			},
			{
				Config: notificationServiceEventBridgeUnitTestConfig(api.server.URL, "us-west-2", "123456789013", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("buildkite_notification_service.test", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.TestCheckResourceAttr("buildkite_notification_service.test", "aws_event_bridge.aws_region", "us-west-2"),
			},
		},
	})
}

func TestNotificationServiceEventBridgeMetadataLifecycle(t *testing.T) {
	api := newNotificationServiceTestAPI(t)
	api.setProvider(notificationServiceProviderAWSEventBridge)
	withMetadata := notificationServiceEventBridgeUnitTestConfig(
		api.server.URL,
		"us-east-1",
		"123456789012",
		`include_build_meta_data = "deploy:*"`,
	)
	withoutMetadata := notificationServiceEventBridgeUnitTestConfig(api.server.URL, "us-east-1", "123456789012", "")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: withMetadata,
				Check: resource.TestCheckResourceAttr(
					"buildkite_notification_service.test",
					"aws_event_bridge.include_build_meta_data",
					"deploy:*",
				),
			},
			{
				Config: withoutMetadata,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("buildkite_notification_service.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: func(*terraform.State) error {
					return api.verifyPatch(map[string]any{
						"settings": map[string]any{"include_build_meta_data": nil},
					})
				},
			},
			{
				Config: withoutMetadata,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("buildkite_notification_service.test", plancheck.ResourceActionNoop),
					},
				},
			},
		},
	})
}

func TestNotificationServiceDatadogLifecycle(t *testing.T) {
	api := newNotificationServiceTestAPI(t)
	api.setProvider(notificationServiceProviderDatadogPipelineVisibility)
	config := notificationServiceDatadogUnitTestConfig(api.server.URL, "supersecret")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:             config,
				ResourceName:       "buildkite_notification_service.test",
				ImportState:        true,
				ImportStateId:      notificationServiceTestID,
				ImportStatePersist: true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("buildkite_notification_service.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("buildkite_notification_service.test", "datadog_pipeline_visibility.api_key", "supersecret"),
					func(*terraform.State) error {
						return api.verifyPatch(map[string]any{
							"settings": map[string]any{"api_key": "supersecret"},
						})
					},
				),
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("buildkite_notification_service.test", plancheck.ResourceActionNoop),
					},
				},
			},
		},
	})
}

func TestNotificationServiceOpenTelemetryDefaults(t *testing.T) {
	api := newNotificationServiceTestAPI(t)
	api.setProvider(notificationServiceProviderOpenTelemetryTracing)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy: func(*terraform.State) error {
			if !api.isDeleted() {
				return fmt.Errorf("notification service was not deleted")
			}
			return nil
		},
		Steps: []resource.TestStep{{
			Config: notificationServiceOpenTelemetryUnitTestConfig(api.server.URL),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("buildkite_notification_service.test", "open_telemetry_tracing.endpoint", "https://otel.test"),
				resource.TestCheckResourceAttr("buildkite_notification_service.test", "open_telemetry_tracing.service_name", "buildkite"),
			),
		}},
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

func TestNotificationServiceUpdateTracksPatchBeforeDisableError(t *testing.T) {
	api := newNotificationServiceTestAPI(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: notificationServiceUnitTestConfig(api.server.URL, "initial", true),
			},
			{
				PreConfig: func() {
					api.setFailDisable(true)
				},
				Config:      notificationServiceUnitTestConfig(api.server.URL, "updated", false),
				ExpectError: regexp.MustCompile("Unable to change notification service enabled state"),
			},
			{
				PreConfig: func() {
					api.setFailDisable(false)
				},
				Config: notificationServiceUnitTestConfig(api.server.URL, "updated", false),
				Check: func(*terraform.State) error {
					return api.verifyPatch(map[string]any{"description": "updated"})
				},
			},
		},
	})
}

func TestNotificationServiceOAuthImportUpdatesCommonFields(t *testing.T) {
	api := newNotificationServiceTestAPI(t)
	api.setProvider(notificationServiceProviderSlackWorkspace)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:             notificationServiceOAuthUnitTestConfig(api.server.URL, notificationServiceProviderSlackWorkspace, "", ""),
				ResourceName:       "buildkite_notification_service.test",
				ImportState:        true,
				ImportStateId:      notificationServiceTestID,
				ImportStatePersist: true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: notificationServiceOAuthUnitTestConfig(api.server.URL, notificationServiceProviderSlackWorkspace, "managed after import", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("buildkite_notification_service.test", "provider_type", notificationServiceProviderSlackWorkspace),
					resource.TestCheckResourceAttr("buildkite_notification_service.test", "description", "managed after import"),
				),
			},
		},
	})
}

func TestNotificationServiceOAuthImportRejectsNotificationFilters(t *testing.T) {
	tests := map[string]struct {
		providerType string
		filter       string
	}{
		"Slack Workspace branch": {
			providerType: notificationServiceProviderSlackWorkspace,
			filter:       `branch_configuration = "main"`,
		},
		"Slack Workspace scope": {
			providerType: notificationServiceProviderSlackWorkspace,
			filter: fmt.Sprintf(`
				scope = "some_projects"
				scope_uuids = [%q]
			`, notificationServiceTestID),
		},
		"Linear branch": {
			providerType: notificationServiceProviderLinear,
			filter:       `branch_configuration = "main"`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			api := newNotificationServiceTestAPI(t)
			api.setProvider(test.providerType)

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				Steps: []resource.TestStep{
					{
						Config:             notificationServiceOAuthUnitTestConfig(api.server.URL, test.providerType, "", ""),
						ResourceName:       "buildkite_notification_service.test",
						ImportState:        true,
						ImportStateId:      notificationServiceTestID,
						ImportStatePersist: true,
						ExpectNonEmptyPlan: true,
					},
					{
						Config:      notificationServiceOAuthUnitTestConfig(api.server.URL, test.providerType, "", test.filter),
						PlanOnly:    true,
						ExpectError: regexp.MustCompile("Notification filters are not supported"),
					},
				},
			})
		})
	}
}

func TestNotificationServiceImportRejectsUnsupportedProvider(t *testing.T) {
	api := newNotificationServiceTestAPI(t)
	api.setProvider("slack")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{{
			Config:        notificationServiceUnitTestConfig(api.server.URL, "", true),
			ResourceName:  "buildkite_notification_service.test",
			ImportState:   true,
			ImportStateId: notificationServiceTestID,
			ExpectError:   regexp.MustCompile(`provider_type "slack".*not supported`),
		}},
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
		"settings mismatch": {
			resourceConfig: `
				provider_type = "webhook"
				datadog_pipeline_visibility = { api_key = "secret" }
			`,
			wantError: "settings do not match provider_type",
		},
		"multiple settings objects": {
			resourceConfig: `
				provider_type = "webhook"
				webhook = { url = "https://example.test/hook" }
				datadog_pipeline_visibility = { api_key = "secret" }
			`,
			wantError: "Multiple notification service settings configured",
		},
		"missing required setting": {
			resourceConfig: `
				provider_type = "webhook"
				webhook = {}
			`,
			wantError: "webhook.url is required",
		},
		"scope UUIDs with all scope": {
			resourceConfig: fmt.Sprintf(`
				provider_type = "webhook"
				scope = "all"
				scope_uuids = [%q]
				webhook = { url = "https://example.test/hook" }
			`, notificationServiceTestID),
			wantError: "scope UUIDs require a scoped service",
		},
		"case-insensitive duplicate scope UUIDs": {
			resourceConfig: fmt.Sprintf(`
				provider_type = "webhook"
				scope = "some_projects"
				scope_uuids = [%q, %q]
				webhook = { url = "https://example.test/hook" }
			`, notificationServiceTestID, strings.ToUpper(notificationServiceTestID)),
			wantError: "Duplicate notification service scope UUID",
		},
		// Supported providers ignore build-state flags. Keep this rejection pinned so
		// the field is not accidentally exposed without a provider that can use it.
		"build states": {
			resourceConfig: `
				provider_type = "webhook"
				build_states = { build_failed = true }
				webhook = { url = "https://example.test/hook" }
			`,
			wantError: "(?s)Unsupported argument.*build_states",
		},
		"OAuth creation": {
			resourceConfig: `provider_type = "linear"`,
			wantError:      "OAuth-managed notification service cannot be created",
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

func TestNotificationServiceAllowsEmptyScopedService(t *testing.T) {
	config := `
		provider "buildkite" {
			organization = "test"
			api_token = "test"
			rest_url = "http://127.0.0.1"
			max_retries = 0
		}

		resource "buildkite_notification_service" "test" {
			provider_type = "webhook"
			scope = "some_projects"
			scope_uuids = []
			webhook = { url = "https://example.test/hook" }
		}
	`

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{{
			Config:             config,
			PlanOnly:           true,
			ExpectNonEmptyPlan: true,
		}},
	})
}

func TestNotificationServiceAllowsComputedScopeUUID(t *testing.T) {
	config := `
		provider "buildkite" {
			organization = "test"
			api_token = "test"
			rest_url = "http://127.0.0.1"
			max_retries = 0
		}

		resource "terraform_data" "scope" {}

		resource "buildkite_notification_service" "test" {
			provider_type = "webhook"
			scope = "some_projects"
			scope_uuids = [terraform_data.scope.id]
			webhook = { url = "https://example.test/hook" }
		}
	`

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{{
			Config:             config,
			PlanOnly:           true,
			ExpectNonEmptyPlan: true,
		}},
	})
}

func TestAccBuildkiteNotificationService(t *testing.T) {
	random := acctest.RandString(10)
	config := func(description string, enabled bool) string {
		return fmt.Sprintf(`
			resource "buildkite_notification_service" "test" {
				provider_type = "webhook"
				description = %q
				enabled = %t

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

			webhook = {
				url = "https://example.test/hook"
				token = "terraform-secret"
				events = ["build.finished"]
			}
		}
	`, restURL, description, enabled)
}

func notificationServiceOAuthUnitTestConfig(restURL, providerType, description, extra string) string {
	return fmt.Sprintf(`
		provider "buildkite" {
			organization = "test"
			api_token = "test"
			rest_url = %q
			max_retries = 0
		}

		resource "buildkite_notification_service" "test" {
			provider_type = %q
			description = %q
			%s
		}
	`, restURL, providerType, description, extra)
}

func notificationServiceDatadogUnitTestConfig(restURL, apiKey string) string {
	return fmt.Sprintf(`
		provider "buildkite" {
			organization = "test"
			api_token = "test"
			rest_url = %q
			max_retries = 0
		}

		resource "buildkite_notification_service" "test" {
			provider_type = "datadog_pipeline_visibility"

			datadog_pipeline_visibility = {
				api_key = %q
			}
		}
	`, restURL, apiKey)
}

func notificationServiceOpenTelemetryUnitTestConfig(restURL string) string {
	return fmt.Sprintf(`
		provider "buildkite" {
			organization = "test"
			api_token = "test"
			rest_url = %q
			max_retries = 0
		}

		resource "buildkite_notification_service" "test" {
			provider_type = "open_telemetry_tracing"

			open_telemetry_tracing = {
				endpoint = "https://otel.test"
			}
		}
	`, restURL)
}

func notificationServiceEventBridgeUnitTestConfig(restURL, region, accountID, extra string) string {
	return fmt.Sprintf(`
		provider "buildkite" {
			organization = "test"
			api_token = "test"
			rest_url = %q
			max_retries = 0
		}

		resource "buildkite_notification_service" "test" {
			provider_type = "aws_event_bridge"

			aws_event_bridge = {
				aws_region = %q
				aws_account_id = %q
				%s
			}
		}
	`, restURL, region, accountID, extra)
}

type notificationServiceTestRequest struct {
	Method      string
	Path        string
	ContentType string
	Body        map[string]any
}

type notificationServiceTestAPI struct {
	t                    *testing.T
	server               *httptest.Server
	mu                   sync.Mutex
	deleted              bool
	description          string
	enabled              bool
	failDisable          bool
	providerType         string
	scope                string
	scopeUUIDs           []string
	awsRegion            string
	awsAccountID         string
	includeBuildMetadata *string
	requests             []notificationServiceTestRequest
}

func newNotificationServiceTestAPI(t *testing.T) *notificationServiceTestAPI {
	t.Helper()
	api := &notificationServiceTestAPI{
		t:            t,
		enabled:      true,
		providerType: notificationServiceProviderWebhook,
		scope:        "all",
		scopeUUIDs:   []string{},
		awsRegion:    "us-east-1",
		awsAccountID: "123456789012",
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
		api.applyRequestBody(body)
		api.writeResponse(w, http.StatusCreated)
	case req.Method == http.MethodGet && req.URL.Path == resourcePath:
		if api.deleted {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		api.writeResponse(w, http.StatusOK)
	case req.Method == http.MethodPatch && req.URL.Path == resourcePath:
		api.applyRequestBody(body)
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

func (api *notificationServiceTestAPI) applyRequestBody(body map[string]any) {
	if description, ok := body["description"].(string); ok {
		api.description = description
	}
	if scope, ok := body["scope"].(string); ok {
		api.scope = scope
	}
	if rawScopeUUIDs, ok := body["scope_uuids"].([]any); ok {
		api.scopeUUIDs = make([]string, 0, len(rawScopeUUIDs))
		for _, rawScopeUUID := range rawScopeUUIDs {
			if scopeUUID, ok := rawScopeUUID.(string); ok {
				api.scopeUUIDs = append(api.scopeUUIDs, scopeUUID)
			}
		}
	}
	if settings, ok := body["settings"].(map[string]any); ok {
		if region, ok := settings["aws_region"].(string); ok {
			api.awsRegion = region
		}
		if accountID, ok := settings["aws_account_id"].(string); ok {
			api.awsAccountID = accountID
		}
		if metadata, ok := settings["include_build_meta_data"]; ok {
			if metadata == nil {
				api.includeBuildMetadata = nil
			} else if metadata, ok := metadata.(string); ok {
				api.includeBuildMetadata = &metadata
			}
		}
	}
}

func (api *notificationServiceTestAPI) writeResponse(w http.ResponseWriter, status int) {
	response := notificationServiceTestResponse(api.providerType)
	if api.providerType == notificationServiceProviderAWSEventBridge {
		maskedAccountID := "XXXXXXXX" + api.awsAccountID[len(api.awsAccountID)-4:]
		eventSourceName := "aws.partner/buildkite.com.test/test"
		settings, err := json.Marshal(notificationServiceAWSEventBridgeAPISettings{
			AWSRegion:            &api.awsRegion,
			AWSAccountID:         &maskedAccountID,
			EventSourceName:      &eventSourceName,
			IncludeBuildMetadata: api.includeBuildMetadata,
		})
		if err != nil {
			api.t.Errorf("encode EventBridge settings: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		response.Settings = settings
	}
	if api.description != "" {
		response.Description = &api.description
	}
	response.Enabled = api.enabled
	response.Scope = api.scope
	response.ScopeUUIDs = api.scopeUUIDs
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		api.t.Errorf("encode response: %v", err)
	}
}

func (api *notificationServiceTestAPI) verifyPatch(want map[string]any) error {
	api.mu.Lock()
	defer api.mu.Unlock()
	patches := 0
	for _, request := range api.requests {
		if request.Method != http.MethodPatch {
			continue
		}
		patches++
		if request.ContentType != "application/json" {
			return fmt.Errorf("PATCH Content-Type = %q, want application/json", request.ContentType)
		}
		if diff := cmp.Diff(request.Body, want); diff != "" {
			return fmt.Errorf("PATCH body mismatch (-got +want):\n%s", diff)
		}
	}
	if patches != 1 {
		return fmt.Errorf("PATCH requests = %d, want 1", patches)
	}
	return nil
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

func (api *notificationServiceTestAPI) setFailDisable(fail bool) {
	api.mu.Lock()
	defer api.mu.Unlock()
	api.failDisable = fail
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
	case notificationServiceProviderAWSEventBridge:
		response.Settings = json.RawMessage(`{
				"aws_region":"us-east-1",
				"aws_account_id":"XXXXXXXX9012",
				"event_source_name":"aws.partner/buildkite.com.test/test",
				"include_build_meta_data":null
			}`)
	case notificationServiceProviderDatadogPipelineVisibility:
		response.Settings = json.RawMessage(`{
			"api_key":"XXXXXXXXcret",
			"datadog_site":"datadoghq.com",
			"datadog_tags":null
		}`)
	case notificationServiceProviderOpenTelemetryTracing:
		response.Settings = json.RawMessage(`{
			"endpoint":"https://otel.test",
			"service_name":"buildkite",
			"resource_attributes":{},
			"tracestate":{}
		}`)
	}
	return response
}
