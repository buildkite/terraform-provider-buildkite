package buildkite

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func testDatasourceOrganizationMemberConfig() string {
	return `
	  data "buildkite_organization_members" "members" {}

		data "buildkite_organization_member" "member" {
			email = data.buildkite_organization_members.members.members[0].email
		}
	`
}

func TestAccBuildkiteOrganizationMemberDatasource(t *testing.T) {
	t.Run("organization members data source can be loaded with defaults", func(t *testing.T) {
		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: testDatasourceOrganizationMemberConfig(),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttrSet("data.buildkite_organization_member.member", "id"),
					),
				},
			},
		})
	})
}

func testAccCheckOrganizationMemberExists(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		// retrieve the resource by name from state
		_, err := s.RootModule().Resources[resourceName]
		if err {
			return fmt.Errorf("Not found: %s", resourceName)
		}
		return nil
	}
}
