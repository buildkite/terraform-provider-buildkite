package buildkite

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccNonAdminTestSuiteTeam(t *testing.T) {
	const address = "buildkite_test_suite_team.secondary"
	secondarySlug := os.Getenv("BUILDKITE_NONADMIN_TEAM_SLUG")
	if secondarySlug == "" {
		secondarySlug = "everyone"
	}
	fixtures := fmt.Sprintf(`
		data "buildkite_team" "owner" {
			slug = %q
		}
		data "buildkite_team" "secondary" {
			slug = %q
		}
	`, testAccFixturePipelineTeamSlug, secondarySlug)
	baseConfig := fixtures + fmt.Sprintf(`
		resource "buildkite_test_suite" "temporary" {
			name = %q
			default_branch = "main"
			team_owner_id = data.buildkite_team.owner.id
		}
	`, testAccResourceName()+"-nonadmin-suite-team")
	config := func(access string) string {
		return baseConfig + fmt.Sprintf(`
			resource "buildkite_test_suite_team" "secondary" {
				test_suite_id = buildkite_test_suite.temporary.id
				team_id = data.buildkite_team.secondary.id
				access_level = %q
			}
		`, access)
	}
	var suiteID, associationID, associationUUID string
	check := func(access string) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			for _, name := range []string{"buildkite_test_suite.temporary", "data.buildkite_team.owner", "data.buildkite_team.secondary"} {
				if rs, ok := s.RootModule().Resources[name]; !ok || rs.Primary == nil || rs.Primary.ID == "" {
					return fmt.Errorf("%s is missing from Terraform state", name)
				}
			}
			suite := s.RootModule().Resources["buildkite_test_suite.temporary"].Primary
			ownerID := s.RootModule().Resources["data.buildkite_team.owner"].Primary.ID
			secondaryID := s.RootModule().Resources["data.buildkite_team.secondary"].Primary.ID
			if ownerID == secondaryID {
				return fmt.Errorf("secondary suite team must differ from its owner")
			}
			if suiteID == "" {
				suiteID = suite.ID
			}
			if suite.ID != suiteID || suite.Attributes["team_owner_id"] != ownerID {
				return fmt.Errorf("temporary suite identity or owner changed")
			}
			r, err := getTestSuite(context.Background(), genqlientGraphql, suiteID, teamPageSize, nil)
			if err != nil {
				return err
			}
			remote, ok := r.Suite.(*getTestSuiteSuite)
			if !ok || remote.Id != suiteID {
				return fmt.Errorf("temporary suite is no longer readable")
			}
			var ownerFound, secondaryFound bool
			for _, edge := range remote.Teams.Edges {
				if edge.Node.Team.Id == ownerID {
					ownerFound = edge.Node.AccessLevel == SuiteAccessLevelsManageAndRead
				}
				if edge.Node.Team.Id == secondaryID {
					secondaryFound = true
					if string(edge.Node.AccessLevel) != access {
						return fmt.Errorf("secondary suite team access = %q, want %q", edge.Node.AccessLevel, access)
					}
				}
			}
			if !ownerFound || secondaryFound != (access != "") {
				return fmt.Errorf("suite owner Manage & Read present: %t; secondary team present: %t (expected access %q)", ownerFound, secondaryFound, access)
			}
			if access == "" {
				if _, exists := s.RootModule().Resources[address]; exists {
					return fmt.Errorf("removed suite-team association remains in Terraform state")
				}
				node, err := getNode(context.Background(), genqlientGraphql, associationID)
				if err != nil {
					return err
				}
				if node.Node != nil {
					return fmt.Errorf("removed suite-team association is still readable")
				}
				return nil
			}
			association, ok := s.RootModule().Resources[address]
			if !ok || association.Primary == nil || association.Primary.ID == "" {
				return fmt.Errorf("temporary suite-team association is missing from Terraform state")
			}
			if associationID == "" {
				associationID, associationUUID = association.Primary.ID, association.Primary.Attributes["uuid"]
			}
			node, err := getNode(context.Background(), genqlientGraphql, associationID)
			if err != nil {
				return err
			}
			link, ok := node.Node.(*getNodeNodeTeamSuite)
			if !ok || link.Id != associationID || link.TeamSuiteUuid != associationUUID || link.Team.Id != secondaryID || link.Suite.Id != suiteID || string(link.AccessLevel) != access {
				return fmt.Errorf("remote suite-team association does not match the expected identity, endpoints, and access")
			}
			return resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(address, "id", associationID),
				resource.TestCheckResourceAttrSet(address, "uuid"),
				resource.TestCheckResourceAttr(address, "uuid", associationUUID),
				resource.TestCheckResourceAttr(address, "access_level", access),
				resource.TestCheckResourceAttrPair(address, "team_id", "data.buildkite_team.secondary", "id"),
				resource.TestCheckResourceAttrPair(address, "test_suite_id", "buildkite_test_suite.temporary", "id"),
			)(s)
		}
	}
	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			testAccNonAdminPreCheck(t)
			if secondarySlug == testAccFixturePipelineTeamSlug {
				t.Fatal("secondary suite team must differ from its owner")
			}
		},
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testTestSuiteDestroy,
		Steps: []resource.TestStep{
			{Config: config("READ_ONLY"), Check: check("READ_ONLY")},
			{
				Config: config("MANAGE_AND_READ"),
				Check:  check("MANAGE_AND_READ"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("buildkite_test_suite.temporary", plancheck.ResourceActionNoop),
					},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{RefreshState: true, Check: check("MANAGE_AND_READ")},
			{ResourceName: address, ImportState: true, ImportStateVerify: true},
			{
				Config: baseConfig,
				Check:  check(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(address, plancheck.ResourceActionDestroy),
						plancheck.ExpectResourceAction("buildkite_test_suite.temporary", plancheck.ResourceActionNoop),
					},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

func TestAccBuildkiteTestSuiteTeamResource(t *testing.T) {
	t.Parallel()
	config := func(ownerTeamName, newTeamName, accessLevel string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_team" "ownerteam" {
			name = "%s-owner-team"
			default_team = false
			privacy = "VISIBLE"
			default_member_role = "MAINTAINER"
		}

		resource "buildkite_team" "newteam" {
			name = "%s-new-team"
			default_team = false
			privacy = "VISIBLE"
			default_member_role = "MAINTAINER"
		}

		resource "buildkite_test_suite" "testsuite" {
			name = "%s-suite"
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

	t.Run("moves a test suite team to another team by replacing it", func(t *testing.T) {
		ownerTeamName := testAccResourceName()
		newTeamName := testAccResourceName()
		thirdTeam := fmt.Sprintf(`
		resource "buildkite_team" "thirdteam" {
			name = "%s-third-team"
			default_team = false
			privacy = "VISIBLE"
			default_member_role = "MAINTAINER"
		}
		`, newTeamName)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckTestSuiteTeamDestroy,
			Steps: []resource.TestStep{
				{
					Config: config(ownerTeamName, newTeamName, "READ_ONLY") + thirdTeam,
				},
				{
					// the access cannot be moved with an update, so changing the team replaces it
					Config: strings.Replace(config(ownerTeamName, newTeamName, "READ_ONLY"), "team_id = buildkite_team.newteam.id", "team_id = buildkite_team.thirdteam.id", 1) + thirdTeam,
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PreApply: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction("buildkite_test_suite_team.teamsuite", plancheck.ResourceActionReplace),
						},
					},
					Check: resource.TestCheckResourceAttrPair("buildkite_test_suite_team.teamsuite", "team_id", "buildkite_team.thirdteam", "id"),
				},
			},
		})
	})

	t.Run("removes a test suite team", func(t *testing.T) {
		ownerTeamName := testAccResourceName()
		newTeamName := testAccResourceName()
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
		ownerTeamName := testAccResourceName()
		newTeamName := testAccResourceName()

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
