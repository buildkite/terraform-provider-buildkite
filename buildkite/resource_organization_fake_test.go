package buildkite

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// fakeOrganizationAPI serves the api-settings endpoint alongside the two GraphQL queries the
// organization resource and data source make for their identifiers, so that the lifecycle can be
// exercised without an organization to change. Its GraphQL side answers with identifiers only: the
// allowlist is api-settings' to report, and nothing here offers a second copy of it.
type fakeOrganizationAPI struct {
	t                 *testing.T
	mu                sync.Mutex
	settings          organizationAPISettings
	readStatus        int
	readBody          string
	patchStatus       int
	patchBody         string
	twoFactorEnforced bool
	twoFactorError    string
	writes            []string
}

func newFakeOrganizationAPI(t *testing.T) (*httptest.Server, *fakeOrganizationAPI) {
	t.Helper()

	api := &fakeOrganizationAPI{t: t}
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)

	return server, api
}

// refusePatch makes writes fail the way the API refuses a setting the organization's plan does not
// include, leaving reads working so the provider can still see which features are missing.
func (a *fakeOrganizationAPI) refusePatch(status int, body string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.patchStatus = status
	a.patchBody = body
}

// refuseRead makes api-settings fail the way it answers a caller who is not an organization
// administrator, which the GraphQL identifiers are still readable through.
func (a *fakeOrganizationAPI) refuseRead(status int, body string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.readStatus = status
	a.readBody = body
}

// allowRead undoes refuseRead, the way granting the missing scope would
func (a *fakeOrganizationAPI) allowRead() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.readStatus = 0
	a.readBody = ""
}

// presetAllowedIpAddresses gives the organization an allowlist terraform did not write, the way
// another administrator would
func (a *fakeOrganizationAPI) presetAllowedIpAddresses(allowed string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.settings.AllowedIpAddresses = allowed
}

func (a *fakeOrganizationAPI) allowedIpAddresses() string {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.settings.AllowedIpAddresses
}

// checkAllowedIpAddresses asserts what the allowlist ended up as on the organization, which the
// attribute in state does not answer for on its own.
func (a *fakeOrganizationAPI) checkAllowedIpAddresses(want string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if got := a.allowedIpAddresses(); got != want {
			return fmt.Errorf("remote allowed_ip_addresses = %q, want %q", got, want)
		}
		return nil
	}
}

// enforceTwoFactor gives the organization the 2FA setting it starts with, which enforce_2fa defaults
// to false against
func (a *fakeOrganizationAPI) enforceTwoFactor(enforced bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.twoFactorEnforced = enforced
}

// refuseTwoFactorChange makes the 2FA mutation fail while the REST settings keep answering
func (a *fakeOrganizationAPI) refuseTwoFactorChange(message string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.twoFactorError = message
}

func (a *fakeOrganizationAPI) twoFactorState() bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.twoFactorEnforced
}

// allowlistWrites reports every value the allowlist was written with, which the value it ended up
// with does not distinguish a write that was undone from one that never happened
func (a *fakeOrganizationAPI) allowlistWrites() []string {
	a.mu.Lock()
	defer a.mu.Unlock()

	return slices.Clone(a.writes)
}

func (a *fakeOrganizationAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// httptest serves requests concurrently, while this fake models one organization-wide resource.
	a.mu.Lock()
	defer a.mu.Unlock()

	switch r.URL.Path {
	case "/graphql":
		a.graphql(w, r)
	case "/v2/organizations/acme/api-settings":
		a.apiSettings(w, r)
	default:
		a.t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	}
}

func (a *fakeOrganizationAPI) graphql(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Query     string `json:"query"`
		Variables struct {
			Value bool `json:"value"`
		} `json:"variables"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		a.t.Errorf("unable to read the GraphQL request: %v", err)
		return
	}

	// the organization id is fetched with an anonymous query, everything else with getOrganization
	response := `{"data":{"organization":{"id":"organization-graphql-id"}}}`
	switch {
	case strings.Contains(body.Query, "getOrganization"):
		response = fmt.Sprintf(`{"data":{"organization":{`+
			`"id":"organization-graphql-id",`+
			`"uuid":"5e0e3b0a-1111-2222-3333-444455556666",`+
			`"membersRequireTwoFactorAuthentication":%t}}}`, a.twoFactorEnforced)

	case strings.Contains(body.Query, "setOrganization2FA"):
		if a.twoFactorError != "" {
			response = fmt.Sprintf(`{"errors":[{"message":%q}]}`, a.twoFactorError)
			break
		}
		a.twoFactorEnforced = body.Variables.Value
		response = fmt.Sprintf(`{"data":{"organizationEnforceTwoFactorAuthenticationForMembersUpdate":`+
			`{"organization":{"membersRequireTwoFactorAuthentication":%t}}}}`, a.twoFactorEnforced)
	}

	if _, err := io.WriteString(w, response); err != nil {
		a.t.Errorf("unable to write the response: %v", err)
	}
}

func (a *fakeOrganizationAPI) apiSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if a.readStatus != 0 {
			http.Error(w, a.readBody, a.readStatus)
			return
		}

	case http.MethodPatch:
		if a.patchStatus != 0 {
			http.Error(w, a.patchBody, a.patchStatus)
			return
		}
		if !a.patch(w, r) {
			return
		}

	default:
		a.t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
		return
	}

	if err := json.NewEncoder(w).Encode(a.settings); err != nil {
		a.t.Errorf("unable to write the response: %v", err)
	}
}

func (a *fakeOrganizationAPI) patch(w http.ResponseWriter, r *http.Request) bool {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		a.t.Errorf("unable to read the request: %v", err)
		http.Error(w, `{"message":"invalid JSON"}`, http.StatusBadRequest)
		return false
	}

	// Only the keys the request carries change, which is what lets the resource leave the settings
	// it does not manage alone.
	for key, value := range body {
		switch key {
		case "allowed_ip_addresses":
			allowed, ok := value.(string)
			if !ok {
				a.t.Errorf("allowed_ip_addresses is %T, want a string", value)
			}
			a.settings.AllowedIpAddresses = allowed
			a.writes = append(a.writes, allowed)
		case "restrict_user_api_token_creation":
			restrict, ok := value.(bool)
			if !ok {
				a.t.Errorf("restrict_user_api_token_creation is %T, want a bool", value)
			}
			a.settings.RestrictUserApiTokenCreation = restrict
		case "revoke_inactive_tokens_after_days":
			a.settings.RevokeInactiveTokensAfterDays = optionalInt64FromJSON(value)
		default:
			a.t.Errorf("unexpected key in the request: %s", key)
		}
	}
	return true
}

func fakeOrganizationConfig(server *httptest.Server, settings string) string {
	return fmt.Sprintf(`
	provider "buildkite" {
		organization = "acme"
		api_token    = "fake"
		rest_url     = %q
		graphql_url  = %q
	}

	resource "buildkite_organization" "settings" {
		%s
	}
	`, server.URL, server.URL+"/graphql", settings)
}

func fakeOrganizationDatasourceConfig(server *httptest.Server) string {
	return fmt.Sprintf(`
	provider "buildkite" {
		organization = "acme"
		api_token    = "fake"
		rest_url     = %q
		graphql_url  = %q
	}

	data "buildkite_organization" "settings" {}
	`, server.URL, server.URL+"/graphql")
}

func TestUnitBuildkiteOrganizationAllowlistAgainstFakeAPI(t *testing.T) {
	server, api := newFakeOrganizationAPI(t)
	api.settings.Features.ApiIpAllowList = true

	const name = "buildkite_organization.settings"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fakeOrganizationConfig(server, `allowed_api_ip_addresses = ["1.1.1.1/32", "0.0.0.0/0"]`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "allowed_api_ip_addresses.0", "1.1.1.1/32"),
					resource.TestCheckResourceAttr(name, "allowed_api_ip_addresses.1", "0.0.0.0/0"),
					// the order the attribute lists them in is the order the API is given
					api.checkAllowedIpAddresses("1.1.1.1/32 0.0.0.0/0"),
				),
			},
			{
				ResourceName:      name,
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				// an empty list clears the allowlist and is kept as an empty list rather than read
				// back as the null the API answers with
				Config: fakeOrganizationConfig(server, `allowed_api_ip_addresses = []`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "allowed_api_ip_addresses.#", "0"),
					api.checkAllowedIpAddresses(""),
				),
			},
			{
				Config: fakeOrganizationConfig(server, `allowed_api_ip_addresses = ["4.4.4.4/32"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "allowed_api_ip_addresses.0", "4.4.4.4/32"),
					api.checkAllowedIpAddresses("4.4.4.4/32"),
				),
			},
			{
				// dropping the attribute clears the allowlist, unlike the settings the resource only
				// adopts, and leaves the attribute unset rather than reading "" back as [""]
				Config: fakeOrganizationConfig(server, ``),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(name, "allowed_api_ip_addresses"),
					api.checkAllowedIpAddresses(""),
				),
			},
		},
	})
}

// A configuration that says nothing about the allowlist must not write one: an organization without
// the feature is refused even the empty value it already has.
func TestUnitBuildkiteOrganizationLeavesAnUnconfiguredAllowlistAlone(t *testing.T) {
	server, api := newFakeOrganizationAPI(t)
	api.refusePatch(http.StatusForbidden, `{"message":"Your plan does not include the API IP allow list feature"}`)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fakeOrganizationConfig(server, ``),
				Check:  api.checkAllowedIpAddresses(""),
			},
		},
	})
}

// The API refuses a plan-gated setting with a 403 carrying its own explanation, which the provider
// reports alongside the feature the organization's plan is missing.
func TestUnitBuildkiteOrganizationReportsAPlanGatedAllowlist(t *testing.T) {
	server, api := newFakeOrganizationAPI(t)
	api.refusePatch(http.StatusForbidden, `{"message":"Your plan does not include the API IP allow list feature"}`)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      fakeOrganizationConfig(server, `allowed_api_ip_addresses = ["1.1.1.1/32"]`),
				ExpectError: regexp.MustCompile(`(?s)Your plan does not include the API IP allow list feature.*The allowed API IP addresses feature is not available on this organization's plan`),
			},
		},
	})
}

// An allowlist the provider cannot read is one it cannot compare the configuration with either, and
// an empty configuration serializes to the same "" a fallback would read it as, so a create that
// guessed would leave the allowlist in place while reporting it gone.
func TestUnitBuildkiteOrganizationRefusesToWriteUnreadableSettings(t *testing.T) {
	server, api := newFakeOrganizationAPI(t)
	api.settings.AllowedIpAddresses = "1.1.1.1/32"
	api.refuseRead(http.StatusForbidden, `{"message":"Forbidden"}`)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      fakeOrganizationConfig(server, `allowed_api_ip_addresses = []`),
				ExpectError: regexp.MustCompile(`(?s)Unable to read organization API settings.*The API token needs the read_organization_settings scope`),
			},
		},
	})
}

// Destroying clears the allowlist the resource owns, and state does not say whether there is one to
// clear: a refresh that could not read the settings keeps reporting the allowlist it last saw, so an
// allowlist set out of band would outlive the resource that is supposed to own it.
func TestUnitBuildkiteOrganizationRefusesToDestroyWithUnreadableSettings(t *testing.T) {
	server, api := newFakeOrganizationAPI(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             api.checkAllowedIpAddresses(""),
		Steps: []resource.TestStep{
			{
				// nothing says anything about an allowlist, so state records none
				Config: fakeOrganizationConfig(server, ``),
				Check:  api.checkAllowedIpAddresses(""),
			},
			{
				PreConfig: func() {
					api.presetAllowedIpAddresses("1.1.1.1/32")
					api.refuseRead(http.StatusForbidden, `{"message":"Forbidden"}`)
				},
				Config:      fakeOrganizationConfig(server, ``),
				Destroy:     true,
				ExpectError: regexp.MustCompile(`(?s)Unable to read organization API settings.*The API token needs the read_organization_settings scope`),
			},
			{
				// with the settings readable the allowlist state never saw is still cleared
				PreConfig: api.allowRead,
				Config:    fakeOrganizationConfig(server, ``),
				Destroy:   true,
			},
		},
	})
}

// The settings the organization's plan can refuse are written before 2FA is touched. enforce_2fa
// defaults to false, so a create that took the refusal second would turn 2FA off on the way to an
// apply that fails, and record no state to notice it from.
func TestUnitBuildkiteOrganizationKeeps2FAWhenTheAllowlistIsRefused(t *testing.T) {
	server, api := newFakeOrganizationAPI(t)
	api.enforceTwoFactor(true)
	api.refusePatch(http.StatusForbidden, `{"message":"Your plan does not include the API IP allow list feature"}`)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      fakeOrganizationConfig(server, `allowed_api_ip_addresses = ["1.1.1.1/32"]`),
				ExpectError: regexp.MustCompile(`The allowed API IP addresses feature is not available on this organization's plan`),
			},
		},
	})

	// nothing recorded the create, so an organization left without 2FA has no state to notice it by
	if !api.twoFactorState() {
		t.Error("2FA was turned off by a create that went on to fail")
	}
}

// A 2FA change that fails once the settings have been written still has to record them: the
// allowlist is the resource's to clear, and only a resource that reached state is ever destroyed.
func TestUnitBuildkiteOrganizationRecordsSettingsWhenSetting2FAFails(t *testing.T) {
	server, api := newFakeOrganizationAPI(t)
	api.settings.Features.ApiIpAllowList = true
	api.enforceTwoFactor(true)
	api.refuseTwoFactorChange("Unable to enforce two-factor authentication")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      fakeOrganizationConfig(server, `allowed_api_ip_addresses = ["1.1.1.1/32"]`),
				ExpectError: regexp.MustCompile(`Unable to set 2FA`),
			},
		},
	})

	// the allowlist the failed create wrote is cleared again by the destroy the test ends with,
	// which only reaches an organization the failure recorded state for
	if want := []string{"1.1.1.1/32", ""}; !slices.Equal(api.allowlistWrites(), want) {
		t.Errorf("allowlist writes = %q, want %q", api.allowlistWrites(), want)
	}
}

func TestUnitBuildkiteOrganizationDatasourceAgainstFakeAPI(t *testing.T) {
	server, api := newFakeOrganizationAPI(t)
	api.settings.AllowedIpAddresses = "1.1.1.1/32 0.0.0.0/0"

	const name = "data.buildkite_organization.settings"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fakeOrganizationDatasourceConfig(server),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "id", "organization-graphql-id"),
					resource.TestCheckResourceAttr(name, "uuid", "5e0e3b0a-1111-2222-3333-444455556666"),
					resource.TestCheckResourceAttr(name, "allowed_api_ip_addresses.0", "1.1.1.1/32"),
					resource.TestCheckResourceAttr(name, "allowed_api_ip_addresses.1", "0.0.0.0/0"),
				),
			},
		},
	})
}

// api-settings only answers an organization administrator, so a lookup that cannot see the allowlist
// still reports the identifiers rather than failing outright.
func TestUnitBuildkiteOrganizationDatasourceWithoutSettingsAccess(t *testing.T) {
	server, api := newFakeOrganizationAPI(t)
	api.settings.AllowedIpAddresses = "1.1.1.1/32"
	api.refuseRead(http.StatusForbidden, `{"message":"Forbidden"}`)

	const name = "data.buildkite_organization.settings"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fakeOrganizationDatasourceConfig(server),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "id", "organization-graphql-id"),
					resource.TestCheckNoResourceAttr(name, "allowed_api_ip_addresses"),
				),
			},
		},
	})
}
