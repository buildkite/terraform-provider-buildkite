package buildkite

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccTestSuiteTeam_add_remove(t *testing.T) {
	ownerTeamName := acctest.RandString(12)
	newTeamName := acctest.RandString(12)
	var tr teamResourceModel
	var ts getTestSuiteSuite
	var tst testSuiteTeamModel
	t.Parallel()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckTestSuiteTeamDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccCTestSuiteTeamConfigBasic(ownerTeamName, newTeamName, "READ_ONLY"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the new team/test suite exists in the Buildkite API
					testAccCheckTeamExists("buildkite_team.newteam", &tr),
					checkTestSuiteExists("buildkite_test_suite.testsuite", &ts),
					// Confirm the test suite team exists in the buildkite API
					testAccCheckTestSuiteTeamExists("buildkite_test_suite_team.teamsuite", &tst),
					// Confirm the test suite team has the correct values in Buildkite's system
					testAccCheckTestSuiteTeamRemoteValues("READ_ONLY", &tr, &ts, &tst),
					// Confirm the test suite team has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_test_suite_team.teamsuite", "access_level", "READ_ONLY"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite_team.teamsuite", "id"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite_team.teamsuite", "uuid"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite_team.teamsuite", "test_suite_id"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite_team.teamsuite", "team_id"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite_team.teamsuite", "access_level"),
				),
			},
		},
	})
}

func TestAccTestSuiteTeam_update(t *testing.T) {
	ownerTeamName := acctest.RandString(12)
	newTeamName := acctest.RandString(12)
	var tr teamResourceModel
	var ts getTestSuiteSuite
	var tst testSuiteTeamModel
	t.Parallel()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckTestSuiteTeamDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccCTestSuiteTeamConfigBasic(ownerTeamName, newTeamName, "READ_ONLY"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the new team/test suite exists in the Buildkite API
					testAccCheckTeamExists("buildkite_team.newteam", &tr),
					checkTestSuiteExists("buildkite_test_suite.testsuite", &ts),
					// Confirm the test suite team exists in the buildkite API
					testAccCheckTestSuiteTeamExists("buildkite_test_suite_team.teamsuite", &tst),
					// Confirm the test suite team has the correct values in Buildkite's system
					testAccCheckTestSuiteTeamRemoteValues("READ_ONLY", &tr, &ts, &tst),
					// Confirm the test suite team has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_test_suite_team.teamsuite", "access_level", "READ_ONLY"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite_team.teamsuite", "id"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite_team.teamsuite", "uuid"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite_team.teamsuite", "test_suite_id"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite_team.teamsuite", "team_id"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite_team.teamsuite", "access_level"),
				),
			},
			{
				Config: testAccCTestSuiteTeamConfigBasic(ownerTeamName, newTeamName, "MANAGE_AND_READ"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the new team/test suite exists in the Buildkite API
					testAccCheckTeamExists("buildkite_team.newteam", &tr),
					checkTestSuiteExists("buildkite_test_suite.testsuite", &ts),
					// Confirm the test suite team exists in the buildkite API
					testAccCheckTestSuiteTeamExists("buildkite_test_suite_team.teamsuite", &tst),
					// Confirm the test suite team has the correct values in Buildkite's system
					testAccCheckTestSuiteTeamRemoteValues("MANAGE_AND_READ", &tr, &ts, &tst),
					// Confirm the test suite team has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_test_suite_team.teamsuite", "access_level", "MANAGE_AND_READ"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite_team.teamsuite", "id"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite_team.teamsuite", "uuid"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite_team.teamsuite", "test_suite_id"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite_team.teamsuite", "team_id"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite_team.teamsuite", "access_level"),
				),
			},
		},
	})
}

func TestAccTestSuiteTeam_import(t *testing.T) {
	ownerTeamName := acctest.RandString(12)
	newTeamName := acctest.RandString(12)
	var tr teamResourceModel
	var ts getTestSuiteSuite
	var tst testSuiteTeamModel

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckTestSuiteTeamDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccCTestSuiteTeamConfigBasic(ownerTeamName, newTeamName, "READ_ONLY"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the new team/test suite exists in the Buildkite API
					testAccCheckTeamExists("buildkite_team.newteam", &tr),
					checkTestSuiteExists("buildkite_test_suite.testsuite", &ts),
					// Confirm the test suite team exists in the buildkite API
					testAccCheckTestSuiteTeamExists("buildkite_test_suite_team.teamsuite", &tst),
					// Confirm the test suite team has the correct values in Buildkite's system
					testAccCheckTestSuiteTeamRemoteValues("READ_ONLY", &tr, &ts, &tst),
					// Confirm the test suite team has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_test_suite_team.teamsuite", "access_level", "READ_ONLY"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite_team.teamsuite", "id"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite_team.teamsuite", "uuid"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite_team.teamsuite", "test_suite_id"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite_team.teamsuite", "team_id"),
					resource.TestCheckResourceAttrSet("buildkite_test_suite_team.teamsuite", "access_level"),
				),
			},
			{
				// re-import the resource (using the graphql token of the existing resource) and confirm they match
				ResourceName:      "buildkite_test_suite_team.teamsuite",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccCTestSuiteTeamConfigBasic(ownerTeamName, newTeamName, accessLevel string) string {
	config := `
	
	resource "buildkite_team" "ownerteam" {
		name = "Test Suite Team %s"
		default_team = false
		privacy = "VISIBLE"
		default_member_role = "MAINTAINER"
	}

	resource "buildkite_team" "newteam" {
		name = "Test Suite Team %s"
		default_team = false
		privacy = "VISIBLE"
		default_member_role = "MAINTAINER"
	}

	resource "buildkite_test_suite" "testsuite" {
		name = "Test Suite %s"
		default_branch = "main"
		team_owner_id = buildkite_team.ownerteam.id
	}

	resource "buildkite_test_suite_team" "teamsuite" {
		test_suite_id = buildkite_test_suite.testsuite.id
		team_id = buildkite_team.newteam.id
		access_level = "%s"
	}
	`

	return fmt.Sprintf(config, ownerTeamName, newTeamName, ownerTeamName, accessLevel)
}

func testAccCheckTestSuiteTeamExists(resourceName string, tst *testSuiteTeamModel) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		resourceState, ok := s.RootModule().Resources[resourceName]

		if !ok {
			return fmt.Errorf("Not found in state: %s", resourceName)
		}

		if resourceState.Primary.ID == "" {
			return fmt.Errorf("No ID is set in state")
		}

		apiResponse, err := getNode(genqlientGraphql, resourceState.Primary.ID)

		if err != nil {
			return fmt.Errorf("Error fetching test suite team from graphql API: %v", err)
		}

		if teamSuiteNode, ok := apiResponse.GetNode().(*getNodeNodeTeamSuite); ok {
			if teamSuiteNode == nil {
				return fmt.Errorf("Error getting test suite team: nil response")
			}
			updateTeamSuiteTeamResource(tst, *teamSuiteNode)
		}

		return nil
	}
}

func testAccCheckTestSuiteTeamDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_test_suite_team" {
			continue
		}

		apiResponse, err := getNode(genqlientGraphql, rs.Primary.ID)

		if err != nil {
			return fmt.Errorf("Error fetching test suite team from graphql API: %v", err)
		}

		if teamSuiteNode, ok := apiResponse.GetNode().(*getNodeNodeTeamSuite); ok {
			if teamSuiteNode != nil {
				return fmt.Errorf("Test suite team still exists")
			}
		}
	}
	return nil
}

func testAccCheckTestSuiteTeamRemoteValues(accessLevel string, tr *teamResourceModel, ts *getTestSuiteSuite, tst *testSuiteTeamModel) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if tst.TestSuiteId.ValueString() != string(ts.Id) {
			return fmt.Errorf("Remote test suite team suite ID (%s) doesn't match expected value (%s)", tst.TestSuiteId.ValueString(), ts.Id)
		}

		if tst.TeamID.ValueString() != tr.ID.ValueString() {
			return fmt.Errorf("Remote test suite team ID (%s) doesn't match expected value (%s)", tst.TeamID.ValueString(), tr.ID)
		}

		if tst.AccessLevel.ValueString() != accessLevel {
			return fmt.Errorf("Remote test suite team access level (%s) doesn't match expected value (%s)", tst.AccessLevel.ValueString(), accessLevel)
		}

		return nil
	}
}
