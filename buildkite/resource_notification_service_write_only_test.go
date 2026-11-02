package buildkite

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/config"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

func TestNotificationServiceWriteOnlyURLLifecycle(t *testing.T) {
	api := newNotificationServiceTestAPI(t)
	const address = "buildkite_notification_service.test"
	const firstURL = "https://example.test/secret-first"
	const rotatedURL = "https://example.test/secret-rotated"

	step := func(description, url string, version int, wantURL string, urlWrites int) resource.TestStep {
		return resource.TestStep{
			Config: strings.Replace(notificationServiceWriteOnlyURLConfig(api.server.URL, description, version),
				"enabled = true", "enabled = false", 1),
			ConfigVariables: config.Variables{"webhook_url": config.StringVariable(url)},
			ConfigPlanChecks: resource.ConfigPlanChecks{
				PreApply: []plancheck.PlanCheck{notificationServicePlanExcludesSecrets{firstURL, rotatedURL}},
			},
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckNoResourceAttr(address, "webhook.url"),
				resource.TestCheckNoResourceAttr(address, "webhook.url_wo"),
				resource.TestCheckResourceAttr(address, "webhook.url_wo_version", fmt.Sprint(version)),
				func(state *terraform.State) error {
					encoded, err := json.Marshal(state)
					if err != nil {
						return err
					}
					for _, secret := range []string{firstURL, rotatedURL} {
						if strings.Contains(string(encoded), secret) {
							return fmt.Errorf("webhook URL was persisted in state")
						}
					}
					return api.verifyWebhookURL(wantURL, urlWrites)
				},
			),
		}
	}

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(version.Must(version.NewVersion("1.11.1")))},
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy: func(*terraform.State) error {
			if !api.isDeleted() {
				return fmt.Errorf("notification service was not deleted")
			}
			return nil
		},
		Steps: []resource.TestStep{
			step("initial", firstURL, 1, firstURL, 1),
			// A different ephemeral value alone must not produce a diff.
			step("initial", rotatedURL, 1, firstURL, 1),
			// An unrelated update must not rotate the URL either.
			step("updated", rotatedURL, 1, firstURL, 1),
			step("updated", rotatedURL, 2, rotatedURL, 2),
			{
				ResourceName:    address,
				ImportState:     true,
				ImportStateId:   notificationServiceTestID,
				ConfigVariables: config.Variables{"webhook_url": config.StringVariable(rotatedURL)},
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					for _, state := range states {
						if state.Attributes["webhook.url"] != "" || state.Attributes["webhook.url_wo"] != "" {
							return fmt.Errorf("import persisted a webhook URL")
						}
					}
					return nil
				},
			},
		},
	})
}

func TestNotificationServiceWriteOnlyURLMigration(t *testing.T) {
	api := newNotificationServiceTestAPI(t)
	const address = "buildkite_notification_service.test"
	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(version.Must(version.NewVersion("1.11.1")))},
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: notificationServiceUnitTestConfig(api.server.URL, "initial", true),
			},
			{
				Config:          notificationServiceWriteOnlyURLConfig(api.server.URL, "initial", 1),
				ConfigVariables: config.Variables{"webhook_url": config.StringVariable("https://example.test/secret-migrated")},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(address, "webhook.url"),
					resource.TestCheckNoResourceAttr(address, "webhook.url_wo"),
					func(*terraform.State) error { return api.verifyWebhookURL("https://example.test/secret-migrated", 2) },
				),
			},
			{
				Config: notificationServiceUnitTestConfig(api.server.URL, "initial", true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "webhook.url", "https://example.test/hook"),
					resource.TestCheckNoResourceAttr(address, "webhook.url_wo_version"),
					func(*terraform.State) error { return api.verifyWebhookURL("https://example.test/hook", 3) },
				),
			},
		},
	})
}

func TestNotificationServiceWriteOnlyURLAfterImport(t *testing.T) {
	api := newNotificationServiceTestAPI(t)
	api.webhookURL = "https://example.test/secret-before-import"
	const address = "buildkite_notification_service.test"
	const newURL = "https://example.test/secret-after-import"
	cfg := notificationServiceWriteOnlyURLConfig(api.server.URL, "managed after import", 1)
	variables := config.Variables{"webhook_url": config.StringVariable(newURL)}
	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(version.Must(version.NewVersion("1.11.1")))},
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:             cfg,
				ConfigVariables:    variables,
				ResourceName:       address,
				ImportState:        true,
				ImportStateId:      notificationServiceTestID,
				ImportStatePersist: true,
				ExpectNonEmptyPlan: true,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					for _, state := range states {
						if state.Attributes["webhook.url"] != "" || state.Attributes["webhook.url_wo"] != "" {
							return fmt.Errorf("import persisted a webhook URL")
						}
					}
					return nil
				},
			},
			{
				Config:          cfg,
				ConfigVariables: variables,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{notificationServicePlanExcludesSecrets{api.webhookURL, newURL}},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(address, "webhook.url"),
					resource.TestCheckNoResourceAttr(address, "webhook.url_wo"),
					resource.TestCheckResourceAttr(address, "webhook.url_wo_version", "1"),
					func(*terraform.State) error { return api.verifyWebhookURL(newURL, 1) },
				),
			},
		},
	})
}

func TestNotificationServiceWriteOnlyURLValidation(t *testing.T) {
	for name, test := range map[string]struct{ settings, wantError string }{
		"conflicting URLs": {`url = "https://example.test/hook"
url_wo = "https://example.test/secret"
url_wo_version = 1`, "Invalid Attribute Combination"},
		"missing version": {`url_wo = "https://example.test/secret"`, "Invalid Attribute Combination"},
		"missing URL":     {`url_wo_version = 1`, "Invalid Attribute Combination"},
		"empty URL": {`url_wo = ""
url_wo_version = 1`, "Invalid Attribute Value Length"},
		"invalid version": {`url_wo = "https://example.test/secret"
url_wo_version = 0`, "Invalid Attribute Value"},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := strings.Replace(notificationServiceUnitTestConfig("http://127.0.0.1", "test", true),
				`url = "https://example.test/hook"`, test.settings, 1)
			resource.UnitTest(t, resource.TestCase{
				TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(version.Must(version.NewVersion("1.11.1")))},
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				Steps:                    []resource.TestStep{{Config: cfg, PlanOnly: true, ExpectError: regexp.MustCompile(test.wantError)}},
			})
		})
	}
}

func notificationServiceWriteOnlyURLConfig(restURL, description string, version int) string {
	return fmt.Sprintf(`
		variable "webhook_url" {
			type = string
			sensitive = true
			ephemeral = true
		}
		%s
	`, strings.Replace(notificationServiceUnitTestConfig(restURL, description, true),
		`url = "https://example.test/hook"`, fmt.Sprintf("url_wo = var.webhook_url\nurl_wo_version = %d", version), 1))
}

type notificationServicePlanExcludesSecrets []string

func (secrets notificationServicePlanExcludesSecrets) CheckPlan(_ context.Context, req plancheck.CheckPlanRequest, resp *plancheck.CheckPlanResponse) {
	encoded, err := json.Marshal(req.Plan)
	if err != nil {
		resp.Error = err
		return
	}
	for _, secret := range secrets {
		if strings.Contains(string(encoded), secret) {
			resp.Error = fmt.Errorf("webhook URL was persisted in the plan")
			return
		}
	}
}

func (api *notificationServiceTestAPI) verifyWebhookURL(wantURL string, wantWrites int) error {
	api.mu.Lock()
	defer api.mu.Unlock()
	if api.webhookURL != wantURL {
		return fmt.Errorf("API webhook URL = %q, want %q", api.webhookURL, wantURL)
	}
	writes := 0
	for _, req := range api.requests {
		if req.Method != http.MethodPost && req.Method != http.MethodPatch {
			continue
		}
		if settings, ok := req.Body["settings"].(map[string]any); ok {
			if _, ok := settings["url"]; ok {
				writes++
			}
		}
	}
	if writes != wantWrites {
		return fmt.Errorf("URL writes = %d, want %d", writes, wantWrites)
	}
	return nil
}

// Inspect the resource response directly: error diagnostics can bypass the
// framework's automatic write-only nullification.
func TestNotificationServiceWriteOnlyURLMalformedSettings(t *testing.T) {
	for _, operation := range []string{"create", "update", "update and disable"} {
		t.Run(operation, func(t *testing.T) {
			ctx := t.Context()
			const secretURL = "https://example.test/secret-malformed-response"
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				requests.Add(1)
				wantMethod := http.MethodPatch
				if operation == "create" {
					wantMethod = http.MethodPost
				}
				if req.Method != wantMethod {
					t.Errorf("method = %s, want %s", req.Method, wantMethod)
				}
				var payload struct {
					Settings struct {
						URL string `json:"url"`
					} `json:"settings"`
				}
				if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
					t.Errorf("decode request: %v", err)
				}
				if payload.Settings.URL != secretURL {
					t.Error("request did not contain the write-only URL")
				}
				response := notificationServiceTestResponse(notificationServiceProviderWebhook)
				response.Settings = json.RawMessage(`{"url":42}`)
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(response); err != nil {
					t.Errorf("encode response: %v", err)
				}
			}))
			defer server.Close()
			r := &notificationServiceResource{client: newRetryTestClient(t, server.URL, 0, time.Millisecond)}
			sch := resourceSchema(ctx, t, r)
			raw := func(url any, version int64, enabled bool) tftypes.Value {
				return nullObjectWith(ctx, t, sch.Type(), map[string]tftypes.Value{
					"id":            tftypes.NewValue(tftypes.String, notificationServiceTestID),
					"provider_type": tftypes.NewValue(tftypes.String, notificationServiceProviderWebhook),
					"enabled":       tftypes.NewValue(tftypes.Bool, enabled),
					"webhook": nullObjectWith(ctx, t, sch.Attributes["webhook"].GetType(), map[string]tftypes.Value{
						"url_wo":         tftypes.NewValue(tftypes.String, url),
						"url_wo_version": tftypes.NewValue(tftypes.Number, version),
					}),
				})
			}
			plan := tfsdk.Plan{Schema: sch, Raw: raw(nil, 2, operation != "update and disable")}
			config := tfsdk.Config{Schema: sch, Raw: raw(secretURL, 2, operation != "update and disable")}
			state := tfsdk.State{Schema: sch, Raw: raw(nil, 1, true)}
			var diags diag.Diagnostics
			if operation == "create" {
				resp := fwresource.CreateResponse{State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}}
				r.Create(ctx, fwresource.CreateRequest{Plan: plan, Config: config}, &resp)
				state, diags = resp.State, resp.Diagnostics
			} else {
				resp := fwresource.UpdateResponse{State: state}
				r.Update(ctx, fwresource.UpdateRequest{Plan: plan, Config: config, State: state}, &resp)
				state, diags = resp.State, resp.Diagnostics
			}
			if !diagnosticsContain(diags, "Unable to read notification service settings") {
				t.Fatalf("expected settings error, got %v", diags)
			}
			var model notificationServiceResourceModel
			if diags := state.Get(ctx, &model); diags.HasError() {
				t.Fatalf("decode returned state: %v", diags)
			}
			if model.Webhook == nil {
				t.Fatal("missing webhook state")
			}
			if !model.Webhook.URLWO.IsNull() || !model.Webhook.URL.IsNull() {
				t.Error("error response contains a credential-bearing URL")
			}
			if !model.ID.Equal(types.StringValue(notificationServiceTestID)) {
				t.Error("error response lost resource identity")
			}
			if requests.Load() != 1 {
				t.Errorf("requests = %d, want 1", requests.Load())
			}
		})
	}
}
