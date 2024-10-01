package buildkite

import (
	"context"
	"fmt"
	"log"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

type organizationMemberResourceModel struct {
	ID            types.String `tfsdk:"id"`
	UUID          types.String `tfsdk:"uuid"`
	Role          types.String `tfsdk:"role"`
	Email         types.String `tfsdk:"email"`
	Complimentary types.Bool   `tfsdk:"complimentary"`
	SSO           types.String `tfsdk:"sso"`
}

type organizationMemberResource struct {
	client *Client
}

func newOrganizationMemberResource() resource.Resource {
	return &organizationMemberResource{}
}

func (organizationMemberResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_member"
}

func (om *organizationMemberResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	om.client = req.ProviderData.(*Client)
}

func (organizationMemberResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resource_schema.Schema{
		MarkdownDescription: heredoc.Doc(`
		This resource allows you to manage organization membership for specific Buildkite accounts, and maintain specific membership settings.

		More information on organization membership can be found in the [documentation](https://buildkite.com/docs/pipelines/create-your-own#next-steps) and
		additionally on the organization members [GraphQL cookbook](https://buildkite.com/docs/apis/graphql/cookbooks/organizations).
	`),
		Attributes: map[string]resource_schema.Attribute{
			"id": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The GraphQL ID of the organization member.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"uuid": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the organization member.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"role": resource_schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The role of the organization member. Either `ADMIN` or `MEMBER`. ",
			},
			"email": resource_schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "The email associated with the organization member. ",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"complimentary": resource_schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether this organization member is complimentary. ",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"sso": resource_schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The SSO mode of the organization member. Either `REQUIRED` or `OPTIONAL`. ",
			},
		},
	}
}

func (om *organizationMemberResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	resp.Diagnostics.AddError("Cannot create an organization member", "Organization membership is created via accepted invitations/signed in users via SCIM provisioning.")
}

func (om *organizationMemberResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state organizationMemberResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeouts, diags := om.client.timeouts.Read(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var apiResponse *getNodeResponse
	err := retry.RetryContext(ctx, timeouts, func() *retry.RetryError {
		var err error

		log.Printf("Reading organization member with ID %s ...", state.UUID.ValueString())
		apiResponse, err = getNode(ctx, om.client.genqlient, state.ID.ValueString())

		return retryContextError(err)
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read organization member",
			fmt.Sprintf("Unable to read organmization member: %s", err.Error()),
		)
		return
	}

	// Convert fron Node to getNodeNodeOrganizationMember type
	if organizationMember, ok := apiResponse.GetNode().(*getNodeNodeOrganizationMember); ok {
		if organizationMember == nil {
			resp.Diagnostics.AddError(
				"Unable to get organization member",
				"Error getting organization member: nil response",
			)
			return
		}

		// Update organization member model and set in state
		updateOrganizatonMemberResourceRead(&state, *organizationMember)
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	} else {
		// Remove from state if not found
		resp.Diagnostics.AddWarning("Organization member not found", "Removing from state")
		resp.State.RemoveResource(ctx)
		return
	}
}

func (om *organizationMemberResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (om *organizationMemberResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state, plan organizationMemberResourceModel

	diagsState := req.State.Get(ctx, &state)
	diagsPlan := req.Plan.Get(ctx, &plan)

	resp.Diagnostics.Append(diagsState...)
	resp.Diagnostics.Append(diagsPlan...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeouts, diags := om.client.timeouts.Update(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var r *updateOrganiztionMemberResponse
	err := retry.RetryContext(ctx, timeouts, func() *retry.RetryError {
		var err error

		r, err = updateOrganiztionMember(ctx,
			om.client.genqlient,
			state.ID.ValueString(),
			OrganizationMemberRole(plan.Role.ValueString()),
			OrganizationMemberSSOModeEnum(plan.SSO.ValueString()),
		)

		return retryContextError(err)
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to update organization member",
			fmt.Sprintf("Unable to update organization member: %s", err.Error()),
		)
	}

	state.Role = types.StringValue(string(r.OrganizationMemberUpdate.OrganizationMember.MemberRole))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (om *organizationMemberResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state organizationMemberResourceModel

	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := om.client.timeouts.Delete(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		_, err := deleteOrganizationMember(ctx, om.client.genqlient, state.ID.ValueString())

		return retryContextError(err)
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete organization member",
			fmt.Sprintf("Unable to delete organization member: %s", err.Error()),
		)
		return
	}
}

func updateOrganizatonMemberResourceRead(om *organizationMemberResourceModel, omn getNodeNodeOrganizationMember) {
	om.ID = types.StringValue(omn.Id)
	om.UUID = types.StringValue(omn.MemberUuid)
	om.Role = types.StringValue(string(omn.MemberRole))
	om.Email = types.StringValue(omn.MemberUser.Email)
	om.Complimentary = types.BoolValue(omn.Complimentary)
	om.SSO = types.StringPointerValue((*string)(&omn.Sso.Mode))
}
