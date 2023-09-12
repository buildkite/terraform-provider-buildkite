package buildkite

import (
	"context"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
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
		MarkdownDescription: heredoc.Doc(`
			This resource allows you to create and manage a Buildkite Cluster to run your builds in.
			Clusters are useful for grouping agents by there capabilities or permissions.
			Find out more information in our [documentation](https://buildkite.com/docs/clusters/overview).
		`),
		Attributes: map[string]resource_schema.Attribute{
			"id": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The GraphQL ID of the cluster.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"uuid": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the cluster.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": resource_schema.StringAttribute{
				MarkdownDescription: "The name of the Cluster. Can only contain numbers and letters, no spaces or special characters.",
				Required:            true,
			},
			"description": resource_schema.StringAttribute{
				Optional: true,
				MarkdownDescription: heredoc.Doc(`
					This is a description for the cluster, this may describe the usage for it, the region, or something else
					which would help identify the Cluster's purpose.
				`),
			},
			"emoji": resource_schema.StringAttribute{
				Optional: true,
				MarkdownDescription: heredoc.Doc(`
					An emoji to use with the Cluster, this can either be set using :buildkite: notation, or with the
					emoji itself, such as 🚀.
				`),
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

	diags := req.Plan.Get(ctx, &state)

	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := c.client.timeouts.Create(ctx, DefaultTimeout)

	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var r *createClusterResponse
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var err error
		r, err = createCluster(
			ctx,
			c.client.genqlient,
			c.client.organizationId,
			state.Name.ValueString(),
			state.Description.ValueStringPointer(),
			state.Emoji.ValueStringPointer(),
			state.Color.ValueStringPointer(),
		)

		return retryContextError(err)
	})

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

	diags := req.State.Get(ctx, &state)

	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := c.client.timeouts.Read(ctx, DefaultTimeout)

	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var r *getNodeResponse
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var err error
		r, err = getNode(ctx, c.client.genqlient, state.ID.ValueString())

		return retryContextError(err)
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read Cluster",
			fmt.Sprintf("Unable to read Cluster: %s", err.Error()),
		)
		return
	}

	if clusterNode, ok := r.GetNode().(*getNodeNodeCluster); ok {
		if clusterNode == nil {
			resp.Diagnostics.AddError(
				"Unable to get Cluster",
				"Error getting Cluster: nil response",
			)
			return
		}
		updateClusterResourceState(&state, *clusterNode)
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	} else {
		resp.Diagnostics.AddWarning(
			"Cluster not found",
			"Removing Cluster from state...",
		)
		resp.State.RemoveResource(ctx)
	}
}

func (c *clusterResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state, plan clusterResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := c.client.timeouts.Update(ctx, DefaultTimeout)

	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var err error
		_, err = updateCluster(ctx,
			c.client.genqlient,
			c.client.organizationId,
			state.ID.ValueString(),
			plan.Name.ValueString(),
			plan.Description.ValueStringPointer(),
			plan.Emoji.ValueStringPointer(),
			plan.Color.ValueStringPointer(),
		)

		return retryContextError(err)
	})

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

	diags := req.State.Get(ctx, &state)

	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := c.client.timeouts.Delete(ctx, DefaultTimeout)

	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var err error
		_, err = deleteCluster(ctx, c.client.genqlient, c.client.organizationId, state.ID.ValueString())

		return retryContextError(err)
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete Cluster",
			fmt.Sprintf("Unable to delete Cluster: %s", err.Error()),
		)
		return
	}
}

func (c *clusterResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func updateClusterResourceState(state *clusterResourceModel, res getNodeNodeCluster) {
	state.ID = types.StringValue(res.Id)
	state.UUID = types.StringValue(res.Uuid)
	state.Name = types.StringValue(res.Name)
	state.Description = types.StringPointerValue(res.Description)
	state.Emoji = types.StringPointerValue(res.Emoji)
	state.Color = types.StringPointerValue(res.Color)
}
