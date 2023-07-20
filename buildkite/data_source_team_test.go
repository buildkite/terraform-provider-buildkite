package buildkite

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func testDatasourceTeamConfig() string {
	data := `
	%s
	data "buildkite_team" "test" {
	  depends_on = [buildkite_team.test]
	  id = buildkite_team.test.id
	}
	`
	return fmt.Sprintf(data, testAccTeamConfigBasic("developers"))
}

func TestAccDataTeam_Read(t *testing.T) {
	t.Parallel()
	var tr teamResourceModel

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testDatasourceTeamConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckTeamExists("buildkite_team.test", &tr),
					testAccCheckTeamRemoteValues("developers", &tr),
					resource.TestCheckResourceAttr("buildkite_team.test", "name", "developers"),
					resource.TestCheckResourceAttr("buildkite_team.test", "privacy", "VISIBLE"),
				),
			},
		},
	})
}
