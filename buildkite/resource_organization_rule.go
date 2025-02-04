package buildkite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

type organizationRuleResourceModel struct {
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

type ruleDocument struct {
	Rule  string          `json:"rule"`
	Value json.RawMessage `json:"value"`
}

type ruleValue struct {
	Source     string   `json:"source_pipeline"`
	Target     string   `json:"target_pipeline"`
	Conditions []string `json:"conditions"`
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
		MarkdownDescription: heredoc.Doc(`
		An Organization Rule allows specifying explicit rules between two Buildkite resources and the desired effect/action.

		More information on organization rules can be found in the [documentation](https://buildkite.com/docs/pipelines/rules).
	`),
		Attributes: map[string]resource_schema.Attribute{
			"id": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The GraphQL ID of the organization rule.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"uuid": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the organization rule.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"description": resource_schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The description of the organization rule. ",
			},
			"type": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The type of organization rule. ",
			},
			"value": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The JSON document that this organization rule implements. ",
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
				MarkdownDescription: "The UUID of the target resource that this organization rule allows or denies invocation its respective action. ",
			},
			"effect": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Whether this organization rule allows or denies the action to take place between source and target resources. ",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"action": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The action defined between source and target resources. ",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (or *organizationRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, state organizationRuleResourceModel
	var plannedValue ruleValue

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

	// Unmarshall the plan's value into a ruleValue struct instance
	err := json.Unmarshal([]byte(plan.Value.ValueString()), &plannedValue)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create organization rule",
			fmt.Sprintf("Unable to create organization rule: %s ", err.Error()),
		)
		return
	}

	// Confirm that both the source|target pipelines specified in the value attribute of a buildkite_organization_rule are valid UUID strings.
	// If either is not a valid UUID and not empty, the provider will return an error stating this and abort creation of the rule.
	if !isUUID(plannedValue.Source) && len(plannedValue.Source) > 0 {
		resp.Diagnostics.AddError(
			"Unable to create organization rule",
			fmt.Sprintf("%s: source_pipeline is an invalid UUID.", plan.Type.ValueString()),
		)
		return
	} else if !isUUID(plannedValue.Target) && len(plannedValue.Target) > 0 {
		resp.Diagnostics.AddError(
			"Unable to create organization rule",
			fmt.Sprintf("%s: target_pipeline is an invalid UUID.", plan.Type.ValueString()),
		)
		return
	}

	var r *createOrganizationRuleResponse
	err = retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		org, err := or.client.GetOrganizationID()
		if err == nil {
			log.Printf("Creating organization rule ...")
			r, err = createOrganizationRule(
				ctx,
				or.client.genqlient,
				*org,
				plan.Description.ValueStringPointer(),
				plan.Type.ValueString(),
				plan.Value.ValueString(),
			)
		}

		return retryContextError(err)
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create organization rule",
			fmt.Sprintf("Unable to create organization rule: %s ", err.Error()),
		)
		return
	}

	// Obtain the source and target UUIDs of the created organization rule based on the API response.
	sourceUUID, targetUUID, err := obtainCreationUUIDs(r)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create organization rule",
			fmt.Sprintf("Unable to obtain source/target UUIDs: %s ", err.Error()),
		)
		return
	}

	// Obtain the sorted value JSON from the API response (document field in RuleCreatePayload's rule)
	value, err := obtainValueJSON(r.RuleCreate.Rule.Document)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create organization rule",
			fmt.Sprintf("Unable to create organmization rule: %s", err.Error()),
		)
		return
	}

	// Update organization rule model and set in state
	updateOrganizatonRuleCreateState(&state, *r, *sourceUUID, *targetUUID, *value)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (or *organizationRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
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

		log.Printf("Reading organization rule with ID %s ...", state.ID.ValueString())
		apiResponse, err = getNode(ctx, or.client.genqlient, state.ID.ValueString())

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

		// Get the returned rule's sorted value JSON object from its Document field
		value, err := obtainValueJSON(organizationRule.Document)

		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to read organization rule",
				fmt.Sprintf("Unable to read organmization rule: %s", err.Error()),
			)
			return
		}

		// Update organization rule model and set in state
		updateOrganizatonRuleReadState(&state, *organizationRule, *value)
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	} else {
		// Remove from state if not found{{}}
		resp.Diagnostics.AddWarning("Organization rule not found", "Removing from state")
		resp.State.RemoveResource(ctx)
		return
	}
}

func (or *organizationRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (or *organizationRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state organizationRuleResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := or.client.timeouts.Update(ctx, DefaultTimeout)

	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var r *updateOrganizationRuleResponse
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		org, err := or.client.GetOrganizationID()
		if err == nil {
			log.Printf("Updating organization rule with ID %s ...", state.ID.ValueString())
			r, err = updateOrganizationRule(ctx,
				or.client.genqlient,
				*org,
				state.ID.ValueString(),
				plan.Description.ValueStringPointer(),
				*plan.Value.ValueStringPointer(),
			)
		}

		return retryContextError(err)
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to update organization rule",
			fmt.Sprintf("Unable to update organization rule: %s", err.Error()),
		)
		return
	}

	// Obtain the source and target UUIDs of the created organization rule based on the API response.
	sourceUUID, targetUUID, err := obtainUpdateUUIDs(r)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to update organization rule",
			fmt.Sprintf("Unable to obtain source/target UUIDs: %s ", err.Error()),
		)
		return
	}

	// Obtain the sorted value JSON from the API response (document field in RuleCreatePayload's rule)
	value, err := obtainValueJSON(r.RuleUpdate.Rule.Document)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to update organization rule",
			fmt.Sprintf("Unable to update organmization rule: %s", err.Error()),
		)
		return
	}

	updateOrganizationRuleUpdateState(&state, *r, *sourceUUID, *targetUUID, *value)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
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
			log.Printf("Deleting organization rule with ID %s ...", state.ID.ValueString())
			_, err = deleteOrganizationRule(
				ctx,
				or.client.genqlient,
				*org,
				state.ID.ValueString(),
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

func obtainCreationUUIDs(r *createOrganizationRuleResponse) (*string, *string, error) {
	var sourceUUID, targetUUID string

	// The provider will try and determine the source UUID based on type that is returned in the *createOrganizationRuleResponse
	// It will switch based on the SourceType returned in the response and extract the UUID of the respective source based on this.
	// Otherwise, the provider will create and throw an error stating that it cannot obtain the source type from the returned API response.
	// In all cases exhausted, the provider will throw an error stating that the rule's source type can't be determined after creation.

	switch r.RuleCreate.Rule.SourceType {
	case "PIPELINE":
		if ruleCreateSourcePipeline, ok := r.RuleCreate.Rule.Source.(*OrganizationRuleFieldsSourcePipeline); ok {
			sourceUUID = ruleCreateSourcePipeline.Uuid
		} else {
			return nil, nil, errors.New("Error obtaining source type upon creating the organization rule.")
		}
	default:
		// We can't determine the source type from the RuleCreate object - return an error
		return nil, nil, errors.New("Error determining source type upon creating the organization rule.")
	}

	// Now, like above - the provider will try and determine the target UUID based on the *createOrganizationRuleResponse. It will
	// switch based on the TargetType returned in the response and extract the UUID of the respective target based on this.
	// Otherwise, the provider will create and throw an error stating that it cannot obtain the target type from the returned API response.
	// In all cases exhausted, the provider will throw an error stating that the rule's target type can't be determined after creation.

	switch r.RuleCreate.Rule.TargetType {
	case "PIPELINE":
		if ruleCreateTargetPipeline, ok := r.RuleCreate.Rule.Target.(*OrganizationRuleFieldsTargetPipeline); ok {
			targetUUID = ruleCreateTargetPipeline.Uuid
		} else {
			return nil, nil, errors.New("Error obtaining target type upon creating the organization rule.")
		}
	default:
		// We can't determine the target type from the RuleCreate object - return an error
		return nil, nil, errors.New("Error determining target type upon creating the organization rule.")
	}

	return &sourceUUID, &targetUUID, nil
}

func obtainUpdateUUIDs(r *updateOrganizationRuleResponse) (*string, *string, error) {
	var sourceUUID, targetUUID string

	// The provider will try and determine the source UUID based on type that is returned in the *updateOrganizationRuleResponse, notably
	// if it has been changed during a plan->apply sequence. This logic will switch based on the SourceType returned in the update
	// response and extract the UUID of the respective source based on this (i.e "PIPELINE").
	// Otherwise, the provider will create and throw an error stating that it cannot obtain the source type from the returned API response.
	// In all cases exhausted, the provider will throw an error stating that the rule's source type can't be determined after an update.

	switch r.RuleUpdate.Rule.SourceType {
	case "PIPELINE":
		if ruleCreateSourcePipeline, ok := r.RuleUpdate.Rule.Source.(*OrganizationRuleFieldsSourcePipeline); ok {
			sourceUUID = ruleCreateSourcePipeline.Uuid
		} else {
			return nil, nil, errors.New("Error obtaining source type upon updating the organization rule.")
		}
	default:
		// We can't determine the source type from the RuleUpdate object - return an error
		return nil, nil, errors.New("Error determining source type upon updating the organization rule.")
	}

	// Now, like above - the provider will try and determine the target UUID based on the *updateOrganizationRuleResponse. notably
	// if it has been changed during a plan->apply sequence. This logic will switch based on the TargetType returned in the update
	// response and extract the UUID of the respective target based on this (i.e "PIPELINE").
	// Otherwise, the provider will create and throw an error stating that it cannot obtain the target type from the returned API response.
	// In all cases exhausted, the provider will throw an error stating that the rule's target type can't be determined after an update.

	switch r.RuleUpdate.Rule.TargetType {
	case "PIPELINE":
		if ruleCreateTargetPipeline, ok := r.RuleUpdate.Rule.Target.(*OrganizationRuleFieldsTargetPipeline); ok {
			targetUUID = ruleCreateTargetPipeline.Uuid
		} else {
			return nil, nil, errors.New("Error obtaining target type upon updating the organization rule.")
		}
	default:
		// We can't determine the target type from the RuleUpdate object - return an error
		return nil, nil, errors.New("Error determining target type upon updating the organization rule.")
	}

	return &sourceUUID, &targetUUID, nil
}

func obtainReadUUIDs(nr getNodeNodeRule) (string, string) {
	var sourceUUID, targetUUID string

	switch nr.SourceType {
	case "PIPELINE":
		sourceUUID = nr.Source.(*OrganizationRuleFieldsSourcePipeline).Uuid
	}

	switch nr.TargetType {
	case "PIPELINE":
		targetUUID = nr.Target.(*OrganizationRuleFieldsTargetPipeline).Uuid
	}

	return sourceUUID, targetUUID
}

func obtainValueJSON(document string) (*string, error) {
	var rd ruleDocument
	valueMap := make(map[string]interface{})

	// Unmarshall the API obtained document into a ruleDocument struct instance
	err := json.Unmarshal([]byte(document), &rd)
	if err != nil {
		return nil, errors.New("Error unmarshalling the organization rule's JSON document.")
	}

	// Unmarshall the ruleDocument's value into a [string]interface{} map
	err = json.Unmarshal([]byte(string(rd.Value)), &valueMap)
	if err != nil {
		return nil, errors.New("Error unmarshalling the organization rule's value.")
	}

	// Marshall the value map back into a byte slice (sorted)
	valueMarshalled, err := json.Marshal(valueMap)
	if err != nil {
		return nil, errors.New("Error marshalling the organization rule's sorted value JSON.")
	}

	// Convert and return sorted and serialized value string
	value := string(valueMarshalled)
	return &value, nil
}

func updateOrganizatonRuleCreateState(or *organizationRuleResourceModel, ruleCreate createOrganizationRuleResponse, sourceUUID, targetUUID, value string) {
	or.ID = types.StringValue(ruleCreate.RuleCreate.Rule.Id)
	or.UUID = types.StringValue(ruleCreate.RuleCreate.Rule.Uuid)
	or.Description = types.StringPointerValue(ruleCreate.RuleCreate.Rule.Description)
	or.Type = types.StringValue(ruleCreate.RuleCreate.Rule.Type)
	or.Value = types.StringValue(value)
	or.SourceType = types.StringValue(string(ruleCreate.RuleCreate.Rule.SourceType))
	or.SourceUUID = types.StringValue(sourceUUID)
	or.TargetType = types.StringValue(string(ruleCreate.RuleCreate.Rule.TargetType))
	or.TargetUUID = types.StringValue(targetUUID)
	or.Effect = types.StringValue(string(ruleCreate.RuleCreate.Rule.Effect))
	or.Action = types.StringValue(string(ruleCreate.RuleCreate.Rule.Action))
}

func updateOrganizationRuleUpdateState(or *organizationRuleResourceModel, ruleUpdate updateOrganizationRuleResponse, sourceUUID, targetUUID, value string) {
	or.Description = types.StringPointerValue(ruleUpdate.RuleUpdate.Rule.Description)
	or.Value = types.StringValue(value)
	or.SourceUUID = types.StringValue(sourceUUID)
	or.TargetUUID = types.StringValue(targetUUID)
}

func updateOrganizatonRuleReadState(or *organizationRuleResourceModel, orn getNodeNodeRule, value string) {
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
