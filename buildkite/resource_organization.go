package buildkite

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type organizationResourceModel struct {
	AllowedApiIpAddresses         types.List   `tfsdk:"allowed_api_ip_addresses"`
	ID                            types.String `tfsdk:"id"`
	UUID                          types.String `tfsdk:"uuid"`
	Enforce2FA                    types.Bool   `tfsdk:"enforce_2fa"`
	RevokeInactiveTokensAfterDays types.Int64  `tfsdk:"revoke_inactive_tokens_after_days"`
	RestrictUserApiTokenCreation  types.Bool   `tfsdk:"restrict_user_api_token_creation"`
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

			Every setting other than ` + "`enforce_2fa`" + ` is managed through the REST API, so the token also
			needs the ` + "`read_organization_settings`" + ` and ` + "`write_organization_settings`" + ` scopes.
			These are newer than the resource itself: a token made before they existed has to be reissued,
			or refreshing this resource fails.
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
					"~> An allowlist set outside Terraform is read into state on refresh, so removing this attribute " +
					"from your configuration clears the allowlist on the next apply.\n\n" +
					"-> The \"Allowed API IP Addresses\" feature must be enabled on your organization in order to manage the `allowed_api_ip_addresses` attribute.",
			},
			"enforce_2fa": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Sets whether the organization requires two-factor authentication for all members.",
			},
			"revoke_inactive_tokens_after_days": schema.Int64Attribute{
				Optional: true,
				Validators: []validator.Int64{
					int64validator.OneOf(30, 60, 90, 180, 365),
				},
				MarkdownDescription: "The number of days an API access token can go unused before it is revoked. " +
					"Must be one of 30, 60, 90, 180 or 365. If not set, inactive tokens are never revoked.\n\n" +
					"~> Setting this revokes tokens that are already inactive as soon as the change is applied, " +
					"rather than waiting for the next scheduled sweep.\n\n" +
					"~> An interval set outside Terraform is read into state on refresh, so leaving this attribute " +
					"out of your configuration turns automatic revocation off on the next apply.\n\n" +
					"-> The \"Inactive API Token Revocation\" feature must be enabled on your organization in order to manage the `revoke_inactive_tokens_after_days` attribute.",
			},
			"restrict_user_api_token_creation": schema.BoolAttribute{
				Optional: true,
				MarkdownDescription: "Sets whether only organization administrators can create API access tokens that act on behalf of the organization.\n\n" +
					"~> A restriction set outside Terraform is read into state on refresh, so removing this attribute " +
					"from your configuration lifts it on the next apply. Leaving it out of a new configuration does not, " +
					"since the setting is only written once it has been named.",
			},
		},
	}
}

// organizationAPISettingsPatchBody builds the PATCH body for the settings served by the REST API,
// naming a setting only when it is this resource's to write and remote does not already hold the
// wanted value. Leaving an unchanged setting unnamed matters beyond saving bytes: the API refuses a
// plan-gated key on its presence alone, so naming one would fail an apply that never meant to touch
// it. A setting is this resource's to write once the configuration names it, or once state records a
// value the configuration has since dropped. state is nil on create, where nothing has been dropped
// and an unnamed setting belongs to whoever set it.
func organizationAPISettingsPatchBody(plan, state *organizationResourceModel, remote *organizationAPISettings) map[string]any {
	body := make(map[string]any)

	if isKnown(plan.AllowedApiIpAddresses) || (state != nil && allowedIPAddressesFromList(state.AllowedApiIpAddresses) != nil) {
		if want := allowedIPAddressesFromList(plan.AllowedApiIpAddresses); !allowlistsEqual(want, remote.AllowedIPAddresses) {
			body["allowed_ip_addresses"] = want
		}
	}

	if isKnown(plan.RevokeInactiveTokensAfterDays) || (state != nil && isKnown(state.RevokeInactiveTokensAfterDays)) {
		var want *int64
		if isKnown(plan.RevokeInactiveTokensAfterDays) {
			days := plan.RevokeInactiveTokensAfterDays.ValueInt64()
			want = &days
		}
		if !revocationIntervalsEqual(want, remote.RevokeInactiveTokensAfterDays) {
			body["revoke_inactive_tokens_after_days"] = want
		}
	}

	if isKnown(plan.RestrictUserApiTokenCreation) || (state != nil && isKnown(state.RestrictUserApiTokenCreation)) {
		if want := plan.RestrictUserApiTokenCreation.ValueBool(); want != remote.RestrictUserAPITokenCreation {
			body["restrict_user_api_token_creation"] = want
		}
	}

	return body
}

// organizationAPISettingsResetBody returns the settings this resource took over to their defaults,
// naming only the ones remote still holds. It is the patch body for a plan that configures nothing.
func organizationAPISettingsResetBody(state *organizationResourceModel, remote *organizationAPISettings) map[string]any {
	return organizationAPISettingsPatchBody(&organizationResourceModel{
		AllowedApiIpAddresses:         types.ListNull(types.StringType),
		RevokeInactiveTokensAfterDays: types.Int64Null(),
		RestrictUserApiTokenCreation:  types.BoolNull(),
	}, state, remote)
}

// managesAnyAPISetting reports whether state records a setting this resource took over, and so
// whether a destroy has anything to hand back.
func managesAnyAPISetting(state *organizationResourceModel) bool {
	return allowedIPAddressesFromList(state.AllowedApiIpAddresses) != nil ||
		isKnown(state.RevokeInactiveTokensAfterDays) ||
		state.RestrictUserApiTokenCreation.ValueBool()
}

// gatedSettings pairs each plan-gated key with the attribute and feature a diagnostic should name.
var gatedSettings = []struct {
	key       string
	attribute string
	feature   string
	available func(organizationAPISettingsFeatures) bool
}{
	{
		key:       "allowed_ip_addresses",
		attribute: "allowed_api_ip_addresses",
		feature:   "Allowed API IP Addresses",
		available: func(f organizationAPISettingsFeatures) bool { return f.APIIPAllowList },
	},
	{
		key:       "revoke_inactive_tokens_after_days",
		attribute: "revoke_inactive_tokens_after_days",
		feature:   "Inactive API Token Revocation",
		available: func(f organizationAPISettingsFeatures) bool { return f.InactiveAPITokenRevocation },
	},
}

// unavailableSettings names the attributes in body the organization's plan does not cover. Reading
// that from the features the endpoint reports says which setting is at fault, where the 403 the
// request would otherwise earn only says that one of them is.
func unavailableSettings(body map[string]any, features organizationAPISettingsFeatures) []string {
	var unavailable []string
	for _, setting := range gatedSettings {
		if _, named := body[setting.key]; named && !setting.available(features) {
			unavailable = append(unavailable, fmt.Sprintf("%s (%q)", setting.attribute, setting.feature))
		}
	}

	return unavailable
}

// dropUnavailableSettings removes the keys the organization's plan does not cover and returns the
// attributes it dropped, so a destroy can still hand back the settings it is allowed to write.
func dropUnavailableSettings(body map[string]any, features organizationAPISettingsFeatures) []string {
	dropped := unavailableSettings(body, features)
	for _, setting := range gatedSettings {
		if !setting.available(features) {
			delete(body, setting.key)
		}
	}

	return dropped
}

// allowlistsEqual compares two renderings of the allowlist. Null and the empty string both mean "no
// restrictions", and the API stores whatever spacing it was sent, so neither distinction is drift.
func allowlistsEqual(a, b *string) bool {
	return joinAllowlist(a) == joinAllowlist(b)
}

func joinAllowlist(allowlist *string) string {
	if allowlist == nil {
		return ""
	}

	return strings.Join(strings.Fields(*allowlist), " ")
}

func revocationIntervalsEqual(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}

	return *a == *b
}

// allowedIPAddressesFromList renders a CIDR list as the space separated string the API expects, or
// nil for a list that means "no restrictions". Empty, null and unknown elements are dropped rather
// than sent, since the API reads them as CIDR ranges and refuses them.
func allowedIPAddressesFromList(list types.List) *string {
	cidrs := make([]string, 0, len(list.Elements()))
	for _, element := range list.Elements() {
		cidr, ok := element.(types.String)
		if !ok || !isKnown(cidr) || cidr.ValueString() == "" {
			continue
		}
		cidrs = append(cidrs, cidr.ValueString())
	}

	if len(cidrs) == 0 {
		return nil
	}

	joined := strings.Join(cidrs, " ")

	return &joined
}

// readOrganizationAPISettings copies the API's view of the REST-managed settings into state. The
// allowlist is left as it is wherever state already renders to what the API reports, since null, an
// empty list and a list carrying empty strings all mean "no restrictions"; rewriting those into a
// canonical form would leave a difference no apply can settle.
func readOrganizationAPISettings(ctx context.Context, state *organizationResourceModel, settings *organizationAPISettings) diag.Diagnostics {
	var diags diag.Diagnostics

	var allowlist string
	if settings.AllowedIPAddresses != nil {
		allowlist = *settings.AllowedIPAddresses
	}

	cidrs := strings.Fields(allowlist)
	inState := allowedIPAddressesFromList(state.AllowedApiIpAddresses)

	if len(cidrs) == 0 {
		if inState != nil {
			state.AllowedApiIpAddresses = types.ListNull(types.StringType)
		}
	} else if inState == nil || *inState != strings.Join(cidrs, " ") {
		ips, listDiags := types.ListValueFrom(ctx, types.StringType, cidrs)
		diags.Append(listDiags...)
		if diags.HasError() {
			return diags
		}
		state.AllowedApiIpAddresses = ips
	}

	if settings.RevokeInactiveTokensAfterDays != nil {
		state.RevokeInactiveTokensAfterDays = types.Int64Value(*settings.RevokeInactiveTokensAfterDays)
	} else {
		state.RevokeInactiveTokensAfterDays = types.Int64Null()
	}

	// Left alone where state already agrees, for the same reason as the allowlist: an organization
	// that never had the restriction would otherwise read back as a configured false and plan a
	// change to null on every refresh.
	if settings.RestrictUserAPITokenCreation != state.RestrictUserApiTokenCreation.ValueBool() {
		state.RestrictUserApiTokenCreation = types.BoolValue(settings.RestrictUserAPITokenCreation)
	}

	return diags
}

// organizationSettingsError reports a REST failure. A 403 leads with the API's own words, which name
// the entitlement or scope that is missing, and adds the conditions that produce one for the request
// that earned it.
func organizationSettingsError(action string, err error) (summary, detail string) {
	summary = fmt.Sprintf("Unable to %s Organization settings", action)

	if !isAPIStatus(err, http.StatusForbidden) {
		return summary, err.Error()
	}

	causes := "the token is missing the write_organization_settings scope, its user is not an organization " +
		"administrator, or the organization's billing plan does not cover the setting being changed"
	if action == "read" {
		causes = "the token is missing the read_organization_settings scope, or its user is not an " +
			"organization administrator"
	}

	return summary, fmt.Sprintf("%s\n\nThe API refuses this request when %s.\n\n%s", apiErrorMessage(err), causes, err.Error())
}

// fetchOrganization looks the organization up over GraphQL, which is where its ID, UUID and 2FA
// setting live. A slug with no organization behind it comes back as an empty response rather than an
// error, which the removed GetOrganizationID helper used to report.
func (o *organizationResource) fetchOrganization(ctx context.Context) (*getOrganizationResponse, diag.Diagnostics) {
	var diags diag.Diagnostics

	organization, err := getOrganization(ctx, o.client.genqlient, o.client.organization)
	if err != nil {
		diags.AddError(
			"Unable to obtain Organization",
			fmt.Sprintf("Unable to obtain Organization: %s", err.Error()),
		)
		return nil, diags
	}
	if organization.Organization.Id == "" {
		diags.AddError(
			"Unable to find organization",
			fmt.Sprintf("Could not find organization with slug %q", o.client.organization),
		)
		return nil, diags
	}

	return organization, diags
}

// writeOrganizationAPISettings applies body, having first refused a request naming a setting the
// organization's plan does not cover so the practitioner is told which attribute is at fault rather
// than reading it out of a 403. An empty body is no request at all.
func (o *organizationResource) writeOrganizationAPISettings(ctx context.Context, action string, body map[string]any, features organizationAPISettingsFeatures) diag.Diagnostics {
	var diags diag.Diagnostics

	if len(body) == 0 {
		return diags
	}

	if unavailable := unavailableSettings(body, features); len(unavailable) > 0 {
		diags.AddError(
			fmt.Sprintf("Unable to %s Organization settings", action),
			fmt.Sprintf("The organization's billing plan does not cover %s, so %s cannot be changed. Remove the "+
				"attribute from your configuration, or contact Buildkite about the plan feature.",
				strings.Join(unavailable, " or "), pluralise(len(unavailable), "it", "they")),
		)
		return diags
	}

	if _, err := o.client.UpdateOrganizationAPISettings(ctx, o.client.organization, body); err != nil {
		diags.AddError(organizationSettingsError(action, err))
	}

	return diags
}

func pluralise(count int, one, many string) string {
	if count == 1 {
		return one
	}

	return many
}

func (o *organizationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, state organizationResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("Creating settings for organization %s ...", o.client.organization)

	// Read before writing, so a failure looking the organization up can't leave settings changed with
	// nothing recorded in state.
	organization, diags := o.fetchOrganization(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	settings, err := o.client.GetOrganizationAPISettings(ctx, o.client.organization)
	if err != nil {
		resp.Diagnostics.AddError(organizationSettingsError("read", err))
		return
	}

	resp.Diagnostics.Append(o.writeOrganizationAPISettings(ctx, "create", organizationAPISettingsPatchBody(&plan, nil, settings), settings.Features)...)
	if resp.Diagnostics.HasError() {
		return
	}

	state.ID = types.StringValue(organization.Organization.Id)
	state.UUID = types.StringValue(organization.Organization.Uuid)

	// Echoed from the plan rather than from the response, because Terraform requires an optional
	// attribute to end an apply holding exactly what was configured and an empty allowlist comes back
	// as null however it was written.
	state.AllowedApiIpAddresses = plan.AllowedApiIpAddresses
	state.RevokeInactiveTokensAfterDays = plan.RevokeInactiveTokensAfterDays
	state.RestrictUserApiTokenCreation = plan.RestrictUserApiTokenCreation
	state.Enforce2FA = types.BoolValue(organization.Organization.MembersRequireTwoFactorAuthentication)

	// Record what the API settings request wrote before touching 2FA, so a failure there leaves
	// something to destroy rather than settings nothing knows about.
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if isKnown(plan.Enforce2FA) && plan.Enforce2FA.ValueBool() != organization.Organization.MembersRequireTwoFactorAuthentication {
		if _, err := setOrganization2FA(ctx, o.client.genqlient, organization.Organization.Id, plan.Enforce2FA.ValueBool()); err != nil {
			resp.Diagnostics.AddError("Unable to set 2FA", err.Error())
			return
		}

		state.Enforce2FA = plan.Enforce2FA
		resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
	}
}

func (o *organizationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state organizationResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("Reading settings for organization %s ...", o.client.organization)
	response, diags := o.fetchOrganization(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	settings, err := o.client.GetOrganizationAPISettings(ctx, o.client.organization)
	if err != nil {
		resp.Diagnostics.AddError(organizationSettingsError("read", err))
		return
	}

	state.ID = types.StringValue(response.Organization.Id)
	state.UUID = types.StringValue(response.Organization.Uuid)
	state.Enforce2FA = types.BoolValue(response.Organization.MembersRequireTwoFactorAuthentication)

	resp.Diagnostics.Append(readOrganizationAPISettings(ctx, &state, settings)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (o *organizationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (o *organizationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state organizationResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("Updating settings for organization %s ...", o.client.organization)

	organization, diags := o.fetchOrganization(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	settings, err := o.client.GetOrganizationAPISettings(ctx, o.client.organization)
	if err != nil {
		resp.Diagnostics.AddError(organizationSettingsError("read", err))
		return
	}

	// state holds the prior values, which is what tells a setting the configuration dropped apart
	// from one it never named.
	resp.Diagnostics.Append(o.writeOrganizationAPISettings(ctx, "update", organizationAPISettingsPatchBody(&plan, &state, settings), settings.Features)...)
	if resp.Diagnostics.HasError() {
		return
	}

	state.AllowedApiIpAddresses = plan.AllowedApiIpAddresses
	state.RevokeInactiveTokensAfterDays = plan.RevokeInactiveTokensAfterDays
	state.RestrictUserApiTokenCreation = plan.RestrictUserApiTokenCreation
	state.Enforce2FA = types.BoolValue(organization.Organization.MembersRequireTwoFactorAuthentication)

	if isKnown(plan.Enforce2FA) && plan.Enforce2FA.ValueBool() != organization.Organization.MembersRequireTwoFactorAuthentication {
		twoFAResponse, err := setOrganization2FA(ctx, o.client.genqlient, organization.Organization.Id, plan.Enforce2FA.ValueBool())
		if err != nil {
			resp.Diagnostics.AddError("Unable to set 2FA", err.Error())
			// Saved anyway, so the settings the request above did write are not lost to the failure.
			resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
			return
		}
		state.Enforce2FA = types.BoolValue(twoFAResponse.OrganizationEnforceTwoFactorAuthenticationForMembersUpdate.Organization.MembersRequireTwoFactorAuthentication)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (o *organizationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state organizationResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("Deleting settings for organization %s ...", o.client.organization)

	// Nothing to hand back means nothing to read either, so a resource that only ever managed 2FA can
	// still be destroyed by a token without the REST scopes.
	if !managesAnyAPISetting(&state) {
		resp.Diagnostics.AddAttributeWarning(path.Root("enforce_2fa"), "Enforce 2FA setting left intact", "Use the web UI if you wish to change the value")
		return
	}

	settings, err := o.client.GetOrganizationAPISettings(ctx, o.client.organization)
	if err != nil {
		resp.Diagnostics.AddError(organizationSettingsError("read", err))
		return
	}

	// The settings themselves can't be deleted, so returning the ones this resource took over is
	// the closest thing to removing it.
	body := organizationAPISettingsResetBody(&state, settings)

	// An organization downgraded off a plan that once covered one of these settings can no longer
	// write it at all, which would leave the resource undestroyable. Handing back what it still can
	// beats refusing to destroy, so those settings are dropped from the request rather than failing
	// it. Every other refusal, a missing scope or a demoted user among them, is a real error: a
	// destroy that reports success while an IP allowlist stays in force is worse than one that fails.
	if dropped := dropUnavailableSettings(body, settings.Features); len(dropped) > 0 {
		resp.Diagnostics.AddWarning(
			"Organization API settings left as they are",
			fmt.Sprintf("The organization's billing plan no longer covers %s, so %s keep the value Terraform last "+
				"applied and have to be changed in the web UI.",
				strings.Join(dropped, " or "), pluralise(len(dropped), "it", "they")),
		)
	}

	if len(body) > 0 {
		if _, err := o.client.UpdateOrganizationAPISettings(ctx, o.client.organization, body); err != nil {
			resp.Diagnostics.AddError(organizationSettingsError("delete", err))
			return
		}
	}

	resp.Diagnostics.AddAttributeWarning(path.Root("enforce_2fa"), "Enforce 2FA setting left intact", "Use the web UI if you wish to change the value")
}
