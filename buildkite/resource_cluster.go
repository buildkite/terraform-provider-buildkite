package buildkite

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
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
	UUID        types.String `tfsdk:"uuid"`
}

func NewClusterResource() resource.Resource {
	return &ClusterResource{}
}

func (c *ClusterResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster"
}

func (c *ClusterResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	c.client = req.ProviderData.(*Client)
}

func (c *ClusterResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resource_schema.Schema{
		MarkdownDescription: "A Cluster is a group of Agents that can be used to run your builds. " +
			"Clusters are useful for grouping Agents by their capabilities, such as operating system, hardware, or location. ",
		Attributes: map[string]resource_schema.Attribute{
			"id": resource_schema.StringAttribute{
				Computed: true,
			},
			"uuid": resource_schema.StringAttribute{
				Computed: true,
			},
			"name": resource_schema.StringAttribute{
				MarkdownDescription: "The name of the Cluster. Can only contain numbers and letters, no spaces or special characters.",
				Required:            true,
			},
			"description": resource_schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "A description for the Cluster. Consider something short but clear on the Cluster's function.",
			},
			"emoji": resource_schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "An emoji to represent the Cluster. Accepts the format :buildkite:.",
			},
			"color": resource_schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "A color representation of the Cluster. Accepts hex codes, eg #BADA55.",
			},
		},
	}
}

func (c *ClusterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ClusterResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	r, err := createCluster(
		c.client.genqlient,
		c.client.organizationId,
		data.Name.ValueString(),
		data.Description.ValueString(),
		data.Emoji.ValueString(),
		data.Color.ValueString(),
	)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create Cluster",
			fmt.Sprintf("Unable to create Cluster: %s", err.Error()),
		)
		return
	}

	data.ID = types.StringValue(r.ClusterCreate.Cluster.Id)
	data.UUID = types.StringValue(r.ClusterCreate.Cluster.Uuid)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (c *ClusterResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ClusterResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	cluster, err := getCluster(c.client.genqlient, c.client.organization, data.ID.ValueString())

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read Cluster",
			fmt.Sprintf("Unable to read Cluster: %s", err.Error()),
		)
		return
	}

	data.ID = types.StringValue(cluster.Organization.Cluster.Id)
	data.Name = types.StringValue(cluster.Organization.Cluster.Name)
	data.Description = types.StringValue(cluster.Organization.Cluster.Description)
	data.UUID = types.StringValue(cluster.Organization.Cluster.Uuid)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (c *ClusterResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ClusterResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	_, err := updateCluster(
		c.client.genqlient,
		c.client.organizationId,
		data.Name.ValueString(),
		data.Description.ValueString(),
		data.Emoji.ValueString(),
		data.Color.ValueString(),
	)

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

	_, err := deleteCluster(c.client.genqlient, c.client.organizationId, data.ID.ValueString())

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete Cluster",
			fmt.Sprintf("Unable to delete Cluster: %s", err.Error()),
		)
		return
	}
}
