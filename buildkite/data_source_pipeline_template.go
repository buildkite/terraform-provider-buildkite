package buildkite

import (
	"context"
	"fmt"
	"log"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

type pipelineTemplateDatasourceModel struct {
	ID            types.String `tfsdk:"id"`
	UUID          types.String `tfsdk:"uuid"`
	Available     types.Bool   `tfsdk:"available"`
	Configuration types.String `tfsdk:"configuration"`
	Description   types.String `tfsdk:"description"`
	Name          types.String `tfsdk:"name"`
}

type pipelineTemplateDatasource struct {
	client *Client
}

func newPipelineTemplateDatasource() datasource.DataSource {
	return &pipelineTemplateDatasource{}
}

func (pt *pipelineTemplateDatasource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pipeline_template"
}

func (pt *pipelineTemplateDatasource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	pt.client = req.ProviderData.(*Client)
}
func (*pipelineTemplateDatasource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: heredoc.Doc(`
		Use this data source to retrieve a pipeline template by its ID.
		
		More information on pipeline templates can be found in the [documentation](https://buildkite.com/docs/pipelines/templates).
		`),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The GraphQL ID of the pipeline template.",
			},
			"uuid": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the pipeline template.",
			},
			"available": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "If the pipeline template is available for assignment by non admin users.",
			},
			"configuration": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The YAML step configuration for the pipeline template.",
			},
			"description": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The description for the pipeline template.",
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The name of the pipeline template.",
			},
		},
	}
}

func (pt *pipelineTemplateDatasource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state pipelineTemplateDatasourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)

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
			"Unable to get pipeline template",
			fmt.Sprintf("Error getting pipeline template: %s", err.Error()),
		)
		return
	}

	// Convert from Node to getNodeNodePipelineTemplate type
	if pipelineTemplateNode, ok := apiResponse.GetNode().(*getNodeNodePipelineTemplate); ok {
		if !ok {
			resp.Diagnostics.AddError(
				"Unable to get pipeline template",
				"Error getting pipeline template: nil response",
			)
			return
		}
		updatePipelineTemplateDatasourceState(&state, *pipelineTemplateNode)
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	}
}

func updatePipelineTemplateDatasourceState(ptds *pipelineTemplateDatasourceModel, ptn getNodeNodePipelineTemplate) {
	ptds.ID = types.StringValue(ptn.Id)
	ptds.UUID = types.StringValue(ptn.Uuid)
	ptds.Available = types.BoolValue(ptn.Available)
	ptds.Configuration = types.StringValue(ptn.Configuration)
	ptds.Description = types.StringPointerValue(ptn.Description)
	ptds.Name = types.StringValue(ptn.Name)
}
