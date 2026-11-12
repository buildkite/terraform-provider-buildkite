package buildkite

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// This endpoint has its own tuples and editing gates, never an API backing string.
func (a *fakeOrganizationAPI) browserAPISettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && a.browserReadStatus != 0 {
		http.Error(w, `{"message":"browser settings unreadable"}`, a.browserReadStatus)
		return
	}
	if r.Method == http.MethodPatch {
		if a.browserPatchStatus != 0 && !a.browserApplyBeforeError {
			http.Error(w, `{"message":"browser update refused"}`, a.browserPatchStatus)
			return
		}
		if !a.browserSettings.Features.BrowserIpAllowList {
			http.Error(w, `{"message":"browser enrollment and entitlement required"}`, http.StatusForbidden)
			return
		}
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			a.t.Error(err)
			http.Error(w, `{"message":"invalid JSON"}`, http.StatusBadRequest)
			return
		}
		var entries []organizationIPEntry
		if len(body) != 1 || body["allowed_ip_entries"] == nil || string(body["allowed_ip_entries"]) == "null" {
			a.t.Errorf("expected only an explicit browser tuple array, got %s", body)
			http.Error(w, `{"message":"invalid payload"}`, http.StatusUnprocessableEntity)
			return
		}
		if err := json.Unmarshal(body["allowed_ip_entries"], &entries); err != nil {
			a.t.Error(err)
			http.Error(w, `{"message":"invalid tuples"}`, http.StatusUnprocessableEntity)
			return
		}
		for _, entry := range entries {
			if _, hostErr := netip.ParseAddr(entry.IPAddress); hostErr != nil {
				if _, cidrErr := netip.ParsePrefix(entry.IPAddress); cidrErr != nil {
					http.Error(w, `{"message":"invalid IP address"}`, http.StatusUnprocessableEntity)
					return
				}
			}
		}
		a.browserSettings.AllowedIpEntries = entries
		a.browserWrites = append(a.browserWrites, slices.Clone(entries))
		if a.browserApplyBeforeError {
			// A write landed but its response is unreadable. The provider must reconcile via GET.
			_, _ = w.Write([]byte(`{`))
			return
		}
	} else if r.Method != http.MethodGet {
		a.t.Errorf("unexpected browser method: %s", r.Method)
	}
	// REST returns [] for an absent policy, not null.
	if a.browserSettings.AllowedIpEntries == nil {
		a.browserSettings.AllowedIpEntries = []organizationIPEntry{}
	}
	if err := json.NewEncoder(w).Encode(a.browserSettings); err != nil {
		a.t.Error(err)
	}
}

func organizationBrowserEntrySteps(config func(string) string, checkRemote func([]organizationIPEntry) resource.TestCheckFunc) []resource.TestStep {
	const name = "buildkite_organization.settings"
	var steps []resource.TestStep
	for i, tc := range []struct {
		settings string
		entries  []organizationIPEntry
	}{
		{`allowed_browser_ip_entries = [
			{ ip_address = "192.0.2.1", description = "Office" },
			{ ip_address = "2001:db8::/48" },
			{ ip_address = "2001:db8::/48", description = "Second occurrence" }
		]`, []organizationIPEntry{{"192.0.2.1", "Office"}, {"2001:db8::/48", ""}, {"2001:db8::/48", "Second occurrence"}}},
		// Description-only replacement, including the Unicode length boundary.
		{fmt.Sprintf(`allowed_browser_ip_entries = [{ ip_address = "192.0.2.1", description = %q }]`, strings.Repeat("é", 255)), []organizationIPEntry{{"192.0.2.1", strings.Repeat("é", 255)}}},
		{`allowed_browser_ip_entries = []`, []organizationIPEntry{}},
		{`allowed_browser_ip_entries = [{ ip_address = "2001:db8::1" }]`, []organizationIPEntry{{"2001:db8::1", ""}}},
		{``, []organizationIPEntry{}}, // Removing a managed list explicitly clears it.
		{`allowed_browser_ip_entries = [{ ip_address = "198.51.100.0/24", description = "Destroy me" }]`, []organizationIPEntry{{"198.51.100.0/24", "Destroy me"}}},
	} {
		checks := []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(name, "allowed_api_ip_entries.0.description", "API only"),
			checkRemote(tc.entries),
		}
		if tc.settings == "" {
			checks = append(checks, resource.TestCheckNoResourceAttr(name, "allowed_browser_ip_entries"))
		} else {
			checks = append(checks, resource.TestCheckResourceAttr(name, "allowed_browser_ip_entries.#", fmt.Sprint(len(tc.entries))))
			for j, entry := range tc.entries {
				checks = append(checks,
					resource.TestCheckResourceAttr(name, fmt.Sprintf("allowed_browser_ip_entries.%d.ip_address", j), entry.IPAddress),
					resource.TestCheckResourceAttr(name, fmt.Sprintf("allowed_browser_ip_entries.%d.description", j), entry.Description))
			}
		}
		steps = append(steps, resource.TestStep{
			Config:           config(`allowed_api_ip_entries = [{ ip_address = "0.0.0.0/0", description = "API only" }]` + "\n" + tc.settings),
			Check:            resource.ComposeAggregateTestCheckFunc(checks...),
			ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
		})
		if i == 0 {
			steps = append(steps, resource.TestStep{
				ResourceName: name, ImportState: true, ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{"allowed_api_ip_entries", "allowed_api_ip_addresses"},
			})
			steps = append(steps, resource.TestStep{
				Config: config(`allowed_api_ip_entries = [{ ip_address = "0.0.0.0/0", description = "API only" }]
					allowed_browser_ip_entries = [{ ip_address = "198.51.100.1" }, { ip_address = "not-an-ip" }]`),
				ExpectError: regexp.MustCompile("422"),
			}, resource.TestStep{
				// The malformed second entry must not have persisted the valid first entry.
				Config: steps[0].Config, PlanOnly: true, Check: checkRemote(tc.entries),
			})
		}
	}
	return steps
}

func (a *fakeOrganizationAPI) checkBrowserEntries(want []organizationIPEntry) resource.TestCheckFunc {
	return func(*terraform.State) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		if !slices.Equal(a.browserSettings.AllowedIpEntries, want) {
			return fmt.Errorf("browser tuples = %v, want %v", a.browserSettings.AllowedIpEntries, want)
		}
		return nil
	}
}

func TestUnitBuildkiteOrganizationBrowserEntries(t *testing.T) {
	server, api := newFakeOrganizationAPI(t)
	api.browserSettings.Features.BrowserIpAllowList = true
	api.settings.Features.ApiIpAllowList = true
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             api.checkBrowserEntries(nil),
		Steps: organizationBrowserEntrySteps(func(settings string) string {
			return fakeOrganizationConfig(server, settings)
		}, func(want []organizationIPEntry) resource.TestCheckFunc {
			return resource.ComposeAggregateTestCheckFunc(api.checkBrowserEntries(want), func(*terraform.State) error {
				api.mu.Lock()
				defer api.mu.Unlock()
				if !slices.Equal(api.settings.AllowedIpEntries, []organizationIPEntry{{"0.0.0.0/0", "API only"}}) {
					return fmt.Errorf("browser edits changed the remote API policy: %v", api.settings.AllowedIpEntries)
				}
				return nil
			})
		}),
	})
}

func TestAccBuildkiteOrganizationBrowserEntries(t *testing.T) {
	if os.Getenv("BUILDKITE_BROWSER_IP_ENTRIES_ACCEPTANCE") != "1" {
		t.Skip("Set BUILDKITE_BROWSER_IP_ENTRIES_ACCEPTANCE=1 with a disposable enrolled organization and the browser-settings REST API; live browser acceptance is untested")
	}
	check := func(want []organizationIPEntry) resource.TestCheckFunc {
		return func(*terraform.State) error {
			settings, err := getTestClient().getOrganizationBrowserSettings(t.Context())
			if err != nil {
				return err
			}
			if !slices.Equal(settings.AllowedIpEntries, want) {
				return fmt.Errorf("browser tuples = %v, want %v", settings.AllowedIpEntries, want)
			}
			return nil
		}
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             check(nil),
		Steps: organizationBrowserEntrySteps(func(settings string) string {
			return fmt.Sprintf(`resource "buildkite_organization" "settings" { %s }`, settings)
		}, func(want []organizationIPEntry) resource.TestCheckFunc {
			return resource.ComposeAggregateTestCheckFunc(check(want), func(*terraform.State) error {
				settings, err := getTestClient().getOrganizationAPISettings(t.Context())
				if err != nil {
					return err
				}
				if !slices.Equal(settings.AllowedIpEntries, []organizationIPEntry{{"0.0.0.0/0", "API only"}}) {
					return fmt.Errorf("browser edits changed the remote API policy: %v", settings.AllowedIpEntries)
				}
				return nil
			})
		}),
	})
}

func TestUnitBuildkiteOrganizationBrowserUnmanagedPolicy(t *testing.T) {
	server, api := newFakeOrganizationAPI(t)
	policy := []organizationIPEntry{{"192.0.2.1", "Not managed by Terraform"}}
	api.browserSettings.AllowedIpEntries = policy
	// No endpoint is deployed/enrolled: unrelated configurations must still work.
	api.browserReadStatus = http.StatusNotFound
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             api.checkBrowserEntries(policy),
		Steps: []resource.TestStep{
			{Config: fakeOrganizationConfig(server, ``)},
			{ResourceName: "buildkite_organization.settings", ImportState: true, ImportStateVerify: true},
			{PreConfig: func() {
				api.mu.Lock()
				defer api.mu.Unlock()
				api.browserReadStatus = http.StatusForbidden
			}, ResourceName: "buildkite_organization.settings", ImportState: true, ImportStateVerify: true},
		},
	})
	if len(api.browserWrites) != 0 {
		t.Fatalf("unmanaged browser policy was written: %v", api.browserWrites)
	}
}

func TestUnitBuildkiteOrganizationBrowserValidation(t *testing.T) {
	server, _ := newFakeOrganizationAPI(t)
	for _, tc := range []struct {
		name, entries, error string
	}{
		{"null tuple", `[null]`, "Null List Value"},
		{"missing address", `[{ description = "Office" }]`, `attribute "ip_address" is required`},
		{"overlong Unicode description", fmt.Sprintf(`[{ ip_address = "192.0.2.1", description = %q }]`, strings.Repeat("é", 256)), `(?s)at most.*255`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				Steps: []resource.TestStep{{
					Config:   fakeOrganizationConfig(server, "allowed_browser_ip_entries = "+tc.entries),
					PlanOnly: true, ExpectError: regexp.MustCompile(tc.error),
				}},
			})
		})
	}
}

func TestUnitBuildkiteOrganizationBrowserFailureState(t *testing.T) {
	for _, ambiguous := range []bool{false, true} {
		t.Run(fmt.Sprintf("ambiguous=%t", ambiguous), func(t *testing.T) {
			server, api := newFakeOrganizationAPI(t)
			api.browserSettings.Features.BrowserIpAllowList = true
			initial := fakeOrganizationConfig(server, `allowed_browser_ip_entries = [{ ip_address = "192.0.2.1", description = "Office" }]`)
			changed := fakeOrganizationConfig(server, `allowed_browser_ip_entries = [{ ip_address = "192.0.2.1", description = "Renamed" }]`)
			observed := initial
			if ambiguous {
				observed = changed
			}
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				Steps: []resource.TestStep{
					{Config: initial},
					{PreConfig: func() {
						api.mu.Lock()
						defer api.mu.Unlock()
						api.browserPatchStatus = http.StatusUnprocessableEntity
						api.browserApplyBeforeError = ambiguous
					}, Config: changed, ExpectError: regexp.MustCompile("Unable to update organization browser settings")},
					{PreConfig: func() {
						api.mu.Lock()
						defer api.mu.Unlock()
						api.browserReadStatus = http.StatusForbidden
					}, Config: observed, PlanOnly: true},
					{PreConfig: func() {
						api.mu.Lock()
						defer api.mu.Unlock()
						api.browserReadStatus = 0
						api.browserPatchStatus = 0
						api.browserApplyBeforeError = false
					}, Config: changed},
				},
			})
		})
	}
}

func TestUnitBuildkiteOrganizationBrowserEditingUnavailable(t *testing.T) {
	server, api := newFakeOrganizationAPI(t)
	api.browserSettings.Features.BrowserIpAllowList = true
	initial := fakeOrganizationConfig(server, `allowed_browser_ip_entries = [{ ip_address = "192.0.2.1", description = "Office" }]`)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{Config: initial},
			{PreConfig: func() {
				api.mu.Lock()
				defer api.mu.Unlock()
				api.browserSettings.Features.BrowserIpAllowList = false
				// Prove refresh still reports drift after editing availability is lost.
				api.browserSettings.AllowedIpEntries[0].Description = "Changed remotely"
			}, Config: initial, PlanOnly: true, ExpectNonEmptyPlan: true},
			{Config: fakeOrganizationConfig(server, `allowed_browser_ip_entries = []`), ExpectError: regexp.MustCompile("enrollment and entitlement")},
			{Config: initial, PlanOnly: true, ExpectNonEmptyPlan: true,
				Check: resource.TestCheckResourceAttr("buildkite_organization.settings", "allowed_browser_ip_entries.0.description", "Changed remotely")},
			{PreConfig: func() {
				api.mu.Lock()
				defer api.mu.Unlock()
				api.browserSettings.Features.BrowserIpAllowList = true
			}, Config: initial},
		},
	})
}
