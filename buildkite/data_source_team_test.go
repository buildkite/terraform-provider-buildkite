package buildkite

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func testDatasourceTeamConfig(name string) string {
	data := `
		resource "buildkite_team" "acc_tests" {
			name = "%s"
			description = "a cool team of %s"
			privacy = "VISIBLE"
			default_team = true
			default_member_role = "MEMBER"
			members_can_create_pipelines = true
		}

		data "buildkite_team" "data_test" {
			depends_on = [buildkite_team.acc_tests]
			id = buildkite_team.acc_tests.id
		}
	`
	return fmt.Sprintf(data, name, name)
}

func TestAccDataTeam_Read(t *testing.T) {
	t.Parallel()
	var tr teamResourceModel

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testDatasourceTeamConfig("developers"),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckTeamExists("buildkite_team.acc_tests", &tr),
					testAccCheckTeamRemoteValues("developers", &tr),
					resource.TestCheckResourceAttr("data.buildkite_team.data_test", "name", "developers"),
					resource.TestCheckResourceAttr("data.buildkite_team.data_test", "privacy", "VISIBLE"),
				),
			},
		},
	})
}
