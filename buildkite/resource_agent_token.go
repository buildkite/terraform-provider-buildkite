package buildkite

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/shurcooL/graphql"
)

// AgentTokenNode represents a pipeline as returned from the GraphQL API
type AgentTokenNode struct {
	Description graphql.String
	ID          graphql.String
	Token       graphql.String
	UUID        graphql.String
	RevokedAt   graphql.String
}

type AgentTokenStateModel struct {
	Description types.String `tfsdk:"description"`
	Id          types.String `tfsdk:"id"`
	Token       types.String `tfsdk:"token"`
	Uuid        types.String `tfsdk:"uuid"`
}

type AgentTokenResource struct {
	client *Client
}

func newAgentTokenResource() resource.Resource {
	return &AgentTokenResource{}
}

func (at *AgentTokenResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	at.client = req.ProviderData.(*Client)
}

func (at *AgentTokenResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, state AgentTokenStateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	apiResponse, err := createAgentToken(
		at.client.genqlient,
		at.client.organizationId,
		plan.Description.ValueStringPointer(),
	)

	if err != nil {
		resp.Diagnostics.AddError(err.Error(), err.Error())
	}

	state.Description = types.StringPointerValue(apiResponse.AgentTokenCreate.AgentTokenEdge.Node.Description)
	state.Id = types.StringValue(apiResponse.AgentTokenCreate.AgentTokenEdge.Node.Id)
	state.Token = types.StringValue(apiResponse.AgentTokenCreate.TokenValue)
	state.Uuid = types.StringValue(apiResponse.AgentTokenCreate.AgentTokenEdge.Node.Uuid)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (at *AgentTokenResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var plan AgentTokenStateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &plan)...)

	_, err := revokeAgentToken(at.client.genqlient, plan.Id.ValueString(), "Revoked by Terraform")

	if err != nil {
		resp.Diagnostics.AddError(err.Error(), err.Error())
	}
}

func (AgentTokenResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "buildkite_agent_token"
}

func (at *AgentTokenResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var plan, state AgentTokenStateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &plan)...)

	agentToken, err := getAgentToken(at.client.genqlient, fmt.Sprintf("%s/%s", at.client.organization, plan.Uuid.ValueString()))

	if err != nil {
		resp.Diagnostics.AddError(err.Error(), err.Error())
	}
	if agentToken == nil {
		resp.Diagnostics.AddError("Agent token not found", "Removing from state")
		resp.State.RemoveResource(ctx)
		return
	}

	state.Description = types.StringPointerValue(agentToken.AgentToken.Description)
	state.Id = types.StringValue(agentToken.AgentToken.Id)
	state.Token = plan.Token // token is never returned after creation so use the existing value in state
	state.Uuid = types.StringValue(agentToken.AgentToken.Uuid)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (AgentTokenResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resource_schema.Schema{
		Attributes: map[string]resource_schema.Attribute{
			"description": resource_schema.StringAttribute{
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"id": resource_schema.StringAttribute{
				Computed: true,
			},
			"token": resource_schema.StringAttribute{
				Computed:  true,
				Sensitive: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"uuid": resource_schema.StringAttribute{
				Computed: true,
			},
		},
	}
}

func (AgentTokenResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Cannot update an agent token", "A new agent token must be created")
	panic("cannot update an agent token")
}
