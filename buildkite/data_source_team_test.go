package buildkite

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func testDatasourceTeamConfigID(name string) string {
	data := `
		resource "buildkite_team" "acc_tests_id" {
			name = "%s"
			description = "a cool team of %s"
			privacy = "VISIBLE"
			default_team = true
			default_member_role = "MEMBER"
			members_can_create_pipelines = true
		}

		data "buildkite_team" "data_test_id" {
			depends_on = [buildkite_team.acc_tests_id]
			id = buildkite_team.acc_tests_id.id
		}
	`
	return fmt.Sprintf(data, name, name)
}

func testDatasourceTeamConfigSlug(name string) string {
	data := `
		resource "buildkite_team" "acc_tests_slug" {
			name = "%s"
			description = "a cool team of %s"
			privacy = "VISIBLE"
			default_team = true
			default_member_role = "MEMBER"
		}

		data "buildkite_team" "data_test_slug" {
			depends_on = [buildkite_team.acc_tests_slug]
			slug = buildkite_team.acc_tests_slug.slug
		}
	`
	return fmt.Sprintf(data, name, name)
}

func TestAccDataTeam_ReadUsingSlug(t *testing.T) {
	t.Parallel()
	var tr teamResourceModel

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testDatasourceTeamConfigSlug("designers"),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckTeamExists("buildkite_team.acc_tests_slug", &tr),
					testAccCheckTeamRemoteValues("designers", &tr),
					resource.TestCheckResourceAttr("data.buildkite_team.data_test_slug", "name", "designers"),
					resource.TestCheckResourceAttr("data.buildkite_team.data_test_slug", "members_can_create_pipelines", "false"),
				),
			},
		},
	})
}

func TestAccDataTeam_ReadUsingID(t *testing.T) {
	t.Parallel()
	var tr teamResourceModel

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testDatasourceTeamConfigID("developers"),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckTeamExists("buildkite_team.acc_tests_id", &tr),
					testAccCheckTeamRemoteValues("developers", &tr),
					resource.TestCheckResourceAttr("data.buildkite_team.data_test_id", "name", "developers"),
					resource.TestCheckResourceAttr("data.buildkite_team.data_test_id", "members_can_create_pipelines", "true"),
				),
			},
		},
	})
}
