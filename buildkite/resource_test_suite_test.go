package buildkite

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAcc_testSuiteAddRemove(t *testing.T) {
	t.Parallel()
	var suite getTestSuiteSuite

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testTestSuiteDestroy,
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
					checkTestSuiteExists("buildkite_test_suite.suite", &suite),
					checkTestSuiteRemoteValue(&suite, "Name", "test suite"),
					checkTestSuiteRemoteValue(&suite, "DefaultBranch", "main"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "id"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "api_token"),
					resource.TestCheckResourceAttr("buildkite_test_suite.suite", "default_branch", "main"),
					resource.TestCheckResourceAttr("buildkite_test_suite.suite", "name", "test suite"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "team_owner_id"),
				),
			},
		},
	})
}

func TestAcc_testSuiteUpdate(t *testing.T) {
	t.Parallel()
	var suite getTestSuiteSuite

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testTestSuiteDestroy,
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
					checkTestSuiteExists("buildkite_test_suite.suite", &suite),
					checkTestSuiteRemoteValue(&suite, "Name", "test suite update"),
					checkTestSuiteRemoteValue(&suite, "DefaultBranch", "main"),
				),
			},
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
				Taint: []string{"buildkite_team.team"},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "id"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "api_token"),
					resource.TestCheckResourceAttr("buildkite_test_suite.suite", "default_branch", "main"),
					resource.TestCheckResourceAttr("buildkite_test_suite.suite", "name", "test suite update"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "team_owner_id"),
					checkTestSuiteExists("buildkite_test_suite.suite", &suite),
					checkTestSuiteRemoteValue(&suite, "Name", "test suite update"),
					checkTestSuiteRemoteValue(&suite, "DefaultBranch", "main"),
				),
			},
		},
	})
}

func TestAcc_testSuiteTeamOwnerResolution(t *testing.T) {
	t.Parallel()
	var suite getTestSuiteSuite

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testTestSuiteDestroy,
		Steps: []resource.TestStep{
			{
				Config: `
				resource "buildkite_team" "ateam" {
					name = "a team"
					default_team = false
					privacy = "VISIBLE"
					default_member_role = "MAINTAINER"
				}
				resource "buildkite_team" "bteam" {
					name = "b team"
					default_team = false
					privacy = "VISIBLE"
					default_member_role = "MAINTAINER"
				}
				resource "buildkite_test_suite" "suite" {
					name = "test suite update"
					default_branch = "main"
					team_owner_id = resource.buildkite_team.bteam.id
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "id"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "api_token"),
					resource.TestCheckResourceAttr("buildkite_test_suite.suite", "default_branch", "main"),
					resource.TestCheckResourceAttr("buildkite_test_suite.suite", "name", "test suite update"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "team_owner_id"),
					resource.TestCheckResourceAttrPair("buildkite_test_suite.suite", "team_owner_id", "buildkite_team.bteam", "id"),
					checkTestSuiteExists("buildkite_test_suite.suite", &suite),
					checkTestSuiteRemoteValue(&suite, "Name", "test suite update"),
					checkTestSuiteRemoteValue(&suite, "DefaultBranch", "main"),
				),
			},
			{
				Config: `
				resource "buildkite_team" "ateam" {
					name = "a team"
					default_team = false
					privacy = "VISIBLE"
					default_member_role = "MAINTAINER"
				}
				resource "buildkite_team" "bteam" {
					name = "b team"
					default_team = false
					privacy = "VISIBLE"
					default_member_role = "MAINTAINER"
				}
				resource "buildkite_test_suite" "suite" {
					name = "test suite update"
					default_branch = "main"
					team_owner_id = resource.buildkite_team.bteam.id
				}
				resource "buildkite_test_suite_team" "team-a" {
					test_suite_id = buildkite_test_suite.suite.id
					team_id = buildkite_team.ateam.id
					access_level = "MANAGE_AND_READ"
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "id"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "api_token"),
					resource.TestCheckResourceAttr("buildkite_test_suite.suite", "default_branch", "main"),
					resource.TestCheckResourceAttr("buildkite_test_suite.suite", "name", "test suite update"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "team_owner_id"),
					resource.TestCheckResourceAttrPair("buildkite_test_suite.suite", "team_owner_id", "buildkite_team.bteam", "id"),
					checkTestSuiteExists("buildkite_test_suite.suite", &suite),
					checkTestSuiteRemoteValue(&suite, "Name", "test suite update"),
					checkTestSuiteRemoteValue(&suite, "DefaultBranch", "main"),
				),
			},
		},
	})
}

func checkTestSuiteRemoteValue(suite *getTestSuiteSuite, property, value string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if obj := reflect.ValueOf(*suite).FieldByName(property).String(); obj != value {
			return fmt.Errorf("%s property on test suite does not match \"%s\" (\"%s\")", property, value, obj)
		}

		return nil
	}
}

func loadRemoteTestSuite(id string) *getTestSuiteSuite {
	_suite, err := getTestSuite(genqlientGraphql, id, 1)
	if err != nil {
		return nil
	}
	if suite, ok := _suite.Suite.(*getTestSuiteSuite); ok {
		return suite
	}

	return nil
}

func checkTestSuiteExists(name string, suite *getTestSuiteSuite) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]
		if !ok {
			return errors.New("Test suite not found in state")
		}

		_suite := loadRemoteTestSuite(rs.Primary.Attributes["id"])

		if _suite == nil {
			return errors.New("Test suite does not exist on server")
		}

		suite.Id = _suite.Id
		suite.Uuid = _suite.Uuid
		suite.DefaultBranch = _suite.DefaultBranch
		suite.Name = _suite.Name
		suite.Slug = _suite.Slug
		suite.Teams = _suite.Teams

		return nil
	}
}

func testTestSuiteDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_test_suite" {
			continue
		}

		suite, err := getTestSuite(genqlientGraphql, rs.Primary.Attributes["id"], 1)

		if err != nil {
			return fmt.Errorf("Error fetching test suite from graphql API: %v", err)
		}

		if suite.Suite != nil {
			return fmt.Errorf("Test suite still exists: %v", suite)
		}
	}
	return nil
}
