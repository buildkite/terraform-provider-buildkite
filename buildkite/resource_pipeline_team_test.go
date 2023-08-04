package buildkite

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// Confirm we can add and remove a team pipeline resource with the default access level
func TestAccPipelineTeam_add_remove_default_access(t *testing.T) {
	var tp pipelineTeamResourceModel

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testCheckPipelineTeamResourceRemoved,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineTeamConfigBasic("READ_ONLY"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the team pipeline exists in the buildkite API
					testAccCheckPipelineTeamExists("buildkite_pipeline_team.test", &tp),
					// Confirm the team pipeline has the correct values in Buildkite's system
					testAccCheckPipelineTeamRemoteValues("READ_ONLY", &tp),
					// Confirm the team pipeline has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_pipeline_team.test", "access_level", "READ_ONLY"),
				),
			},
		},
	})
}

func TestAccPipelineTeam_add_remove_non_default_access(t *testing.T) {
	var tp pipelineTeamResourceModel

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testCheckPipelineTeamResourceRemoved,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineTeamConfigBasic("BUILD_AND_READ"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the team pipeline exists in the buildkite API
					testAccCheckPipelineTeamExists("buildkite_pipeline_team.test", &tp),
					// Confirm the team pipeline has the correct values in Buildkite's system
					testAccCheckPipelineTeamRemoteValues("BUILD_AND_READ", &tp),
					// Confirm the team pipeline has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_pipeline_team.test", "access_level", "BUILD_AND_READ"),
				),
			},
		},
	})
}

// Confirm that this resource can be imported
func TestAccPipelineTeam_import(t *testing.T) {
	var tp pipelineTeamResourceModel

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testCheckPipelineTeamResourceRemoved,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineTeamConfigBasic("READ_ONLY"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the team pipeline exists in the buildkite API
					testAccCheckPipelineTeamExists("buildkite_pipeline_team.test", &tp),
					// Confirm the team has the correct values in Buildkite's system
					resource.TestCheckResourceAttr("buildkite_pipeline_team.test", "access_level", "READ_ONLY"),
				),
			},
			{
				// re-import the resource and confirm they match
				ResourceName:      "buildkite_pipeline_team.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccPipelineTeamConfigBasic(accessLevel string) string {
	config := `
		resource "buildkite_pipeline" "test-pipeline" {
			name = "acceptance testing pipeline"
		    repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
			steps = ""
		}

		resource "buildkite_team" "test-team" {
			name = "acceptance testing team" 
			privacy = "VISIBLE"
			default_team = true 
			default_member_role = "MEMBER"
		}
		resource "buildkite_pipeline_team" "test" {
		    access_level = "%s"
			team_id = buildkite_team.test-team.id
			pipeline_id = buildkite_pipeline.test-pipeline.id 
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

		if pipelineTeamNode, ok := apiResponse.GetNode().(*getNodeNodePipelineTeam); ok {
			if pipelineTeamNode == nil {
				return fmt.Errorf("Error getting team pipeline: nil response")
			}
			updatePipelineTeamResourceState(tp, *pipelineTeamNode)
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

		if pipelineTeamNode, ok := apiResponse.GetNode().(*getNodeNodePipelineTeam); ok {
			if pipelineTeamNode != nil {
				return fmt.Errorf("Team pipeline still exists")
			}
		}
	}
	return nil
}

func testAccCheckPipelineTeamRemoteValues(accessLevel string, tp *pipelineTeamResourceModel) resource.TestCheckFunc {
	return func(s *terraform.State) error {

		if string(tp.AccessLevel.ValueString()) != accessLevel {
			return fmt.Errorf("remote team pipeline access level (%s) doesn't match expected value (%s)", tp.AccessLevel.ValueString(), accessLevel)
		}
		return nil
	}
}
