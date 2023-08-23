package buildkite

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type ClusterAgentToken struct {
	client *Client
}

type ClusterAgentTokenResourceModel struct {
	Id          types.String `tfsdk:"id"`
	Uuid        types.String `tfsdk:"uuid"`
	Description types.String `tfsdk:"description"`
	Token       types.String `tfsdk:"token"`
	ClusterId   types.String `tfsdk:"cluster_id"`
	ClusterUuid types.String `tfsdk:"cluster_uuid"`
}

func newClusterAgentTokenResource() resource.Resource {
	return &ClusterAgentToken{}
}

func (ct *ClusterAgentToken) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_agent_token"
}

func (ct *ClusterAgentToken) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	ct.client = req.ProviderData.(*Client)
}

func (ct *ClusterAgentToken) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Cluster Agent Token is a token used to connect an agent to a cluster in Buildkite.",
		Attributes: map[string]resource_schema.Attribute{
			"id": resource_schema.StringAttribute{
				Computed: true,
			},
			"uuid": resource_schema.StringAttribute{
				Computed: true,
			},
			"description": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "A description about what this cluster agent token is used for",
			},
			"token": resource_schema.StringAttribute{
				Computed:  true,
				Sensitive: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"cluster_id": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The ID of the Cluster that this Cluster Agent Token belongs to.",
			},
			"cluster_uuid": resource_schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (ct *ClusterAgentToken) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, state ClusterAgentTokenResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("Creating cluster agent token with description %s into cluster %s ...", plan.Description.ValueString(), plan.ClusterId.ValueString())
	r, err := createClusterAgentToken(ctx,
		ct.client.genqlient,
		ct.client.organizationId,
		plan.ClusterId.ValueString(),
		plan.Description.ValueString(),
	)

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

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (ct *ClusterAgentToken) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ClusterAgentTokenResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}
	log.Printf("Getting cluster agent tokens for cluster %s ...", state.ClusterUuid.ValueString())
	tokens, err := getClusterAgentTokens(ctx, ct.client.genqlient, ct.client.organization, state.ClusterUuid.ValueString())

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read Cluster Agent Tokens",
			fmt.Sprintf("Unable to read Cluster Agent Tokens: %s", err.Error()),
		)
		return
	}

	for _, edge := range tokens.Organization.Cluster.AgentTokens.Edges {
		if edge.Node.Id == state.Id.ValueString() {
			log.Printf("Found cluster Token with Description %s in cluster %s", edge.Node.Id, state.ClusterUuid.ValueString())
			state.Description = types.StringValue(edge.Node.Description)
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}

}

func (ct *ClusterAgentToken) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state, plan ClusterAgentTokenResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}
	log.Printf("Updating cluster token %s", state.Id.ValueString())
	response, err := updateClusterAgentToken(ctx,
		ct.client.genqlient,
		ct.client.organizationId,
		state.Id.ValueString(),
		plan.Description.ValueString(),
	)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to update Cluster Agent Token",
			fmt.Sprintf("Unable to update Cluster Agent Token: %s", err.Error()),
		)
		return
	}
	state.Description = types.StringValue(response.ClusterAgentTokenUpdate.ClusterAgentToken.Description)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)

}

func (ct *ClusterAgentToken) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var plan ClusterAgentTokenResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("Revoking Cluster Agent Token %s ...", plan.Id.ValueString())
	_, err := revokeClusterAgentToken(ctx, ct.client.genqlient, ct.client.organizationId, plan.Id.ValueString())

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to revoke Cluster Agent Token",
			fmt.Sprintf("Unable to revoke Cluster Agent Token: %s", err.Error()),
		)
		return
	}
}
