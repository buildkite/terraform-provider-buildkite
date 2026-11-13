package buildkite

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccBuildkitePipelineTeam(t *testing.T) {
	t.Parallel()
	t.Run("pipeline team can be updated", func(t *testing.T) {
		var tp pipelineTeamResourceModel
		teamName := testAccResourceName()
		teamNameNew := testAccResourceName()

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

	t.Run("duplicate pipeline team returns helpful error", func(t *testing.T) {
		teamName := testAccResourceName()

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config:      testAccPipelineTeamConfigDuplicate(teamName),
					ExpectError: regexp.MustCompile(`already been added`),
				},
			},
		})
	})

	t.Run("pipeline team is recreated if removed", func(t *testing.T) {
		teamName := testAccResourceName()

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

func TestAccNonAdminPipelineTeam(t *testing.T) {
	pipelineName := testAccResourceName() + "-nonadmin-team"
	teamSlug := testAccFixturePipelineTeamSlug
	baseConfig := testAccPipelineConfigWithTeam(pipelineName, "Pipeline team acceptance test") + fmt.Sprintf(`
		data "buildkite_team" "secondary" {
			slug = %q
		}
	`, teamSlug)
	config := func(accessLevel string) string {
		return baseConfig + fmt.Sprintf(`
			resource "buildkite_pipeline_team" "secondary" {
				pipeline_id = buildkite_pipeline.pipeline.id
				team_id = data.buildkite_team.secondary.id
				access_level = %q
			}
		`, accessLevel)
	}
	var associationID string
	check := func(accessLevel string) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			pipeline := s.RootModule().Resources["buildkite_pipeline.pipeline"]
			ownerID := s.RootModule().Resources["data.buildkite_team.nonadmin"].Primary.ID
			secondaryID := s.RootModule().Resources["data.buildkite_team.secondary"].Primary.ID
			if ownerID == secondaryID {
				return fmt.Errorf("pipeline-team fixture must differ from the pipeline's original Full Access team")
			}
			remote, err := getPipeline(context.Background(), genqlientGraphql,
				fmt.Sprintf("%s/%s", getenv("BUILDKITE_ORGANIZATION_SLUG"), pipeline.Primary.Attributes["slug"]))
			if err != nil {
				return err
			}
			if remote.Pipeline.Id != pipeline.Primary.ID {
				return fmt.Errorf("temporary pipeline identity changed or pipeline is no longer readable")
			}
			var ownerFound, secondaryFound bool
			for _, edge := range remote.Pipeline.Teams.Edges {
				if edge.Node.Team.Id == ownerID {
					ownerFound = edge.Node.AccessLevel == PipelineAccessLevelsManageBuildAndRead
				}
				if edge.Node.Team.Id == secondaryID {
					secondaryFound = true
					if string(edge.Node.AccessLevel) != accessLevel {
						return fmt.Errorf("secondary team access = %q, want %q", edge.Node.AccessLevel, accessLevel)
					}
				}
			}
			if !ownerFound || secondaryFound != (accessLevel != "") {
				return fmt.Errorf("original Full Access team present: %t; secondary team present: %t (expected access %q)", ownerFound, secondaryFound, accessLevel)
			}
			if accessLevel == "" {
				if _, exists := s.RootModule().Resources["buildkite_pipeline_team.secondary"]; exists {
					return fmt.Errorf("removed pipeline-team association remains in Terraform state")
				}
				return nil
			}
			association := s.RootModule().Resources["buildkite_pipeline_team.secondary"]
			if associationID == "" {
				associationID = association.Primary.ID
			}
			return resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("buildkite_pipeline_team.secondary", "id", associationID),
				resource.TestCheckResourceAttr("buildkite_pipeline_team.secondary", "access_level", accessLevel),
				resource.TestCheckResourceAttrPair("buildkite_pipeline_team.secondary", "team_id", "data.buildkite_team.secondary", "id"),
				resource.TestCheckResourceAttrPair("buildkite_pipeline_team.secondary", "pipeline_id", "buildkite_pipeline.pipeline", "id"),
			)(s)
		}
	}

	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			testAccNonAdminPreCheck(t)
			ownerSlug := os.Getenv("BUILDKITE_NONADMIN_TEAM_SLUG")
			if ownerSlug == "" {
				ownerSlug = "everyone"
			}
			if teamSlug == ownerSlug {
				t.Fatal("persistent pipeline-team fixture must differ from the pipeline's original Full Access team")
			}
		},
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckPipelineDestroy,
		Steps: []resource.TestStep{
			{
				Config: config("READ_ONLY"),
				Check:  check("READ_ONLY"),
			},
			{
				Config: config("MANAGE_BUILD_AND_READ"),
				Check:  check("MANAGE_BUILD_AND_READ"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("buildkite_pipeline_team.secondary", plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("buildkite_pipeline.pipeline", plancheck.ResourceActionNoop),
					},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				RefreshState: true,
				Check:        check("MANAGE_BUILD_AND_READ"),
			},
			{
				ResourceName:      "buildkite_pipeline_team.secondary",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "buildkite_pipeline_team.secondary",
				ImportState:       true,
				ImportStateIdFunc: testAccImportIDFromAttributes("buildkite_pipeline.pipeline", "slug", "data.buildkite_team.secondary", "slug"),
				ImportStateVerify: true,
			},
			{
				Config: baseConfig,
				Check:  check(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("buildkite_pipeline_team.secondary", plancheck.ResourceActionDestroy),
						plancheck.ExpectResourceAction("buildkite_pipeline.pipeline", plancheck.ResourceActionNoop),
					},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

func testAccPipelineTeamConfigBasic(teamName string, accessLevel string) string {
	config := `
	resource "buildkite_pipeline" "acc_test_pipeline" {
	    name = "%s-pipeline"
	    repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
	    steps = "steps:\n- label: ':pipeline: Pipeline Upload'\n  command: buildkite-agent pipeline upload"
		provider_settings = {}
	}

	resource "buildkite_team" "acc_test_team" {
		name = "%s-team"
		privacy = "VISIBLE"
		default_team = false
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

func testAccPipelineTeamConfigDuplicate(teamName string) string {
	config := `
	resource "buildkite_pipeline" "acc_test_pipeline" {
	    name = "%s-pipeline"
	    repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
	    steps = "steps:\n- label: ':pipeline: Pipeline Upload'\n  command: buildkite-agent pipeline upload"
		provider_settings = {}
	}

	resource "buildkite_team" "acc_test_team" {
		name = "%s-team"
		privacy = "VISIBLE"
		default_team = false
		default_member_role = "MEMBER"
		members_can_create_pipelines = true
	}

	resource "buildkite_pipeline_team" "first" {
		access_level = "READ_ONLY"
		team_id = buildkite_team.acc_test_team.id
		pipeline_id = buildkite_pipeline.acc_test_pipeline.id
	}

	resource "buildkite_pipeline_team" "duplicate" {
		access_level = "READ_ONLY"
		team_id = buildkite_team.acc_test_team.id
		pipeline_id = buildkite_pipeline.acc_test_pipeline.id
		depends_on = [buildkite_pipeline_team.first]
	}
	`
	return fmt.Sprintf(config, teamName, teamName)
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

		var apiResponse *getNodeResponse
		var err error
		apiResponse, err = getNode(context.Background(), genqlientGraphql, resourceState.Primary.ID)
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

		var apiResponse *getNodeResponse
		var err error
		apiResponse, err = getNode(context.Background(), genqlientGraphql, rs.Primary.ID)
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
