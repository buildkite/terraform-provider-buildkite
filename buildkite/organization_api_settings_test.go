package buildkite

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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

func TestUpdateOrganizationAPISettings(t *testing.T) {
	t.Parallel()

	var received map[string]any
	var contentType string

	client := testAPISettingsClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("Expected PATCH request, got %s", r.Method)
		}
		if r.URL.Path != testAPISettingsPath {
			t.Errorf("Expected path %s, got %s", testAPISettingsPath, r.URL.Path)
		}
		contentType = r.Header.Get("Content-Type")
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
	// The endpoint documents the header, and a body sent without one is at the mercy of whatever the
	// server decides to parse it as.
	if contentType != "application/json" {
		t.Errorf("Expected a JSON content type, got %q", contentType)
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
