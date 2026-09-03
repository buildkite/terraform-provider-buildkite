package buildkite

import (
	"context"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type pipelinesDatasourceModel struct {
	Search     types.String     `tfsdk:"search"`
	Repository types.String     `tfsdk:"repository"`
	ClusterId  types.String     `tfsdk:"cluster_id"`
	Archived   types.Bool       `tfsdk:"archived"`
	Tags       []string         `tfsdk:"tags"`
	Total      types.Int64      `tfsdk:"total"`
	Pipelines  []pipelinesModel `tfsdk:"pipelines"`
}

type pipelinesModel struct {
	ID            types.String `tfsdk:"id"`
	UUID          types.String `tfsdk:"uuid"`
	Slug          types.String `tfsdk:"slug"`
	Name          types.String `tfsdk:"name"`
	Description   types.String `tfsdk:"description"`
	Repository    types.String `tfsdk:"repository"`
	DefaultBranch types.String `tfsdk:"default_branch"`
	ClusterId     types.String `tfsdk:"cluster_id"`
	Archived      types.Bool   `tfsdk:"archived"`
}

type pipelinesDatasource struct {
	client *Client
}

func newPipelinesDatasource() datasource.DataSource {
	return &pipelinesDatasource{}
}

func (p *pipelinesDatasource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	p.client = req.ProviderData.(*Client)
}

func (p *pipelinesDatasource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pipelines"
}

func (p *pipelinesDatasource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: heredoc.Doc(`
			Use this data source to retrieve the pipelines of an organization, optionally filtered. You can find out more
			about pipelines in the Buildkite [documentation](https://buildkite.com/docs/pipelines).

			The pipelines are read with one paginated GraphQL query (500 per page), so use the filters on large organizations.
		`),
		Attributes: map[string]schema.Attribute{
			"search": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Only return pipelines whose name matches this search, case insensitively.",
			},
			"repository": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Only return pipelines with this repository URL.",
			},
			"cluster_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Only return pipelines in the cluster with this GraphQL ID.",
			},
			"archived": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Only return archived (`true`) or unarchived (`false`) pipelines. Both are returned when not set.",
			},
			"tags": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Only return pipelines that have these tags.",
			},
			"total": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The number of pipelines that match.",
			},
			"pipelines": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "The GraphQL ID of the pipeline.",
							Computed:            true,
						},
						"uuid": schema.StringAttribute{
							MarkdownDescription: "The UUID of the pipeline.",
							Computed:            true,
						},
						"slug": schema.StringAttribute{
							MarkdownDescription: "The slug of the pipeline.",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "The name of the pipeline.",
							Computed:            true,
						},
						"description": schema.StringAttribute{
							MarkdownDescription: "The description of the pipeline.",
							Computed:            true,
						},
						"repository": schema.StringAttribute{
							MarkdownDescription: "The repository URL of the pipeline.",
							Computed:            true,
						},
						"default_branch": schema.StringAttribute{
							MarkdownDescription: "The default branch of the pipeline.",
							Computed:            true,
						},
						"cluster_id": schema.StringAttribute{
							MarkdownDescription: "The GraphQL ID of the cluster the pipeline belongs to.",
							Computed:            true,
						},
						"archived": schema.BoolAttribute{
							MarkdownDescription: "Whether the pipeline is archived.",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (p *pipelinesDatasource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state pipelinesDatasourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var repository *PipelineRepositoryInput
	if !state.Repository.IsNull() {
		repository = &PipelineRepositoryInput{Url: state.Repository.ValueString()}
	}

	state.Pipelines = []pipelinesModel{}
	var cursor *string
	for {
		res, err := GetOrganizationPipelines(ctx, p.client.genqlient, p.client.organization, cursor, state.Search.ValueStringPointer(), repository, state.ClusterId.ValueStringPointer(), state.Archived.ValueBoolPointer(), state.Tags)
		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to get organization pipelines",
				fmt.Sprintf("Error getting organization pipelines: %s", err.Error()),
			)
			return
		}

		state.Total = types.Int64Value(int64(res.Organization.Pipelines.Count))
		for _, pipeline := range res.Organization.Pipelines.Edges {
			updatePipelinesDatasourceState(&state, pipeline)
		}

		if !res.Organization.Pipelines.PageInfo.HasNextPage {
			break
		}

		cursor = &res.Organization.Pipelines.PageInfo.EndCursor
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func updatePipelinesDatasourceState(state *pipelinesDatasourceModel, data GetOrganizationPipelinesOrganizationPipelinesPipelineConnectionEdgesPipelineEdge) {
	pipelineState := pipelinesModel{
		ID:            types.StringValue(data.Node.Id),
		UUID:          types.StringValue(data.Node.Uuid),
		Slug:          types.StringValue(data.Node.Slug),
		Name:          types.StringValue(data.Node.Name),
		Description:   types.StringPointerValue(data.Node.Description),
		Repository:    types.StringValue(data.Node.Repository.Url),
		DefaultBranch: types.StringPointerValue(data.Node.DefaultBranch),
		ClusterId:     types.StringPointerValue(data.Node.Cluster.Id),
		Archived:      types.BoolValue(data.Node.Archived),
	}

	state.Pipelines = append(state.Pipelines, pipelineState)
}
