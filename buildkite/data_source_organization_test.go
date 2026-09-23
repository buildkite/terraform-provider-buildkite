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
					// the identifiers are what a default lookup is for, and they resolve for a caller
					// who cannot see the settings. The allowlist is organization-wide state another
					// test presets, so this one does not assert on it.
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttrSet("data.buildkite_organization.settings", "id"),
						resource.TestCheckResourceAttrSet("data.buildkite_organization.settings", "uuid"),
					),
				},
			},
		})
	})
}
