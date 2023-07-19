package buildkite

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

type pipelineSchedule struct {
	client *Client
}

type pipelineScheduleResourceModel struct {
	Id         types.String `tfsdk:"id"`
	Label      types.String `tfsdk:"label"`
	Cronline   types.String `tfsdk:"cronline"`
	Commit     types.String `tfsdk:"commit"`
	Branch     types.String `tfsdk:"branch"`
	Message    types.String `tfsdk:"message"`
	Env        types.Map    `tfsdk:"env"`
	Enabled    types.Bool   `tfsdk:"enabled"`
	PipelineId types.String `tfsdk:"pipeline_id"`
}

func NewPipelineScheduleResource() resource.Resource {
	return &pipelineSchedule{}
}

func (ps *pipelineSchedule) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pipeline_schedule"
}

func (ps *pipelineSchedule) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	ps.client = req.ProviderData.(*Client)
}

func (ps *pipelineSchedule) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resource_schema.Schema{
		MarkdownDescription: "A Pipeline Schedule is a schedule that triggers a pipeline to run at a specific time.",
		Attributes: map[string]resource_schema.Attribute{
			"id": resource_schema.StringAttribute{
				Computed: true,
			},
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
			"env": resource_schema.MapAttribute{
				ElementType: types.MapType{
					ElemType: types.StringType,
				},
				Optional:            true,
				MarkdownDescription: "The environment variables that the schedule should run on.",
			},
			"enabled": resource_schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Whether the schedule is enabled or not.",
			},
		},
	}
}

func (ps *pipelineSchedule) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, state pipelineScheduleResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf(" ######### Creating Pipeline schedule ... %v", plan)
	log.Printf(" #########Creating Pipeline schedule label #########  %s ...", plan.Label.ValueString())
	log.Printf(" #########Creating Pipeline schedule Branch #########  %s ...", plan.Branch.ValueString())
	log.Printf(" #########Creating Pipeline schedule Enabled #########  %t ...", plan.Enabled.ValueBool())
	log.Printf(" #########Creating Pipeline schedule Message #########  %s ...", plan.Message.ValueString())

	log.Printf("Creating Pipeline schedule %s ...", plan.Label.ValueString())

	envVars := envVarsMapFromTfToString(ctx, plan.Env)
	apiResponse, err := createPipelineSchedule(
		ps.client.genqlient,
		plan.PipelineId.ValueString(),
		plan.Label.ValueStringPointer(),
		plan.Cronline.ValueStringPointer(),
		plan.Message.ValueStringPointer(),
		plan.Commit.ValueStringPointer(),
		plan.Branch.ValueStringPointer(),
		&envVars,
		plan.Enabled.ValueBool())

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create Pipeline schedule",
			fmt.Sprintf("Unable to create Pipeline schedule: %s", err.Error()),
		)
		return
	}

	state.PipelineId = types.StringValue(apiResponse.PipelineScheduleCreate.Pipeline.Id)
	state.Id = types.StringValue(apiResponse.PipelineScheduleCreate.PipelineScheduleEdge.Node.Id)
	state.Label = types.StringPointerValue(apiResponse.PipelineScheduleCreate.PipelineScheduleEdge.Node.Label)
	state.Branch = types.StringPointerValue(apiResponse.PipelineScheduleCreate.PipelineScheduleEdge.Node.Branch)
	state.Commit = types.StringPointerValue(apiResponse.PipelineScheduleCreate.PipelineScheduleEdge.Node.Commit)
	state.Cronline = types.StringPointerValue(apiResponse.PipelineScheduleCreate.PipelineScheduleEdge.Node.Cronline)
	state.Message = types.StringPointerValue(apiResponse.PipelineScheduleCreate.PipelineScheduleEdge.Node.Message)
	m := envVarsArrayToMap(apiResponse.PipelineScheduleCreate.PipelineScheduleEdge.Node.Env)
	state.Env, _ = types.MapValueFrom(ctx, types.MapType{ElemType: types.StringType}, m)
	state.Enabled = types.BoolValue(apiResponse.PipelineScheduleCreate.PipelineScheduleEdge.Node.Enabled)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)

}

func (ps *pipelineSchedule) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pipelineScheduleResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("Reading Pipeline schedule %s ...", state.Label.ValueString())

	_, err := getPipelineSchedule(
		ps.client.genqlient,
		state.Id.ValueString(),
	)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read Pipeline schedule",
			fmt.Sprintf("Unable to read Pipeline schedule: %s", err.Error()),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (ps *pipelineSchedule) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state, plan pipelineScheduleResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	var envVars basetypes.StringValue // temp
	input := PipelineScheduleUpdateInput{
		Id:       state.Id.ValueString(),
		Label:    plan.Label.ValueStringPointer(),
		Cronline: plan.Cronline.ValueStringPointer(),
		Branch:   plan.Branch.ValueStringPointer(),
		Commit:   plan.Commit.ValueStringPointer(),
		Message:  plan.Message.ValueStringPointer(),
		Env:      envVars.ValueStringPointer(),
		Enabled:  plan.Enabled.ValueBool(),
	}

	log.Printf("Updating Pipeline schedule %s ...", plan.Label.ValueString())
	apiResponse, err := updatePipelineSchedule(
		ps.client.genqlient,
		input,
	)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to update Pipeline schedule",
			fmt.Sprintf("Unable to update Pipeline schedule: %s", err.Error()),
		)
		return
	}

	state.Label = types.StringPointerValue(apiResponse.PipelineScheduleUpdate.PipelineSchedule.Label)
	state.Branch = types.StringPointerValue(apiResponse.PipelineScheduleUpdate.PipelineSchedule.Branch)
	state.Commit = types.StringPointerValue(apiResponse.PipelineScheduleUpdate.PipelineSchedule.Commit)
	state.Cronline = types.StringPointerValue(apiResponse.PipelineScheduleUpdate.PipelineSchedule.Cronline)
	state.Message = types.StringPointerValue(apiResponse.PipelineScheduleUpdate.PipelineSchedule.Message)
	m := envVarsArrayToMap(apiResponse.PipelineScheduleUpdate.PipelineSchedule.Env)
	state.Env, _ = types.MapValueFrom(ctx, types.MapType{ElemType: types.StringType}, m)
	state.Enabled = types.BoolValue(apiResponse.PipelineScheduleUpdate.PipelineSchedule.Enabled)

	resp.Diagnostics.Append(req.State.Set(ctx, &state)...)

}

func (ps *pipelineSchedule) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var plan pipelineScheduleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	log.Println("Deleting Pipeline schedule ...")
	_, err := deletePipelineSchedule(ps.client.genqlient, plan.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete Pipeline schedule",
			fmt.Sprintf("Unable to delete Pipeline schedule: %s", err.Error()),
		)
		return
	}
}

func envVarsArrayToMap(envVars []*string) map[string]interface{} {
	envVarsMap := make(map[string]interface{})
	for _, envVar := range envVars {
		if envVar != nil {
			envVarSplit := strings.Split(*envVar, "=")
			envVarsMap[envVarSplit[0]] = envVarSplit[1]
		}
	}
	return envVarsMap
}

func envVarsMapFromTfToString(ctx context.Context, m types.Map) string {
	b := new(bytes.Buffer)

	envVarsMap := make(map[string]interface{})
	// read from the terraform data into the map
	if diags := m.ElementsAs(ctx, &envVarsMap, false); diags != nil {
		return ""
	}

	for key, value := range envVarsMap {
		fmt.Fprintf(b, "%s=%s\n", key, value.(string))
	}
	return b.String()

}

func updatePipelineScheduleResource(ctx context.Context, src getPipelineScheduleNodePipelineSchedule, ps *pipelineScheduleResourceModel) {

	ps.Label = types.StringPointerValue(src.Label)
	ps.Branch = types.StringPointerValue(src.Branch)
	ps.Commit = types.StringPointerValue(src.Commit)
	ps.Cronline = types.StringPointerValue(src.Cronline)
	ps.Enabled = types.BoolValue(src.Enabled)
	m := envVarsArrayToMap(src.Env)
	ps.Env, _ = types.MapValueFrom(ctx, types.MapType{ElemType: types.StringType}, m)
}
