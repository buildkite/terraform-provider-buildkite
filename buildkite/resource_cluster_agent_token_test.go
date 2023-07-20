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
					testAccCheckClusterAgentTokenExists("buildkite_cluster_agent_token.foobar", &ct),
					// Confirm the token has the correct values in Buildkite's system
					testAccCheckClusterAgentTokenRemoteValues(&ct, "Acceptance Test foo"),
					// Confirm the token has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_cluster_agent_token.foobar", "description", "Acceptance Test foo"),
				),
			},
			{
				RefreshState: true,
				PlanOnly:     true,
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the token has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_cluster_agent_token.foobar", "description", "Acceptance Test foo"),
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
					testAccCheckClusterAgentTokenExists("buildkite_cluster_agent_token.foobar", &ct),
					// Confirm the token has the correct values in Buildkite's system
					testAccCheckClusterAgentTokenRemoteValues(&ct, "Acceptance Test foo"),
					// Confirm the token has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_cluster_agent_token.foobar", "description", "Acceptance Test foo"),
				),
			},
			{
				Config: testAccClusterAgentTokenBasic("bar"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the token exists in the buildkite API
					testAccCheckClusterAgentTokenExists("buildkite_cluster_agent_token.foobar", &ct),
					// Confirm the token has the correct values in Buildkite's system
					testAccCheckClusterAgentTokenRemoteValues(&ct, "Acceptance Test bar"),
					// Confirm the token has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_cluster_agent_token.foobar", "description", "Acceptance Test bar"),
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

		apiResponse, err := getNode(genqlientGraphql, resourceState.Primary.ID)

		if err != nil {
			return fmt.Errorf("Error fetching Cluster Agent Token from graphql API: %v", err)
		}

		if clusterAgentTokenNode, ok := apiResponse.GetNode().(*getNodeNodeClusterToken); ok {
			if clusterAgentTokenNode == nil {
				return fmt.Errorf("Error getting Cluster Agent Token: nil response")
			}
			ct.Id = types.StringValue(clusterAgentTokenNode.Id)
			ct.Uuid = types.StringValue(clusterAgentTokenNode.Uuid)
			ct.Description = types.StringValue(clusterAgentTokenNode.Description)
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

		apiResponse, err := getNode(genqlientGraphql, rs.Primary.ID)

		if err != nil {
			return fmt.Errorf("Error fetching Cluster Agent Tokens from graphql API: %v", err)
		}

		// Obtain the ClusterAgentTokenResourceModel
		if clusterAgentTokenNode, ok := apiResponse.GetNode().(*getNodeNodeClusterToken); ok {
			if clusterAgentTokenNode != nil {
				return fmt.Errorf("Cluster agent token still exists in cluster, expected not to find it")
			}
		}
	}

	return nil
}
