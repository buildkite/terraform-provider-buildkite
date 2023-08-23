package buildkite

import (
	"context"
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
					return c.ID.ValueString(), nil
				},
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
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
