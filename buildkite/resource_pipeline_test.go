package buildkite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"maps"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func testAccCheckPipelineDestroy(s *terraform.State) error {
	orgSlug := os.Getenv("BUILDKITE_ORGANIZATION_SLUG")
	if orgSlug == "" {
		return fmt.Errorf("BUILDKITE_ORGANIZATION_SLUG environment variable is not set")
	}

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_pipeline" {
			continue
		}

		log.Printf("[DEBUG] Checking pipeline resource: %s (ID: %s)", rs.Primary.Attributes["name"], rs.Primary.ID)

		pipelineSlug := rs.Primary.Attributes["slug"]
		if pipelineSlug == "" {
			pipelineName := rs.Primary.Attributes["name"]
			if pipelineName == "" {
				log.Printf("[WARN] Pipeline resource has no name, skipping")
				continue
			}
			pipelineSlug = fmt.Sprintf("%s/%s", orgSlug, strings.ToLower(pipelineName))
		} else if !strings.Contains(pipelineSlug, "/") {
			// If the slug doesn't contain a '/', prepend the organization slug
			pipelineSlug = fmt.Sprintf("%s/%s", orgSlug, pipelineSlug)
		}

		log.Printf("[DEBUG] Checking pipeline with slug: %s", pipelineSlug)
		resp, err := getPipeline(context.Background(), genqlientGraphql, pipelineSlug)
		if err != nil {
			if strings.Contains(err.Error(), "not found") ||
				strings.Contains(err.Error(), "pipeline not found") {
				log.Printf("[DEBUG] Pipeline not found (expected): %s", pipelineSlug)
				continue
			}
			log.Printf("[ERROR] Error checking pipeline %s: %v", pipelineSlug, err)
			return fmt.Errorf("error checking if pipeline exists: %v", err)
		}

		if resp.Pipeline.Id != "" {
			log.Printf("[ERROR] Pipeline still exists: %s (ID: %s)", pipelineSlug, resp.Pipeline.Id)
			return fmt.Errorf("pipeline still exists: %s", pipelineSlug)
		}
	}

	return nil
}

func TestPipelineExtraSettingsUseStepKeyAsCommitStatusJSON(t *testing.T) {
	enabled := true

	payload, err := json.Marshal(PipelineExtraSettings{
		UseStepKeyAsCommitStatus: &enabled,
	})
	if err != nil {
		t.Fatalf("failed to marshal provider settings: %v", err)
	}

	if !strings.Contains(string(payload), `"use_step_key_as_commit_status":true`) {
		t.Fatalf("expected use_step_key_as_commit_status in payload, got %s", payload)
	}

	payload, err = json.Marshal(PipelineExtraSettings{})
	if err != nil {
		t.Fatalf("failed to marshal empty provider settings: %v", err)
	}

	if strings.Contains(string(payload), "use_step_key_as_commit_status") {
		t.Fatalf("expected use_step_key_as_commit_status to be omitted from empty payload, got %s", payload)
	}
}

func TestPipelineExtraSettingsBuildIssuesJSON(t *testing.T) {
	enabled := true

	payload, err := json.Marshal(PipelineExtraSettings{BuildIssues: &enabled})
	if err != nil {
		t.Fatalf("failed to marshal provider settings: %v", err)
	}

	if !strings.Contains(string(payload), `"build_issues":true`) {
		t.Fatalf("expected build_issues in payload, got %s", payload)
	}
}

func TestUpdatePipelineResourceExtraInfoUseStepKeyAsCommitStatus(t *testing.T) {
	enabled := true
	extraInfo := PipelineExtraInfo{}
	extraInfo.Provider.Settings.UseStepKeyAsCommitStatus = &enabled

	state := pipelineResourceModel{}
	updatePipelineResourceExtraInfo(&state, &extraInfo)

	if state.ProviderSettings == nil {
		t.Fatal("expected provider settings to be set")
	}

	if !state.ProviderSettings.UseStepKeyAsCommitStatus.ValueBool() {
		t.Fatal("expected use_step_key_as_commit_status to be true")
	}
}

func TestUpdatePipelineResourceExtraInfoBuildIssues(t *testing.T) {
	disabled := false
	extraInfo := PipelineExtraInfo{}
	extraInfo.Provider.Settings.BuildIssues = &disabled

	state := pipelineResourceModel{}
	updatePipelineResourceExtraInfo(&state, &extraInfo)

	if state.ProviderSettings == nil {
		t.Fatal("expected provider settings to be set")
	}
	if state.ProviderSettings.BuildIssues.IsNull() || state.ProviderSettings.BuildIssues.ValueBool() {
		t.Fatal("expected build_issues to be false")
	}
}

func TestMapProviderSettingsFromGraphQLGitHub(t *testing.T) {
	triggerMode := "code"
	enabled := true
	disabled := false
	matchMode := CommandWordMatchModeExact

	repo := RepositoryProviderSettingsFields{
		Provider: &RepositoryProviderSettingsFieldsProviderRepositoryProviderGithub{
			Settings: RepositoryProviderSettingsFieldsProviderRepositoryProviderGithubSettingsRepositoryProviderGitHubSettings{
				TriggerMode:                          &triggerMode,
				BuildIssues:                          &enabled,
				BuildPullRequests:                    &enabled,
				BuildBranches:                        &disabled,
				IssueCommentMatchMode:                &matchMode,
				BuildPullRequestReviewCommentCreated: &enabled,
				ReviewCommentMatchMode:               &matchMode,
				BuildPullRequestDequeued:             &enabled,
				BuildPullRequestReopened:             &enabled,
				UseStepKeyAsCommitStatus:             &enabled,
			},
		},
	}

	got := mapProviderSettingsFromGraphQL(repo)
	if got == nil {
		t.Fatal("expected provider settings to be mapped, got nil")
	}
	if got.TriggerMode.ValueString() != "code" {
		t.Fatalf("trigger_mode: expected \"code\", got %q", got.TriggerMode.ValueString())
	}
	if !got.BuildPullRequests.ValueBool() {
		t.Fatal("build_pull_requests: expected true")
	}
	if !got.BuildIssues.ValueBool() {
		t.Fatal("build_issues: expected true")
	}
	if got.BuildBranches.ValueBool() {
		t.Fatal("build_branches: expected false")
	}
	// CommandWordMatchMode enum (EXACT) must be lowercased to match the schema validator.
	if got.IssueCommentMatchMode.ValueString() != "exact" {
		t.Fatalf("issue_comment_match_mode: expected \"exact\", got %q", got.IssueCommentMatchMode.ValueString())
	}
	if got.ReviewCommentMatchMode.ValueString() != "exact" {
		t.Fatalf("review_comment_match_mode: expected \"exact\", got %q", got.ReviewCommentMatchMode.ValueString())
	}
	if !got.BuildPullRequestReviewCommentCreated.ValueBool() {
		t.Fatal("build_pull_request_review_comment_created: expected true")
	}
	if !got.BuildPullRequestDequeued.ValueBool() {
		t.Fatal("build_pull_request_dequeued: expected true")
	}
	if !got.BuildPullRequestReopened.ValueBool() {
		t.Fatal("build_pull_request_reopened: expected true")
	}
	// use_step_key_as_commit_status is now exposed via GraphQL and mapped directly.
	if !got.UseStepKeyAsCommitStatus.ValueBool() {
		t.Fatal("use_step_key_as_commit_status: expected true")
	}
}

// GitLab Enterprise (and Community) are distinct RepositoryProvider union members that expose the
// same RepositoryProviderGitlabSettings as plain GitLab. Regression guard: they must be mapped, not
// fall through to the nil default (which would skip provider_settings refresh and miss drift).
func TestMapProviderSettingsFromGraphQLGitlabEnterprise(t *testing.T) {
	cond := "build.branch == 'main'"
	enabled := true

	repo := RepositoryProviderSettingsFields{
		Provider: &RepositoryProviderSettingsFieldsProviderRepositoryProviderGitlabEnterprise{
			Settings: RepositoryProviderSettingsFieldsProviderRepositoryProviderGitlabEnterpriseSettingsRepositoryProviderGitlabSettings{
				FilterCondition: &cond,
				FilterEnabled:   &enabled,
			},
		},
	}

	got := mapProviderSettingsFromGraphQL(repo)
	if got == nil {
		t.Fatal("expected GitLab Enterprise provider settings to be mapped, got nil")
	}
	if got.FilterCondition.ValueString() != cond {
		t.Fatalf("filter_condition: expected %q, got %q", cond, got.FilterCondition.ValueString())
	}
	if !got.FilterEnabled.ValueBool() {
		t.Fatal("filter_enabled: expected true")
	}
}

func TestMapProviderSettingsFromGraphQLCursorOrigin(t *testing.T) {
	cond := `build.branch == "main"`
	enabled := true
	disabled := false

	repo := RepositoryProviderSettingsFields{
		Provider: &RepositoryProviderSettingsFieldsProviderRepositoryProviderCursorOrigin{
			Settings: RepositoryProviderSettingsFieldsProviderRepositoryProviderCursorOriginSettings{
				BuildBranches:       &enabled,
				BuildPullRequests:   &enabled,
				BuildTags:           &disabled,
				FilterCondition:     &cond,
				FilterEnabled:       &enabled,
				PublishCommitStatus: &disabled,
			},
		},
	}

	got := mapProviderSettingsFromGraphQL(repo)
	if got == nil {
		t.Fatal("expected Cursor Origin provider settings to be mapped, got nil")
	}
	if !got.BuildBranches.ValueBool() {
		t.Fatal("build_branches: expected true")
	}
	if !got.BuildPullRequests.ValueBool() {
		t.Fatal("build_pull_requests: expected true")
	}
	if got.BuildTags.IsNull() || got.BuildTags.ValueBool() {
		t.Fatal("build_tags: expected false, got null or true")
	}
	if got.FilterCondition.ValueString() != cond {
		t.Fatalf("filter_condition: expected %q, got %q", cond, got.FilterCondition.ValueString())
	}
	if !got.FilterEnabled.ValueBool() {
		t.Fatal("filter_enabled: expected true")
	}
	if got.PublishCommitStatus.IsNull() || got.PublishCommitStatus.ValueBool() {
		t.Fatal("publish_commit_status: expected false, got null or true")
	}
}

func testAccCheckPipelineDestroyFunc(s *terraform.State) error {
	return testAccCheckPipelineDestroy(s)
}

func TestAccBuildkitePipelineResource(t *testing.T) {
	compareRemoteValue := func(prop func() any, value any) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			if v := prop(); v != value {
				return fmt.Errorf("expected (%v) does not match actual (%v)", value, v)
			}
			return nil
		}
	}
	aggregateRemoteCheck := func(pipeline *getPipelinePipeline) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			var err error
			p := s.RootModule().Resources["buildkite_pipeline.pipeline"]

			err = errors.Join(compareRemoteValue(func() any { return pipeline.Name }, p.Primary.Attributes["name"])(s), err)
			err = errors.Join(compareRemoteValue(func() any { return pipeline.Steps.Yaml }, defaultSteps)(s), err)
			err = errors.Join(compareRemoteValue(func() any { return pipeline.Repository.Url }, "https://github.com/buildkite/terraform-provider-buildkite.git")(s), err)
			err = errors.Join(compareRemoteValue(func() any { return pipeline.AllowRebuilds }, true)(s), err)
			err = errors.Join(compareRemoteValue(func() any { return *pipeline.DefaultTimeoutInMinutes }, 0)(s), err)
			err = errors.Join(compareRemoteValue(func() any { return *pipeline.MaximumTimeoutInMinutes }, 0)(s), err)
			err = errors.Join(compareRemoteValue(func() any { return pipeline.BranchConfiguration }, (*string)(nil))(s), err)
			err = errors.Join(compareRemoteValue(func() any { return pipeline.Cluster.Id }, (*string)(nil))(s), err)
			err = errors.Join(compareRemoteValue(func() any { return string(pipeline.Visibility) }, "PRIVATE")(s), err)

			return err
		}
	}

	aggregateRemoteCheckWithTemplateSteps := func(pipeline *getPipelinePipeline) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			var err error
			p := s.RootModule().Resources["buildkite_pipeline.pipeline"]

			err = errors.Join(compareRemoteValue(func() any { return pipeline.Name }, p.Primary.Attributes["name"])(s), err)
			err = errors.Join(compareRemoteValue(func() any { return pipeline.Repository.Url }, "https://github.com/buildkite/terraform-provider-buildkite.git")(s), err)
			err = errors.Join(compareRemoteValue(func() any { return pipeline.AllowRebuilds }, true)(s), err)
			err = errors.Join(compareRemoteValue(func() any { return *pipeline.DefaultTimeoutInMinutes }, 0)(s), err)
			err = errors.Join(compareRemoteValue(func() any { return *pipeline.MaximumTimeoutInMinutes }, 0)(s), err)
			err = errors.Join(compareRemoteValue(func() any { return pipeline.BranchConfiguration }, (*string)(nil))(s), err)
			err = errors.Join(compareRemoteValue(func() any { return pipeline.Cluster.Id }, (*string)(nil))(s), err)

			return err
		}
	}

	t.Run("create pipeline with only required attributes", func(t *testing.T) {
		var pipeline getPipelinePipeline
		pipelineName := acctest.RandString(12)
		config := fmt.Sprintf(`
			resource "buildkite_pipeline" "pipeline" {
				name = "%s"
				repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
			}
		`, pipelineName)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineDestroyFunc,
			Steps: []resource.TestStep{
				{
					Config: config,
					Check: resource.ComposeAggregateTestCheckFunc(
						// check computed values get set
						resource.TestCheckResourceAttrSet("buildkite_pipeline.pipeline", "badge_url"),
						resource.TestCheckResourceAttrSet("buildkite_pipeline.pipeline", "id"),
						resource.TestCheckResourceAttrSet("buildkite_pipeline.pipeline", "uuid"),
						resource.TestCheckResourceAttrSet("buildkite_pipeline.pipeline", "webhook_url"),
						// check api values are expected
						func(s *terraform.State) error {
							slug := fmt.Sprintf("%s/%s", getenv("BUILDKITE_ORGANIZATION_SLUG"), pipelineName)
							resp, err := getPipeline(context.Background(), genqlientGraphql, slug)
							pipeline = resp.Pipeline
							return err
						},
						aggregateRemoteCheck(&pipeline),
						// check state values are correct
						resource.TestCheckNoResourceAttr("buildkite_pipeline.pipeline", "branch_configuration"),
						resource.TestCheckNoResourceAttr("buildkite_pipeline.pipeline", "cluster_id"),
						resource.TestCheckNoResourceAttr("buildkite_pipeline.pipeline", "cluster_name"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "allow_rebuilds", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "cancel_intermediate_builds", "false"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "cancel_intermediate_builds_branch_filter", ""),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "default_branch", ""),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "default_timeout_in_minutes", "0"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "description", ""),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "maximum_timeout_in_minutes", "0"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "name", pipelineName),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "repository", "https://github.com/buildkite/terraform-provider-buildkite.git"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "skip_intermediate_builds", "false"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "skip_intermediate_builds_branch_filter", ""),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "slug", fmt.Sprint(strings.ToLower(pipelineName))),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "steps", defaultSteps),
						// check lists are empty
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "tags.#", "0"),
						resource.TestCheckNoResourceAttr("buildkite_pipeline.pipeline", "tags.#"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.#", "0"),
						resource.TestCheckNoResourceAttr("buildkite_pipeline.pipeline", "provider_settings.#"),
					),
				},
				{
					ResourceName:  "buildkite_pipeline.pipeline",
					ImportState:   true,
					ImportStateId: pipeline.Id,
				},
			},
		})
	})

	t.Run("update pipeline with only required attributes", func(t *testing.T) {
		var pipeline getPipelinePipeline
		pipelineName := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineDestroyFunc,
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(`
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
						}
					`, pipelineName),
					Check: resource.ComposeAggregateTestCheckFunc(
						// check api values are expected
						func(s *terraform.State) error {
							slug := fmt.Sprintf("%s/%s", getenv("BUILDKITE_ORGANIZATION_SLUG"), pipelineName)
							resp, err := getPipeline(context.Background(), genqlientGraphql, slug)
							pipeline = resp.Pipeline
							return err
						},
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "steps", defaultSteps),
					),
				},
				{
					Config: fmt.Sprintf(`
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							repository = "https://github.com/buildkite/terraform-provider.git"
						}
					`, pipelineName),
					Check: resource.ComposeAggregateTestCheckFunc(
						// check the pipeline IDs are the same (so it wasn't recreated)
						func(s *terraform.State) error {
							p := s.RootModule().Resources["buildkite_pipeline.pipeline"]
							if p.Primary.ID != pipeline.Id {
								return fmt.Errorf("Pipelines do not match: %s %s", pipeline.Id, p.Primary.ID)
							}
							return nil
						},
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "steps", defaultSteps),
					),
				},
			},
		})
	})

	t.Run("create pipeline with user defined slug", func(t *testing.T) {
		var pipeline getPipelinePipeline
		pipelineName := acctest.RandString(12)
		slugName := strings.ToLower(acctest.RandString(12))
		config := fmt.Sprintf(`
			resource "buildkite_pipeline" "pipeline" {
				name = "%s"
				slug = "%s"
				repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
			}
		`, pipelineName, slugName)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineDestroyFunc,
			Steps: []resource.TestStep{
				{
					Config: config,
					Check: resource.ComposeAggregateTestCheckFunc(
						// check computed values get set
						resource.TestCheckResourceAttrSet("buildkite_pipeline.pipeline", "badge_url"),
						resource.TestCheckResourceAttrSet("buildkite_pipeline.pipeline", "id"),
						resource.TestCheckResourceAttrSet("buildkite_pipeline.pipeline", "uuid"),
						resource.TestCheckResourceAttrSet("buildkite_pipeline.pipeline", "webhook_url"),
						// check api values are expected
						func(s *terraform.State) error {
							slug := fmt.Sprintf("%s/%s", getenv("BUILDKITE_ORGANIZATION_SLUG"), pipelineName)
							resp, err := getPipeline(context.Background(), genqlientGraphql, slug)
							pipeline = resp.Pipeline
							return err
						},
						aggregateRemoteCheck(&pipeline),
						// check state values are correct
						resource.TestCheckNoResourceAttr("buildkite_pipeline.pipeline", "branch_configuration"),
						resource.TestCheckNoResourceAttr("buildkite_pipeline.pipeline", "cluster_id"),
						resource.TestCheckNoResourceAttr("buildkite_pipeline.pipeline", "cluster_name"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "allow_rebuilds", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "cancel_intermediate_builds", "false"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "cancel_intermediate_builds_branch_filter", ""),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "default_branch", ""),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "default_timeout_in_minutes", "0"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "description", ""),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "maximum_timeout_in_minutes", "0"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "name", pipelineName),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "repository", "https://github.com/buildkite/terraform-provider-buildkite.git"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "skip_intermediate_builds", "false"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "skip_intermediate_builds_branch_filter", ""),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "slug", slugName),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "steps", defaultSteps),
						// check lists are empty
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "tags.#", "0"),
						resource.TestCheckNoResourceAttr("buildkite_pipeline.pipeline", "tags.#"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.#", "0"),
						resource.TestCheckNoResourceAttr("buildkite_pipeline.pipeline", "provider_settings.#"),
					),
				},
			},
		})
	})

	t.Run("update pipeline with user defined slug", func(t *testing.T) {
		var pipeline getPipelinePipeline
		pipelineName := acctest.RandString(12)
		slugName := strings.ToLower(acctest.RandString(12))
		updatedSlugName := strings.ToLower(acctest.RandString(12))

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineDestroyFunc,
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(`
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							slug = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
						}
					`, pipelineName, slugName),
					Check: resource.ComposeAggregateTestCheckFunc(
						// check api values are expected
						func(s *terraform.State) error {
							slug := fmt.Sprintf("%s/%s", getenv("BUILDKITE_ORGANIZATION_SLUG"), pipelineName)
							resp, err := getPipeline(context.Background(), genqlientGraphql, slug)
							pipeline = resp.Pipeline
							return err
						},
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "slug", slugName),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "steps", defaultSteps),
					),
				},
				{
					Config: fmt.Sprintf(`
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							slug = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
						}
					`, pipelineName, updatedSlugName),
					Check: resource.ComposeAggregateTestCheckFunc(
						// check the pipeline IDs are the same (so it wasn't recreated)
						func(s *terraform.State) error {
							p := s.RootModule().Resources["buildkite_pipeline.pipeline"]
							if p.Primary.ID != pipeline.Id {
								return fmt.Errorf("Pipelines do not match: %s %s", pipeline.Id, p.Primary.ID)
							}
							return nil
						},
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "slug", updatedSlugName),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "steps", defaultSteps),
					),
				},
			},
		})
	})

	t.Run("set user defined slug for existing pipeline", func(t *testing.T) {
		var pipeline getPipelinePipeline
		pipelineName := acctest.RandString(12)
		slugName := strings.ToLower(acctest.RandString(12))

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineDestroyFunc,
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(`
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
						}
					`, pipelineName),
					Check: resource.ComposeAggregateTestCheckFunc(
						// check api values are expected
						func(s *terraform.State) error {
							slug := fmt.Sprintf("%s/%s", getenv("BUILDKITE_ORGANIZATION_SLUG"), pipelineName)
							resp, err := getPipeline(context.Background(), genqlientGraphql, slug)
							pipeline = resp.Pipeline
							return err
						},
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "steps", defaultSteps),
					),
				},
				{
					Config: fmt.Sprintf(`
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							slug = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
						}
					`, pipelineName, slugName),
					Check: resource.ComposeAggregateTestCheckFunc(
						// check the pipeline IDs are the same (so it wasn't recreated)
						func(s *terraform.State) error {
							p := s.RootModule().Resources["buildkite_pipeline.pipeline"]
							if p.Primary.ID != pipeline.Id {
								return fmt.Errorf("Pipelines do not match: %s %s", pipeline.Id, p.Primary.ID)
							}
							return nil
						},
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "slug", slugName),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "steps", defaultSteps),
					),
				},
			},
		})
	})

	t.Run("remove user defined slug from existing pipeline", func(t *testing.T) {
		var pipeline getPipelinePipeline
		pipelineId := acctest.RandString(12)
		pipelineName := fmt.Sprintf("TesT --- PipeLine - %s", pipelineId)
		slugName := strings.ToLower(acctest.RandString(12))

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineDestroyFunc,
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(`
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							slug = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
						}
					`, pipelineName, slugName),
					Check: resource.ComposeAggregateTestCheckFunc(
						// check api values are expected
						func(s *terraform.State) error {
							slug := fmt.Sprintf("%s/%s", getenv("BUILDKITE_ORGANIZATION_SLUG"), slugName)
							resp, err := getPipeline(context.Background(), genqlientGraphql, slug)
							pipeline = resp.Pipeline
							return err
						},
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "name", pipelineName),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "slug", slugName),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "steps", defaultSteps),
					),
				},
				{
					Config: fmt.Sprintf(`
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
						}
					`, pipelineName),
					Check: resource.ComposeAggregateTestCheckFunc(
						// check the pipeline IDs are the same (so it wasn't recreated)
						func(s *terraform.State) error {
							p := s.RootModule().Resources["buildkite_pipeline.pipeline"]
							if p.Primary.ID != pipeline.Id {
								return fmt.Errorf("Pipelines do not match: %s %s", pipeline.Id, p.Primary.ID)
							}
							return nil
						},
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "name", pipelineName),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "slug", fmt.Sprintf("test-pipeline-%s", strings.ToLower(pipelineId))),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "steps", defaultSteps),
					),
				},
			},
		})
	})

	t.Run("create pipeline with a pipeline template", func(t *testing.T) {
		var pipeline getPipelinePipeline
		pipelineName := acctest.RandString(12)
		templateName := acctest.RandString(12)
		config := fmt.Sprintf(`
			resource "buildkite_pipeline_template" "template_foo" {
				name = "Template %s"
				configuration = "steps:\n  - label: \":pipeline:\"\n    command: \"buildkite-agent pipeline upload .buildkite/dev.yaml\""
				available = true
			}

			resource "buildkite_pipeline" "pipeline" {
				depends_on = [buildkite_pipeline_template.template_foo]
				name = "%s"
				repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
				pipeline_template_id = buildkite_pipeline_template.template_foo.id
			}
		`, templateName, pipelineName)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineDestroyFunc,
			Steps: []resource.TestStep{
				{
					Config: config,
					Check: resource.ComposeAggregateTestCheckFunc(
						// check computed values get set
						resource.TestCheckResourceAttrSet("buildkite_pipeline.pipeline", "badge_url"),
						resource.TestCheckResourceAttrSet("buildkite_pipeline.pipeline", "id"),
						resource.TestCheckNoResourceAttr("buildkite_pipeline.pipeline", "steps"),
						resource.TestCheckResourceAttrSet("buildkite_pipeline.pipeline", "uuid"),
						resource.TestCheckResourceAttrSet("buildkite_pipeline.pipeline", "webhook_url"),
						resource.TestCheckResourceAttrSet("buildkite_pipeline.pipeline", "pipeline_template_id"),

						// check api values are expected
						func(s *terraform.State) error {
							slug := fmt.Sprintf("%s/%s", getenv("BUILDKITE_ORGANIZATION_SLUG"), pipelineName)
							resp, err := getPipeline(context.Background(), genqlientGraphql, slug)
							pipeline = resp.Pipeline
							return err
						},
						aggregateRemoteCheckWithTemplateSteps(&pipeline),
						// check state values are correct
						resource.TestCheckNoResourceAttr("buildkite_pipeline.pipeline", "branch_configuration"),
						resource.TestCheckNoResourceAttr("buildkite_pipeline.pipeline", "cluster_id"),
						resource.TestCheckNoResourceAttr("buildkite_pipeline.pipeline", "cluster_name"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "allow_rebuilds", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "cancel_intermediate_builds", "false"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "cancel_intermediate_builds_branch_filter", ""),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "default_branch", ""),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "default_timeout_in_minutes", "0"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "description", ""),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "maximum_timeout_in_minutes", "0"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "name", pipelineName),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "repository", "https://github.com/buildkite/terraform-provider-buildkite.git"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "skip_intermediate_builds", "false"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "skip_intermediate_builds_branch_filter", ""),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "slug", fmt.Sprint(strings.ToLower(pipelineName))),

						// check lists are empty
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "tags.#", "0"),
						resource.TestCheckNoResourceAttr("buildkite_pipeline.pipeline", "tags.#"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.#", "0"),
						resource.TestCheckNoResourceAttr("buildkite_pipeline.pipeline", "provider_settings.#"),
					),
				},
			},
		})
	})

	t.Run("create pipeline with empty attributes", func(t *testing.T) {
		var pipeline *getPipelinePipeline
		pipelineName := acctest.RandString(12)
		config := fmt.Sprintf(`
			resource "buildkite_pipeline" "pipeline" {
				name = "%s"
				repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
				tags = []
				provider_settings = {}
			}
		`, pipelineName)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineDestroyFunc,
			Steps: []resource.TestStep{
				{
					Config: config,
					Check: resource.ComposeAggregateTestCheckFunc(
						// check api values are expected
						func(s *terraform.State) error {
							slug := fmt.Sprintf("%s/%s", getenv("BUILDKITE_ORGANIZATION_SLUG"), pipelineName)
							resp, err := getPipeline(context.Background(), genqlientGraphql, slug)
							pipeline = &resp.Pipeline
							return err
						},
						// tags on the remote should be empty
						func(s *terraform.State) error {
							if len(pipeline.Tags) != 0 {
								return fmt.Errorf("Remote tags are not empty")
							}
							return nil
						},
						// check lists are empty
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "tags.#", "0"),
						resource.TestCheckNoResourceAttr("buildkite_pipeline.pipeline", "tags.#"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.trigger_mode", ""),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_issues", "false"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_pull_requests", "false"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.skip_pull_request_builds_for_existing_commits", "false"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_branches", "false"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.publish_commit_status", "false"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.use_step_key_as_commit_status", "false"),
					),
				},
			},
		})
	})

	t.Run("manages github webhooks", func(t *testing.T) {
		pipelineName := acctest.RandString(12)
		clusterName := acctest.RandString(12)
		config := func(enabled bool) string {
			return fmt.Sprintf(`
				resource "buildkite_cluster" "cluster" {
					name = "%s"
				}
				resource "buildkite_pipeline" "pipeline" {
					name = "%s"
					repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
					cluster_id = buildkite_cluster.cluster.id
					github_webhooks_enabled = %t
				}
			`, clusterName, pipelineName, enabled)
		}
		// Confirm the webhooks are enabled or disabled in Buildkite's system
		checkRemote := func(enabled bool) resource.TestCheckFunc {
			return func(s *terraform.State) error {
				var webhooks struct {
					Enabled bool `json:"enabled"`
				}
				path := fmt.Sprintf("/v2/organizations/%s/pipelines/%s/github-webhooks", getenv("BUILDKITE_ORGANIZATION_SLUG"), pipelineName)
				if err := getTestClient().makeRequest(context.Background(), "GET", path, nil, &webhooks); err != nil {
					return err
				}
				if webhooks.Enabled != enabled {
					return fmt.Errorf("Remote github webhooks enabled does not match. Expected: %t, got: %t", enabled, webhooks.Enabled)
				}
				return nil
			}
		}

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineDestroyFunc,
			Steps: []resource.TestStep{
				{
					Config: config(false),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "github_webhooks_enabled", "false"),
						checkRemote(false),
					),
				},
				{
					Config: config(true),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{
							plancheck.ExpectEmptyPlan(),
						},
					},
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "github_webhooks_enabled", "true"),
						checkRemote(true),
					),
				},
				{
					// the setting is only read when it is managed, so it is not part of the imported state
					// (provider_settings is the opposite: import always reads it)
					ResourceName:            "buildkite_pipeline.pipeline",
					ImportState:             true,
					ImportStateVerify:       true,
					ImportStateVerifyIgnore: []string{"github_webhooks_enabled", "provider_settings"},
				},
			},
		})
	})

	t.Run("create pipeline setting all attributes", func(t *testing.T) {
		pipelineName := acctest.RandString(12)
		clusterName := acctest.RandString(12)
		config := fmt.Sprintf(`
			resource "buildkite_cluster" "cluster" {
				name = "%s"
			}
			resource "buildkite_pipeline" "pipeline" {
				name = "%s"
				repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
				cluster_id = buildkite_cluster.cluster.id
				allow_rebuilds = false
				cancel_intermediate_builds = true
				cancel_intermediate_builds_branch_filter = "!main"
				color = "#ebd8f0"
				branch_configuration = "main"
				default_branch = "main"
				default_timeout_in_minutes = 20
				emoji = ":buildkite:"
				maximum_timeout_in_minutes = 30
				description = "terraform test"
				skip_intermediate_builds = true
				skip_intermediate_builds_branch_filter = "!main"
				tags = ["llama"]
				provider_settings = {
					trigger_mode = "code"
					build_issues = true
					build_pull_requests = true
					skip_builds_for_existing_commits = true
					build_branches = true
					build_tags = true
					build_pull_request_ready_for_review = true
					build_pull_request_merge_commits = true
					cancel_deleted_branch_builds = true
					filter_enabled = true
					filter_condition = "true"
					publish_commit_status = true
					publish_blocked_as_pending = true
					publish_commit_status_per_step = true
					use_step_key_as_commit_status = true
					separate_pull_request_statuses = true
					build_merge_group_checks_requested = true
					cancel_when_merge_group_destroyed = true
					use_merge_group_base_commit_for_git_diff_base = true
					build_issue_comment_created = true
					issue_comment_command_word = "ci-force-rerun"
					issue_comment_match_mode = "exact"
					build_pull_request_review_comment_created = true
					review_comment_command_word = "ci-review-rerun"
					review_comment_match_mode = "contains"
					build_pull_request_dequeued = true
					build_pull_request_reopened = true
					build_check_run_completed = true
					build_create_event = true
					build_deployment_status_created = true
					build_pull_request_converted_to_draft = true
					build_pull_request_review_requested = true
					build_pull_request_review_dismissed = true
					build_pull_request_review_submitted = true
					build_release_created = true
					build_release_published = true
					build_release_released = true
				}
			}
		`, clusterName, pipelineName)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineDestroyFunc,
			Steps: []resource.TestStep{
				{
					Config: config,
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttrPair("buildkite_pipeline.pipeline", "cluster_id", "buildkite_cluster.cluster", "id"),
						resource.TestCheckResourceAttrPair("buildkite_pipeline.pipeline", "cluster_name", "buildkite_cluster.cluster", "name"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "tags.0", "llama"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "allow_rebuilds", "false"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "cancel_intermediate_builds", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "cancel_intermediate_builds_branch_filter", "!main"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "branch_configuration", "main"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "color", "#ebd8f0"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "default_branch", "main"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "default_timeout_in_minutes", "20"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "maximum_timeout_in_minutes", "30"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "description", "terraform test"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "emoji", ":buildkite:"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "skip_intermediate_builds", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "skip_intermediate_builds_branch_filter", "!main"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.trigger_mode", "code"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_issues", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_pull_requests", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.skip_builds_for_existing_commits", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_branches", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_tags", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_pull_request_ready_for_review", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_pull_request_merge_commits", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.cancel_deleted_branch_builds", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.filter_enabled", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.filter_condition", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.publish_commit_status", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.publish_blocked_as_pending", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.publish_commit_status_per_step", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.use_step_key_as_commit_status", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.separate_pull_request_statuses", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_merge_group_checks_requested", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.cancel_when_merge_group_destroyed", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.use_merge_group_base_commit_for_git_diff_base", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_issue_comment_created", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.issue_comment_command_word", "ci-force-rerun"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.issue_comment_match_mode", "exact"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_pull_request_review_comment_created", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.review_comment_command_word", "ci-review-rerun"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.review_comment_match_mode", "contains"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_pull_request_dequeued", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_pull_request_reopened", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_check_run_completed", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_create_event", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_deployment_status_created", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_pull_request_converted_to_draft", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_pull_request_review_requested", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_pull_request_review_dismissed", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_pull_request_review_submitted", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_release_created", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_release_published", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_release_released", "true"),
					),
				},
				{
					// Refresh-only: re-reads state via Read, which now sources provider_settings
					// from GraphQL. Unchanged values prove the GraphQL-sourced read is
					// value-equivalent to what was written via REST, including the
					// issue_comment_match_mode enum (lowercased to match the schema) and
					// use_step_key_as_commit_status.
					RefreshState:       true,
					ExpectNonEmptyPlan: false,
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.trigger_mode", "code"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_issues", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_pull_request_merge_commits", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_issue_comment_created", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.issue_comment_command_word", "ci-force-rerun"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.issue_comment_match_mode", "exact"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_pull_request_review_comment_created", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.review_comment_command_word", "ci-review-rerun"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.review_comment_match_mode", "contains"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_pull_request_dequeued", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_pull_request_reopened", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.use_step_key_as_commit_status", "true"),
					),
				},
				{
					// SUP-2592: importing a pipeline must populate provider_settings from the
					// API, not just leave it as whatever was (or wasn't) in prior state.
					// ImportStateVerify diffs every flatmapped attribute - including
					// provider_settings.* - between pre-import and post-import state.
					ResourceName:      "buildkite_pipeline.pipeline",
					ImportState:       true,
					ImportStateVerify: true,
				},
			},
		})
	})

	t.Run("update pipeline setting all attributes", func(t *testing.T) {
		var pipeline getPipelinePipeline
		pipelineName := acctest.RandString(12)
		clusterName := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineDestroyFunc,
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(`
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
						}
					`, pipelineName),
					// check api values are expected
					Check: func(s *terraform.State) error {
						slug := fmt.Sprintf("%s/%s", getenv("BUILDKITE_ORGANIZATION_SLUG"), pipelineName)
						resp, err := getPipeline(context.Background(), genqlientGraphql, slug)
						pipeline = resp.Pipeline
						return err
					},
				},
				{
					Config: fmt.Sprintf(`
						resource "buildkite_cluster" "cluster" {
							name = "%s"
						}
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
							cluster_id = buildkite_cluster.cluster.id
							allow_rebuilds = false
							cancel_intermediate_builds = true
							cancel_intermediate_builds_branch_filter = "!main"
							branch_configuration = "main"
							default_branch = "main"
							default_timeout_in_minutes = 20
							maximum_timeout_in_minutes = 30
							description = "terraform test"
							skip_intermediate_builds = true
							skip_intermediate_builds_branch_filter = "!main"
							tags = ["llama"]
							provider_settings = {
								trigger_mode = "code"
								build_pull_requests = true
								skip_builds_for_existing_commits = true
								build_branches = true
								build_tags = true
								build_pull_request_ready_for_review = true
								build_pull_request_merge_commits = true
								build_pull_request_labels_changed = true
								build_pull_request_base_branch_changed = true
								cancel_deleted_branch_builds = true
								filter_enabled = true
								filter_condition = "true"
								publish_commit_status = true
								publish_blocked_as_pending = true
								publish_commit_status_per_step = true
								use_step_key_as_commit_status = true
								separate_pull_request_statuses = true
								ignore_default_branch_pull_requests = true
								build_merge_group_checks_requested = true
								cancel_when_merge_group_destroyed = true
								use_merge_group_base_commit_for_git_diff_base = true
								build_issue_comment_created = true
								issue_comment_command_word = "/deploy"
								issue_comment_match_mode = "contains"
								build_pull_request_review_comment_created = true
								review_comment_command_word = "/review"
								review_comment_match_mode = "exact"
								build_pull_request_dequeued = true
								build_pull_request_reopened = true
								build_check_run_completed = true
								build_create_event = true
								build_deployment_status_created = true
								build_pull_request_converted_to_draft = true
								build_pull_request_review_requested = true
								build_pull_request_review_dismissed = true
								build_pull_request_review_submitted = true
								build_release_created = true
								build_release_published = true
								build_release_released = true
							}
						}
					`, clusterName, pipelineName),
					Check: resource.ComposeAggregateTestCheckFunc(
						// check the pipeline IDs are the same (so it wasn't recreated)
						func(s *terraform.State) error {
							p := s.RootModule().Resources["buildkite_pipeline.pipeline"]
							if p.Primary.ID != pipeline.Id {
								return fmt.Errorf("Pipelines do not match: %s %s", pipeline.Id, p.Primary.ID)
							}
							return nil
						},
						resource.TestCheckResourceAttrPair("buildkite_pipeline.pipeline", "cluster_id", "buildkite_cluster.cluster", "id"),
						resource.TestCheckResourceAttrPair("buildkite_pipeline.pipeline", "cluster_name", "buildkite_cluster.cluster", "name"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.ignore_default_branch_pull_requests", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_pull_request_merge_commits", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_pull_request_labels_changed", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_pull_request_base_branch_changed", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.use_step_key_as_commit_status", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_merge_group_checks_requested", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.cancel_when_merge_group_destroyed", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.use_merge_group_base_commit_for_git_diff_base", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_issue_comment_created", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.issue_comment_command_word", "/deploy"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.issue_comment_match_mode", "contains"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_pull_request_review_comment_created", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.review_comment_command_word", "/review"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.review_comment_match_mode", "exact"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_pull_request_dequeued", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_pull_request_reopened", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_check_run_completed", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_create_event", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_deployment_status_created", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_pull_request_converted_to_draft", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_pull_request_review_requested", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_pull_request_review_dismissed", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_pull_request_review_submitted", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_release_created", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_release_published", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_release_released", "true"),
						aggregateRemoteCheck(&pipeline),
					),
				},
			},
		})
	})

	t.Run("changing cluster_id updates cluster_name without inconsistency error", func(t *testing.T) {
		pipelineName := acctest.RandString(12)
		clusterNameA := acctest.RandString(12)
		clusterNameB := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineDestroyFunc,
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(`
						resource "buildkite_cluster" "cluster_a" {
							name = "%s"
						}
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
							cluster_id = buildkite_cluster.cluster_a.id
						}
					`, clusterNameA, pipelineName),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttrPair("buildkite_pipeline.pipeline", "cluster_id", "buildkite_cluster.cluster_a", "id"),
						resource.TestCheckResourceAttrPair("buildkite_pipeline.pipeline", "cluster_name", "buildkite_cluster.cluster_a", "name"),
					),
				},
				{
					Config: fmt.Sprintf(`
						resource "buildkite_cluster" "cluster_a" {
							name = "%s"
						}
						resource "buildkite_cluster" "cluster_b" {
							name = "%s"
						}
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
							cluster_id = buildkite_cluster.cluster_b.id
						}
					`, clusterNameA, clusterNameB, pipelineName),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttrPair("buildkite_pipeline.pipeline", "cluster_id", "buildkite_cluster.cluster_b", "id"),
						resource.TestCheckResourceAttrPair("buildkite_pipeline.pipeline", "cluster_name", "buildkite_cluster.cluster_b", "name"),
					),
				},
			},
		})
	})

	t.Run("pipeline is recreated if removed", func(t *testing.T) {
		pipelineName := acctest.RandString(12)
		config := fmt.Sprintf(`
			resource "buildkite_pipeline" "pipeline" {
				name = "%s"
				repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
			}
		`, pipelineName)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineDestroyFunc,
			Steps: []resource.TestStep{
				{
					Config: config,
					Check: func(s *terraform.State) error {
						// remove the pipeline
						pipeline := s.RootModule().Resources["buildkite_pipeline.pipeline"]
						_, err := deletePipeline(context.Background(), genqlientGraphql, pipeline.Primary.ID)
						return err
					},
					ExpectNonEmptyPlan: true,
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{
							// expect terraform to plan a new create
							plancheck.ExpectResourceAction("buildkite_pipeline.pipeline", plancheck.ResourceActionCreate),
						},
					},
				},
			},
		})
	})

	t.Run("pipeline can be deleted", func(t *testing.T) {
		pipelineName := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineDestroyFunc,
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(`
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
						}
					`, pipelineName),
				},
			},
		})
	})

	t.Run("pipeline with cluster can be deleted", func(t *testing.T) {
		pipelineName := acctest.RandString(12)
		clusterName := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineDestroyFunc,
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(`
						resource "buildkite_cluster" "cluster" {
							name = "%s"
						}
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
							cluster_id = buildkite_cluster.cluster.id
						}
					`, clusterName, pipelineName),
				},
			},
		})
	})

	t.Run("empty provider_settings updated from v0 to v1", func(t *testing.T) {
		pipelineName := acctest.RandString(12)

		config := fmt.Sprintf(`
			resource "buildkite_pipeline" "pipeline" {
				name = "%s"
				repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
			}
		`, pipelineName)

		// change repo name to trigger a resource update as well
		configNested := fmt.Sprintf(`
			resource "buildkite_pipeline" "pipeline" {
				name = "%s"
				repository = "https://github.com/buildkite/terraform-Provider-buildkite.git"
			}
		`, pipelineName)

		check := resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "name", pipelineName),
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "repository", "https://github.com/buildkite/terraform-provider-buildkite.git"),
			// Ensure that v0 pipeline's provider_settings is a list of length 1 in state & defaulted attributes are at index 0
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.#", "0"),
		)

		checkNested := resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "name", pipelineName),
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "repository", "https://github.com/buildkite/terraform-Provider-buildkite.git"),
			// Ensure that v1 pipeline's provider_settings defaulted attributes are nested in state when upgraded from v0
			resource.TestCheckNoResourceAttr("buildkite_pipeline.pipeline", "provider_settings"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck: func() { testAccPreCheck(t) },
			Steps: []resource.TestStep{
				{
					Config: config,
					ExternalProviders: map[string]resource.ExternalProvider{
						"buildkite": {
							Source:            "registry.terraform.io/buildkite/buildkite",
							VersionConstraint: "0.27.0",
						},
					},
					Check: check,
				},
				{
					Config:                   configNested,
					ProtoV6ProviderFactories: protoV6ProviderFactories(),
					Check:                    checkNested,
				},
			},
		})
	})

	t.Run("filled provider_settings updated from v0 to v1", func(t *testing.T) {
		pipelineName := acctest.RandString(12)

		config := fmt.Sprintf(`
			resource "buildkite_pipeline" "pipeline" {
				name = "%s"
				repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
				provider_settings {
					trigger_mode = "code"
					build_pull_requests = true
					skip_builds_for_existing_commits = true
					build_branches = true
					build_tags = true
					build_pull_request_ready_for_review = true
					cancel_deleted_branch_builds = true
					filter_enabled = true
					filter_condition = "true"
					publish_commit_status = true
					publish_blocked_as_pending = true
					publish_commit_status_per_step = true
					separate_pull_request_statuses = true
				}
			}
		`, pipelineName)

		configNested := fmt.Sprintf(`
			resource "buildkite_pipeline" "pipeline" {
				name = "%s"
				repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
				provider_settings = {
					trigger_mode = "code"
					build_pull_requests = true
					skip_builds_for_existing_commits = true
					build_branches = true
					build_tags = true
					build_pull_request_ready_for_review = true
					cancel_deleted_branch_builds = true
					filter_enabled = true
					filter_condition = "true"
					publish_commit_status = true
					publish_blocked_as_pending = true
					publish_commit_status_per_step = true
					separate_pull_request_statuses = true
				}
			}
		`, pipelineName)

		check := resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "name", pipelineName),
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "repository", "https://github.com/buildkite/terraform-provider-buildkite.git"),
			// Ensure that v0 pipeline's provider_settings is a list of length 1 in state, attributes set and are at index 0
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.#", "1"),
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.0.build_branches", "true"),
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.0.build_pull_requests", "true"),
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.0.build_pull_request_ready_for_review", "true"),
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.0.build_tags", "true"),
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.0.cancel_deleted_branch_builds", "true"),
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.0.filter_condition", "true"),
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.0.filter_enabled", "true"),
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.0.publish_blocked_as_pending", "true"),
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.0.publish_commit_status", "true"),
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.0.publish_commit_status_per_step", "true"),
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.0.separate_pull_request_statuses", "true"),
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.0.skip_builds_for_existing_commits", "true"),
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.0.skip_pull_request_builds_for_existing_commits", "true"),
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.0.trigger_mode", "code"),
		)

		checkNested := resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "name", pipelineName),
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "repository", "https://github.com/buildkite/terraform-provider-buildkite.git"),
			// Ensure that v1 pipeline's provider_settings set attributes are nested in state when upgraded from v0
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_branches", "true"),
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_issues", "false"),
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_pull_requests", "true"),
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_pull_request_ready_for_review", "true"),
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_tags", "true"),
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.cancel_deleted_branch_builds", "true"),
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.filter_condition", "true"),
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.filter_enabled", "true"),
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.publish_blocked_as_pending", "true"),
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.publish_commit_status", "true"),
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.publish_commit_status_per_step", "true"),
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.separate_pull_request_statuses", "true"),
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.skip_builds_for_existing_commits", "true"),
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.skip_pull_request_builds_for_existing_commits", "true"),
			resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.trigger_mode", "code"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck: func() { testAccPreCheck(t) },
			Steps: []resource.TestStep{
				{
					Config: config,
					ExternalProviders: map[string]resource.ExternalProvider{
						"buildkite": {
							Source:            "registry.terraform.io/buildkite/buildkite",
							VersionConstraint: "0.27.0",
						},
					},
					Check: check,
				},
				{
					Config:                   configNested,
					ProtoV6ProviderFactories: protoV6ProviderFactories(),
					Check:                    checkNested,
				},
			},
		})
	})

	t.Run("provider_settings attributes can be removed without state change", func(t *testing.T) {
		pipelineName := acctest.RandString(12)
		clusterName := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineDestroyFunc,
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(`
						resource "buildkite_cluster" "cluster" {
							name = "%s"
						}
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
							provider_settings = {
								trigger_mode = "none"
							}
						}
					`, clusterName, pipelineName),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "name", pipelineName),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "repository", "https://github.com/buildkite/terraform-provider-buildkite.git"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.trigger_mode", "none"),
					),
				},
				{
					Config: fmt.Sprintf(`
						resource "buildkite_cluster" "cluster" {
							name = "%s"
						}
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
							cluster_id = buildkite_cluster.cluster.id
							provider_settings = {}
						}
					`, clusterName, pipelineName),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{
							plancheck.ExpectEmptyPlan(),
							plancheck.ExpectResourceAction("buildkite_pipeline.pipeline", plancheck.ResourceActionNoop),
						},
					},
				},
			},
		})
	})

	t.Run("provider_settings produces empty plan on re-apply", func(t *testing.T) {
		pipelineName := acctest.RandString(12)
		config := fmt.Sprintf(`
			resource "buildkite_pipeline" "pipeline" {
				name = "%s"
				repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
				provider_settings = {}
			}
		`, pipelineName)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineDestroyFunc,
			Steps: []resource.TestStep{
				{
					Config: config,
				},
				{
					Config: config,
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{
							plancheck.ExpectEmptyPlan(),
						},
					},
				},
			},
		})
	})

	t.Run("create in template mode and change template configuration afterwards", func(t *testing.T) {
		templateName := acctest.RandString(12)
		pipelineName := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineDestroyFunc,
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(`
						resource "buildkite_pipeline_template" "template" {
							name = "%s"
							configuration = "steps: []"
							available = true
						}
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
							pipeline_template_id = buildkite_pipeline_template.template.id
						}
					`, templateName, pipelineName),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "name", pipelineName),
						resource.TestCheckNoResourceAttr("buildkite_pipeline.pipeline", "steps"),
					),
				},
				{
					// now change the template steps, we dont expect the pipeline to change at all
					Config: fmt.Sprintf(`
						resource "buildkite_pipeline_template" "template" {
							name = "%s"
							configuration = "steps: [command: echo hello]"
							available = true
						}
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
							pipeline_template_id = buildkite_pipeline_template.template.id
						}
					`, templateName, pipelineName),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PreApply: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction("buildkite_pipeline_template.template", plancheck.ResourceActionUpdate),
							plancheck.ExpectResourceAction("buildkite_pipeline.pipeline", plancheck.ResourceActionNoop),
						},
					},
				},
			},
		})
	})

	t.Run("create in template mode and change to explicit steps mode", func(t *testing.T) {
		templateName := acctest.RandString(12)
		pipelineName := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineDestroyFunc,
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(`
						resource "buildkite_pipeline_template" "template" {
							name = "%s"
							configuration = "steps: []"
							available = true
						}
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
							pipeline_template_id = buildkite_pipeline_template.template.id
						}
					`, templateName, pipelineName),
					Check: resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "name", pipelineName),
				},
				{
					// now remove the template and set steps
					Config: fmt.Sprintf(`
						resource "buildkite_pipeline_template" "template" {
							name = "%s"
							configuration = "steps: []"
							available = true
						}
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
							steps = "steps: []"
						}
					`, templateName, pipelineName),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "steps", "steps: []"),
					),
				},
			},
		})
	})

	t.Run("create in template mode and change to implicit steps mode", func(t *testing.T) {
		templateName := acctest.RandString(12)
		pipelineName := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineDestroyFunc,
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(`
						resource "buildkite_pipeline_template" "template" {
							name = "%s"
							configuration = "steps: []"
							available = true
						}
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
							pipeline_template_id = buildkite_pipeline_template.template.id
						}
					`, templateName, pipelineName),
					Check: resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "name", pipelineName),
				},
				{
					// now remove the template and steps which should use the default
					Config: fmt.Sprintf(`
						resource "buildkite_pipeline_template" "template" {
							name = "%s"
							configuration = "steps: []"
							available = true
						}
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
						}
					`, templateName, pipelineName),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "steps", defaultSteps),
					),
				},
			},
		})
	})

	t.Run("create in implicit steps mode and change to template mode", func(t *testing.T) {
		templateName := acctest.RandString(12)
		pipelineName := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineDestroyFunc,
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(`
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
						}
					`, pipelineName),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "steps", defaultSteps),
					),
				},
				// now convert to using a template and confirm steps are empty
				{
					Config: fmt.Sprintf(`
						resource "buildkite_pipeline_template" "template" {
							name = "%s"
							configuration = "steps: []"
							available = true
						}
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
							pipeline_template_id = buildkite_pipeline_template.template.id
						}
					`, templateName, pipelineName),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckNoResourceAttr("buildkite_pipeline.pipeline", "steps"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "name", pipelineName),
					),
				},
			},
		})
	})

	t.Run("create in explicit steps mode and change to template mode", func(t *testing.T) {
		templateName := acctest.RandString(12)
		pipelineName := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineDestroyFunc,
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(`
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
							steps = "steps: []"
						}
					`, pipelineName),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "steps", "steps: []"),
					),
				},
				// now convert to using a template and confirm steps are empty
				{
					Config: fmt.Sprintf(`
						resource "buildkite_pipeline_template" "template" {
							name = "%s"
							configuration = "steps: [command: echo hello]"
							available = true
						}
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
							pipeline_template_id = buildkite_pipeline_template.template.id
						}
					`, templateName, pipelineName),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckNoResourceAttr("buildkite_pipeline.pipeline", "steps"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "name", pipelineName),
					),
				},
			},
		})
	})

	t.Run("reject conditional expressions in branch filter fields", func(t *testing.T) {
		pipelineName := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineDestroyFunc,
			Steps: []resource.TestStep{
				{
					// Test rejection of conditional syntax in cancel_intermediate_builds_branch_filter
					Config: fmt.Sprintf(`
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
							cancel_intermediate_builds = true
							cancel_intermediate_builds_branch_filter = "build.branch !~ foo"
						}
					`, pipelineName),
					ExpectError: regexp.MustCompile(`Invalid branch filter pattern`),
				},
				{
					// Test rejection of conditional syntax in skip_intermediate_builds_branch_filter
					Config: fmt.Sprintf(`
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
							skip_intermediate_builds = true
							skip_intermediate_builds_branch_filter = "build.branch =~ /bar/"
						}
					`, pipelineName),
					ExpectError: regexp.MustCompile(`Invalid branch filter pattern`),
				},
				{
					// Test rejection of conditional syntax in branch_configuration
					Config: fmt.Sprintf(`
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
							branch_configuration = "build.branch == 'main'"
						}
					`, pipelineName),
					ExpectError: regexp.MustCompile(`Invalid branch filter pattern`),
				},
				{
					// Test valid simple glob patterns (no conditional operators)
					Config: fmt.Sprintf(`
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
							cancel_intermediate_builds = true
							cancel_intermediate_builds_branch_filter = "!main"
							skip_intermediate_builds = true
							skip_intermediate_builds_branch_filter = "feature/*"
							branch_configuration = "main"
						}
					`, pipelineName),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "cancel_intermediate_builds_branch_filter", "!main"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "skip_intermediate_builds_branch_filter", "feature/*"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "branch_configuration", "main"),
					),
				},
				{
					// Test multiple glob patterns in a single field
					Config: fmt.Sprintf(`
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
							cancel_intermediate_builds = true
							cancel_intermediate_builds_branch_filter = "!main !develop"
							skip_intermediate_builds = true
							skip_intermediate_builds_branch_filter = "feature/* bugfix/*"
							branch_configuration = "*-staging *-production"
						}
					`, pipelineName),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "cancel_intermediate_builds_branch_filter", "!main !develop"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "skip_intermediate_builds_branch_filter", "feature/* bugfix/*"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "branch_configuration", "*-staging *-production"),
					),
				},
			},
		})
	})

	t.Run("validate regex patterns in filter_condition", func(t *testing.T) {
		pipelineName := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineDestroyFunc,
			Steps: []resource.TestStep{
				{
					// Test invalid regex pattern without forward slashes in filter_condition
					Config: fmt.Sprintf(`
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
							provider_settings = {
								trigger_mode = "code"
								filter_enabled = true
								filter_condition = "build.branch =~ feature"
							}
						}
					`, pipelineName),
					ExpectError: regexp.MustCompile(`Invalid regex pattern syntax`),
				},
				{
					// Test valid regex pattern with forward slashes in filter_condition
					Config: fmt.Sprintf(`
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
							provider_settings = {
								trigger_mode = "code"
								filter_enabled = true
								filter_condition = "build.branch =~ /^feature\\/.*/"
							}
						}
					`, pipelineName),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.filter_condition", "build.branch =~ /^feature\\/.*/"),
					),
				},
				{
					// Test valid conditional without regex operators
					Config: fmt.Sprintf(`
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
							provider_settings = {
								trigger_mode = "code"
								filter_enabled = true
								filter_condition = "build.branch == 'main'"
							}
						}
					`, pipelineName),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.filter_condition", "build.branch == 'main'"),
					),
				},
			},
		})
	})

	t.Run("creates pipeline with PUBLIC visibility", func(t *testing.T) {
		pipelineName := acctest.RandString(12)
		config := fmt.Sprintf(`
			resource "buildkite_pipeline" "pipeline" {
				name = "%s"
				repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
				visibility = "PUBLIC"
			}
		`, pipelineName)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineDestroyFunc,
			Steps: []resource.TestStep{
				{
					Config: config,
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "visibility", "PUBLIC"),
						func(s *terraform.State) error {
							slug := fmt.Sprintf("%s/%s", getenv("BUILDKITE_ORGANIZATION_SLUG"), pipelineName)
							resp, err := getPipeline(context.Background(), genqlientGraphql, slug)
							if err != nil {
								return err
							}
							if string(resp.Pipeline.GetVisibility()) != "PUBLIC" {
								return fmt.Errorf("expected visibility PUBLIC, got %s", resp.Pipeline.GetVisibility())
							}
							return nil
						},
					),
				},
			},
		})
	})

	t.Run("creates pipeline with PRIVATE visibility", func(t *testing.T) {
		pipelineName := acctest.RandString(12)
		config := fmt.Sprintf(`
			resource "buildkite_pipeline" "pipeline" {
				name = "%s"
				repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
				visibility = "PRIVATE"
			}
		`, pipelineName)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineDestroyFunc,
			Steps: []resource.TestStep{
				{
					Config: config,
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "visibility", "PRIVATE"),
						func(s *terraform.State) error {
							slug := fmt.Sprintf("%s/%s", getenv("BUILDKITE_ORGANIZATION_SLUG"), pipelineName)
							resp, err := getPipeline(context.Background(), genqlientGraphql, slug)
							if err != nil {
								return err
							}
							if string(resp.Pipeline.GetVisibility()) != "PRIVATE" {
								return fmt.Errorf("expected visibility PRIVATE, got %s", resp.Pipeline.GetVisibility())
							}
							return nil
						},
					),
				},
			},
		})
	})

	t.Run("creates pipeline with default visibility when not specified", func(t *testing.T) {
		pipelineName := acctest.RandString(12)
		config := fmt.Sprintf(`
			resource "buildkite_pipeline" "pipeline" {
				name = "%s"
				repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
			}
		`, pipelineName)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineDestroyFunc,
			Steps: []resource.TestStep{
				{
					Config: config,
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "visibility", "PRIVATE"),
						func(s *terraform.State) error {
							slug := fmt.Sprintf("%s/%s", getenv("BUILDKITE_ORGANIZATION_SLUG"), pipelineName)
							resp, err := getPipeline(context.Background(), genqlientGraphql, slug)
							if err != nil {
								return err
							}
							if string(resp.Pipeline.Visibility) != "PRIVATE" {
								return fmt.Errorf("expected default visibility PRIVATE, got %s", resp.Pipeline.Visibility)
							}
							return nil
						},
					),
				},
			},
		})
	})

	t.Run("updates pipeline visibility from PRIVATE to PUBLIC", func(t *testing.T) {
		pipelineName := acctest.RandString(12)
		configPrivate := fmt.Sprintf(`
			resource "buildkite_pipeline" "pipeline" {
				name = "%s"
				repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
				visibility = "PRIVATE"
			}
		`, pipelineName)
		configPublic := fmt.Sprintf(`
			resource "buildkite_pipeline" "pipeline" {
				name = "%s"
				repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
				visibility = "PUBLIC"
			}
		`, pipelineName)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineDestroyFunc,
			Steps: []resource.TestStep{
				{
					Config: configPrivate,
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "visibility", "PRIVATE"),
					),
				},
				{
					Config: configPublic,
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PreApply: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction("buildkite_pipeline.pipeline", plancheck.ResourceActionUpdate),
						},
					},
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "visibility", "PUBLIC"),
						func(s *terraform.State) error {
							slug := fmt.Sprintf("%s/%s", getenv("BUILDKITE_ORGANIZATION_SLUG"), pipelineName)
							resp, err := getPipeline(context.Background(), genqlientGraphql, slug)
							if err != nil {
								return err
							}
							if string(resp.Pipeline.Visibility) != "PUBLIC" {
								return fmt.Errorf("expected visibility PUBLIC after update, got %s", resp.Pipeline.Visibility)
							}
							return nil
						},
					),
				},
			},
		})
	})

	t.Run("rejects invalid visibility values", func(t *testing.T) {
		pipelineName := acctest.RandString(12)
		config := fmt.Sprintf(`
			resource "buildkite_pipeline" "pipeline" {
				name = "%s"
				repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
				visibility = "INVALID"
			}
		`, pipelineName)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config:      config,
					ExpectError: regexp.MustCompile(`Attribute visibility value must be one of: \["PUBLIC" "PRIVATE"\]`),
				},
			},
		})
	})

	t.Run("creates pipeline with archived = true", func(t *testing.T) {
		pipelineName := acctest.RandString(12)
		config := fmt.Sprintf(`
			resource "buildkite_pipeline" "pipeline" {
				name = "%s"
				repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
				archived = true
			}
		`, pipelineName)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineDestroyFunc,
			Steps: []resource.TestStep{
				{
					Config: config,
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "archived", "true"),
						func(s *terraform.State) error {
							slug := fmt.Sprintf("%s/%s", getenv("BUILDKITE_ORGANIZATION_SLUG"), pipelineName)
							resp, err := getPipeline(context.Background(), genqlientGraphql, slug)
							if err != nil {
								return err
							}
							if !resp.Pipeline.Archived {
								return fmt.Errorf("expected pipeline to be archived, got archived = false")
							}
							return nil
						},
					),
				},
			},
		})
	})

	t.Run("updates pipeline archived state", func(t *testing.T) {
		pipelineName := acctest.RandString(12)
		configUnarchived := fmt.Sprintf(`
			resource "buildkite_pipeline" "pipeline" {
				name = "%s"
				repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
				archived = false
			}
		`, pipelineName)
		configArchived := fmt.Sprintf(`
			resource "buildkite_pipeline" "pipeline" {
				name = "%s"
				repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
				archived = true
			}
		`, pipelineName)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineDestroyFunc,
			Steps: []resource.TestStep{
				{
					Config: configUnarchived,
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "archived", "false"),
					),
				},
				{
					Config: configArchived,
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PreApply: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction("buildkite_pipeline.pipeline", plancheck.ResourceActionUpdate),
						},
					},
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "archived", "true"),
						func(s *terraform.State) error {
							slug := fmt.Sprintf("%s/%s", getenv("BUILDKITE_ORGANIZATION_SLUG"), pipelineName)
							resp, err := getPipeline(context.Background(), genqlientGraphql, slug)
							if err != nil {
								return err
							}
							if !resp.Pipeline.Archived {
								return fmt.Errorf("expected pipeline to be archived after update, got archived = false")
							}
							return nil
						},
					),
				},
				{
					Config: configUnarchived,
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PreApply: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction("buildkite_pipeline.pipeline", plancheck.ResourceActionUpdate),
						},
					},
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "archived", "false"),
						func(s *terraform.State) error {
							slug := fmt.Sprintf("%s/%s", getenv("BUILDKITE_ORGANIZATION_SLUG"), pipelineName)
							resp, err := getPipeline(context.Background(), genqlientGraphql, slug)
							if err != nil {
								return err
							}
							if resp.Pipeline.Archived {
								return fmt.Errorf("expected pipeline to be unarchived after update, got archived = true")
							}
							return nil
						},
					),
				},
			},
		})
	})

	t.Run("archives pipeline with provider_settings", func(t *testing.T) {
		// Regression test: archiving must happen after the provider_settings REST
		// PATCH, otherwise the API rejects it with "Cannot update an archived
		// pipeline".
		pipelineName := acctest.RandString(12)
		configFor := func(archived bool) string {
			return fmt.Sprintf(`
				resource "buildkite_pipeline" "pipeline" {
					name = "%s"
					repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
					archived = %t

					provider_settings = {
						build_branches = true
						build_pull_requests = false
					}
				}
			`, pipelineName, archived)
		}

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineDestroyFunc,
			Steps: []resource.TestStep{
				{
					Config: configFor(false),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "archived", "false"),
					),
				},
				{
					Config: configFor(true),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PreApply: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction("buildkite_pipeline.pipeline", plancheck.ResourceActionUpdate),
						},
					},
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "archived", "true"),
						func(s *terraform.State) error {
							slug := fmt.Sprintf("%s/%s", getenv("BUILDKITE_ORGANIZATION_SLUG"), pipelineName)
							resp, err := getPipeline(context.Background(), genqlientGraphql, slug)
							if err != nil {
								return err
							}
							if !resp.Pipeline.Archived {
								return fmt.Errorf("expected pipeline to be archived, got archived = false")
							}
							return nil
						},
					),
				},
			},
		})
	})

	t.Run("creates archived pipeline with provider_settings", func(t *testing.T) {
		pipelineName := acctest.RandString(12)
		config := fmt.Sprintf(`
			resource "buildkite_pipeline" "pipeline" {
				name = "%s"
				repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
				archived = true

				provider_settings = {
					build_branches = true
				}
			}
		`, pipelineName)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineDestroyFunc,
			Steps: []resource.TestStep{
				{
					Config: config,
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "archived", "true"),
						func(s *terraform.State) error {
							slug := fmt.Sprintf("%s/%s", getenv("BUILDKITE_ORGANIZATION_SLUG"), pipelineName)
							resp, err := getPipeline(context.Background(), genqlientGraphql, slug)
							if err != nil {
								return err
							}
							if !resp.Pipeline.Archived {
								return fmt.Errorf("expected pipeline to be archived, got archived = false")
							}
							return nil
						},
					),
				},
			},
		})
	})
}

// findAndRemoveTeam looks the pipeline up by slug, and state.Slug still holds the slug from the
// pipelineUpdate response at that point, which predates the REST rename. It has to be given
// useSlugValue instead. Getting this wrong is silent: the lookup finds no teams, the old default
// team is never detached, and Update reports success.
func TestPipelineUpdateRemovesTheDefaultTeamByThePostRenameSlug(t *testing.T) {
	t.Parallel()

	const (
		apiSlug    = "renamed-pipeline"
		chosenSlug = "a-chosen-slug"
	)

	server, requests, bodies := newRecordingRetryStub(t,
		// pipelineUpdate
		stubResponse{status: http.StatusOK, body: fmt.Sprintf(`{"data":{"pipelineUpdate":{"pipeline":{
			"id": "pipeline-id", "pipelineUuid": "pipeline-uuid", "name": "p", "slug": %q,
			"defaultBranch": "main", "description": "", "archived": false,
			"repository": {"url": "git@github.com:org/repo.git"}, "steps": {"yaml": "steps: []"},
			"tags": [], "teams": {"edges": []}
		}}}}`, apiSlug)},
		// the REST rename succeeds, so the pipeline now answers to chosenSlug
		stubResponse{status: http.StatusOK, body: `{"slug":"a-chosen-slug"}`},
		// getPipelineTeams, whose request body is what this test is about
		stubResponse{status: http.StatusOK, body: `{"data":{"pipeline":{"teams":{
			"edges": [], "pageInfo": {"hasNextPage": false, "endCursor": ""}
		}}}}`},
	)
	defer server.Close()

	p := &pipelineResource{client: newRetryTestClient(t, server.URL, 0, time.Millisecond)}

	ctx := t.Context()
	var schemaResp fwresource.SchemaResponse
	p.Schema(ctx, fwresource.SchemaRequest{}, &schemaResp)
	schema := schemaResp.Schema

	prior := nullObjectWith(ctx, t, schema.Type(), map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "pipeline-id"),
		"name":            tftypes.NewValue(tftypes.String, "p"),
		"slug":            tftypes.NewValue(tftypes.String, "original-pipeline"),
		"repository":      tftypes.NewValue(tftypes.String, "git@github.com:org/repo.git"),
		"steps":           tftypes.NewValue(tftypes.String, "steps: []"),
		"default_team_id": tftypes.NewValue(tftypes.String, "team-id"),
		"archived":        tftypes.NewValue(tftypes.Bool, false),
	})
	// No default_team_id in the plan, which is what sends Update into findAndRemoveTeam.
	planned := nullObjectWith(ctx, t, schema.Type(), map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "pipeline-id"),
		"name":       tftypes.NewValue(tftypes.String, "p"),
		"slug":       tftypes.NewValue(tftypes.String, chosenSlug),
		"repository": tftypes.NewValue(tftypes.String, "git@github.com:org/repo.git"),
		"steps":      tftypes.NewValue(tftypes.String, "steps: []"),
		"archived":   tftypes.NewValue(tftypes.Bool, false),
	})

	req := fwresource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: schema, Raw: planned},
		State:  tfsdk.State{Schema: schema, Raw: prior},
		Config: tfsdk.Config{Schema: schema, Raw: planned},
	}
	resp := fwresource.UpdateResponse{State: tfsdk.State{Schema: schema, Raw: prior}}

	p.Update(ctx, req, &resp)

	if got := requests.Load(); got < 3 {
		t.Fatalf("Made %d requests, want the rename and the team lookup to both have run", got)
	}

	var teamLookups []string
	for _, body := range bodies() {
		if strings.Contains(body, "getPipelineTeams") {
			teamLookups = append(teamLookups, body)
		}
	}
	if len(teamLookups) == 0 {
		t.Fatalf("No getPipelineTeams request was made; bodies were %v", bodies())
	}
	for _, lookup := range teamLookups {
		if !strings.Contains(lookup, "test-org/"+chosenSlug) {
			t.Errorf("getPipelineTeams asked for the wrong pipeline: %s\nwant the post-rename slug %q", lookup, chosenSlug)
		}
	}
}

func diagnosticsContain(diags diag.Diagnostics, summary string) bool {
	for _, d := range diags.Errors() {
		if strings.Contains(d.Summary(), summary) || strings.Contains(d.Detail(), summary) {
			return true
		}
	}

	return false
}

// From the moment the pipeline exists, every path out of Create has to record it. A pipeline left
// out of state is not merely re-planned: it is orphaned in Buildkite with nothing pointing at it,
// and the next apply tries to create a second one.
func TestPipelineCreatePersistsStateWhenALaterStepFails(t *testing.T) {
	t.Parallel()

	const (
		pipelineName = "a-new-pipeline"
		apiSlug      = "a-new-pipeline"
	)

	pipelineCreated := stubResponse{status: http.StatusOK, body: fmt.Sprintf(`{"data":{"pipelineCreate":{"pipeline":{
		"id": "pipeline-id",
		"pipelineUuid": "pipeline-uuid",
		"name": %q,
		"slug": %q,
		"defaultBranch": "main",
		"description": "",
		"repository": {"url": "git@github.com:org/repo.git"},
		"steps": {"yaml": "steps: []"},
		"tags": [],
		"teams": {"edges": []}
	}}}}`, pipelineName, apiSlug)}

	tests := []struct {
		name    string
		plan    map[string]tftypes.Value
		failure stubResponse
		wantErr string
	}{
		{
			// The rename is the first thing Create does after the pipeline exists.
			name:    "slug rename over REST",
			plan:    map[string]tftypes.Value{"slug": tftypes.NewValue(tftypes.String, "a-chosen-slug")},
			failure: stubResponse{status: http.StatusInternalServerError, body: `{"message":"rename failed"}`},
			wantErr: "Unable to set pipeline slug from REST",
		},
		{
			// Archiving is the last, and it runs after every other step has already applied.
			name:    "archive",
			plan:    map[string]tftypes.Value{"archived": tftypes.NewValue(tftypes.Bool, true)},
			failure: stubResponse{status: http.StatusOK, body: `{"errors":[{"message":"archive exploded"}]}`},
			wantErr: "Unable to archive pipeline",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server, requests := newRetryStub(t, pipelineCreated, testCase.failure)
			defer server.Close()

			client := newRetryTestClient(t, server.URL, 0, time.Millisecond)
			// Primed so the organization lookup does not consume a stubbed response of its own.
			orgID := "organization-id"
			client.organizationId = &orgID
			p := &pipelineResource{client: client}

			ctx := t.Context()
			var schemaResp fwresource.SchemaResponse
			p.Schema(ctx, fwresource.SchemaRequest{}, &schemaResp)
			if schemaResp.Diagnostics.HasError() {
				t.Fatalf("Schema() diagnostics = %v", schemaResp.Diagnostics)
			}
			schema := schemaResp.Schema

			planned := map[string]tftypes.Value{
				"name":       tftypes.NewValue(tftypes.String, pipelineName),
				"repository": tftypes.NewValue(tftypes.String, "git@github.com:org/repo.git"),
				"steps":      tftypes.NewValue(tftypes.String, "steps: []"),
				"archived":   tftypes.NewValue(tftypes.Bool, false),
			}
			for name, value := range testCase.plan {
				planned[name] = value
			}
			raw := nullObjectWith(ctx, t, schema.Type(), planned)

			req := fwresource.CreateRequest{
				Plan:   tfsdk.Plan{Schema: schema, Raw: raw},
				Config: tfsdk.Config{Schema: schema, Raw: raw},
			}
			resp := fwresource.CreateResponse{State: tfsdk.State{Schema: schema, Raw: tftypes.NewValue(schema.Type().TerraformType(ctx), nil)}}

			p.Create(ctx, req, &resp)

			if requests.Load() < 2 {
				t.Fatalf("Made %d requests, want the pipeline to have been created before the failure", requests.Load())
			}
			if !diagnosticsContain(resp.Diagnostics, testCase.wantErr) {
				t.Fatalf("Create() diagnostics = %v, want %q", resp.Diagnostics, testCase.wantErr)
			}

			var persisted pipelineResourceModel
			if diags := resp.State.Get(ctx, &persisted); diags.HasError() {
				t.Fatalf("Reading the persisted state = %v", diags)
			}
			if got := persisted.Id.ValueString(); got != "pipeline-id" {
				t.Errorf("Persisted id = %q, want %q: without it the pipeline is orphaned and the next apply creates a second one", got, "pipeline-id")
			}
			// The rename never applied, so state has to name the slug the pipeline actually answers to.
			if got := persisted.Slug.ValueString(); got != apiSlug {
				t.Errorf("Persisted slug = %q, want %q", got, apiSlug)
			}
		})
	}
}

// Update applies the pipeline mutation before several REST and GraphQL steps that can each fail.
// Terraform accepts state that differs from the plan when the provider also returns an error, and
// persisting it there is the only thing between a failure in a later step and losing every field
// the mutation changed.
func TestPipelineUpdatePersistsStateWhenALaterStepFails(t *testing.T) {
	t.Parallel()

	const (
		priorName   = "original-pipeline"
		updatedName = "renamed-pipeline"
		apiSlug     = "renamed-pipeline"
	)

	// The pipelineUpdate mutation, which every case below needs to succeed before it can fail at
	// something later.
	mutationApplied := stubResponse{status: http.StatusOK, body: fmt.Sprintf(`{"data":{"pipelineUpdate":{"pipeline":{
		"id": "pipeline-id",
		"pipelineUuid": "pipeline-uuid",
		"name": %q,
		"slug": %q,
		"defaultBranch": "main",
		"description": "",
		"repository": {"url": "git@github.com:org/repo.git"},
		"steps": {"yaml": "steps: []"},
		"tags": [],
		"teams": {"edges": []}
	}}}}`, updatedName, apiSlug)}

	tests := []struct {
		name string
		// Attributes to add to the plan and config on top of the shared ones below.
		plan     map[string]tftypes.Value
		config   map[string]tftypes.Value
		failure  stubResponse
		wantErr  string
		wantSlug string
	}{
		{
			// A default team in state and none in the plan sends Update into findAndRemoveTeam, which
			// pages on plain GraphQL. A GraphQL error in a 200 body is non-retryable, so it fails at once.
			name:     "default team removal",
			failure:  stubResponse{status: http.StatusOK, body: `{"errors":[{"message":"team lookup exploded"}]}`},
			wantErr:  "Could not remove default team",
			wantSlug: apiSlug,
		},
		{
			// archived is synced to the plan before any of the steps below run, so a step failing
			// between there and the archive mutation must not persist the archived state. Nothing
			// corrects it afterwards: Read refreshes archived from the API, but under -refresh=false
			// the next plan sees archived true -> true, shows no diff, and the pipeline is never
			// archived.
			name:     "archive requested but an earlier step fails",
			plan:     map[string]tftypes.Value{"archived": tftypes.NewValue(tftypes.Bool, true)},
			failure:  stubResponse{status: http.StatusOK, body: `{"errors":[{"message":"team lookup exploded"}]}`},
			wantErr:  "Could not remove default team",
			wantSlug: apiSlug,
		},
		{
			// A different default team in the plan attaches the new one before detaching the old.
			name:     "default team replacement",
			plan:     map[string]tftypes.Value{"default_team_id": tftypes.NewValue(tftypes.String, "new-team-id")},
			failure:  stubResponse{status: http.StatusOK, body: `{"errors":[{"message":"team attach exploded"}]}`},
			wantErr:  "Could not attach new default team to pipeline",
			wantSlug: apiSlug,
		},
		{
			// The last REST step before the archive mutation, and the one furthest from the pipeline
			// mutation whose result has to survive.
			name: "github webhooks over REST",
			plan: map[string]tftypes.Value{
				"github_webhooks_enabled": tftypes.NewValue(tftypes.Bool, true),
				// Unchanged, so neither team branch runs and the webhook call is what fails.
				"default_team_id": tftypes.NewValue(tftypes.String, "team-id"),
			},
			failure:  stubResponse{status: http.StatusInternalServerError, body: `{"message":"webhook update failed"}`},
			wantErr:  "Unable to set pipeline GitHub webhooks",
			wantSlug: apiSlug,
		},
		{
			// A configured slug renames the pipeline over REST. That call sits between the mutation and
			// everything else, so it is the earliest point at which the update can be lost.
			name:    "slug rename over REST",
			plan:    map[string]tftypes.Value{"slug": tftypes.NewValue(tftypes.String, "a-chosen-slug")},
			config:  map[string]tftypes.Value{"slug": tftypes.NewValue(tftypes.String, "a-chosen-slug")},
			failure: stubResponse{status: http.StatusInternalServerError, body: `{"message":"rename failed"}`},
			wantErr: "Unable to set pipeline slug from REST",
			// The rename did not apply, so the pipeline still answers to the slug the mutation returned.
			wantSlug: apiSlug,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server, requests := newRetryStub(t, mutationApplied, testCase.failure)
			defer server.Close()

			p := &pipelineResource{client: newRetryTestClient(t, server.URL, 0, time.Millisecond)}

			ctx := t.Context()
			var schemaResp fwresource.SchemaResponse
			p.Schema(ctx, fwresource.SchemaRequest{}, &schemaResp)
			if schemaResp.Diagnostics.HasError() {
				t.Fatalf("Schema() diagnostics = %v", schemaResp.Diagnostics)
			}
			schema := schemaResp.Schema

			priorState := nullObjectWith(ctx, t, schema.Type(), map[string]tftypes.Value{
				"id":              tftypes.NewValue(tftypes.String, "pipeline-id"),
				"name":            tftypes.NewValue(tftypes.String, priorName),
				"slug":            tftypes.NewValue(tftypes.String, priorName),
				"repository":      tftypes.NewValue(tftypes.String, "git@github.com:org/repo.git"),
				"steps":           tftypes.NewValue(tftypes.String, "steps: []"),
				"default_team_id": tftypes.NewValue(tftypes.String, "team-id"),
				"archived":        tftypes.NewValue(tftypes.Bool, false),
			})

			planned := map[string]tftypes.Value{
				"id":         tftypes.NewValue(tftypes.String, "pipeline-id"),
				"name":       tftypes.NewValue(tftypes.String, updatedName),
				"repository": tftypes.NewValue(tftypes.String, "git@github.com:org/repo.git"),
				"steps":      tftypes.NewValue(tftypes.String, "steps: []"),
				"archived":   tftypes.NewValue(tftypes.Bool, false),
			}
			for name, value := range testCase.plan {
				planned[name] = value
			}
			configured := maps.Clone(planned)
			for name, value := range testCase.config {
				configured[name] = value
			}

			req := fwresource.UpdateRequest{
				Plan:   tfsdk.Plan{Schema: schema, Raw: nullObjectWith(ctx, t, schema.Type(), planned)},
				State:  tfsdk.State{Schema: schema, Raw: priorState},
				Config: tfsdk.Config{Schema: schema, Raw: nullObjectWith(ctx, t, schema.Type(), configured)},
			}
			resp := fwresource.UpdateResponse{State: tfsdk.State{Schema: schema, Raw: priorState}}

			p.Update(ctx, req, &resp)

			if requests.Load() < 2 {
				t.Fatalf("Made %d requests, want the mutation to have applied before the failure", requests.Load())
			}
			// resp.Private is nil here because only the framework can build one, which adds an
			// unrelated diagnostic; assert on the failure this case is about rather than the count.
			if !diagnosticsContain(resp.Diagnostics, testCase.wantErr) {
				t.Fatalf("Update() diagnostics = %v, want %q", resp.Diagnostics, testCase.wantErr)
			}

			var persisted pipelineResourceModel
			if diags := resp.State.Get(ctx, &persisted); diags.HasError() {
				t.Fatalf("Reading the persisted state = %v", diags)
			}
			if got := persisted.Name.ValueString(); got != updatedName {
				t.Errorf("Persisted name = %q, want %q: the mutation applied, so dropping it leaves Terraform planning the same change again", got, updatedName)
			}
			if got := persisted.Slug.ValueString(); got != testCase.wantSlug {
				t.Errorf("Persisted slug = %q, want %q", got, testCase.wantSlug)
			}
			// The pipeline is only archived by the mutation at the end of Update, so every case here
			// leaves it unarchived no matter what the plan asked for.
			if persisted.Archived.ValueBool() {
				t.Error("Persisted archived = true, want false: the archive mutation never ran")
			}
		})
	}
}

// Update attaches the new default team before detaching the old one, so both are attached until
// findAndRemoveTeam succeeds. Recording the new team before that point is worse than recording
// nothing: the next plan compares the config against it, finds no diff, and never retries the
// detach, while Read only checks that the recorded team is still attached and so never notices the
// old one. The old team keeps MANAGE_BUILD_AND_READ on the pipeline with nothing left to show it.
func TestPipelineUpdateKeepsThePreviousDefaultTeamWhenTheDetachFails(t *testing.T) {
	t.Parallel()

	const (
		previousTeamID = "team-id"
		newTeamID      = "new-team-id"
	)

	server, requests := newRetryStub(t,
		// pipelineUpdate applies.
		stubResponse{status: http.StatusOK, body: `{"data":{"pipelineUpdate":{"pipeline":{
			"id": "pipeline-id", "pipelineUuid": "pipeline-uuid", "name": "p", "slug": "p",
			"defaultBranch": "main", "description": "", "archived": false,
			"repository": {"url": "git@github.com:org/repo.git"}, "steps": {"yaml": "steps: []"},
			"tags": [], "teams": {"edges": []}
		}}}}`},
		// The new team is attached.
		stubResponse{status: http.StatusOK, body: fmt.Sprintf(`{"data":{"teamPipelineCreate":{"teamPipelineEdge":{"node":{
			"id": "team-pipeline-id", "uuid": "team-pipeline-uuid", "pipelineAccessLevel": "MANAGE_BUILD_AND_READ",
			"team": {"id": %q}, "pipeline": {"id": "pipeline-id"}
		}}}}}`, newTeamID)},
		// Detaching the old one does not. A GraphQL error in a 200 body is non-retryable.
		stubResponse{status: http.StatusOK, body: `{"errors":[{"message":"team lookup exploded"}]}`},
	)
	defer server.Close()

	p := &pipelineResource{client: newRetryTestClient(t, server.URL, 0, time.Millisecond)}

	ctx := t.Context()
	var schemaResp fwresource.SchemaResponse
	p.Schema(ctx, fwresource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics = %v", schemaResp.Diagnostics)
	}
	schema := schemaResp.Schema

	prior := nullObjectWith(ctx, t, schema.Type(), map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "pipeline-id"),
		"name":            tftypes.NewValue(tftypes.String, "p"),
		"slug":            tftypes.NewValue(tftypes.String, "p"),
		"repository":      tftypes.NewValue(tftypes.String, "git@github.com:org/repo.git"),
		"steps":           tftypes.NewValue(tftypes.String, "steps: []"),
		"default_team_id": tftypes.NewValue(tftypes.String, previousTeamID),
		"archived":        tftypes.NewValue(tftypes.Bool, false),
	})
	planned := nullObjectWith(ctx, t, schema.Type(), map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "pipeline-id"),
		"name":            tftypes.NewValue(tftypes.String, "p"),
		"repository":      tftypes.NewValue(tftypes.String, "git@github.com:org/repo.git"),
		"steps":           tftypes.NewValue(tftypes.String, "steps: []"),
		"default_team_id": tftypes.NewValue(tftypes.String, newTeamID),
		"archived":        tftypes.NewValue(tftypes.Bool, false),
	})

	req := fwresource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: schema, Raw: planned},
		State:  tfsdk.State{Schema: schema, Raw: prior},
		Config: tfsdk.Config{Schema: schema, Raw: planned},
	}
	resp := fwresource.UpdateResponse{State: tfsdk.State{Schema: schema, Raw: prior}}

	p.Update(ctx, req, &resp)

	if got := requests.Load(); got < 3 {
		t.Fatalf("Made %d requests, want the new team to have been attached before the detach failed", got)
	}
	if !diagnosticsContain(resp.Diagnostics, "Could not remove previous default team") {
		t.Fatalf("Update() diagnostics = %v, want the detach failure reported", resp.Diagnostics)
	}

	var persisted pipelineResourceModel
	if diags := resp.State.Get(ctx, &persisted); diags.HasError() {
		t.Fatalf("Reading the persisted state = %v", diags)
	}
	if got := persisted.Name.ValueString(); got != "p" {
		t.Errorf("Persisted name = %q, want %q: the pipeline mutation still applied", got, "p")
	}
	if got := persisted.DefaultTeamId.ValueString(); got != previousTeamID {
		t.Errorf("Persisted default_team_id = %q, want %q: the old team is still attached, so the next plan has to retry the detach", got, previousTeamID)
	}
}

// Unarchiving is the first mutation Update applies, and the pipeline mutation after it can still
// fail. Returning without recording the unarchive leaves state calling the pipeline archived while
// it is live and accepting builds, and under -refresh=false nothing corrects that before the next
// plan re-runs the unarchive against a pipeline that is no longer archived.
func TestPipelineUpdatePersistsTheUnarchiveWhenTheMutationFails(t *testing.T) {
	t.Parallel()

	server, requests := newRetryStub(t,
		// pipelineUnarchive applies.
		stubResponse{status: http.StatusOK, body: `{"data":{"pipelineUnarchive":{"clientMutationId":""}}}`},
		// updatePipeline does not. A GraphQL error in a 200 body is non-retryable.
		stubResponse{status: http.StatusOK, body: `{"errors":[{"message":"pipeline update exploded"}]}`},
	)
	defer server.Close()

	p := &pipelineResource{client: newRetryTestClient(t, server.URL, 0, time.Millisecond)}

	ctx := t.Context()
	var schemaResp fwresource.SchemaResponse
	p.Schema(ctx, fwresource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics = %v", schemaResp.Diagnostics)
	}
	schema := schemaResp.Schema

	shared := map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "pipeline-id"),
		"name":       tftypes.NewValue(tftypes.String, "p"),
		"repository": tftypes.NewValue(tftypes.String, "git@github.com:org/repo.git"),
		"steps":      tftypes.NewValue(tftypes.String, "steps: []"),
	}
	prior := map[string]tftypes.Value{
		"slug":     tftypes.NewValue(tftypes.String, "p"),
		"archived": tftypes.NewValue(tftypes.Bool, true),
	}
	planned := map[string]tftypes.Value{"archived": tftypes.NewValue(tftypes.Bool, false)}
	maps.Copy(prior, shared)
	maps.Copy(planned, shared)

	priorRaw := nullObjectWith(ctx, t, schema.Type(), prior)
	req := fwresource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: schema, Raw: nullObjectWith(ctx, t, schema.Type(), planned)},
		State:  tfsdk.State{Schema: schema, Raw: priorRaw},
		Config: tfsdk.Config{Schema: schema, Raw: nullObjectWith(ctx, t, schema.Type(), planned)},
	}
	resp := fwresource.UpdateResponse{State: tfsdk.State{Schema: schema, Raw: priorRaw}}

	p.Update(ctx, req, &resp)

	if got := requests.Load(); got < 2 {
		t.Fatalf("Made %d requests, want the unarchive to have applied before the failure", got)
	}
	if !diagnosticsContain(resp.Diagnostics, "Unable to update Pipeline") {
		t.Fatalf("Update() diagnostics = %v, want the pipeline mutation failure reported", resp.Diagnostics)
	}

	var persisted pipelineResourceModel
	if diags := resp.State.Get(ctx, &persisted); diags.HasError() {
		t.Fatalf("Reading the persisted state = %v", diags)
	}
	if persisted.Archived.ValueBool() {
		t.Error("Persisted archived = true, want false: the unarchive applied, so state has to say so")
	}
	if got := persisted.Slug.ValueString(); got != "p" {
		t.Errorf("Persisted slug = %q, want %q: the rename never ran", got, "p")
	}
}
