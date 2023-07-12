package buildkite

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func testAccClusterAgentTokenBasic(description string) string {
	config := `
		resource "buildkite_cluster_agent_token" "foobar" {
			cluster_id = "Q2x1c3Rlci0tLTBhOTY5Yjc0LWEwYjctNDM0MC1hYWNlLWQ4NzQ0MjNmM2Q2Yw=="
			description = "Acceptance Test %s"
		}
	`
	return fmt.Sprintf(config, description)
}

func TestAccClusterAgentToken_add_remove(t *testing.T) {
	t.Parallel()
	var ct ClusterAgentTokenResourceModel

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckClusterAgentTokenDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccClusterAgentTokenBasic("foo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the token exists in the buildkite API
					testAccCheckClusterAgentTokenExists("buildkite_agent_token.foobar", &ct),
					// Confirm the token has the correct values in Buildkite's system
					testAccCheckClusterAgentTokenRemoteValues(&ct, "Acceptance Test foo"),
					// Confirm the token has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_agent_token.foobar", "description", "Acceptance Test foo"),
				),
			},
			{
				RefreshState: true,
				PlanOnly:     true,
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the token has the correct values in terraform state
					resource.TestCheckResourceAttrSet("buildkite_cluster_agent_token.foobar", "description"),
				),
			},
		},
	})
}

func TestAccClusterAgentToken_update(t *testing.T) {
	t.Parallel()
	var ct ClusterAgentTokenResourceModel

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckClusterAgentTokenDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccClusterAgentTokenBasic("foo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the token exists in the buildkite API
					testAccCheckClusterAgentTokenExists("buildkite_agent_token.foobar", &ct),
					// Confirm the token has the correct values in Buildkite's system
					testAccCheckClusterAgentTokenRemoteValues(&ct, "Acceptance Test foobar"),
					// Confirm the token has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_agent_token.foobar", "description", "Acceptance Test foo"),
				),
			},
			{
				Config: testAccClusterAgentTokenBasic("bar"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the token exists in the buildkite API
					testAccCheckClusterAgentTokenExists("buildkite_agent_token.foobar", &ct),
					// Confirm the token has the correct values in Buildkite's system
					testAccCheckClusterAgentTokenRemoteValues(&ct, "Acceptance Test foobar"),
					// Confirm the token has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_agent_token.foobar", "description", "Acceptance Test bar"),
				),
			},
		},
	})
}

func testAccCheckClusterAgentTokenExists(resourceName string, ct *ClusterAgentTokenResourceModel) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		resourceState, ok := s.RootModule().Resources[resourceName]

		if !ok {
			return fmt.Errorf("Not found in state: %s", resourceName)
		}

		if resourceState.Primary.ID == "" {
			return fmt.Errorf("No ID is set in state")
		}

		clusterTokens, err := getClusterAgentTokens(
			genqlientGraphql,
			getenv("BUILDKITE_ORGANIZATION_SLUG"),
			resourceState.Primary.Attributes["cluster_uuid"],
		)

		if err != nil {
			return fmt.Errorf("Error fetching Cluster Agent Tokens from graphql API: %v", err)
		}

		// Obtain the ClusterAgentTokenResourceModel
		for _, edge := range clusterTokens.Organization.Cluster.AgentTokens.Edges {
			if edge.Node.Id == resourceState.Primary.ID {
				ct.Id = types.StringValue(edge.Node.Id)
				ct.Uuid = types.StringValue(edge.Node.Uuid)
				ct.Description = types.StringValue(edge.Node.Description)
				break
			}
		}

		// If ClusterAgentTokenResourceModel isnt set from the queues slice
		if ct.Id.ValueString() == "" {
			return fmt.Errorf("No Cluster agent token found with graphql id: %s", resourceState.Primary.ID)
		}

		return nil
	}
}

func testAccCheckClusterAgentTokenRemoteValues(ct *ClusterAgentTokenResourceModel, description string) resource.TestCheckFunc {
	return func(s *terraform.State) error {

		if ct.Description.ValueString() != description {
			return fmt.Errorf("Remote Cluster queue description (%s) doesn't match expected value (%s)", ct.Description, description)
		}

		return nil
	}
}

func testAccCheckClusterAgentTokenDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_cluster_agent_token" {
			continue
		}

		clusterTokens, err := getClusterAgentTokens(
			genqlientGraphql,
			getenv("BUILDKITE_ORGANIZATION_SLUG"),
			rs.Primary.Attributes["cluster_uuid"],
		)

		if err != nil {
			return fmt.Errorf("Error fetching Cluster Agent Tokens from graphql API: %v", err)
		}

		// Obtain the ClusterAgentTokenResourceModel
		for _, edge := range clusterTokens.Organization.Cluster.AgentTokens.Edges {
			if edge.Node.Id == rs.Primary.ID {
				return fmt.Errorf("Cluster agent token still exists in cluster, expected not to find it")
			}
		}

		return nil
	}

	return nil
}
