package buildkite

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccBuildkiteTestSuiteResource(t *testing.T) {
	basicTestSuite := func(name string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "10s"
				read = "10s"
				update = "10s"
				delete = "10s"
			}
		}

		resource "buildkite_team" "team" {
			name = "test suite team %s"
			default_team = false
			privacy = "VISIBLE"
			default_member_role = "MAINTAINER"
		}
		resource "buildkite_test_suite" "suite" {
			name = "test suite %s"
			default_branch = "main"
			team_owner_id = resource.buildkite_team.team.id
		}
		`, name, name)
	}

	testSuiteWithTwoTeams := func(name string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "10s"
				read = "10s"
				update = "10s"
				delete = "10s"
			}
		}

		resource "buildkite_team" "ateam" {
			name = "a team %s-a"
			default_team = false
			privacy = "VISIBLE"
			default_member_role = "MAINTAINER"
		}
		resource "buildkite_team" "bteam" {
			name = "b team %s-b"
			default_team = false
			privacy = "VISIBLE"
			default_member_role = "MAINTAINER"
		}
		resource "buildkite_test_suite" "suite" {
			name = "test suite update %s"
			default_branch = "main"
			team_owner_id = resource.buildkite_team.bteam.id
		}
		`, name, name, name)
	}

	testSuiteTeamAddition := func(name string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "10s"
				read = "10s"
				update = "10s"
				delete = "10s"
			}
		}

		resource "buildkite_team" "ateam" {
			name = "a team %s-a"
			default_team = false
			privacy = "VISIBLE"
			default_member_role = "MAINTAINER"
		}
		resource "buildkite_team" "bteam" {
			name = "b team %s-b"
			default_team = false
			privacy = "VISIBLE"
			default_member_role = "MAINTAINER"
		}
		resource "buildkite_test_suite" "suite" {
			name = "test suite update %s"
			default_branch = "main"
			team_owner_id = resource.buildkite_team.bteam.id
		}
		resource "buildkite_test_suite_team" "team-a" {
			test_suite_id = buildkite_test_suite.suite.id
			team_id = buildkite_team.ateam.id
			access_level = "MANAGE_AND_READ"
		}
		`, name, name, name)
	}

	t.Run("creates a test suite", func(t *testing.T) {
		var suite getTestSuiteSuite
		randName := acctest.RandString(10)

		check := resource.ComposeAggregateTestCheckFunc(
			checkTestSuiteExists("buildkite_test_suite.suite", &suite),
			checkTestSuiteRemoteValue(&suite, "Name", fmt.Sprintf("test suite %s", randName)),
			checkTestSuiteRemoteValue(&suite, "DefaultBranch", "main"),
			resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "id"),
			resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "api_token"),
			resource.TestCheckResourceAttr("buildkite_test_suite.suite", "default_branch", "main"),
			resource.TestCheckResourceAttr("buildkite_test_suite.suite", "name", fmt.Sprintf("test suite %s", randName)),
			resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "team_owner_id"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testTestSuiteDestroy,
			Steps: []resource.TestStep{
				{
					Config: basicTestSuite(randName),
					Check:  check,
				},
			},
		})
	})

	t.Run("updates a test suite", func(t *testing.T) {
		var suite getTestSuiteSuite
		randName := acctest.RandString(10)

		check := resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "id"),
			resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "api_token"),
			resource.TestCheckResourceAttr("buildkite_test_suite.suite", "default_branch", "main"),
			resource.TestCheckResourceAttr("buildkite_test_suite.suite", "name", fmt.Sprintf("test suite %s", randName)),
			resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "team_owner_id"),
			checkTestSuiteExists("buildkite_test_suite.suite", &suite),
			checkTestSuiteRemoteValue(&suite, "Name", fmt.Sprintf("test suite %s", randName)),
			checkTestSuiteRemoteValue(&suite, "DefaultBranch", "main"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testTestSuiteDestroy,
			Steps: []resource.TestStep{
				{
					Config: basicTestSuite(randName),
					Check:  check,
				},
				{
					Config: basicTestSuite(randName),
					Taint:  []string{"buildkite_team.team"},
					Check:  check,
				},
			},
		})
	})

	t.Run("creates and handles test suite team owner resolution", func(t *testing.T) {
		var suite getTestSuiteSuite
		randName := acctest.RandString(10)

		check := resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "id"),
			resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "api_token"),
			resource.TestCheckResourceAttr("buildkite_test_suite.suite", "default_branch", "main"),
			resource.TestCheckResourceAttr("buildkite_test_suite.suite", "name", fmt.Sprintf("test suite update %s", randName)),
			resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "team_owner_id"),
			resource.TestCheckResourceAttrPair("buildkite_test_suite.suite", "team_owner_id", "buildkite_team.bteam", "id"),
			checkTestSuiteExists("buildkite_test_suite.suite", &suite),
			checkTestSuiteRemoteValue(&suite, "Name", fmt.Sprintf("test suite update %s", randName)),
			checkTestSuiteRemoteValue(&suite, "DefaultBranch", "main"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testTestSuiteDestroy,
			Steps: []resource.TestStep{
				{
					Config: testSuiteWithTwoTeams(randName),
					Check:  check,
				},
				{
					Config: testSuiteTeamAddition(randName),
					Check:  check,
				},
			},
		})
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
	_suite, err := getTestSuite(context.Background(), genqlientGraphql, id, 1)
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

		suite, err := getTestSuite(context.Background(), genqlientGraphql, rs.Primary.Attributes["id"], 1)
		if err != nil {
			return fmt.Errorf("Error fetching test suite from graphql API: %v", err)
		}

		if suite.Suite != nil {
			return fmt.Errorf("Test suite still exists: %v", suite)
		}
	}
	return nil
}
