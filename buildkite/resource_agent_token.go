package buildkite

import (
	"context"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
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

type agentTokenStateModel struct {
	Description types.String `tfsdk:"description"`
	Id          types.String `tfsdk:"id"`
	Token       types.String `tfsdk:"token"`
	Uuid        types.String `tfsdk:"uuid"`
}

type agentTokenResource struct {
	client *Client
}

func newAgentTokenResource() resource.Resource {
	return &agentTokenResource{}
}

func (at *agentTokenResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	at.client = req.ProviderData.(*Client)
}

func (at *agentTokenResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, state agentTokenStateModel

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := at.client.timeouts.Create(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var r *createAgentTokenResponse
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		org, err := at.client.GetOrganizationID()
		if err == nil {
			r, err = createAgentToken(ctx,
				at.client.genqlient,
				*org,
				plan.Description.ValueStringPointer(),
			)
		}

		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create agent token",
			fmt.Sprintf("Unable to create agent token: %s", err.Error()),
		)
		return
	}

	state.Description = types.StringPointerValue(r.AgentTokenCreate.AgentTokenEdge.Node.Description)
	state.Id = types.StringValue(r.AgentTokenCreate.AgentTokenEdge.Node.Id)
	state.Token = types.StringValue(r.AgentTokenCreate.TokenValue)
	state.Uuid = types.StringValue(r.AgentTokenCreate.AgentTokenEdge.Node.Uuid)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (at *agentTokenResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state agentTokenStateModel

	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := at.client.timeouts.Delete(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		_, err := revokeAgentToken(ctx,
			at.client.genqlient,
			state.Id.ValueString(),
			"Revoked by Terraform",
		)

		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to revoke agent token",
			fmt.Sprintf("Unable to revoke agent token: %s", err.Error()),
		)
		return
	}
}

func (agentTokenResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "buildkite_agent_token"
}

func (at *agentTokenResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var plan, state agentTokenStateModel

	diags := req.State.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := at.client.timeouts.Read(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var r *getAgentTokenResponse
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var err error
		r, err = getAgentToken(ctx,
			at.client.genqlient,
			fmt.Sprintf("%s/%s", at.client.organization, plan.Uuid.ValueString()),
		)

		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read agent token",
			fmt.Sprintf("Unable to read agent token: %s", err.Error()),
		)
	}

	if r == nil {
		resp.Diagnostics.AddError("Agent token not found", "Removing from state")
		resp.State.RemoveResource(ctx)
		return
	}

	state.Description = types.StringPointerValue(r.AgentToken.Description)
	state.Id = types.StringValue(r.AgentToken.Id)
	state.Token = plan.Token // token is never returned after creation so use the existing value in state
	state.Uuid = types.StringValue(r.AgentToken.Uuid)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (agentTokenResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resource_schema.Schema{
		MarkdownDescription: heredoc.Doc(`
			This resource allows you to create and manage non-clustered agent tokens.
			You can find out more about clusters in the Buildkite [documentation](https://buildkite.com/docs/agent/v3/tokens).
		`),
		Attributes: map[string]resource_schema.Attribute{
			"description": resource_schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The description of the agent token. Used to help identify its use.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"id": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The GraphQL ID of the agent token.",
			},
			"token": resource_schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "The token value used by an agent to register with the API.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"uuid": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the agent token.",
			},
		},
	}
}

func (agentTokenResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Cannot update an agent token", "A new agent token must be created")
	panic("cannot update an agent token")
}
