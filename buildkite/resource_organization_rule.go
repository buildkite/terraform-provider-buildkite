package buildkite

import (
	"context"
	"fmt"
	//"log"
	
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

type organizationRuleResourceModel struct {
	Id         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	Value      types.String `tfsdk:"value"`
	SourceType types.String `tfsdk:"source_type"`
	SourceUuid types.String `tfsdk:"source_uuid"`
	TargetType types.String `tfsdk:"target_type"`
	TargetUuid types.String `tfsdk:"target_uuid"`
	Effect     types.String `tfsdk:"effect"`
	Action     types.String `tfsdk:"action"`
}

type organizationRuleResource struct {
	client *Client
}

func newOrganizationRuleResource() resource.Resource {
	return &organizationRuleResource{}
}

func (organizationRuleResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_rule"
}

func (or *organizationRuleResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	or.client = req.ProviderData.(*Client)
}

func (organizationRuleResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resource_schema.Schema{
		MarkdownDescription: "An Organization Rule allows specifying explicit rules between two Buildkite resources and desired effect and actions. ",
		Attributes: map[string]resource_schema.Attribute{
			"id": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The GraphQL ID of the organization rule.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name that is given to this organization rule.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"value": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The JSON rule that this organization rule implements.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"source_type": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The source resource type that this organization rule allows or denies to invoke its defined action. ",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"source_uuid": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the resource that this organization rule allows or denies invocating its defined action. ",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"target_type": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The target resource type that this organization rule allows or denies the source to respective action. ",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"target_uuid": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the target resourcee that this organization rule allows or denies invocation its respective action. ",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"effect": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Whether this organization rule allows or denys the action to take place between source and target resources. ",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"action": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The action defined between source and target resources. ",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (or *organizationRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, state organizationRuleResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := or.client.timeouts.Create(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var r *createOrganizationRuleResponse
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		org, err := or.client.GetOrganizationID()
		if err == nil { 
			r, err = createOrganizationRule(
				ctx,
				or.client.genqlient,
				*org,
				plan.Name.ValueString(),
				plan.Value.ValueString(),
			)
		}

		return retryContextError(err)
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create Organization rule",
			fmt.Sprintf("Unable to create organization rule: %s", err.Error()),
		)
		return
	}

	state.Id = types.StringValue(r.RuleCreate.Rule.Id)
	state.Name = types.StringValue(r.RuleCreate.Rule.Name)
	state.Value = types.StringValue(plan.Value.ValueString())
	state.SourceType = types.StringValue(string(r.RuleCreate.Rule.SourceType))
	state.TargetType = types.StringValue(string(r.RuleCreate.Rule.TargetType))
	state.Effect = types.StringValue(string(r.RuleCreate.Rule.Effect))
	state.Action = types.StringValue(string(r.RuleCreate.Rule.Action))

	// Determine source UUID based on type
	if ruleCreateSourcePipeline, ok := r.RuleCreate.Rule.Source.(*OrganizationRuleFieldsSourcePipeline); ok {
		state.SourceUuid = types.StringValue(ruleCreateSourcePipeline.Uuid)
	}

	// Determine source UUID based on type
	if ruleCreateTargetPipeline, ok := r.RuleCreate.Rule.Target.(*OrganizationRuleFieldsTargetPipeline); ok {
		state.TargetUuid = types.StringValue(ruleCreateTargetPipeline.Uuid)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (or *organizationRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	/*
	var state organizationRuleResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

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

		log.Printf("Reading organization rule with ID %s ...", state.Id.ValueString())
		apiResponse, err = getNode(ctx,
			or.client.genqlient,
			state.Id.ValueString(),
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

	// Convert fron Node to getNodeTeamMember type
	if organizationRule, ok := apiResponse.GetNode().(*getNodeNodeOrganization); ok {
		if organizationRule == nil {
			resp.Diagnostics.AddError(
				"Unable to get organization rule",
				"Error getting organization rule: nil response",
			)
			return
		}
		// Update state here with organization rule from API
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	} else {
		// Remove from state if not found{{}}
		resp.Diagnostics.AddWarning("Organization rule not found", "Removing from state")
		resp.State.RemoveResource(ctx)
		return
	}
	*/
}

func (or *organizationRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (or *organizationRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Cannot update an organization rule", "An existing rule must be deleted/re-created")
	panic("cannot update an organization rule")
}

func (or *organizationRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state organizationRuleResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := or.client.timeouts.Delete(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		org, err := or.client.GetOrganizationID()
		if err == nil {
			_, err = deleteOrganizationRule(
				ctx,
				or.client.genqlient,
				*org,
				state.Id.ValueString(),
			)
		}

		return retryContextError(err)
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete organization rule",
			fmt.Sprintf("Unable to delete organization rule: %s", err.Error()),
		)
		return
	}
}
