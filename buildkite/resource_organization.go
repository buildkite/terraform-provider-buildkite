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

// revokeInactiveTokensAfterDaysValues are the only intervals the API accepts.
var revokeInactiveTokensAfterDaysValues = []int64{30, 60, 90, 180, 365}

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

			Every attribute other than ` + "`enforce_2fa`" + ` is managed through the REST API, so the token also
			needs the ` + "`read_organization_settings`" + ` and ` + "`write_organization_settings`" + ` scopes.
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
			"revoke_inactive_tokens_after_days": schema.Int64Attribute{
				Optional: true,
				Validators: []validator.Int64{
					int64validator.OneOf(revokeInactiveTokensAfterDaysValues...),
				},
				MarkdownDescription: "The number of days an API access token can go unused before it is revoked. " +
					"Must be one of 30, 60, 90, 180 or 365. If not set, inactive tokens are never revoked.\n\n" +
					"~> Setting this revokes tokens that are already inactive as soon as the change is applied, " +
					"rather than waiting for the next scheduled sweep.\n\n" +
					"-> The \"Inactive API Token Revocation\" feature must be enabled on your organization in order to manage the `revoke_inactive_tokens_after_days` attribute.",
			},
			"restrict_user_api_token_creation": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Sets whether only organization administrators can create API access tokens that act on behalf of the organization.",
			},
		},
	}
}

// organizationAPISettingsPatchBody builds the PATCH body for the settings served by the REST API.
// A key is only included when the plan sets it, or when a value already in state needs clearing:
// the API refuses a plan-gated setting with a 403 on the key being present at all, so an
// organization whose plan lacks one must not see that key in a request that isn't about it.
// state is nil on create, where there is nothing to clear.
func organizationAPISettingsPatchBody(plan, state *organizationResourceModel) map[string]any {
	body := make(map[string]any)

	switch {
	case isKnown(plan.AllowedApiIpAddresses):
		body["allowed_ip_addresses"] = allowedIPAddressesFromList(plan.AllowedApiIpAddresses)
	case state != nil && isKnown(state.AllowedApiIpAddresses):
		body["allowed_ip_addresses"] = nil
	}

	switch {
	case isKnown(plan.RevokeInactiveTokensAfterDays):
		body["revoke_inactive_tokens_after_days"] = plan.RevokeInactiveTokensAfterDays.ValueInt64()
	case state != nil && isKnown(state.RevokeInactiveTokensAfterDays):
		body["revoke_inactive_tokens_after_days"] = nil
	}

	// Ungated and defaulted, so it is always both safe to send and known.
	body["restrict_user_api_token_creation"] = plan.RestrictUserApiTokenCreation.ValueBool()

	return body
}

// organizationAPISettingsResetBody returns the settings this resource had taken over to their
// defaults, and nil when it had taken over none of them. Restricting it to what is in state keeps
// destroy from naming a plan-gated setting the organization can't be asked about.
func organizationAPISettingsResetBody(state *organizationResourceModel) map[string]any {
	body := make(map[string]any)

	if isKnown(state.AllowedApiIpAddresses) {
		body["allowed_ip_addresses"] = nil
	}
	if isKnown(state.RevokeInactiveTokensAfterDays) {
		body["revoke_inactive_tokens_after_days"] = nil
	}
	if state.RestrictUserApiTokenCreation.ValueBool() {
		body["restrict_user_api_token_creation"] = false
	}

	if len(body) == 0 {
		return nil
	}

	return body
}

// allowedIPAddressesFromList renders a CIDR list as the space separated string the API expects,
// or nil to clear the allowlist. An empty list and a list holding only empty strings both mean
// "no restrictions", which the API spells as null.
func allowedIPAddressesFromList(list types.List) *string {
	cidrs := make([]string, 0, len(list.Elements()))
	for _, cidr := range createCidrSliceFromList(list) {
		if cidr != "" {
			cidrs = append(cidrs, cidr)
		}
	}

	if len(cidrs) == 0 {
		return nil
	}

	joined := strings.Join(cidrs, " ")

	return &joined
}

// readOrganizationAPISettings copies the API's view of the REST-managed settings into state.
//
// The allowlist needs care in the case where the API reports no restrictions. Null, an empty list
// and a list of empty strings all say that same thing, so state keeps whichever of them it already
// held rather than being rewritten into a difference no apply can settle. State holding real ranges
// is a different matter: the allowlist was cleared outside Terraform, and that has to read as drift.
func readOrganizationAPISettings(ctx context.Context, state *organizationResourceModel, settings *organizationAPISettings) diag.Diagnostics {
	var diags diag.Diagnostics

	var allowlist string
	if settings.AllowedIPAddresses != nil {
		allowlist = *settings.AllowedIPAddresses
	}

	switch cidrs := strings.Fields(allowlist); {
	case len(cidrs) > 0:
		ips, listDiags := types.ListValueFrom(ctx, types.StringType, cidrs)
		diags.Append(listDiags...)
		if diags.HasError() {
			return diags
		}
		state.AllowedApiIpAddresses = ips
	case allowedIPAddressesFromList(state.AllowedApiIpAddresses) != nil:
		state.AllowedApiIpAddresses = types.ListNull(types.StringType)
	}

	if settings.RevokeInactiveTokensAfterDays != nil {
		state.RevokeInactiveTokensAfterDays = types.Int64Value(*settings.RevokeInactiveTokensAfterDays)
	} else {
		state.RevokeInactiveTokensAfterDays = types.Int64Null()
	}

	state.RestrictUserApiTokenCreation = types.BoolValue(settings.RestrictUserAPITokenCreation)

	return diags
}

// organizationSettingsError reports a REST failure, leading with the API's own words for a refusal
// that names the billing plan and keeping the request details for everything else.
func organizationSettingsError(action string, err error) (string, string) {
	if isAPIStatus(err, http.StatusForbidden) {
		return fmt.Sprintf("Unable to %s Organization settings", action), apiErrorMessage(err)
	}

	return fmt.Sprintf("Unable to %s Organization settings", action),
		fmt.Sprintf("Unable to %s Organization settings: %s", action, err.Error())
}

func (o *organizationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, state organizationResourceModel

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
	log.Printf("Creating settings for organization %s ...", *org)

	// Read before writing, so a failure looking the organization up can't leave settings changed
	// with nothing recorded in state.
	organization, err := getOrganization(ctx, o.client.genqlient, o.client.organization)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to obtain Organization",
			fmt.Sprintf("Unable to obtain Organization: %s", err.Error()),
		)
		return
	}

	settings, err := o.client.UpdateOrganizationAPISettings(ctx, o.client.organization, organizationAPISettingsPatchBody(&plan, nil))
	if err != nil {
		resp.Diagnostics.AddError(organizationSettingsError("create", err))
		return
	}

	// enforce_2fa defaults rather than being left unset, so the planned value is always known and
	// comparing against the organization is what keeps a no-op apply from writing to it.
	if isKnown(plan.Enforce2FA) && plan.Enforce2FA.ValueBool() != organization.Organization.MembersRequireTwoFactorAuthentication {
		_, err = setOrganization2FA(ctx, o.client.genqlient, *org, plan.Enforce2FA.ValueBool())
		if err != nil {
			resp.Diagnostics.AddError("Unable to set 2FA", err.Error())
			return
		}
	}

	state.ID = types.StringValue(*org)
	state.UUID = types.StringValue(organization.Organization.Uuid)
	state.Enforce2FA = plan.Enforce2FA

	// The two optional settings are echoed from the plan rather than from the response: Terraform
	// requires an optional attribute to end an apply holding exactly what was configured, and an empty
	// allowlist comes back as null however it was written.
	state.AllowedApiIpAddresses = plan.AllowedApiIpAddresses
	state.RevokeInactiveTokensAfterDays = plan.RevokeInactiveTokensAfterDays
	state.RestrictUserApiTokenCreation = types.BoolValue(settings.RestrictUserAPITokenCreation)

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

	settings, err := o.client.GetOrganizationAPISettings(ctx, o.client.organization)
	if err != nil {
		resp.Diagnostics.AddError(organizationSettingsError("read", err))
		return
	}

	state.ID = types.StringValue(*org)
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

	org, err := o.client.GetOrganizationID()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to find organization",
			fmt.Sprintf("Unable to find Organization: %s", err.Error()),
		)
		return
	}
	log.Printf("Updating settings for organization %s ...", *org)

	settings, err := o.client.UpdateOrganizationAPISettings(ctx, o.client.organization, organizationAPISettingsPatchBody(&plan, &state))
	if err != nil {
		resp.Diagnostics.AddError(organizationSettingsError("update", err))
		return
	}

	// state still holds the prior value here, so an unchanged setting is left untouched.
	if isKnown(plan.Enforce2FA) && !plan.Enforce2FA.Equal(state.Enforce2FA) {
		twoFAResponse, err := setOrganization2FA(ctx, o.client.genqlient, *org, plan.Enforce2FA.ValueBool())
		if err != nil {
			resp.Diagnostics.AddError("Unable to set 2FA", err.Error())
			return
		}
		state.Enforce2FA = types.BoolValue(twoFAResponse.OrganizationEnforceTwoFactorAuthenticationForMembersUpdate.Organization.MembersRequireTwoFactorAuthentication)
	}

	state.AllowedApiIpAddresses = plan.AllowedApiIpAddresses
	state.RevokeInactiveTokensAfterDays = plan.RevokeInactiveTokensAfterDays
	state.RestrictUserApiTokenCreation = types.BoolValue(settings.RestrictUserAPITokenCreation)

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (o *organizationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state organizationResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("Deleting settings for organization %s ...", o.client.organization)

	// The settings themselves can't be deleted, so returning the ones this resource took over is
	// the closest thing to removing it.
	if body := organizationAPISettingsResetBody(&state); body != nil {
		if _, err := o.client.UpdateOrganizationAPISettings(ctx, o.client.organization, body); err != nil {
			resp.Diagnostics.AddError(organizationSettingsError("delete", err))
			return
		}
	}

	resp.Diagnostics.AddAttributeWarning(path.Root("enforce_2fa"), "Enforce 2FA setting left intact", "Use the web UI if you wish to change the value")
}
