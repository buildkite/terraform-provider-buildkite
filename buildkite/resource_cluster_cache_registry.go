package buildkite

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"time"

	bkplanmodifier "github.com/buildkite/terraform-provider-buildkite/internal/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

type clusterCacheRegistryResource struct {
	client *Client
}

type clusterCacheRegistryResourceModel struct {
	ID          types.String         `tfsdk:"id"`
	UUID        types.String         `tfsdk:"uuid"`
	ClusterID   types.String         `tfsdk:"cluster_id"`
	ClusterUUID types.String         `tfsdk:"cluster_uuid"`
	Name        types.String         `tfsdk:"name"`
	Slug        types.String         `tfsdk:"slug"`
	Description types.String         `tfsdk:"description"`
	Emoji       types.String         `tfsdk:"emoji"`
	Color       types.String         `tfsdk:"color"`
	Policy      jsontypes.Normalized `tfsdk:"policy"`
	CreatedAt   types.String         `tfsdk:"created_at"`
	UpdatedAt   types.String         `tfsdk:"updated_at"`
}

func newClusterCacheRegistryResource() resource.Resource {
	return &clusterCacheRegistryResource{}
}

func (clusterCacheRegistryResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_cache_registry"
}

func (r *clusterCacheRegistryResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.client = req.ProviderData.(*Client)
	}
}

func (clusterCacheRegistryResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resource_schema.Schema{
		MarkdownDescription: "A Cache Registry stores cached build data for a Buildkite Cluster.",
		Attributes: map[string]resource_schema.Attribute{
			"id": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The GraphQL ID of the cache registry.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"uuid": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the cache registry.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"cluster_id": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The GraphQL ID of the cluster that owns the cache registry.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"cluster_uuid": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the cluster that owns the cache registry.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the cache registry.",
			},
			"slug": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The slug of the cache registry.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					bkplanmodifier.UseStateIfUnchanged("name"),
				},
			},
			"description": resource_schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "A description for the cache registry.",
			},
			"emoji": resource_schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "An emoji for the cache registry using Buildkite emoji syntax.",
			},
			"color": resource_schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "A color for the cache registry as a hex code, for example `#BADA55`.",
			},
			"policy": resource_schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				CustomType:          jsontypes.NormalizedType{},
				MarkdownDescription: "The cache registry policy as a JSON object. When omitted, the API default is retained. To clear all policy rules, set `save.scopes` to `{}`, `restore.scopes` to `[]`, and `rules` to `[]`. Removing an explicitly configured policy retains its current value.",
				Validators: []validator.String{
					cacheRegistryPolicyValidator{},
				},
			},
			"created_at": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The time when the cache registry was created, in RFC3339 format.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"updated_at": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The time when the cache registry was last updated, in RFC3339 format.",
			},
		},
	}
}

func (r *clusterCacheRegistryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var state clusterCacheRegistryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := r.client.timeouts.Create(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var result *createCacheRegistryResponse
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		organizationID, err := r.client.GetOrganizationID()
		if err == nil {
			log.Printf("Creating cache registry %q in cluster %s ...", state.Name.ValueString(), state.ClusterID.ValueString())
			result, err = createCacheRegistry(ctx, r.client.genqlient, *organizationID, state.ClusterID.ValueString(), state.Name.ValueString(), optionalStringPayload(state.Description), optionalStringPayload(state.Emoji), optionalStringPayload(state.Color), cacheRegistryPolicyPayload(state.Policy))
		}
		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to create Cache Registry", fmt.Sprintf("Unable to create Cache Registry: %s", err))
		return
	}

	updateClusterCacheRegistryState(&state, result.CacheRegistryCreate.CacheRegistry.CacheRegistryValues)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *clusterCacheRegistryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state clusterCacheRegistryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := r.client.timeouts.Read(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var result *getCacheRegistryByNodeResponse
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var err error
		log.Printf("Reading cache registry with ID %s ...", state.ID.ValueString())
		result, err = getCacheRegistryByNode(ctx, r.client.genqlient, state.ID.ValueString())
		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Cache Registry", fmt.Sprintf("Unable to read Cache Registry: %s", err))
		return
	}

	cacheRegistry, ok := result.Node.(*getCacheRegistryByNodeNodeCacheRegistry)
	if !ok || cacheRegistry == nil {
		resp.Diagnostics.AddWarning("Cache Registry not found", "Removing Cache Registry from state...")
		resp.State.RemoveResource(ctx)
		return
	}

	updateClusterCacheRegistryState(&state, cacheRegistry.CacheRegistryValues)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *clusterCacheRegistryResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state clusterCacheRegistryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := r.client.timeouts.Update(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var result *updateCacheRegistryResponse
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		organizationID, err := r.client.GetOrganizationID()
		if err == nil {
			log.Printf("Updating cache registry with ID %s ...", state.ID.ValueString())
			result, err = updateCacheRegistry(ctx, r.client.genqlient, *organizationID, state.ID.ValueString(), plan.Name.ValueString(), optionalStringPayload(plan.Description), optionalStringPayload(plan.Emoji), optionalStringPayload(plan.Color), cacheRegistryPolicyPayload(plan.Policy))
		}
		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to update Cache Registry", fmt.Sprintf("Unable to update Cache Registry: %s", err))
		return
	}

	updateClusterCacheRegistryState(&plan, result.CacheRegistryUpdate.CacheRegistry.CacheRegistryValues)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *clusterCacheRegistryResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state clusterCacheRegistryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := r.client.timeouts.Delete(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		organizationID, err := r.client.GetOrganizationID()
		if err == nil {
			log.Printf("Deleting cache registry with ID %s ...", state.ID.ValueString())
			_, err = deleteCacheRegistry(ctx, r.client.genqlient, *organizationID, state.ID.ValueString())
		}
		if isResourceNotFoundError(err) {
			return nil
		}
		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to delete Cache Registry", fmt.Sprintf("Unable to delete Cache Registry: %s", err))
	}
}

func (r *clusterCacheRegistryResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() || req.State.Raw.IsNull() {
		return
	}

	var plan, state clusterCacheRegistryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !plan.Name.Equal(state.Name) {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("slug"), types.StringUnknown())...)
	}
}

func (r *clusterCacheRegistryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func updateClusterCacheRegistryState(state *clusterCacheRegistryResourceModel, result CacheRegistryValues) {
	state.ID = types.StringValue(result.Id)
	state.UUID = types.StringValue(result.Uuid)
	state.ClusterID = types.StringValue(result.CacheRegistryCluster.Id)
	state.ClusterUUID = types.StringValue(result.CacheRegistryCluster.Uuid)
	state.Name = types.StringValue(result.Name)
	state.Slug = types.StringValue(result.Slug)
	state.Description = types.StringPointerValue(result.Description)
	state.Emoji = types.StringPointerValue(result.Emoji)
	state.Color = types.StringPointerValue(result.Color)
	if state.Policy.IsNull() || state.Policy.IsUnknown() || result.Policy == nil || !cacheRegistryPoliciesEquivalent(state.Policy.ValueString(), *result.Policy) {
		state.Policy = jsontypes.NewNormalizedPointerValue(result.Policy)
	}
	state.CreatedAt = types.StringValue(result.CreatedAt.Format(time.RFC3339))
	state.UpdatedAt = types.StringValue(result.UpdatedAt.Format(time.RFC3339))
}

func cacheRegistryPolicyPayload(policy jsontypes.Normalized) *string {
	if policy.IsNull() || policy.IsUnknown() {
		return nil
	}
	return policy.ValueStringPointer()
}

func cacheRegistryPoliciesEquivalent(left, right string) bool {
	var leftPolicy, rightPolicy map[string]any
	if json.Unmarshal([]byte(left), &leftPolicy) != nil || json.Unmarshal([]byte(right), &rightPolicy) != nil {
		return false
	}

	normalize := func(policy map[string]any) {
		for section, scopes := range map[string]any{"save": map[string]any{}, "restore": []any{}} {
			value, ok := policy[section].(map[string]any)
			if !ok {
				value = map[string]any{}
				policy[section] = value
			}
			if _, ok := value["scopes"]; !ok {
				value["scopes"] = scopes
			}
		}
		if _, ok := policy["rules"]; !ok {
			policy["rules"] = []any{}
		}
		if rules, ok := policy["rules"].([]any); ok {
			for _, rawRule := range rules {
				rule, ok := rawRule.(map[string]any)
				if !ok {
					continue
				}
				if action, ok := rule["action"].(string); ok {
					rule["action"] = []any{action}
				}
			}
		}
	}

	normalize(leftPolicy)
	normalize(rightPolicy)
	return reflect.DeepEqual(leftPolicy, rightPolicy)
}

type cacheRegistryPolicyValidator struct{}

func (cacheRegistryPolicyValidator) Description(context.Context) string {
	return "must be a valid JSON object"
}

func (cacheRegistryPolicyValidator) MarkdownDescription(context.Context) string {
	return "Must be a valid JSON object."
}

func (cacheRegistryPolicyValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	var policy map[string]json.RawMessage
	if err := json.Unmarshal([]byte(req.ConfigValue.ValueString()), &policy); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid cache registry policy", fmt.Sprintf("Policy must be a valid JSON object: %s", err))
		return
	}
	if policy == nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid cache registry policy", "Policy must be a JSON object, not null.")
	}
}
