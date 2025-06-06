package buildkite

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccBuildkiteAgentToken(t *testing.T) {

	basic := func(name string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "10s"
				read = "10s"
				update = "10s"
				delete = "10s"
			}
		}

		resource "buildkite_agent_token" "foobar" {
			description = "Acceptance Test %s"
		}
		`, name)
	}

	// Confirm that we can create a new agent token, and then delete it without error
	t.Run("adds an agent token", func(t *testing.T) {
		var resourceToken AgentTokenNode
		randName := acctest.RandString(10)

		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm the token exists in the buildkite API
			testAccCheckAgentTokenExists("buildkite_agent_token.foobar", &resourceToken),
			// Confirm the token has the correct values in Buildkite's system
			testAccCheckAgentTokenRemoteValues(&resourceToken, fmt.Sprintf("Acceptance Test %s", randName)),
			// Confirm the token has the correct values in terraform state
			resource.TestCheckResourceAttr("buildkite_agent_token.foobar", "description", fmt.Sprintf("Acceptance Test %s", randName)),
			resource.TestCheckResourceAttrSet("buildkite_agent_token.foobar", "id"),
			resource.TestCheckResourceAttrSet("buildkite_agent_token.foobar", "token"),
			resource.TestCheckResourceAttrSet("buildkite_agent_token.foobar", "uuid"),
		)

		checkRefresh := resource.ComposeAggregateTestCheckFunc(
			// Confirm the token has the correct values in terraform state
			resource.TestCheckResourceAttrSet("buildkite_agent_token.foobar", "id"),
			resource.TestCheckResourceAttrSet("buildkite_agent_token.foobar", "token"),
			resource.TestCheckResourceAttrSet("buildkite_agent_token.foobar", "uuid"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckAgentTokenResourceDestroy,
			Steps: []resource.TestStep{
				{
					Config: basic(randName),
					Check:  check,
				},
				{
					RefreshState: true,
					PlanOnly:     true,
					Check:        checkRefresh,
				},
			},
		})
	})

	// Confirm that we can create a new agent token, and then update the description
	// Technically tokens can't be updated, so this will actuall do a delete+create
	t.Run("updates an agent token", func(t *testing.T) {
		var resourceToken AgentTokenNode
		randName := acctest.RandString(10)
		randNameUpdated := acctest.RandString(10)

		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm the token exists in the buildkite API
			testAccCheckAgentTokenExists("buildkite_agent_token.foobar", &resourceToken),
			// Quick check to confirm the local state is correct before we update it
			resource.TestCheckResourceAttr("buildkite_agent_token.foobar", "description", fmt.Sprintf("Acceptance Test %s", randName)),
		)

		checkUpdated := resource.ComposeAggregateTestCheckFunc(
			// Confirm the token exists in the buildkite API
			testAccCheckAgentTokenExists("buildkite_agent_token.foobar", &resourceToken),
			// Confirm the token has the updated values in Buildkite's system
			testAccCheckAgentTokenRemoteValues(&resourceToken, fmt.Sprintf("Acceptance Test %s", randNameUpdated)),
			// Confirm the token has the updated values in terraform state
			resource.TestCheckResourceAttr("buildkite_agent_token.foobar", "description", fmt.Sprintf("Acceptance Test %s", randNameUpdated)),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckAgentTokenResourceDestroy,
			Steps: []resource.TestStep{
				{
					Config: basic(randName),
					Check:  check,
				},
				{
					Config: basic(randNameUpdated),
					Check:  checkUpdated,
				},
			},
		})
	})
}

func testAccCheckAgentTokenExists(resourceName string, resourceToken *AgentTokenNode) resource.TestCheckFunc {
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
				AgentToken AgentTokenNode `graphql:"... on AgentToken"`
			} `graphql:"node(id: $id)"`
		}

		vars := map[string]interface{}{
			"id": resourceState.Primary.ID,
		}

		err := graphqlClient.Query(context.Background(), &query, vars)
		if err != nil {
			return fmt.Errorf("Error fetching agent token from graphql API: %v", err)
		}

		if string(query.Node.AgentToken.ID) == "" {
			return fmt.Errorf("No agent token found with graphql id: %s", resourceState.Primary.ID)
		}

		// This is a property of the resource that can't be controleld by the user. The value in the TF
		// state should always just match the remote value. Is this the best place for this assertion?
		if string(query.Node.AgentToken.UUID) != resourceState.Primary.Attributes["uuid"] {
			return fmt.Errorf("agent token UUID in state doesn't match remote UUID")
		}

		*resourceToken = query.Node.AgentToken

		return nil
	}
}

func testAccCheckAgentTokenRemoteValues(resourceToken *AgentTokenNode, description string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if string(resourceToken.Description) != description {
			return fmt.Errorf("remote agent token description (%s) doesn't match expected value (%s)", resourceToken.Description, description)
		}
		return nil
	}
}

// verifies the agent token has been destroyed
func testAccCheckAgentTokenResourceDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_agent_token" {
			continue
		}
	}
	return nil
}
