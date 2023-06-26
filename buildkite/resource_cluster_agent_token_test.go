package buildkite

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func testAccClusterAgentTokenBasic(description string) string {
	config := `
		resource "buildkite_cluster_agent_token" "foobar" {
			cluster_id = "acceptance_test_cluster_id"
			description = "Acceptance Test %s"
		}
	`
	return fmt.Sprintf(config, description)
}

func TestAccClusterAgentToken_AddRemove(t *testing.T) {
	t.Parallel()
	var c ClusterAgentTokenResourceModel

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckClusterAgentTokenDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccClusterAgentTokenBasic("foo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckClusterAgentTokenRemoteValues(c.Id.ValueString()),
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

func TestAccClusterAgentToken_Update(t *testing.T) {
	t.Parallel()
	var c ClusterAgentTokenResourceModel

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckClusterAgentTokenDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccClusterAgentTokenBasic("bar"),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckClusterAgentTokenRemoteValues(c.Id.ValueString()),
					resource.TestCheckResourceAttr("buildkite_cluster_agent_token.foobar", "description", "Acceptance Test bar"),
				),
			},
			{
				Config: testAccClusterAgentTokenBasic("bat"),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckClusterAgentTokenRemoteValues(c.Id.ValueString()),
					resource.TestCheckResourceAttr("buildkite_cluster_agent_token.foobar", "description", "Acceptance Test bat"),
				),
			},
		},
	})
}

func testAccCheckClusterAgentTokenRemoteValues(id string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		return nil
	}
}

func testAccCheckClusterAgentTokenDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_cluster_agent_token" {
			continue
		}
	}

	return nil
}
