package buildkite

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// Confirm we can add and remove a team member
func TestAccTeamMember_add_remove(t *testing.T) {
	var tm teamMemberResourceModel

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testCheckTeamMemberResourceRemoved,
		Steps: []resource.TestStep{
			{
				Config: testAccTeamMemberConfigBasic("MEMBER"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the team member exists in the buildkite API
					testAccCheckTeamMemberExists("buildkite_team_member.test", &tm),
					// Confirm the team member has the correct values in Buildkite's system
					testAccCheckTeamMemberRemoteValues("MEMBER", &tm),
					// Confirm the team member has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_team_member.test", "role", "MEMBER"),
				),
			},
		},
	})
}

func TestAccTeamMember_add_remove_non_default_role(t *testing.T) {
	var tm teamMemberResourceModel

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testCheckTeamMemberResourceRemoved,
		Steps: []resource.TestStep{
			{
				Config: testAccTeamMemberConfigBasic("MAINTAINER"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the team member exists in the buildkite API
					testAccCheckTeamMemberExists("buildkite_team_member.test", &tm),
					// Confirm the team member has the correct values in Buildkite's system
					testAccCheckTeamMemberRemoteValues("MAINTAINER", &tm),
					// Confirm the team member has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_team_member.test", "role", "MAINTAINER"),
				),
			},
		},
	})
}

func TestAccTeamMember_update(t *testing.T) {
	var tm teamMemberResourceModel

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testCheckTeamMemberResourceRemoved,
		Steps: []resource.TestStep{
			{
				Config: testAccTeamMemberConfigBasic("MEMBER"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the team member exists in the buildkite API
					testAccCheckTeamMemberExists("buildkite_team_member.test", &tm),
					// Confirm the team has the correct values in Buildkite's system
					testAccCheckTeamMemberRemoteValues("MEMBER", &tm),
				),
			},
			{
				Config: testAccTeamMemberConfigBasic("MAINTAINER"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the team member exists in the buildkite API
					testAccCheckTeamMemberExists("buildkite_team_member.test", &tm),
					// Confirm the team has the correct values in Buildkite's system
					testAccCheckTeamMemberRemoteValues("MAINTAINER", &tm),
				),
			},
		},
	})
}

// Confirm that this resource can be imported
func TestAccTeamMember_import(t *testing.T) {
	var tm teamMemberResourceModel

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testCheckTeamMemberResourceRemoved,
		Steps: []resource.TestStep{
			{
				Config: testAccTeamMemberConfigBasic("MEMBER"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the team member exists in the buildkite API
					testAccCheckTeamMemberExists("buildkite_team_member.test", &tm),
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

func testAccTeamMemberConfigBasic(role string) string {
	config := `
		resource "buildkite_team" "test" {
			name = "acceptance testing"
			description = "a cool team for testing"
			privacy = "VISIBLE"
			default_team = true
			default_member_role = "MEMBER"
			members_can_create_pipelines = false
			timeouts {
				create = "1m"
				read = "1m"
				update = "1m"
				delete = "1m"
			}
		}
		resource "buildkite_team_member" "test" {
		    role = "%s"
			team_id = buildkite_team.test.id
			user_id = "VXNlci0tLThkYjI5MjBlLTNjNjAtNDhhNy1hM2Y4LTI1ODRiZTM3NGJhYw=="
			timeouts {
				create = "1m"
				read = "1m"
				update = "1m"
				delete = "1m"
			}}
	`
	return fmt.Sprintf(config, role)
}

func testAccCheckTeamMemberExists(resourceName string, tm *teamMemberResourceModel) resource.TestCheckFunc {
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
			return fmt.Errorf("Error fetching team member from graphql API: %v", err)
		}

		if teamMemberNode, ok := apiResponse.GetNode().(*getNodeNodeTeamMember); ok {
			if teamMemberNode == nil {
				return fmt.Errorf("Error getting team member: nil response")
			}
			updateTeamMemberResourceState(tm, *teamMemberNode)
		}

		return nil
	}
}

// verify the team member has been removed
func testCheckTeamMemberResourceRemoved(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_team_member" {
			continue
		}

		apiResponse, err := getNode(context.Background(), genqlientGraphql, rs.Primary.ID)

		if err != nil {
			return fmt.Errorf("Error fetching team member from graphql API: %v", err)
		}

		if teamMemberNode, ok := apiResponse.GetNode().(*getNodeNodeTeamMember); ok {
			if teamMemberNode != nil {
				return fmt.Errorf("Team member still exists")
			}
		}
	}
	return nil
}

func testAccCheckTeamMemberRemoteValues(role string, tm *teamMemberResourceModel) resource.TestCheckFunc {
	return func(s *terraform.State) error {

		if string(tm.Role.ValueString()) != role {
			return fmt.Errorf("remote team member role (%s) doesn't match expected value (%s)", tm.Role.ValueString(), role)
		}
		return nil
	}
}
