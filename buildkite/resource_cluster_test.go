package buildkite

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func testAccClusterBasic(name string) string {
	config := `
		resource "buildkite_cluster" "foo" {
			name = "%s_test_cluster"
		}
	`
	return fmt.Sprintf(config, name)
}

func TestAccCluster_AddRemove(t *testing.T) {
	resName := acctest.RandString(12)
	t.Parallel()
	var c clusterResourceModel

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckClusterDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccClusterBasic(resName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckClusterExists("buildkite_cluster.foo", &c),
					testAccCheckClusterRemoteValues(&c, fmt.Sprintf("%s_test_cluster", resName)),
					resource.TestCheckResourceAttr("buildkite_cluster.foo", "name", fmt.Sprintf("%s_test_cluster", resName)),
					resource.TestCheckResourceAttrSet("buildkite_cluster.foo", "id"),
					resource.TestCheckResourceAttrSet("buildkite_cluster.foo", "uuid"),
				),
			},
			{
				RefreshState: true,
				PlanOnly:     true,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("buildkite_cluster.foo", "name"),
				),
			},
		},
	})
}

func TestAccCluster_Update(t *testing.T) {
	resName := acctest.RandString(12)
	newResName := acctest.RandString(15)
	t.Parallel()
	var c = new(clusterResourceModel)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckClusterDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccClusterBasic(resName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckClusterExists("buildkite_cluster.foo", c),
					testAccCheckClusterRemoteValues(c, fmt.Sprintf("%s_test_cluster", resName)),
					resource.TestCheckResourceAttr("buildkite_cluster.foo", "name", fmt.Sprintf("%s_test_cluster", resName)),
					resource.TestCheckResourceAttrSet("buildkite_cluster.foo", "id"),
					resource.TestCheckResourceAttrSet("buildkite_cluster.foo", "uuid"),
				),
			},
			{
				Config: testAccClusterBasic(newResName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckClusterExists("buildkite_cluster.foo", c),
					testAccCheckClusterRemoteValues(c, fmt.Sprintf("%s_test_cluster", newResName)),
					resource.TestCheckResourceAttr("buildkite_cluster.foo", "name", fmt.Sprintf("%s_test_cluster", newResName)),
				),
			},
		},
	})
}

func TestAccCluster_Import(t *testing.T) {
	resName := acctest.RandString(13)
	t.Parallel()
	var c = new(clusterResourceModel)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckClusterDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccClusterBasic(resName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckClusterExists("buildkite_cluster.foo", c),
					resource.TestCheckResourceAttr("buildkite_cluster.foo", "name", fmt.Sprintf("%s_test_cluster", resName)),
				),
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
