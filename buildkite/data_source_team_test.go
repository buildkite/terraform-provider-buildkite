package buildkite

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

// Confirm that we can read a team based on the slug
func TestAccDataTeam_read(t *testing.T) {
	var resourceTeam TeamNode

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: providerFactories(),
		CheckDestroy:      testAccCheckTeamResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccDataTeamConfigBasic("foo", "buildkite_team.foobar.slug"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the team exists in the buildkite API
					testAccCheckTeamExists("buildkite_team.foobar", &resourceTeam),
					// Confirm the team data source has the correct values in terraform state
					resource.TestCheckResourceAttr("data.buildkite_team.foobar", "name", "Test Team foo"),
					resource.TestCheckResourceAttr("data.buildkite_team.foobar", "description", "A test team foo"),
					resource.TestCheckResourceAttr("data.buildkite_team.foobar", "privacy", "VISIBLE"),
					resource.TestCheckResourceAttr("data.buildkite_team.foobar", "default_team", "true"),
					resource.TestCheckResourceAttr("data.buildkite_team.foobar", "default_member_role", "MEMBER"),
					resource.TestCheckResourceAttr("data.buildkite_team.foobar", "slug", "test-team-foo"),
					resource.TestCheckResourceAttr("data.buildkite_team.foobar", "members_can_create_pipelines", "false"),
				),
			},
		},
	})
}

// Confirm that we get a nice error if no team is found
func TestAccDataTeam_readNotFound(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: providerFactories(),
		CheckDestroy:      testAccCheckTeamResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config:      testAccDataTeamConfigBasic("foo", "\"bar\""),
				ExpectError: regexp.MustCompile("Team not found: 'bar'"),
			},
		},
	})
}

func testAccDataTeamConfigBasic(name, slug string) string {
	config := `
		resource "buildkite_team" "foobar" {
			name = "Test Team %s"
			description = "A test team %s"
			privacy = "VISIBLE"
			default_team = true
			default_member_role = "MEMBER"
		}

		data "buildkite_team" "foobar" {
			slug = %s
		}
	`
	return fmt.Sprintf(config, name, name, slug)
}
