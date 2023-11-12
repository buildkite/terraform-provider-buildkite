package buildkite

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccBuildkiteOrganizationBannerResource(t *testing.T) {

	config := func(name string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "10s"
				read = "10s"
				update = "10s"
				delete = "10s"
			}
		}

		resource "buildkite_organization_banner" "banner_foo" {
			message = ":buildkite: Test %s organization banner! :buildkite:"
		}
		`, name)
	}

	t.Run("creates an organization banner", func(t *testing.T) {
		message := acctest.RandString(12)
		var obr organizationBannerResourceModel

		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm the organization banner exists in the buildkite API
			testAccCheckOrganizationBannerExists(&obr, "buildkite_organization_banner.banner_foo"),
			// Confirm the organization banner has the correct values in Buildkite's system
			testAccCheckOrganizationBannerRemoteValues(&obr, fmt.Sprintf(":buildkite: Test %s organization banner! :buildkite:", message)),
			// Check all organization banner resource attributes are set in state (required attributes)
			resource.TestCheckResourceAttrSet("buildkite_organization_banner.banner_foo", "id"),
			resource.TestCheckResourceAttrSet("buildkite_organization_banner.banner_foo", "uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_banner.banner_foo", "message"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationBannerDestroy,
			Steps: []resource.TestStep{
				{
					Config: config(message),
					Check:  check,
				},
			},
		})
	})

	t.Run("updates an organization banner", func(t *testing.T) {
		t.Skip()
		message := acctest.RandString(12)
		updatedMessage := acctest.RandString(12)
		var obr organizationBannerResourceModel

		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm the organization banner exists in the buildkite API
			testAccCheckOrganizationBannerExists(&obr, "buildkite_organization_banner.banner_foo"),
			// Confirm the organization banner has the correct values in Buildkite's system
			testAccCheckOrganizationBannerRemoteValues(&obr, fmt.Sprintf(":buildkite: Test %s organization banner! :buildkite:", message)),
			// Check all organization banner resource attributes are set in state (required attributes)
			resource.TestCheckResourceAttrSet("buildkite_organization_banner.banner_foo", "id"),
			resource.TestCheckResourceAttrSet("buildkite_organization_banner.banner_foo", "uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_banner.banner_foo", "message"),
		)

		ckecUpdated := resource.ComposeAggregateTestCheckFunc(
			// Confirm the organization banner exists in the buildkite API
			testAccCheckOrganizationBannerExists(&obr, "buildkite_organization_banner.banner_foo"),
			// Confirm the organization banner has the correct values in Buildkite's system
			testAccCheckOrganizationBannerRemoteValues(&obr, fmt.Sprintf(":buildkite: Test %s organization banner! :buildkite:", updatedMessage)),
			// Check all organization banner resource attributes are set in state (required attributes)
			resource.TestCheckResourceAttrSet("buildkite_organization_banner.banner_foo", "message"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationBannerDestroy,
			Steps: []resource.TestStep{
				{
					Config: config(message),
					Check:  check,
				},
				{
					Config: config(updatedMessage),
					Check:  ckecUpdated,
				},
			},
		})
	})

	t.Run("imports an organization banner", func(t *testing.T) {
		t.Skip()
		message := acctest.RandString(12)
		var obr organizationBannerResourceModel

		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm the organization banner exists in the buildkite API
			testAccCheckOrganizationBannerExists(&obr, "buildkite_organization_banner.banner_foo"),
			// Confirm the organization banner has the correct values in Buildkite's system
			testAccCheckOrganizationBannerRemoteValues(&obr, fmt.Sprintf(":buildkite: Test %s organization banner! :buildkite:", message)),
			// Check all organization banner resource attributes are set in state (required attributes)
			resource.TestCheckResourceAttrSet("buildkite_organization_banner.banner_foo", "id"),
			resource.TestCheckResourceAttrSet("buildkite_organization_banner.banner_foo", "uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_banner.banner_foo", "message"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationBannerDestroy,
			Steps: []resource.TestStep{
				{
					Config: config(message),
					Check:  check,
				},
				{
					ResourceName:      "buildkite_organization_banner.banner_foo",
					ImportState:       true,
					ImportStateVerify: true,
				},
			},
		})
	})
}

func testAccCheckOrganizationBannerExists(obr *organizationBannerResourceModel, name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]

		if !ok {
			return fmt.Errorf("Not found in state: %s", name)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("No ID is set in state")
		}

		r, err := getOrganiztionBanner(
			context.Background(),
			genqlientGraphql,
			getenv("BUILDKITE_ORGANIZATION_SLUG"),
		)

		if err != nil {
			return fmt.Errorf("Error fetching organization banners from GraphQL API: %v", err)
		}

		if bannerNode, ok := getBannerNode(r); ok {
			if bannerNode.Id == "" {
				return fmt.Errorf(fmt.Sprintf("No organization banner found in organiztaion %s", getenv("BUILDKITE_ORGANIZATION_SLUG")))
			}
			updateOrganizationBannerResource(*bannerNode, obr)
		}

		return nil
	}
}

func testAccCheckOrganizationBannerRemoteValues(obr *organizationBannerResourceModel, message string) resource.TestCheckFunc {
	return func(s *terraform.State) error {

		if obr.Message.ValueString() != message {
			return fmt.Errorf("Remote organization banner message (%s) doesn't match expected value (%s)", obr.Message, message)
		}

		return nil
	}
}

func testAccCheckOrganizationBannerDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_organization_banner" {
			continue
		}

		r, err := getOrganiztionBanner(
			context.Background(),
			genqlientGraphql,
			getenv("BUILDKITE_ORGANIZATION_SLUG"),
		)

		if err != nil {
			return fmt.Errorf("Error fetching organization banners from GraphQL API: %v", err)
		}

		if bannerNode, ok := getBannerNode(r); ok {
			if bannerNode != nil {
				return fmt.Errorf("Organization banner still exists")
			}
		}
	}
	return nil
}

func getBannerNode(r *getOrganiztionBannerResponse) (*getOrganiztionBannerOrganizationBannersOrganizationBannerConnectionEdgesOrganizationBannerEdgeNodeOrganizationBanner, bool) {
	if len(r.Organization.Banners.Edges) == 1 {
		return &r.Organization.Banners.Edges[0].Node, true
	} else {
		return nil, false
	}
}
