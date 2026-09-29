package buildkite

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
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
		name    string
		config  organizationResourceModel
		plan    organizationResourceModel
		current organizationAPISettings
		want    string
	}{
		{"unset attributes are not sent", unset, model(noList, types.StringUnknown(), types.BoolUnknown()), organizationAPISettings{RevokeInactiveTokensAfterDays: days(30), RestrictUserApiTokenCreation: true}, `{}`},
		{"values kept from state for unset attributes are not sent", unset, model(noList, types.StringValue("DAYS_90"), types.BoolValue(true)), organizationAPISettings{}, `{}`},
		{"unchanged values are not sent", model(noList, types.StringValue("DAYS_90"), types.BoolValue(true)), model(noList, types.StringValue("DAYS_90"), types.BoolValue(true)), organizationAPISettings{RevokeInactiveTokensAfterDays: days(90), RestrictUserApiTokenCreation: true}, `{}`},
		{"changed period is sent", model(noList, types.StringValue("DAYS_60"), types.BoolNull()), model(noList, types.StringValue("DAYS_60"), types.BoolValue(false)), organizationAPISettings{RevokeInactiveTokensAfterDays: days(90)}, `{"revoke_inactive_tokens_after_days":60}`},
		{"never is sent as null", model(noList, types.StringValue("NEVER"), types.BoolValue(false)), model(noList, types.StringValue("NEVER"), types.BoolValue(false)), organizationAPISettings{RevokeInactiveTokensAfterDays: days(90)}, `{"revoke_inactive_tokens_after_days":null}`},
		{"changed restriction is sent", model(noList, types.StringNull(), types.BoolValue(false)), model(noList, types.StringValue("NEVER"), types.BoolValue(false)), organizationAPISettings{RestrictUserApiTokenCreation: true}, `{"restrict_user_api_token_creation":false}`},
		{"both are sent", model(noList, types.StringValue("DAYS_365"), types.BoolValue(true)), model(noList, types.StringValue("DAYS_365"), types.BoolValue(true)), organizationAPISettings{}, `{"restrict_user_api_token_creation":true,"revoke_inactive_tokens_after_days":365}`},
		// the allowlist is owned outright, so the plan says what it should be with no help from config
		{"allowlist is sent", model(list("1.1.1.1/32"), types.StringNull(), types.BoolNull()), model(list("1.1.1.1/32"), types.StringValue("NEVER"), types.BoolValue(false)), organizationAPISettings{}, `{"allowed_ip_addresses":"1.1.1.1/32"}`},
		{"unchanged allowlist is not sent", model(list("1.1.1.1/32", "0.0.0.0/0"), types.StringNull(), types.BoolNull()), model(list("1.1.1.1/32", "0.0.0.0/0"), types.StringValue("NEVER"), types.BoolValue(false)), organizationAPISettings{AllowedIpAddresses: "1.1.1.1/32 0.0.0.0/0"}, `{}`},
		{"removed allowlist is cleared", unset, model(noList, types.StringValue("NEVER"), types.BoolValue(false)), organizationAPISettings{AllowedIpAddresses: "1.1.1.1/32"}, `{"allowed_ip_addresses":""}`},
		{"empty string clears the allowlist", model(list(""), types.StringNull(), types.BoolNull()), model(list(""), types.StringValue("NEVER"), types.BoolValue(false)), organizationAPISettings{AllowedIpAddresses: "1.1.1.1/32"}, `{"allowed_ip_addresses":""}`},
		// an organization without the feature is refused even an unchanged allowlist, so it is left out
		{"unset allowlist is not sent to an organization without one", unset, model(noList, types.StringValue("NEVER"), types.BoolValue(false)), organizationAPISettings{}, `{}`},
		{"allowlist and a token setting travel together", model(list("1.1.1.1/32"), types.StringNull(), types.BoolValue(true)), model(list("1.1.1.1/32"), types.StringValue("NEVER"), types.BoolValue(true)), organizationAPISettings{}, `{"allowed_ip_addresses":"1.1.1.1/32","restrict_user_api_token_creation":true}`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(apiSettingsPatch(&tc.config, &tc.plan, &tc.current))
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

// A failed PATCH has to leave the api-settings attributes describing the organization rather than
// the plan, or state claims a setting that was never applied. Update persists state on that path,
// and readAPISettings falls back to state when a refresh cannot read the settings, so a wrong value
// put there once is re-adopted rather than replanned and never shows in a plan. Only a 4xx says the
// PATCH was refused: after anything else it may have landed, so the organization is read again.
func TestUpdateAPISettingsReportsTheOrganizationWhenThePatchFails(t *testing.T) {
	t.Parallel()

	before := stubResponse{status: http.StatusOK, body: `{"allowed_ip_addresses":"9.9.9.9/32","revoke_inactive_tokens_after_days":null,"restrict_user_api_token_creation":false}`}
	after := stubResponse{status: http.StatusOK, body: `{"allowed_ip_addresses":"1.2.3.4/32","revoke_inactive_tokens_after_days":30,"restrict_user_api_token_creation":true}`}
	serverError := stubResponse{status: http.StatusInternalServerError, body: `{"message":"patch failed"}`}

	tests := []struct {
		name      string
		responses []stubResponse
		// the attributes state should end up with, which is what the organization has
		wantAllowed  string
		wantRevoke   string
		wantRestrict bool
		wantApplied  int
		wantDetail   string
		// the read, the PATCH, and the read back that only a PATCH which may have landed gets
		wantRequests int64
	}{
		{
			name:        "a refused patch changed nothing, so it is not read back",
			responses:   []stubResponse{before, {status: http.StatusUnprocessableEntity, body: `{"message":"invalid"}`}, after},
			wantAllowed: "9.9.9.9/32", wantRevoke: revokeInactiveTokensNever, wantRestrict: false,
			wantRequests: 2,
		},
		{
			name:        "a 5xx after the patch landed records what landed",
			responses:   []stubResponse{before, serverError, after},
			wantAllowed: "1.2.3.4/32", wantRevoke: "DAYS_30", wantRestrict: true, wantApplied: 3, wantRequests: 3,
		},
		{
			name:        "an undecodable response after the patch landed records what landed",
			responses:   []stubResponse{before, {status: http.StatusOK, body: `not json`}, after},
			wantAllowed: "1.2.3.4/32", wantRevoke: "DAYS_30", wantRestrict: true, wantApplied: 3, wantRequests: 3,
		},
		{
			name:        "a 5xx that cannot be read back keeps the values from before it",
			responses:   []stubResponse{before, serverError, serverError},
			wantAllowed: "9.9.9.9/32", wantRevoke: revokeInactiveTokensNever, wantRestrict: false,
			wantDetail: "could not be read back", wantRequests: 3,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server, requests := newRetryStub(t, testCase.responses...)
			defer server.Close()

			o := &organizationResource{client: newRetryTestClient(t, server.URL, 0, time.Millisecond)}

			ctx := t.Context()
			configured := organizationResourceModel{
				AllowedApiIpAddresses:        listOfStrings(ctx, t, "1.2.3.4/32"),
				RevokeInactiveTokensAfter:    types.StringValue("DAYS_30"),
				RestrictUserApiTokenCreation: types.BoolValue(true),
			}
			// as Update seeds it, from the prior state
			state := organizationResourceModel{
				AllowedApiIpAddresses:        listOfStrings(ctx, t, "9.9.9.9/32"),
				RevokeInactiveTokensAfter:    types.StringValue(revokeInactiveTokensNever),
				RestrictUserApiTokenCreation: types.BoolValue(false),
			}

			var diags diag.Diagnostics
			applied := o.updateAPISettings(ctx, &configured, &configured, &state, &diags)

			if !diagnosticsContain(diags, "Unable to update organization API settings") {
				t.Fatalf("updateAPISettings diagnostics = %v, want the PATCH failure reported", diags)
			}
			if testCase.wantDetail != "" && !strings.Contains(fmt.Sprint(diags), testCase.wantDetail) {
				t.Errorf("updateAPISettings diagnostics = %v, want them to mention %q", diags, testCase.wantDetail)
			}
			if got := requests.Load(); got != testCase.wantRequests {
				t.Errorf("Made %d requests, want %d", got, testCase.wantRequests)
			}
			if len(applied) != testCase.wantApplied {
				t.Errorf("updateAPISettings applied = %q, want %d changes", applied, testCase.wantApplied)
			}
			if got := allowedApiIpAddressesValue(state.AllowedApiIpAddresses); got != testCase.wantAllowed {
				t.Errorf("Persisted allowed_api_ip_addresses = %q, want %q", got, testCase.wantAllowed)
			}
			if got := state.RevokeInactiveTokensAfter.ValueString(); got != testCase.wantRevoke {
				t.Errorf("Persisted revoke_inactive_tokens_after = %q, want %q", got, testCase.wantRevoke)
			}
			if got := state.RestrictUserApiTokenCreation.ValueBool(); got != testCase.wantRestrict {
				t.Errorf("Persisted restrict_user_api_token_creation = %t, want %t", got, testCase.wantRestrict)
			}
		})
	}
}

// Update writes the api-settings before it changes 2FA, so a 2FA failure must not drop the settings
// the patch applied: Terraform would plan them again, and in the meantime state disagrees with the
// organization.
func TestOrganizationUpdatePersistsTheAppliedAPISettingsWhen2FAFails(t *testing.T) {
	t.Parallel()

	server, requests := newRetryStub(t,
		// The api-settings GET, then a PATCH that applies the new period.
		stubResponse{status: http.StatusOK, body: `{"allowed_ip_addresses":"","revoke_inactive_tokens_after_days":null,"restrict_user_api_token_creation":false}`},
		stubResponse{status: http.StatusOK, body: `{"allowed_ip_addresses":"","revoke_inactive_tokens_after_days":30,"restrict_user_api_token_creation":false}`},
		// setOrganization2FA, which does not.
		stubResponse{status: http.StatusOK, body: `{"errors":[{"message":"mutation exploded"}]}`},
	)
	defer server.Close()

	client := newRetryTestClient(t, server.URL, 0, time.Millisecond)
	orgID := "organization-id"
	client.organizationId = &orgID
	o := &organizationResource{client: client}

	ctx := t.Context()
	sch := resourceSchema(ctx, t, o)

	// An unchanged allowlist, so it stays out of the patch and only the period is sent.
	allowlist := tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{})
	prior := nullObjectWith(ctx, t, sch.Type(), map[string]tftypes.Value{
		"id":                           tftypes.NewValue(tftypes.String, "organization-id"),
		"uuid":                         tftypes.NewValue(tftypes.String, "organization-uuid"),
		"allowed_api_ip_addresses":     allowlist,
		"enforce_2fa":                  tftypes.NewValue(tftypes.Bool, false),
		"revoke_inactive_tokens_after": tftypes.NewValue(tftypes.String, revokeInactiveTokensNever),
	})
	planned := nullObjectWith(ctx, t, sch.Type(), map[string]tftypes.Value{
		"id":                           tftypes.NewValue(tftypes.String, "organization-id"),
		"uuid":                         tftypes.NewValue(tftypes.String, "organization-uuid"),
		"allowed_api_ip_addresses":     allowlist,
		"enforce_2fa":                  tftypes.NewValue(tftypes.Bool, true),
		"revoke_inactive_tokens_after": tftypes.NewValue(tftypes.String, "DAYS_30"),
	})

	req := fwresource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: sch, Raw: planned},
		State:  tfsdk.State{Schema: sch, Raw: prior},
		Config: tfsdk.Config{Schema: sch, Raw: planned},
	}
	resp := fwresource.UpdateResponse{State: tfsdk.State{Schema: sch, Raw: prior}}

	o.Update(ctx, req, &resp)

	if got := requests.Load(); got < 3 {
		t.Fatalf("Made %d requests, want 3: the api-settings read and patch have to precede the failing 2FA mutation", got)
	}
	if !diagnosticsContain(resp.Diagnostics, "Unable to set 2FA") {
		t.Fatalf("Update() diagnostics = %v, want the 2FA failure reported", resp.Diagnostics)
	}

	var persisted organizationResourceModel
	if diags := resp.State.Get(ctx, &persisted); diags.HasError() {
		t.Fatalf("Reading the persisted state = %v", diags)
	}
	if got := persisted.RevokeInactiveTokensAfter.ValueString(); got != "DAYS_30" {
		t.Errorf("Persisted revoke_inactive_tokens_after = %q, want %q: the patch applied, so state has to say so", got, "DAYS_30")
	}
	if persisted.Enforce2FA.ValueBool() {
		t.Error("Persisted enforce_2fa = true, want false: the 2FA mutation failed, so it never applied")
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

	organizationIs := func(enforced2FA bool) stubResponse {
		return stubResponse{status: http.StatusOK, body: fmt.Sprintf(`{"data":{"organization":{
			"id": "organization-id",
			"uuid": "organization-uuid",
			"membersRequireTwoFactorAuthentication": %t
		}}}`, enforced2FA)}
	}
	apiSettingsAre := func(allowed string) stubResponse {
		return stubResponse{status: http.StatusOK, body: fmt.Sprintf(
			`{"allowed_ip_addresses":%q,"revoke_inactive_tokens_after_days":null,"restrict_user_api_token_creation":false}`, allowed)}
	}
	patchApplied := stubResponse{status: http.StatusOK, body: `{"allowed_ip_addresses":"","revoke_inactive_tokens_after_days":null,"restrict_user_api_token_creation":false}`}
	twoFAFails := stubResponse{status: http.StatusOK, body: `{"errors":[{"message":"mutation exploded"}]}`}

	tests := []struct {
		name string
		// The allowlist the config asks for. Unset means the attribute is absent, which clears it.
		configuredAllowlist string
		// Set when the config also manages revoke_inactive_tokens_after.
		configuredRevoke string
		// Set when the config also manages restrict_user_api_token_creation.
		configuredRestrict bool
		responses          []stubResponse
		// Substrings the single warning has to contain, or none to assert there is no warning.
		wantWarned []string
	}{
		{
			name:                "allowlist applied, then the 2FA mutation fails",
			configuredAllowlist: configuredAllowlist,
			responses:           []stubResponse{organizationIs(false), apiSettingsAre(""), patchApplied, twoFAFails},
			wantWarned:          []string{`allowlist was set to "1.2.3.4/32"`},
		},
		{
			// Already what the config asks for, so the patch is empty, no request is made, and this
			// apply is not responsible for the allowlist being in place.
			name:                "allowlist already matched",
			configuredAllowlist: configuredAllowlist,
			responses:           []stubResponse{organizationIs(false), apiSettingsAre(configuredAllowlist), twoFAFails},
		},
		{
			// No allowlist in the config against an organization that has one, which clears it. The
			// wording differs from a set, because "set to \"\"" would read as a change to nothing.
			name:       "allowlist cleared, then the 2FA mutation fails",
			responses:  []stubResponse{organizationIs(false), apiSettingsAre(configuredAllowlist), patchApplied, twoFAFails},
			wantWarned: []string{"allowlist was cleared"},
		},
		{
			// The allowlist is unchanged here, so the warning must name the period and not it.
			name:             "revocation period applied, then the 2FA mutation fails",
			configuredRevoke: "DAYS_30",
			responses:        []stubResponse{organizationIs(false), apiSettingsAre(""), patchApplied, twoFAFails},
			wantWarned:       []string{"inactive API token revocation was set to DAYS_30"},
		},
		{
			// One PATCH carrying every api-setting, so the warning has to list all three as a sentence.
			name:                "every api-setting applied, then the 2FA mutation fails",
			configuredAllowlist: configuredAllowlist,
			configuredRevoke:    "DAYS_30",
			configuredRestrict:  true,
			responses:           []stubResponse{organizationIs(false), apiSettingsAre(""), patchApplied, twoFAFails},
			wantWarned: []string{`Before this operation failed, the API IP allowlist was set to "1.2.3.4/32", ` +
				"inactive API token revocation was set to DAYS_30, and user API token creation was restricted."},
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
			sch := resourceSchema(ctx, t, o)

			attributes := map[string]tftypes.Value{"enforce_2fa": tftypes.NewValue(tftypes.Bool, true)}
			if testCase.configuredAllowlist != "" {
				attributes["allowed_api_ip_addresses"] = tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{
					tftypes.NewValue(tftypes.String, testCase.configuredAllowlist),
				})
			}
			if testCase.configuredRevoke != "" {
				attributes["revoke_inactive_tokens_after"] = tftypes.NewValue(tftypes.String, testCase.configuredRevoke)
			}
			if testCase.configuredRestrict {
				attributes["restrict_user_api_token_creation"] = tftypes.NewValue(tftypes.Bool, true)
			}
			raw := nullObjectWith(ctx, t, sch.Type(), attributes)

			req := fwresource.CreateRequest{
				Plan:   tfsdk.Plan{Schema: sch, Raw: raw},
				Config: tfsdk.Config{Schema: sch, Raw: raw},
			}
			resp := fwresource.CreateResponse{State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}}

			o.Create(ctx, req, &resp)

			if got := requests.Load(); got < int64(len(testCase.responses)) {
				t.Fatalf("Made %d requests, want %d: the failure has to come from the last stubbed response", got, len(testCase.responses))
			}
			if !diagnosticsContain(resp.Diagnostics, "Unable to set 2FA") {
				t.Fatalf("Create() diagnostics = %v, want the 2FA failure reported", resp.Diagnostics)
			}
			if !resp.State.Raw.IsNull() {
				t.Errorf("Create() persisted %v, want no state: persisting taints the instance, and replacing it clears the API IP allowlist", resp.State.Raw)
			}

			warnings := resp.Diagnostics.Warnings()
			var detail string
			for _, d := range warnings {
				if d.Summary() == "Organization settings changed but not recorded" {
					detail = d.Detail()
				}
			}
			if len(testCase.wantWarned) == 0 {
				if detail != "" {
					t.Errorf("Create() warned %q, want no warning: this apply changed nothing before it failed", detail)
				}
				return
			}
			if detail == "" {
				t.Fatalf("Create() warnings = %v, want one naming the settings that applied", warnings)
			}
			for _, want := range testCase.wantWarned {
				if !strings.Contains(detail, want) {
					t.Errorf("Warning detail = %q, want it to mention %q", detail, want)
				}
			}
			if testCase.configuredAllowlist == "" && testCase.configuredRevoke != "" && strings.Contains(detail, "allowlist") {
				t.Errorf("Warning detail = %q, want no mention of the allowlist: this apply did not change it", detail)
			}
		})
	}
}

// A failed api-settings GET makes updateAPISettings return before it assigns any of the attributes
// it owns. Update persists unconditionally now, so without seeding state from the prior values
// first, that path writes nulls over settings the organization still has. readAPISettings falls back
// to state when a refresh cannot read the settings, so a null put there once is re-adopted.
func TestOrganizationUpdateKeepsPriorAPISettingsWhenTheReadFails(t *testing.T) {
	t.Parallel()

	// Only the api-settings GET is reached: it precedes both the patch and the 2FA mutation.
	server, requests := newRetryStub(t,
		stubResponse{status: http.StatusInternalServerError, body: `{"message":"api-settings unavailable"}`},
	)
	defer server.Close()

	client := newRetryTestClient(t, server.URL, 0, time.Millisecond)
	orgID := "organization-id"
	client.organizationId = &orgID
	o := &organizationResource{client: client}

	ctx := t.Context()
	sch := resourceSchema(ctx, t, o)

	allowlist := tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{
		tftypes.NewValue(tftypes.String, "9.9.9.9/32"),
	})
	prior := nullObjectWith(ctx, t, sch.Type(), map[string]tftypes.Value{
		"id":                               tftypes.NewValue(tftypes.String, "organization-id"),
		"uuid":                             tftypes.NewValue(tftypes.String, "organization-uuid"),
		"allowed_api_ip_addresses":         allowlist,
		"enforce_2fa":                      tftypes.NewValue(tftypes.Bool, true),
		"revoke_inactive_tokens_after":     tftypes.NewValue(tftypes.String, "DAYS_30"),
		"restrict_user_api_token_creation": tftypes.NewValue(tftypes.Bool, true),
	})
	// Only revoke_inactive_tokens_after changes, and everything is applied after the GET, so nothing
	// runs before the read fails.
	planned := nullObjectWith(ctx, t, sch.Type(), map[string]tftypes.Value{
		"id":                               tftypes.NewValue(tftypes.String, "organization-id"),
		"uuid":                             tftypes.NewValue(tftypes.String, "organization-uuid"),
		"allowed_api_ip_addresses":         allowlist,
		"enforce_2fa":                      tftypes.NewValue(tftypes.Bool, true),
		"revoke_inactive_tokens_after":     tftypes.NewValue(tftypes.String, "DAYS_90"),
		"restrict_user_api_token_creation": tftypes.NewValue(tftypes.Bool, true),
	})

	req := fwresource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: sch, Raw: planned},
		State:  tfsdk.State{Schema: sch, Raw: prior},
		Config: tfsdk.Config{Schema: sch, Raw: planned},
	}
	resp := fwresource.UpdateResponse{State: tfsdk.State{Schema: sch, Raw: prior}}

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
	if got := allowedApiIpAddressesValue(persisted.AllowedApiIpAddresses); got != "9.9.9.9/32" {
		t.Errorf("Persisted allowed_api_ip_addresses = %q, want %q: nothing applied, so the prior value stands", got, "9.9.9.9/32")
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

// Each api-setting a patch carried is named in the attribute's terms, with the wording that fits the
// value it was set to rather than a raw value such as "set to NEVER" or "set to false".
func TestDescribeAPISettingsChanges(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	tests := []struct {
		name    string
		payload map[string]any
		plan    organizationResourceModel
		want    []string
	}{
		{
			name:    "allowlist set",
			payload: map[string]any{"allowed_ip_addresses": "1.2.3.4/32 5.6.7.8/32"},
			plan:    organizationResourceModel{AllowedApiIpAddresses: listOfStrings(ctx, t, "1.2.3.4/32", "5.6.7.8/32")},
			want:    []string{`the API IP allowlist was set to "1.2.3.4/32 5.6.7.8/32"`},
		},
		{
			name:    "allowlist cleared",
			payload: map[string]any{"allowed_ip_addresses": ""},
			plan:    organizationResourceModel{AllowedApiIpAddresses: types.ListNull(types.StringType)},
			want:    []string{"the API IP allowlist was cleared"},
		},
		{
			name:    "revocation set to a period",
			payload: map[string]any{"revoke_inactive_tokens_after_days": revokePeriodToDays("DAYS_90")},
			plan:    organizationResourceModel{RevokeInactiveTokensAfter: types.StringValue("DAYS_90")},
			want:    []string{"inactive API token revocation was set to DAYS_90"},
		},
		{
			name:    "revocation turned off",
			payload: map[string]any{"revoke_inactive_tokens_after_days": revokePeriodToDays(revokeInactiveTokensNever)},
			plan:    organizationResourceModel{RevokeInactiveTokensAfter: types.StringValue(revokeInactiveTokensNever)},
			want:    []string{"inactive API token revocation was turned off"},
		},
		{
			name:    "token creation restricted",
			payload: map[string]any{"restrict_user_api_token_creation": true},
			plan:    organizationResourceModel{RestrictUserApiTokenCreation: types.BoolValue(true)},
			want:    []string{"user API token creation was restricted"},
		},
		{
			name:    "token creation restriction removed",
			payload: map[string]any{"restrict_user_api_token_creation": false},
			plan:    organizationResourceModel{RestrictUserApiTokenCreation: types.BoolValue(false)},
			want:    []string{"the restriction on user API token creation was removed"},
		},
		{
			// a setting the plan holds but the patch did not carry was not changed by it
			name:    "nothing sent",
			payload: map[string]any{},
			plan:    organizationResourceModel{RestrictUserApiTokenCreation: types.BoolValue(true)},
			want:    []string{},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := describeAPISettingsChanges(testCase.payload, &testCase.plan); !slices.Equal(got, testCase.want) {
				t.Errorf("describeAPISettingsChanges() = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestJoinChanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		changes []string
		want    string
	}{
		{[]string{"a"}, "a"},
		{[]string{"a", "b"}, "a and b"},
		{[]string{"a", "b", "c"}, "a, b, and c"},
	}

	for _, testCase := range tests {
		if got := joinChanges(testCase.changes); got != testCase.want {
			t.Errorf("joinChanges(%q) = %q, want %q", testCase.changes, got, testCase.want)
		}
	}
}

// listOfStrings builds the attribute value for a known-good allowlist
func listOfStrings(ctx context.Context, t *testing.T, values ...string) types.List {
	t.Helper()

	list, diags := types.ListValueFrom(ctx, types.StringType, values)
	if diags.HasError() {
		t.Fatalf("building a list of %q = %v", values, diags)
	}
	return list
}
