package buildkite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"reflect"
	"regexp"
	"strings"
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
	"github.com/vektah/gqlparser/v2/gqlerror"
)

type clusterCacheRegistryResource struct {
	client *Client
}

type clusterCacheRegistryResourceModel struct {
	ID             types.String         `tfsdk:"id"`
	UUID           types.String         `tfsdk:"uuid"`
	ClusterID      types.String         `tfsdk:"cluster_id"`
	ClusterUUID    types.String         `tfsdk:"cluster_uuid"`
	OrganizationID types.String         `tfsdk:"organization_id"`
	Name           types.String         `tfsdk:"name"`
	Slug           types.String         `tfsdk:"slug"`
	Description    types.String         `tfsdk:"description"`
	Emoji          types.String         `tfsdk:"emoji"`
	Color          types.String         `tfsdk:"color"`
	Policy         jsontypes.Normalized `tfsdk:"policy"`
	CreatedAt      types.String         `tfsdk:"created_at"`
	UpdatedAt      types.String         `tfsdk:"updated_at"`
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
		MarkdownDescription: "A Cache Registry stores cached build data for a Buildkite Cluster. Buildkite Cache is currently in private preview and must be enabled for your Buildkite organization. Refresh removes state only after verifying that the registry or its parent cluster was deleted. Access or API errors retain state and return diagnostics.",
		Attributes: map[string]resource_schema.Attribute{
			"organization_id": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The GraphQL ID of the owning organization. The provider must remain configured for this organization.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
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
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
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
		organizationID, err := r.cacheRegistryOrganization(ctx)
		if err == nil {
			var uuid string
			uuid, err = r.cacheRegistryParent(ctx, organizationID, state.ClusterID.ValueString())
			if err == nil && uuid == "" {
				err = errors.New("the configured organization does not contain the requested cluster")
			}
			if err == nil {
				state.ClusterUUID = types.StringValue(uuid)
			}
		}
		if err == nil {
			log.Printf("Creating cache registry %q in cluster %s ...", state.Name.ValueString(), state.ClusterID.ValueString())
			result, err = createCacheRegistry(ctx, r.client.genqlient, organizationID, state.ClusterID.ValueString(), state.Name.ValueString(), optionalStringPayload(state.Description), optionalStringPayload(state.Emoji), optionalStringPayload(state.Color), cacheRegistryPolicyPayload(state.Policy))
			state.OrganizationID = types.StringValue(organizationID)
			if err != nil && result != nil && result.CacheRegistryCreate.CacheRegistry.Id != "" {
				return retry.NonRetryableError(err)
			}
		}
		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to create Cache Registry", fmt.Sprintf("Unable to create Cache Registry: %s", err))
		if result == nil || result.CacheRegistryCreate.CacheRegistry.Id == "" {
			return
		}
	}

	values := result.CacheRegistryCreate.CacheRegistry.CacheRegistryValues
	if err := cacheRegistryOwner(values, state.OrganizationID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Invalid Cache Registry create response", err.Error())
	}
	if err := updateClusterCacheRegistryAppliedState(&state, values); err != nil {
		resp.Diagnostics.AddError("Invalid Cache Registry create response", err.Error())
	}
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

	var cacheRegistry *CacheRegistryValues
	var absent bool
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var err error
		cacheRegistry, err = r.lookupCacheRegistry(ctx, &state)
		absent = errors.Is(err, errCacheRegistryAbsent)
		if absent {
			return nil
		}
		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Cache Registry", fmt.Sprintf("Unable to read Cache Registry: %s", err))
		return
	}

	if absent {
		resp.Diagnostics.AddWarning("Cache Registry not found", "Removing Cache Registry from state...")
		resp.State.RemoveResource(ctx)
		return
	}

	if err := updateClusterCacheRegistryState(&state, *cacheRegistry); err != nil {
		resp.Diagnostics.AddError("Invalid Cache Registry read response", err.Error())
	}
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
		_, err := r.lookupCacheRegistry(ctx, &state)
		if errors.Is(err, errCacheRegistryAbsent) {
			err = errors.New("cache registry no longer exists; run terraform plan before applying again")
		}
		if err == nil {
			log.Printf("Updating cache registry with ID %s ...", state.ID.ValueString())
			result, err = updateCacheRegistry(ctx, r.client.genqlient, state.OrganizationID.ValueString(), state.ID.ValueString(), plan.Name.ValueString(), optionalStringPayload(plan.Description), optionalStringPayload(plan.Emoji), optionalStringPayload(plan.Color), cacheRegistryPolicyPayload(plan.Policy))
			if err != nil && result != nil && result.CacheRegistryUpdate.CacheRegistry.Id != "" {
				return retry.NonRetryableError(err)
			}
		}
		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to update Cache Registry", fmt.Sprintf("Unable to update Cache Registry: %s", err))
		if result == nil || result.CacheRegistryUpdate.CacheRegistry.Id == "" {
			return
		}
	}

	values := result.CacheRegistryUpdate.CacheRegistry.CacheRegistryValues
	if values.Id != state.ID.ValueString() {
		resp.Diagnostics.AddError("Invalid Cache Registry update response", "The returned Cache Registry ID differs from the existing resource ID")
		return
	}
	if err := cacheRegistryOwner(values, state.OrganizationID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Invalid Cache Registry update response", err.Error())
	}
	plan.OrganizationID, plan.ClusterID, plan.ClusterUUID = state.OrganizationID, state.ClusterID, state.ClusterUUID
	if err := updateClusterCacheRegistryAppliedState(&plan, values); err != nil {
		resp.Diagnostics.AddError("Invalid Cache Registry update response", err.Error())
	}
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
		_, err := r.lookupCacheRegistry(ctx, &state)
		if errors.Is(err, errCacheRegistryAbsent) {
			return nil
		}
		if err != nil {
			return retryContextError(err)
		}

		log.Printf("Deleting cache registry with ID %s ...", state.ID.ValueString())
		result, err := deleteCacheRegistry(ctx, r.client.genqlient, state.OrganizationID.ValueString(), state.ID.ValueString())
		if err != nil {
			if isCacheRegistryNotFoundError(err) {
				_, verifyErr := r.lookupCacheRegistry(ctx, &state)
				if errors.Is(verifyErr, errCacheRegistryAbsent) {
					return nil
				}
				if verifyErr != nil {
					return retryContextError(verifyErr)
				}
			}
			return retryContextError(err)
		}
		if result.CacheRegistryDelete.DeletedCacheRegistryId != state.ID.ValueString() {
			return retry.NonRetryableError(errors.New("deletion response did not confirm the Cache Registry ID"))
		}
		return nil
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to delete Cache Registry", fmt.Sprintf("Unable to delete Cache Registry: %s", err))
	}
}

func verifyCacheRegistryOrganization(access CacheRegistryOrganizationAccess, expected string) error {
	if access.Id == "" || access.Permissions == nil || access.Permissions.OrganizationMemberView == nil || access.Permissions.OrganizationMemberView.Allowed == nil || !*access.Permissions.OrganizationMemberView.Allowed {
		return errors.New("unable to verify authorized organization membership; restore organization access before retrying")
	}
	if expected != "" && access.Id != expected {
		return fmt.Errorf("cache registry belongs to organization %q, but the provider is configured for organization %q", expected, access.Id)
	}
	return nil
}

func (r *clusterCacheRegistryResource) cacheRegistryOrganization(ctx context.Context) (string, error) {
	result, err := getCacheRegistryOrganization(ctx, r.client.genqlient, r.client.organization)
	if err != nil {
		return "", err
	}
	if result.Organization == nil {
		return "", errors.New("configured organization is missing or inaccessible")
	}
	return result.Organization.Id, verifyCacheRegistryOrganization(result.Organization.CacheRegistryOrganizationAccess, "")
}

func cacheRegistryOwner(values CacheRegistryValues, organizationID string) error {
	owner := values.CacheRegistryCluster.Organization
	if owner == nil || owner.Id == "" {
		return errors.New("unable to verify Cache Registry owning organization")
	}
	if owner.Id != organizationID {
		return fmt.Errorf("cache registry belongs to organization %q, but the provider is configured for organization %q", owner.Id, organizationID)
	}
	return nil
}

func cacheRegistryNextCursor(cursor *string, seen map[string]bool) error {
	if cursor == nil || *cursor == "" || seen[*cursor] {
		return errors.New("incomplete Cache Registry lookup: missing or repeated pagination cursor")
	}
	seen[*cursor] = true
	return nil
}

func (r *clusterCacheRegistryResource) cacheRegistryParent(ctx context.Context, organizationID, clusterID string) (string, error) {
	var cursor *string
	seen := map[string]bool{}
	for {
		result, err := getCacheRegistryClusters(ctx, r.client.genqlient, r.client.organization, cursor)
		if err != nil {
			return "", err
		}
		if result.Organization == nil {
			return "", errors.New("configured organization is missing or inaccessible")
		}
		if err := verifyCacheRegistryOrganization(result.Organization.CacheRegistryOrganizationAccess, organizationID); err != nil {
			return "", err
		}
		connection := result.Organization.Clusters
		if connection == nil || connection.Edges == nil || connection.PageInfo == nil {
			return "", errors.New("organization clusters are inaccessible")
		}
		for _, edge := range connection.Edges {
			if edge == nil || edge.Node == nil || edge.Node.Id == "" || edge.Node.Uuid == "" {
				return "", errors.New("incomplete organization clusters response")
			}
			if edge.Node.Id == clusterID {
				return edge.Node.Uuid, nil
			}
		}
		if !connection.PageInfo.HasNextPage {
			return "", nil
		}
		cursor = connection.PageInfo.EndCursor
		if err := cacheRegistryNextCursor(cursor, seen); err != nil {
			return "", err
		}
	}
}

var errCacheRegistryAbsent = errors.New("cache registry is confirmed absent")

func (r *clusterCacheRegistryResource) lookupCacheRegistry(ctx context.Context, state *clusterCacheRegistryResourceModel) (*CacheRegistryValues, error) {
	result, err := getCacheRegistryByNode(ctx, r.client.genqlient, state.ID.ValueString(), r.client.organization)
	if err != nil {
		return nil, err
	}
	if result.Organization == nil {
		return nil, errors.New("configured organization is missing or inaccessible")
	}
	if err := verifyCacheRegistryOrganization(result.Organization.CacheRegistryOrganizationAccess, state.OrganizationID.ValueString()); err != nil {
		return nil, err
	}
	organizationID := result.Organization.Id
	if node, ok := result.Node.(*getCacheRegistryByNodeNodeCacheRegistry); ok && node != nil {
		if err := cacheRegistryOwner(node.CacheRegistryValues, organizationID); err != nil {
			return nil, err
		}
		state.OrganizationID = types.StringValue(organizationID)
		return &node.CacheRegistryValues, nil
	}
	if state.OrganizationID.ValueString() == "" || state.ClusterID.ValueString() == "" {
		return nil, errors.New("cannot establish ownership of a missing or inaccessible Cache Registry. For older state, restore access and refresh to record organization_id; an ID-only import requires an accessible registry")
	}
	clusterUUID, err := r.cacheRegistryParent(ctx, organizationID, state.ClusterID.ValueString())
	if err != nil {
		return nil, err
	}
	if clusterUUID == "" {
		return nil, errCacheRegistryAbsent
	}
	var cursor *string
	seen := map[string]bool{}
	for {
		result, err := getClusterCacheRegistries(ctx, r.client.genqlient, r.client.organization, clusterUUID, cursor)
		if err != nil {
			return nil, err
		}
		if result.Organization == nil {
			return nil, fmt.Errorf("organization %q was not found or is inaccessible", r.client.organization)
		}
		if err := verifyCacheRegistryOrganization(result.Organization.CacheRegistryOrganizationAccess, organizationID); err != nil {
			return nil, err
		}
		if result.Organization.Cluster == nil || result.Organization.Cluster.Id != state.ClusterID.ValueString() {
			return nil, fmt.Errorf("cluster %q was not found or is inaccessible", clusterUUID)
		}
		connection := result.Organization.Cluster.CacheRegistries
		if connection == nil || connection.Edges == nil || connection.PageInfo == nil {
			return nil, errors.New("cache registries are not available for this cluster")
		}

		for _, edge := range connection.Edges {
			if edge == nil || edge.Node == nil || edge.Node.Id == "" {
				return nil, errors.New("incomplete cache registries response")
			}
			if edge.Node.Id == state.ID.ValueString() {
				return nil, errors.New("the Cache Registry still exists but is not accessible through the Relay node lookup")
			}
		}
		if !connection.PageInfo.HasNextPage {
			return nil, errCacheRegistryAbsent
		}
		cursor = connection.PageInfo.EndCursor
		if err := cacheRegistryNextCursor(cursor, seen); err != nil {
			return nil, err
		}
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

func updateClusterCacheRegistryAppliedState(state *clusterCacheRegistryResourceModel, result CacheRegistryValues) error {
	planned := state.Policy
	organizationID, clusterID, clusterUUID, name := state.OrganizationID, state.ClusterID, state.ClusterUUID, state.Name
	err := updateClusterCacheRegistryState(state, result)
	state.OrganizationID, state.ClusterID, state.ClusterUUID = organizationID, clusterID, clusterUUID
	if result.Name == "" {
		state.Name = name
	}
	if result.Uuid == "" {
		state.UUID = types.StringNull()
	}
	if result.Slug == "" {
		state.Slug = types.StringNull()
	}
	if result.CreatedAt.IsZero() {
		state.CreatedAt = types.StringNull()
	}
	if result.UpdatedAt.IsZero() {
		state.UpdatedAt = types.StringNull()
	}
	if err != nil {
		return err
	}
	if !planned.IsNull() && !planned.IsUnknown() && !planned.Equal(state.Policy) {
		return fmt.Errorf("cache registry %q returned a policy that differs from the plan. The returned policy and remote identity have been recorded in state. Review the remote policy and run terraform plan before applying again. Planned policy: %s; returned policy: %s", state.ID.ValueString(), planned.ValueString(), state.Policy.ValueString())
	}
	return nil
}

func updateClusterCacheRegistryState(state *clusterCacheRegistryResourceModel, result CacheRegistryValues) error {
	state.ID = types.StringValue(result.Id)
	state.UUID = types.StringValue(result.Uuid)
	state.ClusterID = types.StringValue(result.CacheRegistryCluster.Id)
	state.ClusterUUID = types.StringValue(result.CacheRegistryCluster.Uuid)
	if result.CacheRegistryCluster.Organization != nil {
		state.OrganizationID = types.StringValue(result.CacheRegistryCluster.Organization.Id)
	}
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
	if result.Policy == nil {
		return fmt.Errorf("cache registry %q returned a null policy; expected a JSON object. The remote identity has been recorded in state. Check the API response and run terraform plan before applying again", result.Id)
	}
	if _, err := normalizeCacheRegistryPolicy(*result.Policy); err != nil {
		if !json.Valid([]byte(*result.Policy)) {
			state.Policy = jsontypes.NewNormalizedNull()
		}
		return fmt.Errorf("cache registry %q returned an invalid policy: %w. The remote identity has been recorded in state. Check the API response and run terraform plan before applying again. Returned policy: %s", result.Id, err, *result.Policy)
	}
	return nil
}

func cacheRegistryPolicyPayload(policy jsontypes.Normalized) *string {
	if policy.IsNull() || policy.IsUnknown() {
		return nil
	}
	return policy.ValueStringPointer()
}

var cacheRegistryNotFoundRegex = regexp.MustCompile(`(?i)no\s+cache\s+registry\s+found`)

func isCacheRegistryNotFoundError(err error) bool {
	var errList gqlerror.List
	if !errors.As(err, &errList) {
		return false
	}
	for _, graphqlError := range errList {
		if graphqlError == nil || !cacheRegistryNotFoundRegex.MatchString(graphqlError.Message) {
			return false
		}
	}
	return len(errList) > 0
}

func cacheRegistryPoliciesEquivalent(left, right string) bool {
	leftPolicy, err := normalizeCacheRegistryPolicy(left)
	if err != nil {
		return false
	}
	rightPolicy, err := normalizeCacheRegistryPolicy(right)
	return err == nil && reflect.DeepEqual(leftPolicy, rightPolicy)
}

func normalizeCacheRegistryPolicy(raw string) (map[string]any, error) {
	if !json.Valid([]byte(raw)) {
		return nil, errors.New("expected a valid JSON object")
	}
	var policy map[string]any
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&policy); err != nil {
		return nil, fmt.Errorf("expected a JSON object: %w", err)
	}
	if policy == nil {
		return nil, errors.New("expected a JSON object, not null")
	}
	for section, scopes := range map[string]any{"save": map[string]any{}, "restore": []any{}} {
		if _, exists := policy[section]; !exists {
			policy[section] = map[string]any{}
		}
		value, ok := policy[section].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("expected %s to be an object", section)
		}
		if _, exists := value["scopes"]; !exists {
			value["scopes"] = scopes
		}
		if reflect.TypeOf(value["scopes"]) != reflect.TypeOf(scopes) {
			return nil, fmt.Errorf("unexpected type for %s.scopes", section)
		}
	}
	if _, exists := policy["rules"]; !exists {
		policy["rules"] = []any{}
	}
	rules, ok := policy["rules"].([]any)
	if !ok {
		return nil, errors.New("expected rules to be an array")
	}
	for i, rawRule := range rules {
		rule, ok := rawRule.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("expected rules[%d] to be an object", i)
		}
		if action, ok := rule["action"].(string); ok {
			rule["action"] = []any{action}
		}
		for _, key := range []string{"name", "when"} {
			if rule[key] == nil {
				delete(rule, key)
			}
		}
	}
	return policy, nil
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
