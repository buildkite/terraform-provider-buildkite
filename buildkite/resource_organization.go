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
				Computed: true,
				Default:  booldefault.StaticBool(false),
				MarkdownDescription: "Sets whether only organization administrators can create API access tokens that act on behalf of the organization. " +
					"Defaults to false.\n\n" +
					"~> The default applies whether or not you set the attribute, so leaving it out of your configuration " +
					"lifts the restriction from an organization that had it enabled.",
			},
		},
	}
}

// organizationAPISettingsPatchBody builds the PATCH body for the settings served by the REST API. The
// API refuses a setting the billing plan does not cover on the key being present at all, so a key is
// named only when the plan asks for it, or when state holds a value that needs clearing. An allowlist
// state already records as empty is neither. state is nil on create.
func organizationAPISettingsPatchBody(plan, state *organizationResourceModel) map[string]any {
	body := make(map[string]any)

	if isKnown(plan.AllowedApiIpAddresses) {
		body["allowed_ip_addresses"] = allowedIPAddressesFromList(plan.AllowedApiIpAddresses)
	} else if state != nil && allowedIPAddressesFromList(state.AllowedApiIpAddresses) != nil {
		body["allowed_ip_addresses"] = nil
	}

	if isKnown(plan.RevokeInactiveTokensAfterDays) {
		body["revoke_inactive_tokens_after_days"] = plan.RevokeInactiveTokensAfterDays.ValueInt64()
	} else if state != nil && isKnown(state.RevokeInactiveTokensAfterDays) {
		body["revoke_inactive_tokens_after_days"] = nil
	}

	// Ungated and defaulted, so it is always both safe to send and known.
	body["restrict_user_api_token_creation"] = plan.RestrictUserApiTokenCreation.ValueBool()

	return body
}

// organizationAPISettingsResetBody returns the settings this resource had taken over to their
// defaults, and nil when it had taken over none of them.
func organizationAPISettingsResetBody(state *organizationResourceModel) map[string]any {
	body := make(map[string]any)

	if allowedIPAddressesFromList(state.AllowedApiIpAddresses) != nil {
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

	state.RestrictUserApiTokenCreation = types.BoolValue(settings.RestrictUserAPITokenCreation)

	return diags
}

// organizationSettingsError reports a REST failure. A 403 leads with the API's own words, which name
// the entitlement or scope that is missing, and adds the three things that produce one.
func organizationSettingsError(action string, err error) (string, string) {
	summary := fmt.Sprintf("Unable to %s Organization settings", action)

	if isAPIStatus(err, http.StatusForbidden) {
		return summary, fmt.Sprintf("%s\n\nThe API refuses this request when the token is missing the "+
			"read_organization_settings or write_organization_settings scope, when its user is not an organization "+
			"administrator, or when the organization's billing plan does not cover the setting being changed.\n\n%s",
			apiErrorMessage(err), err.Error())
	}

	return summary, fmt.Sprintf("%s: %s", summary, err.Error())
}

func (o *organizationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, state organizationResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("Creating settings for organization %s ...", o.client.organization)

	// Read before writing, so a failure looking the organization up can't leave settings changed with
	// nothing recorded in state. This also supplies the ID and UUID the removed GraphQL mutation used
	// to return, and the 2FA setting to compare the plan against.
	organization, err := getOrganization(ctx, o.client.genqlient, o.client.organization)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to obtain Organization",
			fmt.Sprintf("Unable to obtain Organization: %s", err.Error()),
		)
		return
	}
	if organization.Organization.Id == "" {
		resp.Diagnostics.AddError(
			"Unable to find organization",
			fmt.Sprintf("Could not find organization with slug %q", o.client.organization),
		)
		return
	}

	if _, err := o.client.UpdateOrganizationAPISettings(ctx, o.client.organization, organizationAPISettingsPatchBody(&plan, nil)); err != nil {
		resp.Diagnostics.AddError(organizationSettingsError("create", err))
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

	// Comparing against the organization keeps a create whose 2FA setting already matches from writing
	// to it at all.
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

	log.Printf("Reading settings for organization ...")
	response, err := getOrganization(ctx, o.client.genqlient, o.client.organization)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to obtain Organization",
			fmt.Sprintf("Unable to obtain Organization: %s", err.Error()),
		)
		return
	}
	if response.Organization.Id == "" {
		resp.Diagnostics.AddError(
			"Unable to find organization",
			fmt.Sprintf("Could not find organization with slug %q", o.client.organization),
		)
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

	org, err := o.client.GetOrganizationID()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to find organization",
			fmt.Sprintf("Unable to find Organization: %s", err.Error()),
		)
		return
	}
	log.Printf("Updating settings for organization %s ...", *org)

	// state holds the prior values, which is what tells the request apart from the settings it must
	// leave unnamed.
	if _, err := o.client.UpdateOrganizationAPISettings(ctx, o.client.organization, organizationAPISettingsPatchBody(&plan, &state)); err != nil {
		resp.Diagnostics.AddError(organizationSettingsError("update", err))
		return
	}

	state.AllowedApiIpAddresses = plan.AllowedApiIpAddresses
	state.RevokeInactiveTokensAfterDays = plan.RevokeInactiveTokensAfterDays
	state.RestrictUserApiTokenCreation = plan.RestrictUserApiTokenCreation

	if isKnown(plan.Enforce2FA) {
		twoFAResponse, err := setOrganization2FA(ctx, o.client.genqlient, *org, plan.Enforce2FA.ValueBool())
		if err != nil {
			resp.Diagnostics.AddError("Unable to set 2FA", err.Error())
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

	// The settings themselves can't be deleted, so returning the ones this resource took over is
	// the closest thing to removing it.
	if body := organizationAPISettingsResetBody(&state); body != nil {
		if _, err := o.client.UpdateOrganizationAPISettings(ctx, o.client.organization, body); err != nil {
			// A refusal here would otherwise leave the resource undestroyable, which an organization
			// downgraded off a plan that once covered one of these settings would hit on every attempt.
			if !isAPIStatus(err, http.StatusForbidden) {
				resp.Diagnostics.AddError(organizationSettingsError("delete", err))
				return
			}

			resp.Diagnostics.AddWarning(
				"Organization API settings left as they are",
				fmt.Sprintf("Removing this resource could not return the organization's API settings to their "+
					"defaults, so they keep the values Terraform last applied and have to be changed in the web "+
					"UI.\n\n%s", apiErrorMessage(err)),
			)
		}
	}

	resp.Diagnostics.AddAttributeWarning(path.Root("enforce_2fa"), "Enforce 2FA setting left intact", "Use the web UI if you wish to change the value")
}
