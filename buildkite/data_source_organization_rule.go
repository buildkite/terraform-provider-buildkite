package buildkite

import (
	"context"
	"fmt"
	"log"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

type organizationRuleDatasourceModel struct {
	ID          types.String `tfsdk:"id"`
	UUID        types.String `tfsdk:"uuid"`
	Description types.String `tfsdk:"description"`
	Type        types.String `tfsdk:"type"`
	Value       types.String `tfsdk:"value"`
	SourceType  types.String `tfsdk:"source_type"`
	SourceUUID  types.String `tfsdk:"source_uuid"`
	TargetType  types.String `tfsdk:"target_type"`
	TargetUUID  types.String `tfsdk:"target_uuid"`
	Effect      types.String `tfsdk:"effect"`
	Action      types.String `tfsdk:"action"`
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
		
		More information on organization rules can be found in the [documentation](https://buildkite.com/docs/pipelines/rules/overview).
		`),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The GraphQL ID of the organization rule. ",
			},
			"uuid": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The UUID of the organization rule. ",
			},
			"description": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The description of the organization rule. ",
			},
			"type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The type of organization rule. ",
			},
			"value": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The JSON document that this organization rule implements. ",
			},
			"source_type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The source resource type that this organization rule allows or denies to invoke its defined action. ",
			},
			"source_uuid": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the resource that this organization rule allows or denies invocating its defined action. ",
			},
			"target_type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The target resource type that this organization rule allows or denies the source to respective action. ",
			},
			"target_uuid": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the target resourcee that this organization rule allows or denies invocation its respective action. ",
			},
			"effect": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Whether this organization rule allows or denys the action to take place between source and target resources. ",
			},
			"action": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The action defined between source and target resources. ",
			},
		},
	}
}

func (or *organizationRuleDatasource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state organizationRuleDatasourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeouts, diags := or.client.timeouts.Read(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	// If a UUID is entered through an organization rule data source
	if !state.UUID.IsNull() {
		matchFound := false
		err := retry.RetryContext(ctx, timeouts, func() *retry.RetryError {
			var cursor *string
			for {
				r, err := getOrganizationRules(
					ctx,
					or.client.genqlient,
					or.client.organization,
					cursor)
				if err != nil {
					if isRetryableError(err) {
						return retry.RetryableError(err)
					}
					resp.Diagnostics.AddError(
						"Unable to read organizatiion rules",
						fmt.Sprintf("Unable to read organizatiion rules: %s", err.Error()),
					)
					return retry.NonRetryableError(err)
				}

				for _, rule := range r.Organization.Rules.Edges {
					if rule.Node.Uuid == state.UUID.ValueString() {
						matchFound = true
						// Update data source state from the found rule
						value, err := obtainValueJSON(rule.Node.Document)
						if err != nil {
							resp.Diagnostics.AddError(
								"Unable to read organization rule",
								fmt.Sprintf("Unable to read organmization rule: %s", err.Error()),
							)
						}
						updateOrganizatonRuleDatasourceFromNode(&state, rule.Node, *value)
						break
					}
				}

				// If there is a match, or there is no next page, break
				if matchFound || !r.Organization.Rules.PageInfo.HasNextPage {
					break
				}

				// Move to the next cursor
				cursor = &r.Organization.Rules.PageInfo.EndCursor
			}
			return nil
		})

		if err != nil {
			resp.Diagnostics.AddError("Unable to find organization rule", err.Error())
			return
		}

		if !matchFound {
			resp.Diagnostics.AddError("Unable to find organization rule",
				fmt.Sprintf("Could not find an organization rule with UUID \"%s\"", state.UUID.ValueString()))
			return
		}
		// Otherwise if a ID is specified
	} else if !state.ID.IsNull() {
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
			// Update data source state from the found rule
			value, err := obtainValueJSON(organizationRule.Document)
			if err != nil {
				resp.Diagnostics.AddError(
					"Unable to read organization rule",
					fmt.Sprintf("Unable to read organmization rule: %s", err.Error()),
				)
			}
			updateOrganizatonRuleDatasourceState(&state, *organizationRule, *value)
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func obtainDatasourceReadUUIDs(orn getOrganizationRulesOrganizationRulesRuleConnectionEdgesRuleEdgeNodeRule) (string, string) {
	var sourceUUID, targetUUID string

	switch orn.SourceType {
	case "PIPELINE":
		sourceUUID = orn.Source.(*OrganizationRuleFieldsSourcePipeline).Uuid
	}

	switch orn.TargetType {
	case "PIPELINE":
		targetUUID = orn.Target.(*OrganizationRuleFieldsTargetPipeline).Uuid
	}

	return sourceUUID, targetUUID
}

func updateOrganizatonRuleDatasourceState(or *organizationRuleDatasourceModel, orn getNodeNodeRule, value string) {
	sourceUUID, targetUUID := obtainReadUUIDs(orn)

	or.ID = types.StringValue(orn.Id)
	or.UUID = types.StringValue(orn.Uuid)
	or.Description = types.StringPointerValue(orn.Description)
	or.Type = types.StringValue(orn.Type)
	or.Value = types.StringValue(value)
	or.SourceType = types.StringValue(string(orn.SourceType))
	or.SourceUUID = types.StringValue(sourceUUID)
	or.TargetType = types.StringValue(string(orn.TargetType))
	or.TargetUUID = types.StringValue(targetUUID)
	or.Effect = types.StringValue(string(orn.Effect))
	or.Action = types.StringValue(string(orn.Action))
}

func updateOrganizatonRuleDatasourceFromNode(or *organizationRuleDatasourceModel, orn getOrganizationRulesOrganizationRulesRuleConnectionEdgesRuleEdgeNodeRule, value string) {
	sourceUUID, targetUUID := obtainDatasourceReadUUIDs(orn)

	or.ID = types.StringValue(orn.Id)
	or.UUID = types.StringValue(orn.Uuid)
	or.Description = types.StringPointerValue(orn.Description)
	or.Type = types.StringValue(orn.Type)
	or.Value = types.StringValue(value)
	or.SourceType = types.StringValue(string(orn.SourceType))
	or.SourceUUID = types.StringValue(sourceUUID)
	or.TargetType = types.StringValue(string(orn.TargetType))
	or.TargetUUID = types.StringValue(targetUUID)
	or.Effect = types.StringValue(string(orn.Effect))
	or.Action = types.StringValue(string(orn.Action))
}
