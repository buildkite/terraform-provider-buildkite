package buildkite

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov5"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func testAccClusterBasic(name string) string {
	config := `
		resource "buildkite_cluster" "foo" {
			name = "%s_test_cluster"
			description = "Test cluster"
		}
	`
	return fmt.Sprintf(config, name)
}

func protoV5ProviderFactories() map[string]func() (tfprotov5.ProviderServer, error) {
	return map[string]func() (tfprotov5.ProviderServer, error){
		"buildkite": providerserver.NewProtocol5WithError(New("testing")),
	}
}

func TestAccCluster_AddRemove(t *testing.T) {
	t.Parallel()
	var c ClusterResourceModel

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		CheckDestroy:             testAccCheckClusterDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccClusterBasic("foo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckClusterExists("buildkite_cluster.foo", &c),
					testAccCheckClusterRemoteValues(&c, "Test cluster"),
				),
			},
			{
				RefreshState: true,
				PlanOnly:     true,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("buildkite_cluster.foo", "name"),
					resource.TestCheckResourceAttrSet("buildkite_cluster.foo", "description"),
				),
			},
		},
	})
}

func TestAccCluster_Update(t *testing.T) {
	t.Parallel()
	var c ClusterResourceModel

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV5ProviderFactories: protoV5ProviderFactories(),
		CheckDestroy:             testAccCheckClusterDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccClusterBasic("bar"),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckClusterExists("buildkite_cluster.foo", &c),
					resource.TestCheckResourceAttr("buildkite_cluster.foo", "name", "bar_test_cluster"),
				),
			},
			{
				Config: testAccClusterBasic("baz"),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckClusterExists("buildkite_cluster.foo", &c),
					resource.TestCheckResourceAttr("buildkite_cluster.foo", "name", "baz_test_cluster"),
				),
			},
		},
	})
}

func testAccCheckClusterExists(n string, c *ClusterResourceModel) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("cluster not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no cluster ID is set")
		}

		_, err := getCluster(genqlientGraphql, getenv("BUILDKITE_ORGANIZATION_SLUG"), rs.Primary.Attributes["uuid"])

		if err != nil {
			return err
		}
		return nil
	}
}

func testAccCheckClusterRemoteValues(c *ClusterResourceModel, description string) resource.TestCheckFunc {
	return func(s *terraform.State) error {

		if string(c.Description.ValueString()) != description {
			return fmt.Errorf("unexpected description: %s", c.Description)
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
