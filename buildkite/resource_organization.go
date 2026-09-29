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
			Every attribute other than ` + "`enforce_2fa`" + ` is managed through the organization API settings
			endpoint. The resource reads that endpoint on every apply, so the token needs the
			` + "`read_organization_settings`" + ` scope whatever the configuration sets, and
			` + "`write_organization_settings`" + ` to change any of those attributes.
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
					"If omitted, the current setting is left unchanged. Requires the inactive API token revocation feature on the organization's plan.",
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
					"If omitted, the current setting is left unchanged.",
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
	// What this apply has already changed on the organization. Create records no state when a later
	// step fails, so anything collected here has to be reported instead. Reported in a defer so a
	// new early return cannot drop the warning, which is the only record these changes applied.
	var applied []string
	defer func() {
		if resp.Diagnostics.HasError() {
			warnAboutUnrecordedChanges(applied, &resp.Diagnostics)
		}
	}()

	state.ID = types.StringValue(*org)
	state.UUID = types.StringValue(organization.Organization.Uuid)

	// api-settings goes first. A setting the organization's plan does not include is refused
	// outright, and refusing it changes nothing, so that failure cannot leave 2FA already flipped
	// on an organization terraform has no state for.
	applied = append(applied, o.updateAPISettings(ctx, &config, &plan, &state, &resp.Diagnostics)...)
	if resp.Diagnostics.HasError() {
		return
	}

	state.Enforce2FA = types.BoolValue(organization.Organization.MembersRequireTwoFactorAuthentication)
	if !plan.Enforce2FA.IsNull() && !plan.Enforce2FA.IsUnknown() && plan.Enforce2FA.ValueBool() != organization.Organization.MembersRequireTwoFactorAuthentication {
		if _, err := setOrganization2FA(ctx, o.client.genqlient, *org, plan.Enforce2FA.ValueBool()); err != nil {
			resp.Diagnostics.AddError("Unable to set 2FA", err.Error())
			// no state. Recording a failed create taints the resource, and the replacement that
			// follows destroys before it creates, clearing an allowlist that did land. Leaving the
			// create unrecorded keeps the organization as terraform found it, and applying again
			// writes only what still differs before retrying 2FA. The deferred warning is what
			// names the api-settings that did land.
			return
		}
		change := "two-factor authentication enforcement was removed"
		if plan.Enforce2FA.ValueBool() {
			change = "two-factor authentication was enforced for all members"
		}
		applied = append(applied, change)
		state.Enforce2FA = plan.Enforce2FA
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
	state.ID = types.StringValue(*org)
	state.UUID = prior.UUID
	// Seeded from prior so a step failing before it is reached keeps the last known value rather
	// than persisting a null over it. Each is overwritten by the step that owns it.
	state.Enforce2FA = prior.Enforce2FA
	state.AllowedApiIpAddresses = prior.AllowedApiIpAddresses
	state.RevokeInactiveTokensAfter = prior.RevokeInactiveTokensAfter
	state.RestrictUserApiTokenCreation = prior.RestrictUserApiTokenCreation

	// Whatever applies below has to be recorded on every path out, not just the one that reaches
	// the end. Deferred so a new early return cannot forget it. Unlike Create, an Update that
	// returns state alongside an error is not tainted, so there is nothing to weigh against
	// recording it.
	defer func() {
		resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
	}()

	// as in Create, the refusable write goes first so it cannot fail behind a 2FA change
	o.updateAPISettings(ctx, &config, &plan, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	if !plan.Enforce2FA.IsNull() && !plan.Enforce2FA.IsUnknown() && !plan.Enforce2FA.Equal(prior.Enforce2FA) {
		twoFAResponse, err := setOrganization2FA(ctx, o.client.genqlient, *org, plan.Enforce2FA.ValueBool())
		if err != nil {
			resp.Diagnostics.AddError("Unable to set 2FA", err.Error())
			resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
			return
		}
		state.Enforce2FA = types.BoolValue(twoFAResponse.OrganizationEnforceTwoFactorAuthenticationForMembersUpdate.Organization.MembersRequireTwoFactorAuthentication)
	}
}

func (o *organizationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	org, err := o.client.GetOrganizationID()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to find organization",
			fmt.Sprintf("Unable to find Organization: %s", err.Error()),
		)
		return
	}
	log.Printf("Deleting settings for organization %s ...", *org)
	// the allowlist is the one setting terraform owns outright, so it goes when the resource does.
	// State does not answer whether there is one to clear: a refresh that could not read the settings
	// keeps whatever it last saw, so the organization is asked instead. Organizations without the
	// allowlist feature are refused even the empty value they already have, which is what the
	// request is skipped for.
	current, err := o.client.getOrganizationAPISettings(ctx)
	if err != nil {
		// a destroy that cannot read has no way to finish, so say what does finish it. Every status
		// lands here, not just the forbidden one the scope hint covers.
		resp.Diagnostics.AddError(
			"Unable to read organization API settings",
			unreadableAPISettingsDetail(err)+" Destroying this resource clears the allowed API IP addresses, "+
				"which the organization has to answer for first. Restore access and destroy again, or remove "+
				"the resource from state with terraform state rm to leave the organization's settings as they are.",
		)
		return
	}
	if current.AllowedIpAddresses != "" {
		if _, err := o.client.updateOrganizationAPISettings(ctx, map[string]any{"allowed_ip_addresses": ""}); err != nil {
			resp.Diagnostics.AddError(
				"Unable to delete Organization settings",
				fmt.Sprintf("Unable to clear the allowed API IP addresses: %s", err.Error()),
			)
			return
		}
	}

	resp.Diagnostics.AddAttributeWarning(path.Root("enforce_2fa"), "Enforce 2FA setting left intact", "Use the web UI if you wish to change the value")
	resp.Diagnostics.AddWarning("API access token settings left intact", "Use the web UI if you wish to change them")
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
		fmt.Sprintf("Before this operation failed, %s. No state was recorded for this resource, so terraform will not "+
			"clear those changes: they stay in effect until this resource is applied again, which writes only the "+
			"settings that still differ, or until they are changed in the Buildkite web UI.",
			strings.Join(applied, ", and ")),
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
	AllowedIpAddresses            string `json:"allowed_ip_addresses"`
	RevokeInactiveTokensAfterDays *int64 `json:"revoke_inactive_tokens_after_days"`
	RestrictUserApiTokenCreation  bool   `json:"restrict_user_api_token_creation"`
	Features                      struct {
		ApiIpAllowList             bool `json:"api_ip_allow_list"`
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

// apiSettingsFromModel returns the api-settings recorded in state
func apiSettingsFromModel(model *organizationResourceModel) *organizationAPISettings {
	return &organizationAPISettings{
		AllowedIpAddresses:            allowedApiIpAddressesValue(model.AllowedApiIpAddresses),
		RevokeInactiveTokensAfterDays: revokePeriodToDays(model.RevokeInactiveTokensAfter.ValueString()),
		RestrictUserApiTokenCreation:  model.RestrictUserApiTokenCreation.ValueBool(),
	}
}

// apiSettingsPatch returns the configured api-settings that differ from current
func apiSettingsPatch(config, plan *organizationResourceModel, current *organizationAPISettings) map[string]any {
	payload := map[string]any{}
	// the allowlist is owned rather than adopted: dropping it from the configuration clears it, so
	// the plan alone says what it should be. Unchanged values stay out of the request because
	// organizations without the allowlist feature are refused even those.
	if allowed := allowedApiIpAddressesValue(plan.AllowedApiIpAddresses); allowed != current.AllowedIpAddresses {
		payload["allowed_ip_addresses"] = allowed
	}
	// config says whether an attribute is managed, plan holds the value to apply
	if !config.RevokeInactiveTokensAfter.IsNull() && !plan.RevokeInactiveTokensAfter.IsUnknown() {
		if revoke := plan.RevokeInactiveTokensAfter.ValueString(); revoke != revokePeriodFromDays(current.RevokeInactiveTokensAfterDays) {
			payload["revoke_inactive_tokens_after_days"] = revokePeriodToDays(revoke)
		}
	}
	if !config.RestrictUserApiTokenCreation.IsNull() && !plan.RestrictUserApiTokenCreation.IsUnknown() {
		if restrict := plan.RestrictUserApiTokenCreation.ValueBool(); restrict != current.RestrictUserApiTokenCreation {
			payload["restrict_user_api_token_creation"] = restrict
		}
	}
	return payload
}

// unreadableAPISettingsDetail describes a settings read that failed, naming the access a forbidden
// answer asks for. The endpoint requires an organization administrator to read as well as write, so a
// token already carrying the scope is refused just the same. Callers with a way out of the failure
// append it.
func unreadableAPISettingsDetail(err error) string {
	detail := fmt.Sprintf("Unable to read organization API settings: %s", err.Error())
	if isAPIStatus(err, http.StatusForbidden) {
		detail += fmt.Sprintf(" The API token needs the %s scope, and its user must be an organization administrator.", readOrganizationSettingsScope)
	}
	return detail
}

func addUnreadableAPISettingsError(diags *diag.Diagnostics, err error) {
	diags.AddError("Unable to read organization API settings", unreadableAPISettingsDetail(err))
}

func (o *organizationResource) readAPISettings(ctx context.Context, state *organizationResourceModel, diags *diag.Diagnostics) {
	settings, err := o.client.getOrganizationAPISettings(ctx)
	if err != nil {
		if !isAPIStatus(err, http.StatusForbidden) {
			addUnreadableAPISettingsError(diags, err)
			return
		}
		// tolerate a refresh that is not allowed to read the settings
		diags.AddWarning("Unable to read organization API settings", fmt.Sprintf("Unable to read organization API settings, keeping the last known values. The API token needs the %s scope, and its user must be an organization administrator: %s", readOrganizationSettingsScope, err.Error()))
		settings = apiSettingsFromModel(state)
	}

	allowed, allowedDiags := allowedApiIpAddressesFromAPI(ctx, settings.AllowedIpAddresses, state.AllowedApiIpAddresses)
	diags.Append(allowedDiags...)
	if diags.HasError() {
		return
	}
	state.AllowedApiIpAddresses = allowed
	state.RevokeInactiveTokensAfter = types.StringValue(revokePeriodFromDays(settings.RevokeInactiveTokensAfterDays))
	state.RestrictUserApiTokenCreation = types.BoolValue(settings.RestrictUserApiTokenCreation)
}

// updateAPISettings sends the configured api-settings that changed and records the result on state.
// It returns a description of each setting the patch applied, for a caller that has to tell the
// practitioner what an apply it could not record left behind.
func (o *organizationResource) updateAPISettings(ctx context.Context, config, plan, state *organizationResourceModel, diags *diag.Diagnostics) []string {
	// settings that are about to be written have to be read first. The allowlist is owned outright,
	// so an unreadable one cannot be told from an empty one, and skipping the request on that guess
	// would record an allowlist the organization never took.
	current, err := o.client.getOrganizationAPISettings(ctx)
	if err != nil {
		addUnreadableAPISettingsError(diags, err)
		return nil
	}

	// what state had before it is made to describe the plan, so the failure path below can put back
	// an allowlist the patch never applied
	recorded := state.AllowedApiIpAddresses
	// the allowlist is not adopted from the organization, so state follows the configuration exactly
	state.AllowedApiIpAddresses = plan.AllowedApiIpAddresses
	// the remaining attributes are left as they are when they are not configured
	state.RevokeInactiveTokensAfter = plan.RevokeInactiveTokensAfter
	if state.RevokeInactiveTokensAfter.IsNull() || state.RevokeInactiveTokensAfter.IsUnknown() {
		state.RevokeInactiveTokensAfter = types.StringValue(revokePeriodFromDays(current.RevokeInactiveTokensAfterDays))
	}
	state.RestrictUserApiTokenCreation = plan.RestrictUserApiTokenCreation
	if state.RestrictUserApiTokenCreation.IsNull() || state.RestrictUserApiTokenCreation.IsUnknown() {
		state.RestrictUserApiTokenCreation = types.BoolValue(current.RestrictUserApiTokenCreation)
	}

	payload := apiSettingsPatch(config, plan, current)
	if len(payload) == 0 {
		return nil
	}

	log.Printf("Updating API settings for organization %s ...", o.client.organization)
	if _, err := o.client.updateOrganizationAPISettings(ctx, payload); err != nil {
		detail := fmt.Sprintf("Unable to update organization API settings: %s", err.Error())
		// a 403 answers a plan-gated setting as readily as a token without the scope, so name the
		// feature the organization's plan is missing where that is what the request asked for
		if isAPIStatus(err, http.StatusForbidden) {
			if _, ok := payload["allowed_ip_addresses"]; ok && !current.Features.ApiIpAllowList {
				detail += " The allowed API IP addresses feature is not available on this organization's plan."
			}
			if _, ok := payload["revoke_inactive_tokens_after_days"]; ok && !current.Features.InactiveApiTokenRevocation {
				detail += " Inactive API token revocation is not available on this organization's plan."
			}
		}
		diags.AddError("Unable to update organization API settings", detail)
		// The patch is one request, so nothing in it applied and state has to describe the
		// organization rather than the plan. Update records state on this path, and a later refresh
		// that cannot read the settings keeps whatever it last saw, so a planned value left here
		// would be adopted rather than replanned.
		allowed, allowedDiags := allowedApiIpAddressesFromAPI(ctx, current.AllowedIpAddresses, recorded)
		diags.Append(allowedDiags...)
		state.AllowedApiIpAddresses = allowed
		state.RevokeInactiveTokensAfter = types.StringValue(revokePeriodFromDays(current.RevokeInactiveTokensAfterDays))
		state.RestrictUserApiTokenCreation = types.BoolValue(current.RestrictUserApiTokenCreation)

		return nil
	}

	return describeAPISettingsChanges(payload, plan)
}

// describeAPISettingsChanges names the api-settings a patch applied, in the terms the attributes
// use. The payload says which settings were sent, so the description cannot disagree with the
// request the way a second comparison against the organization could.
func describeAPISettingsChanges(payload map[string]any, plan *organizationResourceModel) []string {
	changes := make([]string, 0, len(payload))
	if _, ok := payload["allowed_ip_addresses"]; ok {
		if allowed := allowedApiIpAddressesValue(plan.AllowedApiIpAddresses); allowed == "" {
			changes = append(changes, "the API IP allowlist was cleared")
		} else {
			changes = append(changes, fmt.Sprintf("the API IP allowlist was set to %q", allowed))
		}
	}
	if _, ok := payload["revoke_inactive_tokens_after_days"]; ok {
		if period := plan.RevokeInactiveTokensAfter.ValueString(); period == revokeInactiveTokensNever {
			changes = append(changes, "inactive API token revocation was turned off")
		} else {
			changes = append(changes, fmt.Sprintf("inactive API token revocation was set to %s", period))
		}
	}
	if _, ok := payload["restrict_user_api_token_creation"]; ok {
		if plan.RestrictUserApiTokenCreation.ValueBool() {
			changes = append(changes, "user API token creation was restricted")
		} else {
			changes = append(changes, "the restriction on user API token creation was removed")
		}
	}
	return changes
}
