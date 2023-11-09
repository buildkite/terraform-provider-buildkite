package buildkite

import (
	"context"
	"fmt"
	"log"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

type pipelineTemplateResourceModel struct {
	ID            types.String `tfsdk:"id"`
	UUID          types.String `tfsdk:"uuid"`
	Available     types.Bool   `tfsdk:"available"`
	Configuration types.String `tfsdk:"configuration"`
	Description   types.String `tfsdk:"description"`
	Name          types.String `tfsdk:"name"`
}

type pipelineTemplateResource struct {
	client *Client
}

func newPipelineTemplateResource() resource.Resource {
	return &pipelineTemplateResource{}
}

func (pipelineTemplateResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pipeline_template"
}

func (pt *pipelineTemplateResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	pt.client = req.ProviderData.(*Client)
}

func (pipelineTemplateResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resource_schema.Schema{
		MarkdownDescription: heredoc.Doc(`
			This resource allows for standardized step configurations that can be used within various pipelines of an organization.
		
			More information on pipeline templates can be found in the [documentation](https://buildkite.com/docs/pipelines/templates).
		`),
		Attributes: map[string]resource_schema.Attribute{
			"id": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The GraphQL ID of the pipeline template. ",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"uuid": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the pipeline template. ",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"available": resource_schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "If the pipeline template is available for assignment by non admin users. ",
			},
			"configuration": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The YAML step configuration for the pipeline template. ",
			},
			"description": resource_schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "A description for the pipeline template. ",
			},
			"name": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the pipeline template. ",
			},
		},
	}
}

func (pt *pipelineTemplateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, state pipelineTemplateResourceModel

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := pt.client.timeouts.Create(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var r *createPipelineTemplateResponse
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var err error

		log.Printf("Creating pipeline template %s ...", plan.Name.ValueString())
		r, err = createPipelineTemplate(ctx,
			pt.client.genqlient,
			pt.client.organizationId,
			plan.Name.ValueString(),
			plan.Configuration.ValueString(),
			plan.Description.ValueStringPointer(),
			plan.Available.ValueBool(),
		)

		return retryContextError(err)
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create pipeline template",
			fmt.Sprintf("Unable to create pipeline template %s", err.Error()),
		)
		return
	}

	state.ID = types.StringValue(r.PipelineTemplateCreate.PipelineTemplate.Id)
	state.UUID = types.StringValue(r.PipelineTemplateCreate.PipelineTemplate.Uuid)
	state.Name = types.StringValue(r.PipelineTemplateCreate.PipelineTemplate.Name)
	state.Configuration = types.StringValue(r.PipelineTemplateCreate.PipelineTemplate.Configuration)
	state.Description = types.StringPointerValue(r.PipelineTemplateCreate.PipelineTemplate.Description)
	state.Available = types.BoolValue(r.PipelineTemplateCreate.PipelineTemplate.Available)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (pt *pipelineTemplateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pipelineTemplateResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeouts, diags := pt.client.timeouts.Read(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var apiResponse *getNodeResponse
	err := retry.RetryContext(ctx, timeouts, func() *retry.RetryError {
		var err error

		log.Printf("Reading pipeline template with ID %s ...", state.ID.ValueString())
		apiResponse, err = getNode(ctx,
			pt.client.genqlient,
			state.ID.ValueString(),
		)

		return retryContextError(err)
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read pipeline pipeline",
			fmt.Sprintf("Unable to read pipeline template: %s", err.Error()),
		)
		return
	}

	// Convert from Node to getNodeNodePipelineTemplate type
	if pipelineTemplateNode, ok := apiResponse.GetNode().(*getNodeNodePipelineTemplate); ok {
		if pipelineTemplateNode == nil {
			resp.Diagnostics.AddError(
				"Unable to get pipeline template",
				"Error getting pipeline template: nil response",
			)
			return
		}
		updatePipelineTemplateResourceState(&state, *pipelineTemplateNode)
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	} else {
		// Resource not found, remove from state
		resp.Diagnostics.AddWarning("Pipeline template resource not found", "Removing it from state")
		resp.State.RemoveResource(ctx)
	}
}

func (pt *pipelineTemplateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (pt *pipelineTemplateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state pipelineTemplateResourceModel

	diagsState := req.State.Get(ctx, &state)
	diagsPlan := req.Plan.Get(ctx, &plan)

	resp.Diagnostics.Append(diagsState...)
	resp.Diagnostics.Append(diagsPlan...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := pt.client.timeouts.Create(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var r *updatePipelineTemplateResponse
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var err error

		log.Printf("Updating pipeline template %s with ID %s ...", plan.Name.ValueString(), plan.ID.ValueString())
		r, err = updatePipelineTemplate(ctx,
			pt.client.genqlient,
			pt.client.organizationId,
			plan.ID.ValueString(),
			plan.Name.ValueString(),
			plan.Configuration.ValueString(),
			plan.Description.ValueString(),
			plan.Available.ValueBool(),
		)

		return retryContextError(err)
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create pipeline template",
			fmt.Sprintf("Unable to create pipeline template %s", err.Error()),
		)
		return
	}

	state.Name = types.StringValue(r.PipelineTemplateUpdate.PipelineTemplate.Name)
	state.Configuration = types.StringValue(r.PipelineTemplateUpdate.PipelineTemplate.Configuration)
	state.Description = types.StringPointerValue(r.PipelineTemplateUpdate.PipelineTemplate.Description)
	state.Available = types.BoolValue(r.PipelineTemplateUpdate.PipelineTemplate.Available)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (pt *pipelineTemplateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var plan pipelineTemplateResourceModel

	diags := req.State.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := pt.client.timeouts.Delete(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var err error

		log.Printf("Deleting pipeline template %s with ID %s ...", plan.Name.ValueString(), plan.ID.ValueString())
		_, err = deletePipelineTemplate(ctx,
			pt.client.genqlient,
			pt.client.organizationId,
			plan.ID.ValueString(),
		)

		return retryContextError(err)
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete pipeline template",
			fmt.Sprintf("Unable to delete pipeline template: %s", err.Error()),
		)
		return
	}
}

func updatePipelineTemplateResourceState(ptr *pipelineTemplateResourceModel, ptn getNodeNodePipelineTemplate) {
	ptr.ID = types.StringValue(ptn.Id)
	ptr.UUID = types.StringValue(ptn.Uuid)
	ptr.Available = types.BoolValue(ptn.Available)
	ptr.Configuration = types.StringValue(ptn.Configuration)
	ptr.Description = types.StringPointerValue(ptn.Description)
	ptr.Name = types.StringValue(ptn.Name)
}
