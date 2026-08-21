package buildkite

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// fakePipelineSettingsAPI serves the pipeline settings endpoints against settings held in memory, so
// that the resource's lifecycle can be exercised without an organization to change. What it is
// really checking is that every apply leaves state matching the plan Terraform made for it, which
// depends on how the API answers and so cannot be seen from the resource alone.
type fakePipelineSettingsAPI struct {
	t        *testing.T
	settings organizationPipelineSettings
}

func newFakePipelineSettingsAPI(t *testing.T) *httptest.Server {
	t.Helper()

	api := &fakePipelineSettingsAPI{t: t}
	api.settings.DefaultBranch = ptr("main")
	api.settings.ScheduledJobExpiryInMinutes = ptr(int64(43200))
	api.settings.PublicPipelineCreation.Enabled = true
	api.settings.HostedAgentsTerminalAccess.Enabled = true
	api.settings.BuildExports.Available = true
	api.settings.BuildExports.SupportedStrategies = []string{"s3", "gcs"}

	server := httptest.NewServer(api)
	t.Cleanup(server.Close)

	return server
}

func (a *fakePipelineSettingsAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const base = "/v2/organizations/acme/pipeline-settings"

	switch {
	case r.URL.Path == base && r.Method == http.MethodGet:

	case r.URL.Path == base && r.Method == http.MethodPatch:
		a.patch(w, r)

	case r.URL.Path == base+"/public-pipelines":
		a.settings.PublicPipelineCreation.Enabled = r.Method == http.MethodPut

	case r.URL.Path == base+"/hosted-agents-ssh":
		a.settings.HostedAgentsTerminalAccess.Enabled = r.Method == http.MethodPut

	case r.URL.Path == base+"/build-export":
		if !a.buildExport(w, r) {
			return
		}

	default:
		a.t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
		return
	}

	if err := json.NewEncoder(w).Encode(a.settings); err != nil {
		a.t.Errorf("unable to write the response: %v", err)
	}
}

func (a *fakePipelineSettingsAPI) patch(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		a.t.Errorf("unable to read the request: %v", err)
		return
	}

	// Only the keys the request carries change, which is what lets the resource leave the settings
	// it does not manage alone.
	for key, value := range body {
		switch key {
		case "default_branch":
			a.settings.DefaultBranch = optionalStringFromJSON(value)
		case "default_cluster_id":
			a.settings.DefaultClusterID = optionalStringFromJSON(value)
		case "default_timeout_in_minutes":
			a.settings.DefaultTimeoutInMinutes = optionalInt64FromJSON(value)
		case "maximum_timeout_in_minutes":
			a.settings.MaximumTimeoutInMinutes = optionalInt64FromJSON(value)
		case "scheduled_job_expiry_in_minutes":
			a.settings.ScheduledJobExpiryInMinutes = optionalInt64FromJSON(value)
		default:
			a.t.Errorf("unexpected key in the request: %s", key)
		}
	}
}

// buildExport mirrors the API's refusal to clear the export through its own endpoint, which is what
// makes an empty location a delete rather than a write.
func (a *fakePipelineSettingsAPI) buildExport(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodDelete {
		a.settings.BuildExports.Enabled = false
		a.settings.BuildExports.Location = nil
		a.settings.BuildExports.StrategyID = nil
		return true
	}

	var body buildExportRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		a.t.Errorf("unable to read the request: %v", err)
		return false
	}
	if body.Location == "" || body.StrategyID == "" {
		http.Error(w, `{"message":"location is required"}`, http.StatusUnprocessableEntity)
		return false
	}

	a.settings.BuildExports.Enabled = true
	a.settings.BuildExports.Location = &body.Location
	a.settings.BuildExports.StrategyID = &body.StrategyID
	return true
}

func TestAccBuildkiteOrganizationPipelineSettingsResourceAgainstFakeAPI(t *testing.T) {
	server := newFakePipelineSettingsAPI(t)

	config := func(settings string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			organization = "acme"
			api_token    = "fake"
			rest_url     = %q
		}

		resource "buildkite_organization_pipeline_settings" "settings" {
			%s
		}
		`, server.URL, settings)
	}

	const name = "buildkite_organization_pipeline_settings.settings"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				// adopting the organization's settings changes none of them
				Config: config(``),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "id", "acme"),
					resource.TestCheckResourceAttr(name, "default_branch", "main"),
					resource.TestCheckResourceAttr(name, "scheduled_job_expiry_in_minutes", "43200"),
					resource.TestCheckResourceAttr(name, "public_pipeline_creation_enabled", "true"),
					resource.TestCheckNoResourceAttr(name, "default_cluster_id"),
					resource.TestCheckNoResourceAttr(name, "build_export_location"),
					resource.TestCheckResourceAttr(name, "build_export_available", "true"),
					resource.TestCheckResourceAttr(name, "build_export_supported_strategies.0", "s3"),
				),
			},
			{
				Config: config(`
					default_branch                        = "trunk"
					default_cluster_id                    = "3f4b6df0-1234-5678-abcd-9e0a1b2c3d4e"
					default_timeout_in_minutes            = 60
					maximum_timeout_in_minutes            = 120
					scheduled_job_expiry_in_minutes       = 1440
					public_pipeline_creation_enabled      = false
					hosted_agents_terminal_access_enabled = false
				`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "default_branch", "trunk"),
					resource.TestCheckResourceAttr(name, "default_cluster_id", "3f4b6df0-1234-5678-abcd-9e0a1b2c3d4e"),
					resource.TestCheckResourceAttr(name, "maximum_timeout_in_minutes", "120"),
					resource.TestCheckResourceAttr(name, "scheduled_job_expiry_in_minutes", "1440"),
					resource.TestCheckResourceAttr(name, "public_pipeline_creation_enabled", "false"),
					resource.TestCheckResourceAttr(name, "hosted_agents_terminal_access_enabled", "false"),
				),
			},
			{
				// an empty cluster leaves new pipelines without a default one
				Config: config(`default_cluster_id = ""`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "default_cluster_id", ""),
					// the settings dropped from the configuration are left where they were
					resource.TestCheckResourceAttr(name, "default_branch", "trunk"),
					resource.TestCheckResourceAttr(name, "public_pipeline_creation_enabled", "false"),
				),
			},
			{
				Config: config(`
					build_export_location    = "my-export-bucket"
					build_export_strategy_id = "s3"
				`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "build_export_location", "my-export-bucket"),
					resource.TestCheckResourceAttr(name, "build_export_strategy_id", "s3"),
				),
			},
			{
				ResourceName:      name,
				ImportState:       true,
				ImportStateId:     "acme",
				ImportStateVerify: true,
				// An import has no configuration to read, so a setting cleared with an empty string
				// is indistinguishable from one that was never set and arrives as null.
				ImportStateVerifyIgnore: []string{"default_cluster_id"},
			},
			{
				// emptying both stops the export
				Config: config(`
					build_export_location    = ""
					build_export_strategy_id = ""
				`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "build_export_location", ""),
					resource.TestCheckResourceAttr(name, "build_export_strategy_id", ""),
				),
			},
			{
				// destroying leaves every setting as it is
				Config:  config(``),
				Destroy: true,
			},
		},
	})

	rejects := func(t *testing.T, settings, message string) {
		t.Helper()

		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config:      config(settings),
					ExpectError: regexp.MustCompile(message),
				},
			},
		})
	}

	t.Run("rejects a build export location without a strategy", func(t *testing.T) {
		rejects(t, `build_export_location = "my-export-bucket"`, "Invalid Attribute Combination")
	})

	t.Run("rejects a build export location carrying a scheme", func(t *testing.T) {
		rejects(t, `
			build_export_location    = "s3://my-export-bucket"
			build_export_strategy_id = "s3"
		`, "must be a bare bucket name")
	})

	t.Run("rejects a build export half cleared", func(t *testing.T) {
		rejects(t, `
			build_export_location    = ""
			build_export_strategy_id = "s3"
		`, "Mismatched build export location and strategy")
	})

	t.Run("rejects a scheduled job expiry out of range", func(t *testing.T) {
		rejects(t, `scheduled_job_expiry_in_minutes = 59`, "Invalid Attribute Value")
	})
}

func ptr[T any](v T) *T { return &v }

func optionalStringFromJSON(value any) *string {
	s, ok := value.(string)
	if !ok {
		return nil
	}
	return &s
}

func optionalInt64FromJSON(value any) *int64 {
	f, ok := value.(float64)
	if !ok {
		return nil
	}
	i := int64(f)
	return &i
}
