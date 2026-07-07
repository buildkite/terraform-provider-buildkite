package buildkite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"testing"

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

func TestMapProviderSettingsFromGraphQLGitHub(t *testing.T) {
	triggerMode := "code"
	enabled := true
	disabled := false
	matchMode := CommandWordMatchModeExact

	repo := RepositoryProviderSettingsFields{
		Provider: &RepositoryProviderSettingsFieldsProviderRepositoryProviderGithub{
			Settings: RepositoryProviderSettingsFieldsProviderRepositoryProviderGithubSettingsRepositoryProviderGitHubSettings{
				TriggerMode:              &triggerMode,
				BuildPullRequests:        &enabled,
				BuildBranches:            &disabled,
				IssueCommentMatchMode:    &matchMode,
				UseStepKeyAsCommitStatus: &enabled,
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
	if got.BuildBranches.ValueBool() {
		t.Fatal("build_branches: expected false")
	}
	// CommandWordMatchMode enum (EXACT) must be lowercased to match the schema validator.
	if got.IssueCommentMatchMode.ValueString() != "exact" {
		t.Fatalf("issue_comment_match_mode: expected \"exact\", got %q", got.IssueCommentMatchMode.ValueString())
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
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_pull_request_merge_commits", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.build_issue_comment_created", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.issue_comment_command_word", "ci-force-rerun"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.issue_comment_match_mode", "exact"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.use_step_key_as_commit_status", "true"),
					),
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
