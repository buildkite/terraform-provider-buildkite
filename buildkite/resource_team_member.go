package buildkite

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

type teamMemberResourceModel struct {
	Id       types.String   `tfsdk:"id"`
	Uuid     types.String   `tfsdk:"uuid"`
	Role     types.String   `tfsdk:"role"`
	TeamId   types.String   `tfsdk:"team_id"`
	UserId   types.String   `tfsdk:"user_id"`
	Timeouts timeouts.Value `tfsdk:"timeouts"`
}

type teamMemberResource struct {
	client *Client
}

func newTeamMemberResource() resource.Resource {
	return &teamMemberResource{}
}

func (teamMemberResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_team_member"
}

func (tm *teamMemberResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	tm.client = req.ProviderData.(*Client)
}

func (teamMemberResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resource_schema.Schema{
		MarkdownDescription: "A team member resource allows for the management of team membership for existing organization users.",
		Attributes: map[string]resource_schema.Attribute{
			"id": resource_schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"uuid": resource_schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"team_id": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The GraphQL ID of the team.",
			},
			"user_id": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The GraphQL ID of the user.",
			},
			"role": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The role for the user. Either MEMBER or MAINTAINER.",
				Validators: []validator.String{
					stringvalidator.OneOf("MEMBER", "MAINTAINER"),
				},
			},
		},
		Blocks: map[string]resource_schema.Block{
			"timeouts": timeouts.BlockAll(ctx),
		},
	}
}

func (tm *teamMemberResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var state teamMemberResourceModel

	diags := req.Plan.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := state.Timeouts.Create(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	log.Printf("Creating team member into team %s ...", state.TeamId.ValueString())
	var r *createTeamMemberResponse
	retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var err error
		r, err = createTeamMember(ctx,
			tm.client.genqlient,
			state.TeamId.ValueString(),
			state.UserId.ValueString(),
			*state.Role.ValueStringPointer(),
		)

		if err != nil {
			if isRetryableError(err) {
				return retry.RetryableError(err)
			}
			resp.Diagnostics.AddError(
				"Unable to create team member",
				fmt.Sprintf("Unable to create team member: %s", err.Error()),
			)
			return retry.NonRetryableError(err)
		}

		return nil
	})

	// Update state with values from API response/plan
	state.Id = types.StringValue(r.TeamMemberCreate.TeamMemberEdge.Node.Id)
	state.Uuid = types.StringValue(r.TeamMemberCreate.TeamMemberEdge.Node.Uuid)
	// The role of the user will by default be "MEMBER" if none was entered in the plan
	state.Role = types.StringValue(string(r.TeamMemberCreate.TeamMemberEdge.Node.Role))

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (tm *teamMemberResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state teamMemberResourceModel

	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := state.Timeouts.Create(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	log.Printf("Reading team member %s ...", state.Id.ValueString())
	var r *getNodeResponse
	retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var err error
		r, err = getNode(ctx, 
			tm.client.genqlient, 
			state.Id.ValueString(),
		)

		if err != nil {
			if isRetryableError(err) {
				return retry.RetryableError(err)
			}
			resp.Diagnostics.AddError(
				"Unable to read team member",
				fmt.Sprintf("Unable to read ream member: %s", err.Error()),
			)
			return retry.NonRetryableError(err)
		}

		return nil
	})

	// Convert fron Node to getNodeTeamMember type
	if teamMemberNode, ok := r.GetNode().(*getNodeNodeTeamMember); ok {
		if teamMemberNode == nil {
			resp.Diagnostics.AddError(
				"Unable to get team member",
				"Error getting team member: nil response",
			)
			return
		}
		updateTeamMemberResourceState(&state, *teamMemberNode)
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	}
}

func (tm *teamMemberResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (tm *teamMemberResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state, plan teamMemberResourceModel

	// Obtain team member's ID from state, new role from plan
	diagsState := req.State.Get(ctx, &state)
	diagsPlan := req.Plan.Get(ctx, &plan)

	resp.Diagnostics.Append(diagsPlan...)
	resp.Diagnostics.Append(diagsState...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := state.Timeouts.Create(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	log.Printf("Updating team member %s with role %s ...", state.Id.ValueString(), plan.Role.ValueString())
	var r *updateTeamMemberResponse
	retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var err error
		r, err = updateTeamMember(ctx, 
			tm.client.genqlient, 
			state.Id.ValueString(), 
			*plan.Role.ValueStringPointer(),
		)

		if err != nil {
			if isRetryableError(err) {
				return retry.RetryableError(err)
			}
			resp.Diagnostics.AddError(
				"Unable to update Team member",
				fmt.Sprintf("Unable to update Team member: %s", err.Error()),
			)
			return retry.NonRetryableError(err)
		}

		return nil
	})

	// Update state with revised role
	newRole := types.StringValue(string(r.TeamMemberUpdate.TeamMember.Role))
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("role"), newRole)...)
}

func (tm *teamMemberResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state teamMemberResourceModel

	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := state.Timeouts.Create(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	log.Printf("Deleting team member with ID %s ...", state.Id.ValueString())
	retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		_, err := deleteTeamMember(ctx, 
			tm.client.genqlient, 
			state.Id.ValueString(),
		)

		if err != nil {
			if isRetryableError(err) {
				return retry.RetryableError(err)
			}
			resp.Diagnostics.AddError(
				"Unable to delete team member",
				fmt.Sprintf("Unable to delete team member: %s", err.Error()),
			)
			return retry.NonRetryableError(err)
		}
		return nil
	})
}

func updateTeamMemberResourceState(tmr *teamMemberResourceModel, tmn getNodeNodeTeamMember) {
	tmr.Id = types.StringValue(tmn.Id)
	tmr.Uuid = types.StringValue(tmn.Uuid)
	tmr.TeamId = types.StringValue(tmn.Team.Id)
	tmr.UserId = types.StringValue(tmn.User.Id)
	tmr.Role = types.StringValue(string(tmn.Role))
}
