package buildkite

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type clusterResource struct {
	client *Client
}

type clusterResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Emoji       types.String `tfsdk:"emoji"`
	Color       types.String `tfsdk:"color"`
	UUID        types.String `tfsdk:"uuid"`
}

func newClusterResource() resource.Resource {
	return &clusterResource{}
}

func (c *clusterResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster"
}

func (c *clusterResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	c.client = req.ProviderData.(*Client)
}

func (c *clusterResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resource_schema.Schema{
		MarkdownDescription: "A Cluster is a group of Agents that can be used to run your builds. " +
			"Clusters are useful for grouping Agents by their capabilities, such as operating system, hardware, or location. ",
		Attributes: map[string]resource_schema.Attribute{
			"id": resource_schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"uuid": resource_schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
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

func (c *clusterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var state *clusterResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	r, err := createCluster(ctx,
		c.client.genqlient,
		c.client.organizationId,
		state.Name.ValueString(),
		state.Description.ValueStringPointer(),
		state.Emoji.ValueStringPointer(),
		state.Color.ValueStringPointer(),
	)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create Cluster",
			fmt.Sprintf("Unable to create Cluster: %s", err.Error()),
		)
		return
	}

	state.ID = types.StringValue(r.ClusterCreate.Cluster.Id)
	state.UUID = types.StringValue(r.ClusterCreate.Cluster.Uuid)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (c *clusterResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state clusterResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	r, err := getCluster(ctx, c.client.genqlient, c.client.organization, state.UUID.ValueString())

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read Cluster",
			fmt.Sprintf("Unable to read Cluster: %s", err.Error()),
		)
		return
	}

	updateClusterResourceState(r.Organization.Cluster, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (c *clusterResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state, plan clusterResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()

	_, err := updateCluster(ctx,
		c.client.genqlient,
		c.client.organizationId,
		id,
		plan.Name.ValueString(),
		plan.Description.ValueStringPointer(),
		plan.Emoji.ValueStringPointer(),
		plan.Color.ValueStringPointer(),
	)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to update Cluster",
			fmt.Sprintf("Unable to update Cluster: %s", err.Error()),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (c *clusterResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state clusterResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	_, err := deleteCluster(ctx, c.client.genqlient, c.client.organizationId, state.ID.ValueString())

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete Cluster",
			fmt.Sprintf("Unable to delete Cluster: %s", err.Error()),
		)
		return
	}
}

func (c *clusterResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("uuid"), req, resp)
}

func updateClusterResourceState(cl getClusterOrganizationCluster, c *clusterResourceModel) {
	c.ID = types.StringValue(cl.Id)
	c.UUID = types.StringValue(cl.Uuid)
	c.Name = types.StringValue(cl.Name)
	c.Description = types.StringPointerValue(cl.Description)
	c.Emoji = types.StringPointerValue(cl.Emoji)
	c.Color = types.StringPointerValue(cl.Color)
}
