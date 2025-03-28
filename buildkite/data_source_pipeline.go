package buildkite

import (
	"context"
	"fmt"
	"log"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type pipelineDataSourceModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	DefaultBranch types.String `tfsdk:"default_branch"`
	Description   types.String `tfsdk:"description"`
	Repository    types.String `tfsdk:"repository"`
	Slug          types.String `tfsdk:"slug"`
	UUID          types.String `tfsdk:"uuid"`
	WebhookUrl    types.String `tfsdk:"webhook_url"`
}

type pipelineDatasource struct {
	client *Client
}

func newPipelineDatasource() datasource.DataSource {
	return &pipelineDatasource{}
}

func (p *pipelineDatasource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	p.client = req.ProviderData.(*Client)
}

func (*pipelineDatasource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pipeline"
}

func (*pipelineDatasource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: heredoc.Doc(`
			Use this data source to look up properties on a specific pipeline. This is particularly useful for looking up the webhook URL for each pipeline.

			More info in the Buildkite [documentation](https://buildkite.com/docs/pipelines).
		`),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The GraphQL ID of the pipeline.",
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The name of the pipeline.",
			},
			"default_branch": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The default branch to prefill when new builds are created or triggered.",
			},
			"description": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The description of the pipeline.",
			},
			"repository": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The git URL of the repository.",
			},
			"slug": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The slug of the pipeline.",
			},
			"uuid": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the pipeline.",
			},
			"webhook_url": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The Buildkite webhook URL that triggers builds on this pipeline.",
			},
		},
	}
}

func (c *pipelineDatasource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state pipelineDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	orgPipelineSlug := fmt.Sprintf("%s/%s", c.client.organization, state.Slug.ValueString())

	log.Printf("Obtaining pipeline with slug %s ...", orgPipelineSlug)
	pipeline, err := getPipeline(ctx, c.client.genqlient, orgPipelineSlug)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read pipeline",
			fmt.Sprintf("Unable to read pipeline: %s", err.Error()),
		)
		return
	}

	if pipeline.Pipeline.Id == "" {
		resp.Diagnostics.AddError(
			"Unable to find pipeline",
			fmt.Sprintf("Could not find pipeline with slug \"%s\"", orgPipelineSlug),
		)
		return
	}

	state.ID = types.StringValue(pipeline.Pipeline.Id)
	state.DefaultBranch = types.StringValue(pipeline.Pipeline.DefaultBranch)
	state.Description = types.StringValue(pipeline.Pipeline.Description)
	state.Name = types.StringValue(pipeline.Pipeline.Name)
	state.Repository = types.StringValue(pipeline.Pipeline.Repository.Url)
	state.Slug = types.StringValue(pipeline.Pipeline.Slug)
	state.UUID = types.StringValue(pipeline.Pipeline.PipelineUuid)
	state.WebhookUrl = types.StringValue(pipeline.Pipeline.WebhookURL)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
