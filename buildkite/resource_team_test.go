package buildkite

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// Confirm that we can create a team, and then delete it without error
func TestAccTeam_add_remove(t *testing.T) {
	var resourceTeam TeamNode

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: providerFactories(),
		CheckDestroy:      testAccCheckTeamResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccTeamConfigBasic("developers"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the team exists in the buildkite API
					testAccCheckTeamExists("buildkite_team.test", &resourceTeam),
					// Confirm the team has the correct values in Buildkite's system
					testAccCheckTeamRemoteValues(&resourceTeam, "developers"),
					// Confirm the team has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_team.test", "name", "developers"),
					resource.TestCheckResourceAttr("buildkite_team.test", "privacy", "VISIBLE"),
				),
			},
		},
	})
}

func TestAccTeam_update(t *testing.T) {
	var resourceTeam TeamNode

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: providerFactories(),
		CheckDestroy:      testAccCheckTeamResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccTeamConfigBasic("developers"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the team exists in the buildkite API
					testAccCheckTeamExists("buildkite_team.test", &resourceTeam),
					// Quick check to confirm the local state is correct before we update it
					resource.TestCheckResourceAttr("buildkite_team.test", "name", "developers"),
				),
			},
			{
				Config: testAccTeamConfigBasic("wombats"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the team exists in the buildkite API
					testAccCheckTeamExists("buildkite_team.test", &resourceTeam),
					// Confirm the team has the updated values in Buildkite's system
					testAccCheckTeamRemoteValues(&resourceTeam, "wombats"),
					// Confirm the team has the updated values in terraform state
					resource.TestCheckResourceAttr("buildkite_team.test", "name", "wombats"),
					resource.TestCheckResourceAttr("buildkite_team.test", "name", "wombats"),
				),
			},
			{
				Config: testAccTeamConfigSecret("wombats"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the team exists in the buildkite API
					testAccCheckTeamExists("buildkite_team.test", &resourceTeam),
					// Confirm the team has the updated values in Buildkite's system
					testAccCheckTeamRemoteValues(&resourceTeam, "wombats"),
					// Confirm the team has the updated values in terraform state
					resource.TestCheckResourceAttr("buildkite_team.test", "name", "wombats"),
					resource.TestCheckResourceAttr("buildkite_team.test", "description", "a secret team of wombats"),
					resource.TestCheckResourceAttr("buildkite_team.test", "privacy", "SECRET"),
				),
			},
		},
	})
}

// Confirm that this resource can be imported
func TestAccTeam_import(t *testing.T) {
	var resourceTeam TeamNode

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: providerFactories(),
		CheckDestroy:      testAccCheckTeamResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccTeamConfigBasic("important"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the team exists in the buildkite API
					testAccCheckTeamExists("buildkite_team.test", &resourceTeam),
					// Quick check to confirm the local state is correct before we re-import it
					resource.TestCheckResourceAttr("buildkite_team.test", "name", "important"),
				),
			},
			{
				// re-import the resource (using the graphql token of the existing resource) and confirm they match
				ResourceName:      "buildkite_team.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// Confirm that this resource can be removed
func TestAccTeam_disappears(t *testing.T) {
	var node TeamNode
	resourceName := "buildkite_team.test"

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: providerFactories(),
		CheckDestroy:      testAccCheckPipelineResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccTeamConfigBasic("foo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the team exists in the buildkite API
					testAccCheckTeamExists(resourceName, &node),
					// Ensure its removal from the spec
					testAccCheckResourceDisappears(Provider("testing"), resourceTeam(), resourceName),
				),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func testAccCheckTeamExists(resourceName string, resourceTeam *TeamNode) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		resourceState, ok := s.RootModule().Resources[resourceName]

		if !ok {
			return fmt.Errorf("Not found in state: %s", resourceName)
		}

		if resourceState.Primary.ID == "" {
			return fmt.Errorf("No ID is set in state")
		}

		var query struct {
			Node struct {
				Team TeamNode `graphql:"... on Team"`
			} `graphql:"node(id: $id)"`
		}

		vars := map[string]interface{}{
			"id": resourceState.Primary.ID,
		}

		err := graphqlClient.Query(context.Background(), &query, vars)
		if err != nil {
			return fmt.Errorf("Error fetching team from graphql API: %v", err)
		}

		if string(query.Node.Team.ID) == "" {
			return fmt.Errorf("No team found with graphql id: %s", resourceState.Primary.ID)
		}

		*resourceTeam = query.Node.Team

		return nil
	}
}

func testAccCheckTeamRemoteValues(resourceTeam *TeamNode, name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {

		if string(resourceTeam.Name) != name {
			return fmt.Errorf("remote team name (%s) doesn't match expected value (%s)", resourceTeam.Name, name)
		}
		return nil
	}
}

func testAccTeamConfigBasic(name string) string {
	config := `
		resource "buildkite_team" "test" {
		    name = "%s"
			description = "a cool team of %s"
		    privacy = "VISIBLE"
		    default_team = true
		    default_member_role = "MEMBER"
		}
	`
	return fmt.Sprintf(config, name, name)
}

func testAccTeamConfigSecret(name string) string {
	config := `
		resource "buildkite_team" "test" {
		    name = "%s"
			description = "a secret team of %s"
		    privacy = "SECRET"
		    default_team = true
		    default_member_role = "MEMBER"
		}
	`
	return fmt.Sprintf(config, name, name)
}

// verifies the team has been destroyed
func testAccCheckTeamResourceDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_team" {
			continue
		}

		// Try to find the resource remotely
		var query struct {
			Node struct {
				Team TeamNode `graphql:"... on Team"`
			} `graphql:"node(id: $id)"`
		}

		vars := map[string]interface{}{
			"id": rs.Primary.ID,
		}

		err := graphqlClient.Query(context.Background(), &query, vars)
		if err == nil {
			if string(query.Node.Team.ID) != "" &&
				string(query.Node.Team.ID) == rs.Primary.ID {
				return fmt.Errorf("Team still exists")
			}
		}

		return err
	}

	return nil
}
