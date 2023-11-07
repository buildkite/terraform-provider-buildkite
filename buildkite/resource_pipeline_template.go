package buildkite

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type PipelineTemplateResourceModel struct {
	Id            types.String `tfsdk:"id"`
	UUID          types.String `tfsdk:"uuid"`
	Available     types.Bool   `tfsdk:"available"`
	Configuration types.String `tfsdk:"configuration"`
	Description   types.String `tfsdk:"description"`
	Name          types.String `tfsdk:"name"`
}

type PipelineTemplateResource struct {
	client *Client
}

func NewPipelineTemplateResource() resource.Resource {
	return &PipelineTemplateResource{}
}

func (PipelineTemplateResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pipeline_template"
}

func (pt *PipelineTemplateResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	pt.client = req.ProviderData.(*Client)
}

func (PipelineTemplateResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resource_schema.Schema{
		MarkdownDescription: "A Pipeline Template is a standardised step configuration to use across various pipelines within an organization. ",
		Attributes: map[string]resource_schema.Attribute{
			"id": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The GraphQL ID of the pipeline template.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"uuid": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the pipeline template.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"available": resource_schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "If the pipeline template is available for assignment by non admin users",
			},
			"configuration": resource_schema.StringAttribute{
				Optional:            true,
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

func (pt *PipelineTemplateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {

}

func (pt *PipelineTemplateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {

}

func (pt *PipelineTemplateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {

}

func (pt *PipelineTemplateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {

}

func (pt *PipelineTemplateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {

}
