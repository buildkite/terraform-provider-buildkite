package buildkite

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestDataCluster_read(t *testing.T) {
	t.Parallel()
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				Config: `data "buildkite_cluster" "default" {
							name = "acceptance testing"
						}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.buildkite_cluster.default", "uuid", "0a969b74-a0b7-4340-aace-d874423f3d6c"),
					resource.TestCheckResourceAttr("data.buildkite_cluster.default", "color", "#f1efff"),
				),
			},
		},
	})
}

func TestDataCluster_read_not_exists(t *testing.T) {
	t.Parallel()
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				Config: `data "buildkite_cluster" "default" {
							name = "doesn't exist"
						}`,
				ExpectError: regexp.MustCompile("Unable to find Cluster"),
			},
		},
	})
}
