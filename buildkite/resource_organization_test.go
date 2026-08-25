package buildkite

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
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
	list := func(cidrs ...string) types.List {
		values := make([]attr.Value, len(cidrs))
		for i, c := range cidrs {
			values[i] = types.StringValue(c)
		}
		return types.ListValueMust(types.StringType, values)
	}
	noList := types.ListNull(types.StringType)
	model := func(allowed types.List, revoke types.String, restrict types.Bool) organizationResourceModel {
		return organizationResourceModel{AllowedApiIpAddresses: allowed, RevokeInactiveTokensAfter: revoke, RestrictUserApiTokenCreation: restrict}
	}
	unset := model(noList, types.StringNull(), types.BoolNull())
	testCases := []struct {
		name         string
		config       organizationResourceModel
		plan         organizationResourceModel
		current      organizationAPISettings
		currentKnown bool
		want         string
	}{
		{"unset attributes are not sent", unset, model(noList, types.StringUnknown(), types.BoolUnknown()), organizationAPISettings{RevokeInactiveTokensAfterDays: days(30), RestrictUserApiTokenCreation: true}, true, `{}`},
		{"values kept from state for unset attributes are not sent", unset, model(noList, types.StringValue("DAYS_90"), types.BoolValue(true)), organizationAPISettings{}, true, `{}`},
		{"unchanged values are not sent", model(noList, types.StringValue("DAYS_90"), types.BoolValue(true)), model(noList, types.StringValue("DAYS_90"), types.BoolValue(true)), organizationAPISettings{RevokeInactiveTokensAfterDays: days(90), RestrictUserApiTokenCreation: true}, true, `{}`},
		{"changed period is sent", model(noList, types.StringValue("DAYS_60"), types.BoolNull()), model(noList, types.StringValue("DAYS_60"), types.BoolValue(false)), organizationAPISettings{RevokeInactiveTokensAfterDays: days(90)}, true, `{"revoke_inactive_tokens_after_days":60}`},
		{"never is sent as null", model(noList, types.StringValue("NEVER"), types.BoolValue(false)), model(noList, types.StringValue("NEVER"), types.BoolValue(false)), organizationAPISettings{RevokeInactiveTokensAfterDays: days(90)}, true, `{"revoke_inactive_tokens_after_days":null}`},
		{"changed restriction is sent", model(noList, types.StringNull(), types.BoolValue(false)), model(noList, types.StringValue("NEVER"), types.BoolValue(false)), organizationAPISettings{RestrictUserApiTokenCreation: true}, true, `{"restrict_user_api_token_creation":false}`},
		{"both are sent", model(noList, types.StringValue("DAYS_365"), types.BoolValue(true)), model(noList, types.StringValue("DAYS_365"), types.BoolValue(true)), organizationAPISettings{}, true, `{"restrict_user_api_token_creation":true,"revoke_inactive_tokens_after_days":365}`},
		// when the current settings can't be read, configured values are always sent
		{"unknown current and explicit false", model(noList, types.StringNull(), types.BoolValue(false)), model(noList, types.StringUnknown(), types.BoolValue(false)), organizationAPISettings{}, false, `{"restrict_user_api_token_creation":false}`},
		{"unknown current and explicit never", model(noList, types.StringValue("NEVER"), types.BoolNull()), model(noList, types.StringValue("NEVER"), types.BoolUnknown()), organizationAPISettings{}, false, `{"revoke_inactive_tokens_after_days":null}`},
		{"unknown current and nothing configured", unset, model(noList, types.StringValue("DAYS_30"), types.BoolValue(true)), organizationAPISettings{}, false, `{}`},
		// the allowlist is owned outright, so the plan says what it should be with no help from config
		{"allowlist is sent", model(list("1.1.1.1/32"), types.StringNull(), types.BoolNull()), model(list("1.1.1.1/32"), types.StringValue("NEVER"), types.BoolValue(false)), organizationAPISettings{}, true, `{"allowed_ip_addresses":"1.1.1.1/32"}`},
		{"unchanged allowlist is not sent", model(list("1.1.1.1/32", "0.0.0.0/0"), types.StringNull(), types.BoolNull()), model(list("1.1.1.1/32", "0.0.0.0/0"), types.StringValue("NEVER"), types.BoolValue(false)), organizationAPISettings{AllowedIpAddresses: "1.1.1.1/32 0.0.0.0/0"}, true, `{}`},
		{"removed allowlist is cleared", unset, model(noList, types.StringValue("NEVER"), types.BoolValue(false)), organizationAPISettings{AllowedIpAddresses: "1.1.1.1/32"}, true, `{"allowed_ip_addresses":""}`},
		{"empty string clears the allowlist", model(list(""), types.StringNull(), types.BoolNull()), model(list(""), types.StringValue("NEVER"), types.BoolValue(false)), organizationAPISettings{AllowedIpAddresses: "1.1.1.1/32"}, true, `{"allowed_ip_addresses":""}`},
		// an organization without the feature is refused even an unchanged allowlist, so it is left out
		{"unset allowlist is not sent to an organization without one", unset, model(noList, types.StringValue("NEVER"), types.BoolValue(false)), organizationAPISettings{}, false, `{}`},
		{"allowlist and a token setting travel together", model(list("1.1.1.1/32"), types.StringNull(), types.BoolValue(true)), model(list("1.1.1.1/32"), types.StringValue("NEVER"), types.BoolValue(true)), organizationAPISettings{}, true, `{"allowed_ip_addresses":"1.1.1.1/32","restrict_user_api_token_creation":true}`},
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
			if _, err := getTestClient().updateOrganizationAPISettings(context.Background(), map[string]any{"allowed_ip_addresses": "0.0.0.0/0"}); err != nil {
				t.Fatalf("Unable to preset the allowed API IP addresses: %v", err)
			}
			if _, err := getTestClient().getOrganizationAPISettings(context.Background()); err != nil {
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
		settings, err := getTestClient().getOrganizationAPISettings(context.Background())
		if err != nil {
			return err
		}

		if settings.AllowedIpAddresses != strings.Join(ip_addresses, " ") {
			return fmt.Errorf("Allowed IP addresses do not match. Expected: %s, got: %s", ip_addresses, settings.AllowedIpAddresses)
		}
		return nil
	}
}
