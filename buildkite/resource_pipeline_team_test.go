package buildkite

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccBuildkitePipelineTeam(t *testing.T) {
	t.Run("pipeline team can be created", func(t *testing.T) {
		var tp pipelineTeamResourceModel
		teamName := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testCheckPipelineTeamResourceRemoved,
			Steps: []resource.TestStep{
				{
					Config: testAccPipelineTeamConfigBasic(teamName, "READ_ONLY"),
					Check: resource.ComposeAggregateTestCheckFunc(
						// Confirm the test resource team & team pipeline exists in the Buildkite API
						testAccCheckPipelineTeamExists("buildkite_pipeline_team.pipelineteam", &tp),
						// Confirm the team pipeline has the correct values in Buildkite's system
						testAccCheckPipelineTeamRemoteValues("READ_ONLY", &tp),
						// Confirm the team pipeline has the correct values in terraform state
						resource.TestCheckResourceAttrPair("buildkite_pipeline_team.pipelineteam", "team_id", "buildkite_team.acc_test_team", "id"),
						resource.TestCheckResourceAttrPair("buildkite_pipeline_team.pipelineteam", "pipeline_id", "buildkite_pipeline.acc_test_pipeline", "id"),
						resource.TestCheckResourceAttr("buildkite_pipeline_team.pipelineteam", "access_level", "READ_ONLY"),
					),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{
							plancheck.ExpectEmptyPlan(),
							plancheck.ExpectResourceAction("buildkite_pipeline.acc_test_pipeline", plancheck.ResourceActionNoop),
						},
					},
				},
				{
					ResourceName:      "buildkite_pipeline_team.pipelineteam",
					ImportState:       true,
					ImportStateId:     tp.Id.ValueString(),
					ImportStateVerify: true,
				},
			},
		})
	})

	t.Run("pipeline team can be updated", func(t *testing.T) {
		var tp pipelineTeamResourceModel
		teamName := acctest.RandString(12)
		teamNameNew := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testCheckPipelineTeamResourceRemoved,
			Steps: []resource.TestStep{
				{
					Config: testAccPipelineTeamConfigBasic(teamName, "READ_ONLY"),
					Check: resource.ComposeAggregateTestCheckFunc(
						// Confirm the test resource team & team pipeline exists in the Buildkite API
						testAccCheckPipelineTeamExists("buildkite_pipeline_team.pipelineteam", &tp),
						// Confirm the team has the correct values in Buildkite's system
						testAccCheckPipelineTeamRemoteValues("READ_ONLY", &tp),
						// Confirm the team pipeline has the correct values in terraform state
						resource.TestCheckResourceAttrPair("buildkite_pipeline_team.pipelineteam", "team_id", "buildkite_team.acc_test_team", "id"),
						resource.TestCheckResourceAttrPair("buildkite_pipeline_team.pipelineteam", "pipeline_id", "buildkite_pipeline.acc_test_pipeline", "id"),
						resource.TestCheckResourceAttr("buildkite_pipeline_team.pipelineteam", "access_level", "READ_ONLY"),
					),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{
							plancheck.ExpectEmptyPlan(),
							plancheck.ExpectResourceAction("buildkite_pipeline.acc_test_pipeline", plancheck.ResourceActionNoop),
						},
					},
				},
				{
					Config: testAccPipelineTeamConfigBasic(teamNameNew, "MANAGE_BUILD_AND_READ"),
					Check: resource.ComposeAggregateTestCheckFunc(
						// Confirm the test resource team & team pipeline exists in the Buildkite API
						testAccCheckPipelineTeamExists("buildkite_pipeline_team.pipelineteam", &tp),
						// Confirm the team has the correct values in Buildkite's system
						testAccCheckPipelineTeamRemoteValues("MANAGE_BUILD_AND_READ", &tp),
						// Confirm the team pipeline has the correct values in terraform state
						resource.TestCheckResourceAttrPair("buildkite_pipeline_team.pipelineteam", "team_id", "buildkite_team.acc_test_team", "id"),
						resource.TestCheckResourceAttrPair("buildkite_pipeline_team.pipelineteam", "pipeline_id", "buildkite_pipeline.acc_test_pipeline", "id"),
						resource.TestCheckResourceAttr("buildkite_pipeline_team.pipelineteam", "access_level", "MANAGE_BUILD_AND_READ"),
					),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{
							plancheck.ExpectEmptyPlan(),
							plancheck.ExpectResourceAction("buildkite_pipeline.acc_test_pipeline", plancheck.ResourceActionNoop),
						},
					},
				},
			},
		})
	})

	t.Run("pipeline team is recreated if removed", func(t *testing.T) {
		teamName := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: testAccPipelineTeamConfigBasic(teamName, "READ_ONLY"),
					Check: func(s *terraform.State) error {
						pipelineTeam := s.RootModule().Resources["buildkite_pipeline_team.pipelineteam"]
						_, err := deleteTeamPipeline(context.Background(),
							genqlientGraphql,
							pipelineTeam.Primary.ID)
						return err
					},
					ExpectNonEmptyPlan: true,
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction("buildkite_pipeline_team.pipelineteam", plancheck.ResourceActionCreate),
						},
					},
				},
			},
		})
	})
}

func testAccPipelineTeamConfigBasic(teamName string, accessLevel string) string {
	config := `
	resource "buildkite_pipeline" "acc_test_pipeline" {
	    name = "acctest pipeline %s"
	    repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
	    steps = "steps:\n- label: ':pipeline: Pipeline Upload'\n  command: buildkite-agent pipeline upload"
		provider_settings = {}
	}

	resource "buildkite_team" "acc_test_team" {
		name = "acctest team %s"
		privacy = "VISIBLE"
		default_team = true
		default_member_role = "MEMBER"
		members_can_create_pipelines = true
	}

	resource "buildkite_pipeline_team" "pipelineteam" {
		access_level = "%s"
		team_id = buildkite_team.acc_test_team.id
		pipeline_id = buildkite_pipeline.acc_test_pipeline.id
	}
	`
	return fmt.Sprintf(config, teamName, teamName, accessLevel)
}

func testAccCheckPipelineTeamExists(resourceName string, tp *pipelineTeamResourceModel) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		resourceState, ok := s.RootModule().Resources[resourceName]

		if !ok {
			return fmt.Errorf("Not found in state: %s", resourceName)
		}

		if resourceState.Primary.ID == "" {
			return fmt.Errorf("No ID is set in state")
		}

		apiResponse, err := getNode(context.Background(), genqlientGraphql, resourceState.Primary.ID)
		if err != nil {
			return fmt.Errorf("Error fetching team pipeline from graphql API: %v", err)
		}

		if pipelineTeamNode, ok := apiResponse.GetNode().(*getNodeNodeTeamPipeline); ok {
			if pipelineTeamNode == nil {
				return fmt.Errorf("Error getting team pipeline: nil response")
			}
			updateTeamPipelineResourceState(tp, *pipelineTeamNode)
		}

		return nil
	}
}

// verify the team pipeline has been removed
func testCheckPipelineTeamResourceRemoved(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_pipeline_team" {
			continue
		}

		apiResponse, err := getNode(context.Background(), genqlientGraphql, rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("Error fetching team pipeline from graphql API: %v", err)
		}

		if pipelineTeamNode, ok := apiResponse.GetNode().(*getNodeNodeTeamPipeline); ok {
			if pipelineTeamNode != nil {
				return fmt.Errorf("Team pipeline still exists")
			}
		}
	}
	return nil
}

func testAccCheckPipelineTeamRemoteValues(accessLevel string, tp *pipelineTeamResourceModel) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if tp.AccessLevel.ValueString() != accessLevel {
			return fmt.Errorf("remote team pipeline access level (%s) doesn't match expected value (%s)", tp.AccessLevel.ValueString(), accessLevel)
		}
		return nil
	}
}
