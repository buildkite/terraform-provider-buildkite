package buildkite

import (
	"context"
	"fmt"
	"os"
	"testing"

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

func TestAccCluster_AddRemove(t *testing.T) {
	t.Parallel()
	var c ClusterResourceModel

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckClusterDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccClusterBasic("foo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckClusterRemoteValues(c.ID.ValueString()),
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
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckClusterDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccClusterBasic("bar"),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckClusterRemoteValues(c.ID.ValueString()),
					resource.TestCheckResourceAttr("buildkite_cluster.foo", "name", "bar_test_cluster"),
				),
			},
			{
				Config: testAccClusterBasic("baz"),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckClusterRemoteValues(c.ID.ValueString()),
					resource.TestCheckResourceAttr("buildkite_cluster.foo", "name", "baz_test_cluster"),
				),
			},
		},
	})
}

func testAccCheckClusterRemoteValues(id string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		resp, err := getCluster(genqlientGraphql, os.Getenv("BUILDKITE_ORGANIZATION_SLUG"), id)

		if err != nil {
			return err
		}

		if resp.Organization.Cluster.Name != id {
			return fmt.Errorf("Cluster name does not match")
		}
		return nil
	}
}

func testAccCheckClusterDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_cluster" {
			continue
		}

		var getClusterQuery struct {
			Organization struct {
				Cluster struct {
					ID string `graphql:"id"`
				}
			}
		}

		err := graphqlClient.Query(context.Background(), &getClusterQuery, map[string]interface{}{
			"id": rs.Primary.ID,
		})

		if err == nil {
			return fmt.Errorf("Cluster still exists")
		}
		return nil
	}
	return nil
}
