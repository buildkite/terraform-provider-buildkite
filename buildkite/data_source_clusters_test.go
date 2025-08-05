package buildkite

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccBuildkiteClustersDatasource(t *testing.T) {
	t.Run("clusters data source can be loaded with defaults", func(t *testing.T) {
		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: `data "buildkite_clusters" "clusters" {}`,
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttrSet("data.buildkite_clusters.clusters", "clusters.0.name"),
					),
				},
			},
		})
	})
}
