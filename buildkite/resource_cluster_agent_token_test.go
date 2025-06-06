package buildkite

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccBuildkiteClusterAgentTokenResource(t *testing.T) {
	configBasic := func(fields ...string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "10s"
				read = "10s"
				update = "10s"
				delete = "10s"
			}
		}

		resource "buildkite_cluster" "cluster_test" {
			name = "Test cluster %s"
			description = "Acceptance testing cluster"
		}

		resource "buildkite_cluster_agent_token" "foobar" {
			cluster_id = buildkite_cluster.cluster_test.id
			description = "Acceptance Test %s"
		}

		`, fields[0], fields[1])
	}

	configAllowedIPsBasic := func(name, description string, allowed_ip_addresses []string) string {
		config := `

		provider "buildkite" {
			timeouts = {
				create = "10s"
				read = "10s"
				update = "10s"
				delete = "10s"
			}
		}

		resource "buildkite_cluster" "cluster_test" {
			name = "Test cluster %s"
			description = "Acceptance testing cluster"
		}

		resource "buildkite_cluster_agent_token" "foobar" {
			cluster_id = buildkite_cluster.cluster_test.id
			description = "Acceptance Test %s"
			allowed_ip_addresses = %v
		}
		`

		marshalled_ips, _ := json.Marshal(allowed_ip_addresses)

		return fmt.Sprintf(config, name, description, string(marshalled_ips))
	}

	t.Run("creates a cluster agent token", func(t *testing.T) {
		var ct clusterAgentTokenResourceModel
		clusterName := acctest.RandString(10)
		tokenDesc := acctest.RandString(10)

		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm the token exists in the buildkite API
			testAccCheckClusterAgentTokenExists("buildkite_cluster_agent_token.foobar", &ct),
			// Confirm the token has the correct values in Buildkite's system
			testAccCheckClusterAgentTokenRemoteValues(&ct, fmt.Sprintf("Acceptance Test %s", tokenDesc)),
			// Confirm the token has the correct values in terraform state
			resource.TestCheckResourceAttr("buildkite_cluster_agent_token.foobar", "description", fmt.Sprintf("Acceptance Test %s", tokenDesc)),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterAgentTokenDestroy,
			Steps: []resource.TestStep{
				{
					Config: configBasic(clusterName, tokenDesc),
					Check:  check,
				},
				{
					RefreshState: true,
					PlanOnly:     true,
					Check: resource.ComposeAggregateTestCheckFunc(
						// Confirm the token has the correct values in terraform state
						resource.TestCheckResourceAttr("buildkite_cluster_agent_token.foobar", "description", fmt.Sprintf("Acceptance Test %s", tokenDesc)),
					),
				},
			},
		})
	})

	t.Run("creates a cluster agent token with allowed IPs", func(t *testing.T) {
		var ct clusterAgentTokenResourceModel
		clusterName := acctest.RandString(10)
		tokenDesc := acctest.RandString(10)
		allowedIps := []string{"10.100.1.0/28"}

		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm the token exists in the buildkite API
			testAccCheckClusterAgentTokenExists("buildkite_cluster_agent_token.foobar", &ct),
			// Confirm the token has the correct values in Buildkite's system
			testAccCheckClusterAgentTokenRemoteValues(&ct, fmt.Sprintf("Acceptance Test %s", tokenDesc)),
			// Confirm the token has the correct values in terraform state
			resource.TestCheckResourceAttr("buildkite_cluster_agent_token.foobar", "description", fmt.Sprintf("Acceptance Test %s", tokenDesc)),
			resource.TestCheckResourceAttr("buildkite_cluster_agent_token.foobar", "allowed_ip_addresses.#", "1"),
			resource.TestCheckResourceAttr("buildkite_cluster_agent_token.foobar", "allowed_ip_addresses.0", "10.100.1.0/28"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterAgentTokenDestroy,
			Steps: []resource.TestStep{
				{
					Config: configAllowedIPsBasic(clusterName, tokenDesc, allowedIps),
					Check:  check,
				},
			},
		})
	})

	t.Run("updates a cluster agent token", func(t *testing.T) {
		var ct clusterAgentTokenResourceModel
		clusterName := acctest.RandString(10)
		tokenDesc := acctest.RandString(10)
		updatedTokenDesc := acctest.RandString(10)

		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm the token exists in the buildkite API
			testAccCheckClusterAgentTokenExists("buildkite_cluster_agent_token.foobar", &ct),
			// Confirm the token has the correct values in Buildkite's system
			testAccCheckClusterAgentTokenRemoteValues(&ct, fmt.Sprintf("Acceptance Test %s", tokenDesc)),
			// Confirm the token has the correct values in terraform state
			resource.TestCheckResourceAttr("buildkite_cluster_agent_token.foobar", "description", fmt.Sprintf("Acceptance Test %s", tokenDesc)),
		)

		ckecUpdated := resource.ComposeAggregateTestCheckFunc(
			// Confirm the token exists in the buildkite API
			testAccCheckClusterAgentTokenExists("buildkite_cluster_agent_token.foobar", &ct),
			// Confirm the token has the correct values in Buildkite's system
			testAccCheckClusterAgentTokenRemoteValues(&ct, fmt.Sprintf("Acceptance Test %s", updatedTokenDesc)),
			// Confirm the token has the correct values in terraform state
			resource.TestCheckResourceAttr("buildkite_cluster_agent_token.foobar", "description", fmt.Sprintf("Acceptance Test %s", updatedTokenDesc)),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterAgentTokenDestroy,
			Steps: []resource.TestStep{
				{
					Config: configBasic(clusterName, tokenDesc),
					Check:  check,
				},
				{
					Config: configBasic(clusterName, updatedTokenDesc),
					Check:  ckecUpdated,
				},
			},
		})
	})
}

func testAccCheckClusterAgentTokenExists(resourceName string, ct *clusterAgentTokenResourceModel) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		resourceState, ok := s.RootModule().Resources[resourceName]

		if !ok {
			return fmt.Errorf("Not found in state: %s", resourceName)
		}

		if resourceState.Primary.ID == "" {
			return fmt.Errorf("No ID is set in state")
		}
		clusterTokens, err := getClusterAgentTokens(
			context.Background(),
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

		// If ClusterAgentTokenResourceModel isnt set from the token slice
		if ct.Id.ValueString() == "" {
			return fmt.Errorf("No Cluster agent token found with graphql id: %s", resourceState.Primary.ID)
		}

		return nil
	}
}

func testAccCheckClusterAgentTokenRemoteValues(ct *clusterAgentTokenResourceModel, description string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if ct.Description.ValueString() != description {
			return fmt.Errorf("Remote Cluster agent token description (%s) doesn't match expected value (%s)", ct.Description, description)
		}

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
