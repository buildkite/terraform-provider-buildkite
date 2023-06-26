package buildkite

import (
	"context"
	"fmt"

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
}

func NewClusterAgentTokenResource() resource.Resource {
	return &ClusterAgentToken{}
}

func (ct *ClusterAgentToken) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_agent_token"
}

func (ct *ClusterAgentToken) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]resource_schema.Attribute{
			"id": resource_schema.StringAttribute{
				Computed: true,
			},
			"uuid": resource_schema.StringAttribute{
				Computed: true,
			},
			"description": resource_schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "A description about what this cluster agent token is used for",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
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
				MarkdownDescription: "The ID of the Cluster that this Cluster Queue belongs to.",
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

	r, err := createClusterAgentToken(ct.client.genqlient,
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
	state.ClusterId = plan.ClusterId

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (ct *ClusterAgentToken) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// to implement
}

func (ct *ClusterAgentToken) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state, plan ClusterAgentTokenResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}
	_, err := updateClusterAgentToken(
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

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)

}

func (ct *ClusterAgentToken) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var plan ClusterAgentTokenResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	_, err := revokeClusterAgentToken(ct.client.genqlient, ct.client.organizationId, plan.Id.ValueString())

	if err != nil {
		resp.Diagnostics.AddError(err.Error(), err.Error())
		return
	}
}

func (ct *ClusterAgentToken) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	ct.client = req.ProviderData.(*Client)
}
