package buildkite

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccBuildkitePipeline(t *testing.T) {
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
			Steps: []resource.TestStep{
				{
					Config: config,
					Check: resource.ComposeAggregateTestCheckFunc(
						// check computed values get set
						resource.TestCheckResourceAttrSet("buildkite_pipeline.pipeline", "badge_url"),
						resource.TestCheckResourceAttrSet("buildkite_pipeline.pipeline", "id"),
						resource.TestCheckResourceAttrSet("buildkite_pipeline.pipeline", "slug"),
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
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "steps", defaultSteps),
						// check lists are empty
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "tags.#", "0"),
						resource.TestCheckNoResourceAttr("buildkite_pipeline.pipeline", "tags.#"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "team.#", "0"),
						resource.TestCheckNoResourceAttr("buildkite_pipeline.pipeline", "team.#"),
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

	t.Run("create pipeline with empty attributes", func(t *testing.T) {
		var pipeline *getPipelinePipeline
		pipelineName := acctest.RandString(12)
		config := fmt.Sprintf(`
			resource "buildkite_pipeline" "pipeline" {
				name = "%s"
				repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
				tags = []
				provider_settings {}
			}
		`, pipelineName)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
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
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.#", "1"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.0.trigger_mode", ""),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.0.build_pull_requests", "false"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.0.skip_pull_request_builds_for_existing_commits", "false"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.0.build_branches", "false"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.0.publish_commit_status", "false"),
					),
				},
			},
		})
	})

	t.Run("create pipeline setting all attributes", func(t *testing.T) {
		pipelineName := acctest.RandString(12)
		teamName := acctest.RandString(12)
		clusterName := acctest.RandString(12)
		config := fmt.Sprintf(`
			resource "buildkite_team" "team" {
				name = "pipeline test %s"
				default_team = false
				default_member_role = "MEMBER"
				privacy = "VISIBLE"
			}
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
				team {
					slug = buildkite_team.team.slug
					access_level = "READ_ONLY"
				}
			}
		`, teamName, clusterName, pipelineName)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: config,
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttrPair("buildkite_pipeline.pipeline", "cluster_id", "buildkite_cluster.cluster", "id"),
						resource.TestCheckResourceAttrPair("buildkite_pipeline.pipeline", "team.0.team_id", "buildkite_team.team", "id"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "tags.0", "llama"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "team.0.access_level", "READ_ONLY"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "allow_rebuilds", "false"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "cancel_intermediate_builds", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "cancel_intermediate_builds_branch_filter", "!main"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "branch_configuration", "main"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "default_branch", "main"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "default_timeout_in_minutes", "20"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "maximum_timeout_in_minutes", "30"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "description", "terraform test"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "skip_intermediate_builds", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "skip_intermediate_builds_branch_filter", "!main"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.0.trigger_mode", "code"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.0.build_pull_requests", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.0.skip_builds_for_existing_commits", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.0.build_branches", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.0.build_tags", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.0.build_pull_request_ready_for_review", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.0.cancel_deleted_branch_builds", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.0.filter_enabled", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.0.filter_condition", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.0.publish_commit_status", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.0.publish_blocked_as_pending", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.0.publish_commit_status_per_step", "true"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.0.separate_pull_request_statuses", "true"),
					),
				},
			},
		})
	})

	t.Run("update pipeline setting all attributes", func(t *testing.T) {
		var pipeline getPipelinePipeline
		pipelineName := acctest.RandString(12)
		teamName := acctest.RandString(12)
		clusterName := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
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
						resource "buildkite_team" "team" {
							name = "pipeline test %s"
							default_team = false
							default_member_role = "MEMBER"
							privacy = "VISIBLE"
						}
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
							team {
								slug = buildkite_team.team.slug
								access_level = "READ_ONLY"
							}
						}
					`, teamName, clusterName, pipelineName),
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
						resource.TestCheckResourceAttrPair("buildkite_pipeline.pipeline", "team.0.team_id", "buildkite_team.team", "id"),
						aggregateRemoteCheck(&pipeline),
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
			CheckDestroy: func(s *terraform.State) error {
				resp, err := getPipeline(context.Background(), genqlientGraphql, fmt.Sprintf("%s/%s", getenv("BUILDKITE_ORGANIZATION_SLUG"), pipelineName))
				if resp.Pipeline.Name == pipelineName {
					return fmt.Errorf("Pipeline still exists: %s", pipelineName)
				}
				return err
			},
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
			CheckDestroy: func(s *terraform.State) error {
				resp, err := getPipeline(context.Background(), genqlientGraphql, fmt.Sprintf("%s/%s", getenv("BUILDKITE_ORGANIZATION_SLUG"), pipelineName))
				if err != nil {
					return err
				}
				if resp.Pipeline.Name == pipelineName {
					return fmt.Errorf("Pipeline still exists: %s", pipelineName)
				}
				return err
			},
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

	t.Run("team added to pipeline is ignored", func(t *testing.T) {
		var teamID string
		var pipeline getPipelinePipeline
		pipelineName := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy: func(s *terraform.State) error {
				// if team created in the tests still exists,it must be deleted
				_, err := getNode(context.Background(), genqlientGraphql, teamID)
				if err != nil {
					return err
				}
				_, err = teamDelete(context.Background(), genqlientGraphql, teamID)
				return err
			},
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(`
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"

						}
					`, pipelineName),
					Check: resource.ComposeTestCheckFunc(
						func(s *terraform.State) error {
							slug := fmt.Sprintf("%s/%s", getenv("BUILDKITE_ORGANIZATION_SLUG"), pipelineName)
							resp, err := getPipeline(context.Background(), genqlientGraphql, slug)
							pipeline = resp.Pipeline
							return err
						},
						func(s *terraform.State) error {
							// add new team to pipeline
							team, err := teamCreate(context.Background(), genqlientGraphql, organizationID, fmt.Sprintf("pipeline adhoc team %s", acctest.RandString(6)), nil, "VISIBLE", false, "MEMBER", false)
							teamID = team.TeamCreate.TeamEdge.Node.Id
							if err != nil {
								return err
							}
							_, err = teamPipelineCreate(context.Background(), genqlientGraphql, teamID, string(pipeline.Id), PipelineAccessLevelsBuildAndRead)
							if err != nil {
								return err
							}
							return nil
						},
					),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction("buildkite_pipeline.pipeline", plancheck.ResourceActionNoop),
						},
					},
				},
			},
		})
	})

	t.Run("team access changed in api causes terraform to update", func(t *testing.T) {
		var pipeline getPipelinePipeline
		pipelineName := acctest.RandString(12)
		teamName := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(`
						resource "buildkite_team" "team" {
							name = "pipeline team test %s"
							default_team = false
							default_member_role = "MEMBER"
							privacy = "VISIBLE"
						}
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
							team {
								slug = buildkite_team.team.slug
								access_level = "READ_ONLY"
							}
						}
					`, teamName, pipelineName),
					Check: resource.ComposeTestCheckFunc(
						func(s *terraform.State) error {
							slug := fmt.Sprintf("%s/%s", getenv("BUILDKITE_ORGANIZATION_SLUG"), pipelineName)
							resp, err := getPipeline(context.Background(), genqlientGraphql, slug)
							pipeline = resp.Pipeline
							return err
						},
						func(s *terraform.State) error {
							// change team access level
							_, err := teamPipelineUpdate(context.Background(), genqlientGraphql, pipeline.Teams.Edges[0].Node.Id, PipelineAccessLevelsBuildAndRead)
							if err != nil {
								return err
							}
							return nil
						},
					),
					ExpectNonEmptyPlan: true,
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction("buildkite_pipeline.pipeline", plancheck.ResourceActionUpdate),
						},
					},
				},
			},
		})
	})

	t.Run("team removed in api causes terraform to update", func(t *testing.T) {
		var pipeline getPipelinePipeline
		pipelineName := acctest.RandString(12)
		teamName := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(`
						resource "buildkite_team" "team" {
							name = "pipeline team test %s"
							default_team = false
							default_member_role = "MEMBER"
							privacy = "VISIBLE"
						}
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
							team {
								slug = buildkite_team.team.slug
								access_level = "READ_ONLY"
							}
						}
					`, teamName, pipelineName),
					Check: resource.ComposeTestCheckFunc(
						func(s *terraform.State) error {
							slug := fmt.Sprintf("%s/%s", getenv("BUILDKITE_ORGANIZATION_SLUG"), pipelineName)
							resp, err := getPipeline(context.Background(), genqlientGraphql, slug)
							pipeline = resp.Pipeline
							return err
						},
						func(s *terraform.State) error {
							// change team access level
							_, err := teamPipelineDelete(context.Background(), genqlientGraphql, pipeline.Teams.Edges[0].Node.Id)
							if err != nil {
								return err
							}
							return nil
						},
					),
					ExpectNonEmptyPlan: true,
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction("buildkite_pipeline.pipeline", plancheck.ResourceActionUpdate),
						},
					},
				},
			},
		})
	})

	t.Run("updating provider maintains teams", func(t *testing.T) {
		pipelineName := acctest.RandString(12)
		teamName := acctest.RandString(12)
		config := fmt.Sprintf(`
			resource "buildkite_team" "team" {
				name = "pipeline team test %s"
				default_team = false
				default_member_role = "MEMBER"
				privacy = "VISIBLE"
			}
			resource "buildkite_pipeline" "pipeline" {
				name = "%s"
				repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
				team {
					slug = buildkite_team.team.slug
					access_level = "BUILD_AND_READ"
				}
			}
		`, teamName, pipelineName)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck: func() { testAccPreCheck(t) },
			Steps: []resource.TestStep{
				{
					// create a pipeline and link a team using the old provider
					Config: config,
					ExternalProviders: map[string]resource.ExternalProvider{
						"buildkite": {
							Source:            "registry.terraform.io/buildkite/buildkite",
							VersionConstraint: "0.23.0",
						},
					},
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "team.#", "1"),
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "provider_settings.#", "1"),
					),
				},
				{
					// now when using the new provider, we expect teams to still be 1 and no change to be made
					Config:                   config,
					ProtoV6ProviderFactories: protoV6ProviderFactories(),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "team.#", "1"),
						resource.TestCheckNoResourceAttr("buildkite_pipeline.pipeline", "provider_settings.#"),
					),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PreApply: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction("buildkite_pipeline.pipeline", plancheck.ResourceActionUpdate),
						},
					},
				},
			},
		})
	})
}
