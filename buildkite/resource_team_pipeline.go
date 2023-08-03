package buildkite

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type teamPipelineResourceModel struct {
	Id          types.String `tfsdk:"id"`
	Uuid        types.String `tfsdk:"uuid"`
	PipelineId  types.String `tfsdk:"pipeline_id"`
	TeamId      types.String `tfsdk:"team_id"`
	AccessLevel types.String `tfsdk:"access_level"`
}

type teamPipelineResource struct {
	client *Client
}

func newteamPipelineResource() resource.Resource {
	return &teamPipelineResource{}
}

func (teamPipelineResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_team_pipeline"
}

func (tp *teamPipelineResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	tp.client = req.ProviderData.(*Client)
}

func (teamPipelineResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resource_schema.Schema{
		MarkdownDescription: "A team pipeline resource sets a team's access for the pipeline.",
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
			"pipeline_id": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The GraphQL ID of the pipeline.",
			},
			"access_level": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The access level for the team. Either READ_ONLY, BUILD_AND_READ or MANAGE_BUILD_AND_READ.",
				Validators: []validator.String{
					stringvalidator.OneOf("READ_ONLY", "BUILD_AND_READ", "MANAGE_BUILD_AND_READ"),
				},
			},
		},
	}
}

func (tp *teamPipelineResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var state teamPipelineResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("Creating team pipeline into team %s ...", state.TeamId.ValueString())
	apiResponse, err := createTeamPipeline(
		tp.client.genqlient,
		state.TeamId.ValueString(),
		state.PipelineId.ValueString(),
		parseAccessLevelString(state.AccessLevel.ValueString()),
	)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create team pipeline",
			fmt.Sprintf("Unable to create team pipeline: %s", err.Error()),
		)
		return
	}

	// Update state with values from API response/plan
	state.Id = types.StringValue(apiResponse.TeamPipelineCreate.TeamPipelineEdge.Node.Id)
	state.Uuid = types.StringValue(apiResponse.TeamPipelineCreate.TeamPipelineEdge.Node.Uuid)
	state.PipelineId = types.StringValue(apiResponse.TeamPipelineCreate.TeamPipelineEdge.Node.Pipeline.Id)
	state.TeamId = types.StringValue(apiResponse.TeamPipelineCreate.TeamPipelineEdge.Node.Team.Id)
	state.AccessLevel = types.StringValue(string(apiResponse.TeamPipelineCreate.TeamPipelineEdge.Node.AccessLevel))

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (tp *teamPipelineResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state teamPipelineResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	apiResponse, err := getNode(tp.client.genqlient, state.Id.ValueString())

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read team pipeline",
			fmt.Sprintf("Unable to read team pipeline: %s", err.Error()),
		)
		return
	}

	// Convert fron Node to getNodeNodeTeamPipeline type
	if teamPipelineNode, ok := apiResponse.GetNode().(*getNodeNodeTeamPipeline); ok {
		if teamPipelineNode == nil {
			resp.Diagnostics.AddError(
				"Unable to get team pipeline",
				"Error getting team pipeline: nil response",
			)
			return
		}
		updateTeamPipelineResourceState(&state, *teamPipelineNode)
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	}
}

func (tp *teamPipelineResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (tp *teamPipelineResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var id, accessLevel string

	// Obtain team pipeline's ID from state, new role from plan
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &id)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("access_level"), &accessLevel)...)

	if resp.Diagnostics.HasError() {
		return
	}

	apiResponse, err := updateTeamPipeline(tp.client.genqlient, id, parseAccessLevelString(accessLevel))
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to update Team pipeline",
			fmt.Sprintf("Unable to update Team pipeline: %s", err.Error()),
		)
		return
	}

	// Update state with revised access level
	newAccessLevel := types.StringValue(string(apiResponse.TeamPipelineUpdate.TeamPipeline.AccessLevel))
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("access_level"), newAccessLevel)...)
}

func (tp *teamPipelineResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state teamPipelineResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	_, err := deleteTeamPipeline(tp.client.genqlient, state.Id.ValueString())

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete team pipeline",
			fmt.Sprintf("Unable to delete team pipeline: %s", err.Error()),
		)
		return
	}
}

func updateTeamPipelineResourceState(tpState *teamPipelineResourceModel, tpNode getNodeNodeTeamPipeline) {
	tpState.Id = types.StringValue(tpNode.Id)
	tpState.Uuid = types.StringValue(tpNode.Uuid)
	tpState.TeamId = types.StringValue(tpNode.Team.Id)
	tpState.PipelineId = types.StringValue(tpNode.Pipeline.Id)
	tpState.AccessLevel = types.StringValue(string(tpNode.AccessLevel))
}

func parseAccessLevelString(str string) PipelineAccessLevels {
	switch str {
	case "READ_ONLY":
		return PipelineAccessLevelsReadOnly
	case "BUILD_AND_READ":
		return PipelineAccessLevelsBuildAndRead
	case "MANAGE_BUILD_AND_READ":
		return PipelineAccessLevelsManageBuildAndRead
	default:
		return PipelineAccessLevelsReadOnly
	}
}
