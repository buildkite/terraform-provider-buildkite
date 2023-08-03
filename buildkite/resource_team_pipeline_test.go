package buildkite

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// Confirm we can add and remove a team member
func TestAccTeamPipeline_add_remove(t *testing.T) {
	var tm teamPipelineResourceModel

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testCheckTeamPipelineResourceRemoved,
		Steps: []resource.TestStep{
			{
				Config: testAccTeamPipelineConfigBasic("MEMBER"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the team member exists in the buildkite API
					testAccCheckTeamPipelineExists("buildkite_team_member.test", &tm),
					// Confirm the team member has the correct values in Buildkite's system
					testAccCheckTeamPipelineRemoteValues("MEMBER", &tm),
					// Confirm the team member has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_team_member.test", "role", "MEMBER"),
				),
			},
		},
	})
}

func TestAccTeamPipeline_add_remove_non_default_role(t *testing.T) {
	var tm teamPipelineResourceModel

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testCheckTeamPipelineResourceRemoved,
		Steps: []resource.TestStep{
			{
				Config: testAccTeamPipelineConfigBasic("MAINTAINER"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the team member exists in the buildkite API
					testAccCheckTeamPipelineExists("buildkite_team_member.test", &tm),
					// Confirm the team member has the correct values in Buildkite's system
					testAccCheckTeamPipelineRemoteValues("MAINTAINER", &tm),
					// Confirm the team member has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_team_member.test", "role", "MAINTAINER"),
				),
			},
		},
	})
}

func TestAccTeamPipeline_update(t *testing.T) {
	var tm teamPipelineResourceModel

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testCheckTeamPipelineResourceRemoved,
		Steps: []resource.TestStep{
			{
				Config: testAccTeamPipelineConfigBasic("MEMBER"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the team member exists in the buildkite API
					testAccCheckTeamPipelineExists("buildkite_team_member.test", &tm),
					// Confirm the team has the correct values in Buildkite's system
					testAccCheckTeamPipelineRemoteValues("MEMBER", &tm),
				),
			},
			{
				Config: testAccTeamPipelineConfigBasic("MAINTAINER"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the team member exists in the buildkite API
					testAccCheckTeamPipelineExists("buildkite_team_member.test", &tm),
					// Confirm the team has the correct values in Buildkite's system
					testAccCheckTeamPipelineRemoteValues("MAINTAINER", &tm),
				),
			},
		},
	})
}

// Confirm that this resource can be imported
func TestAccTeamPipeline_import(t *testing.T) {
	var tm teamPipelineResourceModel

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testCheckTeamPipelineResourceRemoved,
		Steps: []resource.TestStep{
			{
				Config: testAccTeamPipelineConfigBasic("MEMBER"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the team member exists in the buildkite API
					testAccCheckTeamPipelineExists("buildkite_team_member.test", &tm),
					// Confirm the team has the correct values in Buildkite's system
					resource.TestCheckResourceAttr("buildkite_team_member.test", "role", "MEMBER"),
				),
			},
			{
				// re-import the resource and confirm they match
				ResourceName:      "buildkite_team_member.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccTeamPipelineConfigBasic(accessLevel string) string {
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
		resource "buildkite_team_member" "test" {
		    access_level = "%s"
			team_id = buildkite_team.test-team.id
			pipeline_id = buildkite_pipeline.test-pipeline.id 
		}
	`
	return fmt.Sprintf(config, accessLevel)
}

func testAccCheckTeamPipelineExists(resourceName string, tp *teamPipelineResourceModel) resource.TestCheckFunc {
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
			return fmt.Errorf("Error fetching team member from graphql API: %v", err)
		}

		if teamPipelineNode, ok := apiResponse.GetNode().(*getNodeNodeTeamPipeline); ok {
			if teamPipelineNode == nil {
				return fmt.Errorf("Error getting team member: nil response")
			}
			updateTeamPipelineResourceState(tp, *teamPipelineNode)
		}

		return nil
	}
}

// verify the team member has been removed
func testCheckTeamPipelineResourceRemoved(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_team_member" {
			continue
		}

		apiResponse, err := getNode(genqlientGraphql, rs.Primary.ID)

		if err != nil {
			return fmt.Errorf("Error fetching team member from graphql API: %v", err)
		}

		if teamPipelineNode, ok := apiResponse.GetNode().(*getNodeNodeTeamPipeline); ok {
			if teamPipelineNode != nil {
				return fmt.Errorf("Team member still exists")
			}
		}
	}
	return nil
}

func testAccCheckTeamPipelineRemoteValues(accessLevel string, tp *teamPipelineResourceModel) resource.TestCheckFunc {
	return func(s *terraform.State) error {

		if string(tp.AccessLevel.ValueString()) != accessLevel {
			return fmt.Errorf("remote team pipeline access level (%s) doesn't match expected value (%s)", tp.AccessLevel.ValueString(), accessLevel)
		}
		return nil
	}
}
