package buildkite

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// Confirm we can add and remove a team member
func TestAccTeamMember_add_remove(t *testing.T) {
	var resourceTeamMember TeamMemberNode

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testCheckTeamMemberResourceRemoved,
		Steps: []resource.TestStep{
			{
				Config: testAccTeamMemberConfigBasic("MEMBER"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the team member exists in the buildkite API
					testAccChecKTeamMemberExists("buildkite_team_member.test", &resourceTeamMember),
					// Confirm the team has the correct values in Buildkite's system
					testAccCheckTeamMemberRemoteValues(&resourceTeamMember, "MEMBER"),
					// Confirm the team member has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_team_member.test", "role", "MEMBER"),
				),
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
		}
		resource "buildkite_team_member" "test" {
		    role = "%s"
			team_id = buildkite_team.test.id
			user_id = "VXNlci0tLWRlOTdmMjBiLWJkZmMtNGNjOC1hOTcwLTY1ODNiZTk2ZGEyYQ=="
		}
	`
	return fmt.Sprintf(config, role)
}

func testAccChecKTeamMemberExists(resourceName string, resourceTeamMember *TeamMemberNode) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		resourceState, ok := s.RootModule().Resources[resourceName]

		if !ok {
			return fmt.Errorf("Not found in state: %s", resourceName)
		}

		if resourceState.Primary.ID == "" {
			return fmt.Errorf("No ID is set in state")
		}

		provider := testAccProvider.Meta().(*Client)
		var query struct {
			Node struct {
				TeamMember TeamMemberNode `graphql:"... on TeamMember"`
			} `graphql:"node(id: $id)"`
		}

		vars := map[string]interface{}{
			"id": resourceState.Primary.ID,
		}

		err := provider.graphql.Query(context.Background(), &query, vars)
		if err != nil {
			return fmt.Errorf("Error fetching team from graphql API: %v", err)
		}

		if string(query.Node.TeamMember.ID) == "" {
			return fmt.Errorf("No team found with graphql id: %s", resourceState.Primary.ID)
		}

		*resourceTeamMember = query.Node.TeamMember

		return nil
	}
}

// verify the team member has been removed
func testCheckTeamMemberResourceRemoved(s *terraform.State) error {
	provider := testAccProvider.Meta().(*Client)

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_team_member" {
			continue
		}

		// Try to find the resource remotely
		var query struct {
			Node struct {
				TeamMember TeamMemberNode `graphql:"... on TeamMember"`
			} `graphql:"node(id: $id)"`
		}

		vars := map[string]interface{}{
			"id": rs.Primary.ID,
		}

		err := provider.graphql.Query(context.Background(), &query, vars)
		if err == nil {
			if string(query.Node.TeamMember.ID) != "" &&
				string(query.Node.TeamMember.ID) == rs.Primary.ID {
				return fmt.Errorf("Team still exists")
			}
		}

		return err
	}

	return nil
}

func testAccCheckTeamMemberRemoteValues(resourceTeamMember *TeamMemberNode, name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {

		if string(resourceTeamMember.Role) != name {
			return fmt.Errorf("remote team member role (%s) doesn't match expected value (%s)", resourceTeamMember.Role, name)
		}
		return nil
	}
}
