package buildkite

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
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

	timeout, diags := c.client.timeouts.Read(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var matchFound bool
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var cursor *string
		for {
			r, err := getClusterByName(ctx, c.client.genqlient, c.client.organization, cursor)
			if err != nil {
				if isRetryableError(err) {
					return retry.RetryableError(err)
				}
				resp.Diagnostics.AddError(
					"Unable to read Cluster",
					fmt.Sprintf("Unable to read Cluster: %s", err.Error()),
				)
				return retry.NonRetryableError(err)
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
		return nil
	})

	if err != nil {
		resp.Diagnostics.AddError("Unable to find Cluster", err.Error())
		return
	}

	// if there is no match found by here then the cluster doesn't exist
	if !matchFound {
		resp.Diagnostics.AddError("Unable to find Cluster", fmt.Sprintf("Could not find cluster with name \"%s\"", state.Name.ValueString()))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (*clusterDatasource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a cluster by name.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"uuid": schema.StringAttribute{
				Computed: true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the Cluster to find.",
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
