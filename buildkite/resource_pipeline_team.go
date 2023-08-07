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

type pipelineTeamResourceModel struct {
	Id          types.String `tfsdk:"id"`
	Uuid        types.String `tfsdk:"uuid"`
	PipelineId  types.String `tfsdk:"pipeline_id"`
	TeamId      types.String `tfsdk:"team_id"`
	AccessLevel types.String `tfsdk:"access_level"`
}

type pipelineTeamResource struct {
	client *Client
}

func newPipelineTeamResource() resource.Resource {
	return &pipelineTeamResource{}
}

func (pipelineTeamResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pipeline_team"
}

func (tp *pipelineTeamResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	tp.client = req.ProviderData.(*Client)
}

func (pipelineTeamResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
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

func (tp *pipelineTeamResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var state pipelineTeamResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("Creating team pipeline into team %s ...", state.TeamId.ValueString())
	apiResponse, err := createTeamPipeline(
		tp.client.genqlient,
		state.TeamId.ValueString(),
		state.PipelineId.ValueString(),
		PipelineAccessLevels(state.AccessLevel.ValueString()),
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
	state.AccessLevel = types.StringValue(string(apiResponse.TeamPipelineCreate.TeamPipelineEdge.Node.PipelineAccessLevel))

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (tp *pipelineTeamResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pipelineTeamResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("Reading Team pipeline id=%s ...", state.Id.ValueString())
	apiResponse, err := getNode(
		tp.client.genqlient,
		state.Id.ValueString(),
	)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read team pipeline",
			fmt.Sprintf("Unable to read team pipeline: %s", err.Error()),
		)
	}

	// Convert from Node to getNodeNodeTeamPipeline type
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
	} else {
		// Resource not found, remove from state
		resp.Diagnostics.AddWarning("Team pipeline resource not found", "Removing team pipeline from state")
		resp.State.RemoveResource(ctx)
	}

}

func (tp *pipelineTeamResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (tp *pipelineTeamResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state pipelineTeamResourceModel
	var accessLevel string

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("access_level"), &accessLevel)...)

	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("Updating Team pipeline id=%s ...", state.Id.ValueString())
	_, err := updateTeamPipeline(tp.client.genqlient, state.Id.ValueString(), PipelineAccessLevels(accessLevel))
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to update Team pipeline",
			fmt.Sprintf("Unable to update Team pipeline: %s", err.Error()),
		)
		return
	}

	state.AccessLevel = types.StringValue(accessLevel)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (tp *pipelineTeamResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pipelineTeamResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	log.Println("Deleting Team pipeline ...")
	_, err := deleteTeamPipeline(tp.client.genqlient, state.Id.ValueString())

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete team pipeline",
			fmt.Sprintf("Unable to delete team pipeline: %s", err.Error()),
		)
		return
	}
}

func updateTeamPipelineResourceState(tpState *pipelineTeamResourceModel, tpNode getNodeNodeTeamPipeline) {
	tpState.Id = types.StringValue(tpNode.Id)
	tpState.Uuid = types.StringValue(tpNode.Uuid)
	tpState.TeamId = types.StringValue(tpNode.Team.Id)
	tpState.PipelineId = types.StringValue(tpNode.Pipeline.Id)
	tpState.AccessLevel = types.StringValue(string(tpNode.PipelineAccessLevel))
}
