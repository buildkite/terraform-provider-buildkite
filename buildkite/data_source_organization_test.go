package buildkite

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccDataOrganization(t *testing.T) {
	t.Parallel()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testCheckOrganizationSettingsResourceRemoved,
		Steps: []resource.TestStep{
			{
				Config: testDatasourceOrganization(),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm that the allowed IP addresses are set correctly in Buildkite's system
					testAccCheckOrganizationSettingsRemoteValues([]string{"0.0.0.0/0", "1.1.1.1/32", "1.0.0.1/32"}),
					// Check that the second IP added to the list is the one we expect, this also ensures the length is greater than 1
					// allowing us to assert the first IP is also added correctly
					resource.TestCheckResourceAttr("data.buildkite_organization.settings", "allowed_api_ip_addresses.2", "1.0.0.1/32"),
				),
			},
		},
	})
}

func testDatasourceOrganization() string {
	data := `
	%s
	data "buildkite_organization" "settings" {
	  depends_on = [buildkite_organization_settings.let_them_in]
	}
	`
	return fmt.Sprintf(data, testAccOrganizationSettingsConfigBasic([]string{"0.0.0.0/0", "1.1.1.1/32", "1.0.0.1/32"}))
}
