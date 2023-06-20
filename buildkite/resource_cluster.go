package buildkite

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type ClusterResource struct {
	client *Client
}

type ClusterResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Emoji       types.String `tfsdk:"emoji"`
	Color       types.String `tfsdk:"color"`
}

func (c *ClusterResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "_cluster"
}

func (c *ClusterResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Cluster is a group of Agents that can be used to run your builds. " +
			"Clusters are useful for grouping Agents by their capabilities, such as operating system, hardware, or location. ",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the Cluster. Can only contain numbers and letters, no spaces or special characters.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "A description for the Cluster. Consider something short but clear on the Cluster's function.",
			},
			"emoji": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "An emoji to represent the Cluster. Accepts the format :buildkite:.",
			},
			"color": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "A color representation of the Cluster. Accepts hex codes, eg #BADA55.",
			},
		},
	}
}

func (c *ClusterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *ClusterResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	createReq := ClusterCreateInput{
		Name:        data.Name.ValueString(),
		Description: data.Description.ValueString(),
		Emoji:       data.Emoji.ValueString(),
		Color:       data.Color.ValueString(),
	}

	r, err := createCluster(c.client.genqlient, createReq)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create Cluster",
			fmt.Sprintf("Unable to create Cluster: %s", err.Error()),
		)
		return
	}

	data.ID = types.StringValue(r.ClusterCreate.Cluster.Id)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (c *ClusterResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ClusterResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	cluster, err := getCluster(c.client.genqlient, c.client.organizationId, data.ID.ValueString())

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read Cluster",
			fmt.Sprintf("Unable to read Cluster: %s", err.Error()),
		)
		return
	}

	data.Name = types.StringValue(cluster.Organization.Cluster.Name)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (c *ClusterResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ClusterResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	updateReq := ClusterUpdateInput{
		Name:        data.Name.ValueString(),
		Description: data.Description.ValueString(),
		Emoji:       data.Emoji.ValueString(),
		Color:       data.Color.ValueString(),
	}

	_, err := updateCluster(c.client.genqlient, updateReq)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to update Cluster",
			fmt.Sprintf("Unable to update Cluster: %s", err.Error()),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (c *ClusterResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ClusterResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	deleteReq := ClusterDeleteInput{
		Id: data.ID.ValueString(),
	}

	_, err := deleteCluster(c.client.genqlient, deleteReq)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete Cluster",
			fmt.Sprintf("Unable to delete Cluster: %s", err.Error()),
		)
		return
	}
}
