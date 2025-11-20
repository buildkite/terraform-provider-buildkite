package buildkite

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

const (
	// Available instance shapes
	MacARM64InstanceM4Medium string = "MACOS_ARM64_M4_6X28"
	MacARM64InstanceM4Large  string = "MACOS_ARM64_M4_12X56"
	LinuxAMD64InstanceSmall  string = "LINUX_AMD64_2X4"
	LinuxAMD64InstanceMedium string = "LINUX_AMD64_4X16"
	LinuxAMD64InstanceLarge  string = "LINUX_AMD64_8X32"
	LinuxAMD64InstanceXLarge string = "LINUX_AMD64_16X64"
	LinuxARM64InstanceSmall  string = "LINUX_ARM64_2X4"
	LinuxARM64InstanceMedium string = "LINUX_ARM64_4X16"
	LinuxARM64InstanceLarge  string = "LINUX_ARM64_8X32"
	LinuxARM64InstanceXLarge string = "LINUX_ARM64_16X64"

	// Available macOS versions
	MacOSSonoma  string = "SONOMA"
	MacOSSequoia string = "SEQUOIA"
	MacOSTahoe   string = "TAHOE"

	RetryAgentAffinityPreferWarmest   string = "prefer-warmest"
	RetryAgentAffinityPreferDifferent string = "prefer-different"
)

var MacInstanceShapes = []string{
	MacARM64InstanceM4Medium,
	MacARM64InstanceM4Large,
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

var MacOSVersions = []string{
	MacOSSonoma,
	MacOSSequoia,
	MacOSTahoe,
}

type clusterQueueResourceModel struct {
	Id                 types.String              `tfsdk:"id"`
	Uuid               types.String              `tfsdk:"uuid"`
	ClusterId          types.String              `tfsdk:"cluster_id"`
	ClusterUuid        types.String              `tfsdk:"cluster_uuid"`
	Key                types.String              `tfsdk:"key"`
	Description        types.String              `tfsdk:"description"`
	DispatchPaused     types.Bool                `tfsdk:"dispatch_paused"`
	RetryAgentAffinity types.String              `tfsdk:"retry_agent_affinity"`
	HostedAgents       *hostedAgentResourceModel `tfsdk:"hosted_agents"`
}

type hostedAgentResourceModel struct {
	Mac           *macConfigModel   `tfsdk:"mac"`
	Linux         *linuxConfigModel `tfsdk:"linux"`
	InstanceShape types.String      `tfsdk:"instance_shape"`
}

type macConfigModel struct {
	XcodeVersion types.String `tfsdk:"xcode_version"`
	MacosVersion types.String `tfsdk:"macos_version"`
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
			"retry_agent_affinity": resource_schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Specifies which agent should be preferred when a job is retried. Valid values are `prefer-warmest` (prefer agents that have recently finished jobs) and `prefer-different` (prefer a different agent if available). Defaults to `prefer-warmest`.",
				Default:             stringdefault.StaticString(RetryAgentAffinityPreferWarmest),
				Validators: []validator.String{
					stringvalidator.OneOf(RetryAgentAffinityPreferWarmest, RetryAgentAffinityPreferDifferent),
				},
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
								Required:    true,
								Description: "Required selection of a specific XCode version to be selected for jobs in the queue to have available. Please note that this value is currently experimental and may not function as expected.",
							},
							"macos_version": resource_schema.StringAttribute{
								Optional:    true,
								Description: "Optional selection of a specific macOS version to be selected for jobs in the queue to have available. Please note that this value is currently experimental and may not function as expected.",
							},
						},
					},
					"linux": resource_schema.SingleNestedAttribute{
						Optional: true,
						Attributes: map[string]resource_schema.Attribute{
							"agent_image_ref": resource_schema.StringAttribute{
								Required:    true,
								Description: "A URL reference to a container image that will be used for jobs running within the queue. This URL is required to be publicly available, or pushed to the internal registry available within the cluster. Please note that this value is currently experimental and in preview. Please contact support@buildkite.com to enable this functionality for your organization.",
							},
						},
					},
					"instance_shape": resource_schema.StringAttribute{
						Required: true,
						MarkdownDescription: heredoc.Doc(`
								The instance shape to use for the Hosted Agent cluster queue. This can be a MacOS instance shape or a Linux instance shape.
								Valid values are:
								- ` + strings.Join(MacInstanceShapes, "\n								- ") + `
								- ` + strings.Join(LinuxInstanceShapes, "\n								- ") + `
							`),
						Validators: []validator.String{
							stringvalidator.OneOf(
								append(MacInstanceShapes, LinuxInstanceShapes...)...,
							),
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

	hosted := (*HostedAgentsQueueSettingsCreateInput)(nil)
	if plan.HostedAgents != nil {
		hosted = &HostedAgentsQueueSettingsCreateInput{
			InstanceShape: HostedAgentInstanceShapeName(plan.HostedAgents.InstanceShape.ValueString()),
		}

		if plan.HostedAgents.Linux != nil {
			hosted.PlatformSettings = HostedAgentsPlatformSettingsInput{
				Linux: &HostedAgentsLinuxPlatformSettingsInput{
					AgentImageRef: plan.HostedAgents.Linux.ImageAgentRef.ValueStringPointer(),
				},
			}
		} else if plan.HostedAgents.Mac != nil {
			if plan.HostedAgents.Mac.MacosVersion.IsNull() {
				hosted.PlatformSettings = HostedAgentsPlatformSettingsInput{
					Macos: &HostedAgentsMacosPlatformSettingsInput{
						XcodeVersion: plan.HostedAgents.Mac.XcodeVersion.ValueStringPointer(),
					},
				}
			} else {
				version := HostedAgentMacOSVersion(plan.HostedAgents.Mac.MacosVersion.ValueString())

				hosted.PlatformSettings = HostedAgentsPlatformSettingsInput{
					Macos: &HostedAgentsMacosPlatformSettingsInput{
						XcodeVersion: plan.HostedAgents.Mac.XcodeVersion.ValueStringPointer(),
						MacosVersion: &version,
					},
				}
			}
		}
	}

	org, err := cq.client.GetOrganizationID()
	if err != nil {
		resp.Diagnostics.AddError("Unable to get organization ID", fmt.Sprintf("Failed to get organization ID: %s", err.Error()))
		return
	}

	log.Printf("Creating cluster queue with key %s into cluster %s ...", plan.Key.ValueString(), plan.ClusterId.ValueString())
	r, err := createClusterQueue(ctx,
		cq.client.genqlient,
		*org,
		plan.ClusterId.ValueString(),
		plan.Key.ValueString(),
		plan.Description.ValueStringPointer(),
		hosted,
	)
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

	state.DispatchPaused = types.BoolValue(false) // Start with false, update below if needed

	// GraphQL API does not allow Cluster Queue to be created with Dispatch Paused
	// so Pause Dispatch after creation if required
	if plan.DispatchPaused.ValueBool() {
		log.Printf("Pausing dispatch on cluster queue with key %s", plan.Key.ValueString())
		err = cq.pauseDispatch(ctx, timeout, state, &resp.Diagnostics)
		if err != nil {
			// Error is added to diagnostics within pauseDispatch
			return
		}
		state.DispatchPaused = types.BoolValue(true)
	}

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
			if r.ClusterQueueCreate.ClusterQueue.HostedAgents.PlatformSettings.Macos.MacosVersion == nil {
				state.HostedAgents.Mac = &macConfigModel{
					XcodeVersion: types.StringValue(r.ClusterQueueCreate.ClusterQueue.HostedAgents.PlatformSettings.Macos.XcodeVersion),
				}
			} else {
				state.HostedAgents.Mac = &macConfigModel{
					XcodeVersion: types.StringValue(r.ClusterQueueCreate.ClusterQueue.HostedAgents.PlatformSettings.Macos.XcodeVersion),
					MacosVersion: types.StringValue(string(*r.ClusterQueueCreate.ClusterQueue.HostedAgents.PlatformSettings.Macos.MacosVersion)),
				}
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

	// Timeout is not used here
	_, readDiags := cq.client.timeouts.Read(ctx, DefaultTimeout)
	resp.Diagnostics.Append(readDiags...)

	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("Getting cluster queue with ID %s using Node interface...", state.Id.ValueString())
	r, err := getClusterQueueByNode(ctx, cq.client.genqlient, state.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read Cluster Queue",
			fmt.Sprintf("Unable to read Cluster Queue: %s", err.Error()),
		)
		return
	}

	// Check if the node exists and is a ClusterQueue
	if r.Node == nil {
		resp.Diagnostics.AddWarning(
			"Cluster Queue not found",
			"Removing Cluster Queue from state...",
		)
		resp.State.RemoveResource(ctx)
		return
	}

	clusterQueue, ok := r.Node.(*getClusterQueueByNodeNodeClusterQueue)
	if !ok {
		resp.Diagnostics.AddError(
			"Invalid node type",
			"The returned node is not a ClusterQueue",
		)
		return
	}

	log.Printf("Found cluster queue with ID %s", clusterQueue.Id)
	// Update ClusterQueueResourceModel with Node values
	updateClusterQueueResourceFromNode(*clusterQueue, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
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
	hosted := (*HostedAgentsQueueSettingsUpdateInput)(nil)
	if state.HostedAgents != nil {
		hosted = &HostedAgentsQueueSettingsUpdateInput{
			InstanceShape: HostedAgentInstanceShapeName(plan.HostedAgents.InstanceShape.ValueString()),
		}

		// Set Hosted Agents input fields to nil to account for when
		// hosted_agents.linux or hosted_agents.mac attributes are removed
		// and a null can be sent in the clusterQueueUpdate mutation API call
		if state.HostedAgents.Linux != nil {
			hosted.PlatformSettings = HostedAgentsPlatformSettingsInput{
				Linux: &HostedAgentsLinuxPlatformSettingsInput{
					AgentImageRef: nil,
				},
			}
		} else if state.HostedAgents.Mac != nil {
			hosted.PlatformSettings = HostedAgentsPlatformSettingsInput{
				Macos: &HostedAgentsMacosPlatformSettingsInput{
					XcodeVersion: nil,
				},
			}
		}

		if plan.HostedAgents.Linux != nil {
			hosted.PlatformSettings.Linux = &HostedAgentsLinuxPlatformSettingsInput{
				AgentImageRef: plan.HostedAgents.Linux.ImageAgentRef.ValueStringPointer(),
			}
		} else if plan.HostedAgents.Mac != nil {
			hosted.PlatformSettings.Macos = &HostedAgentsMacosPlatformSettingsInput{
				XcodeVersion: plan.HostedAgents.Mac.XcodeVersion.ValueStringPointer(),
			}

			if !plan.HostedAgents.Mac.MacosVersion.IsNull() {
				version := HostedAgentMacOSVersion(plan.HostedAgents.Mac.MacosVersion.ValueString())
				hosted.PlatformSettings.Macos.MacosVersion = &version
			}
		}
	}

	org, err := cq.client.GetOrganizationID()
	if err != nil {
		resp.Diagnostics.AddError("Unable to get organization ID", fmt.Sprintf("Failed to get organization ID: %s", err.Error()))
		return
	}

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
			// Error added to diagnostics within pauseDispatch
			return
		}
	}

	// Planned to be false (changing from true to false)
	if !planDispatchPaused && stateDispatchPaused {
		if err := cq.resumeDispatch(ctx, timeout, state, &resp.Diagnostics); err != nil {
			// Error added to diagnostics within resumeDispatch
			return
		}
	}

	r, err = updateClusterQueue(ctx,
		cq.client.genqlient,
		*org,
		state.Id.ValueString(),
		plan.Description.ValueStringPointer(),
		hosted,
	)
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
			var version types.String

			if r.ClusterQueueUpdate.ClusterQueue.HostedAgents.PlatformSettings.Macos.MacosVersion != nil {
				version = types.StringValue(string(*r.ClusterQueueUpdate.ClusterQueue.HostedAgents.PlatformSettings.Macos.MacosVersion))
			}

			state.HostedAgents.Mac = &macConfigModel{
				XcodeVersion: types.StringValue(r.ClusterQueueUpdate.ClusterQueue.HostedAgents.PlatformSettings.Macos.XcodeVersion),
				MacosVersion: version,
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

	_, deleteDiags := cq.client.timeouts.Delete(ctx, DefaultTimeout)
	resp.Diagnostics.Append(deleteDiags...)

	if resp.Diagnostics.HasError() {
		return
	}

	org, err := cq.client.GetOrganizationID()
	if err != nil {
		resp.Diagnostics.AddError("Unable to get organization ID", fmt.Sprintf("Failed to get organization ID: %s", err.Error()))
		return
	}

	log.Printf("Deleting cluster queue %s ...", plan.Id.ValueString())
	_, err = deleteClusterQueue(ctx,
		cq.client.genqlient,
		*org,
		plan.Id.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete Cluster Queue",
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

func updateClusterQueueResourceFromNode(clusterQueueNode getClusterQueueByNodeNodeClusterQueue, cq *clusterQueueResourceModel) {
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
		if clusterQueueNode.HostedAgents.PlatformSettings.Linux.AgentImageRef != "" {
			cq.HostedAgents.Linux = &linuxConfigModel{
				ImageAgentRef: types.StringValue(clusterQueueNode.HostedAgents.PlatformSettings.Linux.AgentImageRef),
			}
		}
		if clusterQueueNode.HostedAgents.PlatformSettings.Macos.XcodeVersion != "" {
			if clusterQueueNode.HostedAgents.PlatformSettings.Macos.MacosVersion != nil {
				cq.HostedAgents.Mac = &macConfigModel{
					XcodeVersion: types.StringValue(clusterQueueNode.HostedAgents.PlatformSettings.Macos.XcodeVersion),
					MacosVersion: types.StringValue(string(*clusterQueueNode.HostedAgents.PlatformSettings.Macos.MacosVersion)),
				}
			} else {
				cq.HostedAgents.Mac = &macConfigModel{
					XcodeVersion: types.StringValue(clusterQueueNode.HostedAgents.PlatformSettings.Macos.XcodeVersion),
				}
			}
		}
	}
}

func (cq *clusterQueueResource) pauseDispatch(ctx context.Context, timeout time.Duration, state clusterQueueResourceModel, diag *diag.Diagnostics) error {
	log.Printf("Pausing dispatch for cluster queue %s", state.Key.ValueString())
	_, err := pauseDispatchClusterQueue(ctx, cq.client.genqlient, state.Id.ValueString())
	if err != nil {
		diag.AddError(
			"Unable to pause Cluster Queue dispatch",
			fmt.Sprintf("Unable to pause dispatch for cluster queue: %s", err.Error()),
		)
	}

	return err
}

func (cq *clusterQueueResource) resumeDispatch(ctx context.Context, timeout time.Duration, state clusterQueueResourceModel, diag *diag.Diagnostics) error {
	log.Printf("Resuming dispatch for cluster queue %s", state.Key.ValueString())
	_, err := resumeDispatchClusterQueue(ctx, cq.client.genqlient, state.Id.ValueString())
	if err != nil {
		diag.AddError(
			"Unable to resume Cluster Queue dispatch",
			fmt.Sprintf("Unable to resume dispatch for cluster queue: %s", err.Error()),
		)
	}

	return err
}

type clusterQueueRestResponse struct {
	RetryAgentAffinity string `json:"retry_agent_affinity"`
}

func (cq *clusterQueueResource) getClusterQueueViaREST(ctx context.Context, clusterUuid, queueUuid string) (*clusterQueueRestResponse, error) {
	if clusterUuid == "" || queueUuid == "" {
		return nil, fmt.Errorf("clusterUuid and queueUuid must not be empty")
	}
	path := fmt.Sprintf("/v2/organizations/%s/clusters/%s/queues/%s", cq.client.organization, clusterUuid, queueUuid)
	var response clusterQueueRestResponse
	err := cq.client.makeRequest(ctx, "GET", path, nil, &response)
	if err != nil {
		return nil, err
	}
	if response.RetryAgentAffinity != RetryAgentAffinityPreferWarmest && response.RetryAgentAffinity != RetryAgentAffinityPreferDifferent {
		return nil, fmt.Errorf("invalid retry_agent_affinity value: %s", response.RetryAgentAffinity)
	}
	return &response, nil
}

func (cq *clusterQueueResource) updateClusterQueueViaREST(ctx context.Context, clusterUuid, queueUuid string, retryAgentAffinity string) error {
	if clusterUuid == "" || queueUuid == "" {
		return fmt.Errorf("clusterUuid and queueUuid must not be empty")
	}
	if retryAgentAffinity == "" {
		return fmt.Errorf("retryAgentAffinity must not be empty")
	}
	path := fmt.Sprintf("/v2/organizations/%s/clusters/%s/queues/%s", cq.client.organization, clusterUuid, queueUuid)
	payload := map[string]interface{}{
		"retry_agent_affinity": retryAgentAffinity,
	}
	return cq.client.makeRequest(ctx, "PATCH", path, payload, nil)
}

func (cq *clusterQueueResource) syncRetryAgentAffinity(ctx context.Context, plan, state *clusterQueueResourceModel) (types.String, error) {
	if state.ClusterUuid.IsNull() || state.Uuid.IsNull() {
		return types.StringNull(), fmt.Errorf("cluster_uuid and uuid must be set before syncing retry_agent_affinity")
	}

	desiredValue := RetryAgentAffinityPreferWarmest
	if !plan.RetryAgentAffinity.IsNull() && !plan.RetryAgentAffinity.IsUnknown() {
		desiredValue = plan.RetryAgentAffinity.ValueString()
	}

	currentValue := state.RetryAgentAffinity.ValueString()

	if currentValue == "" || currentValue != desiredValue {
		err := cq.updateClusterQueueViaREST(ctx, state.ClusterUuid.ValueString(), state.Uuid.ValueString(), desiredValue)
		if err != nil {
			return types.StringNull(), err
		}
	}

	return types.StringValue(desiredValue), nil
}
