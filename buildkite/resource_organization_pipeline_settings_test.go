package buildkite

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestPipelineSettingsPatch(t *testing.T) {
	t.Parallel()

	str := func(s string) *string { return &s }
	num := func(i int64) *int64 { return &i }
	settings := func(branch, cluster *string, defaultTimeout, maximumTimeout, expiry *int64) organizationPipelineSettings {
		return organizationPipelineSettings{
			DefaultBranch:               branch,
			DefaultClusterID:            cluster,
			DefaultTimeoutInMinutes:     defaultTimeout,
			MaximumTimeoutInMinutes:     maximumTimeout,
			ScheduledJobExpiryInMinutes: expiry,
		}
	}
	unset := organizationPipelineSettingsResourceModel{
		DefaultBranch:               types.StringNull(),
		DefaultClusterID:            types.StringNull(),
		DefaultTimeoutInMinutes:     types.Int64Null(),
		MaximumTimeoutInMinutes:     types.Int64Null(),
		ScheduledJobExpiryInMinutes: types.Int64Null(),
	}
	withBranch := func(branch types.String) organizationPipelineSettingsResourceModel {
		model := unset
		model.DefaultBranch = branch
		return model
	}
	withCluster := func(cluster types.String) organizationPipelineSettingsResourceModel {
		model := unset
		model.DefaultClusterID = cluster
		return model
	}
	withTimeouts := func(defaultTimeout, maximumTimeout, expiry types.Int64) organizationPipelineSettingsResourceModel {
		model := unset
		model.DefaultTimeoutInMinutes = defaultTimeout
		model.MaximumTimeoutInMinutes = maximumTimeout
		model.ScheduledJobExpiryInMinutes = expiry
		return model
	}

	testCases := []struct {
		name    string
		config  organizationPipelineSettingsResourceModel
		plan    organizationPipelineSettingsResourceModel
		current organizationPipelineSettings
		want    string
	}{
		{
			"unset attributes are not sent",
			unset,
			withBranch(types.StringUnknown()),
			settings(str("main"), nil, num(30), nil, num(43200)),
			`{}`,
		},
		{
			"values kept from state for unset attributes are not sent",
			unset,
			withBranch(types.StringValue("main")),
			settings(nil, nil, nil, nil, nil),
			`{}`,
		},
		{
			"unchanged values are not sent",
			withBranch(types.StringValue("main")),
			withBranch(types.StringValue("main")),
			settings(str("main"), nil, nil, nil, nil),
			`{}`,
		},
		{
			"a changed branch is sent",
			withBranch(types.StringValue("trunk")),
			withBranch(types.StringValue("trunk")),
			settings(str("main"), nil, nil, nil, nil),
			`{"default_branch":"trunk"}`,
		},
		{
			"a branch is sent when the organization has none",
			withBranch(types.StringValue("main")),
			withBranch(types.StringValue("main")),
			settings(nil, nil, nil, nil, nil),
			`{"default_branch":"main"}`,
		},
		{
			"a changed cluster is sent",
			withCluster(types.StringValue("3f4b6df0-1234-5678-abcd-9e0a1b2c3d4e")),
			withCluster(types.StringValue("3f4b6df0-1234-5678-abcd-9e0a1b2c3d4e")),
			settings(nil, str("11111111-2222-3333-4444-555555555555"), nil, nil, nil),
			`{"default_cluster_id":"3f4b6df0-1234-5678-abcd-9e0a1b2c3d4e"}`,
		},
		{
			"an empty cluster is sent as null",
			withCluster(types.StringValue("")),
			withCluster(types.StringValue("")),
			settings(nil, str("11111111-2222-3333-4444-555555555555"), nil, nil, nil),
			`{"default_cluster_id":null}`,
		},
		{
			"an empty cluster is not sent when there is no default",
			withCluster(types.StringValue("")),
			withCluster(types.StringValue("")),
			settings(nil, nil, nil, nil, nil),
			`{}`,
		},
		{
			"changed timeouts are sent",
			withTimeouts(types.Int64Value(60), types.Int64Value(120), types.Int64Value(1440)),
			withTimeouts(types.Int64Value(60), types.Int64Value(120), types.Int64Value(1440)),
			settings(nil, nil, num(30), num(120), num(43200)),
			`{"default_timeout_in_minutes":60,"scheduled_job_expiry_in_minutes":1440}`,
		},
		{
			"a zero timeout is sent",
			withTimeouts(types.Int64Value(0), types.Int64Null(), types.Int64Null()),
			withTimeouts(types.Int64Value(0), types.Int64Null(), types.Int64Null()),
			settings(nil, nil, num(30), nil, nil),
			`{"default_timeout_in_minutes":0}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(pipelineSettingsPatch(&tc.config, &tc.plan, &tc.current))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestClearableStringValue(t *testing.T) {
	t.Parallel()

	bucket := "my-export-bucket"

	testCases := []struct {
		name       string
		configured types.String
		remote     *string
		want       types.String
	}{
		{"a value is read from the API", types.StringNull(), &bucket, types.StringValue(bucket)},
		{"a value replaces what was configured", types.StringValue(""), &bucket, types.StringValue(bucket)},
		{"an unmanaged setting reads back as null", types.StringNull(), nil, types.StringNull()},
		{"a cleared setting keeps its empty string", types.StringValue(""), nil, types.StringValue("")},
		{"a setting cleared elsewhere reads back as null", types.StringValue(bucket), nil, types.StringNull()},
		{"an unknown value reads back as null", types.StringUnknown(), nil, types.StringNull()},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := clearableStringValue(tc.configured, tc.remote); !got.Equal(tc.want) {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestAccBuildkiteOrganizationPipelineSettingsResource(t *testing.T) {
	config := func(settings string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_organization_pipeline_settings" "settings" {
			%s
		}
		`, settings)
	}

	t.Run("manages the pipeline defaults", func(t *testing.T) {
		resource.Test(t, resource.TestCase{
			PreCheck: func() {
				testAccPreCheck(t)
				preserveOrganizationPipelineSettings(t)
			},
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testCheckOrganizationPipelineSettingsRemain,
			Steps: []resource.TestStep{
				{
					// unmanaged: the current values are only read into state
					Config: config(``),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_organization_pipeline_settings.settings", "id", getenv("BUILDKITE_ORGANIZATION_SLUG")),
						resource.TestCheckResourceAttrSet("buildkite_organization_pipeline_settings.settings", "scheduled_job_expiry_in_minutes"),
						resource.TestCheckResourceAttrSet("buildkite_organization_pipeline_settings.settings", "public_pipeline_creation_enabled"),
					),
				},
				{
					Config: config(`
						default_branch                  = "trunk"
						default_timeout_in_minutes      = 61
						maximum_timeout_in_minutes      = 121
						scheduled_job_expiry_in_minutes = 1441
					`),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_organization_pipeline_settings.settings", "default_branch", "trunk"),
						resource.TestCheckResourceAttr("buildkite_organization_pipeline_settings.settings", "default_timeout_in_minutes", "61"),
						testAccCheckOrganizationPipelineSettingsRemoteValues(func(settings *organizationPipelineSettings) error {
							if settings.DefaultBranch == nil || *settings.DefaultBranch != "trunk" {
								return fmt.Errorf("remote default_branch is %v, want trunk", settings.DefaultBranch)
							}
							if settings.MaximumTimeoutInMinutes == nil || *settings.MaximumTimeoutInMinutes != 121 {
								return fmt.Errorf("remote maximum_timeout_in_minutes is %v, want 121", settings.MaximumTimeoutInMinutes)
							}
							if settings.ScheduledJobExpiryInMinutes == nil || *settings.ScheduledJobExpiryInMinutes != 1441 {
								return fmt.Errorf("remote scheduled_job_expiry_in_minutes is %v, want 1441", settings.ScheduledJobExpiryInMinutes)
							}
							return nil
						}),
					),
				},
				{
					ResourceName:      "buildkite_organization_pipeline_settings.settings",
					ImportState:       true,
					ImportStateId:     getenv("BUILDKITE_ORGANIZATION_SLUG"),
					ImportStateVerify: true,
				},
				{
					// removing the attributes leaves the settings as they are
					Config: config(``),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{
							plancheck.ExpectEmptyPlan(),
						},
					},
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_organization_pipeline_settings.settings", "default_branch", "trunk"),
						resource.TestCheckResourceAttr("buildkite_organization_pipeline_settings.settings", "default_timeout_in_minutes", "61"),
					),
				},
			},
		})
	})

	t.Run("manages public pipeline creation", func(t *testing.T) {
		var original *organizationPipelineSettings

		resource.Test(t, resource.TestCase{
			PreCheck: func() {
				testAccPreCheck(t)
				original = preserveOrganizationPipelineSettings(t)
			},
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testCheckOrganizationPipelineSettingsRemain,
			Steps: []resource.TestStep{
				{
					Config: config(`public_pipeline_creation_enabled = false`),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_organization_pipeline_settings.settings", "public_pipeline_creation_enabled", "false"),
						testAccCheckOrganizationPipelineSettingsRemoteValues(func(settings *organizationPipelineSettings) error {
							if settings.PublicPipelineCreation.Enabled {
								return fmt.Errorf("remote public pipeline creation is enabled, want disabled")
							}
							return nil
						}),
					),
				},
				{
					Config: config(`public_pipeline_creation_enabled = true`),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_organization_pipeline_settings.settings", "public_pipeline_creation_enabled", "true"),
						testAccCheckOrganizationPipelineSettingsRemoteValues(func(settings *organizationPipelineSettings) error {
							if !settings.PublicPipelineCreation.Enabled {
								return fmt.Errorf("remote public pipeline creation is disabled, want enabled")
							}
							return nil
						}),
					),
				},
				{
					// removing the attribute leaves the toggle as it is
					Config: config(``),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{
							plancheck.ExpectEmptyPlan(),
						},
					},
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_organization_pipeline_settings.settings", "public_pipeline_creation_enabled", "true"),
						testAccCheckOrganizationPipelineSettingsRemoteValues(func(settings *organizationPipelineSettings) error {
							// the hosted agent toggle was never configured, so it is only read back
							if settings.HostedAgentsTerminalAccess.Enabled != original.HostedAgentsTerminalAccess.Enabled {
								return fmt.Errorf("remote hosted agent remote access changed, want it left alone")
							}
							return nil
						}),
					),
				},
			},
		})
	})

	t.Run("manages build data export", func(t *testing.T) {
		bucket := fmt.Sprintf("tf-acc-%s", getenv("BUILDKITE_ORGANIZATION_SLUG"))

		resource.Test(t, resource.TestCase{
			PreCheck: func() {
				testAccPreCheck(t)
				settings := preserveOrganizationPipelineSettings(t)
				// exporting build data is only writable on plans that include it
				if !settings.BuildExports.Available {
					t.Skip("exporting build data is not available on this organization's plan")
				}
			},
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testCheckOrganizationPipelineSettingsRemain,
			Steps: []resource.TestStep{
				{
					Config: config(fmt.Sprintf(`
						build_export_location    = %q
						build_export_strategy_id = "s3"
					`, bucket)),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_organization_pipeline_settings.settings", "build_export_location", bucket),
						resource.TestCheckResourceAttr("buildkite_organization_pipeline_settings.settings", "build_export_strategy_id", "s3"),
						resource.TestCheckResourceAttr("buildkite_organization_pipeline_settings.settings", "build_export_available", "true"),
						testAccCheckOrganizationPipelineSettingsRemoteValues(func(settings *organizationPipelineSettings) error {
							if !settings.BuildExports.Enabled {
								return fmt.Errorf("remote build data export is disabled, want enabled")
							}
							if settings.BuildExports.Location == nil || *settings.BuildExports.Location != bucket {
								return fmt.Errorf("remote build export location is %v, want %s", settings.BuildExports.Location, bucket)
							}
							return nil
						}),
					),
				},
				{
					// emptying both stops the export, which the API only accepts on its own endpoint
					Config: config(`
						build_export_location    = ""
						build_export_strategy_id = ""
					`),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_organization_pipeline_settings.settings", "build_export_location", ""),
						resource.TestCheckResourceAttr("buildkite_organization_pipeline_settings.settings", "build_export_strategy_id", ""),
						testAccCheckOrganizationPipelineSettingsRemoteValues(func(settings *organizationPipelineSettings) error {
							if settings.BuildExports.Enabled {
								return fmt.Errorf("remote build data export is enabled, want disabled")
							}
							return nil
						}),
					),
				},
			},
		})
	})

}

// preserveOrganizationPipelineSettings restores the organization's pipeline settings once the test
// has finished. They belong to the whole organization rather than to anything the test created, so
// a test that changes them has to put them back.
func preserveOrganizationPipelineSettings(t *testing.T) *organizationPipelineSettings {
	t.Helper()

	client := getTestClient()
	original, err := client.getOrganizationPipelineSettings(context.Background())
	if err != nil {
		t.Skipf("unable to read the organization pipeline settings, which needs the read_organization_settings scope and an organization administrator: %v", err)
	}

	t.Cleanup(func() {
		ctx := context.Background()

		payload := map[string]any{
			"default_branch":             original.DefaultBranch,
			"default_cluster_id":         original.DefaultClusterID,
			"default_timeout_in_minutes": original.DefaultTimeoutInMinutes,
			"maximum_timeout_in_minutes": original.MaximumTimeoutInMinutes,
		}
		// the organization has no way to express an absent expiry, so it is only sent when it was read
		if original.ScheduledJobExpiryInMinutes != nil {
			payload["scheduled_job_expiry_in_minutes"] = original.ScheduledJobExpiryInMinutes
		}
		if _, err := client.updateOrganizationPipelineSettings(ctx, payload); err != nil {
			t.Errorf("unable to restore the organization pipeline settings: %v", err)
		}

		restoreToggle := func(subPath string, enabled bool) {
			method := http.MethodDelete
			if enabled {
				method = http.MethodPut
			}
			if _, err := client.setOrganizationPipelineSetting(ctx, method, subPath, nil); err != nil {
				t.Errorf("unable to restore %s: %v", subPath, err)
			}
		}
		restoreToggle(pipelineSettingsPublicPipelinesPath, original.PublicPipelineCreation.Enabled)
		restoreToggle(pipelineSettingsHostedAgentsSSHPath, original.HostedAgentsTerminalAccess.Enabled)

		if !original.BuildExports.Available {
			return
		}
		if original.BuildExports.Enabled && original.BuildExports.Location != nil && original.BuildExports.StrategyID != nil {
			payload := buildExportRequest{Location: *original.BuildExports.Location, StrategyID: *original.BuildExports.StrategyID}
			if _, err := client.setOrganizationPipelineSetting(ctx, http.MethodPut, pipelineSettingsBuildExportPath, payload); err != nil {
				t.Errorf("unable to restore the build data export: %v", err)
			}
		} else if _, err := client.setOrganizationPipelineSetting(ctx, http.MethodDelete, pipelineSettingsBuildExportPath, nil); err != nil {
			t.Errorf("unable to restore the build data export: %v", err)
		}
	})

	return original
}

func testAccCheckOrganizationPipelineSettingsRemoteValues(check func(*organizationPipelineSettings) error) resource.TestCheckFunc {
	return func(*terraform.State) error {
		settings, err := getTestClient().getOrganizationPipelineSettings(context.Background())
		if err != nil {
			return fmt.Errorf("unable to read the organization pipeline settings: %w", err)
		}
		return check(settings)
	}
}

// testCheckOrganizationPipelineSettingsRemain confirms the destroy left the organization's settings
// where they were, which is the whole of what destroying this resource does.
func testCheckOrganizationPipelineSettingsRemain(*terraform.State) error {
	if _, err := getTestClient().getOrganizationPipelineSettings(context.Background()); err != nil {
		return fmt.Errorf("unable to read the organization pipeline settings after destroy: %w", err)
	}
	return nil
}
