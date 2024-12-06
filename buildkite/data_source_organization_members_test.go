package buildkite

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccBuildkiteOrganizationMembersDatasource(t *testing.T) {
	t.Run("organization members data source can be loaded with defaults", func(t *testing.T) {
		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: `data "buildkite_organization_members" "members" {}`,
					Check: resource.ComposeTestCheckFunc(
						testAccCheckOrganizationMembersExist("data.buildkite_organization_members.members"),
					),
				},
			},
		})
	})
}

func testAccCheckOrganizationMembersExist(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		// retrieve the resource by name from state
		_, err := s.RootModule().Resources[resourceName]
		if err != nil {
			return fmt.Errorf("Not found: %s", resourceName)
		}
		return nil
	}
}
