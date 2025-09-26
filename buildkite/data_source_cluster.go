package buildkite

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type clusterDatasource struct {
	client *Client
}

type clusterDatasourceModel struct {
	ID          types.String      `tfsdk:"id"`
	UUID        types.String      `tfsdk:"uuid"`
	Name        types.String      `tfsdk:"name"`
	Description types.String      `tfsdk:"description"`
	Emoji       types.String      `tfsdk:"emoji"`
	Color       types.String      `tfsdk:"color"`
	Maintainers []maintainerModel `tfsdk:"maintainers"`
}

func newClusterDatasource() datasource.DataSource {
	return &clusterDatasource{}
}

func (c *clusterDatasource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	c.client = req.ProviderData.(*Client)
}

func (*clusterDatasource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster"
}

func (c *clusterDatasource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state clusterDatasourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, diags := c.client.timeouts.Read(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var r *getClusterByNameResponse
	var err error
	cursor := (*string)(nil)
	matchFound := false

	// Loop through all pages until a match is found or we run out of pages
	for {
		r, err = getClusterByName(ctx, c.client.genqlient, c.client.organization, cursor)
		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to read Cluster",
				fmt.Sprintf("Unable to read Cluster: %s", err.Error()),
			)
			return
		}

		// loop over this page of results to try find the matching cluster
		for _, cluster := range r.Organization.Clusters.Edges {
			if cluster.Node.Name == state.Name.ValueString() {
				matchFound = true
				state.Color = types.StringPointerValue(cluster.Node.Color)
				state.Description = types.StringPointerValue(cluster.Node.Description)
				state.Emoji = types.StringPointerValue(cluster.Node.Emoji)
				state.ID = types.StringValue(cluster.Node.Id)
				state.Name = types.StringValue(cluster.Node.Name)
				state.UUID = types.StringValue(cluster.Node.Uuid)

				// Fetch maintainers for this cluster
				maintainers, err := c.client.listClusterMaintainers(ctx, c.client.organization, cluster.Node.Uuid)
				if err != nil {
					// Log warning but don't fail the entire request - maintainers might not be accessible
					resp.Diagnostics.AddWarning(
						"Unable to fetch cluster maintainers",
						fmt.Sprintf("Could not fetch maintainers for cluster %s: %s", cluster.Node.Name, err.Error()),
					)
					state.Maintainers = []maintainerModel{}
				} else {
					state.Maintainers = maintainers
				}
				break
			}
		}

		// end here if we found a match or there are no more pages to search
		if matchFound || !r.Organization.Clusters.PageInfo.HasNextPage {
			break
		}
		cursor = &r.Organization.Clusters.PageInfo.EndCursor
	}

	// If there is no match found by here then the cluster doesn't exist
	if !matchFound {
		resp.Diagnostics.AddError("Unable to find Cluster", fmt.Sprintf("Could not find cluster with name \"%s\"", state.Name.ValueString()))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (*clusterDatasource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to retrieve a cluster by name. You can find out more about clusters in the Buildkite [documentation](https://buildkite.com/docs/clusters/overview).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The GraphQL ID of the cluster.",
			},
			"uuid": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the cluster",
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the cluster to retrieve.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The description of the cluster.",
			},
			"emoji": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The emoji of the cluster.",
			},
			"color": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The color of the cluster.",
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
	}
}
