package buildkite

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// Confirm we can add and remove a team pipeline resource with the default access level
func TestAccPipelineTeam_AddRemoveWithDefaultsAccess(t *testing.T) {
	var tp pipelineTeamResourceModel
	teamName := acctest.RandString(12)
	t.Parallel()

	resource.Test(t, resource.TestCase{
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
					resource.TestCheckResourceAttrSet("buildkite_pipeline_team.pipelineteam", "team_id"),
					resource.TestCheckResourceAttrSet("buildkite_pipeline_team.pipelineteam", "pipeline_id"),
					resource.TestCheckResourceAttr("buildkite_pipeline_team.pipelineteam", "access_level", "READ_ONLY"),
				),
			},
		},
	})
}

func TestAccPipelineTeam_AddRemoveWithNonDefaultAccess(t *testing.T) {
	var tp pipelineTeamResourceModel
	teamName := acctest.RandString(12)
	t.Parallel()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testCheckPipelineTeamResourceRemoved,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineTeamConfigBasic(teamName, "BUILD_AND_READ"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the test resource team & team pipeline exists in the Buildkite API
					testAccCheckPipelineTeamExists("buildkite_pipeline_team.pipelineteam", &tp),
					// Confirm the team pipeline has the correct values in Buildkite's system
					testAccCheckPipelineTeamRemoteValues("BUILD_AND_READ", &tp),
					// Confirm the team pipeline has the correct values in terraform state
					resource.TestCheckResourceAttrSet("buildkite_pipeline_team.pipelineteam", "team_id"),
					resource.TestCheckResourceAttrSet("buildkite_pipeline_team.pipelineteam", "pipeline_id"),
					resource.TestCheckResourceAttr("buildkite_pipeline_team.pipelineteam", "access_level", "BUILD_AND_READ"),
				),
			},
		},
	})
}

func TestAccPipelineTeam_Update(t *testing.T) {
	var tp pipelineTeamResourceModel
	t.Parallel()
	t.Skip("Skipping until we can figure out how to update the access level of a team pipeline resource")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testCheckPipelineTeamResourceRemoved,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineTeamConfigUpdateBasic("READ_ONLY"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the test resource team & team pipeline exists in the Buildkite API
					testAccCheckPipelineTeamExists("buildkite_pipeline_team.pipelineteam", &tp),
					// Confirm the team has the correct values in Buildkite's system
					testAccCheckPipelineTeamRemoteValues("READ_ONLY", &tp),
					// Confirm the team pipeline has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_pipeline_team.pipelineteam", "access_level", "READ_ONLY"),
				),
			},
			{
				Config: testAccPipelineTeamConfigUpdateBasic("MANAGE_BUILD_AND_READ"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the test resource team & team pipeline exists in the Buildkite API
					testAccCheckPipelineTeamExists("buildkite_pipeline_team.pipelineteam", &tp),
					// Confirm the team has the correct values in Buildkite's system
					testAccCheckPipelineTeamRemoteValues("MANAGE_BUILD_AND_READ", &tp),
					// Confirm the team pipeline has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_pipeline_team.pipelineteam", "access_level", "MANAGE_BUILD_AND_READ"),
				),
			},
		},
	})
}

// Confirm that this resource can be imported
func TestAccPipelineTeam_Import(t *testing.T) {
	var tp pipelineTeamResourceModel
	teamName := acctest.RandString(12)
	t.Parallel()

	resource.Test(t, resource.TestCase{
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
					resource.TestCheckResourceAttrSet("buildkite_pipeline_team.pipelineteam", "team_id"),
					resource.TestCheckResourceAttrSet("buildkite_pipeline_team.pipelineteam", "pipeline_id"),
					resource.TestCheckResourceAttr("buildkite_pipeline_team.pipelineteam", "access_level", "READ_ONLY"),
				),
			},
			{
				// re-import the resource and confirm they match
				ResourceName:      "buildkite_pipeline_team.pipelineteam",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccPipelineTeamConfigBasic(teamName string, accessLevel string) string {
	config := `
	resource "buildkite_pipeline" "acc_test_pipeline" {
	    name = "acctest pipeline %s"
	    repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
	    steps = "steps:\n- label: ':pipeline: Pipeline Upload'\n  command: buildkite-agent pipeline upload"
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

func testAccPipelineTeamConfigUpdateBasic(accessLevel string) string {
	config := `
	resource "buildkite_pipeline" "acc_test_pipeline" {
	    name = "acctest pipeline update"
	    repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
	    steps = "steps:\n- label: ':pipeline: Pipeline Upload'\n  command: buildkite-agent pipeline upload"
	}

	resource "buildkite_team" "acc_test_team" {
		name = "acctest team update"
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
	return fmt.Sprintf(config, accessLevel)
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

		apiResponse, err := getNode(genqlientGraphql, resourceState.Primary.ID)

		if err != nil {
			return fmt.Errorf("Error fetching team pipeline from graphql API: %v", err)
		}

		if pipelineTeamNode, ok := apiResponse.GetNode().(*getNodeNodeTeamPipeline); ok {
			if pipelineTeamNode == nil {
				return fmt.Errorf("Error getting team pipeline: nil response")
			}
			fmt.Printf("pipeline access level in BK: %s", pipelineTeamNode.PipelineAccessLevel)
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

		apiResponse, err := getNode(genqlientGraphql, rs.Primary.ID)

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
