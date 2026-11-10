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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

type clusterAgentToken struct {
	client *Client
}

type clusterAgentTokenResourceModel struct {
	Id                 types.String `tfsdk:"id"`
	Uuid               types.String `tfsdk:"uuid"`
	Description        types.String `tfsdk:"description"`
	Token              types.String `tfsdk:"token"`
	ClusterId          types.String `tfsdk:"cluster_id"`
	ClusterUuid        types.String `tfsdk:"cluster_uuid"`
	AllowedIpAddresses types.List   `tfsdk:"allowed_ip_addresses"`
	ExpiresAt          types.String `tfsdk:"expires_at"`
	Permissions        types.Set    `tfsdk:"permissions"`
	AllowAllQueues     types.Bool   `tfsdk:"allow_all_queues"`
	AllowedQueueIds    types.Set    `tfsdk:"allowed_queue_ids"`
}

func newClusterAgentTokenResource() resource.Resource {
	return &clusterAgentToken{}
}

func (ct *clusterAgentToken) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_agent_token"
}

func (ct *clusterAgentToken) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	ct.client = req.ProviderData.(*Client)
}

func (ct *clusterAgentToken) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Cluster Agent Token is a token used to connect an agent to a cluster in Buildkite. " +
			"Import using the token's GraphQL ID. Import reads the token's metadata, permissions and queue bindings, but cannot recover its secret value.\n\n" +
			"Queue access is independent of action permissions: selecting a queue does not grant `run_jobs` or other permissions. " +
			"Queue-binding changes do not disconnect existing agents or cancel running jobs. " +
			"Scoped agent-token permissions must be enabled for the organization to manage queue access. When the API does not expose queue access, the last known settings are retained, but drift cannot be detected.",
		Attributes: map[string]resource_schema.Attribute{
			"id": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The GraphQL ID of the token.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"uuid": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the token.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"description": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "A description about what this cluster agent token is used for.",
			},
			"token": resource_schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "The token value used by an agent to register with the API.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"cluster_id": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The GraphQL ID of the Cluster that this Cluster Agent Token belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"cluster_uuid": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the Cluster that this token belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"allowed_ip_addresses": schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				MarkdownDescription: "A list of CIDR-notation IPv4 addresses from which agents can use this Cluster Agent Token." +
					"If not set, all IP addresses are allowed (the same as setting 0.0.0.0/0).",
			},
			"expires_at": resource_schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The time at which the token expires, in RFC 3339 format with up to millisecond precision (e.g. `2030-01-01T00:00:00Z`) and at least 10 minutes in the future. If not set, the token never expires. Changing this value replaces the token, since the expiry cannot be updated.",
				Validators: []validator.String{
					rfc3339Validator{},
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"permissions": schema.SetAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				MarkdownDescription: "The token's permission allow list. Values are validated by the Buildkite API. " +
					"Omitting this grants the API's default permissions on creation and leaves the stored scope unchanged on updates, including when removing this attribute from configuration. " +
					"An explicit empty set (`[]`) grants no permissions. Configured permissions are managed as an unordered set, and changes made outside Terraform are detected. " +
					"Scoped agent-token permissions must be enabled for the organization. When the API does not expose permissions, the last known scope is retained.",
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
			},
			"allow_all_queues": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Whether the token can access every queue in its cluster. " +
					"Omitting this uses the API's default of `true` on creation and leaves the stored mode unchanged on updates, including when removing it from configuration. " +
					"Creating a restricted token requires `false` and an explicit `allowed_queue_ids` set. " +
					"To switch a token with queue bindings to all queues, explicitly set this to `true` and `allowed_queue_ids = []` together.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"allowed_queue_ids": schema.SetAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				MarkdownDescription: "The complete, unordered set of GraphQL queue IDs this token can access when `allow_all_queues` is `false`, such as `buildkite_cluster_queue.workers.id`. " +
					"Queues must belong to the token's cluster. Omitting this uses the API's empty selection on creation and preserves the stored selection on updates, including when removing it from configuration. " +
					"An empty set (`[]`) does not change the access mode: with `allow_all_queues = false` it denies every queue. " +
					"Nonempty bindings cannot be combined with all-queues access. Configured bindings detect changes made outside Terraform. " +
					"Bindings do not select an agent's queue: agents must target an allowed queue, or the cluster's default queue must be allowed when no queue tag is supplied.",
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (ct *clusterAgentToken) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, state clusterAgentTokenResourceModel

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	permissions, diags := clusterTokenStringSetToWrite(ctx, plan.Permissions)
	resp.Diagnostics.Append(diags...)
	allowedQueueIds, diags := clusterTokenStringSetToWrite(ctx, plan.AllowedQueueIds)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := ct.client.createTimeout(ctx)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Create CIDR slice from AllowedApiIpAddresses in the plan
	cidrs := createCidrSliceFromList(plan.AllowedIpAddresses)

	var expiresAt *time.Time
	if !plan.ExpiresAt.IsNull() {
		// already checked by the validator
		t, _ := time.Parse(time.RFC3339, plan.ExpiresAt.ValueString())
		expiresAt = &t
	}

	var r *createClusterAgentTokenResponse
	err := retryMutation(ctx, timeout, func(ctx context.Context) *retry.RetryError {
		org, err := ct.client.GetOrganizationID(ctx)
		if err == nil {

			log.Printf("Creating cluster agent token with description %s into cluster %s ...", plan.Description.ValueString(), plan.ClusterId.ValueString())
			r, err = createClusterAgentToken(ctx,
				ct.client.genqlient,
				*org,
				plan.ClusterId.ValueString(),
				plan.Description.ValueString(),
				strings.Join(cidrs, " "),
				expiresAt,
				permissions,
				clusterTokenQueueModeToWrite(plan.AllowAllQueues),
				allowedQueueIds,
			)
		}

		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create Cluster Agent Token",
			fmt.Sprintf("Unable to create Cluster Agent Token: %s", err.Error()),
		)
		return
	}

	state.Id = types.StringValue(r.ClusterAgentTokenCreate.ClusterAgentToken.Id)
	state.Uuid = types.StringValue(r.ClusterAgentTokenCreate.ClusterAgentToken.Uuid)
	state.Description = types.StringValue(r.ClusterAgentTokenCreate.ClusterAgentToken.Description)
	state.Token = types.StringValue(r.ClusterAgentTokenCreate.TokenValue)
	state.ClusterId = types.StringValue(r.ClusterAgentTokenCreate.ClusterAgentToken.Cluster.Id)
	state.ClusterUuid = types.StringValue(r.ClusterAgentTokenCreate.ClusterAgentToken.Cluster.Uuid)
	state.AllowedIpAddresses = plan.AllowedIpAddresses
	state.ExpiresAt = plan.ExpiresAt
	state.Permissions, diags = clusterTokenPermissionsFromAPI(ctx, r.ClusterAgentTokenCreate.ClusterAgentToken.Permissions, plan.Permissions)
	resp.Diagnostics.Append(diags...)
	state.AllowAllQueues = plan.AllowAllQueues
	state.AllowedQueueIds = plan.AllowedQueueIds
	resp.Diagnostics.Append(state.setQueueAccess(ctx, r.ClusterAgentTokenCreate.ClusterAgentToken.AllowAllQueues, r.ClusterAgentTokenCreate.ClusterAgentToken.AllowedQueueIds)...)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (ct *clusterAgentToken) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state clusterAgentTokenResourceModel

	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := ct.client.readTimeout(ctx)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var r *getClusterAgentTokenResponse
	err := retry.RetryContext(requestCtx, timeout, func() *retry.RetryError {
		var err error

		log.Printf("Getting cluster agent token %s ...", state.Id.ValueString())
		r, err = getClusterAgentToken(requestCtx,
			ct.client.genqlient,
			state.Id.ValueString(),
		)

		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read Cluster Agent Tokens",
			fmt.Sprintf("Unable to read Cluster Agent Tokens: %s", err.Error()),
		)
		return
	}

	node, ok := r.Node.(*getClusterAgentTokenNodeClusterToken)
	if !ok || node == nil || node.Cluster.Organization.Slug != ct.client.organization {
		resp.State.RemoveResource(ctx)
		return
	}
	if state.Uuid.IsNull() && node.AllowedIpAddresses != "" && node.AllowedIpAddresses != "0.0.0.0/0" {
		state.AllowedIpAddresses, diags = types.ListValueFrom(ctx, types.StringType, strings.Fields(node.AllowedIpAddresses))
		resp.Diagnostics.Append(diags...)
	}
	state.Description = types.StringValue(node.Description)
	state.ExpiresAt = expiresAtFromAPI(node.ExpiresAt, state.ExpiresAt)
	state.Uuid = types.StringValue(node.Uuid)
	state.ClusterId = types.StringValue(node.Cluster.Id)
	state.ClusterUuid = types.StringValue(node.Cluster.Uuid)
	state.Permissions, diags = clusterTokenPermissionsFromAPI(ctx, node.Permissions, state.Permissions)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(state.setQueueAccess(ctx, node.AllowAllQueues, node.AllowedQueueIds)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (ct *clusterAgentToken) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state, plan clusterAgentTokenResourceModel

	diagsState := req.State.Get(ctx, &state)
	diagsPlan := req.Plan.Get(ctx, &plan)

	resp.Diagnostics.Append(diagsState...)
	resp.Diagnostics.Append(diagsPlan...)
	var config clusterAgentTokenResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)

	if resp.Diagnostics.HasError() {
		return
	}

	permissions, diags := clusterTokenStringSetToWrite(ctx, plan.Permissions)
	if config.Permissions.IsNull() {
		permissions = nil
	}
	resp.Diagnostics.Append(diags...)
	allowedQueueIds, diags := clusterTokenStringSetToWrite(ctx, plan.AllowedQueueIds)
	if config.AllowedQueueIds.IsNull() {
		allowedQueueIds = nil
	}
	resp.Diagnostics.Append(diags...)
	allowAllQueues := clusterTokenQueueModeToWrite(plan.AllowAllQueues)
	if config.AllowAllQueues.IsNull() {
		allowAllQueues = nil
	}
	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := ct.client.updateTimeout(ctx)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Create CIDR slice from AllowedApiIpAddresses in the plan
	cidrs := createCidrSliceFromList(plan.AllowedIpAddresses)

	var r *updateClusterAgentTokenResponse
	err := retryMutation(ctx, timeout, func(ctx context.Context) *retry.RetryError {
		org, err := ct.client.GetOrganizationID(ctx)
		if err == nil {
			log.Printf("Updating cluster token %s", state.Id.ValueString())
			r, err = updateClusterAgentToken(ctx,
				ct.client.genqlient,
				*org,
				state.Id.ValueString(),
				plan.Description.ValueString(),
				strings.Join(cidrs, " "),
				permissions,
				allowAllQueues,
				allowedQueueIds,
			)
		}

		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to update Cluster Agent Token",
			fmt.Sprintf("Unable to update Cluster Agent Token: %s", err.Error()),
		)
		return
	}

	state.Description = types.StringValue(r.ClusterAgentTokenUpdate.ClusterAgentToken.Description)
	state.AllowedIpAddresses = plan.AllowedIpAddresses
	state.Permissions, diags = clusterTokenPermissionsFromAPI(ctx, r.ClusterAgentTokenUpdate.ClusterAgentToken.Permissions, plan.Permissions)
	resp.Diagnostics.Append(diags...)
	state.AllowAllQueues = plan.AllowAllQueues
	state.AllowedQueueIds = plan.AllowedQueueIds
	resp.Diagnostics.Append(state.setQueueAccess(ctx, r.ClusterAgentTokenUpdate.ClusterAgentToken.AllowAllQueues, r.ClusterAgentTokenUpdate.ClusterAgentToken.AllowedQueueIds)...)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (ct *clusterAgentToken) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func clusterTokenStringSetToWrite(ctx context.Context, configured types.Set) (*[]string, diag.Diagnostics) {
	if configured.IsNull() || configured.IsUnknown() {
		return nil, nil
	}
	values := make([]string, 0, len(configured.Elements()))
	diags := configured.ElementsAs(ctx, &values, false)
	return &values, diags
}

func clusterTokenQueueModeToWrite(mode types.Bool) *bool {
	if mode.IsNull() || mode.IsUnknown() {
		return nil
	}
	return mode.ValueBoolPointer()
}

func clusterTokenPermissionsFromAPI(ctx context.Context, remote []string, current types.Set) (types.Set, diag.Diagnostics) {
	if remote == nil {
		if current.IsUnknown() {
			return types.SetNull(types.StringType), nil
		}
		if !current.IsNull() {
			var diags diag.Diagnostics
			diags.AddWarning("Cluster Agent Token permissions unavailable", "The API did not expose this token's permissions. The last known scope has been retained, but permission changes outside Terraform cannot be detected until scoped agent-token permissions are enabled for the organization.")
			return current, diags
		}
		return current, nil
	}
	return types.SetValueFrom(ctx, types.StringType, remote)
}

func (state *clusterAgentTokenResourceModel) setQueueAccess(ctx context.Context, allowAllQueues *bool, allowedQueueIds []string) diag.Diagnostics {
	var diags diag.Diagnostics
	unavailable := false
	if allowAllQueues != nil {
		state.AllowAllQueues = types.BoolValue(*allowAllQueues)
	} else if state.AllowAllQueues.IsUnknown() {
		state.AllowAllQueues = types.BoolNull()
	} else if !state.AllowAllQueues.IsNull() {
		unavailable = true
	}
	if allowedQueueIds != nil {
		var setDiags diag.Diagnostics
		state.AllowedQueueIds, setDiags = types.SetValueFrom(ctx, types.StringType, allowedQueueIds)
		diags.Append(setDiags...)
	} else if state.AllowedQueueIds.IsUnknown() {
		state.AllowedQueueIds = types.SetNull(types.StringType)
	} else if !state.AllowedQueueIds.IsNull() {
		unavailable = true
	}
	if unavailable {
		diags.AddWarning("Cluster Agent Token queue access unavailable", "The API did not expose this token's queue access settings. The last known settings have been retained, but queue-binding changes outside Terraform cannot be detected until scoped agent-token permissions are enabled for the organization.")
	}
	return diags
}

func (ct *clusterAgentToken) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var plan clusterAgentTokenResourceModel

	diags := req.State.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := ct.client.deleteTimeout(ctx)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	err := retryMutation(ctx, timeout, func(ctx context.Context) *retry.RetryError {
		org, err := ct.client.GetOrganizationID(ctx)
		if err == nil {
			log.Printf("Revoking Cluster Agent Token %s ...", plan.Id.ValueString())
			_, err = revokeClusterAgentToken(ctx,
				ct.client.genqlient,
				*org,
				plan.Id.ValueString(),
			)
		}

		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to revoke Cluster Agent Token",
			fmt.Sprintf("Unable to revoke Cluster Agent Token: %s", err.Error()),
		)
		return
	}
}

// expiresAtFromAPI keeps the configured value when it names the same instant as the API value
func expiresAtFromAPI(remote *time.Time, current types.String) types.String {
	if remote == nil {
		return types.StringNull()
	}
	if configured, err := time.Parse(time.RFC3339, current.ValueString()); err == nil && configured.Equal(*remote) {
		return current
	}
	return types.StringValue(remote.UTC().Format(time.RFC3339Nano))
}

type rfc3339Validator struct{}

func (v rfc3339Validator) Description(ctx context.Context) string {
	return "value must be an RFC 3339 timestamp with at most millisecond precision"
}

func (v rfc3339Validator) MarkdownDescription(ctx context.Context) string {
	return "value must be an RFC 3339 timestamp with at most millisecond precision"
}

func (v rfc3339Validator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	t, err := time.Parse(time.RFC3339, req.ConfigValue.ValueString())
	if err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid timestamp", fmt.Sprintf("Value must be an RFC 3339 timestamp such as 2030-01-01T00:00:00Z: %s", err.Error()))
		return
	}
	// the API returns milliseconds, so anything finer would never read back the same
	if !t.Equal(t.Truncate(time.Millisecond)) {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid timestamp", "Value must have at most millisecond precision")
	}
}
