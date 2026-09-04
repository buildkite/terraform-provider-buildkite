package buildkite

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAllowedApiIpAddressesFromAPI(t *testing.T) {
	t.Parallel()

	list := func(cidrs ...string) types.List {
		values := make([]attr.Value, len(cidrs))
		for i, c := range cidrs {
			values[i] = types.StringValue(c)
		}
		return types.ListValueMust(types.StringType, values)
	}
	null := types.ListNull(types.StringType)

	testCases := []struct {
		name    string
		remote  string
		current types.List
		want    types.List
	}{
		{"unset attribute stays null for an empty allowlist", "", null, null},
		{"empty list is kept as is", "", list(), list()},
		{"explicit empty string round trips", "", list(""), list("")},
		{"remote allowlist is split", "1.1.1.1/32 0.0.0.0/0", list("1.1.1.1/32"), list("1.1.1.1/32", "0.0.0.0/0")},
		{"matching allowlist is unchanged", "0.0.0.0/0 1.1.1.1/32", list("0.0.0.0/0", "1.1.1.1/32"), list("0.0.0.0/0", "1.1.1.1/32")},
		{"remote allowlist is read when unset", "1.1.1.1/32", null, list("1.1.1.1/32")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, diags := allowedApiIpAddressesFromAPI(context.Background(), tc.remote, tc.current)
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if !got.Equal(tc.want) {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestAllowedApiIpAddressesValue(t *testing.T) {
	t.Parallel()

	list := func(cidrs ...string) types.List {
		values := make([]attr.Value, len(cidrs))
		for i, c := range cidrs {
			values[i] = types.StringValue(c)
		}
		return types.ListValueMust(types.StringType, values)
	}
	null := types.ListNull(types.StringType)

	// pairs that serialize to the same value must not trigger the mutation
	testCases := []struct {
		name    string
		planned types.List
		current types.List
		changed bool
		value   string
	}{
		{"null and null", null, null, false, ""},
		{"null and empty list", null, list(), false, ""},
		{"null and empty string", null, list(""), false, ""},
		{"empty list and empty string", list(), list(""), false, ""},
		{"same list", list("1.1.1.1/32"), list("1.1.1.1/32"), false, "1.1.1.1/32"},
		{"different list", list("1.1.1.1/32"), list("0.0.0.0/0"), true, "1.1.1.1/32"},
		{"set from nothing", list("0.0.0.0/0", "1.1.1.1/32"), null, true, "0.0.0.0/0 1.1.1.1/32"},
		{"cleared", null, list("1.1.1.1/32"), true, ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			planned, current := allowedApiIpAddressesValue(tc.planned), allowedApiIpAddressesValue(tc.current)
			if (planned != current) != tc.changed {
				t.Errorf("planned %q vs current %q: changed = %t, want %t", planned, current, planned != current, tc.changed)
			}
			if planned != tc.value {
				t.Errorf("planned value = %q, want %q", planned, tc.value)
			}
		})
	}
}

func TestRevokePeriodDays(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		period string
		days   int64
	}{
		{"NEVER", 0},
		{"DAYS_30", 30},
		{"DAYS_60", 60},
		{"DAYS_90", 90},
		{"DAYS_180", 180},
		{"DAYS_365", 365},
	}
	if len(testCases) != len(revokeInactiveTokenPeriods) {
		t.Fatalf("expected a case for each of %v", revokeInactiveTokenPeriods)
	}

	for _, tc := range testCases {
		t.Run(tc.period, func(t *testing.T) {
			var days *int64
			if tc.days != 0 {
				days = &tc.days
			}
			if got := revokePeriodFromDays(days); got != tc.period {
				t.Errorf("revokePeriodFromDays(%v) = %s, want %s", days, got, tc.period)
			}
			got := revokePeriodToDays(tc.period)
			if (got == nil) != (days == nil) || (got != nil && *got != tc.days) {
				t.Errorf("revokePeriodToDays(%s) = %v, want %v", tc.period, got, days)
			}
		})
	}
}

func TestApiSettingsPatch(t *testing.T) {
	t.Parallel()

	days := func(d int64) *int64 { return &d }
	model := func(revoke types.String, restrict types.Bool) organizationResourceModel {
		return organizationResourceModel{RevokeInactiveTokensAfter: revoke, RestrictUserApiTokenCreation: restrict}
	}
	unset := model(types.StringNull(), types.BoolNull())
	testCases := []struct {
		name         string
		config       organizationResourceModel
		plan         organizationResourceModel
		current      organizationAPISettings
		currentKnown bool
		want         string
	}{
		{"unset attributes are not sent", unset, model(types.StringUnknown(), types.BoolUnknown()), organizationAPISettings{RevokeInactiveTokensAfterDays: days(30), RestrictUserApiTokenCreation: true}, true, `{}`},
		{"values kept from state for unset attributes are not sent", unset, model(types.StringValue("DAYS_90"), types.BoolValue(true)), organizationAPISettings{}, true, `{}`},
		{"unchanged values are not sent", model(types.StringValue("DAYS_90"), types.BoolValue(true)), model(types.StringValue("DAYS_90"), types.BoolValue(true)), organizationAPISettings{RevokeInactiveTokensAfterDays: days(90), RestrictUserApiTokenCreation: true}, true, `{}`},
		{"changed period is sent", model(types.StringValue("DAYS_60"), types.BoolNull()), model(types.StringValue("DAYS_60"), types.BoolValue(false)), organizationAPISettings{RevokeInactiveTokensAfterDays: days(90)}, true, `{"revoke_inactive_tokens_after_days":60}`},
		{"never is sent as null", model(types.StringValue("NEVER"), types.BoolValue(false)), model(types.StringValue("NEVER"), types.BoolValue(false)), organizationAPISettings{RevokeInactiveTokensAfterDays: days(90)}, true, `{"revoke_inactive_tokens_after_days":null}`},
		{"changed restriction is sent", model(types.StringNull(), types.BoolValue(false)), model(types.StringValue("NEVER"), types.BoolValue(false)), organizationAPISettings{RestrictUserApiTokenCreation: true}, true, `{"restrict_user_api_token_creation":false}`},
		{"both are sent", model(types.StringValue("DAYS_365"), types.BoolValue(true)), model(types.StringValue("DAYS_365"), types.BoolValue(true)), organizationAPISettings{}, true, `{"restrict_user_api_token_creation":true,"revoke_inactive_tokens_after_days":365}`},
		// when the current settings can't be read, configured values are always sent
		{"unknown current and explicit false", model(types.StringNull(), types.BoolValue(false)), model(types.StringUnknown(), types.BoolValue(false)), organizationAPISettings{}, false, `{"restrict_user_api_token_creation":false}`},
		{"unknown current and explicit never", model(types.StringValue("NEVER"), types.BoolNull()), model(types.StringValue("NEVER"), types.BoolUnknown()), organizationAPISettings{}, false, `{"revoke_inactive_tokens_after_days":null}`},
		{"unknown current and nothing configured", unset, model(types.StringValue("DAYS_30"), types.BoolValue(true)), organizationAPISettings{}, false, `{}`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(apiSettingsPatch(&tc.config, &tc.plan, &tc.current, tc.currentKnown))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

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

		checkUpdated := resource.ComposeAggregateTestCheckFunc(
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
					Check:  checkUpdated,
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

		checkUpdated := resource.ComposeAggregateTestCheckFunc(
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
					Check:  checkUpdated,
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

		checkUpdated := resource.ComposeAggregateTestCheckFunc(
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
					Check:  checkUpdated,
				},
			},
		})
	})

	t.Run("manages an organization without configuring the allowed API IP address list", func(t *testing.T) {
		resource.Test(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testCheckOrganizationResourceRemoved,
			Steps: []resource.TestStep{
				{
					Config: configNoAllowedIPs(),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttrSet("buildkite_organization.let_them_in", "uuid"),
						resource.TestCheckNoResourceAttr("buildkite_organization.let_them_in", "allowed_api_ip_addresses"),
					),
				},
				{
					ResourceName:      "buildkite_organization.let_them_in",
					ImportState:       true,
					ImportStateVerify: true,
				},
			},
		})
	})

	t.Run("adopts an existing allowed API IP address list", func(t *testing.T) {
		// Give the organization an allowlist before terraform manages it (0.0.0.0/0 keeps the API reachable)
		presetAllowlist := func() {
			org, err := getOrganization(context.Background(), genqlientGraphql, getenv("BUILDKITE_ORGANIZATION_SLUG"))
			if err != nil {
				t.Fatalf("Unable to read organization: %v", err)
			}
			if _, err := setApiIpAddresses(context.Background(), genqlientGraphql, org.Organization.Id, "0.0.0.0/0"); err != nil {
				t.Fatalf("Unable to preset the allowed API IP addresses: %v", err)
			}
			if _, err := getOrganization(context.Background(), genqlientGraphql, getenv("BUILDKITE_ORGANIZATION_SLUG")); err != nil {
				t.Fatalf("API unreachable after presetting the allowed API IP addresses: %v", err)
			}
		}

		resource.Test(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testCheckOrganizationResourceRemoved,
			Steps: []resource.TestStep{
				{
					PreConfig: presetAllowlist,
					Config:    config([]string{"0.0.0.0/0"}),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{
							plancheck.ExpectEmptyPlan(),
						},
					},
					Check: resource.ComposeAggregateTestCheckFunc(
						// Confirm the existing allowlist is kept in Buildkite's system
						testAccCheckOrganizationRemoteValues([]string{"0.0.0.0/0"}),
						resource.TestCheckResourceAttr("buildkite_organization.let_them_in", "allowed_api_ip_addresses.0", "0.0.0.0/0"),
					),
				},
				{
					Config: configNoAllowedIPs(),
					Check: resource.ComposeAggregateTestCheckFunc(
						// Confirm the allowlist is cleared once the attribute is removed
						testAccCheckOrganizationRemoteValues([]string{""}),
						resource.TestCheckNoResourceAttr("buildkite_organization.let_them_in", "allowed_api_ip_addresses"),
					),
				},
			},
		})
	})

	configAPISettings := func(settings string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_organization" "let_them_in" {
			%s
		}
		`, settings)
	}

	t.Run("manages restricting user API token creation", func(t *testing.T) {
		resource.Test(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testCheckOrganizationResourceRemoved,
			Steps: []resource.TestStep{
				{
					// unmanaged: the current values are only read into state
					Config: configAPISettings(``),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_organization.let_them_in", "restrict_user_api_token_creation", "false"),
						resource.TestCheckResourceAttr("buildkite_organization.let_them_in", "revoke_inactive_tokens_after", "NEVER"),
						testAccCheckOrganizationAPISettingsRemoteValues("NEVER", false),
					),
				},
				{
					Config: configAPISettings(`restrict_user_api_token_creation = true`),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_organization.let_them_in", "restrict_user_api_token_creation", "true"),
						testAccCheckOrganizationAPISettingsRemoteValues("NEVER", true),
					),
				},
				{
					ResourceName:      "buildkite_organization.let_them_in",
					ImportState:       true,
					ImportStateVerify: true,
				},
				{
					// removing the attribute leaves the setting as it is
					Config: configAPISettings(``),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{
							plancheck.ExpectEmptyPlan(),
						},
					},
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_organization.let_them_in", "restrict_user_api_token_creation", "true"),
						testAccCheckOrganizationAPISettingsRemoteValues("NEVER", true),
					),
				},
				{
					// it has to be lifted explicitly
					Config: configAPISettings(`restrict_user_api_token_creation = false`),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_organization.let_them_in", "restrict_user_api_token_creation", "false"),
						testAccCheckOrganizationAPISettingsRemoteValues("NEVER", false),
					),
				},
			},
		})
	})

	t.Run("manages inactive API token revocation", func(t *testing.T) {
		resource.Test(t, resource.TestCase{
			PreCheck: func() {
				testAccPreCheck(t)
				// the setting can only be changed on plans with the inactive API token revocation feature
				if settings, err := getTestClient().getOrganizationAPISettings(context.Background()); err != nil {
					t.Skipf("unable to read organization api-settings (needs the read_organization_settings scope): %v", err)
				} else if !settings.Features.InactiveApiTokenRevocation {
					t.Skip("inactive API token revocation is not available on this organization's plan")
				}
			},
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testCheckOrganizationResourceRemoved,
			Steps: []resource.TestStep{
				{
					Config: configAPISettings(`revoke_inactive_tokens_after = "DAYS_30"`),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_organization.let_them_in", "revoke_inactive_tokens_after", "DAYS_30"),
						testAccCheckOrganizationAPISettingsRemoteValues("DAYS_30", false),
					),
				},
				{
					Config: configAPISettings(`revoke_inactive_tokens_after = "DAYS_90"`),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_organization.let_them_in", "revoke_inactive_tokens_after", "DAYS_90"),
						testAccCheckOrganizationAPISettingsRemoteValues("DAYS_90", false),
					),
				},
				{
					ResourceName:      "buildkite_organization.let_them_in",
					ImportState:       true,
					ImportStateVerify: true,
				},
				{
					// removing the attribute leaves the setting as it is
					Config: configAPISettings(``),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{
							plancheck.ExpectEmptyPlan(),
						},
					},
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_organization.let_them_in", "revoke_inactive_tokens_after", "DAYS_90"),
						testAccCheckOrganizationAPISettingsRemoteValues("DAYS_90", false),
					),
				},
				{
					// NEVER disables revocation again
					Config: configAPISettings(`revoke_inactive_tokens_after = "NEVER"`),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_organization.let_them_in", "revoke_inactive_tokens_after", "NEVER"),
						testAccCheckOrganizationAPISettingsRemoteValues("NEVER", false),
					),
				},
			},
		})
	})

	t.Run("rejects an unsupported inactive token revocation period", func(t *testing.T) {
		resource.Test(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config:      configAPISettings(`revoke_inactive_tokens_after = "DAYS_45"`),
					PlanOnly:    true,
					ExpectError: regexp.MustCompile(`(?s)revoke_inactive_tokens_after.*value must be one of`),
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

func testAccCheckOrganizationAPISettingsRemoteValues(revokeAfter string, restrictTokenCreation bool) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		settings, err := getTestClient().getOrganizationAPISettings(context.Background())
		if err != nil {
			return err
		}
		if got := revokePeriodFromDays(settings.RevokeInactiveTokensAfterDays); got != revokeAfter {
			return fmt.Errorf("Remote revoke_inactive_tokens_after does not match. Expected: %s, got: %s", revokeAfter, got)
		}
		if settings.RestrictUserApiTokenCreation != restrictTokenCreation {
			return fmt.Errorf("Remote restrict_user_api_token_creation does not match. Expected: %t, got: %t", restrictTokenCreation, settings.RestrictUserApiTokenCreation)
		}
		return nil
	}
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

// A failed PATCH has to leave the api-settings attributes describing the organization rather than
// the plan, or state claims a setting that was never applied. That matters most when the GET was
// also refused: readAPISettings falls back to state on a 403, so it re-adopts whatever is there on
// every refresh, and a wrong value put there once is never corrected and never shows in a plan.
func TestUpdateAPISettingsReportsTheOrganizationWhenThePatchFails(t *testing.T) {
	t.Parallel()

	patchRefused := stubResponse{status: http.StatusInternalServerError, body: `{"message":"patch failed"}`}
	tests := []struct {
		name string
		get  stubResponse
	}{
		{
			name: "current settings readable",
			get:  stubResponse{status: http.StatusOK, body: `{"revoke_inactive_tokens_after_days":null,"restrict_user_api_token_creation":false}`},
		},
		{
			// Without the read scope the prior state stands in for the organization's settings.
			name: "current settings refused",
			get:  stubResponse{status: http.StatusForbidden, body: `{"message":"no read_organization_settings scope"}`},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server, _ := newRetryStub(t, testCase.get, patchRefused)
			defer server.Close()

			o := &organizationResource{client: newRetryTestClient(t, server.URL, 0, time.Millisecond)}

			configured := organizationResourceModel{
				RevokeInactiveTokensAfter:    types.StringValue("DAYS_30"),
				RestrictUserApiTokenCreation: types.BoolValue(true),
			}
			prior := organizationResourceModel{
				RevokeInactiveTokensAfter:    types.StringValue(revokeInactiveTokensNever),
				RestrictUserApiTokenCreation: types.BoolValue(false),
			}

			var state organizationResourceModel
			var diags diag.Diagnostics
			o.updateAPISettings(context.Background(), &configured, &configured, &prior, &state, &diags)

			if !diags.HasError() {
				t.Fatalf("updateAPISettings diagnostics = %v, want the PATCH failure reported", diags)
			}
			if got := state.RevokeInactiveTokensAfter.ValueString(); got != revokeInactiveTokensNever {
				t.Errorf("Persisted revoke_inactive_tokens_after = %q, want %q: the PATCH failed, so the configured period never applied", got, revokeInactiveTokensNever)
			}
			if state.RestrictUserApiTokenCreation.ValueBool() {
				t.Error("Persisted restrict_user_api_token_creation = true, want false: the PATCH failed, so the restriction never applied")
			}
		})
	}
}

// Update applies 2FA before the api-settings PATCH, so a failure in the PATCH must not drop the 2FA
// change: Terraform would plan it again, and in the meantime state disagrees with the organization.
func TestOrganizationUpdatePersistsTheEnforced2FAWhenTheAPISettingsPatchFails(t *testing.T) {
	t.Parallel()

	server, requests := newRetryStub(t,
		// setOrganization2FA applies.
		stubResponse{status: http.StatusOK, body: `{"data":{"organizationEnforceTwoFactorAuthenticationForMembersUpdate":{"organization":{
			"id": "organization-id",
			"membersRequireTwoFactorAuthentication": true
		}}}}`},
		// The api-settings GET, and then a PATCH that does not.
		stubResponse{status: http.StatusOK, body: `{"revoke_inactive_tokens_after_days":null,"restrict_user_api_token_creation":false}`},
		stubResponse{status: http.StatusInternalServerError, body: `{"message":"patch failed"}`},
	)
	defer server.Close()

	client := newRetryTestClient(t, server.URL, 0, time.Millisecond)
	orgID := "organization-id"
	client.organizationId = &orgID
	o := &organizationResource{client: client}

	ctx := t.Context()
	var schemaResp fwresource.SchemaResponse
	o.Schema(ctx, fwresource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics = %v", schemaResp.Diagnostics)
	}
	schema := schemaResp.Schema

	// An unchanged allowlist, so updateAllowedApiIpAddresses makes no request of its own.
	allowlist := tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{})
	prior := nullObjectWith(ctx, t, schema.Type(), map[string]tftypes.Value{
		"id":                           tftypes.NewValue(tftypes.String, "organization-id"),
		"uuid":                         tftypes.NewValue(tftypes.String, "organization-uuid"),
		"allowed_api_ip_addresses":     allowlist,
		"enforce_2fa":                  tftypes.NewValue(tftypes.Bool, false),
		"revoke_inactive_tokens_after": tftypes.NewValue(tftypes.String, revokeInactiveTokensNever),
	})
	planned := nullObjectWith(ctx, t, schema.Type(), map[string]tftypes.Value{
		"id":                           tftypes.NewValue(tftypes.String, "organization-id"),
		"uuid":                         tftypes.NewValue(tftypes.String, "organization-uuid"),
		"allowed_api_ip_addresses":     allowlist,
		"enforce_2fa":                  tftypes.NewValue(tftypes.Bool, true),
		"revoke_inactive_tokens_after": tftypes.NewValue(tftypes.String, "DAYS_30"),
	})

	req := fwresource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: schema, Raw: planned},
		State:  tfsdk.State{Schema: schema, Raw: prior},
		Config: tfsdk.Config{Schema: schema, Raw: planned},
	}
	resp := fwresource.UpdateResponse{State: tfsdk.State{Schema: schema, Raw: prior}}

	o.Update(ctx, req, &resp)

	if got := requests.Load(); got < 3 {
		t.Fatalf("Made %d requests, want 2FA to have applied before the PATCH failed", got)
	}
	if !diagnosticsContain(resp.Diagnostics, "Unable to update organization API settings") {
		t.Fatalf("Update() diagnostics = %v, want the PATCH failure reported", resp.Diagnostics)
	}

	var persisted organizationResourceModel
	if diags := resp.State.Get(ctx, &persisted); diags.HasError() {
		t.Fatalf("Reading the persisted state = %v", diags)
	}
	if !persisted.Enforce2FA.ValueBool() {
		t.Error("Persisted enforce_2fa = false, want true: the 2FA mutation applied, so state has to say so")
	}
	if got := persisted.RevokeInactiveTokensAfter.ValueString(); got != revokeInactiveTokensNever {
		t.Errorf("Persisted revoke_inactive_tokens_after = %q, want %q: the PATCH failed", got, revokeInactiveTokensNever)
	}
}

// This resource does not create an organization, it applies settings to one that already exists, and
// each step compares before it mutates. So the recoverable answer to a half-applied Create is to
// record nothing and let the next apply re-run it. Recording the part that applied instead would
// taint the instance, and a tainted instance is replaced rather than updated: Delete would clear the
// API IP allowlist before Create put it back. What the practitioner does need is to be told which
// settings are live on their organization despite the failure.
func TestOrganizationCreateWarnsAboutUnrecordedChanges(t *testing.T) {
	t.Parallel()

	const configuredAllowlist = "1.2.3.4/32"

	organizationIs := func(allowlist string, enforced2FA bool) stubResponse {
		return stubResponse{status: http.StatusOK, body: fmt.Sprintf(`{"data":{"organization":{
			"id": "organization-id",
			"uuid": "organization-uuid",
			"allowedApiIpAddresses": %q,
			"membersRequireTwoFactorAuthentication": %t
		}}}`, allowlist, enforced2FA)}
	}
	allowlistUpdated := stubResponse{status: http.StatusOK, body: `{"data":{"organizationApiIpAllowlistUpdate":{"organization":{
		"id": "organization-id",
		"uuid": "organization-uuid",
		"allowedApiIpAddresses": "",
		"membersRequireTwoFactorAuthentication": false
	}}}}`}
	twoFAUpdated := stubResponse{status: http.StatusOK, body: `{"data":{"organizationEnforceTwoFactorAuthenticationForMembersUpdate":{"organization":{
		"id": "organization-id",
		"membersRequireTwoFactorAuthentication": true
	}}}}`}
	graphQLFails := stubResponse{status: http.StatusOK, body: `{"errors":[{"message":"mutation exploded"}]}`}
	apiSettingsReadable := stubResponse{status: http.StatusOK, body: `{"revoke_inactive_tokens_after_days":null,"restrict_user_api_token_creation":false}`}
	patchFails := stubResponse{status: http.StatusInternalServerError, body: `{"message":"patch failed"}`}

	tests := []struct {
		name string
		// The allowlist the config asks for. Unset means the attribute is absent, which clears it.
		configuredAllowlist string
		// Set when the config manages revoke_inactive_tokens_after, which is what makes Create reach
		// the api-settings PATCH rather than failing at the 2FA mutation before it.
		configuredRevoke string
		responses        []stubResponse
		wantError        string
		// Substrings the single warning has to contain, or none to assert there is no warning.
		wantWarned []string
	}{
		{
			name:                "allowlist applied, then the 2FA mutation fails",
			configuredAllowlist: configuredAllowlist,
			responses:           []stubResponse{organizationIs("", false), allowlistUpdated, graphQLFails},
			wantError:           "Unable to set 2FA",
			wantWarned:          []string{`allowlist was set to "1.2.3.4/32"`},
		},
		{
			// Already what the config asks for, so updateAllowedApiIpAddresses makes no request and
			// this apply is not responsible for the allowlist being in place.
			name:                "allowlist already matched",
			configuredAllowlist: configuredAllowlist,
			responses:           []stubResponse{organizationIs(configuredAllowlist, false), graphQLFails},
			wantError:           "Unable to set 2FA",
		},
		{
			// No allowlist in the config against an organization that has one, which clears it. The
			// wording differs from a set, because "set to \"\"" would read as a change to nothing.
			name:       "allowlist cleared, then the 2FA mutation fails",
			responses:  []stubResponse{organizationIs(configuredAllowlist, false), allowlistUpdated, graphQLFails},
			wantError:  "Unable to set 2FA",
			wantWarned: []string{"allowlist was cleared"},
		},
		{
			// 2FA is as sticky as the allowlist and just as absent from state, so the failure after it
			// has to name it. The allowlist is unchanged here, so it must not be named.
			name:             "2FA applied, then the api-settings PATCH fails",
			configuredRevoke: "DAYS_30",
			responses:        []stubResponse{organizationIs("", false), twoFAUpdated, apiSettingsReadable, patchFails},
			wantError:        "Unable to update organization API settings",
			wantWarned:       []string{"two-factor authentication was enforced"},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server, requests := newRetryStub(t, testCase.responses...)
			defer server.Close()

			client := newRetryTestClient(t, server.URL, 0, time.Millisecond)
			orgID := "organization-id"
			client.organizationId = &orgID
			o := &organizationResource{client: client}

			ctx := t.Context()
			var schemaResp fwresource.SchemaResponse
			o.Schema(ctx, fwresource.SchemaRequest{}, &schemaResp)
			if schemaResp.Diagnostics.HasError() {
				t.Fatalf("Schema() diagnostics = %v", schemaResp.Diagnostics)
			}
			schema := schemaResp.Schema

			attributes := map[string]tftypes.Value{"enforce_2fa": tftypes.NewValue(tftypes.Bool, true)}
			if testCase.configuredAllowlist != "" {
				attributes["allowed_api_ip_addresses"] = tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{
					tftypes.NewValue(tftypes.String, testCase.configuredAllowlist),
				})
			}
			if testCase.configuredRevoke != "" {
				attributes["revoke_inactive_tokens_after"] = tftypes.NewValue(tftypes.String, testCase.configuredRevoke)
			}
			raw := nullObjectWith(ctx, t, schema.Type(), attributes)

			req := fwresource.CreateRequest{
				Plan:   tfsdk.Plan{Schema: schema, Raw: raw},
				Config: tfsdk.Config{Schema: schema, Raw: raw},
			}
			resp := fwresource.CreateResponse{State: tfsdk.State{Schema: schema, Raw: tftypes.NewValue(schema.Type().TerraformType(ctx), nil)}}

			o.Create(ctx, req, &resp)

			if got := requests.Load(); got < int64(len(testCase.responses)) {
				t.Fatalf("Made %d requests, want %d: the failure has to come from the last stubbed response", got, len(testCase.responses))
			}
			if !diagnosticsContain(resp.Diagnostics, testCase.wantError) {
				t.Fatalf("Create() diagnostics = %v, want %q", resp.Diagnostics, testCase.wantError)
			}
			if !resp.State.Raw.IsNull() {
				t.Errorf("Create() persisted %v, want no state: persisting taints the instance, and replacing it clears the API IP allowlist", resp.State.Raw)
			}

			warnings := resp.Diagnostics.Warnings()
			if len(testCase.wantWarned) == 0 {
				for _, d := range warnings {
					if d.Summary() == "Organization settings changed but not recorded" {
						t.Errorf("Create() warned %q, want no warning: this apply changed nothing before it failed", d.Detail())
					}
				}
				return
			}

			var detail string
			for _, d := range warnings {
				if d.Summary() == "Organization settings changed but not recorded" {
					detail = d.Detail()
				}
			}
			if detail == "" {
				t.Fatalf("Create() warnings = %v, want one naming the settings that applied", warnings)
			}
			for _, want := range testCase.wantWarned {
				if !strings.Contains(detail, want) {
					t.Errorf("Warning detail = %q, want it to mention %q", detail, want)
				}
			}
			if testCase.configuredRevoke != "" && strings.Contains(detail, "allowlist") {
				t.Errorf("Warning detail = %q, want no mention of the allowlist: this apply did not change it", detail)
			}
		})
	}
}

// A non-403 failure on the api-settings GET makes updateAPISettings return before it assigns either
// attribute it owns. Update persists unconditionally now, so without seeding state from the prior
// values first, that path writes nulls over settings the organization still has. readAPISettings
// falls back to state on a 403, so a null put there once is re-adopted on every refresh.
func TestOrganizationUpdateKeepsPriorAPISettingsWhenTheReadFails(t *testing.T) {
	t.Parallel()

	// Only the api-settings GET is reached: the allowlist and 2FA both match the prior state, so
	// neither sends a mutation of its own.
	server, requests := newRetryStub(t,
		stubResponse{status: http.StatusInternalServerError, body: `{"message":"api-settings unavailable"}`},
	)
	defer server.Close()

	client := newRetryTestClient(t, server.URL, 0, time.Millisecond)
	orgID := "organization-id"
	client.organizationId = &orgID
	o := &organizationResource{client: client}

	ctx := t.Context()
	var schemaResp fwresource.SchemaResponse
	o.Schema(ctx, fwresource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics = %v", schemaResp.Diagnostics)
	}
	schema := schemaResp.Schema

	allowlist := tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{})
	prior := nullObjectWith(ctx, t, schema.Type(), map[string]tftypes.Value{
		"id":                               tftypes.NewValue(tftypes.String, "organization-id"),
		"uuid":                             tftypes.NewValue(tftypes.String, "organization-uuid"),
		"allowed_api_ip_addresses":         allowlist,
		"enforce_2fa":                      tftypes.NewValue(tftypes.Bool, true),
		"revoke_inactive_tokens_after":     tftypes.NewValue(tftypes.String, "DAYS_30"),
		"restrict_user_api_token_creation": tftypes.NewValue(tftypes.Bool, true),
	})
	// Only the description of an unrelated attribute changes, so nothing before the GET mutates.
	planned := nullObjectWith(ctx, t, schema.Type(), map[string]tftypes.Value{
		"id":                               tftypes.NewValue(tftypes.String, "organization-id"),
		"uuid":                             tftypes.NewValue(tftypes.String, "organization-uuid"),
		"allowed_api_ip_addresses":         allowlist,
		"enforce_2fa":                      tftypes.NewValue(tftypes.Bool, true),
		"revoke_inactive_tokens_after":     tftypes.NewValue(tftypes.String, "DAYS_90"),
		"restrict_user_api_token_creation": tftypes.NewValue(tftypes.Bool, true),
	})

	req := fwresource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: schema, Raw: planned},
		State:  tfsdk.State{Schema: schema, Raw: prior},
		Config: tfsdk.Config{Schema: schema, Raw: planned},
	}
	resp := fwresource.UpdateResponse{State: tfsdk.State{Schema: schema, Raw: prior}}

	o.Update(ctx, req, &resp)

	if got := requests.Load(); got < 1 {
		t.Fatalf("Made %d requests, want the api-settings read to have been attempted", got)
	}
	if !diagnosticsContain(resp.Diagnostics, "Unable to read organization API settings") {
		t.Fatalf("Update() diagnostics = %v, want the read failure reported", resp.Diagnostics)
	}

	var persisted organizationResourceModel
	if diags := resp.State.Get(ctx, &persisted); diags.HasError() {
		t.Fatalf("Reading the persisted state = %v", diags)
	}
	if got := persisted.RevokeInactiveTokensAfter.ValueString(); got != "DAYS_30" {
		t.Errorf("Persisted revoke_inactive_tokens_after = %q, want %q: nothing applied, so the prior value stands", got, "DAYS_30")
	}
	if !persisted.RestrictUserApiTokenCreation.ValueBool() {
		t.Error("Persisted restrict_user_api_token_creation = false, want true: nothing applied, so the prior value stands")
	}
	if !persisted.Enforce2FA.ValueBool() {
		t.Error("Persisted enforce_2fa = false, want true: nothing applied, so the prior value stands")
	}
}
