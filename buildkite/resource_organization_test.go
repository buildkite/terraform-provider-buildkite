package buildkite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccBuildkiteOrganizationResource(t *testing.T) {
	// The timeouts keep a run that loses API access to a bad allowlist from hanging on the retries.
	orgConfig := func(attributes string) string {
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
		`, attributes)
	}

	config := func(ip_addresses []string) string {
		marshal, _ := json.Marshal(ip_addresses)

		return orgConfig(fmt.Sprintf("allowed_api_ip_addresses = %s", string(marshal)))
	}

	configNoSettings := func() string {
		return orgConfig("")
	}

	// Leaves allowed_api_ip_addresses out, so the request this drives names only the ungated setting.
	// Whether the target organization is entitled to the allowlist is not what this asserts; that is
	// pinned by TestOrganizationAPISettingsPatchBody.
	configRestrictTokenCreation := func(restrict bool) string {
		return orgConfig(fmt.Sprintf("restrict_user_api_token_creation = %t", restrict))
	}

	configRevokeInactiveTokens := func(days int) string {
		return orgConfig(fmt.Sprintf("revoke_inactive_tokens_after_days = %d", days))
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
					Config: configNoSettings(),
					Check:  checkUpdated,
				},
			},
		})
	})

	t.Run("restricts user API token creation without touching the allowlist", func(t *testing.T) {
		resource.Test(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t); skipIfOrganizationHasAllowlist(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testCheckOrganizationResourceRemoved,
			Steps: []resource.TestStep{
				{
					Config: configRestrictTokenCreation(true),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_organization.let_them_in", "restrict_user_api_token_creation", "true"),
						// Create leaves the allowlist alone, so it stays out of state, which the precheck is
						// what makes true: an organization with an allowlist of its own would read one back and
						// leave the step's implicit plan non-empty.
						resource.TestCheckNoResourceAttr("buildkite_organization.let_them_in", "allowed_api_ip_addresses"),
						testAccCheckOrganizationAPISettingsRemoteValues(func(settings *organizationAPISettings) error {
							if !settings.RestrictUserAPITokenCreation {
								return fmt.Errorf("Expected restrict_user_api_token_creation to be true in Buildkite's system")
							}
							return nil
						}),
					),
				},
				{
					Config: configRestrictTokenCreation(false),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_organization.let_them_in", "restrict_user_api_token_creation", "false"),
						testAccCheckOrganizationAPISettingsRemoteValues(func(settings *organizationAPISettings) error {
							if settings.RestrictUserAPITokenCreation {
								return fmt.Errorf("Expected restrict_user_api_token_creation to be false in Buildkite's system")
							}
							return nil
						}),
					),
				},
			},
		})
	})

	t.Run("revokes inactive API tokens", func(t *testing.T) {
		resource.Test(t, resource.TestCase{
			// Inside PreCheck, so the credentials it needs are only reached once TF_ACC is set.
			PreCheck:                 func() { testAccPreCheck(t); skipUnlessInactiveTokenRevocationTestable(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testCheckOrganizationResourceRemoved,
			Steps: []resource.TestStep{
				{
					Config: configRevokeInactiveTokens(365),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_organization.let_them_in", "revoke_inactive_tokens_after_days", "365"),
						testAccCheckOrganizationAPISettingsRemoteValues(func(settings *organizationAPISettings) error {
							if settings.RevokeInactiveTokensAfterDays == nil || *settings.RevokeInactiveTokensAfterDays != 365 {
								return fmt.Errorf("Expected 365 days in Buildkite's system, got %v", settings.RevokeInactiveTokensAfterDays)
							}
							return nil
						}),
					),
				},
				{
					Config: configNoSettings(),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckNoResourceAttr("buildkite_organization.let_them_in", "revoke_inactive_tokens_after_days"),
						testAccCheckOrganizationAPISettingsRemoteValues(func(settings *organizationAPISettings) error {
							if settings.RevokeInactiveTokensAfterDays != nil {
								return fmt.Errorf("Expected revocation to be off in Buildkite's system, got %d", *settings.RevokeInactiveTokensAfterDays)
							}
							return nil
						}),
					),
				},
			},
		})
	})

	// The provider version this replaces read an empty allowlist back as a one-element list holding "",
	// so state written by it has to settle here in a single apply. It also must not name the gated
	// setting on the way, which TestOrganizationAPISettingsPatchBody pins on the request itself.
	t.Run("settles state written before the move to REST", func(t *testing.T) {
		config := `resource "buildkite_organization" "let_them_in" {}`

		// The last release to write the allowlist over GraphQL.
		released := map[string]resource.ExternalProvider{
			"buildkite": {
				Source:            "registry.terraform.io/buildkite/buildkite",
				VersionConstraint: "1.38.0",
			},
		}

		resource.Test(t, resource.TestCase{
			PreCheck:     func() { testAccPreCheck(t) },
			CheckDestroy: testCheckOrganizationResourceRemoved,
			Steps: []resource.TestStep{
				{
					Config:            config,
					ExternalProviders: released,
				},
				{
					// A refresh by that version is what leaves the empty-string element behind, and it
					// reports the attribute it just cleared as a difference.
					RefreshState:      true,
					ExternalProviders: released,
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_organization.let_them_in", "allowed_api_ip_addresses.#", "1"),
						resource.TestCheckResourceAttr("buildkite_organization.let_them_in", "allowed_api_ip_addresses.0", ""),
					),
					ExpectNonEmptyPlan: true,
				},
				{
					Config:                   config,
					ProtoV6ProviderFactories: protoV6ProviderFactories(),
					Check:                    resource.TestCheckNoResourceAttr("buildkite_organization.let_them_in", "allowed_api_ip_addresses"),
				},
			},
		})
	})

	// Create only writes 2FA when the plan disagrees with the organization, and both operations record
	// what the organization actually holds, so the setting has to survive an apply that never names it
	// as well as one that turns it on.
	t.Run("enforces 2FA", func(t *testing.T) {
		enforced := func(want bool) resource.TestCheckFunc {
			return resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("buildkite_organization.let_them_in", "enforce_2fa", strconv.FormatBool(want)),
				func(s *terraform.State) error {
					organization, err := getOrganization(context.Background(), genqlientGraphql, getenv("BUILDKITE_ORGANIZATION_SLUG"))
					if err != nil {
						return err
					}
					if got := organization.Organization.MembersRequireTwoFactorAuthentication; got != want {
						return fmt.Errorf("Expected enforce_2fa to be %t in Buildkite's system, got %t", want, got)
					}
					return nil
				},
			)
		}

		resource.Test(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testCheckOrganizationResourceRemoved,
			Steps: []resource.TestStep{
				// The organization under test starts with 2FA off, so this exercises the branch that skips
				// the mutation entirely.
				{
					Config: configNoSettings(),
					Check:  enforced(false),
				},
				{
					Config: orgConfig("enforce_2fa = true"),
					Check:  enforced(true),
				},
				{
					Config: orgConfig("enforce_2fa = false"),
					Check:  enforced(false),
				},
			},
		})
	})

	t.Run("rejects a revocation interval the API does not accept", func(t *testing.T) {
		resource.Test(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: configRevokeInactiveTokens(45),
					// Loose on whitespace because Terraform wraps diagnostics at the terminal width.
					ExpectError: regexp.MustCompile(`must\s+be\s+one\s+of`),
					PlanOnly:    true,
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

// testCheckOrganizationResourceRemoved asserts that destroying the resource returned the settings it
// managed to their defaults. The organization itself outlives the resource, so there is nothing else
// for a destroy check to look at.
func testCheckOrganizationResourceRemoved(s *terraform.State) error {
	settings, err := getTestClient().GetOrganizationAPISettings(context.Background(), getenv("BUILDKITE_ORGANIZATION_SLUG"))
	if err != nil {
		return err
	}

	if settings.AllowedIPAddresses != nil && len(strings.Fields(*settings.AllowedIPAddresses)) > 0 {
		return fmt.Errorf("Expected the API IP allowlist to be cleared, got %q", *settings.AllowedIPAddresses)
	}
	if settings.RevokeInactiveTokensAfterDays != nil {
		return fmt.Errorf("Expected inactive token revocation to be off, got %d days", *settings.RevokeInactiveTokensAfterDays)
	}
	if settings.RestrictUserAPITokenCreation {
		return fmt.Errorf("Expected restrict_user_api_token_creation to be false")
	}

	return nil
}

// skipIfOrganizationHasAllowlist keeps the tests that assert the allowlist is left untouched off an
// organization that has one, where reading it back would leave a non-empty plan and fail the step
// with a diff rather than a reason.
func skipIfOrganizationHasAllowlist(t *testing.T) {
	t.Helper()

	settings, err := getTestClient().GetOrganizationAPISettings(context.Background(), getenv("BUILDKITE_ORGANIZATION_SLUG"))
	if err != nil {
		t.Fatalf("Unable to read the organization's API settings: %v", err)
	}
	if settings.AllowedIPAddresses != nil && len(strings.Fields(*settings.AllowedIPAddresses)) > 0 {
		t.Skipf("The organization already has an API IP allowlist (%q)", *settings.AllowedIPAddresses)
	}
}

// skipUnlessInactiveTokenRevocationTestable keeps the revocation test off an organization that
// can't run it. The billing plan has to include the feature, and the run has to be opted in:
// switching the setting on revokes every already-inactive token in the organization immediately
// rather than at the next sweep, which is not something to do to a shared test organization
// without being asked.
func skipUnlessInactiveTokenRevocationTestable(t *testing.T) {
	t.Helper()

	if os.Getenv("BUILDKITE_TEST_INACTIVE_TOKEN_REVOCATION") == "" {
		t.Skip("Set BUILDKITE_TEST_INACTIVE_TOKEN_REVOCATION=1 to run this test; it revokes inactive API tokens in the target organization")
	}

	settings, err := getTestClient().GetOrganizationAPISettings(context.Background(), getenv("BUILDKITE_ORGANIZATION_SLUG"))
	if err != nil {
		t.Fatalf("Unable to read the organization's API settings: %v", err)
	}
	if !settings.Features.InactiveAPITokenRevocation {
		t.Skip("The organization's billing plan does not include inactive API token revocation")
	}
}

// testAccCheckOrganizationAPISettingsRemoteValues asserts against what the REST API reports, which
// is the side of the move to REST that state alone can't confirm.
func testAccCheckOrganizationAPISettingsRemoteValues(check func(*organizationAPISettings) error) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		settings, err := getTestClient().GetOrganizationAPISettings(context.Background(), getenv("BUILDKITE_ORGANIZATION_SLUG"))
		if err != nil {
			return err
		}

		return check(settings)
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

func cidrList(cidrs ...string) types.List {
	elements := make([]attr.Value, 0, len(cidrs))
	for _, cidr := range cidrs {
		elements = append(elements, types.StringValue(cidr))
	}

	return types.ListValueMust(types.StringType, elements)
}

// Clearing the allowlist through the API stores a null, but the column is a plain nullable string
// with no normalisation behind it, so an allowlist emptied by some other route can read back as "".
// Either way it has to leave the attribute alone rather than reading back as a single empty CIDR,
// while a value the API does report has to replace state so drift is visible.
func TestReadOrganizationAPISettingsAllowlist(t *testing.T) {
	t.Parallel()

	empty := ""
	single := "1.1.1.1/32"
	multiple := "1.1.1.1/32 0.0.0.0/0"

	tests := []struct {
		name   string
		remote *string
		state  types.List
		want   types.List
	}{
		{"an unset attribute stays null when the API reports nothing", nil, types.ListNull(types.StringType), types.ListNull(types.StringType)},
		{"an empty string is treated the same as null", &empty, types.ListNull(types.StringType), types.ListNull(types.StringType)},
		{"an empty list is left as it is", &empty, cidrList(), cidrList()},
		{"a list holding an empty string round-trips", &empty, cidrList(""), cidrList("")},
		{"a reported allowlist is read onto an unset attribute", &single, types.ListNull(types.StringType), cidrList("1.1.1.1/32")},
		{"a reported allowlist replaces what state held", &multiple, cidrList("1.1.1.1/32"), cidrList("1.1.1.1/32", "0.0.0.0/0")},
		{"an allowlist cleared outside Terraform reads as drift", &empty, cidrList("1.1.1.1/32"), types.ListNull(types.StringType)},
		// The empty element is dropped on the way out, so rewriting state without it would leave a
		// difference against the configuration that no apply could settle.
		{"an empty element alongside real ranges is left in place", &single, cidrList("", "1.1.1.1/32"), cidrList("", "1.1.1.1/32")},
		{"a reordered allowlist reads as drift", &multiple, cidrList("0.0.0.0/0", "1.1.1.1/32"), cidrList("1.1.1.1/32", "0.0.0.0/0")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			state := organizationResourceModel{AllowedApiIpAddresses: tt.state}
			settings := organizationAPISettings{AllowedIPAddresses: tt.remote}

			if diags := readOrganizationAPISettings(context.Background(), &state, &settings); diags.HasError() {
				t.Fatalf("readOrganizationAPISettings reported %v", diags.Errors())
			}
			if !state.AllowedApiIpAddresses.Equal(tt.want) {
				t.Errorf("Expected %s, got %s", tt.want, state.AllowedApiIpAddresses)
			}
		})
	}
}

// Both of these are read straight from the API, so what the practitioner did or did not configure
// shows up as a difference against the plan rather than being hidden here.
func TestReadOrganizationAPISettingsScalars(t *testing.T) {
	t.Parallel()

	ninety := int64(90)

	tests := []struct {
		name           string
		settings       organizationAPISettings
		state          organizationResourceModel
		wantDays       types.Int64
		wantRestricted types.Bool
	}{
		{
			name:           "settings that are off read as null and false",
			state:          organizationResourceModel{RevokeInactiveTokensAfterDays: types.Int64Value(90), RestrictUserApiTokenCreation: types.BoolValue(true)},
			wantDays:       types.Int64Null(),
			wantRestricted: types.BoolValue(false),
		},
		{
			name:           "an interval set outside Terraform is read onto an unset attribute",
			settings:       organizationAPISettings{RevokeInactiveTokensAfterDays: &ninety, RestrictUserAPITokenCreation: true},
			state:          organizationResourceModel{RevokeInactiveTokensAfterDays: types.Int64Null(), RestrictUserApiTokenCreation: types.BoolValue(false)},
			wantDays:       types.Int64Value(90),
			wantRestricted: types.BoolValue(true),
		},
		{
			name:           "an interval changed outside Terraform reads as drift",
			settings:       organizationAPISettings{RevokeInactiveTokensAfterDays: &ninety},
			state:          organizationResourceModel{RevokeInactiveTokensAfterDays: types.Int64Value(30), RestrictUserApiTokenCreation: types.BoolValue(false)},
			wantDays:       types.Int64Value(90),
			wantRestricted: types.BoolValue(false),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			state := tt.state
			state.AllowedApiIpAddresses = types.ListNull(types.StringType)

			if diags := readOrganizationAPISettings(context.Background(), &state, &tt.settings); diags.HasError() {
				t.Fatalf("readOrganizationAPISettings reported %v", diags.Errors())
			}
			if !state.RevokeInactiveTokensAfterDays.Equal(tt.wantDays) {
				t.Errorf("Expected revoke_inactive_tokens_after_days %s, got %s", tt.wantDays, state.RevokeInactiveTokensAfterDays)
			}
			if !state.RestrictUserApiTokenCreation.Equal(tt.wantRestricted) {
				t.Errorf("Expected restrict_user_api_token_creation %s, got %s", tt.wantRestricted, state.RestrictUserApiTokenCreation)
			}
		})
	}
}

// unsetModel is a configuration that names none of the REST-managed settings, which is both the
// starting point for most of these cases and, as state, what an organization Terraform has never
// written looks like.
func unsetModel() organizationResourceModel {
	return organizationResourceModel{
		AllowedApiIpAddresses:         types.ListNull(types.StringType),
		RevokeInactiveTokensAfterDays: types.Int64Null(),
		RestrictUserApiTokenCreation:  types.BoolNull(),
	}
}

func apiSettings(allowlist string, days int64, restrict bool) *organizationAPISettings {
	settings := &organizationAPISettings{RestrictUserAPITokenCreation: restrict}
	if allowlist != "" {
		settings.AllowedIPAddresses = &allowlist
	}
	if days != 0 {
		settings.RevokeInactiveTokensAfterDays = &days
	}

	return settings
}

func TestOrganizationAPISettingsPatchBody(t *testing.T) {
	t.Parallel()

	withValues := func(allowlist types.List, days types.Int64, restrict types.Bool) organizationResourceModel {
		return organizationResourceModel{
			AllowedApiIpAddresses:         allowlist,
			RevokeInactiveTokensAfterDays: days,
			RestrictUserApiTokenCreation:  restrict,
		}
	}

	tests := []struct {
		name   string
		plan   organizationResourceModel
		state  *organizationResourceModel
		remote *organizationAPISettings
		want   map[string]any
	}{
		{
			// The whole point of the endpoint's features map: a setting nobody configured must not be
			// named, because naming a gated one is a 403 and naming the ungated one silently lifts a
			// restriction the organization set for itself.
			name:   "a create that configures nothing writes nothing",
			plan:   unsetModel(),
			remote: apiSettings("10.0.0.0/8", 90, true),
			want:   map[string]any{},
		},
		{
			name:   "create names only the settings the configuration sets",
			plan:   withValues(types.ListNull(types.StringType), types.Int64Null(), types.BoolValue(true)),
			remote: apiSettings("", 0, false),
			want:   map[string]any{"restrict_user_api_token_creation": true},
		},
		{
			name:   "create sends every setting the plan sets",
			plan:   withValues(cidrList("10.0.0.0/8", "192.168.0.0/16"), types.Int64Value(90), types.BoolValue(true)),
			remote: apiSettings("", 0, false),
			want: map[string]any{
				"allowed_ip_addresses":              "10.0.0.0/8 192.168.0.0/16",
				"revoke_inactive_tokens_after_days": int64(90),
				"restrict_user_api_token_creation":  true,
			},
		},
		{
			// Naming a gated setting the organization is not entitled to fails the whole request, so a
			// no-op write of one would take an unrelated apply down with it.
			name:   "a setting that already holds the wanted value is not named",
			plan:   withValues(cidrList("10.0.0.0/8"), types.Int64Value(90), types.BoolValue(true)),
			state:  ptr(withValues(cidrList("10.0.0.0/8"), types.Int64Value(90), types.BoolValue(true))),
			remote: apiSettings("10.0.0.0/8", 90, true),
			want:   map[string]any{},
		},
		{
			name:   "dropping an attribute clears it with an explicit null",
			plan:   unsetModel(),
			state:  ptr(withValues(cidrList("10.0.0.0/8"), types.Int64Value(90), types.BoolValue(true))),
			remote: apiSettings("10.0.0.0/8", 90, true),
			want: map[string]any{
				"allowed_ip_addresses":              nil,
				"revoke_inactive_tokens_after_days": nil,
				"restrict_user_api_token_creation":  false,
			},
		},
		{
			// An empty list says "no restrictions", which the API spells as null, not "".
			name:   "an empty allowlist clears one the organization holds",
			plan:   withValues(cidrList(), types.Int64Null(), types.BoolNull()),
			remote: apiSettings("10.0.0.0/8", 0, false),
			want:   map[string]any{"allowed_ip_addresses": nil},
		},
		{
			// `allowed_api_ip_addresses = []` is a natural thing to write for a conditional list, and on
			// an organization without the entitlement naming the key at all would be a 403.
			name:   "an empty allowlist is not named when the organization has none",
			plan:   withValues(cidrList(""), types.Int64Null(), types.BoolNull()),
			remote: apiSettings("", 0, false),
			want:   map[string]any{},
		},
		{
			// State written before the allowlist moved to REST holds a one-element list containing "".
			name:   "an allowlist an earlier provider version left empty is not named",
			plan:   unsetModel(),
			state:  ptr(withValues(cidrList(""), types.Int64Null(), types.BoolNull())),
			remote: apiSettings("", 0, false),
			want:   map[string]any{},
		},
		{
			name:   "the API's own spacing is not drift",
			plan:   withValues(cidrList("10.0.0.0/8", "192.168.0.0/16"), types.Int64Null(), types.BoolNull()),
			remote: apiSettings("  10.0.0.0/8   192.168.0.0/16 ", 0, false),
			want:   map[string]any{},
		},
		{
			name:   "a reordered allowlist is drift",
			plan:   withValues(cidrList("192.168.0.0/16", "10.0.0.0/8"), types.Int64Null(), types.BoolNull()),
			remote: apiSettings("10.0.0.0/8 192.168.0.0/16", 0, false),
			want:   map[string]any{"allowed_ip_addresses": "192.168.0.0/16 10.0.0.0/8"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assertBodyEqual(t, organizationAPISettingsPatchBody(&tt.plan, tt.state, tt.remote), tt.want)
		})
	}
}

func TestOrganizationAPISettingsResetBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		state  organizationResourceModel
		remote *organizationAPISettings
		want   map[string]any
	}{
		{
			name:   "settings this resource never took over are left alone",
			state:  unsetModel(),
			remote: apiSettings("10.0.0.0/8", 90, true),
			want:   map[string]any{},
		},
		{
			name: "an allowlist an earlier provider version left empty is nothing to reset",
			state: organizationResourceModel{
				AllowedApiIpAddresses:         cidrList(""),
				RevokeInactiveTokensAfterDays: types.Int64Null(),
				RestrictUserApiTokenCreation:  types.BoolNull(),
			},
			remote: apiSettings("", 0, false),
			want:   map[string]any{},
		},
		{
			name: "each managed setting is returned to its default",
			state: organizationResourceModel{
				AllowedApiIpAddresses:         cidrList("10.0.0.0/8"),
				RevokeInactiveTokensAfterDays: types.Int64Value(90),
				RestrictUserApiTokenCreation:  types.BoolValue(true),
			},
			remote: apiSettings("10.0.0.0/8", 90, true),
			want: map[string]any{
				"allowed_ip_addresses":              nil,
				"revoke_inactive_tokens_after_days": nil,
				"restrict_user_api_token_creation":  false,
			},
		},
		{
			name: "only the allowlist is named when it is all this resource took over",
			state: organizationResourceModel{
				AllowedApiIpAddresses:         cidrList("10.0.0.0/8"),
				RevokeInactiveTokensAfterDays: types.Int64Null(),
				RestrictUserApiTokenCreation:  types.BoolNull(),
			},
			remote: apiSettings("10.0.0.0/8", 0, false),
			want:   map[string]any{"allowed_ip_addresses": nil},
		},
		{
			// The one an organization without either entitlement can still be asked.
			name: "only the ungated setting is named when it is all this resource took over",
			state: organizationResourceModel{
				AllowedApiIpAddresses:         types.ListNull(types.StringType),
				RevokeInactiveTokensAfterDays: types.Int64Null(),
				RestrictUserApiTokenCreation:  types.BoolValue(true),
			},
			remote: apiSettings("", 0, true),
			want:   map[string]any{"restrict_user_api_token_creation": false},
		},
		{
			// Someone else turned the setting off between the last apply and the destroy, so there is
			// nothing left to hand back.
			name: "a setting already back at its default is not named",
			state: organizationResourceModel{
				AllowedApiIpAddresses:         cidrList("10.0.0.0/8"),
				RevokeInactiveTokensAfterDays: types.Int64Value(90),
				RestrictUserApiTokenCreation:  types.BoolValue(true),
			},
			remote: apiSettings("", 0, false),
			want:   map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assertBodyEqual(t, organizationAPISettingsResetBody(&tt.state, tt.remote), tt.want)
		})
	}
}

func TestUnavailableSettings(t *testing.T) {
	t.Parallel()

	entitled := organizationAPISettingsFeatures{APIIPAllowList: true, InactiveAPITokenRevocation: true}
	body := func() map[string]any {
		return map[string]any{
			"allowed_ip_addresses":              nil,
			"revoke_inactive_tokens_after_days": nil,
			"restrict_user_api_token_creation":  false,
		}
	}

	if got := unavailableSettings(body(), entitled); len(got) != 0 {
		t.Errorf("Expected an entitled organization to be refused nothing, got %v", got)
	}
	if got := unavailableSettings(map[string]any{"restrict_user_api_token_creation": true}, organizationAPISettingsFeatures{}); len(got) != 0 {
		t.Errorf("Expected the ungated setting to be writable without any entitlement, got %v", got)
	}
	if got := unavailableSettings(body(), organizationAPISettingsFeatures{APIIPAllowList: true}); len(got) != 1 ||
		!strings.Contains(got[0], "revoke_inactive_tokens_after_days") {
		t.Errorf("Expected only the revocation interval to be refused, got %v", got)
	}

	// Dropping is what lets a destroy hand back the settings the organization can still write.
	remaining := body()
	dropped := dropUnavailableSettings(remaining, organizationAPISettingsFeatures{})
	if len(dropped) != 2 {
		t.Errorf("Expected both gated settings to be dropped, got %v", dropped)
	}
	if _, named := remaining["restrict_user_api_token_creation"]; !named || len(remaining) != 1 {
		t.Errorf("Expected the ungated setting to survive on its own, got %v", remaining)
	}
}

func TestOrganizationSettingsError(t *testing.T) {
	t.Parallel()

	refused := &apiError{
		Method:     http.MethodPatch,
		URL:        "https://api.buildkite.com" + testAPISettingsPath,
		StatusCode: http.StatusForbidden,
		Body:       `{"message":"Your billing plan doesn't support the API IP allowlist"}`,
	}

	summary, detail := organizationSettingsError("update", refused)
	if summary != "Unable to update Organization settings" {
		t.Errorf("Expected the action in the summary, got %q", summary)
	}
	// The API's own words come first, because the entitlement they name is what the practitioner acts
	// on, but what was asked of which endpoint still has to be recoverable.
	if !strings.HasPrefix(detail, "Your billing plan doesn't support the API IP allowlist") {
		t.Errorf("Expected the API's explanation to lead, got %q", detail)
	}
	for _, want := range []string{"write_organization_settings", "administrator", "billing plan", refused.URL} {
		if !strings.Contains(detail, want) {
			t.Errorf("Expected %q in the detail, got %q", want, detail)
		}
	}

	// A refused read cannot be about the write scope or the billing plan, so it says neither.
	refusedRead := *refused
	refusedRead.Method = http.MethodGet
	refusedRead.Body = `{"message":"Forbidden"}`
	_, detail = organizationSettingsError("read", &refusedRead)
	if !strings.Contains(detail, "read_organization_settings") {
		t.Errorf("Expected the read scope to be named, got %q", detail)
	}
	for _, unwanted := range []string{"write_organization_settings", "billing plan"} {
		if strings.Contains(detail, unwanted) {
			t.Errorf("Expected no %q advice on a refused read, got %q", unwanted, detail)
		}
	}

	summary, detail = organizationSettingsError("read", errors.New("dial tcp: connection refused"))
	if summary != "Unable to read Organization settings" {
		t.Errorf("Expected the action in the summary, got %q", summary)
	}
	if detail != "dial tcp: connection refused" {
		t.Errorf("Expected a transport failure to be reported on its own, got %q", detail)
	}
}

// Destroying the resource hands the settings it took over back to their defaults. A billing plan
// that no longer covers one of them is the one refusal that must not fail the destroy, because it
// would otherwise repeat on every attempt; everything else has to surface, since a destroy that
// reports success while an IP allowlist stays in force is worse than one that fails.
func TestOrganizationResourceDelete(t *testing.T) {
	t.Parallel()

	state := organizationResourceModel{
		ID:                            types.StringValue("organization-id"),
		UUID:                          types.StringValue("organization-uuid"),
		Enforce2FA:                    types.BoolValue(false),
		AllowedApiIpAddresses:         cidrList("10.0.0.0/8"),
		RevokeInactiveTokensAfterDays: types.Int64Null(),
		RestrictUserApiTokenCreation:  types.BoolValue(true),
	}

	tests := []struct {
		name        string
		state       *organizationResourceModel
		features    string
		patchStatus int
		wantPatch   map[string]any
		wantError   string
		wantWarning string
	}{
		{
			name:      "hands every managed setting back",
			features:  `{"api_ip_allow_list":true,"inactive_api_token_revocation":true}`,
			wantPatch: map[string]any{"allowed_ip_addresses": nil, "restrict_user_api_token_creation": false},
		},
		{
			name:        "keeps going without the settings the plan no longer covers",
			features:    `{"api_ip_allow_list":false,"inactive_api_token_revocation":false}`,
			wantPatch:   map[string]any{"restrict_user_api_token_creation": false},
			wantWarning: "allowed_api_ip_addresses",
		},
		{
			name:        "reports a refusal that is not about the billing plan",
			features:    `{"api_ip_allow_list":true,"inactive_api_token_revocation":true}`,
			patchStatus: http.StatusForbidden,
			wantPatch:   map[string]any{"allowed_ip_addresses": nil, "restrict_user_api_token_creation": false},
			wantError:   "write_organization_settings",
		},
		{
			// Nothing was taken over, so nothing is read and nothing is written: a resource that only
			// ever managed 2FA is destroyable by a token without the REST scopes.
			name:      "asks the API nothing when it took nothing over",
			state:     ptr(unsetModel()),
			wantPatch: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var patched map[string]any
			requests := 0
			client := testAPISettingsClient(t, func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Method == http.MethodPatch {
					if err := json.NewDecoder(r.Body).Decode(&patched); err != nil {
						t.Errorf("Failed to decode request body: %v", err)
					}
					if tt.patchStatus != 0 {
						w.WriteHeader(tt.patchStatus)
						if _, err := w.Write([]byte(`{"message":"Forbidden"}`)); err != nil {
							t.Errorf("Failed to write response: %v", err)
						}
						return
					}
				}

				if _, err := fmt.Fprintf(w, `{
					"allowed_ip_addresses": "10.0.0.0/8",
					"revoke_inactive_tokens_after_days": null,
					"restrict_user_api_token_creation": true,
					"features": %s
				}`, tt.features); err != nil {
					t.Errorf("Failed to write response: %v", err)
				}
			})

			deleting := state
			if tt.state != nil {
				deleting = *tt.state
			}

			resp := &fwresource.DeleteResponse{State: organizationState(t, deleting)}
			(&organizationResource{client: client}).Delete(context.Background(), fwresource.DeleteRequest{
				State: organizationState(t, deleting),
			}, resp)

			assertBodyEqual(t, patched, tt.wantPatch)
			if tt.wantPatch == nil && requests != 0 {
				t.Errorf("Expected the API not to be called at all, got %d requests", requests)
			}

			errors := resp.Diagnostics.Errors()
			if tt.wantError == "" && len(errors) > 0 {
				t.Fatalf("Expected the destroy to succeed, got %v", errors)
			}
			if tt.wantError != "" {
				if len(errors) == 0 {
					t.Fatalf("Expected an error mentioning %q", tt.wantError)
				}
				if !strings.Contains(errors[0].Detail(), tt.wantError) {
					t.Errorf("Expected %q in the error, got %q", tt.wantError, errors[0].Detail())
				}
			}

			warnings := resp.Diagnostics.Warnings()
			if tt.wantWarning != "" && !slices.ContainsFunc(warnings, func(d diag.Diagnostic) bool {
				return strings.Contains(d.Detail(), tt.wantWarning)
			}) {
				t.Errorf("Expected a warning naming %q, got %v", tt.wantWarning, warnings)
			}
		})
	}
}

// organizationState builds the tfsdk.State the framework would hand a CRUD method, which is the only
// way to drive one directly.
func organizationState(t *testing.T, model organizationResourceModel) tfsdk.State {
	t.Helper()

	schemaResp := &fwresource.SchemaResponse{}
	(&organizationResource{}).Schema(context.Background(), fwresource.SchemaRequest{}, schemaResp)

	state := tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), nil)}
	if diags := state.Set(context.Background(), model); diags.HasError() {
		t.Fatalf("Failed to build state: %v", diags.Errors())
	}

	return state
}

func ptr[T any](value T) *T {
	return &value
}

// assertBodyEqual compares the body as the API would see it. Both sides round through JSON first,
// because an omitted key and one holding a typed nil pointer look the same in a map and different on
// the wire, and because a number that arrives as a string has to fail rather than compare equal.
func assertBodyEqual(t *testing.T, body, want map[string]any) {
	t.Helper()

	if got, wanted := marshalBody(t, body), marshalBody(t, want); !reflect.DeepEqual(got, wanted) {
		t.Errorf("Expected the body to be %v, got %v", wanted, got)
	}
}

func marshalBody(t *testing.T, body map[string]any) map[string]any {
	t.Helper()

	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Failed to marshal body: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal body: %v", err)
	}

	return decoded
}
