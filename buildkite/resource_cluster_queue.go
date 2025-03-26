package buildkite

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

const (
	// Available instance shapes
	MacInstanceSmall         string = "MACOS_M2_4X7"
	MacInstanceMedium        string = "MACOS_M2_6X14"
	MacInstanceLarge         string = "MACOS_M2_12X28"
	MacInstanceXLarge        string = "MACOS_M4_12X56"
	LinuxAMD64InstanceSmall  string = "LINUX_AMD64_2X4"
	LinuxAMD64InstanceMedium string = "LINUX_AMD64_4X16"
	LinuxAMD64InstanceLarge  string = "LINUX_AMD64_8X32"
	LinuxAMD64InstanceXLarge string = "LINUX_AMD64_16X64"
	LinuxARM64InstanceSmall  string = "LINUX_ARM64_2X4"
	LinuxARM64InstanceMedium string = "LINUX_ARM64_4X16"
	LinuxARM64InstanceLarge  string = "LINUX_ARM64_8X32"
	LinuxARM64InstanceXLarge string = "LINUX_ARM64_16X64"
)

var MacInstanceShapes = []string{
	MacInstanceSmall,
	MacInstanceMedium,
	MacInstanceLarge,
	MacInstanceXLarge,
}

var LinuxInstanceShapes = []string{
	LinuxAMD64InstanceSmall,
	LinuxAMD64InstanceMedium,
	LinuxAMD64InstanceLarge,
	LinuxAMD64InstanceXLarge,
	LinuxARM64InstanceSmall,
	LinuxARM64InstanceMedium,
	LinuxARM64InstanceLarge,
	LinuxARM64InstanceXLarge,
}

type clusterQueueResourceModel struct {
	Id             types.String              `tfsdk:"id"`
	Uuid           types.String              `tfsdk:"uuid"`
	ClusterId      types.String              `tfsdk:"cluster_id"`
	ClusterUuid    types.String              `tfsdk:"cluster_uuid"`
	Key            types.String              `tfsdk:"key"`
	Description    types.String              `tfsdk:"description"`
	DispatchPaused types.Bool                `tfsdk:"dispatch_paused"`
	HostedAgents   *hostedAgentResourceModel `tfsdk:"hosted_agents"`
}

type hostedAgentResourceModel struct {
	Mac           *macConfigModel   `tfsdk:"mac"`
	Linux         *linuxConfigModel `tfsdk:"linux"`
	InstanceShape types.String      `tfsdk:"instance_shape"`
}

type macConfigModel struct {
	XcodeVersion types.String `tfsdk:"xcode_version"`
}

type linuxConfigModel struct {
	ImageAgentRef types.String `tfsdk:"agent_image_ref"`
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
			"hosted_agents": resource_schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Control the settings for the Buildkite hosted agents.",
				Validators: []validator.Object{
					&hostedAgentValidator{},
				},
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.RequiresReplaceIf(func(ctx context.Context, or planmodifier.ObjectRequest, rrifr *objectplanmodifier.RequiresReplaceIfFuncResponse) {
						var data *struct {
							Mac           types.Object `tfsdk:"mac"`
							Linux         types.Object `tfsdk:"linux"`
							InstanceShape types.String `tfsdk:"instance_shape"`
						}

						rrifr.Diagnostics.Append(or.ConfigValue.As(ctx, &data, basetypes.ObjectAsOptions{})...)

						if rrifr.Diagnostics.HasError() {
							return
						}

						// If the hosted_agents attribute is added or removed e.g., change from a Hosted Agent to Self-Hosted Agent Queue
						if or.StateValue.IsNull() && !or.PlanValue.IsNull() || or.ConfigValue.IsNull() {
							rrifr.RequiresReplace = true
							return
						}

						rrifr.RequiresReplace = false
					}, "Recreates the resource if the hosted_agents attribute is added or removed.", "Recreates the resource if the hosted_agents attribute is added or removed."),
				},
				Attributes: map[string]resource_schema.Attribute{
					"mac": resource_schema.SingleNestedAttribute{
						Optional: true,
						Attributes: map[string]resource_schema.Attribute{
							"xcode_version": resource_schema.StringAttribute{
								Optional:    true,
								Computed:    true,
								Description: "Optional selection of a specific XCode version to be selected for jobs in the queue to have available. Please note that this value is currently experimental and may not function as expected.",
							},
						},
					},
					"linux": resource_schema.SingleNestedAttribute{
						Optional: true,
						Attributes: map[string]resource_schema.Attribute{
							"agent_image_ref": resource_schema.StringAttribute{
								Optional:    true,
								Computed:    true,
								Description: "A URL reference to a container image that will be used for jobs running within the queue. This URL is required to be publicly available, or pushed to the internal registry available within the cluster. Please note that this value is currently experimental and in preview. Please contact support@buildkite.com to enable this functionality for your organization.",
							},
						},
					},
					"instance_shape": resource_schema.StringAttribute{
						Required: true,
						Validators: []validator.String{
							stringvalidator.OneOf(
								append(MacInstanceShapes, LinuxInstanceShapes...)...,
							),
						},
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.RequiresReplace(),
						},
					},
				},
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

	var hosted *HostedAgentsQueueSettingsCreateInput
	if plan.HostedAgents != nil {
		hosted = &HostedAgentsQueueSettingsCreateInput{
			InstanceShape:    HostedAgentInstanceShapeName(plan.HostedAgents.InstanceShape.ValueString()),
			PlatformSettings: HostedAgentsPlatformSettingsInput{},
		}

		if plan.HostedAgents.Linux != nil {
			hosted.PlatformSettings.Linux = HostedAgentsLinuxPlatformSettingsInput{
				AgentImageRef: plan.HostedAgents.Linux.ImageAgentRef.ValueString(),
			}
		}
		if plan.HostedAgents.Mac != nil {
			hosted.PlatformSettings.Macos = HostedAgentsMacosPlatformSettingsInput{
				XcodeVersion: plan.HostedAgents.Mac.XcodeVersion.ValueString(),
			}
		}
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
				hosted,
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
	// so Pause Dispatch after creation
	if plan.DispatchPaused.ValueBool() {
		err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
			log.Printf("Pausing dispatch on cluster queue with key %s", plan.Key.ValueString())
			return retryContextError(cq.pauseDispatch(ctx, timeout, state, &resp.Diagnostics))
		})
		if err != nil {
			return
		}
	}
	state.DispatchPaused = plan.DispatchPaused

	if plan.HostedAgents != nil {
		state.HostedAgents = &hostedAgentResourceModel{
			InstanceShape: types.StringValue(string(r.ClusterQueueCreate.ClusterQueue.HostedAgents.InstanceShape.Name)),
		}
		if plan.HostedAgents.Linux != nil {
			state.HostedAgents.Linux = &linuxConfigModel{
				ImageAgentRef: types.StringValue(r.ClusterQueueCreate.ClusterQueue.HostedAgents.PlatformSettings.Linux.AgentImageRef),
			}
		}
		if plan.HostedAgents.Mac != nil {
			state.HostedAgents.Mac = &macConfigModel{
				XcodeVersion: plan.HostedAgents.Mac.XcodeVersion,
			}
		}
	}

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

	var matchFound bool
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var cursor *string
		for {
			log.Printf("Getting cluster queues for cluster %s ...", state.ClusterUuid.ValueString())
			r, err := getClusterQueues(ctx, cq.client.genqlient, cq.client.organization, state.ClusterUuid.ValueString(), cursor)
			if err != nil {
				if isRetryableError(err) {
					return retry.RetryableError(err)
				}
				resp.Diagnostics.AddError(
					"Unable to read Cluster Queues",
					fmt.Sprintf("Unable to read Cluster Queues: %s", err.Error()),
				)
				return retry.NonRetryableError(err)
			}

			// Find the cluster queue from the returned queues to update state
			for _, edge := range r.Organization.Cluster.Queues.Edges {
				if edge.Node.Id == state.Id.ValueString() {
					matchFound = true
					log.Printf("Found cluster queue with ID %s in cluster %s", edge.Node.Id, state.ClusterUuid.ValueString())
					// Update ClusterQueueResourceModel with Node values and append
					updateClusterQueueResource(edge.Node, &state)
					resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
					break
				}
			}

			// end here if we found a match or there are no more pages to search
			if matchFound || !r.Organization.Cluster.Queues.PageInfo.HasNextPage {
				break
			}
			cursor = &r.Organization.Cluster.Queues.PageInfo.EndCursor
		}
		return nil
	})

	// Cluster queue could not be found in returned queues and should be removed from state
	if !matchFound || err != nil {
		resp.Diagnostics.AddWarning(
			"Cluster Queue not found",
			"Removing Cluster Queue from state...",
		)
		resp.State.RemoveResource(ctx)
	}
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
	var hosted *HostedAgentsQueueSettingsUpdateInput
	if state.HostedAgents != nil {
		hosted = &HostedAgentsQueueSettingsUpdateInput{
			InstanceShape:    HostedAgentInstanceShapeName(plan.HostedAgents.InstanceShape.ValueString()),
			PlatformSettings: HostedAgentsPlatformSettingsInput{},
		}

		if plan.HostedAgents.Linux != nil {
			hosted.PlatformSettings.Linux = HostedAgentsLinuxPlatformSettingsInput{
				AgentImageRef: plan.HostedAgents.Linux.ImageAgentRef.ValueString(),
			}
		}
		if plan.HostedAgents.Mac != nil {
			hosted.PlatformSettings.Macos = HostedAgentsMacosPlatformSettingsInput{
				XcodeVersion: plan.HostedAgents.Mac.XcodeVersion.ValueString(),
			}
		}
	}

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
				hosted,
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
	if state.HostedAgents != nil {
		state.HostedAgents = &hostedAgentResourceModel{
			InstanceShape: types.StringValue(string(r.ClusterQueueUpdate.ClusterQueue.HostedAgents.InstanceShape.Name)),
		}
		if plan.HostedAgents.Mac != nil {
			state.HostedAgents.Mac = &macConfigModel{
				XcodeVersion: types.StringValue(r.ClusterQueueUpdate.ClusterQueue.HostedAgents.PlatformSettings.Macos.XcodeVersion),
			}
		}
		if plan.HostedAgents.Linux != nil {
			state.HostedAgents.Linux = &linuxConfigModel{
				ImageAgentRef: types.StringValue(r.ClusterQueueUpdate.ClusterQueue.HostedAgents.PlatformSettings.Linux.AgentImageRef),
			}
		}
	}

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

type hostedAgentValidator struct{}

func (v hostedAgentValidator) Description(ctx context.Context) string {
	return "validates platform and instance shape compatibility"
}

func (v hostedAgentValidator) MarkdownDescription(ctx context.Context) string {
	return "Validates that the instance shape is compatible with the selected platform"
}

func (v hostedAgentValidator) ValidateObject(ctx context.Context, req validator.ObjectRequest, resp *validator.ObjectResponse) {
	// Skip validation if config is null
	if req.ConfigValue.IsNull() {
		return
	}

	var data struct {
		Mac           types.Object `tfsdk:"mac"`
		Linux         types.Object `tfsdk:"linux"`
		InstanceShape types.String `tfsdk:"instance_shape"`
	}

	diags := req.ConfigValue.As(ctx, &data, basetypes.ObjectAsOptions{})
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Skip if no instance shape is specified
	if data.InstanceShape.IsNull() || data.InstanceShape.IsUnknown() {
		return
	}

	shape := data.InstanceShape.ValueString()
	hasMac := !data.Mac.IsNull() && !data.Mac.IsUnknown()
	hasLinux := !data.Linux.IsNull() && !data.Linux.IsUnknown()

	// Validate that only one platform is specified
	if hasMac && hasLinux {
		resp.Diagnostics.AddAttributeError(
			path.Root("hosted_agents"),
			"Invalid platform configuration",
			"Only one platform (mac or linux) can be specified at a time",
		)
		return
	}

	// Validate Mac shapes
	if hasMac {
		isValid := false
		for _, validShape := range MacInstanceShapes {
			if shape == validShape {
				isValid = true
				break
			}
		}
		if !isValid {
			resp.Diagnostics.AddAttributeError(
				path.Root("instance_shape"),
				"Invalid instance shape for Mac platform",
				fmt.Sprintf("Instance shape %s is not valid for Mac platform. Valid shapes are: %v", shape, MacInstanceShapes),
			)
		}
	}

	// Validate Linux shapes
	if hasLinux {
		isValid := false
		for _, validShape := range LinuxInstanceShapes {
			if shape == validShape {
				isValid = true
				break
			}
		}
		if !isValid {
			resp.Diagnostics.AddAttributeError(
				path.Root("instance_shape"),
				"Invalid instance shape for Linux platform",
				fmt.Sprintf("Instance shape %s is not valid for Linux platform. Valid shapes are: %v", shape, LinuxInstanceShapes),
			)
		}
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

	if clusterQueueNode.Hosted {
		cq.HostedAgents = &hostedAgentResourceModel{
			InstanceShape: types.StringValue(string(clusterQueueNode.HostedAgents.InstanceShape.Name)),
		}
		if len(clusterQueueNode.HostedAgents.PlatformSettings.Linux.AgentImageRef) > 0 {
			cq.HostedAgents.Linux = &linuxConfigModel{
				ImageAgentRef: types.StringValue(clusterQueueNode.HostedAgents.PlatformSettings.Linux.AgentImageRef),
			}
		}
		if len(clusterQueueNode.HostedAgents.PlatformSettings.Macos.XcodeVersion) > 0 {
			cq.HostedAgents.Mac = &macConfigModel{
				XcodeVersion: types.StringValue(clusterQueueNode.HostedAgents.PlatformSettings.Macos.XcodeVersion),
			}
		}
	}
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
