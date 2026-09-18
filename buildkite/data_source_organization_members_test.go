package buildkite

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccBuildkiteOrganizationMembersDatasource(t *testing.T) {
	t.Run("organization members data source can be loaded with defaults", func(t *testing.T) {
		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: `data "buildkite_organization_members" "members" {}`,
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttrSet("data.buildkite_organization_members.members", "members.0.email"),
					),
				},
			},
		})
	})

	t.Run("organization members data source can be filtered by team and role", func(t *testing.T) {
		name := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(`
					data "buildkite_organization_members" "all" {}

					resource "buildkite_team" "team" {
						name = "%s"
						privacy = "VISIBLE"
						default_team = false
						default_member_role = "MEMBER"
					}

					resource "buildkite_team_member" "member" {
						team_id = buildkite_team.team.id
						user_id = data.buildkite_organization_members.all.members[0].id
						role = "MEMBER"
					}

					data "buildkite_organization_members" "team" {
						team = buildkite_team.team.slug
						depends_on = [buildkite_team_member.member]
					}

					data "buildkite_organization_members" "admins" {
						team = buildkite_team.team.slug
						role = "ADMIN"
						depends_on = [buildkite_team_member.member]
					}
					`, name),
					Check: resource.ComposeAggregateTestCheckFunc(
						// Confirm the team filter finds the one member added above
						resource.TestCheckResourceAttr("data.buildkite_organization_members.team", "members.#", "1"),
						resource.TestCheckResourceAttrPair("data.buildkite_organization_members.team", "members.0.id", "buildkite_team_member.member", "user_id"),
						// Confirm the role filter is accepted alongside it (the member may or may not be an admin)
						resource.TestCheckResourceAttrSet("data.buildkite_organization_members.admins", "members.#"),
					),
				},
			},
		})
	})
}
