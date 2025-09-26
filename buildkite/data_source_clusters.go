package buildkite

import (
	"context"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type clustersDatasourceModel struct {
	Clusters []clustersModel `tfsdk:"clusters"`
}

type clustersModel struct {
	ID           types.String               `tfsdk:"id"`
	UUID         types.String               `tfsdk:"uuid"`
	Name         types.String               `tfsdk:"name"`
	Description  types.String               `tfsdk:"description"`
	Emoji        types.String               `tfsdk:"emoji"`
	Color        types.String               `tfsdk:"color"`
	DefaultQueue *clustersDefaultQueueModel `tfsdk:"default_queue"`
	Maintainers  []maintainerModel          `tfsdk:"maintainers"`
}

type clustersDefaultQueueModel struct {
	ID          types.String `tfsdk:"id"`
	UUID        types.String `tfsdk:"uuid"`
	Key         types.String `tfsdk:"key"`
	Description types.String `tfsdk:"description"`
}

type clustersDatasource struct {
	client *Client
}

func newClustersDatasource() datasource.DataSource {
	return &clustersDatasource{}
}

func (c *clustersDatasource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	c.client = req.ProviderData.(*Client)
}

func (c *clustersDatasource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clusters"
}

func (c *clustersDatasource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: heredoc.Doc(`
			Use this data source to retrieve clusters of an organization. You can find out more about clusters in the Buildkite
			[documentation](https://buildkite.com/docs/agent/v3/clusters).
		`),
		Attributes: map[string]schema.Attribute{
			"clusters": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "The GraphQL ID of the cluster.",
							Computed:            true,
						},
						"uuid": schema.StringAttribute{
							MarkdownDescription: "The UUID of the cluster.",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "The name of the cluster.",
							Computed:            true,
						},
						"description": schema.StringAttribute{
							MarkdownDescription: "The description of the cluster.",
							Computed:            true,
						},
						"emoji": schema.StringAttribute{
							MarkdownDescription: "The emoji for the cluster.",
							Computed:            true,
						},
						"color": schema.StringAttribute{
							MarkdownDescription: "The color for the cluster.",
							Computed:            true,
						},
						"default_queue": schema.SingleNestedAttribute{
							MarkdownDescription: "The default queue for the cluster.",
							Computed:            true,
							Attributes: map[string]schema.Attribute{
								"id": schema.StringAttribute{
									MarkdownDescription: "The GraphQL ID of the default queue.",
									Computed:            true,
								},
								"uuid": schema.StringAttribute{
									MarkdownDescription: "The UUID of the default queue.",
									Computed:            true,
								},
								"key": schema.StringAttribute{
									MarkdownDescription: "The key of the default queue.",
									Computed:            true,
								},
								"description": schema.StringAttribute{
									MarkdownDescription: "The description of the default queue.",
									Computed:            true,
								},
							},
						},
						"maintainers": schema.ListNestedAttribute{
							Computed:            true,
							MarkdownDescription: "List of maintainers (users and teams) for this cluster.",
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"permission_id": schema.StringAttribute{
										Computed:            true,
										MarkdownDescription: "The permission ID of the maintainer.",
									},
									"actor_id": schema.StringAttribute{
										Computed:            true,
										MarkdownDescription: "The UUID of the actor (user or team).",
									},
									"actor_type": schema.StringAttribute{
										Computed:            true,
										MarkdownDescription: "The type of the actor (user or team).",
									},
									"actor_name": schema.StringAttribute{
										Computed:            true,
										MarkdownDescription: "The name of the actor.",
									},
									"actor_email": schema.StringAttribute{
										Computed:            true,
										MarkdownDescription: "The email of the actor (only for users).",
									},
									"actor_slug": schema.StringAttribute{
										Computed:            true,
										MarkdownDescription: "The slug of the actor (only for teams).",
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func (c *clustersDatasource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state clustersDatasourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var cursor *string
	for {
		res, err := GetOrganizationClusters(ctx, c.client.genqlient, c.client.organization, cursor)
		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to get organization clusters",
				fmt.Sprintf("Error getting organization clusters: %s", err.Error()),
			)
			return
		}

		if len(res.Organization.Clusters.Edges) == 0 {
			resp.Diagnostics.AddError(
				"No organization clusters found",
				fmt.Sprintf("Error getting clusters for organization: %s", c.client.organization),
			)
			return
		}

		for _, cluster := range res.Organization.Clusters.Edges {
			updateClustersDatasourceState(ctx, c.client, resp, &state, cluster)
		}

		if !res.Organization.Clusters.PageInfo.HasNextPage {
			break
		}

		cursor = &res.Organization.Clusters.PageInfo.EndCursor
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func updateClustersDatasourceState(ctx context.Context, client *Client, resp *datasource.ReadResponse, state *clustersDatasourceModel, data GetOrganizationClustersOrganizationClustersClusterConnectionEdgesClusterEdge) {
	clusterState := clustersModel{
		ID:          types.StringValue(data.Node.Id),
		UUID:        types.StringValue(data.Node.Uuid),
		Name:        types.StringValue(data.Node.Name),
		Description: types.StringPointerValue(data.Node.Description),
		Emoji:       types.StringPointerValue(data.Node.Emoji),
		Color:       types.StringPointerValue(data.Node.Color),
	}

	if data.Node.DefaultQueue != nil {
		clusterState.DefaultQueue = &clustersDefaultQueueModel{
			ID:          types.StringValue(data.Node.DefaultQueue.Id),
			UUID:        types.StringValue(data.Node.DefaultQueue.Uuid),
			Key:         types.StringValue(data.Node.DefaultQueue.Key),
			Description: types.StringPointerValue(data.Node.DefaultQueue.Description),
		}
	}

	// Fetch maintainers for this cluster
	maintainers, err := client.listClusterMaintainers(ctx, client.organization, data.Node.Uuid)
	if err != nil {
		// Log warning but don't fail the entire request - maintainers might not be accessible
		resp.Diagnostics.AddWarning(
			"Unable to fetch cluster maintainers",
			fmt.Sprintf("Could not fetch maintainers for cluster %s: %s", data.Node.Name, err.Error()),
		)
		clusterState.Maintainers = []maintainerModel{}
	} else {
		clusterState.Maintainers = maintainers
	}

	state.Clusters = append(state.Clusters, clusterState)
}
