package buildkite

import (
	"context"
	"fmt"
	"net/http"
)

// organizationAPISettings mirrors the /v2/organizations/{slug}/api-settings response.
// allowed_ip_addresses is a space separated list of CIDR ranges, and both it and
// revoke_inactive_tokens_after_days are null when the setting is off.
type organizationAPISettings struct {
	URL                           string                          `json:"url"`
	AllowedIPAddresses            *string                         `json:"allowed_ip_addresses"`
	RevokeInactiveTokensAfterDays *int64                          `json:"revoke_inactive_tokens_after_days"`
	RestrictUserAPITokenCreation  bool                            `json:"restrict_user_api_token_creation"`
	Features                      organizationAPISettingsFeatures `json:"features"`
}

// organizationAPISettingsFeatures reports which settings the organization's billing plan
// entitles it to. Writing a setting the plan doesn't cover is a 403.
type organizationAPISettingsFeatures struct {
	APIIPAllowList             bool `json:"api_ip_allow_list"`
	InactiveAPITokenRevocation bool `json:"inactive_api_token_revocation"`
}

func organizationAPISettingsPath(orgSlug string) string {
	return fmt.Sprintf("/v2/organizations/%s/api-settings", orgSlug)
}

// GetOrganizationAPISettings reads the organization's API settings.
func (c *Client) GetOrganizationAPISettings(ctx context.Context, orgSlug string) (*organizationAPISettings, error) {
	var settings organizationAPISettings
	if err := c.makeRequest(ctx, http.MethodGet, organizationAPISettingsPath(orgSlug), nil, &settings); err != nil {
		return nil, err
	}

	return &settings, nil
}

// UpdateOrganizationAPISettings patches the settings named in body and returns the full
// settings afterwards.
//
// body is a map rather than a struct because the API gates plan-restricted settings on key
// presence, not on value: sending "allowed_ip_addresses": null to an organization whose plan
// lacks the IP allowlist is a 403 even though it changes nothing. Omitting a key and sending
// it as null therefore mean different things, which struct tags can't express.
func (c *Client) UpdateOrganizationAPISettings(ctx context.Context, orgSlug string, body map[string]any) (*organizationAPISettings, error) {
	var settings organizationAPISettings
	if err := c.makeRequest(ctx, http.MethodPatch, organizationAPISettingsPath(orgSlug), body, &settings); err != nil {
		return nil, err
	}

	return &settings, nil
}
