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

func TestAccBuildkiteTeamMember(t *testing.T) {
	basic := func(name, role string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "10s"
				read = "10s"
				update = "10s"
				delete = "10s"
			}
		}

		resource "buildkite_team" "test" {
			name = "acceptance testing %s"
			description = "a cool team for testing"
			privacy = "VISIBLE"
			default_team = true
			default_member_role = "MEMBER"
			members_can_create_pipelines = false
		}

		resource "buildkite_team_member" "test" {
		    role = "%s"
			team_id = buildkite_team.test.id
			user_id = "VXNlci0tLThkYjI5MjBlLTNjNjAtNDhhNy1hM2Y4LTI1ODRiZTM3NGJhYw=="
		}
		`, name, role)
	}

	t.Run("adds a team member", func(t *testing.T) {
		var tm teamMemberResourceModel
		randName := acctest.RandString(10)

		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm the team member exists in the buildkite API
			testAccCheckTeamMemberExists("buildkite_team_member.test", &tm),
			// Confirm the team member has the correct values in Buildkite's system
			testAccCheckTeamMemberRemoteValues("MEMBER", &tm),
			// Confirm the team member has the correct values in terraform state
			resource.TestCheckResourceAttr("buildkite_team_member.test", "role", "MEMBER"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testCheckTeamMemberResourceRemoved,
			Steps: []resource.TestStep{
				{
					Config: basic(randName, "MEMBER"),
					Check:  check,
				},
			},
		})
	})

	t.Run("adds a team member as a maintainer", func(t *testing.T) {
		var tm teamMemberResourceModel
		randName := acctest.RandString(10)

		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm the team member exists in the buildkite API
			testAccCheckTeamMemberExists("buildkite_team_member.test", &tm),
			// Confirm the team member has the correct values in Buildkite's system
			testAccCheckTeamMemberRemoteValues("MAINTAINER", &tm),
			// Confirm the team member has the correct values in terraform state
			resource.TestCheckResourceAttr("buildkite_team_member.test", "role", "MAINTAINER"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testCheckTeamMemberResourceRemoved,
			Steps: []resource.TestStep{
				{
					Config: basic(randName, "MAINTAINER"),
					Check:  check,
				},
			},
		})
	})

	t.Run("updates a team member from member to maintainer", func(t *testing.T) {
		var tm teamMemberResourceModel
		randName := acctest.RandString(10)

		checkMember := resource.ComposeAggregateTestCheckFunc(
			// Confirm the team member exists in the buildkite API
			testAccCheckTeamMemberExists("buildkite_team_member.test", &tm),
			// Confirm the team has the correct values in Buildkite's system
			testAccCheckTeamMemberRemoteValues("MEMBER", &tm),
		)

		checkMaintainer := resource.ComposeAggregateTestCheckFunc(
			// Confirm the team member exists in the buildkite API
			testAccCheckTeamMemberExists("buildkite_team_member.test", &tm),
			// Confirm the team has the correct values in Buildkite's system
			testAccCheckTeamMemberRemoteValues("MAINTAINER", &tm),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testCheckTeamMemberResourceRemoved,
			Steps: []resource.TestStep{
				{
					Config: basic(randName, "MEMBER"),
					Check:  checkMember,
				},
				{
					Config: basic(randName, "MAINTAINER"),
					Check:  checkMaintainer,
				},
			},
		})
	})

	t.Run("imports a team member", func(t *testing.T) {
		var tm teamMemberResourceModel
		randName := acctest.RandString(10)

		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm the team member exists in the buildkite API
			testAccCheckTeamMemberExists("buildkite_team_member.test", &tm),
			// Confirm the team has the correct values in Buildkite's system
			resource.TestCheckResourceAttr("buildkite_team_member.test", "role", "MEMBER"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testCheckTeamMemberResourceRemoved,
			Steps: []resource.TestStep{
				{
					Config: basic(randName, "MEMBER"),
					Check:  check,
				},
				{
					// re-import the resource and confirm they match
					ResourceName:      "buildkite_team_member.test",
					ImportState:       true,
					ImportStateVerify: true,
				},
			},
		})
	})

	t.Run("team member is recreated if removed", func(t *testing.T) {
		resName := acctest.RandString(12)

		check := func(s *terraform.State) error {
			teamMember := s.RootModule().Resources["buildkite_team_member.test"]
			_, err := deleteTeamMember(context.Background(), genqlientGraphql, teamMember.Primary.ID)
			return err
		}

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config:             basic(resName, "MEMBER"),
					Check:              check,
					ExpectNonEmptyPlan: true,
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{
							// expect terraform to plan a new create
							plancheck.ExpectResourceAction("buildkite_team_member.test", plancheck.ResourceActionCreate),
						},
					},
				},
			},
		})
	})
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
