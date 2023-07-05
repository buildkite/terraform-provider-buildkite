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
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testCheckTeamMemberResourceRemoved,
		Steps: []resource.TestStep{
			{
				Config: testAccTeamMemberConfigBasic("MEMBER"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the team member exists in the buildkite API
					testAccCheckTeamMemberExists("buildkite_team_member.test", &resourceTeamMember),
					// Confirm the team member has the correct values in Buildkite's system
					testAccCheckTeamMemberRemoteValues(&resourceTeamMember, "MEMBER"),
					// Confirm the team member has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_team_member.test", "role", "MEMBER"),
				),
			},
		},
	})
}

func TestAccTeamMember_add_remove_non_default_role(t *testing.T) {
	var resourceTeamMember TeamMemberNode

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testCheckTeamMemberResourceRemoved,
		Steps: []resource.TestStep{
			{
				Config: testAccTeamMemberConfigBasic("MAINTAINER"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the team member exists in the buildkite API
					testAccCheckTeamMemberExists("buildkite_team_member.test", &resourceTeamMember),
					// Confirm the team member has the correct values in Buildkite's system
					testAccCheckTeamMemberRemoteValues(&resourceTeamMember, "MAINTAINER"),
					// Confirm the team member has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_team_member.test", "role", "MAINTAINER"),
				),
			},
		},
	})
}

func TestAccTeamMember_update(t *testing.T) {
	var resourceTeamMember TeamMemberNode

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testCheckTeamMemberResourceRemoved,
		Steps: []resource.TestStep{
			{
				Config: testAccTeamMemberConfigBasic("MEMBER"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the team member exists in the buildkite API
					testAccCheckTeamMemberExists("buildkite_team_member.test", &resourceTeamMember),
					// Confirm the team has the correct values in Buildkite's system
					testAccCheckTeamMemberRemoteValues(&resourceTeamMember, "MEMBER"),
				),
			},
			{
				Config: testAccTeamMemberConfigBasic("MAINTAINER"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the team member exists in the buildkite API
					testAccCheckTeamMemberExists("buildkite_team_member.test", &resourceTeamMember),
					// Confirm the team has the correct values in Buildkite's system
					testAccCheckTeamMemberRemoteValues(&resourceTeamMember, "MAINTAINER"),
				),
			},
		},
	})
}

// Confirm that this resource can be imported
func TestAccTeamMember_import(t *testing.T) {
	var resourceTeamMember TeamMemberNode

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testCheckTeamMemberResourceRemoved,
		Steps: []resource.TestStep{
			{
				Config: testAccTeamMemberConfigBasic("MEMBER"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the team member exists in the buildkite API
					testAccCheckTeamMemberExists("buildkite_team_member.test", &resourceTeamMember),
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

// Confirm that this resource can be removed
func TestAccTeamMember_disappears(t *testing.T) {
	var teamMember TeamMemberNode

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testCheckTeamMemberResourceRemoved,
		Steps: []resource.TestStep{
			{
				Config: testAccTeamMemberConfigBasic("MEMBER"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the team member exists in the buildkite API
					testAccCheckTeamMemberExists("buildkite_team_member.test", &teamMember),
					// Ensure its removed
					//testAccCheckResourceDisappears(Provider("testing"), resourceTeamMember(), "buildkite_team_member.test"),
				),
				ExpectNonEmptyPlan: true,
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
			user_id = "VXNlci0tLThkYjI5MjBlLTNjNjAtNDhhNy1hM2Y4LTI1ODRiZTM3NGJhYw=="
		}
	`
	return fmt.Sprintf(config, role)
}

func testAccCheckTeamMemberExists(resourceName string, resourceTeamMember *TeamMemberNode) resource.TestCheckFunc {
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
				TeamMember TeamMemberNode `graphql:"... on TeamMember"`
			} `graphql:"node(id: $id)"`
		}

		vars := map[string]interface{}{
			"id": resourceState.Primary.ID,
		}

		err := graphqlClient.Query(context.Background(), &query, vars)
		if err != nil {
			return fmt.Errorf("Error fetching team from graphql API: %v", err)
		}

		if string(query.Node.TeamMember.ID) == "" {
			return fmt.Errorf("No team member found with graphql id: %s", resourceState.Primary.ID)
		}

		*resourceTeamMember = query.Node.TeamMember

		return nil
	}
}

// verify the team member has been removed
func testCheckTeamMemberResourceRemoved(s *terraform.State) error {
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

		err := graphqlClient.Query(context.Background(), &query, vars)
		if err == nil {
			if string(query.Node.TeamMember.ID) != "" &&
				string(query.Node.TeamMember.ID) == rs.Primary.ID {
				return fmt.Errorf("Team member still exists")
			}
		}

		return err
	}

	return nil
}

func testAccCheckTeamMemberRemoteValues(resourceTeamMember *TeamMemberNode, role string) resource.TestCheckFunc {
	return func(s *terraform.State) error {

		if string(resourceTeamMember.Role) != role {
			return fmt.Errorf("remote team member role (%s) doesn't match expected value (%s)", resourceTeamMember.Role, role)
		}
		return nil
	}
}
