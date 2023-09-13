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

func TestAccBuildkiteTeam(t *testing.T) {
	configBasic := func(name string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "10s"
			}
		}

		resource "buildkite_team" "acc_tests" {
			name = "%s"
			description = "a cool team of %s"
			privacy = "VISIBLE"
			default_team = true
			default_member_role = "MEMBER"
			members_can_create_pipelines = true
		}
		`, name, name)
	}

	configSecret := func(name string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "10s"
				update = "10s"
			}
		}

		resource "buildkite_team" "acc_tests" {
			name = "%s"
			privacy = "SECRET"
			description = "a secret team of %s"
			default_team = true
			default_member_role = "MEMBER"
			members_can_create_pipelines = true
		}
		`, name, name)
	}

	t.Run("creates a team", func(t *testing.T) {
		resName := acctest.RandString(12)
		var tr teamResourceModel

		check := resource.ComposeAggregateTestCheckFunc(
			testAccCheckTeamExists("buildkite_team.acc_tests", &tr),
			testAccCheckTeamRemoteValues(resName, &tr),
			resource.TestCheckResourceAttr("buildkite_team.acc_tests", "name", resName),
			resource.TestCheckResourceAttr("buildkite_team.acc_tests", "privacy", "VISIBLE"),
		)

		checkPostRefresh := resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttrSet("buildkite_team.acc_tests", "name"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckTeamResourceDestroy,
			Steps: []resource.TestStep{
				{
					Config: configBasic(resName),
					Check:  check,
				},
				{
					RefreshState: true,
					PlanOnly:     true,
					Check:        checkPostRefresh,
				},
			},
		})
	})

	t.Run("updates a team", func(t *testing.T) {
		resName := acctest.RandString(12)
		resNameNew := acctest.RandString(12)
		var tr teamResourceModel

		check := resource.ComposeAggregateTestCheckFunc(
			testAccCheckTeamExists("buildkite_team.acc_tests", &tr),
			resource.TestCheckResourceAttr("buildkite_team.acc_tests", "name", resName),
		)

		checkNewName := resource.ComposeAggregateTestCheckFunc(
			testAccCheckTeamExists("buildkite_team.acc_tests", &tr),
			testAccCheckTeamRemoteValues(resNameNew, &tr),
			resource.TestCheckResourceAttr("buildkite_team.acc_tests", "name", resNameNew),
		)

		checkSecretTeam := resource.ComposeAggregateTestCheckFunc(
			testAccCheckTeamExists("buildkite_team.acc_tests", &tr),
			testAccCheckTeamRemoteValues(resNameNew, &tr),
			resource.TestCheckResourceAttr("buildkite_team.acc_tests", "name", resNameNew),
			resource.TestCheckResourceAttr("buildkite_team.acc_tests", "description", fmt.Sprintf("a secret team of %s", resNameNew)),
			resource.TestCheckResourceAttr("buildkite_team.acc_tests", "privacy", "SECRET"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckTeamResourceDestroy,
			Steps: []resource.TestStep{
				{
					Config: configBasic(resName),
					Check:  check,
				},
				{
					Config: configBasic(resNameNew),
					Check:  checkNewName,
				},
				{
					Config: configSecret(resNameNew),
					Check:  checkSecretTeam,
				},
			},
		})
	})

	t.Run("imports a team", func(t *testing.T) {
		resName := acctest.RandString(12)
		var tr teamResourceModel

		check := resource.ComposeAggregateTestCheckFunc(
			testAccCheckTeamExists("buildkite_team.acc_tests", &tr),
			resource.TestCheckResourceAttr("buildkite_team.acc_tests", "name", resName),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckTeamResourceDestroy,
			Steps: []resource.TestStep{
				{
					Config: configBasic(resName),
					Check:  check,
				},
				{
					ResourceName:      "buildkite_team.acc_tests",
					ImportState:       true,
					ImportStateVerify: true,
				},
			},
		})
	})

	t.Run("team is recreated if removed", func(t *testing.T) {
		resName := acctest.RandString(12)

		check := func(s *terraform.State) error {
			team := s.RootModule().Resources["buildkite_team.acc_tests"]
			_, err := teamDelete(context.Background(), genqlientGraphql, team.Primary.ID)
			return err
		}

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config:             configBasic(resName),
					Check:              check,
					ExpectNonEmptyPlan: true,
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{
							// expect terraform to plan a new create
							plancheck.ExpectResourceAction("buildkite_team.acc_tests", plancheck.ResourceActionCreate),
						},
					},
				},
			},
		})
	})
}

func testAccCheckTeamExists(name string, tr *teamResourceModel) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]

		if !ok {
			return fmt.Errorf("Not found in state: %s", name)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("No ID is set in state")
		}

		r, err := getNode(context.Background(), genqlientGraphql, rs.Primary.ID)

		if err != nil {
			return err
		}

		if teamNode, ok := r.GetNode().(*getNodeNodeTeam); ok {
			if teamNode == nil {
				return fmt.Errorf("Team not found: nil response")
			}
			updateTeamResourceState(tr, *teamNode)
		}
		return nil
	}
}

func testAccCheckTeamRemoteValues(name string, tr *teamResourceModel) resource.TestCheckFunc {
	return func(s *terraform.State) error {

		if tr.Name.ValueString() != name {
			return fmt.Errorf("remote team name (%s) doesn't match expected value (%s)", tr.Name, name)
		}
		return nil
	}
}

func testAccCheckTeamResourceDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_team" {
			continue
		}

		r, err := getNode(context.Background(), genqlientGraphql, rs.Primary.ID)

		if err != nil {
			return err
		}

		if teamNode, ok := r.GetNode().(*getNodeNodeTeam); ok {
			if teamNode != nil {
				return fmt.Errorf("Team still exists: %v", teamNode)
			}
		}
	}
	return nil
}
