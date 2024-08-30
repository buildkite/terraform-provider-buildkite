package buildkite

import (
	"context"
	"fmt"
	"log"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

type organizationRuleDatasourceModel struct {
	ID         types.String `tfsdk:"id"`
	UUID       types.String `tfsdk:"uuid"`
	Type       types.String `tfsdk:"type"`
	Value      types.String `tfsdk:"value"`
	SourceType types.String `tfsdk:"source_type"`
	SourceUUID types.String `tfsdk:"source_uuid"`
	TargetType types.String `tfsdk:"target_type"`
	TargetUUID types.String `tfsdk:"target_uuid"`
	Effect     types.String `tfsdk:"effect"`
	Action     types.String `tfsdk:"action"`
}

type organizationRuleDatasource struct {
	client *Client
}

func newOrganizationRuleDatasource() datasource.DataSource {
	return &organizationRuleDatasource{}
}

func (organizationRuleDatasource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_rule"
}

func (or *organizationRuleDatasource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	or.client = req.ProviderData.(*Client)
}

func (organizationRuleDatasource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: heredoc.Doc(`
		Use this data source to retrieve an organization rule by its ID.
		
		More information on pipeline templates can be found in the [documentation](https://buildkite.com/docs/pipelines/rules/overview).
		`),
		Attributes: map[string]schema.Attribute{
			"id": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The GraphQL ID of the organization rule. ",
			},
			"uuid": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the organization rule. ",
			},
			"type": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The type of organization rule. ",
			},
			"value": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The JSON document that this organization rule implements. ",
			},
			"source_type": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The source resource type that this organization rule allows or denies to invoke its defined action. ",
			},
			"source_uuid": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the resource that this organization rule allows or denies invocating its defined action. ",
			},
			"target_type": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The target resource type that this organization rule allows or denies the source to respective action. ",
			},
			"target_uuid": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the target resourcee that this organization rule allows or denies invocation its respective action. ",
			},
			"effect": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Whether this organization rule allows or denys the action to take place between source and target resources. ",
			},
			"action": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The action defined between source and target resources. ",
			},
		},
	}
}

func (or *organizationRuleDatasource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state organizationRuleResourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeouts, diags := or.client.timeouts.Read(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var apiResponse *getNodeResponse
	err := retry.RetryContext(ctx, timeouts, func() *retry.RetryError {
		var err error

		log.Printf("Reading organization rule with ID %s ...", state.UUID.ValueString())
		apiResponse, err = getNode(ctx,
			or.client.genqlient,
			state.ID.ValueString(),
		)

		return retryContextError(err)
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read organization rule",
			fmt.Sprintf("Unable to read organmization rule: %s", err.Error()),
		)
		return
	}

	// Convert fron Node to getNodeNodeRule type
	if organizationRule, ok := apiResponse.GetNode().(*getNodeNodeRule); ok {
		if organizationRule == nil {
			resp.Diagnostics.AddError(
				"Unable to get organization rule",
				"Error getting organization rule: nil response",
			)
			return
		}

		updateOrganizatonRuleDatasourceState(&state, *organizationRule)
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	}
}

func updateOrganizatonRuleDatasourceState(or *organizationRuleResourceModel, orn getNodeNodeRule) {
	sourceUUID, targetUUID := obtainReadUUIDs(orn)
	value := obtainValueJSON(sourceUUID, targetUUID, string(orn.Action))

	or.ID = types.StringValue(orn.Id)
	or.UUID = types.StringValue(orn.Uuid)
	or.Type = types.StringValue(orn.Type)
	or.Value = types.StringValue(value)
	or.SourceType = types.StringValue(string(orn.SourceType))
	or.SourceUUID = types.StringValue(sourceUUID)
	or.TargetType = types.StringValue(string(orn.TargetType))
	or.TargetUUID = types.StringValue(targetUUID)
	or.Effect = types.StringValue(string(orn.Effect))
	or.Action = types.StringValue(string(orn.Action))
}