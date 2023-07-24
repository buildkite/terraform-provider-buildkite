package buildkite

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
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
					team {
						id = resource.buildkite_team.team.id
						access_level = "READ_ONLY"
					}
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "id"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "api_token"),
					resource.TestCheckResourceAttr("buildkite_test_suite.suite", "default_branch", "main"),
					resource.TestCheckResourceAttr("buildkite_test_suite.suite", "name", "test suite"),
					resource.TestCheckResourceAttr("buildkite_test_suite.suite", "team.#", "1"),
					resource.TestCheckResourceAttrPair("buildkite_test_suite.suite", "team.0.id", "buildkite_team.team", "id"),
					resource.TestCheckResourceAttr("buildkite_test_suite.suite", "team.0.access_level", "READ_ONLY"),
					// TODO add check that it exists in the API
				),
			},
		},
	})
}

func TestAcc_testSuiteAddRemove_v6(t *testing.T) {
	t.Parallel()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"buildkite": providerserver.NewProtocol6WithError(New("testing", true)),
		},
		// CheckDestroy: , // TODO add destroy check
		Steps: []resource.TestStep{
			{
				Config: `
				resource "buildkite_team" "team" {
					name = "test suite team v6"
					default_team = false
					privacy = "VISIBLE"
					default_member_role = "MAINTAINER"
				}
				resource "buildkite_test_suite" "suite" {
					name = "test suite"
					default_branch = "main"
					teams = [{
						id = resource.buildkite_team.team.id
						access_level = "READ_ONLY"
					}]
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "id"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "api_token"),
					resource.TestCheckResourceAttr("buildkite_test_suite.suite", "default_branch", "main"),
					resource.TestCheckResourceAttr("buildkite_test_suite.suite", "name", "test suite"),
					resource.TestCheckResourceAttr("buildkite_test_suite.suite", "team.#", "1"),
					resource.TestCheckResourceAttrPair("buildkite_test_suite.suite", "team.0.id", "buildkite_team.team", "id"),
					resource.TestCheckResourceAttr("buildkite_test_suite.suite", "team.0.access_level", "READ_ONLY"),
					// TODO add check that it exists in the API
				),
			},
		},
	})
}
