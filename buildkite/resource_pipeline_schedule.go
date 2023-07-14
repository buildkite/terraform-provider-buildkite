package buildkite

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type PipelineSchedule struct {
	client *Client
}

type PipelineScheduleResourceModel struct {
	Id         types.String `tfsdk:"id"`
	Label      types.String `tfsdk:"label"`
	Cronline   types.String `tfsdk:"cronline"`
	Commit     types.String `tfsdk:"commit"`
	Branch     types.String `tfsdk:"branch"`
	Message    types.String `tfsdk:"message"`
	Env        types.String `tfsdk:"env"`
	Enabled    types.Bool   `tfsdk:"enabled"`
	PipelineId types.String `tfsdk:"pipeline_id"`
}

func NewPipelineScheduleResource() resource.Resource {
	return &PipelineSchedule{}
}

func (ps *PipelineSchedule) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pipeline_schedule"
}

func (ps *PipelineSchedule) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	ps.client = req.ProviderData.(*Client)
}

func (ps *PipelineSchedule) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resource_schema.Schema{
		MarkdownDescription: "A Pipeline Schedule is a schedule that triggers a pipeline to run at a specific time.",
		Attributes: map[string]resource_schema.Attribute{
			"pipeline_id": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The ID of the pipeline that this schedule belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"label": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "A label to describe the schedule.",
			},
			"cronline": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cronline that describes when the schedule should run.",
			},
			"branch": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The branch that the schedule should run on.",
			},
			"commit": resource_schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The commit that the schedule should run on.",
			},
			"message": resource_schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The message that the schedule should run on.",
			},
			"env": resource_schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The environment variables that the schedule should run on.",
			},
			"enabled": resource_schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Whether the schedule is enabled or not.",
			},
			"id": resource_schema.StringAttribute{
				Computed: true,
			},
		},
	}
}

func (ps *PipelineSchedule) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan PipelineScheduleResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("Creating Pipeline schedule %s ...", plan.Label.ValueString())
	apiResponse, err := createPipelineSchedule(
		ps.client.genqlient,
		plan.PipelineId.ValueString(),
		plan.Label.ValueString(),
		plan.Cronline.ValueString(),
		plan.Branch.ValueString(),
		plan.Commit.ValueString(),
		plan.Message.ValueString(),
		plan.Env.ValueString(),
		plan.Enabled.ValueBool())

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create Pipeline schedule",
			fmt.Sprintf("Unable to create Pipeline schedule: %s", err.Error()),
		)
		return
	}

	state := PipelineScheduleResourceModel{
		PipelineId: types.StringValue(apiResponse.PipelineScheduleCreate.Pipeline.Id),
		Id:         types.StringValue(apiResponse.PipelineScheduleCreate.PipelineScheduleEdge.Node.Id),
		Label:      types.StringValue(apiResponse.PipelineScheduleCreate.PipelineScheduleEdge.Node.Label),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)

}

func (ps *PipelineSchedule) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	//TODO
}

func (ps *PipelineSchedule) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	//TODO
}

func (ps *PipelineSchedule) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	//TODO
}
