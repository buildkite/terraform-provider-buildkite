package buildkite

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccBuildkiteClusterResource(t *testing.T) {
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

		resource "buildkite_cluster" "foo" {
			name = "%s_test_cluster"
		}
		`, name)
	}

	complex := func(fields ...string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "10s"
				read = "10s"
				update = "10s"
				delete = "10s"
			}
		}

		resource "buildkite_cluster" "foo" {
			name = "%s_test_cluster"
			description = "Just another Buildkite cluster"
			emoji = "%s"
			color = "%s"
		}
		`, fields[0], fields[1], fields[2])
	}

	t.Run("Creates a Cluster with basic settings", func(t *testing.T) {
		var c clusterResourceModel
		randName := acctest.RandString(5)
		check := resource.ComposeAggregateTestCheckFunc(
			testAccCheckClusterExists("buildkite_cluster.foo", &c),
			testAccCheckClusterRemoteValues(&c, fmt.Sprintf("%s_test_cluster", randName)),
			resource.TestCheckResourceAttr("buildkite_cluster.foo", "name", fmt.Sprintf("%s_test_cluster", randName)),
			resource.TestCheckResourceAttrSet("buildkite_cluster.foo", "id"),
			resource.TestCheckResourceAttrSet("buildkite_cluster.foo", "uuid"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterDestroy,
			Steps: []resource.TestStep{
				{
					Config: basic(randName),
					Check:  check,
				},
			},
		})
	})

	t.Run("Creates a Cluster with complex settings", func(t *testing.T) {
		var c clusterResourceModel
		randName := acctest.RandString(5)
		check := resource.ComposeAggregateTestCheckFunc(
			testAccCheckClusterExists("buildkite_cluster.foo", &c),
			testAccCheckClusterRemoteValues(&c, fmt.Sprintf("%s_test_cluster", randName)),
			resource.TestCheckResourceAttr("buildkite_cluster.foo", "name", fmt.Sprintf("%s_test_cluster", randName)),
			resource.TestCheckResourceAttrSet("buildkite_cluster.foo", "id"),
			resource.TestCheckResourceAttrSet("buildkite_cluster.foo", "uuid"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterDestroy,
			Steps: []resource.TestStep{
				{
					Config: complex(randName, ":triple-green-shell:", "#6932f9"),
					Check:  check,
				},
			},
		})
	})

	t.Run("Updates a Cluster using complex settings", func(t *testing.T) {
		var c clusterResourceModel
		randName := acctest.RandString(5)
		randNameUpdated := acctest.RandString(5)
		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterDestroy,
			Steps: []resource.TestStep{
				{
					Config: complex(randName, ":one-does-not-simply:", "#BADA55"),
					Check: resource.ComposeAggregateTestCheckFunc(
						testAccCheckClusterExists("buildkite_cluster.foo", &c),
						testAccCheckClusterRemoteValues(&c, fmt.Sprintf("%s_test_cluster", randName)),
						resource.TestCheckResourceAttr("buildkite_cluster.foo", "name", fmt.Sprintf("%s_test_cluster", randName)),
						resource.TestCheckResourceAttrSet("buildkite_cluster.foo", "id"),
						resource.TestCheckResourceAttrSet("buildkite_cluster.foo", "uuid"),
						resource.TestCheckResourceAttr("buildkite_cluster.foo", "emoji", ":one-does-not-simply:"),
						resource.TestCheckResourceAttr("buildkite_cluster.foo", "color", "#BADA55"),
					),
				},
				{
					Config: complex(randNameUpdated, ":terraform:", "#b31625"),
					Check: resource.ComposeAggregateTestCheckFunc(
						testAccCheckClusterExists("buildkite_cluster.foo", &c),
						testAccCheckClusterRemoteValues(&c, fmt.Sprintf("%s_test_cluster", randNameUpdated)),
						resource.TestCheckResourceAttr("buildkite_cluster.foo", "name", fmt.Sprintf("%s_test_cluster", randNameUpdated)),
						resource.TestCheckResourceAttrSet("buildkite_cluster.foo", "id"),
						resource.TestCheckResourceAttrSet("buildkite_cluster.foo", "uuid"),
						resource.TestCheckResourceAttrSet("buildkite_cluster.foo", "description"),
						resource.TestCheckResourceAttr("buildkite_cluster.foo", "emoji", ":terraform:"),
						resource.TestCheckResourceAttr("buildkite_cluster.foo", "color", "#b31625"),
					),
				},
			},
		})
	})

	t.Run("Imports a Cluster", func(t *testing.T) {
		var c clusterResourceModel
		randName := acctest.RandString(5)
		check := resource.ComposeAggregateTestCheckFunc(
			testAccCheckClusterExists("buildkite_cluster.foo", &c),
			resource.TestCheckResourceAttr("buildkite_cluster.foo", "name", fmt.Sprintf("%s_test_cluster", randName)),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterDestroy,
			Steps: []resource.TestStep{
				{
					Config: basic(randName),
					Check:  check,
				},
				{
					ResourceName:      "buildkite_cluster.foo",
					ImportState:       true,
					ImportStateVerify: true,
				},
			},
		})
	})
}

func testAccCheckClusterExists(name string, c *clusterResourceModel) resource.TestCheckFunc {
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

		if clusterNode, ok := r.GetNode().(*getNodeNodeCluster); ok {
			if clusterNode == nil {
				return fmt.Errorf("Team not found: nil response")
			}
			updateClusterResourceState(c, *clusterNode)
		}
		return nil
	}
}

func testAccCheckClusterRemoteValues(c *clusterResourceModel, name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if c.Name.ValueString() != name {
			return fmt.Errorf("unexpected name: %s, wanted: %s", c.Name, name)
		}
		return nil
	}
}

func testAccCheckClusterDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_cluster" {
			continue
		}

		r, err := getNode(context.Background(), genqlientGraphql, rs.Primary.ID)
		if err != nil {
			return err
		}

		if clusterNode, ok := r.GetNode().(*getNodeNodeCluster); ok {
			if clusterNode != nil {
				return fmt.Errorf("Cluster still exists: %v", clusterNode)
			}
		}
	}
	return nil
}
