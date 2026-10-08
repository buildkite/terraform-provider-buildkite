package buildkite

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type organizationMembershipResourceModel struct {
	ID                 types.String `tfsdk:"id"`
	Email              types.String `tfsdk:"email"`
	UUID               types.String `tfsdk:"uuid"`
	UserID             types.String `tfsdk:"user_id"`
	InvitationID       types.String `tfsdk:"invitation_id"`
	State              types.String `tfsdk:"state"`
	Role               types.String `tfsdk:"role"`
	SSOMode            types.String `tfsdk:"sso_mode"`
	SendInvitation     types.Bool   `tfsdk:"send_invitation"`
	DowngradeOnDestroy types.Bool   `tfsdk:"downgrade_on_destroy"`
}

type organizationMembershipResource struct{ client *Client }

const organizationMembershipImportPending = "importEmailReconciliation"

func newOrganizationMembershipResource() resource.Resource {
	return &organizationMembershipResource{}
}

func (r *organizationMembershipResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_membership"
}

func (r *organizationMembershipResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.client = req.ProviderData.(*Client)
	}
}

func (r *organizationMembershipResource) ConfigValidators(context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.AtLeastOneOf(path.MatchRoot("email"), path.MatchRoot("uuid")),
	}
}

func (r *organizationMembershipResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: heredoc.Doc(`
			Manages an organization's membership lifecycle using the REST and GraphQL APIs. Existing members and pending
			invitations are adopted. Set ` + "`send_invitation = true`" + ` to invite an absent user by email.
			Apply returns while the invitation is pending; a later refresh discovers acceptance.
			While an invitation is pending, refresh uses a targeted GraphQL email lookup to detect users who joined
			through SSO without accepting it, then confirms a match with the REST member's email. This avoids
			listing all organization members. Once a user UUID is known, or an invitation supplies accepted_by,
			refresh looks up that member directly through REST without an email lookup.

			Destroy removes active members by default, including adopted members. Removing a member can
			lose their team memberships and tokens; re-inviting them does not restore those. Set
			` + "`downgrade_on_destroy = true`" + ` to retain active members with role MEMBER and unchanged SSO mode.
			Pending invitations are always revoked on destroy. Apply a change to this flag before removing the resource.

			Do not let SCIM and Terraform manage the same membership lifecycle. An externally removed member
			remains tracked as pending if its tracked invitation is still pending, so destroy can revoke it;
			otherwise it becomes absent. An expired/revoked invitation also becomes absent when no member exists.
			A later apply can send a new invitation only when
			invitation sending is enabled; Terraform cannot accept it or immediately recreate an active membership.
			There is no invitation resend operation. Changing a pending invitation's role or SSO mode revokes
			and replaces it, sending a new email; this requires invitation sending to be enabled.
			Invitations with team assignments cannot be replaced because the API does not return their team roles;
			wait for acceptance before changing role or SSO mode. Manage teams separately with
			` + "`buildkite_team_member`" + `.

			The token needs read_user, read_organizations and write_organizations, and permission to manage the member.
			Email lookups additionally require the graphql scope. Lookup failures do not fall back to a member scan.
			Invitation operations additionally need read_organization_invitations and write_organization_invitations.
			Use a different organization administrator's token: this resource refuses to manage the token owner's
			membership, including import, refresh and destroy. Removing that member would revoke the running token.
			Each active-member lookup verifies the current token owner through GET /v2/user and fails closed if it cannot.
		`),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, MarkdownDescription: "Stable resource identifier, initially the member's user UUID or invitation UUID. Does not change on acceptance.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"email": schema.StringAttribute{
				Optional: true, Computed: true, MarkdownDescription: "Email address used to adopt or invite. Adoption by email requires the member's organization notification email, or primary email if no notification email is configured. A match only on another account email fails explicitly; use the organization email or UUID instead. Specify email or uuid. Required to invite an absent user. Retained after adoption even if the user's primary email changes. The first apply after import reconciles an active member's email selector, or a pending invitation's email casing, without replacement. Subsequent changes to a configured email replace the resource.",
				Validators: []validator.String{stringvalidator.LengthAtLeast(1)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplaceIf(
					func(ctx context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
						if req.ConfigValue.IsNull() {
							return
						}
						pending, diags := req.Private.GetKey(ctx, organizationMembershipImportPending)
						resp.Diagnostics.Append(diags...)
						var membershipState types.String
						resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("state"), &membershipState)...)
						// An imported UUID is authoritative. For pending invitations only
						// casing can be reconciled: a different address is a new invitee.
						resp.RequiresReplace = len(pending) == 0 || (membershipState.ValueString() != "active" && !strings.EqualFold(req.PlanValue.ValueString(), req.StateValue.ValueString()))
					},
					"Changing a configured email replaces the resource, except for initial import reconciliation.",
					"Changing a configured email replaces the resource, except for initial import reconciliation.",
				)},
			},
			"uuid": schema.StringAttribute{
				Optional: true, Computed: true, MarkdownDescription: "User UUID for an existing member. Null until the user is identified; retained if the member is later removed while a tracked invitation remains pending. Changing a configured UUID replaces the resource.",
				Validators:    []validator.String{stringvalidator.RegexMatches(importUuidRegex, "must be a user UUID")},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplaceIfConfigured(), stringplanmodifier.UseNonNullStateForUnknown()},
			},
			"user_id": schema.StringAttribute{
				Computed: true, MarkdownDescription: "GraphQL user ID for buildkite_team_member. Null until the user is identified, then retained. Membership state must be active before managing team memberships.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
			},
			"invitation_id": schema.StringAttribute{
				Computed: true, MarkdownDescription: "UUID of the tracked invitation, or null when an existing active membership was adopted.",
			},
			"state": schema.StringAttribute{
				Computed: true, MarkdownDescription: "Membership state: pending or active.",
			},
			"role": schema.StringAttribute{
				Required: true, MarkdownDescription: "Organization role: MEMBER or ADMIN.",
				Validators: []validator.String{stringvalidator.OneOf("MEMBER", "ADMIN")},
			},
			"sso_mode": schema.StringAttribute{
				Required: true, MarkdownDescription: "SSO requirement: REQUIRED or OPTIONAL.",
				Validators: []validator.String{stringvalidator.OneOf("REQUIRED", "OPTIONAL")},
			},
			"send_invitation": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				MarkdownDescription: "Allow sending invitations to absent users and replacing pending invitations when role or SSO mode changes. Defaults to false.",
			},
			"downgrade_on_destroy": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				MarkdownDescription: "Demote an active member to MEMBER instead of removing them on destroy, leaving SSO mode unchanged. Pending invitations are still revoked. Defaults to false.",
			},
		},
	}
}

func (r *organizationMembershipResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	pending, diags := req.Private.GetKey(ctx, organizationMembershipImportPending)
	resp.Diagnostics.Append(diags...)
	if len(pending) != 0 {
		// Ensure the first apply consumes import provenance even if config already
		// matches the API. Otherwise a later identity change could evade replacement.
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("state"), types.StringUnknown())...)
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("invitation_id"), types.StringUnknown())...)
		// A matching config did not trigger the framework's computed-unknown pass.
		// Leave room for invitation acceptance during this state-only update.
		for _, name := range []string{"uuid", "user_id"} {
			var value types.String
			resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root(name), &value)...)
			if value.IsNull() {
				resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(name), types.StringUnknown())...)
			}
		}
	}
}

// lookup follows a known user UUID rather than email once active, so an email change
// cannot make destroy target a different person. Accepted invitations give us that UUID.
func (r *organizationMembershipResource) lookup(ctx context.Context, state *organizationMembershipResourceModel) (member *organizationMembershipMember, invitation *organizationMembershipInvitation, err error) {
	// Absence is a lifecycle state, not an operational failure. All other API
	// errors must propagate so Read cannot discard state on a permissions outage.
	defer func() {
		if errors.Is(err, errOrganizationMembershipNotFound) {
			err = nil
		}
		// Check every resolved member, including imports, changed tokens and
		// acceptance races, before any caller can adopt or mutate membership.
		if err == nil && member != nil {
			err = r.client.checkOrganizationMembershipNotSelf(ctx, member.ID)
		}
	}()
	if id := state.UUID.ValueString(); id != "" {
		member, err := r.client.getOrganizationMembershipMember(ctx, id)
		if !errors.Is(err, errOrganizationMembershipNotFound) || state.InvitationID.ValueString() == "" {
			return member, nil, err
		}
		// A membership 404 is not absence while its tracked invitation is pending.
		// Keep that invitation for offboarding, but never retarget the known UUID.
		invitation, err := r.client.getOrganizationMembershipInvitation(ctx, state.InvitationID.ValueString())
		if err != nil {
			return nil, nil, err
		}
		if invitation.State == "pending" {
			return nil, invitation, nil
		}
		return nil, nil, nil
	}
	if id := state.InvitationID.ValueString(); id != "" {
		invitation, err := r.client.getOrganizationMembershipInvitation(ctx, id)
		if err != nil && !errors.Is(err, errOrganizationMembershipNotFound) {
			return nil, nil, err
		}
		if invitation != nil && invitation.AcceptedBy != nil && invitation.AcceptedBy.ID != "" {
			member, err := r.client.getOrganizationMembershipMember(ctx, invitation.AcceptedBy.ID)
			return member, nil, err
		}
		// SSO can create membership without accepting an invitation to a different
		// email on the same account. Pending alone does not prove the user is absent.
		member, err := r.client.findOrganizationMembershipMember(ctx, state.Email.ValueString())
		if !errors.Is(err, errOrganizationMembershipNotFound) {
			return member, nil, err
		}
		if invitation != nil && invitation.State == "pending" {
			return nil, invitation, nil
		}
		return nil, nil, nil
	}
	member, err = r.client.findOrganizationMembershipMember(ctx, state.Email.ValueString())
	if !errors.Is(err, errOrganizationMembershipNotFound) {
		return member, nil, err
	}
	invitation, err = r.client.findOrganizationMembershipInvitation(ctx, state.Email.ValueString())
	return nil, invitation, err
}

func (state *organizationMembershipResourceModel) fromMember(member *organizationMembershipMember) error {
	if member.SSOMode == nil {
		return fmt.Errorf("member response omitted sso_mode; use an organization administrator's token")
	}
	if state.ID.ValueString() == "" {
		state.ID = types.StringValue(member.ID)
	}
	// Email is the adoption/invitation selector, not a managed user profile field.
	// Replacing it with a primary email would plan a destructive replacement.
	if state.Email.ValueString() == "" {
		state.Email = types.StringValue(member.Email)
	}
	state.UUID = types.StringValue(member.ID)
	state.UserID = types.StringValue(base64.StdEncoding.EncodeToString([]byte("User---" + member.ID)))
	if state.InvitationID.IsUnknown() {
		state.InvitationID = types.StringNull()
	}
	state.State = types.StringValue("active")
	state.Role = types.StringValue(strings.ToUpper(member.Role))
	state.SSOMode = types.StringValue(strings.ToUpper(*member.SSOMode))
	return nil
}

func (state *organizationMembershipResourceModel) fromInvitation(invitation *organizationMembershipInvitation) {
	if state.ID.ValueString() == "" {
		state.ID = types.StringValue(invitation.ID)
	}
	if state.Email.ValueString() == "" {
		state.Email = types.StringValue(invitation.Email)
	}
	// Retain established identity if membership disappeared but the invitation
	// is still pending. Presence is reported by State, not by the user identifiers.
	if state.UUID.IsUnknown() {
		state.UUID = types.StringNull()
	}
	if state.UserID.IsUnknown() {
		state.UserID = types.StringNull()
	}
	state.InvitationID = types.StringValue(invitation.ID)
	state.State = types.StringValue("pending")
	state.Role = types.StringValue(strings.ToUpper(invitation.Role))
	state.SSOMode = types.StringValue(strings.ToUpper(invitation.SSOMode))
}

func (r *organizationMembershipResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var state organizationMembershipResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, diags := r.client.createTimeout(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := r.apply(requestCtx, &state); err != nil {
		resp.Diagnostics.AddError("Unable to manage organization membership", err.Error())
		// Recording a failed adoption would taint an existing member and make
		// Terraform delete that person before retrying. No new membership was created.
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *organizationMembershipResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state organizationMembershipResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, diags := r.client.readTimeout(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	member, invitation, err := r.lookup(requestCtx, &state)
	if err == nil {
		switch {
		case member != nil:
			err = state.fromMember(member)
		case invitation != nil:
			state.fromInvitation(invitation)
		default:
			resp.State.RemoveResource(ctx)
			return
		}
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to read organization membership", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *organizationMembershipResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state, plan organizationMembershipResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, diags := r.client.updateTimeout(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	plan.ID, plan.UUID, plan.InvitationID = state.ID, state.UUID, state.InvitationID
	if plan.Email.IsUnknown() {
		plan.Email = state.Email
	}
	if err := r.apply(requestCtx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to update organization membership", err.Error())
		// Preserve the last known state, not unknown computed values from the plan.
		// A revoked invitation is still tracked and the next refresh observes its absence.
		resp.State = req.State
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	pending, diags := req.Private.GetKey(ctx, organizationMembershipImportPending)
	resp.Diagnostics.Append(diags...)
	if !resp.Diagnostics.HasError() && len(pending) != 0 {
		resp.Diagnostics.Append(resp.Private.SetKey(ctx, organizationMembershipImportPending, nil)...)
	}
}

func (r *organizationMembershipResource) apply(ctx context.Context, state *organizationMembershipResourceModel) error {
	role, ssoMode := strings.ToLower(state.Role.ValueString()), strings.ToLower(state.SSOMode.ValueString())
	member, invitation, err := r.lookup(ctx, state)
	if err != nil {
		return err
	}
	if member != nil {
		// Refuse ambiguous identity before changing anyone's permissions.
		if email := state.Email.ValueString(); state.ID.ValueString() == "" && email != "" && !strings.EqualFold(email, member.Email) {
			return fmt.Errorf("email and user UUID do not identify the same organization member")
		}
		if err := state.fromMember(member); err != nil {
			return err
		}
		updated, err := r.client.updateOrganizationMembershipMember(ctx, member, role, ssoMode)
		if err != nil {
			return err
		}
		return state.fromMember(updated)
	}
	if invitation != nil {
		state.fromInvitation(invitation)
		if invitation.Role == role && invitation.SSOMode == ssoMode {
			return nil
		}
		if !state.SendInvitation.ValueBool() {
			return fmt.Errorf("changing a pending invitation requires send_invitation = true; the invitation must be revoked and replaced")
		}
		if len(invitation.Teams) != 0 {
			return fmt.Errorf("cannot replace an invitation with team assignments without losing those assignments; wait for acceptance before changing role or SSO mode")
		}
		err := r.client.makeRequest(ctx, http.MethodDelete, r.client.organizationMembershipPath("invitations", invitation.ID), nil, nil)
		if isAPIStatus(err, http.StatusUnprocessableEntity) {
			// Acceptance can race with revoke. Update that member, never delete it.
			member, _, lookupErr := r.lookup(ctx, state)
			if lookupErr != nil {
				return lookupErr
			}
			if member != nil {
				updated, updateErr := r.client.updateOrganizationMembershipMember(ctx, member, role, ssoMode)
				if updateErr != nil {
					return updateErr
				}
				return state.fromMember(updated)
			}
		}
		if err != nil && !isAPIStatus(err, http.StatusNotFound) {
			return err
		}
	}
	if !state.SendInvitation.ValueBool() || state.Email.ValueString() == "" {
		return fmt.Errorf("organization member not found; provision the user first, or supply email and set send_invitation = true")
	}
	// A UUID alone cannot become an unknown invitee. A tracked invitation may
	// still be replaced after external membership removal, using its original email.
	if state.UUID.ValueString() != "" && invitation == nil {
		return fmt.Errorf("user UUID is not an organization member; invite using email without a configured UUID")
	}
	email := state.Email.ValueString()
	if invitation != nil {
		email = invitation.Email
	}
	created, err := r.client.createOrganizationMembershipInvitation(ctx, email, role, ssoMode)
	if err != nil {
		return err
	}
	state.fromInvitation(created)
	return nil
}

func (r *organizationMembershipResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state organizationMembershipResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, diags := r.client.deleteTimeout(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := r.remove(requestCtx, &state); err != nil {
		resp.Diagnostics.AddError("Unable to delete organization membership", err.Error())
	}
}

func (r *organizationMembershipResource) remove(ctx context.Context, state *organizationMembershipResourceModel) error {
	member, invitation, err := r.lookup(ctx, state)
	if err != nil {
		return err
	}
	// SSO can leave the tracked invitation pending after membership is active.
	// Revoke it before removing/demoting the member so it cannot restore access.
	if id := state.InvitationID.ValueString(); invitation == nil && id != "" {
		var lookupErr error
		invitation, lookupErr = r.client.getOrganizationMembershipInvitation(ctx, id)
		if lookupErr != nil && !errors.Is(lookupErr, errOrganizationMembershipNotFound) {
			return lookupErr
		}
	}
	if invitation != nil && invitation.State == "pending" {
		err = r.client.makeRequest(ctx, http.MethodDelete, r.client.organizationMembershipPath("invitations", invitation.ID), nil, nil)
		if isAPIStatus(err, http.StatusUnprocessableEntity) {
			// Re-evaluate exactly once if acceptance/revocation raced with destroy.
			remaining, lookupErr := r.client.getOrganizationMembershipInvitation(ctx, invitation.ID)
			if lookupErr != nil && !errors.Is(lookupErr, errOrganizationMembershipNotFound) {
				return lookupErr
			}
			if remaining != nil && remaining.State == "pending" {
				return fmt.Errorf("invitation is still pending but the API refused to revoke it")
			}
			member, _, err = r.lookup(ctx, state)
			if err != nil {
				return err
			}
		}
		if err != nil && !isAPIStatus(err, http.StatusNotFound) {
			return err
		}
	}
	if member != nil {
		if state.DowngradeOnDestroy.ValueBool() {
			_, err = r.client.updateOrganizationMembershipMember(ctx, member, "member", "")
		} else {
			err = r.client.makeRequest(ctx, http.MethodDelete, r.client.organizationMembershipPath("members", member.ID), nil, nil)
		}
	}
	if isAPIStatus(err, http.StatusNotFound) {
		return nil
	}
	return err
}

func (r *organizationMembershipResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := strings.TrimPrefix(req.ID, "invitation/")
	if !importUuidRegex.MatchString(id) {
		resp.Diagnostics.AddError("Invalid membership import ID", "Use a member's user UUID, or invitation/<invitation UUID> for a pending invitation.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
	if strings.HasPrefix(req.ID, "invitation/") {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("invitation_id"), id)...)
		// Read needs the invitation's email when acceptance has not supplied a user UUID.
		invitation, err := r.client.getOrganizationMembershipInvitation(ctx, id)
		if err != nil || invitation == nil {
			resp.Diagnostics.AddError("Unable to import invitation", fmt.Sprintf("Invitation could not be read: %v", err))
			return
		}
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("email"), invitation.Email)...)
	} else {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("uuid"), id)...)
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("send_invitation"), false)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("downgrade_on_destroy"), false)...)
	resp.Diagnostics.Append(resp.Private.SetKey(ctx, organizationMembershipImportPending, []byte("true"))...)
}
