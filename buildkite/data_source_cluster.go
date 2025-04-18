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
	var state clusterResourceModel

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
		},
	}
}
