package buildkite

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccBuildkiteOrganizationResource(t *testing.T) {
	config := func(ip_addresses []string) string {
		config := `

		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_organization" "let_them_in" {
			allowed_api_ip_addresses = %v
		}
		`
		marshal, _ := json.Marshal(ip_addresses)

		return fmt.Sprintf(config, string(marshal))
	}

	configNoAllowedIPs := func() string {
		config := `

		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_organization" "let_them_in" {}
		`

		return config
	}

	t.Run("creates an organization", func(t *testing.T) {
		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm that the allowed IP addresses are set correctly in Buildkite's system
			testAccCheckOrganizationRemoteValues([]string{"0.0.0.0/0", "1.1.1.1/32", "1.0.0.1/32"}),
			// Check that the second IP added to the list is the one we expect, 0.0.0.0/0, this also ensures the length is greater than 1
			// allowing us to assert the first IP is also added correctly
			resource.TestCheckResourceAttr("buildkite_organization.let_them_in", "allowed_api_ip_addresses.1", "1.1.1.1/32"),
		)

		resource.Test(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testCheckOrganizationResourceRemoved,
			Steps: []resource.TestStep{
				{
					Config: config([]string{"0.0.0.0/0", "1.1.1.1/32", "1.0.0.1/32"}),
					Check:  check,
				},
			},
		})
	})

	t.Run("updates an organization", func(t *testing.T) {
		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm that the allowed IP addresses are set correctly in Buildkite's system
			testAccCheckOrganizationRemoteValues([]string{"0.0.0.0/0", "1.1.1.1/32", "1.0.0.1/32"}),
			// Check that the second IP added to the list is the one we expect, 0.0.0.0/0, this also ensures the length is greater than 1
			// allowing us to assert the first IP is also added correctly
			resource.TestCheckResourceAttr("buildkite_organization.let_them_in", "allowed_api_ip_addresses.2", "1.0.0.1/32"),
		)

		ckeckUpdated := resource.ComposeAggregateTestCheckFunc(
			// Confirm that the allowed IP addresses are set correctly in Buildkite's system
			testAccCheckOrganizationRemoteValues([]string{"0.0.0.0/0", "4.4.4.4/32"}),
			// This check allows us to ensure that TF still has access (0.0.0.0/0) and that the new IP address is added correctly
			resource.TestCheckResourceAttr("buildkite_organization.let_them_in", "allowed_api_ip_addresses.1", "4.4.4.4/32"),
		)

		resource.Test(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testCheckOrganizationResourceRemoved,
			Steps: []resource.TestStep{
				{
					Config: config([]string{"0.0.0.0/0", "1.1.1.1/32", "1.0.0.1/32"}),
					Check:  check,
				},
				{
					Config: config([]string{"0.0.0.0/0", "4.4.4.4/32"}),
					Check:  ckeckUpdated,
				},
			},
		})
	})

	t.Run("updates an organization with an empty string allowed API IP address list", func(t *testing.T) {
		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm that the allowed IP addresses are set correctly in Buildkite's system
			testAccCheckOrganizationRemoteValues([]string{"0.0.0.0/0", "1.1.1.1/32", "1.0.0.1/32"}),
			// Check that the second IP added to the list is the one we expect, 0.0.0.0/0, this also ensures the length is greater than 1
			// allowing us to assert the first IP is also added correctly
			resource.TestCheckResourceAttr("buildkite_organization.let_them_in", "allowed_api_ip_addresses.2", "1.0.0.1/32"),
		)

		ckeckUpdated := resource.ComposeAggregateTestCheckFunc(
			// Confirm that the allowed IP addresses are set correctly in Buildkite's system
			testAccCheckOrganizationRemoteValues([]string{""}),
			// Check the allowed IP address list in state is of length 1, and a empty string element
			resource.TestCheckResourceAttr("buildkite_organization.let_them_in", "allowed_api_ip_addresses.#", "1"),
			resource.TestCheckResourceAttr("buildkite_organization.let_them_in", "allowed_api_ip_addresses.0", ""),
		)

		resource.Test(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testCheckOrganizationResourceRemoved,
			Steps: []resource.TestStep{
				{
					Config: config([]string{"0.0.0.0/0", "1.1.1.1/32", "1.0.0.1/32"}),
					Check:  check,
				},
				{
					Config: config([]string{""}),
					Check:  ckeckUpdated,
				},
			},
		})
	})

	t.Run("updates an organization by removing the allowed API IP address list property", func(t *testing.T) {
		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm that the allowed IP addresses are set correctly in Buildkite's system
			testAccCheckOrganizationRemoteValues([]string{"0.0.0.0/0", "1.1.1.1/32", "1.0.0.1/32"}),
			// Check that the second IP added to the list is the one we expect, 0.0.0.0/0, this also ensures the length is greater than 1
			// allowing us to assert the first IP is also added correctly
			resource.TestCheckResourceAttr("buildkite_organization.let_them_in", "allowed_api_ip_addresses.2", "1.0.0.1/32"),
		)

		ckeckUpdated := resource.ComposeAggregateTestCheckFunc(
			// Confirm that the allowed IP addresses are set correctly in Buildkite's system
			testAccCheckOrganizationRemoteValues([]string{""}),
			// Check the allowed IP address list in not set in state
			resource.TestCheckNoResourceAttr("buildkite_organization.let_them_in", "allowed_api_ip_addresses"),
		)

		resource.Test(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testCheckOrganizationResourceRemoved,
			Steps: []resource.TestStep{
				{
					Config: config([]string{"0.0.0.0/0", "1.1.1.1/32", "1.0.0.1/32"}),
					Check:  check,
				},
				{
					Config: configNoAllowedIPs(),
					Check:  ckeckUpdated,
					// After clearing the IPs, state will be set to null and refresh will restore the attribute to an empty-string list of length 1
					ExpectNonEmptyPlan: true,
				},
			},
		})
	})

	t.Run("imports an organization", func(t *testing.T) {
		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm that the allowed IP addresses are set correctly in Buildkite's system
			testAccCheckOrganizationRemoteValues([]string{"0.0.0.0/0", "1.1.1.1/32", "1.0.0.1/32"}),
			// Check that the second IP added to the list is the one we expect, 0.0.0.0/0, this also ensures the length is greater than 1
			// allowing us to assert the first IP is also added correctly
			resource.TestCheckResourceAttr("buildkite_organization.let_them_in", "allowed_api_ip_addresses.2", "1.0.0.1/32"),
		)

		resource.Test(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testCheckOrganizationResourceRemoved,
			Steps: []resource.TestStep{
				{
					Config: config([]string{"0.0.0.0/0", "1.1.1.1/32", "1.0.0.1/32"}),
					Check:  check,
				},
				{
					ResourceName:      "buildkite_organization.let_them_in",
					ImportState:       true,
					ImportStateVerify: true,
				},
			},
		})
	})
}

func testCheckOrganizationResourceRemoved(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_organization" {
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
			return fmt.Errorf("Organization still exist")
		}
		return nil
	}
	return nil
}

func testAccCheckOrganizationRemoteValues(ip_addresses []string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		resp, err := getOrganization(context.Background(), genqlientGraphql, getenv("BUILDKITE_ORGANIZATION_SLUG"))
		if err != nil {
			return err
		}

		if resp.Organization.AllowedApiIpAddresses != strings.Join(ip_addresses, " ") {
			return fmt.Errorf("Allowed IP addresses do not match. Expected: %s, got: %s", ip_addresses, resp.Organization.AllowedApiIpAddresses)
		}
		return nil
	}
}
