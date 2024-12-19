package buildkite

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

type clusterQueueResourceModel struct {
	Id             types.String `tfsdk:"id"`
	Uuid           types.String `tfsdk:"uuid"`
	ClusterId      types.String `tfsdk:"cluster_id"`
	ClusterUuid    types.String `tfsdk:"cluster_uuid"`
	Key            types.String `tfsdk:"key"`
	Description    types.String `tfsdk:"description"`
	DispatchPaused types.Bool   `tfsdk:"dispatch_paused"`
}

type clusterQueueResource struct {
	client *Client
}

func newClusterQueueResource() resource.Resource {
	return &clusterQueueResource{}
}

func (clusterQueueResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_queue"
}

func (cq *clusterQueueResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	cq.client = req.ProviderData.(*Client)
}

func (clusterQueueResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resource_schema.Schema{
		MarkdownDescription: "A Cluster Queue is a queue belonging to a specific Cluster for its Agents to target builds on. ",
		Attributes: map[string]resource_schema.Attribute{
			"id": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The GraphQL ID of the cluster queue.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"uuid": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the cluster queue.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"cluster_uuid": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the cluster this queue belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"cluster_id": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The ID of the cluster that this cluster queue belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"key": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The key of the cluster queue.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": resource_schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "A description for the cluster queue.",
			},
			"dispatch_paused": resource_schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The dispatch state of a cluster queue.",
				Default:             booldefault.StaticBool(false),
			},
		},
	}
}

func (cq *clusterQueueResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, state clusterQueueResourceModel

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := cq.client.timeouts.Create(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var r *createClusterQueueResponse
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		org, err := cq.client.GetOrganizationID()
		if err == nil {
			log.Printf("Creating cluster queue with key %s into cluster %s ...", plan.Key.ValueString(), plan.ClusterId.ValueString())
			r, err = createClusterQueue(ctx,
				cq.client.genqlient,
				*org,
				plan.ClusterId.ValueString(),
				plan.Key.ValueString(),
				plan.Description.ValueStringPointer(),
			)
		}

		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create Cluster Queue",
			fmt.Sprintf("Unable to create Cluster Queue: %s", err.Error()),
		)
		return
	}

	state.Id = types.StringValue(r.ClusterQueueCreate.ClusterQueue.Id)
	state.Uuid = types.StringValue(r.ClusterQueueCreate.ClusterQueue.Uuid)
	state.ClusterId = plan.ClusterId
	state.ClusterUuid = types.StringValue(r.ClusterQueueCreate.ClusterQueue.Cluster.Uuid)
	state.Key = types.StringValue(r.ClusterQueueCreate.ClusterQueue.Key)
	state.Description = types.StringPointerValue(r.ClusterQueueCreate.ClusterQueue.Description)

	// GraphQL API does not allow Cluster Queue to be created with Dispatch Paused
	if plan.DispatchPaused.ValueBool() {
		resp.Diagnostics.AddWarning(
			"Cluster Queue cannot be created with Dispatch Paused",
			"Setting dispatch_paused in State to 'true' but requires re-run of Apply",
		)
	}
	state.DispatchPaused = plan.DispatchPaused
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (cq *clusterQueueResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state clusterQueueResourceModel

	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := cq.client.timeouts.Read(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var r *getClusterQueuesResponse
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var err error

		log.Printf("Getting cluster queues for cluster %s ...", state.ClusterUuid.ValueString())
		r, err = getClusterQueues(ctx,
			cq.client.genqlient,
			cq.client.organization, state.ClusterUuid.ValueString(),
		)

		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read Cluster Queues",
			fmt.Sprintf("Unable to read Cluster Queues: %s", err.Error()),
		)
		return
	}

	// Find the cluster queue from the returned queues to update state
	for _, edge := range r.Organization.Cluster.Queues.Edges {
		if edge.Node.Id == state.Id.ValueString() {
			log.Printf("Found cluster queue with ID %s in cluster %s", edge.Node.Id, state.ClusterUuid.ValueString())
			// Update ClusterQueueResourceModel with Node values and append
			updateClusterQueueResource(edge.Node, &state)
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}

	// Cluster queue could not be found in returned queues and should be removed from state
	resp.Diagnostics.AddWarning(
		"Cluster Queue not found",
		"Removing Cluster Queue from state...",
	)
	resp.State.RemoveResource(ctx)
}

func (cq *clusterQueueResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importComponents := strings.Split(req.ID, ",")

	if len(importComponents) != 2 || importComponents[0] == "" || importComponents[1] == "" {
		resp.Diagnostics.AddError(
			"Unexpected Import Identifier",
			fmt.Sprintf("Expected import identifier with format: id,cluster_uuid. Got: %q", req.ID),
		)
		return
	}

	// Adding the cluster queue ID/cluster UUID to state for Read
	log.Printf("Importing cluster queue %s ...", importComponents[0])
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), importComponents[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_uuid"), importComponents[1])...)
}

func (cq *clusterQueueResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state clusterQueueResourceModel
	var planDispatchPaused, stateDispatchPaused bool

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := cq.client.timeouts.Update(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var r *updateClusterQueueResponse

	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		org, err := cq.client.GetOrganizationID()
		if err == nil {
			log.Printf("Updating cluster queue %s ...", state.Id.ValueString())

			// Extract the planned and state values for DispatchPaused attribute
			// to compare if the Queue should be paused or resumed
			// if neither do nothing
			planDispatchPaused = plan.DispatchPaused.ValueBool()
			stateDispatchPaused = state.DispatchPaused.ValueBool()

			// Check the planned value against the current state value
			// Planned to be true (changing from false to true)
			if planDispatchPaused && !stateDispatchPaused {
				if err := cq.pauseDispatch(ctx, timeout, state, &resp.Diagnostics); err != nil {
					return retryContextError(err)
				}
			}

			// Planned to be false (changing from true to false)
			if !planDispatchPaused && stateDispatchPaused {
				if err := cq.resumeDispatch(ctx, timeout, state, &resp.Diagnostics); err != nil {
					return retryContextError(err)
				}
			}

			r, err = updateClusterQueue(ctx,
				cq.client.genqlient,
				*org,
				state.Id.ValueString(),
				plan.Description.ValueStringPointer(),
			)
		}

		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to update Cluster Queue",
			fmt.Sprintf("Unable to update Cluster Queue: %s", err.Error()),
		)
		return
	}

	state.Description = types.StringPointerValue(r.ClusterQueueUpdate.ClusterQueue.Description)
	state.DispatchPaused = types.BoolValue(r.ClusterQueueUpdate.ClusterQueue.DispatchPaused)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (cq *clusterQueueResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var plan clusterQueueResourceModel

	diags := req.State.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := cq.client.timeouts.Delete(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		org, err := cq.client.GetOrganizationID()
		if err == nil {
			log.Printf("Deleting cluster queue %s ...", plan.Id.ValueString())
			_, err = deleteClusterQueue(ctx,
				cq.client.genqlient,
				*org,
				plan.Id.ValueString(),
			)
		}

		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create Cluster Queue",
			fmt.Sprintf("Unable to delete Cluster Queue: %s", err.Error()),
		)
		return
	}
}

func updateClusterQueueResource(clusterQueueNode getClusterQueuesOrganizationClusterQueuesClusterQueueConnectionEdgesClusterQueueEdgeNodeClusterQueue, cq *clusterQueueResourceModel) {
	cq.Id = types.StringValue(clusterQueueNode.Id)
	cq.Uuid = types.StringValue(clusterQueueNode.Uuid)
	cq.Key = types.StringValue(clusterQueueNode.Key)
	cq.Description = types.StringPointerValue(clusterQueueNode.Description)
	cq.ClusterId = types.StringValue(clusterQueueNode.Cluster.Id)
	cq.ClusterUuid = types.StringValue(clusterQueueNode.Cluster.Uuid)
	cq.DispatchPaused = types.BoolValue(clusterQueueNode.DispatchPaused)
}

func (cq *clusterQueueResource) pauseDispatch(ctx context.Context, timeout time.Duration, state clusterQueueResourceModel, diag *diag.Diagnostics) error {
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		log.Printf("Pausing dispatch for cluster queue %s", state.Key)
		_, err := pauseDispatchClusterQueue(ctx, cq.client.genqlient, state.Id.ValueString())
		return retryContextError(err)
	})

	if err != nil {
		diag.AddError(
			"Unable to pause cluster queue dispatch",
			fmt.Sprintf("Unable to pause dispatch for cluster queue: %s", err.Error()),
		)
	}

	return err
}

func (cq *clusterQueueResource) resumeDispatch(ctx context.Context, timeout time.Duration, state clusterQueueResourceModel, diag *diag.Diagnostics) error {
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		log.Printf("Resuming dispatch for cluster queue %s", state.Key)
		_, err := resumeDispatchClusterQueue(ctx, cq.client.genqlient, state.Id.ValueString())
		return retryContextError(err)
	})

	if err != nil {
		diag.AddError(
			"Unable to resume cluster queue dispatch",
			fmt.Sprintf("Unable to resume dispatch for cluster queue: %s", err.Error()),
		)
	}

	return err
}
