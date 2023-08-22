package buildkite

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccDataOrganization(t *testing.T) {
	t.Run("organization data source can be loaded from slug", func(t *testing.T) {
		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: testAccOrganizationSettingsConfigBasic([]string{"0.0.0.0/0", "1.1.1.1/32", "1.0.0.1/32"}),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("data.buildkite_organization.settings", "allowed_api_ip_addresses.2", "1.0.0.1/32"),
					),
				},
			},
		})
	})

}

func testAccOrganizationSettingsConfigBasic(ip_addresses []string) string {
	config := `
		data "buildkite_organization" "settings" {
      		allowed_api_ip_addresses = %v
		}
	`
	marshal, _ := json.Marshal(ip_addresses)
	return fmt.Sprintf(config, string(marshal))
}
