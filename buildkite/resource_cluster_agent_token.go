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
	Id               types.String `tfsdk:"id"`
	Uuid             types.String `tfsdk:"uuid"`
	Description      types.String `tfsdk:"description"`
	TokenValue       types.String `tfsdk:"tokenValue"`
	JobTokensEnabled types.Bool   `tfsdk:"jobTokensEnabled"`
	ClusterId        types.String `tfsdk:"clusterId"`
}

func NewClusterAgentTokenResource() resource.Resource {
	return &ClusterAgentToken{}
}

func (ct *ClusterAgentToken) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "buildkite_cluster_agent_token"
}

func (ct *ClusterAgentToken) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]resource_schema.Attribute{
			"id": resource_schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"uuid": resource_schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"description": resource_schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "A description about what this cluster agent token is used for",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"tokenValue": resource_schema.StringAttribute{
				Computed:  true,
				Sensitive: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"jobTokensEnabled": resource_schema.BoolAttribute{
				Computed:            true,
				Optional:            true,
				MarkdownDescription: "Agents registered with this token will use a unique token for each job. Please note that this feature is not yet available to all organizations",
			},
			"clusterId": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The ID of the Cluster that this Cluster Queue belongs to.",
			},
		},
	}
}

func (ct *ClusterAgentToken) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, state *ClusterAgentTokenResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	createReq := ClusterAgentTokenCreateInput{
		OrganizationId: ct.client.organizationId,
		ClusterId:      plan.ClusterId.ValueString(),
		Description:    plan.Description.ValueString(),
	}

	r, err := createClusterAgentToken(ct.client.genqlient, createReq)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create Cluster Agent Token",
			fmt.Sprintf("Unable to create Cluster Agent Token: %s", err.Error()),
		)
		return
	}

	state.Id = types.StringValue(r.ClusterAgentTokenCreate.ClusterAgentToken.Id)
	state.Description = types.StringValue(r.ClusterAgentTokenCreate.ClusterAgentToken.Description)
	state.JobTokensEnabled = types.BoolValue(r.ClusterAgentTokenCreate.ClusterAgentToken.JobTokensEnabled)
	state.TokenValue = types.StringValue(r.ClusterAgentTokenCreate.TokenValue)
	state.ClusterId = plan.ClusterId

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)

}

func (ct *ClusterAgentToken) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// to implement
}

func (ct *ClusterAgentToken) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// to implement
}

func (ct *ClusterAgentToken) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// to implement
}

func (ct *ClusterAgentToken) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	ct.client = req.ProviderData.(*Client)
}
