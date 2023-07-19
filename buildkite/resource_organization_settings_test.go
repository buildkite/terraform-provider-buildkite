package buildkite

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccOrganizationSettings_create(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testCheckOrganizationSettingsResourceRemoved,
		Steps: []resource.TestStep{
			{
				Config: testAccOrganizationSettingsConfigBasic([]string{"0.0.0.0/0", "1.1.1.1/32", "1.0.0.1/32"}),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm that the allowed IP addresses are set correctly in Buildkite's system
					testAccCheckOrganizationSettingsRemoteValues([]string{"0.0.0.0/0", "1.1.1.1/32", "1.0.0.1/32"}),
					// Check that the second IP added to the list is the one we expect, 0.0.0.0/0, this also ensures the length is greater than 1
					// allowing us to assert the first IP is also added correctly
					resource.TestCheckResourceAttr("buildkite_organization_settings.let_them_in", "allowed_api_ip_addresses.1", "1.1.1.1/32"),
				),
			},
		},
	})
}

func TestAccOrganizationSettings_update(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testCheckOrganizationSettingsResourceRemoved,
		Steps: []resource.TestStep{
			{
				Config: testAccOrganizationSettingsConfigBasic([]string{"0.0.0.0/0", "1.1.1.1/32", "1.0.0.1/32"}),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm that the allowed IP addresses are set correctly in Buildkite's system
					testAccCheckOrganizationSettingsRemoteValues([]string{"0.0.0.0/0", "1.1.1.1/32", "1.0.0.1/32"}),
					// Check that the second IP added to the list is the one we expect, 0.0.0.0/0, this also ensures the length is greater than 1
					// allowing us to assert the first IP is also added correctly
					resource.TestCheckResourceAttr("buildkite_organization_settings.let_them_in", "allowed_api_ip_addresses.2", "1.0.0.1/32"),
				),
			},

			{
				Config: testAccOrganizationSettingsConfigBasic([]string{"0.0.0.0/0", "4.4.4.4/32"}),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm that the allowed IP addresses are set correctly in Buildkite's system
					testAccCheckOrganizationSettingsRemoteValues([]string{"0.0.0.0/0", "4.4.4.4/32"}),
					// This check allows us to ensure that TF still has access (0.0.0.0/0) and that the new IP address is added correctly
					resource.TestCheckResourceAttr("buildkite_organization_settings.let_them_in", "allowed_api_ip_addresses.1", "4.4.4.4/32"),
				),
			},
		},
	})
}

func TestAccOrganizationSettings_import(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testCheckOrganizationSettingsResourceRemoved,
		Steps: []resource.TestStep{
			{
				Config: testAccOrganizationSettingsConfigBasic([]string{"0.0.0.0/0", "1.1.1.1/32", "1.0.0.1/32"}),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm that the allowed IP addresses are set correctly in Buildkite's system
					testAccCheckOrganizationSettingsRemoteValues([]string{"0.0.0.0/0", "1.1.1.1/32", "1.0.0.1/32"}),
					// Check that the second IP added to the list is the one we expect, 0.0.0.0/0, this also ensures the length is greater than 1
					// allowing us to assert the first IP is also added correctly
					resource.TestCheckResourceAttr("buildkite_organization_settings.let_them_in", "allowed_api_ip_addresses.2", "1.0.0.1/32"),
				),
			},
			{
				ResourceName:      "buildkite_organization_settings.let_them_in",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccOrganizationSettingsConfigBasic(ip_addresses []string) string {
	config := `
	
	resource "buildkite_organization_settings" "let_them_in" {
        allowed_api_ip_addresses = %v
	}
	`
	marshal, _ := json.Marshal(ip_addresses)

	return fmt.Sprintf(config, string(marshal))
}

func testCheckOrganizationSettingsResourceRemoved(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_organization_settings" {
			continue
		}

		var getOrganizationQuery struct {
			Organization struct {
				AllowedApiIpAddresses string
			}
		}

		err := graphqlClient.Query(context.Background(), &getOrganizationQuery, map[string]interface{}{
			"slug": rs.Primary.ID,
		})

		if err == nil {
			return fmt.Errorf("Organization settings still exist")
		}
		return nil
	}
	return nil
}

func testAccCheckOrganizationSettingsRemoteValues(ip_addresses []string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		resp, err := getOrganization(genqlientGraphql, getenv("BUILDKITE_ORGANIZATION_SLUG"))

		if err != nil {
			return err
		}

		if resp.Organization.AllowedApiIpAddresses != strings.Join(ip_addresses, " ") {
			return fmt.Errorf("Allowed IP addresses do not match. Expected: %s, got: %s", ip_addresses, resp.Organization.AllowedApiIpAddresses)
		}
		return nil
	}
}
