package buildkite

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccBuildkiteTestSuiteDatasource(t *testing.T) {
	t.Run("Can find a datasource", func(t *testing.T) {
		suiteName := acctest.RandString(12)
		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(`
						resource "buildkite_test_suite" "acc_tests" {
							name = "%s"
							default_branch = "main"
							team_owner_id = buildkite_team.acc_tests_team.id
						}

						resource "buildkite_team" "acc_tests_team" {
							name = "%s-team"
							privacy = "VISIBLE"
							default_team = false
							default_member_role = "MEMBER"
						}

						data "buildkite_test_suite" "data_test_id" {
							depends_on = [buildkite_test_suite.acc_tests]
							slug = buildkite_test_suite.acc_tests.slug
						}
					`, suiteName, suiteName),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("data.buildkite_test_suite.data_test_id", "name", suiteName),
						resource.TestCheckResourceAttrPair("data.buildkite_test_suite.data_test_id", "id", "buildkite_test_suite.acc_tests", "id"),
					),
				},
			},
		})
	})
}
