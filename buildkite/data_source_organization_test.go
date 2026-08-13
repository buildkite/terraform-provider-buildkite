package buildkite

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccBuildkiteOrganizationDatasource(t *testing.T) {
	t.Run("organization data source can be loaded with defaults", func(t *testing.T) {
		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: `data "buildkite_organization" "settings" {}`,
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttrSet("data.buildkite_organization.settings", "id"),
						resource.TestCheckResourceAttrSet("data.buildkite_organization.settings", "uuid"),
						// Set either way: an organization with no allowlist reports an empty list rather than
						// one holding a single empty CIDR.
						resource.TestCheckResourceAttrSet("data.buildkite_organization.settings", "allowed_api_ip_addresses.#"),
					),
				},
			},
		})
	})
}
