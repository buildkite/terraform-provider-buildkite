package buildkite

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAcc_testSuiteAddRemove(t *testing.T) {
	t.Parallel()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		// CheckDestroy: , // TODO add destroy check
		Steps: []resource.TestStep{
			{
				Config: `
				resource "buildkite_team" "team" {
					name = "test suite team"
					default_team = false
					privacy = "VISIBLE"
					default_member_role = "MAINTAINER"
				}
				resource "buildkite_test_suite" "suite" {
					name = "test suite"
					default_branch = "main"
					team_owner_id = resource.buildkite_team.team.id
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "id"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "api_token"),
					resource.TestCheckResourceAttr("buildkite_test_suite.suite", "default_branch", "main"),
					resource.TestCheckResourceAttr("buildkite_test_suite.suite", "name", "test suite"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "team_owner_id"),
					// TODO add check that it exists in the API
				),
			},
		},
	})
}

func TestAcc_testSuiteUpdate(t *testing.T) {
	t.Parallel()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		// CheckDestroy: , // TODO add destroy check
		Steps: []resource.TestStep{
			{
				Config: `
				resource "buildkite_team" "team" {
					name = "test suite team update"
					default_team = false
					privacy = "VISIBLE"
					default_member_role = "MAINTAINER"
				}
				resource "buildkite_test_suite" "suite" {
					name = "test suite update"
					default_branch = "main"
					team_owner_id = resource.buildkite_team.team.id
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "id"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "api_token"),
					resource.TestCheckResourceAttr("buildkite_test_suite.suite", "default_branch", "main"),
					resource.TestCheckResourceAttr("buildkite_test_suite.suite", "name", "test suite update"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "team_owner_id"),
					// TODO add check that it exists in the API
				),
			},
			{
				Config: `
				resource "buildkite_team" "team2" {
					name = "test suite team update 2"
					default_team = false
					privacy = "VISIBLE"
					default_member_role = "MAINTAINER"
				}
				resource "buildkite_test_suite" "suite" {
					name = "test suite update"
					default_branch = "main"
					team_owner_id = resource.buildkite_team.team2.id
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "id"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "api_token"),
					resource.TestCheckResourceAttr("buildkite_test_suite.suite", "default_branch", "main"),
					resource.TestCheckResourceAttr("buildkite_test_suite.suite", "name", "test suite update"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "team_owner_id"),
					// TODO add check that it exists in the API
				),
			},
		},
	})
}
