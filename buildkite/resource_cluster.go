package buildkite

import (
	"context"
	"fmt"
	"regexp"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

type clusterResource struct {
	client *Client
}

type clusterResourceModel struct {
	ID                      types.String `tfsdk:"id"`
	Name                    types.String `tfsdk:"name"`
	Description             types.String `tfsdk:"description"`
	Emoji                   types.String `tfsdk:"emoji"`
	Color                   types.String `tfsdk:"color"`
	UUID                    types.String `tfsdk:"uuid"`
	AgentTracingServiceUUID types.String `tfsdk:"agent_tracing_service_uuid"`
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
			"agent_tracing_service_uuid": resource_schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: heredoc.Doc(`
					The UUID of the OpenTelemetry tracing notification service that agents in this Cluster export traces
					to, such as ` + "`buildkite_notification_service.otel.id`" + `. The service must be enabled, cover all
					pipelines, and have no branch filter, and the organization must have agent tracing enabled.
					Set this to ` + "`\"\"`" + ` to clear the selection.
					Setting, changing, or clearing the selection only affects agents that register afterwards: running
					agents keep their current exporter configuration and keep tracing until they are restarted or
					re-registered.
					Leaving this unset adopts the Cluster's current selection, and **removing it from configuration does
					not clear the selection: agents keep exporting traces** until it is set to ` + "`\"\"`" + ` or cleared in
					the Buildkite UI or API.
					If the selected service is later deleted, disabled, or given a branch filter, the Cluster still reports
					its UUID and plans stay clean, but agents that register afterwards silently stop receiving tracing
					configuration.
					The API reports no selection unless agent tracing is enabled for the organization and the API token can
					manage the Cluster.
				`),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Validators: []validator.String{
					// the API returns the UUID lowercased, so any other form would never match what is read back
					stringvalidator.RegexMatches(agentTracingServiceUUIDRegex, `must be a lowercase UUID, such as a notification service's id, or "" to clear the selection`),
				},
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

	timeout, diags := c.client.createTimeout(ctx)

	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var r *createClusterResponse
	err := retryMutation(ctx, timeout, func(ctx context.Context) *retry.RetryError {
		org, err := c.client.GetOrganizationID(ctx)
		if err == nil {
			r, err = createCluster(
				ctx,
				c.client.genqlient,
				*org,
				state.Name.ValueString(),
				state.Description.ValueStringPointer(),
				state.Emoji.ValueStringPointer(),
				state.Color.ValueStringPointer(),
				agentTracingServiceUUIDToWrite(state.AgentTracingServiceUUID, types.StringNull()),
			)
		}

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
	state.AgentTracingServiceUUID = agentTracingServiceUUIDFromAPI(r.ClusterCreate.Cluster.AgentTracingServiceUuid, state.AgentTracingServiceUUID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (c *clusterResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state clusterResourceModel

	diags := req.State.Get(ctx, &state)

	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := c.client.readTimeout(ctx)

	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var r *getNodeResponse
	err := retry.RetryContext(requestCtx, timeout, func() *retry.RetryError {
		var err error
		r, err = getNode(requestCtx, c.client.genqlient, state.ID.ValueString())

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

	timeout, diags := c.client.updateTimeout(ctx)

	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	err := retryMutation(ctx, timeout, func(ctx context.Context) *retry.RetryError {
		org, err := c.client.GetOrganizationID(ctx)
		if err == nil && clearsAgentTracingService(plan.AgentTracingServiceUUID, state.AgentTracingServiceUUID) {
			_, err = updateClusterClearingAgentTracingService(ctx,
				c.client.genqlient,
				*org,
				state.ID.ValueString(),
				plan.Name.ValueString(),
				plan.Description.ValueStringPointer(),
				plan.Emoji.ValueStringPointer(),
				plan.Color.ValueStringPointer(),
			)
		} else if err == nil {
			_, err = updateCluster(ctx,
				c.client.genqlient,
				*org,
				state.ID.ValueString(),
				plan.Name.ValueString(),
				plan.Description.ValueStringPointer(),
				plan.Emoji.ValueStringPointer(),
				plan.Color.ValueStringPointer(),
				agentTracingServiceUUIDToWrite(plan.AgentTracingServiceUUID, state.AgentTracingServiceUUID),
			)
		}

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

	timeout, diags := c.client.deleteTimeout(ctx)

	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	err := retryMutation(ctx, timeout, func(ctx context.Context) *retry.RetryError {
		org, err := c.client.GetOrganizationID(ctx)
		if err == nil {
			_, err = deleteCluster(ctx, c.client.genqlient, *org, state.ID.ValueString())
		}

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
	state.AgentTracingServiceUUID = agentTracingServiceUUIDFromAPI(res.AgentTracingServiceUuid, state.AgentTracingServiceUUID)
}

// agentTracingServiceUUIDRegex accepts a lowercase UUID, or "" to clear the selection
var agentTracingServiceUUIDRegex = regexp.MustCompile(`^(|[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$`)

// agentTracingServiceUUIDToWrite returns the selection to send, or nil to leave it out of the mutation. The
// API rejects the key, even as null, for an organization without agent tracing, so it is sent only when a
// configured selection differs from the current one. An unconfigured attribute plans the current value, and
// "" is cleared by clearsAgentTracingService instead.
func agentTracingServiceUUIDToWrite(planned, current types.String) *string {
	if planned.IsNull() || planned.IsUnknown() || planned.ValueString() == "" || planned.Equal(current) {
		return nil
	}
	return planned.ValueStringPointer()
}

// clearsAgentTracingService reports whether a planned "" has a selection to clear. A null current value
// is already no selection as far as the API has said, so there is nothing to send.
func clearsAgentTracingService(planned, current types.String) bool {
	return !planned.IsUnknown() && planned.ValueString() == "" && !planned.IsNull() && current.ValueString() != ""
}

// agentTracingServiceUUIDFromAPI keeps a cleared "" that the API reads back as null, so clearing does not
// plan again on every refresh
func agentTracingServiceUUIDFromAPI(remote *string, prior types.String) types.String {
	if remote == nil && !prior.IsNull() && !prior.IsUnknown() && prior.ValueString() == "" {
		return prior
	}
	return types.StringPointerValue(remote)
}
