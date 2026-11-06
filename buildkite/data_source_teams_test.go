package buildkite

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccBuildkiteTeamsDatasource(t *testing.T) {
	t.Run("teams data source reads permissions correctly", func(t *testing.T) {
		config := `
		resource "buildkite_team" "test_team_perms" {
			name = "test-team-permissions"
			description = "team for testing permissions"
			privacy = "VISIBLE"
			default_team = false
			default_member_role = "MEMBER"
			members_can_create_pipelines = true
			members_can_create_suites = true
			members_can_create_registries = false
			members_can_destroy_registries = false
			members_can_destroy_packages = false
		}

		data "buildkite_teams" "teams" {
			depends_on = [buildkite_team.test_team_perms]
		}
		`

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: config,
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttrSet("data.buildkite_teams.teams", "teams.#"),
					),
				},
			},
		})
	})
}
