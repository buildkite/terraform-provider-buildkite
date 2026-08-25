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

func TestAccBuildkiteTeamRegistryResource(t *testing.T) {
	config := func(name, accessLevel string) string {
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
			name = "Registry Owner Team %s"
			default_team = false
			privacy = "VISIBLE"
			default_member_role = "MAINTAINER"
		}

		resource "buildkite_team" "newteam" {
			name = "Registry New Team %s"
			default_team = false
			privacy = "VISIBLE"
			default_member_role = "MAINTAINER"
		}

		resource "buildkite_registry" "registry" {
			name = "registry-%s"
			ecosystem = "java"
			team_ids = [buildkite_team.ownerteam.uuid]
		}

		resource "buildkite_team_registry" "teamregistry" {
			registry_id = buildkite_registry.registry.id
			team_id = buildkite_team.newteam.id
			access_level = "%s"
		}
		`, name, name, name, accessLevel)
	}

	t.Run("creates a team registry", func(t *testing.T) {
		name := acctest.RandString(12)
		var tr teamResourceModel
		var trm teamRegistryModel

		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm the new team exists in the Buildkite API
			testAccCheckTeamExists("buildkite_team.newteam", &tr),
			// Confirm the team registry exists in the buildkite API
			testAccCheckTeamRegistryExists("buildkite_team_registry.teamregistry", &trm),
			// Confirm the team registry has the correct values in Buildkite's system
			testAccCheckTeamRegistryRemoteValues("READ_ONLY", &tr, &trm),
			// Confirm the team registry has the correct values in terraform state
			resource.TestCheckResourceAttr("buildkite_team_registry.teamregistry", "access_level", "READ_ONLY"),
			resource.TestCheckResourceAttrPair("buildkite_team_registry.teamregistry", "registry_id", "buildkite_registry.registry", "id"),
			resource.TestCheckResourceAttrPair("buildkite_team_registry.teamregistry", "team_id", "buildkite_team.newteam", "id"),
			resource.TestCheckResourceAttrSet("buildkite_team_registry.teamregistry", "id"),
			resource.TestCheckResourceAttrSet("buildkite_team_registry.teamregistry", "uuid"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckTeamRegistryDestroy,
			Steps: []resource.TestStep{
				{
					Config: config(name, "READ_ONLY"),
					Check:  check,
				},
			},
		})
	})

	t.Run("updates a team registry access level", func(t *testing.T) {
		name := acctest.RandString(12)
		var tr teamResourceModel
		var trm teamRegistryModel

		check := func(accessLevel string) resource.TestCheckFunc {
			return resource.ComposeAggregateTestCheckFunc(
				// Confirm the new team exists in the Buildkite API
				testAccCheckTeamExists("buildkite_team.newteam", &tr),
				// Confirm the team registry exists in the buildkite API
				testAccCheckTeamRegistryExists("buildkite_team_registry.teamregistry", &trm),
				// Confirm the team registry has the correct values in Buildkite's system
				testAccCheckTeamRegistryRemoteValues(accessLevel, &tr, &trm),
				// Confirm the team registry has the correct values in terraform state
				resource.TestCheckResourceAttr("buildkite_team_registry.teamregistry", "access_level", accessLevel),
			)
		}

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckTeamRegistryDestroy,
			Steps: []resource.TestStep{
				{
					Config: config(name, "READ_ONLY"),
					Check:  check("READ_ONLY"),
				},
				{
					Config: config(name, "READ_WRITE_AND_ADMIN"),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{
							plancheck.ExpectEmptyPlan(),
						},
					},
					Check: check("READ_WRITE_AND_ADMIN"),
				},
			},
		})
	})

	t.Run("imports a team registry", func(t *testing.T) {
		name := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckTeamRegistryDestroy,
			Steps: []resource.TestStep{
				{
					Config: config(name, "READ_AND_WRITE"),
				},
				{
					ResourceName:      "buildkite_team_registry.teamregistry",
					ImportState:       true,
					ImportStateVerify: true,
				},
			},
		})
	})

	t.Run("team registry is recreated if removed", func(t *testing.T) {
		name := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckTeamRegistryDestroy,
			Steps: []resource.TestStep{
				{
					Config: config(name, "READ_ONLY"),
					Check:  testAccCheckTeamRegistryDisappears("buildkite_team_registry.teamregistry"),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{
							// expect terraform to plan a new create
							plancheck.ExpectResourceAction("buildkite_team_registry.teamregistry", plancheck.ResourceActionCreate),
						},
					},
					ExpectNonEmptyPlan: true,
				},
			},
		})
	})
}

func testAccCheckTeamRegistryExists(resourceName string, trm *teamRegistryModel) resource.TestCheckFunc {
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
			return fmt.Errorf("Error fetching team registry from graphql API: %v", err)
		}

		if teamRegistryNode, ok := apiResponse.GetNode().(*getNodeNodeTeamRegistry); ok {
			if teamRegistryNode == nil {
				return fmt.Errorf("Error getting team registry: nil response")
			}
			updateTeamRegistryResource(trm, teamRegistryNode.TeamRegistryFields)
		}

		return nil
	}
}

func testAccCheckTeamRegistryDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_team_registry" {
			continue
		}

		apiResponse, err := getNode(context.Background(), genqlientGraphql, rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("Error fetching team registry from graphql API: %v", err)
		}

		if teamRegistryNode, ok := apiResponse.GetNode().(*getNodeNodeTeamRegistry); ok {
			if teamRegistryNode != nil {
				return fmt.Errorf("Team registry still exists")
			}
		}
	}
	return nil
}

func testAccCheckTeamRegistryRemoteValues(accessLevel string, tr *teamResourceModel, trm *teamRegistryModel) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if trm.TeamID.ValueString() != tr.ID.ValueString() {
			return fmt.Errorf("Remote team registry team ID (%s) doesn't match expected value (%s)", trm.TeamID.ValueString(), tr.ID)
		}

		if trm.AccessLevel.ValueString() != accessLevel {
			return fmt.Errorf("Remote team registry access level (%s) doesn't match expected value (%s)", trm.AccessLevel.ValueString(), accessLevel)
		}

		return nil
	}
}

func testAccCheckTeamRegistryDisappears(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		resourceState, ok := s.RootModule().Resources[resourceName]

		if !ok {
			return fmt.Errorf("Resource not found: %s", resourceName)
		}

		if resourceState.Primary.ID == "" {
			return fmt.Errorf("Resource ID missing: %s", resourceName)
		}

		_, err := deleteTeamRegistry(context.Background(), genqlientGraphql, resourceState.Primary.ID)

		return err
	}
}
