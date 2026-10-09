package buildkite

import (
	"context"
	"fmt"
	"net/http"
	"regexp"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
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
	ID                          types.String `tfsdk:"id"`
	Name                        types.String `tfsdk:"name"`
	Description                 types.String `tfsdk:"description"`
	Emoji                       types.String `tfsdk:"emoji"`
	Color                       types.String `tfsdk:"color"`
	UUID                        types.String `tfsdk:"uuid"`
	AgentTracingServiceUUID     types.String `tfsdk:"agent_tracing_service_uuid"`
	HostedGitMirrorEnabled      types.Bool   `tfsdk:"hosted_git_mirror_enabled"`
	HostedContainerCacheEnabled types.Bool   `tfsdk:"hosted_container_cache_enabled"`
}

// hostedSettingsRequirement explains why the hosted cache settings cannot be enabled on a new cluster: the
// API refuses to change them until the cluster has a hosted queue, and a queue needs the cluster first.
const hostedSettingsRequirement = "hosted_git_mirror_enabled and hosted_container_cache_enabled can only be changed " +
	"once the Cluster has at least one hosted queue (a buildkite_cluster_queue with hosted_agents). " +
	"A new Cluster has both disabled, so they can only be set to false when it is created, and a tainted, " +
	"replaced, or externally deleted Cluster is created again from scratch. " +
	"Create the Cluster and its hosted queue first, then enable these attributes in a later apply. " +
	"A value not known until apply is refused when creating too, since it could be true: use a literal false, " +
	"or leave the attribute unset. " +
	"If an apply is failing on this, remove the attributes, apply so the hosted queue is created, then add them back."

// hostedSettingsAccess ends both hosted cache settings' descriptions
const hostedSettingsAccess = "Reading and changing it is done through the REST API, so the API token needs the `read_clusters`\n" +
	"and `write_clusters` scopes and permission to manage the Cluster. Without `read_clusters` or permission to\n" +
	"manage the Cluster, the last known value is kept, and a change made outside Terraform is not detected. The\n" +
	"same happens, with a warning, when a read fails for another reason, such as the REST API rate limit.\n"

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

var _ resource.ResourceWithModifyPlan = &clusterResource{}

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
			"hosted_git_mirror_enabled": resource_schema.BoolAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: heredoc.Doc(`
					Whether hosted agents in this Cluster keep a git mirror of the repositories they check out, to
					speed up checkouts. This only applies to a Cluster with at least one hosted queue: a new Cluster has
					it disabled, so it can only be set to false when the Cluster is created, and the API refuses to
					change it until the Cluster has a hosted queue, so enable it in a later apply. Changing it is synced
					to the hosted agents platform, and the change fails if that sync does. Leaving this unset adopts the
					Cluster's current setting.
				`) + hostedSettingsAccess,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"hosted_container_cache_enabled": resource_schema.BoolAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: heredoc.Doc(`
					Whether hosted agents in this Cluster cache container images between jobs. This only applies to a
					Cluster with at least one hosted queue: a new Cluster has it disabled, so it can only be set to false
					when the Cluster is created, and the API refuses to change it until the Cluster has a hosted queue,
					so enable it in a later apply. Buildkite enables it when the Cluster's first hosted queue is
					created, so a configured false plans one more update after that queue is added. Leaving this unset
					adopts the Cluster's current setting.
				`) + hostedSettingsAccess,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

// ModifyPlan refuses to enable the hosted cache settings on a new Cluster, since the API would refuse that
// anyway once the Cluster had been created, and Terraform would then taint it. A new Cluster has both
// disabled, so false needs no write and is accepted. A value not known until apply could be true.
func (c *clusterResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if !req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	for _, attribute := range []string{"hosted_git_mirror_enabled", "hosted_container_cache_enabled"} {
		var value types.Bool
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(attribute), &value)...)
		if value.IsUnknown() || value.ValueBool() {
			resp.Diagnostics.AddAttributeError(
				path.Root(attribute),
				"Cannot enable "+attribute+" when creating or replacing a Cluster",
				hostedSettingsRequirement,
			)
		}
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

	requestCtx, cancel := mutationContext(ctx, timeout)
	defer cancel()

	var r *createClusterResponse
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		org, err := c.client.GetOrganizationID(requestCtx)
		if err == nil {
			r, err = createCluster(
				requestCtx,
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

	// ModifyPlan keeps the hosted cache settings out of a create, so they are only read here. The cluster
	// exists already, so a failed read is a warning that leaves them for the next refresh, not a taint.
	settings, err := c.getClusterHostedSettings(ctx, state.UUID.ValueString())
	if err != nil {
		resp.Diagnostics.AddWarning(
			"Unable to read the Cluster's hosted cache settings",
			fmt.Sprintf("Cluster %s was created, but its hosted cache settings could not be read: %s", state.Name.ValueString(), err.Error()),
		)
		settings = &clusterHostedSettings{}
	}
	setClusterHostedSettings(state, settings)

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

		settings, err := c.getClusterHostedSettings(ctx, state.UUID.ValueString())
		if err != nil {
			// The settings come over REST, which can fail where GraphQL has just read the cluster: a token
			// without the read_clusters scope is refused, a cluster outside the provider's organization is
			// not found, and REST has its own rate limit. None of that should fail a refresh that may not
			// involve these settings at all, so the last known values are kept. A refusal with nothing to
			// keep is a token that never reads them, which is not worth a warning on every refresh.
			refused := isAPIStatus(err, http.StatusForbidden) || isAPIStatus(err, http.StatusNotFound)
			if !refused || !state.HostedGitMirrorEnabled.IsNull() || !state.HostedContainerCacheEnabled.IsNull() {
				resp.Diagnostics.AddWarning(
					"Unable to read the Cluster's hosted cache settings",
					fmt.Sprintf("Cluster %s was read, but its hosted cache settings could not be, keeping the last known values. Reading them needs the read_clusters scope and a Cluster in the provider's organization: %s", state.Name.ValueString(), err.Error()),
				)
			}
			settings = &clusterHostedSettings{}
		}
		setClusterHostedSettings(&state, settings)

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

	// An update that only changes the hosted cache settings has nothing to send over GraphQL. The settings
	// written over REST are taken out of the comparison rather than the GraphQL ones listed, so an attribute
	// added to the mutation later is sent without anyone having to remember this.
	graphQLPlan := plan
	graphQLPlan.HostedGitMirrorEnabled = state.HostedGitMirrorEnabled
	graphQLPlan.HostedContainerCacheEnabled = state.HostedContainerCacheEnabled

	if graphQLPlan != state {
		requestCtx, cancel := mutationContext(ctx, timeout)
		defer cancel()

		err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
			org, err := c.client.GetOrganizationID(requestCtx)
			if err == nil && clearsAgentTracingService(plan.AgentTracingServiceUUID, state.AgentTracingServiceUUID) {
				_, err = updateClusterClearingAgentTracingService(requestCtx,
					c.client.genqlient,
					*org,
					state.ID.ValueString(),
					plan.Name.ValueString(),
					plan.Description.ValueStringPointer(),
					plan.Emoji.ValueStringPointer(),
					plan.Color.ValueStringPointer(),
				)
			} else if err == nil {
				_, err = updateCluster(requestCtx,
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
	}

	// GraphQL has no hosted cache settings, so they are a second write, over REST. The plan already holds
	// what this sends and, for a setting it leaves out, what state holds, so the response is not recorded:
	// it would also fill a setting planned as null, which Terraform refuses as an inconsistent result.
	if payload := hostedSettingsToWrite(plan, state); payload != nil {
		settingsCtx, cancel := mutationContext(ctx, timeout)
		defer cancel()

		if err := c.updateClusterHostedSettings(settingsCtx, state.UUID.ValueString(), payload); err != nil {
			// the rest of the update has applied, so record it with the settings as they were
			plan.HostedGitMirrorEnabled = state.HostedGitMirrorEnabled
			plan.HostedContainerCacheEnabled = state.HostedContainerCacheEnabled
			resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
			resp.Diagnostics.AddError(
				"Unable to update the Cluster's hosted cache settings",
				fmt.Sprintf("Unable to update the hosted cache settings of Cluster %s: %s", plan.Name.ValueString(), err.Error()),
			)
			return
		}
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

	requestCtx, cancel := mutationContext(ctx, timeout)
	defer cancel()

	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		org, err := c.client.GetOrganizationID(requestCtx)
		if err == nil {
			_, err = deleteCluster(requestCtx, c.client.genqlient, *org, state.ID.ValueString())
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

// clusterHostedSettings is both the PATCH body, where an absent key leaves that setting alone, and the
// REST cluster's settings, which the API leaves out for a token that cannot manage the cluster
type clusterHostedSettings struct {
	HostedGitMirrorEnabled      *bool `json:"hosted_git_mirror_enabled,omitempty"`
	HostedContainerCacheEnabled *bool `json:"hosted_container_cache_enabled,omitempty"`
}

func (c *clusterResource) getClusterHostedSettings(ctx context.Context, clusterUUID string) (*clusterHostedSettings, error) {
	var settings clusterHostedSettings
	path := fmt.Sprintf("/v2/organizations/%s/clusters/%s", c.client.organization, clusterUUID)
	if err := c.client.makeRequest(ctx, http.MethodGet, path, nil, &settings); err != nil {
		return nil, err
	}
	return &settings, nil
}

func (c *clusterResource) updateClusterHostedSettings(ctx context.Context, clusterUUID string, payload *clusterHostedSettings) error {
	path := fmt.Sprintf("/v2/organizations/%s/clusters/%s", c.client.organization, clusterUUID)
	// the response is not recorded, see Update, but makeRequest decodes it and fails on a nil target
	err := c.client.makeRequest(ctx, http.MethodPatch, path, payload, &struct{}{})
	// with booleans to send, the API only refuses them for a cluster without a hosted queue
	if isAPIStatus(err, http.StatusUnprocessableEntity) {
		return fmt.Errorf("%w\n\n%s", err, hostedSettingsRequirement)
	}
	return err
}

// hostedSettingsToWrite returns the configured settings that differ from the current ones, or nil when
// there are none. Sending only those keeps a git mirror sync out of an update that does not change it.
func hostedSettingsToWrite(plan, current clusterResourceModel) *clusterHostedSettings {
	changed := func(planned, existing types.Bool) *bool {
		if planned.IsNull() || planned.IsUnknown() || planned.Equal(existing) {
			return nil
		}
		return planned.ValueBoolPointer()
	}

	payload := clusterHostedSettings{
		HostedGitMirrorEnabled:      changed(plan.HostedGitMirrorEnabled, current.HostedGitMirrorEnabled),
		HostedContainerCacheEnabled: changed(plan.HostedContainerCacheEnabled, current.HostedContainerCacheEnabled),
	}
	if payload.HostedGitMirrorEnabled == nil && payload.HostedContainerCacheEnabled == nil {
		return nil
	}
	return &payload
}

// setClusterHostedSettings records the settings the API reported. One it left out keeps the last known
// value, or becomes null where there is none, rather than failing the read.
func setClusterHostedSettings(state *clusterResourceModel, settings *clusterHostedSettings) {
	fromAPI := func(remote *bool, prior types.Bool) types.Bool {
		if remote != nil {
			return types.BoolValue(*remote)
		}
		if prior.IsUnknown() {
			return types.BoolNull()
		}
		return prior
	}

	state.HostedGitMirrorEnabled = fromAPI(settings.HostedGitMirrorEnabled, state.HostedGitMirrorEnabled)
	state.HostedContainerCacheEnabled = fromAPI(settings.HostedContainerCacheEnabled, state.HostedContainerCacheEnabled)
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
