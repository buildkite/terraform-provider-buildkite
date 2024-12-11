package buildkite

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
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
	t.Run("loads an organization member data source with required attribute", func(t *testing.T) {
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
