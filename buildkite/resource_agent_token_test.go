package buildkite

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// Confirm that we can create a new agent token, and then delete it without error
func TestAccAgentToken_add_remove(t *testing.T) {
	var resourceToken AgentTokenNode

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: providerFactories(),
		CheckDestroy:      testAccCheckAgentTokenResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccAgentTokenConfigBasic("foo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the token exists in the buildkite API
					testAccCheckAgentTokenExists("buildkite_agent_token.foobar", &resourceToken),
					// Confirm the token has the correct values in Buildkite's system
					testAccCheckAgentTokenRemoteValues(&resourceToken, "Acceptance Test foo"),
					// Confirm the token has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_agent_token.foobar", "description", "Acceptance Test foo"),
				),
			},
			{
				RefreshState: true,
				PlanOnly:     true,
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the token has the correct values in terraform state
					resource.TestCheckResourceAttrWith("buildkite_agent_token.foobar", "token", func(value string) error {
						if value == "" {
							return errors.New("Token should not be empty.")
						}
						return nil
					}),
				),
			},
		},
	})
}

// Confirm that we can create a new agent token, and then update the description
// Technically tokens can't be updated, so this will actuall do a delete+create
func TestAccAgentToken_update(t *testing.T) {
	var resourceToken AgentTokenNode

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: providerFactories(),
		CheckDestroy:      testAccCheckAgentTokenResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccAgentTokenConfigBasic("foo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the token exists in the buildkite API
					testAccCheckAgentTokenExists("buildkite_agent_token.foobar", &resourceToken),
					// Quick check to confirm the local state is correct before we update it
					resource.TestCheckResourceAttr("buildkite_agent_token.foobar", "description", "Acceptance Test foo"),
				),
			},
			{
				Config: testAccAgentTokenConfigBasic("bar"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the token exists in the buildkite API
					testAccCheckAgentTokenExists("buildkite_agent_token.foobar", &resourceToken),
					// Confirm the token has the updated values in Buildkite's system
					testAccCheckAgentTokenRemoteValues(&resourceToken, "Acceptance Test bar"),
					// Confirm the token has the updated values in terraform state
					resource.TestCheckResourceAttr("buildkite_agent_token.foobar", "description", "Acceptance Test bar"),
				),
			},
		},
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

		provider := Provider("testing").Meta().(*Client)
		var query struct {
			Node struct {
				AgentToken AgentTokenNode `graphql:"... on AgentToken"`
			} `graphql:"node(id: $id)"`
		}

		vars := map[string]interface{}{
			"id": resourceState.Primary.ID,
		}

		err := provider.graphql.Query(context.Background(), &query, vars)
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

func testAccAgentTokenConfigBasic(description string) string {
	config := `
		resource "buildkite_agent_token" "foobar" {
			description = "Acceptance Test %s"
		}
	`
	return fmt.Sprintf(config, description)
}

// verifies the agent token has been destroyed
func testAccCheckAgentTokenResourceDestroy(s *terraform.State) error {
	provider := Provider("testing").Meta().(*Client)

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_agent_token" {
			continue
		}

		var query struct {
			Node struct {
				AgentToken AgentTokenNode `graphql:"... on AgentToken"`
			} `graphql:"node(id: $id)"`
		}

		vars := map[string]interface{}{
			"id": rs.Primary.ID,
		}

		err := provider.graphql.Query(context.Background(), &query, vars)
		if err != nil {
			if strings.Contains(err.Error(), "This agent registration token was already revoked") {
				// not sure why it's already revoked, but fine by us. It's the state we need
				continue
			} else {
				return err
			}

		}
		if string(query.Node.AgentToken.ID) == "" {
			return fmt.Errorf("Token not found, expected to find it in a revoked state")
		}
		if string(query.Node.AgentToken.ID) != rs.Primary.ID {
			return fmt.Errorf("Found unexpected token")
		}
		if string(query.Node.AgentToken.RevokedAt) == "" {
			return fmt.Errorf("Agent token found but not revoked as expected")
		}
	}

	return nil
}
