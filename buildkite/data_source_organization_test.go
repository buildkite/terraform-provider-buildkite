package buildkite

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
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
						testAccCheckOrganizationDatasourceAllowlist(),
					),
				},
			},
		})
	})
}

// testAccCheckOrganizationDatasourceAllowlist pins the count against what the organization actually
// has. An organization with no allowlist has to report none: the provider version this replaces
// reported a one-element list holding an empty CIDR, which is what a bare "is the count set" check
// cannot tell apart.
func testAccCheckOrganizationDatasourceAllowlist() resource.TestCheckFunc {
	return func(s *terraform.State) error {
		organization, err := getOrganization(context.Background(), genqlientGraphql, getenv("BUILDKITE_ORGANIZATION_SLUG"))
		if err != nil {
			return err
		}

		want := len(strings.Fields(organization.Organization.AllowedApiIpAddresses))
		check := resource.TestCheckResourceAttr("data.buildkite_organization.settings", "allowed_api_ip_addresses.#", strconv.Itoa(want))
		if err := check(s); err != nil {
			return err
		}

		for i := range want {
			if err := resource.TestCheckResourceAttrSet("data.buildkite_organization.settings", fmt.Sprintf("allowed_api_ip_addresses.%d", i))(s); err != nil {
				return err
			}
		}

		return nil
	}
}
