package buildkite

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/shurcooL/graphql"
)

type teamResource struct {
	client *Client
}

type teamResourceModel struct {
	ID                        types.String   `tfsdk:"id"`
	UUID                      types.String   `tfsdk:"uuid"`
	Name                      types.String   `tfsdk:"name"`
	Description               types.String   `tfsdk:"description"`
	Privacy                   TeamPrivacy    `tfsdk:"privacy"`
	IsDefaultTeam             types.Bool     `tfsdk:"default_team"`
	DefaultMemberRole         TeamMemberRole `tfsdk:"default_member_role"`
	Slug                      types.String   `tfsdk:"slug"`
	MembersCanCreatePipelines types.Bool     `tfsdk:"members_can_create_pipelines"`
}

// This is required due to the getTeam function not using Genqlient
type TeamNode struct {
	Description               graphql.String
	ID                        graphql.String
	IsDefaultTeam             graphql.Boolean
	DefaultMemberRole         graphql.String
	Name                      graphql.String
	MembersCanCreatePipelines graphql.Boolean
	Privacy                   TeamPrivacy
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
		MarkdownDescription: "A Cluster is a group of Agents that can be used to run your builds. " +
			"Clusters are useful for grouping Agents by their capabilities, such as operating system, hardware, or location. ",
		Attributes: map[string]resource_schema.Attribute{
			"id": resource_schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				MarkdownDescription: "The ID of the Team. This is a computed value and cannot be set.",
			},
			"uuid": resource_schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				MarkdownDescription: "The UUID of the Team. This is a computed value and cannot be set.",
			},
			"name": resource_schema.StringAttribute{
				MarkdownDescription: "The name of the Team.",
				Required:            true,
			},
			"description": resource_schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "A description for the Team. This is displayed in the Buildkite UI.",
			},
			"privacy": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The privacy setting for the Team. This can be either `VISIBLE` or `SECRET`.",
				Validators: []validator.String{
					stringvalidator.OneOf("VISIBLE", "SECRET"),
				},
			},
			"default_team": resource_schema.BoolAttribute{
				Required:            true,
				MarkdownDescription: "Whether this is the default Team for the Organization.",
			},
			"default_member_role": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The default role for new members of the Team. This can be either `MEMBER` or `MAINTAINER`.",
				Validators: []validator.String{
					stringvalidator.OneOf("MEMBER", "MAINTAINER"),
				},
			},
			"slug": resource_schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"members_can_create_pipelines": resource_schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Whether members of the Team can create Pipelines.",
			},
		},
	}
}

func (t *teamResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var state *teamResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	r, err := teamCreate(
		t.client.genqlient,
		t.client.organizationId,
		state.Name.ValueString(),
		*state.Description.ValueStringPointer(),
		state.Privacy,
		state.IsDefaultTeam.ValueBool(),
		state.DefaultMemberRole,
		state.MembersCanCreatePipelines.ValueBool(),
	)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create team.",
			fmt.Sprintf("Unable to create team: %s", err.Error()),
		)
	}

	state.ID = types.StringValue(r.TeamCreate.TeamEdge.Node.Id)
	state.UUID = types.StringValue(r.TeamCreate.TeamEdge.Node.Uuid)
	state.Slug = types.StringValue(r.TeamCreate.TeamEdge.Node.Slug)

	resp.Diagnostics.Append(resp.State.Set(ctx, *state)...)
}

func (t *teamResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state teamResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	res, err := getTeam(t.client.genqlient, state.ID.ValueString())

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read team.",
			fmt.Sprintf("Unable to read team: %s", err.Error()),
		)
		return
	}

	if converted, ok := res.GetNode().(*getTeamNodeTeam); ok {
		if converted == nil {
			resp.Diagnostics.AddError(
				"Unable to get team",
				"Error getting team: nil response",
			)
			return
		}
		updateTeamResourceState(&state, converted)
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	}
}

func (t *teamResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state, plan teamResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	_, err := teamUpdate(
		t.client.genqlient,
		state.ID.ValueString(),
		plan.Name.ValueString(),
		*plan.Description.ValueStringPointer(),
		plan.Privacy,
		plan.IsDefaultTeam.ValueBool(),
		plan.DefaultMemberRole,
		plan.MembersCanCreatePipelines.ValueBool(),
	)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to update Team",
			fmt.Sprintf("Unable to update Team: %s", err.Error()),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (t *teamResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state teamResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	_, err := teamDelete(t.client.genqlient, state.ID.ValueString())

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete Team",
			fmt.Sprintf("Unable to delete Team: %s", err.Error()),
		)
		return
	}
}

func updateTeamResourceState(state *teamResourceModel, res *getTeamNodeTeam) {
	state.ID = types.StringValue(res.Id)
	state.UUID = types.StringValue(res.Uuid)
	state.Slug = types.StringValue(res.Slug)
	state.Name = types.StringValue(res.Name)
	state.Privacy = res.Privacy
	state.Description = types.StringValue(res.Description)
	state.IsDefaultTeam = types.BoolValue(res.IsDefaultTeam)
	state.DefaultMemberRole = res.DefaultMemberRole
	state.MembersCanCreatePipelines = types.BoolValue(res.MembersCanCreatePipelines)
}
