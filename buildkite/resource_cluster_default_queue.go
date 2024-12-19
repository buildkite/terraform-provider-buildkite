package buildkite

import (
	"context"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

type clusterDefaultQueueResource struct {
	client *Client
}

type clusterDefaultQueueResourceModel struct {
	ClusterId types.String `tfsdk:"cluster_id"`
	ID        types.String `tfsdk:"id"`
	Key       types.String `tfsdk:"key"`
	QueueId   types.String `tfsdk:"queue_id"`
	UUID      types.String `tfsdk:"uuid"`
}

func newDefaultQueueClusterResource() resource.Resource {
	return &clusterDefaultQueueResource{}
}

// Create implements resource.Resource.
func (c *clusterDefaultQueueResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan clusterDefaultQueueResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := c.client.timeouts.Create(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	// modify cluster to set default
	var r *setClusterDefaultQueueResponse
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		org, err := c.client.GetOrganizationID()
		if err == nil {
			r, err = setClusterDefaultQueue(ctx, c.client.genqlient, *org, plan.ClusterId.ValueString(), plan.QueueId.ValueString())
		}

		return retryContextError(err)
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to attach default queue",
			fmt.Sprintf("Unable to attach default queue: %s", err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(r.ClusterUpdate.Cluster.Id)
	plan.UUID = types.StringValue(r.ClusterUpdate.Cluster.Uuid)
	plan.Key = types.StringValue(r.ClusterUpdate.Cluster.DefaultQueue.Key)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (c *clusterDefaultQueueResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state clusterDefaultQueueResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := c.client.timeouts.Delete(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		org, err := c.client.GetOrganizationID()
		if err == nil {
			_, err = removeClusterDefaultQueue(ctx, c.client.genqlient, *org, state.ClusterId.ValueString())
		}

		return retryContextError(err)
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to remove default queue",
			fmt.Sprintf("Unable to remove default queue: %s", err.Error()),
		)
		return
	}
}

// Metadata implements resource.Resource.
func (*clusterDefaultQueueResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_default_queue"
}

func (c *clusterDefaultQueueResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	c.client = req.ProviderData.(*Client)
}

// Read implements resource.Resource.
func (c *clusterDefaultQueueResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state clusterDefaultQueueResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

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
			"Unable to read cluster",
			fmt.Sprintf("Unable to read cluster: %s", err.Error()),
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
		state.ClusterId = types.StringValue(clusterNode.Id)
		state.UUID = types.StringValue(clusterNode.Uuid)
		state.QueueId = types.StringValue(clusterNode.DefaultQueue.Id)
		state.Key = types.StringValue(clusterNode.DefaultQueue.Key)
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	} else {
		resp.Diagnostics.AddWarning(
			"Cluster not found",
			"Removing Cluster from state...",
		)
		resp.State.RemoveResource(ctx)
	}
}

func (c *clusterDefaultQueueResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// Schema implements resource.Resource.
func (c *clusterDefaultQueueResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: heredoc.Doc(`
			This resource allows you to manage a default queue for a Buildkite Cluster.
			Find out more information in our [documentation](https://buildkite.com/docs/clusters/overview).
		`),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The GraphQL ID of the cluster.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"uuid": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the cluster.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"cluster_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The GraphQL ID of the cluster to which to add a default queue.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"queue_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The GraphQL ID of the cluster queue to set as default on the cluster.",
			},
			"key": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				MarkdownDescription: "The Key for the cluster queue; its unique identifier",
			},
		},
	}
}

// Update implements resource.Resource.
func (c *clusterDefaultQueueResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan clusterDefaultQueueResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := c.client.timeouts.Update(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	// modify cluster to set default
	var r *setClusterDefaultQueueResponse
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		org, err := c.client.GetOrganizationID()
		if err == nil {
			r, err = setClusterDefaultQueue(ctx, c.client.genqlient, *org, plan.ClusterId.ValueString(), plan.QueueId.ValueString())
		}

		return retryContextError(err)
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to attach default queue",
			fmt.Sprintf("Unable to attach default queue: %s", err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(r.ClusterUpdate.Cluster.Id)
	plan.UUID = types.StringValue(r.ClusterUpdate.Cluster.Uuid)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
