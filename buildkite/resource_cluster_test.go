package buildkite

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccCluster(t *testing.T) {
	var c clusterResourceModel
	basic := func(name string) string {
		return fmt.Sprintf(`
		resource "buildkite_cluster" "foo" {
			name = "%s_test_cluster"
		}
		`, name)
	}

	complex := func(name string) string {
		return fmt.Sprintf(`
		resource "buildkite_cluster" "foo" {
			name = "%s_test_cluster"
			description = "Just another Buildkite cluster"
			emoji = ":one-does-not-simply:"
			color = "#BADA55"
		}
		`, name)
	}

	t.Run("Creates a Cluster", func(t *testing.T) {
		randName := acctest.RandString(5)
		check := resource.ComposeAggregateTestCheckFunc(
			testAccCheckClusterExists("buildkite_cluster.foo", &c),
			testAccCheckClusterRemoteValues(&c, fmt.Sprintf("%s_test_cluster", randName)),
			resource.TestCheckResourceAttr("buildkite_cluster.foo", "name", fmt.Sprintf("%s_test_cluster", randName)),
			resource.TestCheckResourceAttrSet("buildkite_cluster.foo", "id"),
			resource.TestCheckResourceAttrSet("buildkite_cluster.foo", "uuid"),
		)

		testCase := func(t *testing.T, config string) {
			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckClusterDestroy,
				Steps: []resource.TestStep{
					{
						Config: config,
						Check:  check,
					},
				},
			})
		}
		t.Run("with basic settings", func(t *testing.T) {
			testCase(t, basic(randName))
		})

		t.Run("with complex settings", func(t *testing.T) {
			testCase(t, complex(randName))
		})
	})

	t.Run("Updates a Cluster", func(t *testing.T) {
		randName := acctest.RandString(5)
		randNameUpdated := acctest.RandString(5)
		testCase := func(t *testing.T, originalConfig, newConfig string) {
			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckClusterDestroy,
				Steps: []resource.TestStep{
					{
						Config: originalConfig,
						Check: resource.ComposeAggregateTestCheckFunc(
							testAccCheckClusterExists("buildkite_cluster.foo", &c),
							testAccCheckClusterRemoteValues(&c, fmt.Sprintf("%s_test_cluster", randName)),
							resource.TestCheckResourceAttr("buildkite_cluster.foo", "name", fmt.Sprintf("%s_test_cluster", randName)),
							resource.TestCheckResourceAttrSet("buildkite_cluster.foo", "id"),
							resource.TestCheckResourceAttrSet("buildkite_cluster.foo", "uuid"),
						),
					},
					{
						Config: newConfig,
						Check: resource.ComposeAggregateTestCheckFunc(
							testAccCheckClusterExists("buildkite_cluster.foo", &c),
							testAccCheckClusterRemoteValues(&c, fmt.Sprintf("%s_test_cluster", randNameUpdated)),
							resource.TestCheckResourceAttr("buildkite_cluster.foo", "name", fmt.Sprintf("%s_test_cluster", randNameUpdated)),
							resource.TestCheckResourceAttrSet("buildkite_cluster.foo", "id"),
							resource.TestCheckResourceAttrSet("buildkite_cluster.foo", "uuid"),
						),
					},
				},
			})
		}
		t.Run("with basic settings", func(t *testing.T) {
			testCase(t, basic(randName), basic(randNameUpdated))
		})
	})

	t.Run("Imports a Cluster", func(t *testing.T) {
		randName := acctest.RandString(5)
		check := resource.ComposeAggregateTestCheckFunc(
			testAccCheckClusterExists("buildkite_cluster.foo", &c),
			resource.TestCheckResourceAttr("buildkite_cluster.foo", "name", fmt.Sprintf("%s_test_cluster", randName)),
		)

		testCase := func(t *testing.T, config string) {
			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckClusterDestroy,
				Steps: []resource.TestStep{
					{
						Config: config,
						Check:  check,
					},
					{
						ResourceName: "buildkite_cluster.foo",
						ImportStateIdFunc: func(s *terraform.State) (string, error) {
							return c.UUID.ValueString(), nil
						},
						ImportState:       true,
						ImportStateVerify: true,
					},
				},
			})
		}
		t.Run("with basic settings", func(t *testing.T) {
			testCase(t, basic(randName))
		})
		t.Run("with complex settings", func(t *testing.T) {
			testCase(t, complex(randName))
		})
	})
}

func testAccCheckClusterExists(n string, c *clusterResourceModel) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("cluster not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no cluster ID is set")
		}

		r, err := getCluster(genqlientGraphql, getenv("BUILDKITE_ORGANIZATION_SLUG"), rs.Primary.Attributes["uuid"])

		if err != nil {
			return err
		}

		updateClusterResourceState(r.Organization.Cluster, c)
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

		_, err := getCluster(genqlientGraphql, getenv("BUILDKITE_ORGANIZATION_SLUG"), rs.Primary.Attributes["uuid"])

		if err == nil {
			return fmt.Errorf("Cluster still exists")
		}
		return nil
	}
	return nil
}
