package buildkite

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type organizationResourceModel struct {
	AllowedApiIpAddresses        types.List   `tfsdk:"allowed_api_ip_addresses"`
	ID                           types.String `tfsdk:"id"`
	UUID                         types.String `tfsdk:"uuid"`
	Enforce2FA                   types.Bool   `tfsdk:"enforce_2fa"`
	RevokeInactiveTokensAfter    types.String `tfsdk:"revoke_inactive_tokens_after"`
	RestrictUserApiTokenCreation types.Bool   `tfsdk:"restrict_user_api_token_creation"`
}

type organizationResource struct {
	client *Client
}

func newOrganizationResource() resource.Resource {
	return &organizationResource{}
}

func (organizationResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization"
}

func (o *organizationResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	o.client = req.ProviderData.(*Client)
}

func (*organizationResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: heredoc.Doc(`
			This resource allows you to manage the settings for an organization.

			The user of your API token must be an organization administrator to manage organization settings.
		`),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The GraphQL ID of the organization.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"uuid": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the organization.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"allowed_api_ip_addresses": schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				MarkdownDescription: "A list of IP addresses in CIDR format that are allowed to access the Buildkite API." +
					"If not set, all IP addresses are allowed (the same as setting 0.0.0.0/0).\n\n" +
					"-> The \"Allowed API IP Addresses\" feature must be enabled on your organization in order to manage the `allowed_api_ip_addresses` attribute.",
			},
			"enforce_2fa": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Sets whether the organization requires two-factor authentication for all members.",
			},
			"revoke_inactive_tokens_after": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "The period of inactivity after which user API access tokens are revoked. Valid values are `NEVER`, `DAYS_30`, `DAYS_60`, `DAYS_90`, `DAYS_180` and `DAYS_365`. " +
					"If omitted, the current setting is left unchanged. Requires an API token with the `read_organization_settings` and `write_organization_settings` scopes and the inactive API token revocation feature on the organization's plan.",
				Validators: []validator.String{
					stringvalidator.OneOf(revokeInactiveTokenPeriods...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"restrict_user_api_token_creation": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Whether only organization administrators can create new API access tokens for this organization. " +
					"If omitted, the current setting is left unchanged. Requires an API token with the `read_organization_settings` and `write_organization_settings` scopes.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (o *organizationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var config, plan, state organizationResourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	org, err := o.client.GetOrganizationID()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to find organization",
			fmt.Sprintf("Unable to find Organization: %s", err.Error()),
		)
		return
	}
	organization, err := getOrganization(ctx, o.client.genqlient, o.client.organization)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to obtain Organization",
			fmt.Sprintf("Unable to obtain Organization: %s", err.Error()),
		)
		return
	}

	log.Printf("Creating settings for organization %s ...", *org)
	// compare with the organization's current allowlist so a matching one is left as it is
	current, diags := allowedApiIpAddressesFromAPI(ctx, organization.Organization.AllowedApiIpAddresses, plan.AllowedApiIpAddresses)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	// What this apply has already changed on the organization. Create records no state when a later
	// step fails, so anything collected here has to be reported instead. Reported in a defer so a
	// new early return cannot drop the warning, which is the only record these changes applied.
	var applied []string
	defer func() {
		if resp.Diagnostics.HasError() {
			warnAboutUnrecordedChanges(applied, &resp.Diagnostics)
		}
	}()

	plannedAllowlist := allowedApiIpAddressesValue(plan.AllowedApiIpAddresses)
	allowlistChanged, err := o.updateAllowedApiIpAddresses(ctx, *org, plan.AllowedApiIpAddresses, current)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create Organization settings",
			fmt.Sprintf("Unable to create Organization settings: %s", err.Error()),
		)
		return
	}
	if allowlistChanged {
		change := fmt.Sprintf("the API IP allowlist was set to %q", plannedAllowlist)
		if plannedAllowlist == "" {
			change = "the API IP allowlist was cleared"
		}
		applied = append(applied, change)
	}

	if !plan.Enforce2FA.IsNull() && !plan.Enforce2FA.IsUnknown() && plan.Enforce2FA.ValueBool() != organization.Organization.MembersRequireTwoFactorAuthentication {
		_, err = setOrganization2FA(ctx, o.client.genqlient, *org, plan.Enforce2FA.ValueBool())
		if err != nil {
			resp.Diagnostics.AddError("Unable to set 2FA", err.Error())
			return
		}
		change := "two-factor authentication enforcement was removed"
		if plan.Enforce2FA.ValueBool() {
			change = "two-factor authentication was enforced for all members"
		}
		applied = append(applied, change)
	}

	state.ID = types.StringValue(*org)
	state.UUID = types.StringValue(organization.Organization.Uuid)
	state.Enforce2FA = plan.Enforce2FA
	state.AllowedApiIpAddresses = plan.AllowedApiIpAddresses

	o.updateAPISettings(ctx, &config, &plan, nil, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (o *organizationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state organizationResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("Reading settings for organization ...")
	response, err := getOrganization(ctx, o.client.genqlient, o.client.organization)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to obtain Organization",
			fmt.Sprintf("Unable to obtain Organization: %s", err.Error()),
		)
		return
	}

	org, err := o.client.GetOrganizationID()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to find organization",
			fmt.Sprintf("Unable to find Organization: %s", err.Error()),
		)
		return
	}
	state.ID = types.StringValue(*org)
	state.UUID = types.StringValue(response.Organization.Uuid)
	state.Enforce2FA = types.BoolValue(response.Organization.MembersRequireTwoFactorAuthentication)

	ips, diag := allowedApiIpAddressesFromAPI(ctx, response.Organization.AllowedApiIpAddresses, state.AllowedApiIpAddresses)
	if diag.HasError() {
		resp.Diagnostics.Append(diag...)
		return
	}
	state.AllowedApiIpAddresses = ips

	o.readAPISettings(ctx, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// allowedApiIpAddressesFromAPI maps the space separated allowlist from the API onto allowed_api_ip_addresses
func allowedApiIpAddressesFromAPI(ctx context.Context, remote string, current types.List) (types.List, diag.Diagnostics) {
	// keep an unset (or empty) attribute as is instead of reading "" back as [""]
	if remote == "" && (current.IsNull() || len(current.Elements()) == 0) {
		return current, nil
	}
	return types.ListValueFrom(ctx, types.StringType, strings.Split(remote, " "))
}

func (o *organizationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (o *organizationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config, plan, prior, state organizationResourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)

	if resp.Diagnostics.HasError() {
		return
	}

	org, err := o.client.GetOrganizationID()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to find organization",
			fmt.Sprintf("Unable to find Organization: %s", err.Error()),
		)
		return
	}
	log.Printf("Updating settings for organization %s ...", *org)
	if _, err := o.updateAllowedApiIpAddresses(ctx, *org, plan.AllowedApiIpAddresses, prior.AllowedApiIpAddresses); err != nil {
		resp.Diagnostics.AddError(
			"Unable to update Organization settings",
			fmt.Sprintf("Unable to update Organization settings: %s", err.Error()),
		)
		return
	}

	state.ID = types.StringValue(*org)
	state.UUID = prior.UUID
	state.AllowedApiIpAddresses = plan.AllowedApiIpAddresses
	// Seeded from prior so a step failing before it is reached keeps the last known value rather
	// than persisting a null over it. Each is overwritten by the step that owns it.
	state.Enforce2FA = prior.Enforce2FA
	state.RevokeInactiveTokensAfter = prior.RevokeInactiveTokensAfter
	state.RestrictUserApiTokenCreation = prior.RestrictUserApiTokenCreation

	// The allowlist mutation above has applied, so every path out from here has to record it, along
	// with whatever else applied before a later step failed. Deferred so a new early return cannot
	// forget it. Unlike Create, an Update that returns state alongside an error is not tainted, so
	// there is nothing to weigh against recording it.
	defer func() {
		resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
	}()

	if !plan.Enforce2FA.IsNull() && !plan.Enforce2FA.IsUnknown() && !plan.Enforce2FA.Equal(prior.Enforce2FA) {
		twoFAResponse, err := setOrganization2FA(ctx, o.client.genqlient, *org, plan.Enforce2FA.ValueBool())
		if err != nil {
			resp.Diagnostics.AddError("Unable to set 2FA", err.Error())
			return
		}
		state.Enforce2FA = types.BoolValue(twoFAResponse.OrganizationEnforceTwoFactorAuthenticationForMembersUpdate.Organization.MembersRequireTwoFactorAuthentication)
	}

	o.updateAPISettings(ctx, &config, &plan, &prior, &state, &resp.Diagnostics)
}

func (o *organizationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state organizationResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	org, err := o.client.GetOrganizationID()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to find organization",
			fmt.Sprintf("Unable to find Organization: %s", err.Error()),
		)
		return
	}
	log.Printf("Deleting settings for organization %s ...", *org)
	if _, err := o.updateAllowedApiIpAddresses(ctx, *org, types.ListNull(types.StringType), state.AllowedApiIpAddresses); err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete Organization settings",
			fmt.Sprintf("Unable to delete Organization settings: %s", err.Error()),
		)
		return
	}

	resp.Diagnostics.AddAttributeWarning(path.Root("enforce_2fa"), "Enforce 2FA setting left intact", "Use the web UI if you wish to change the value")
	resp.Diagnostics.AddWarning("API access token settings left intact", "Use the web UI if you wish to change them")
}

// updateAllowedApiIpAddresses sets the API IP allowlist, skipping the mutation when it is unchanged.
// It reports whether the allowlist was actually sent, so a caller that has to tell the practitioner
// what this apply changed does not have to work that out a second time and get a different answer.
func (o *organizationResource) updateAllowedApiIpAddresses(ctx context.Context, orgID string, planned, current types.List) (bool, error) {
	plannedValue := allowedApiIpAddressesValue(planned)
	// the mutation is rejected for organizations without the allowlist feature, even for ""
	if plannedValue == allowedApiIpAddressesValue(current) {
		return false, nil
	}
	_, err := setApiIpAddresses(ctx, o.client.genqlient, orgID, plannedValue)

	return err == nil, err
}

// warnAboutUnrecordedChanges reports the settings Create applied before failing at a later step.
// Create deliberately records no state in that case, because this resource applies settings to an
// organization that already exists rather than creating one: every step compares before it mutates,
// so a re-apply converges, while state returned alongside an error would taint the instance, and a
// tainted instance is replaced by a Delete that clears the allowlist. The cost of that choice is
// that nothing in state says the changes took effect, so say it here.
func warnAboutUnrecordedChanges(applied []string, diags *diag.Diagnostics) {
	if len(applied) == 0 {
		return
	}

	diags.AddWarning(
		"Organization settings changed but not recorded",
		fmt.Sprintf("Before this operation failed, %s. Those changes are not recorded in state, so they stay in effect "+
			"until this resource is applied again or they are changed in the Buildkite web UI.", strings.Join(applied, ", and ")),
	)
}

// allowedApiIpAddressesValue serializes the attribute for the API, where null, [] and [""] are all ""
func allowedApiIpAddressesValue(cidrs types.List) string {
	return strings.Join(createCidrSliceFromList(cidrs), " ")
}

const revokeInactiveTokensNever = "NEVER"

// revokeInactiveTokenPeriods mirrors the RevokeInactiveTokenPeriod GraphQL enum
var revokeInactiveTokenPeriods = []string{revokeInactiveTokensNever, "DAYS_30", "DAYS_60", "DAYS_90", "DAYS_180", "DAYS_365"}

// revokePeriodFromDays converts the REST revoke_inactive_tokens_after_days (nil = never) to a period
func revokePeriodFromDays(days *int64) string {
	if days == nil {
		return revokeInactiveTokensNever
	}
	return fmt.Sprintf("DAYS_%d", *days)
}

// revokePeriodToDays converts a period to the REST revoke_inactive_tokens_after_days (nil = never)
func revokePeriodToDays(period string) *int64 {
	var days int64
	if _, err := fmt.Sscanf(period, "DAYS_%d", &days); err != nil {
		return nil
	}
	return &days
}

type organizationAPISettings struct {
	RevokeInactiveTokensAfterDays *int64 `json:"revoke_inactive_tokens_after_days"`
	RestrictUserApiTokenCreation  bool   `json:"restrict_user_api_token_creation"`
	Features                      struct {
		InactiveApiTokenRevocation bool `json:"inactive_api_token_revocation"`
	} `json:"features"`
}

func (c *Client) getOrganizationAPISettings(ctx context.Context) (*organizationAPISettings, error) {
	var settings organizationAPISettings
	err := c.makeRequest(ctx, http.MethodGet, fmt.Sprintf("/v2/organizations/%s/api-settings", c.organization), nil, &settings)
	return &settings, err
}

func (c *Client) updateOrganizationAPISettings(ctx context.Context, payload map[string]any) (*organizationAPISettings, error) {
	var settings organizationAPISettings
	err := c.makeRequest(ctx, http.MethodPatch, fmt.Sprintf("/v2/organizations/%s/api-settings", c.organization), payload, &settings)
	return &settings, err
}

// apiSettingsFromModel returns the api-settings recorded in state, or the API defaults
func apiSettingsFromModel(model *organizationResourceModel) *organizationAPISettings {
	settings := &organizationAPISettings{}
	if model != nil {
		settings.RevokeInactiveTokensAfterDays = revokePeriodToDays(model.RevokeInactiveTokensAfter.ValueString())
		settings.RestrictUserApiTokenCreation = model.RestrictUserApiTokenCreation.ValueBool()
	}
	return settings
}

// apiSettingsPatch returns the configured api-settings that differ from current, or all of them when current isn't known
func apiSettingsPatch(config, plan *organizationResourceModel, current *organizationAPISettings, currentKnown bool) map[string]any {
	payload := map[string]any{}
	// config says whether an attribute is managed, plan holds the value to apply
	if !config.RevokeInactiveTokensAfter.IsNull() && !plan.RevokeInactiveTokensAfter.IsUnknown() {
		if revoke := plan.RevokeInactiveTokensAfter.ValueString(); !currentKnown || revoke != revokePeriodFromDays(current.RevokeInactiveTokensAfterDays) {
			payload["revoke_inactive_tokens_after_days"] = revokePeriodToDays(revoke)
		}
	}
	if !config.RestrictUserApiTokenCreation.IsNull() && !plan.RestrictUserApiTokenCreation.IsUnknown() {
		if restrict := plan.RestrictUserApiTokenCreation.ValueBool(); !currentKnown || restrict != current.RestrictUserApiTokenCreation {
			payload["restrict_user_api_token_creation"] = restrict
		}
	}
	return payload
}

func (o *organizationResource) readAPISettings(ctx context.Context, state *organizationResourceModel, diags *diag.Diagnostics) {
	settings, err := o.client.getOrganizationAPISettings(ctx)
	if err != nil {
		if !isAPIStatus(err, http.StatusForbidden) {
			diags.AddError("Unable to read organization API settings", fmt.Sprintf("Unable to read organization API settings: %s", err.Error()))
			return
		}
		// tolerate tokens without the read_organization_settings scope
		diags.AddWarning("Unable to read organization API settings", fmt.Sprintf("Unable to read organization API settings, keeping the last known values. The API token needs the read_organization_settings scope: %s", err.Error()))
		settings = apiSettingsFromModel(state)
	}
	state.RevokeInactiveTokensAfter = types.StringValue(revokePeriodFromDays(settings.RevokeInactiveTokensAfterDays))
	state.RestrictUserApiTokenCreation = types.BoolValue(settings.RestrictUserApiTokenCreation)
}

// updateAPISettings sends the configured api-settings that changed and records the result on state
func (o *organizationResource) updateAPISettings(ctx context.Context, config, plan, prior, state *organizationResourceModel, diags *diag.Diagnostics) {
	current, err := o.client.getOrganizationAPISettings(ctx)
	currentKnown := err == nil
	if err != nil {
		if !isAPIStatus(err, http.StatusForbidden) {
			diags.AddError("Unable to read organization API settings", fmt.Sprintf("Unable to read organization API settings: %s", err.Error()))
			return
		}
		// without the read scope every configured value is sent, and the rest keeps its last known value
		diags.AddWarning("Unable to read organization API settings", fmt.Sprintf("Unable to read organization API settings, keeping the last known values. The API token needs the read_organization_settings scope: %s", err.Error()))
		current = apiSettingsFromModel(prior)
	}

	// attributes that are not configured are left as they are
	state.RevokeInactiveTokensAfter = plan.RevokeInactiveTokensAfter
	if state.RevokeInactiveTokensAfter.IsNull() || state.RevokeInactiveTokensAfter.IsUnknown() {
		state.RevokeInactiveTokensAfter = types.StringValue(revokePeriodFromDays(current.RevokeInactiveTokensAfterDays))
	}
	state.RestrictUserApiTokenCreation = plan.RestrictUserApiTokenCreation
	if state.RestrictUserApiTokenCreation.IsNull() || state.RestrictUserApiTokenCreation.IsUnknown() {
		state.RestrictUserApiTokenCreation = types.BoolValue(current.RestrictUserApiTokenCreation)
	}

	payload := apiSettingsPatch(config, plan, current, currentKnown)
	if len(payload) == 0 {
		return
	}

	log.Printf("Updating API settings for organization %s ...", o.client.organization)
	if _, err := o.client.updateOrganizationAPISettings(ctx, payload); err != nil {
		detail := fmt.Sprintf("Unable to update organization API settings: %s", err.Error())
		if _, ok := payload["revoke_inactive_tokens_after_days"]; ok && currentKnown && !current.Features.InactiveApiTokenRevocation {
			detail += " Inactive API token revocation is not available on this organization's plan."
		}
		diags.AddError("Unable to update organization API settings", detail)
		// Nothing applied, so these have to describe the organization rather than the plan. On a
		// refused GET current stands in from the prior state, which is also what readAPISettings
		// falls back to, so leaving a planned value here would never be corrected by a refresh.
		state.RevokeInactiveTokensAfter = types.StringValue(revokePeriodFromDays(current.RevokeInactiveTokensAfterDays))
		state.RestrictUserApiTokenCreation = types.BoolValue(current.RestrictUserApiTokenCreation)
	}
}
