package buildkite

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const testAPISettingsPath = "/v2/organizations/test-org/api-settings"

func testAPISettingsClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return &Client{
		http:         server.Client(),
		restURL:      server.URL,
		organization: "test-org",
	}
}

func cidrList(cidrs ...string) types.List {
	elements := make([]attr.Value, 0, len(cidrs))
	for _, cidr := range cidrs {
		elements = append(elements, types.StringValue(cidr))
	}

	return types.ListValueMust(types.StringType, elements)
}

func TestGetOrganizationAPISettings(t *testing.T) {
	t.Parallel()

	client := testAPISettingsClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET request, got %s", r.Method)
		}
		if r.URL.Path != testAPISettingsPath {
			t.Errorf("Expected path %s, got %s", testAPISettingsPath, r.URL.Path)
		}

		if _, err := w.Write([]byte(`{
			"url": "https://api.buildkite.com/v2/organizations/test-org/api-settings",
			"allowed_ip_addresses": "10.0.0.0/8 192.168.0.0/16",
			"revoke_inactive_tokens_after_days": 90,
			"restrict_user_api_token_creation": true,
			"features": { "api_ip_allow_list": true, "inactive_api_token_revocation": false }
		}`)); err != nil {
			t.Fatalf("Failed to write response: %v", err)
		}
	})

	settings, err := client.GetOrganizationAPISettings(context.Background(), "test-org")
	if err != nil {
		t.Fatalf("GetOrganizationAPISettings failed: %v", err)
	}

	if settings.AllowedIPAddresses == nil || *settings.AllowedIPAddresses != "10.0.0.0/8 192.168.0.0/16" {
		t.Errorf("Expected the allowlist to round-trip, got %v", settings.AllowedIPAddresses)
	}
	if settings.RevokeInactiveTokensAfterDays == nil || *settings.RevokeInactiveTokensAfterDays != 90 {
		t.Errorf("Expected 90 days, got %v", settings.RevokeInactiveTokensAfterDays)
	}
	if !settings.RestrictUserAPITokenCreation {
		t.Error("Expected restrict_user_api_token_creation to be true")
	}
	if !settings.Features.APIIPAllowList {
		t.Error("Expected the api_ip_allow_list feature to be available")
	}
	if settings.Features.InactiveAPITokenRevocation {
		t.Error("Expected the inactive_api_token_revocation feature to be unavailable")
	}
}

// The API returns null for a setting that is off, which has to stay distinguishable from a setting
// that is on and happens to be empty.
func TestGetOrganizationAPISettingsWithSettingsOff(t *testing.T) {
	t.Parallel()

	client := testAPISettingsClient(t, func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte(`{
			"allowed_ip_addresses": null,
			"revoke_inactive_tokens_after_days": null,
			"restrict_user_api_token_creation": false,
			"features": { "api_ip_allow_list": false, "inactive_api_token_revocation": false }
		}`)); err != nil {
			t.Fatalf("Failed to write response: %v", err)
		}
	})

	settings, err := client.GetOrganizationAPISettings(context.Background(), "test-org")
	if err != nil {
		t.Fatalf("GetOrganizationAPISettings failed: %v", err)
	}

	if settings.AllowedIPAddresses != nil {
		t.Errorf("Expected a nil allowlist, got %q", *settings.AllowedIPAddresses)
	}
	if settings.RevokeInactiveTokensAfterDays != nil {
		t.Errorf("Expected a nil revocation interval, got %d", *settings.RevokeInactiveTokensAfterDays)
	}
}

// Clearing the allowlist through the API stores a null, but the column is a plain nullable string
// with no normalisation behind it, so an allowlist emptied by some other route can read back as "".
// Either way it has to leave the attribute alone rather than reading back as a single empty CIDR,
// while a value the API does report has to replace state so drift is visible.
func TestReadOrganizationAPISettingsAllowlistMapping(t *testing.T) {
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

func TestUpdateOrganizationAPISettings(t *testing.T) {
	t.Parallel()

	var received map[string]any

	client := testAPISettingsClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("Expected PATCH request, got %s", r.Method)
		}
		if r.URL.Path != testAPISettingsPath {
			t.Errorf("Expected path %s, got %s", testAPISettingsPath, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}

		if _, err := w.Write([]byte(`{
			"allowed_ip_addresses": "10.0.0.0/8",
			"revoke_inactive_tokens_after_days": 30,
			"restrict_user_api_token_creation": true,
			"features": { "api_ip_allow_list": true, "inactive_api_token_revocation": true }
		}`)); err != nil {
			t.Fatalf("Failed to write response: %v", err)
		}
	})

	settings, err := client.UpdateOrganizationAPISettings(context.Background(), "test-org", map[string]any{
		"allowed_ip_addresses":              "10.0.0.0/8",
		"revoke_inactive_tokens_after_days": int64(30),
		"restrict_user_api_token_creation":  true,
	})
	if err != nil {
		t.Fatalf("UpdateOrganizationAPISettings failed: %v", err)
	}

	if received["allowed_ip_addresses"] != "10.0.0.0/8" {
		t.Errorf("Expected the allowlist to be sent as a space separated string, got %v", received["allowed_ip_addresses"])
	}
	if settings.RevokeInactiveTokensAfterDays == nil || *settings.RevokeInactiveTokensAfterDays != 30 {
		t.Errorf("Expected the response to be returned, got %v", settings.RevokeInactiveTokensAfterDays)
	}
}

// A refusal on billing grounds is the one failure whose wording the user needs, so it has to reach
// them rather than being flattened into a status code.
func TestUpdateOrganizationAPISettingsReportsPlanRefusal(t *testing.T) {
	t.Parallel()

	client := testAPISettingsClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		if _, err := w.Write([]byte(`{"message":"Your billing plan doesn't support the API IP allowlist"}`)); err != nil {
			t.Fatalf("Failed to write response: %v", err)
		}
	})

	_, err := client.UpdateOrganizationAPISettings(context.Background(), "test-org", map[string]any{
		"allowed_ip_addresses": "10.0.0.0/8",
	})
	if err == nil {
		t.Fatal("Expected an error for a refused setting")
	}
	if !isAPIStatus(err, http.StatusForbidden) {
		t.Errorf("Expected a 403, got %s", err)
	}
	if got := apiErrorMessage(err); got != "Your billing plan doesn't support the API IP allowlist" {
		t.Errorf("Expected the API's explanation to survive, got %q", got)
	}
}

func TestOrganizationAPISettingsPatchBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		plan  organizationResourceModel
		state *organizationResourceModel
		want  map[string]any
	}{
		{
			// The gated keys have to be absent rather than null: the API refuses them on presence
			// alone, so naming them would lock a plan without them out of the ungated setting too.
			name: "create with only the ungated setting names nothing else",
			plan: organizationResourceModel{
				AllowedApiIpAddresses:         types.ListNull(types.StringType),
				RevokeInactiveTokensAfterDays: types.Int64Null(),
				RestrictUserApiTokenCreation:  types.BoolValue(true),
			},
			want: map[string]any{"restrict_user_api_token_creation": true},
		},
		{
			name: "create sends every setting the plan sets",
			plan: organizationResourceModel{
				AllowedApiIpAddresses:         cidrList("10.0.0.0/8", "192.168.0.0/16"),
				RevokeInactiveTokensAfterDays: types.Int64Value(90),
				RestrictUserApiTokenCreation:  types.BoolValue(false),
			},
			want: map[string]any{
				"allowed_ip_addresses":              "10.0.0.0/8 192.168.0.0/16",
				"revoke_inactive_tokens_after_days": int64(90),
				"restrict_user_api_token_creation":  false,
			},
		},
		{
			name: "dropping an attribute clears it with an explicit null",
			plan: organizationResourceModel{
				AllowedApiIpAddresses:         types.ListNull(types.StringType),
				RevokeInactiveTokensAfterDays: types.Int64Null(),
				RestrictUserApiTokenCreation:  types.BoolValue(false),
			},
			state: &organizationResourceModel{
				AllowedApiIpAddresses:         cidrList("10.0.0.0/8"),
				RevokeInactiveTokensAfterDays: types.Int64Value(90),
				RestrictUserApiTokenCreation:  types.BoolValue(false),
			},
			want: map[string]any{
				"allowed_ip_addresses":              nil,
				"revoke_inactive_tokens_after_days": nil,
				"restrict_user_api_token_creation":  false,
			},
		},
		{
			// An empty list says "no restrictions", which the API spells as null, not "".
			name: "an empty allowlist clears rather than sending an empty string",
			plan: organizationResourceModel{
				AllowedApiIpAddresses:         cidrList(""),
				RevokeInactiveTokensAfterDays: types.Int64Null(),
				RestrictUserApiTokenCreation:  types.BoolValue(false),
			},
			want: map[string]any{
				"allowed_ip_addresses":             nil,
				"restrict_user_api_token_creation": false,
			},
		},
		{
			// Nothing was set and nothing is being set, so the request stays silent even though state
			// carries the attribute.
			name: "an untouched gated setting stays out of the body",
			plan: organizationResourceModel{
				AllowedApiIpAddresses:         types.ListNull(types.StringType),
				RevokeInactiveTokensAfterDays: types.Int64Null(),
				RestrictUserApiTokenCreation:  types.BoolValue(true),
			},
			state: &organizationResourceModel{
				AllowedApiIpAddresses:         types.ListNull(types.StringType),
				RevokeInactiveTokensAfterDays: types.Int64Null(),
				RestrictUserApiTokenCreation:  types.BoolValue(false),
			},
			want: map[string]any{"restrict_user_api_token_creation": true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assertBodyEqual(t, organizationAPISettingsPatchBody(&tt.plan, tt.state), tt.want)
		})
	}
}

func TestOrganizationAPISettingsResetBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		state organizationResourceModel
		want  map[string]any
	}{
		{
			name: "settings this resource never took over are left alone",
			state: organizationResourceModel{
				AllowedApiIpAddresses:         types.ListNull(types.StringType),
				RevokeInactiveTokensAfterDays: types.Int64Null(),
				RestrictUserApiTokenCreation:  types.BoolValue(false),
			},
			want: nil,
		},
		{
			name: "each managed setting is returned to its default",
			state: organizationResourceModel{
				AllowedApiIpAddresses:         cidrList("10.0.0.0/8"),
				RevokeInactiveTokensAfterDays: types.Int64Value(90),
				RestrictUserApiTokenCreation:  types.BoolValue(true),
			},
			want: map[string]any{
				"allowed_ip_addresses":              nil,
				"revoke_inactive_tokens_after_days": nil,
				"restrict_user_api_token_creation":  false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			body := organizationAPISettingsResetBody(&tt.state)
			if tt.want == nil {
				if body != nil {
					t.Errorf("Expected no request at all, got %v", body)
				}
				return
			}

			assertBodyEqual(t, body, tt.want)
		})
	}
}

// assertBodyEqual compares the body as the API would see it. It rounds through JSON first, because
// an omitted key and one holding a typed nil pointer look the same in a map and different on the
// wire, and that difference is the whole point of these cases.
func assertBodyEqual(t *testing.T, body, want map[string]any) {
	t.Helper()

	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Failed to marshal body: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("Failed to unmarshal body: %v", err)
	}

	for key, wantValue := range want {
		gotValue, ok := got[key]
		if !ok {
			t.Errorf("Expected the body to carry %q", key)
			continue
		}
		// JSON numbers decode as float64, so compare the rendered forms rather than the types.
		if fmt.Sprint(gotValue) != fmt.Sprint(wantValue) {
			t.Errorf("Expected %q to be %v, got %v", key, wantValue, gotValue)
		}
	}

	for key, gotValue := range got {
		if _, ok := want[key]; !ok {
			t.Errorf("Expected the body to stay silent about %q, got %v", key, gotValue)
		}
	}
}
