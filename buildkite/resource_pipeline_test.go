package buildkite

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// Confirm that we can create a new pipeline, and then delete it without error
func TestAccPipeline_add_remove(t *testing.T) {
	var resourcePipeline PipelineNode

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckPipelineResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineConfigBasic("foo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Confirm the pipeline has the correct values in Buildkite's system
					testAccCheckPipelineRemoteValues(&resourcePipeline, "Test Pipeline foo"),
					// Confirm the pipeline has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "name", "Test Pipeline foo"),
				),
			},
		},
	})
}

// Confirm that we can create a new pipeline with a cluster, and then delete it without error
func TestAccPipeline_add_delete_withcluster(t *testing.T) {
	var resourcePipeline PipelineNode

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckPipelineResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineConfigBasicWithCluster("foo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Confirm the pipeline has the correct values in Buildkite's system
					testAccCheckPipelineRemoteValues(&resourcePipeline, "Test Pipeline foo"),
					// Confirm the pipeline has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "name", "Test Pipeline foo"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "cluster_id", "Q2x1c3Rlci0tLTFkNmIxOTg5LTJmYjctNDRlMC04MWYyLTAxYjIxNzQ4MTVkMg=="),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "allow_rebuilds", "true"),
				),
			},
			{
				Config: testAccPipelineConfigBasicWithCluster("bar"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Confirm the pipeline has the correct values in Buildkite's system
					testAccCheckPipelineRemoteValues(&resourcePipeline, "Test Pipeline bar"),
					// Confirm the pipeline has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "name", "Test Pipeline bar"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "cluster_id", "Q2x1c3Rlci0tLTFkNmIxOTg5LTJmYjctNDRlMC04MWYyLTAxYjIxNzQ4MTVkMg=="),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "allow_rebuilds", "true"),
				),
			},
		},
	})
}

// Confirm that we can create a new pipeline with a cluster, and then remove it from the cluster
func TestAccPipeline_add_remove_withcluster(t *testing.T) {
	var resourcePipeline PipelineNode

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckPipelineResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineConfigBasicWithCluster("foo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Confirm the pipeline has the correct values in Buildkite's system
					testAccCheckPipelineRemoteValues(&resourcePipeline, "Test Pipeline foo"),
					// Confirm the pipeline has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "name", "Test Pipeline foo"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "cluster_id", "Q2x1c3Rlci0tLTFkNmIxOTg5LTJmYjctNDRlMC04MWYyLTAxYjIxNzQ4MTVkMg=="),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "allow_rebuilds", "true"),
				),
			},
			{
				Config: testAccPipelineConfigBasicWithTeam("foo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Confirm the pipeline has the correct values in Buildkite's system
					testAccCheckPipelineRemoteValues(&resourcePipeline, "Test Pipeline foo"),
					// Confirm the pipeline has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "name", "Test Pipeline foo"),
					resource.TestCheckNoResourceAttr("buildkite_pipeline.foobar", "cluster_id"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "allow_rebuilds", "true"),
				),
			},
		},
	})
}

// TestAccPipeline_add_remove_withcluster_old_version tests that the provider doesn't get stuffed up when coming from an
// older version to this new version. the first step runs against the published v0.21.1 version and the second step runs
// against the current provider. the configuration is the same between each one so there should be no state/plan change
func TestAccPipeline_add_remove_withcluster_old_version(t *testing.T) {
	var resourcePipeline PipelineNode
	pipelineName := acctest.RandString(12)

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		CheckDestroy: testAccCheckPipelineResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineConfigBasicWithCluster(pipelineName),
				ExternalProviders: map[string]resource.ExternalProvider{
					"buildkite": {
						Source:            "registry.terraform.io/buildkite/buildkite",
						VersionConstraint: "0.21.1",
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Confirm the pipeline has the correct values in Buildkite's system
					testAccCheckPipelineRemoteValues(&resourcePipeline, fmt.Sprintf("Test Pipeline %s", pipelineName)),
					// Confirm the pipeline has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "name", fmt.Sprintf("Test Pipeline %s", pipelineName)),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "cluster_id", "Q2x1c3Rlci0tLTFkNmIxOTg5LTJmYjctNDRlMC04MWYyLTAxYjIxNzQ4MTVkMg=="),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "allow_rebuilds", "true"),
					// check provider settings are present
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.trigger_mode", "code"),
				),
			},
			{
				Config:                   testAccPipelineConfigBasicWithCluster(pipelineName),
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Confirm the pipeline has the correct values in Buildkite's system
					testAccCheckPipelineRemoteValues(&resourcePipeline, fmt.Sprintf("Test Pipeline %s", pipelineName)),
					// Confirm the pipeline has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "name", fmt.Sprintf("Test Pipeline %s", pipelineName)),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "cluster_id", "Q2x1c3Rlci0tLTFkNmIxOTg5LTJmYjctNDRlMC04MWYyLTAxYjIxNzQ4MTVkMg=="),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "allow_rebuilds", "true"),
					// check provider settings are not present
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.#", "0"),
				),
			},
		},
	})
}

// TestAccPipeline_add_remove_withoutcluster_old_version tests that the provider doesn't get stuffed up when coming from an
// older version to this new version. the first step runs against the published v0.21.1 version and the second step runs
// against the current provider. the configuration is the same between each one so there should be no state/plan change
func TestAccPipeline_add_remove_withoutcluster_old_version(t *testing.T) {
	var resourcePipeline PipelineNode
	pipelineName := acctest.RandString(12)

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		CheckDestroy: testAccCheckPipelineResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineConfigBasic(pipelineName),
				ExternalProviders: map[string]resource.ExternalProvider{
					"buildkite": {
						Source:            "registry.terraform.io/buildkite/buildkite",
						VersionConstraint: "0.21.1",
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Confirm the pipeline has the correct values in Buildkite's system
					testAccCheckPipelineRemoteValues(&resourcePipeline, fmt.Sprintf("Test Pipeline %s", pipelineName)),
					// Confirm the pipeline has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "name", fmt.Sprintf("Test Pipeline %s", pipelineName)),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "cluster_id", ""),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "allow_rebuilds", "true"),
					// check provider settings are present
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.trigger_mode", "code"),
				),
			},
			{
				Config:                   testAccPipelineConfigBasic(pipelineName),
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Confirm the pipeline has the correct values in Buildkite's system
					testAccCheckPipelineRemoteValues(&resourcePipeline, fmt.Sprintf("Test Pipeline %s", pipelineName)),
					// Confirm the pipeline has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "name", fmt.Sprintf("Test Pipeline %s", pipelineName)),
					resource.TestCheckNoResourceAttr("buildkite_pipeline.foobar", "cluster_id"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "allow_rebuilds", "true"),
					// check provider settings are not present
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.#", "0"),
				),
			},
		},
	})
}

func TestAccPipeline_team_added(t *testing.T) {
	var resourcePipeline PipelineNode
	var teamID string
	pipelineName := acctest.RandString(12)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		CheckDestroy:             removeTeamAndPipeline(&teamID),
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineConfigBasic(pipelineName),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// check no teams are present
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "team.#", "0"),
					// add a team to the pipeline outside of terraform
					func(s *terraform.State) error {
						team, err := teamCreate(genqlientGraphql, organizationID, acctest.RandString(6), nil, "VISIBLE", false, "MEMBER", false)
						teamID = team.TeamCreate.TeamEdge.Node.Id
						if err != nil {
							return err
						}
						_, err = teamPipelineCreate(genqlientGraphql, teamID, string(resourcePipeline.ID), PipelineAccessLevelsBuildAndRead)
						if err != nil {
							return err
						}
						return nil
					},
				),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccPipeline_team_changed(t *testing.T) {
	var resourcePipeline PipelineNode
	pipelineName := acctest.RandString(12)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		CheckDestroy:             testAccCheckPipelineResourceDestroy,
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineConfigBasicWithTeam(pipelineName),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// check no teams are present
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "team.#", "1"),
					// change the team access level outside of terraform
					func(s *terraform.State) error {
						_, err := teamPipelineUpdate(genqlientGraphql, string(resourcePipeline.Teams.Edges[0].Node.ID), PipelineAccessLevelsReadOnly)
						if err != nil {
							return err
						}
						return nil
					},
				),
				// expect non-empty plan because team access level has changed
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccPipeline_team_removed(t *testing.T) {
	var resourcePipeline PipelineNode
	pipelineName := acctest.RandString(12)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		CheckDestroy:             testAccCheckPipelineResourceDestroy,
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineConfigBasicWithTeam(pipelineName),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// check no teams are present
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "team.#", "1"),
					// remove the team from the api
					func(s *terraform.State) error {
						_, err := teamPipelineDelete(genqlientGraphql, string(resourcePipeline.Teams.Edges[0].Node.ID))
						if err != nil {
							return err
						}
						return nil
					},
				),
				// expect non-empty plan because it should want to link the team again
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccPipeline_add_remove_complex(t *testing.T) {
	var resourcePipeline PipelineNode
	steps := `"steps:\n- command: buildkite-agent pipeline upload\n"`

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckPipelineResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineConfigComplex("bar", steps),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Confirm the pipeline has the correct values in Buildkite's system
					testAccCheckPipelineRemoteValues(&resourcePipeline, "Test Pipeline bar"),
					// Confirm the pipeline has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "name", "Test Pipeline bar"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "repository", "https://github.com/buildkite/terraform-provider-buildkite.git"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "steps", "steps:\n- command: buildkite-agent pipeline upload\n"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "default_branch", "main"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "description", "A test pipeline produced via Terraform"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "branch_configuration", "main"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "skip_intermediate_builds", "true"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "skip_intermediate_builds_branch_filter", "main"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "allow_rebuilds", "true"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "cancel_intermediate_builds", "true"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "cancel_intermediate_builds_branch_filter", "!main"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "tags.0", "test-tag"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "tags.1", "🛫"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.trigger_mode", "code"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.build_branches", "false"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.build_pull_request_forks", "true"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.build_pull_request_ready_for_review", "true"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.build_pull_request_labels_changed", "true"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.build_pull_requests", "false"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.build_tags", "true"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.cancel_deleted_branch_builds", "true"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.filter_enabled", "true"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.filter_condition", "build.pull_request.labels includes \"CI: yes\""),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.prefix_pull_request_fork_branch_names", "true"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.publish_blocked_as_pending", "true"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.publish_commit_status", "true"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.publish_commit_status_per_step", "true"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.pull_request_branch_filter_configuration", "features/*"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.pull_request_branch_filter_enabled", "true"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.separate_pull_request_statuses", "true"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.skip_pull_request_builds_for_existing_commits", "false"),
					resource.TestMatchResourceAttr("buildkite_pipeline.foobar", "webhook_url", regexp.MustCompile(`^https://webhook.buildkite.com/deliver/[a-z0-9]{50}$`)),
					resource.TestMatchResourceAttr("buildkite_pipeline.foobar", "badge_url", regexp.MustCompile(`^https://badge.buildkite.com/[a-z0-9]{50}\.svg$`)),
				),
			},
		},
	})
}

func TestAccPipeline_add_remove_withteams(t *testing.T) {
	var resourcePipeline PipelineNode

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckPipelineResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineConfigBasicWithTeam("foo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Confirm the pipeline has the correct values in Buildkite's system
					testAccCheckPipelineRemoteValues(&resourcePipeline, "Test Pipeline foo"),
					// Confirm the pipeline has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "name", "Test Pipeline foo"),
				),
			},
		},
	})
}

func TestAccPipeline_add_remove_withtimeouts(t *testing.T) {
	var resourcePipeline PipelineNode

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckPipelineResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineConfigBasicWithTimeouts("foo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Confirm the pipeline has the correct values in Buildkite's system
					testAccCheckPipelineRemoteValues(&resourcePipeline, "Test Pipeline foo"),
					// Confirm the pipeline has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "maximum_timeout_in_minutes", "20"),
				),
			},
		},
	})
}

func TestAccPipeline_add_remove_withdefaultsteps(t *testing.T) {
	var resourcePipeline PipelineNode

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckPipelineResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineConfigBasicWithNoSteps("foo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Confirm the pipeline has the correct values in Buildkite's system
					testAccCheckPipelineRemoteValues(&resourcePipeline, "Test Pipeline foo"),
					// Confirm the pipeline has the correct values in Buildkite's system
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "steps", "steps:\n- label: ':pipeline: Pipeline Upload'\n  command: buildkite-agent pipeline upload"),
				),
			},
		},
	})
}

func TestAccPipeline_add_remove_withdefinedsteps(t *testing.T) {
	var resourcePipeline PipelineNode
	steps := `"steps:\n- command: echo 'Hello from the Buildkite Terraform Provider!'\n"`

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckPipelineResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineConfigBasicWithSteps("bar", steps),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Confirm the pipeline has the correct values in Buildkite's system
					testAccCheckPipelineRemoteValues(&resourcePipeline, "Test Pipeline bar"),
					// Confirm the pipeline has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "steps", "steps:\n- command: echo 'Hello from the Buildkite Terraform Provider!'\n"),
				),
			},
		},
	})
}

// Confirm that we can create a new pipeline, and then update the description
func TestAccPipeline_update(t *testing.T) {
	var resourcePipeline PipelineNode

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckPipelineResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineConfigBasic("foo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Quick check to confirm the local state is correct before we update it
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "name", "Test Pipeline foo"),
				),
			},
			{
				Config: testAccPipelineConfigBasic("bar"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Confirm the pipeline has the updated values in Buildkite's system
					testAccCheckPipelineRemoteValues(&resourcePipeline, "Test Pipeline bar"),
					// Confirm the pipeline has the updated values in terraform state
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "name", "Test Pipeline bar"),
				),
			},
		},
	})
}

func TestAccPipeline_update_withteams(t *testing.T) {
	var resourcePipeline PipelineNode

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckPipelineResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineConfigBasicWithTeam("foo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Quick check to confirm the local state is correct before we update it
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "name", "Test Pipeline foo"),
				),
			},
			{
				Config: testAccPipelineConfigBasicWithTeam("bar"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Confirm the pipeline has the updated values in Buildkite's system
					testAccCheckPipelineRemoteValues(&resourcePipeline, "Test Pipeline bar"),
					// Confirm the pipeline has the updated values in terraform state
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "name", "Test Pipeline bar"),
				),
			},
		},
	})
}

func TestAccPipeline_add_team(t *testing.T) {
	var resourcePipeline PipelineNode

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckPipelineResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineConfigBasic("foo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Quick check to confirm the local state is correct before we update it
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "name", "Test Pipeline foo"),
				),
			},
			{
				Config: testAccPipelineConfigBasicWithTeam("foo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Confirm the pipeline has the updated values in Buildkite's system
					testAccCheckPipelineRemoteValues(&resourcePipeline, "Test Pipeline foo"),
					// Confirm the pipeline has the updated values in terraform state
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "name", "Test Pipeline foo"),
				),
			},
		},
	})
}

func TestAccPipeline_remove_team(t *testing.T) {
	var resourcePipeline PipelineNode

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckPipelineResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineConfigBasicWithTeam("foo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Quick check to confirm the local state is correct before we update it
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "name", "Test Pipeline foo"),
				),
			},
			{
				Config: testAccPipelineConfigBasic("foo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Confirm the pipeline has the updated values in Buildkite's system
					testAccCheckPipelineRemoteValues(&resourcePipeline, "Test Pipeline foo"),
					// Confirm the pipeline has the updated values in terraform state
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "name", "Test Pipeline foo"),
				),
			},
		},
	})
}

// Confirm that this resource can be imported
func TestAccPipeline_import(t *testing.T) {
	var resourcePipeline PipelineNode

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckPipelineResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineConfigBasic("foo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Quick check to confirm the local state is correct before we re-import it
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "name", "Test Pipeline foo"),
				),
			},
			{
				// re-import the resource (using the graphql token of the existing resource) and confirm they match
				ResourceName:            "buildkite_pipeline.foobar",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"deletion_protection", "archive_on_delete"},
			},
		},
	})
}

// Confirm that this resource can be removed
func TestAccPipeline_disappears(t *testing.T) {
	var node PipelineNode
	resourceName := "buildkite_pipeline.foobar"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckPipelineResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineConfigBasic("foo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists(resourceName, &node),
					// Ensure its removal from the spec
					testAccCheckPipelineDisappears(resourceName),
				),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// Testing for deletion protection on pipeline

func testAccPipelineDeletionProtectionConfig(name string, deletion_protection bool) string {
	config := `
		resource "buildkite_pipeline" "deletion_test" {
			name = "%s"
			repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
			steps = ""
			deletion_protection = %t
		}
	`
	return fmt.Sprintf(config, name, deletion_protection)
}

func TestAccPipelineDeletionProtection_create(t *testing.T) {
	var node PipelineNode
	resourceName := "buildkite_pipeline.deletion_test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckPipelineResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineDeletionProtectionConfig("this_should_pass", false),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists(resourceName, &node),
					// Ensure deletion_protection is present in the config
					resource.TestCheckResourceAttr(resourceName, "deletion_protection", "false"),
				),
			},
		},
	})
}

func TestAccPipelineDeletionProtection_update(t *testing.T) {
	var node PipelineNode
	resourceName := "buildkite_pipeline.deletion_test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckPipelineResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineDeletionProtectionConfig("deletion_protection_update", true),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists(resourceName, &node),
					// Ensure deletion_protection is present in the config
					resource.TestCheckResourceAttr(resourceName, "deletion_protection", "true"),
				),
			},
			{
				Config: testAccPipelineDeletionProtectionConfig("deletion_protection_update", false),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists(resourceName, &node),
					// Ensure deletion_protection is updated in the config
					resource.TestCheckResourceAttr(resourceName, "deletion_protection", "false"),
				),
			},
		},
	})
}

func TestAccPipelineDeletionProtection_import(t *testing.T) {
	var node PipelineNode
	resourceName := "buildkite_pipeline.deletion_test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckPipelineResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineDeletionProtectionConfig("this_should_pass", false),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists(resourceName, &node),
					// Ensure deletion_protection is present in the config
					resource.TestCheckResourceAttr(resourceName, "name", "this_should_pass"),
				),
			},
			{
				// re-import the resource (using the graphql token of the existing resource) and confirm they match
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"deletion_protection", "archive_on_delete"},
			},
		},
	})
}

func TestAccPipelineBranchConfiguration(t *testing.T) {
	var node PipelineNode
	resourceName := "buildkite_pipeline.branch_config"
	config := `
		resource "buildkite_pipeline" "branch_config" {
			name = "Test Pipeline branch_config"
			repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
	        branch_configuration = "%s"
		}
	`

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckPipelineResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(config, "main"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists(resourceName, &node),
					// Ensure deletion_protection is present in the config
					resource.TestCheckResourceAttr(resourceName, "branch_configuration", "main"),
				),
			},
			{
				Config: fmt.Sprintf(config, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists(resourceName, &node),
					resource.TestCheckResourceAttr(resourceName, "branch_configuration", ""),
					func(s *terraform.State) error {
						if string(node.BranchConfiguration) != "" {
							return errors.New(fmt.Sprintf("remote branch_configuration: \"%s\" is not empty", string(node.BranchConfiguration)))
						}
						return nil
					},
				),
			},
		},
	})
}

// func TestAccPipelineDeletionProtection_fail(t *testing.T) {
// AT THE MOMENT, TESTING THAT DESTROY FAILS ISN'T POSSIBLE
// Closed PR is here: https://github.com/hashicorp/terraform-plugin-sdk/pull/976
// 	resource.Test(t, resource.TestCase{
// 		PreCheck:     func() { testAccPreCheck(t) },
// 		Providers:    testAccProviders,
// 		CheckDestroy: testAccCheckPipelineResourceDestroy,
// 		Steps: []resource.TestStep{
// 			{
// 				Config:      testAccPipelineDeletionProtectionConfig("this_should_fail", true),
// 				ExpectError: regexp.MustCompile("Deletion protection is enabled for pipeline: this_should_fail"),
// 			},
// 		},
// 	})
// }

func testAccCheckPipelineExists(resourceName string, resourcePipeline *PipelineNode) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		resourceState, ok := s.RootModule().Resources[resourceName]

		if !ok {
			return fmt.Errorf("Not found in state: %s", resourceName)
		}

		if resourceState.Primary.ID == "" {
			return fmt.Errorf("No ID is set in state")
		}

		var query struct {
			Node struct {
				Pipeline PipelineNode `graphql:"... on Pipeline"`
			} `graphql:"node(id: $id)"`
		}

		vars := map[string]interface{}{
			"id": resourceState.Primary.ID,
		}

		err := graphqlClient.Query(context.Background(), &query, vars)
		if err != nil {
			return fmt.Errorf("Error fetching pipeline from graphql API: %v", err)
		}

		if string(query.Node.Pipeline.ID) == "" {
			return fmt.Errorf("No pipeline found with graphql id: %s", resourceState.Primary.ID)
		}

		if string(query.Node.Pipeline.Slug) != resourceState.Primary.Attributes["slug"] {
			return fmt.Errorf("Pipeline slug in state doesn't match remote slug")
		}

		if string(query.Node.Pipeline.WebhookURL) != resourceState.Primary.Attributes["webhook_url"] {
			return fmt.Errorf("Pipeline webhook URL in state doesn't match remote webhook URL")
		}

		*resourcePipeline = query.Node.Pipeline

		return nil
	}
}

func testAccCheckPipelineRemoteValues(resourcePipeline *PipelineNode, name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {

		if string(resourcePipeline.Name) != name {
			return fmt.Errorf("remote pipeline name (%s) doesn't match expected value (%s)", resourcePipeline.Name, name)
		}
		return nil
	}
}

func testAccPipelineConfigBasic(name string) string {
	config := `
		resource "buildkite_pipeline" "foobar" {
			name = "Test Pipeline %s"
			repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
			steps = ""
		}
	`
	return fmt.Sprintf(config, name)
}

func testAccPipelineConfigBasicWithTeam(name string) string {
	config := `
		resource "buildkite_pipeline" "foobar" {
			name = "Test Pipeline %s"
			repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
			steps = ""

			team {
				slug = "everyone"
				access_level = "MANAGE_BUILD_AND_READ"
			}
		}
	`
	return fmt.Sprintf(config, name)
}

func testAccPipelineConfigBasicWithCluster(name string) string {
	config := `
		resource "buildkite_pipeline" "foobar" {
			name = "Test Pipeline %s"
			repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id = "Q2x1c3Rlci0tLTFkNmIxOTg5LTJmYjctNDRlMC04MWYyLTAxYjIxNzQ4MTVkMg=="
			allow_rebuilds = true

			team {
				slug = "everyone"
				access_level = "MANAGE_BUILD_AND_READ"
			}
		}
	`
	return fmt.Sprintf(config, name)
}

func testAccPipelineConfigBasicWithTimeouts(name string) string {
	config := `
		resource "buildkite_pipeline" "foobar" {
			name = "Test Pipeline %s"
			repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
			steps = ""

			default_timeout_in_minutes = 10
			maximum_timeout_in_minutes = 20
		}
	`
	return fmt.Sprintf(config, name)
}

func testAccPipelineConfigBasicWithNoSteps(name string) string {
	config := `
		resource "buildkite_pipeline" "foobar" {
		    name = "Test Pipeline %s"
		    repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
		}
	`
	return fmt.Sprintf(config, name)
}

func testAccPipelineConfigBasicWithSteps(name string, steps string) string {
	config := `
		resource "buildkite_pipeline" "foobar" {
		    name = "Test Pipeline %s"
		    repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
		    steps = %s
		}
	`
	return fmt.Sprintf(config, name, steps)
}

func testAccPipelineConfigComplex(name string, steps string) string {
	config := `
        resource "buildkite_pipeline" "foobar" {
            name = "Test Pipeline %s"
			repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
            steps = %s
            default_branch = "main"
            description = "A test pipeline produced via Terraform"
            branch_configuration = "main"
            skip_intermediate_builds = true
            skip_intermediate_builds_branch_filter = "main"
            allow_rebuilds = true
            cancel_intermediate_builds = true
            cancel_intermediate_builds_branch_filter = "!main"
			tags = ["🛫", "test-tag"]
			provider_settings {
				trigger_mode = "code"
				build_branches = false
				build_pull_request_forks = true
				build_pull_request_ready_for_review = true
				build_pull_request_labels_changed = true
				build_pull_requests = false
				build_tags = true
				cancel_deleted_branch_builds = true
				filter_enabled = true
				filter_condition = "build.pull_request.labels includes \"CI: yes\""
				prefix_pull_request_fork_branch_names = true
				publish_blocked_as_pending = true
				publish_commit_status = true
				publish_commit_status_per_step = true
				pull_request_branch_filter_configuration = "features/*"
				pull_request_branch_filter_enabled = true
				separate_pull_request_statuses = true
				skip_pull_request_builds_for_existing_commits = false
			}
        }
	`
	return fmt.Sprintf(config, name, steps)
}

func removeTeamAndPipeline(id *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		_, err := teamDelete(genqlientGraphql, *id)
		if err != nil {
			return err
		}
		return testAccCheckPipelineResourceDestroy(s)
	}
}

// verifies the Pipeline has been destroyed
func testAccCheckPipelineResourceDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_pipeline" {
			continue
		}

		// Try to find the resource remotely
		var query struct {
			Node struct {
				Pipeline PipelineNode `graphql:"... on Pipeline"`
			} `graphql:"node(id: $id)"`
		}

		vars := map[string]interface{}{
			"id": rs.Primary.ID,
		}

		err := graphqlClient.Query(context.Background(), &query, vars)
		if err == nil {
			if string(query.Node.Pipeline.ID) != "" &&
				string(query.Node.Pipeline.ID) == rs.Primary.ID {
				return fmt.Errorf("Pipeline still exists")
			}
		}

		return err
	}

	return nil
}

// testAccCheckPipelineDisappears verifies the Provider has had the resource removed from state
func testAccCheckPipelineDisappears(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		resourceState, ok := s.RootModule().Resources[resourceName]

		if !ok {
			return fmt.Errorf("resource not found: %s", resourceName)
		}

		if resourceState.Primary.ID == "" {
			return fmt.Errorf("resource ID missing: %s", resourceName)
		}

		_, err := deletePipeline(genqlientGraphql, resourceState.Primary.ID)
		return err
	}
}
