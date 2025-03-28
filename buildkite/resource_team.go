package buildkite

import (
	"context"
	"fmt"

	custom_modifier "github.com/buildkite/terraform-provider-buildkite/internal/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/shurcooL/graphql"
)

type teamResource struct {
	client *Client
}

type teamResourceModel struct {
	ID                        types.String `tfsdk:"id"`
	UUID                      types.String `tfsdk:"uuid"`
	Name                      types.String `tfsdk:"name"`
	Description               types.String `tfsdk:"description"`
	Privacy                   types.String `tfsdk:"privacy"`
	IsDefaultTeam             types.Bool   `tfsdk:"default_team"`
	DefaultMemberRole         types.String `tfsdk:"default_member_role"`
	Slug                      types.String `tfsdk:"slug"`
	MembersCanCreatePipelines types.Bool   `tfsdk:"members_can_create_pipelines"`
}

// This is required due to the getTeam function not using Genqlient
type TeamNode struct {
	Description               graphql.String
	ID                        graphql.String
	IsDefaultTeam             graphql.Boolean
	DefaultMemberRole         graphql.String
	Name                      graphql.String
	MembersCanCreatePipelines graphql.Boolean
	Privacy                   graphql.String
	Slug                      graphql.String
	UUID                      graphql.String
}

func newTeamResource() resource.Resource {
	return &teamResource{}
}

func (t *teamResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_team"
}

func (t *teamResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	t.client = req.ProviderData.(*Client)
}

func (t *teamResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resource_schema.Schema{
		MarkdownDescription: "A Team is a group of users that can be given permissions for using Pipelines." +
			"This feature is only available to Business and Enterprise customers.  You can find out more about Teams in the Buildkite [documentation](https://buildkite.com/docs/team-management/permissions).",
		Attributes: map[string]resource_schema.Attribute{
			"id": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The GraphQL ID of the team.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"uuid": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the team.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": resource_schema.StringAttribute{
				MarkdownDescription: "The name of the team.",
				Required:            true,
			},
			"description": resource_schema.StringAttribute{
				Computed:            true,
				Default:             stringdefault.StaticString(""),
				Optional:            true,
				MarkdownDescription: "A description for the team. This is displayed in the Buildkite UI.",
			},
			"privacy": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The privacy setting for the team. This can be either `VISIBLE` or `SECRET`.",
				Validators: []validator.String{
					stringvalidator.OneOf("VISIBLE", "SECRET"),
				},
			},
			"default_team": resource_schema.BoolAttribute{
				Required:            true,
				MarkdownDescription: "Whether this is the default team for the organization.",
			},
			"default_member_role": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The default role for new members of the team. This can be either `MEMBER` or `MAINTAINER`.",
				Validators: []validator.String{
					stringvalidator.OneOf("MEMBER", "MAINTAINER"),
				},
			},
			"slug": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The generated slug for the team.",
				PlanModifiers: []planmodifier.String{
					custom_modifier.UseStateIfUnchanged("name"),
				},
			},
			"members_can_create_pipelines": resource_schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Whether members of the team can create Pipelines.",
			},
		},
	}
}

func (t *teamResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (t *teamResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var state teamResourceModel

	diags := req.Plan.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := t.client.timeouts.Create(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var r *teamCreateResponse
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		org, err := t.client.GetOrganizationID()
		if err == nil {
			r, err = teamCreate(ctx,
				t.client.genqlient,
				*org,
				state.Name.ValueString(),
				state.Description.ValueString(),
				state.Privacy.ValueString(),
				state.IsDefaultTeam.ValueBool(),
				state.DefaultMemberRole.ValueString(),
				state.MembersCanCreatePipelines.ValueBool(),
			)
		}

		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create team.",
			fmt.Sprintf("Unable to create team: %s", err.Error()),
		)
		return
	}

	state.ID = types.StringValue(r.TeamCreate.TeamEdge.Node.Id)
	state.UUID = types.StringValue(r.TeamCreate.TeamEdge.Node.Uuid)
	state.Slug = types.StringValue(r.TeamCreate.TeamEdge.Node.Slug)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (t *teamResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state teamResourceModel

	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := t.client.timeouts.Read(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var response *getNodeResponse
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var err error
		response, err = getNode(ctx,
			t.client.genqlient,
			state.ID.ValueString(),
		)

		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read team.",
			fmt.Sprintf("Unable to read team: %s", err.Error()),
		)
		return
	}

	if teamNode, ok := response.GetNode().(*getNodeNodeTeam); ok {
		if teamNode == nil {
			resp.Diagnostics.AddError(
				"Unable to get team",
				"Error getting team: nil response",
			)
			return
		}
		updateTeamResourceState(&state, *teamNode)
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	} else {
		// Resource not found, remove from state
		resp.Diagnostics.AddWarning("Team resource not found", "Removing team from state")
		resp.State.RemoveResource(ctx)
	}
}

func (t *teamResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state, plan teamResourceModel
	diagsState := req.State.Get(ctx, &state)
	diagsPlan := req.Plan.Get(ctx, &plan)

	resp.Diagnostics.Append(diagsPlan...)
	resp.Diagnostics.Append(diagsState...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := t.client.timeouts.Update(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var response *teamUpdateResponse
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var err error
		response, err = teamUpdate(ctx,
			t.client.genqlient,
			state.ID.ValueString(),
			plan.Name.ValueString(),
			plan.Description.ValueString(),
			plan.Privacy.ValueString(),
			plan.IsDefaultTeam.ValueBool(),
			plan.DefaultMemberRole.ValueString(),
			plan.MembersCanCreatePipelines.ValueBool(),
		)

		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to update Team",
			fmt.Sprintf("Unable to update Team: %s", err.Error()),
		)
		return
	}

	plan.Slug = types.StringValue(response.TeamUpdate.Team.Slug)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (t *teamResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state teamResourceModel

	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := t.client.timeouts.Delete(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		_, err := teamDelete(ctx,
			t.client.genqlient,
			state.ID.ValueString(),
		)

		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete Team",
			fmt.Sprintf("Unable to delete Team: %s", err.Error()),
		)
		return
	}
}

func updateTeamResourceState(state *teamResourceModel, res getNodeNodeTeam) {
	state.ID = types.StringValue(res.Id)
	state.UUID = types.StringValue(res.Uuid)
	state.Slug = types.StringValue(res.Slug)
	state.Name = types.StringValue(res.Name)
	state.Privacy = types.StringValue(string(res.GetPrivacy()))
	state.Description = types.StringValue(res.Description)
	state.IsDefaultTeam = types.BoolValue(res.IsDefaultTeam)
	state.DefaultMemberRole = types.StringValue(string(res.GetDefaultMemberRole()))
	state.MembersCanCreatePipelines = types.BoolValue(res.MembersCanCreatePipelines)
}
