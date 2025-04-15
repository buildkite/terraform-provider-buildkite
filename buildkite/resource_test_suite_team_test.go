package buildkite

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccBuildkiteTestSuiteTeamResource(t *testing.T) {
	config := func(ownerTeamName, newTeamName, accessLevel string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "10s"
				read = "10s"
				update = "10s"
				delete = "10s"
			}
		}

		resource "buildkite_team" "ownerteam" {
			name = "Test Suite Owner Team %s"
			default_team = false
			privacy = "VISIBLE"
			default_member_role = "MAINTAINER"
		}

		resource "buildkite_team" "newteam" {
			name = "Test Suite New Team %s"
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
		`, ownerTeamName, newTeamName, ownerTeamName, accessLevel)
	}

	t.Run("creates a test suite team", func(t *testing.T) {
		ownerTeamName := acctest.RandString(12)
		newTeamName := acctest.RandString(12)
		var tr teamResourceModel
		var ts getTestSuiteSuite
		var tst testSuiteTeamModel

		check := resource.ComposeAggregateTestCheckFunc(
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
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckTestSuiteTeamDestroy,
			Steps: []resource.TestStep{
				{
					Config: config(ownerTeamName, newTeamName, "READ_ONLY"),
					Check:  check,
				},
			},
		})
	})

	t.Run("updates a test suite teams access level", func(t *testing.T) {
		ownerTeamName := acctest.RandString(12)
		newTeamName := acctest.RandString(12)
		var tr teamResourceModel
		var ts getTestSuiteSuite
		var tst testSuiteTeamModel

		checkReadOnly := resource.ComposeAggregateTestCheckFunc(
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
		)

		checkManageAndRead := resource.ComposeAggregateTestCheckFunc(
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
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckTestSuiteTeamDestroy,
			Steps: []resource.TestStep{
				{
					Config: config(ownerTeamName, newTeamName, "READ_ONLY"),
					Check:  checkReadOnly,
				},
				{
					Config: config(ownerTeamName, newTeamName, "MANAGE_AND_READ"),
					Check:  checkManageAndRead,
				},
			},
		})
	})

	t.Run("imports a test suite team", func(t *testing.T) {
		ownerTeamName := acctest.RandString(12)
		newTeamName := acctest.RandString(12)
		var tr teamResourceModel
		var ts getTestSuiteSuite
		var tst testSuiteTeamModel

		check := resource.ComposeAggregateTestCheckFunc(
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
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckTestSuiteTeamDestroy,
			Steps: []resource.TestStep{
				{
					Config: config(ownerTeamName, newTeamName, "READ_ONLY"),
					Check:  check,
				},
				{
					// re-import the resource (using the graphql token of the existing resource) and confirm they match
					ResourceName:      "buildkite_test_suite_team.teamsuite",
					ImportState:       true,
					ImportStateVerify: true,
				},
			},
		})
	})

	t.Run("removes a test suite team", func(t *testing.T) {
		ownerTeamName := acctest.RandString(12)
		newTeamName := acctest.RandString(12)
		var tr teamResourceModel
		var ts getTestSuiteSuite
		var tst testSuiteTeamModel

		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm the new team/test suite exists in the Buildkite API
			testAccCheckTeamExists("buildkite_team.newteam", &tr),
			checkTestSuiteExists("buildkite_test_suite.testsuite", &ts),
			// Confirm the test suite team exists in the buildkite API
			testAccCheckTestSuiteTeamExists("buildkite_test_suite_team.teamsuite", &tst),
			// Ensure the test suite team removal from spec
			testAccCheckTestSuiteTeamDisappears("buildkite_test_suite_team.teamsuite"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config:             config(ownerTeamName, newTeamName, "READ_ONLY"),
					Check:              check,
					ExpectNonEmptyPlan: true,
				},
			},
		})
	})

	t.Run("test suite team is recreated if removed", func(t *testing.T) {
		ownerTeamName := acctest.RandString(12)
		newTeamName := acctest.RandString(12)

		check := func(s *terraform.State) error {
			teamSuite := s.RootModule().Resources["buildkite_test_suite_team.teamsuite"]
			_, err := deleteTestSuiteTeam(context.Background(), genqlientGraphql, teamSuite.Primary.ID)
			return err
		}

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config:             config(ownerTeamName, newTeamName, "READ_ONLY"),
					Check:              check,
					ExpectNonEmptyPlan: true,
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{
							// expect terraform to plan a new create
							plancheck.ExpectResourceAction("buildkite_test_suite_team.teamsuite", plancheck.ResourceActionCreate),
						},
					},
				},
			},
		})
	})
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

		apiResponse, err := getNode(context.Background(), genqlientGraphql, resourceState.Primary.ID)
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

		apiResponse, err := getNode(context.Background(), genqlientGraphql, rs.Primary.ID)
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

func testAccCheckTestSuiteTeamDisappears(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		resourceState, ok := s.RootModule().Resources[resourceName]

		if !ok {
			return fmt.Errorf("Resource not found: %s", resourceName)
		}

		if resourceState.Primary.ID == "" {
			return fmt.Errorf("Resource ID missing: %s", resourceName)
		}

		_, err := deleteTestSuiteTeam(context.Background(), genqlientGraphql, resourceState.Primary.ID)

		return err
	}
}
