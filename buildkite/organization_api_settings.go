package buildkite

import (
	"context"
	"fmt"
	"net/http"
)

// CONTRIBUTING would ordinarily prefer GraphQL, and two of these three settings have a mutation
// there, but restrict_user_api_token_creation has no GraphQL surface at all. Writing all three here
// keeps one organization's API settings from being split across two APIs.

// organizationAPISettings mirrors the /v2/organizations/{slug}/api-settings response.
// allowed_ip_addresses is a space separated list of CIDR ranges, and both it and
// revoke_inactive_tokens_after_days are null when the setting is off.
type organizationAPISettings struct {
	AllowedIPAddresses            *string                         `json:"allowed_ip_addresses"`
	RevokeInactiveTokensAfterDays *int64                          `json:"revoke_inactive_tokens_after_days"`
	RestrictUserAPITokenCreation  bool                            `json:"restrict_user_api_token_creation"`
	Features                      organizationAPISettingsFeatures `json:"features"`
}

// organizationAPISettingsFeatures reports which settings the organization's billing plan entitles it
// to. The API refuses a gated key on its presence alone, so this is what tells a setting that is
// switched off apart from one the organization cannot write at all.
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

// UpdateOrganizationAPISettings patches the settings named in body and returns the full settings
// afterwards. body is a map rather than a struct because omitting a key and sending it as null mean
// different things here, which struct tags can't express.
func (c *Client) UpdateOrganizationAPISettings(ctx context.Context, orgSlug string, body map[string]any) (*organizationAPISettings, error) {
	var settings organizationAPISettings
	if err := c.makeRequest(ctx, http.MethodPatch, organizationAPISettingsPath(orgSlug), body, &settings); err != nil {
		return nil, err
	}

	return &settings, nil
}
