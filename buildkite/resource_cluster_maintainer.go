package buildkite

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

// Shared maintainer types for data sources
type maintainerModel struct {
	PermissionUUID types.String `tfsdk:"permission_uuid"`
	ActorUUID      types.String `tfsdk:"actor_uuid"`
	ActorType      types.String `tfsdk:"actor_type"`
	ActorName      types.String `tfsdk:"actor_name"`
	ActorEmail     types.String `tfsdk:"actor_email"`
	ActorSlug      types.String `tfsdk:"actor_slug"`
}

type clusterMaintainerResource struct {
	client *Client
}

type clusterMaintainerResourceModel struct {
	ID          types.String `tfsdk:"id"`
	ClusterUUID types.String `tfsdk:"cluster_uuid"`
	UserUUID    types.String `tfsdk:"user_uuid"`
	TeamUUID    types.String `tfsdk:"team_uuid"`
	ActorUUID   types.String `tfsdk:"actor_uuid"`
	ActorType   types.String `tfsdk:"actor_type"`
	ActorName   types.String `tfsdk:"actor_name"`
	ActorEmail  types.String `tfsdk:"actor_email"`
	ActorSlug   types.String `tfsdk:"actor_slug"`
}

type clusterMaintainerAPIResponse struct {
	ID    string `json:"id"`
	Actor struct {
		ID        string  `json:"id"`
		GraphQLID string  `json:"graphql_id"`
		Name      *string `json:"name,omitempty"`
		Email     *string `json:"email,omitempty"`
		Slug      *string `json:"slug,omitempty"`
		Type      string  `json:"type"`
	} `json:"actor"`
}

type clusterMaintainerCreateRequest struct {
	User *string `json:"user,omitempty"`
	Team *string `json:"team,omitempty"`
}

func newClusterMaintainerResource() resource.Resource {
	return &clusterMaintainerResource{}
}

func (c *clusterMaintainerResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_maintainer"
}

func (c *clusterMaintainerResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	c.client = req.ProviderData.(*Client)
}

func (c *clusterMaintainerResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resource_schema.Schema{
		Version: 1,
		MarkdownDescription: heredoc.Doc(`
			This resource allows you to manage cluster maintainers in Buildkite. Maintainers can be either users or teams
			that have permission to manage a specific cluster. Find out more information in our
			[documentation](https://buildkite.com/docs/clusters/manage-clusters).
		`),
		Attributes: map[string]resource_schema.Attribute{
			"id": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The permission ID of the cluster maintainer.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"cluster_uuid": resource_schema.StringAttribute{
				MarkdownDescription: "The UUID of the cluster.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"user_uuid": resource_schema.StringAttribute{
				MarkdownDescription: heredoc.Doc(`
					The UUID of the user to add as a maintainer. This is mutually exclusive with team_uuid.
					Only one of user_uuid or team_uuid can be specified.
				`),
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"team_uuid": resource_schema.StringAttribute{
				MarkdownDescription: heredoc.Doc(`
					The UUID of the team to add as a maintainer. This is mutually exclusive with user_uuid.
					Only one of user_uuid or team_uuid can be specified.
				`),
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"actor_uuid": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the actor (user or team) that is the maintainer.",
			},
			"actor_type": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The type of the actor (user or team).",
			},
			"actor_name": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The name of the actor.",
			},
			"actor_email": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The email of the actor (only for users).",
			},
			"actor_slug": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The slug of the actor (only for teams).",
			},
		},
	}
}

func (c *clusterMaintainerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var state *clusterMaintainerResourceModel

	diags := req.Plan.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Validate that exactly one of user_uuid or team_uuid is specified
	userUUIDSet := !state.UserUUID.IsNull() && !state.UserUUID.IsUnknown() && state.UserUUID.ValueString() != ""
	teamUUIDSet := !state.TeamUUID.IsNull() && !state.TeamUUID.IsUnknown() && state.TeamUUID.ValueString() != ""

	if !userUUIDSet && !teamUUIDSet {
		resp.Diagnostics.AddError(
			"Invalid configuration",
			"Either user_uuid or team_uuid must be specified",
		)
		return
	}

	if userUUIDSet && teamUUIDSet {
		resp.Diagnostics.AddError(
			"Invalid configuration",
			"Only one of user_uuid or team_uuid can be specified, not both",
		)
		return
	}

	timeout, diags := c.client.timeouts.Create(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var result *clusterMaintainerAPIResponse
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var err error
		result, err = c.createClusterMaintainer(ctx, state)
		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create cluster maintainer",
			fmt.Sprintf("Unable to create cluster maintainer: %s", err.Error()),
		)
		return
	}

	// Update state with response data
	c.updateStateFromAPIResponse(state, result)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (c *clusterMaintainerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state clusterMaintainerResourceModel

	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := c.client.timeouts.Read(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var result *clusterMaintainerAPIResponse
	var notFound bool
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var err error
		result, err = c.getClusterMaintainer(ctx, &state)
		if err != nil {
			if strings.Contains(err.Error(), "404") {
				notFound = true
				return nil
			}
			return retryContextError(err)
		}
		return nil
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read cluster maintainer",
			fmt.Sprintf("Unable to read cluster maintainer: %s", err.Error()),
		)
		return
	}

	if notFound {
		resp.Diagnostics.AddWarning(
			"Cluster maintainer not found",
			"Removing cluster maintainer from state...",
		)
		resp.State.RemoveResource(ctx)
		return
	}

	// Update state with response data
	c.updateStateFromAPIResponse(&state, result)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (c *clusterMaintainerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Since all meaningful attributes require replacement, this should not be called
	resp.Diagnostics.AddError(
		"Update not supported",
		"Cluster maintainer resource does not support updates. All changes require replacement.",
	)
}

func (c *clusterMaintainerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state clusterMaintainerResourceModel

	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := c.client.timeouts.Delete(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		err := c.deleteClusterMaintainer(ctx, &state)
		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete cluster maintainer",
			fmt.Sprintf("Unable to delete cluster maintainer: %s", err.Error()),
		)
		return
	}
}

func (c *clusterMaintainerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import format: {cluster_uuid}/{permission_uuid}
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 {
		resp.Diagnostics.AddError(
			"Invalid import format",
			"Expected format: {cluster_uuid}/{permission_uuid}",
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_uuid"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)

	// The Read method will populate the rest of the state
}

func (c *clusterMaintainerResource) UpgradeState(ctx context.Context) map[int64]resource.StateUpgrader {
	schemaV0 := clusterMaintainerSchemaV0()
	return map[int64]resource.StateUpgrader{
		// State upgrade from Version 0 (cluster_id, user_id, team_id, actor_id) to Version 1 (cluster_uuid, user_uuid, team_uuid, actor_uuid)
		0: {
			PriorSchema:   &schemaV0,
			StateUpgrader: upgradeClusterMaintainerStateV0toV1,
		},
	}
}

// API helper functions

func (c *clusterMaintainerResource) createClusterMaintainer(ctx context.Context, state *clusterMaintainerResourceModel) (*clusterMaintainerAPIResponse, error) {
	path := fmt.Sprintf("/v2/organizations/%s/clusters/%s/maintainers",
		c.client.organization,
		state.ClusterUUID.ValueString(),
	)

	reqBody := clusterMaintainerCreateRequest{}
	if !state.UserUUID.IsNull() && state.UserUUID.ValueString() != "" {
		userUUID := state.UserUUID.ValueString()
		reqBody.User = &userUUID
	}
	if !state.TeamUUID.IsNull() && state.TeamUUID.ValueString() != "" {
		teamUUID := state.TeamUUID.ValueString()
		reqBody.Team = &teamUUID
	}

	var result clusterMaintainerAPIResponse
	err := c.client.makeRequest(ctx, http.MethodPost, path, reqBody, &result)
	if err != nil {
		return nil, fmt.Errorf("error creating cluster maintainer: %w", err)
	}

	return &result, nil
}

func (c *clusterMaintainerResource) getClusterMaintainer(ctx context.Context, state *clusterMaintainerResourceModel) (*clusterMaintainerAPIResponse, error) {
	path := fmt.Sprintf("/v2/organizations/%s/clusters/%s/maintainers/%s",
		c.client.organization,
		state.ClusterUUID.ValueString(),
		state.ID.ValueString(),
	)

	var result clusterMaintainerAPIResponse
	err := c.client.makeRequest(ctx, http.MethodGet, path, nil, &result)
	if err != nil {
		return nil, fmt.Errorf("error getting cluster maintainer: %w", err)
	}

	return &result, nil
}

func (c *clusterMaintainerResource) deleteClusterMaintainer(ctx context.Context, state *clusterMaintainerResourceModel) error {
	path := fmt.Sprintf("/v2/organizations/%s/clusters/%s/maintainers/%s",
		c.client.organization,
		state.ClusterUUID.ValueString(),
		state.ID.ValueString(),
	)

	err := c.client.makeRequest(ctx, http.MethodDelete, path, nil, nil)
	if err != nil {
		return fmt.Errorf("error deleting cluster maintainer: %w", err)
	}

	return nil
}

func (c *clusterMaintainerResource) updateStateFromAPIResponse(state *clusterMaintainerResourceModel, result *clusterMaintainerAPIResponse) {
	state.ID = types.StringValue(result.ID)
	state.ActorUUID = types.StringValue(result.Actor.ID)
	state.ActorType = types.StringValue(result.Actor.Type)

	if result.Actor.Name != nil {
		state.ActorName = types.StringValue(*result.Actor.Name)
	} else {
		state.ActorName = types.StringNull()
	}

	if result.Actor.Email != nil {
		state.ActorEmail = types.StringValue(*result.Actor.Email)
	} else {
		state.ActorEmail = types.StringNull()
	}

	if result.Actor.Slug != nil {
		state.ActorSlug = types.StringValue(*result.Actor.Slug)
	} else {
		state.ActorSlug = types.StringNull()
	}

	// Ensure the correct user_uuid or team_uuid is set based on actor type
	switch result.Actor.Type {
	case "user":
		state.UserUUID = types.StringValue(result.Actor.ID)
		state.TeamUUID = types.StringNull()
	case "team":
		state.TeamUUID = types.StringValue(result.Actor.ID)
		state.UserUUID = types.StringNull()
	}
}

// listClusterMaintainers retrieves all maintainers for a specific cluster
func (c *Client) listClusterMaintainers(ctx context.Context, orgSlug, clusterID string) ([]maintainerModel, error) {
	path := fmt.Sprintf("/v2/organizations/%s/clusters/%s/maintainers", orgSlug, clusterID)

	var apiResponse []clusterMaintainerAPIResponse
	err := c.makeRequest(ctx, http.MethodGet, path, nil, &apiResponse)
	if err != nil {
		// Handle different error types appropriately
		if strings.Contains(err.Error(), "status: 403") {
			return []maintainerModel{}, fmt.Errorf("insufficient permissions to read cluster maintainers (requires manage_cluster permission): %w", err)
		}
		if strings.Contains(err.Error(), "status: 404") {
			return []maintainerModel{}, fmt.Errorf("cluster not found: %w", err)
		}
		return nil, fmt.Errorf("error listing cluster maintainers: %w", err)
	}

	// Convert API response to maintainer models
	maintainers := make([]maintainerModel, len(apiResponse))
	for i, maintainer := range apiResponse {
		maintainers[i] = maintainerModel{
			PermissionUUID: types.StringValue(maintainer.ID),
			ActorUUID:      types.StringValue(maintainer.Actor.ID),
			ActorType:      types.StringValue(maintainer.Actor.Type),
		}

		if maintainer.Actor.Name != nil {
			maintainers[i].ActorName = types.StringValue(*maintainer.Actor.Name)
		} else {
			maintainers[i].ActorName = types.StringNull()
		}

		if maintainer.Actor.Email != nil {
			maintainers[i].ActorEmail = types.StringValue(*maintainer.Actor.Email)
		} else {
			maintainers[i].ActorEmail = types.StringNull()
		}

		if maintainer.Actor.Slug != nil {
			maintainers[i].ActorSlug = types.StringValue(*maintainer.Actor.Slug)
		} else {
			maintainers[i].ActorSlug = types.StringNull()
		}
	}

	return maintainers, nil
}

// clusterMaintainerSchemaV0 returns the schema for version 0 of the cluster maintainer resource
func clusterMaintainerSchemaV0() resource_schema.Schema {
	return resource_schema.Schema{
		Attributes: map[string]resource_schema.Attribute{
			"id": resource_schema.StringAttribute{
				Computed: true,
			},
			"cluster_id": resource_schema.StringAttribute{
				Required: true,
			},
			"user_id": resource_schema.StringAttribute{
				Optional: true,
			},
			"team_id": resource_schema.StringAttribute{
				Optional: true,
			},
			"actor_id": resource_schema.StringAttribute{
				Computed: true,
			},
			"actor_type": resource_schema.StringAttribute{
				Computed: true,
			},
			"actor_name": resource_schema.StringAttribute{
				Computed: true,
			},
			"actor_email": resource_schema.StringAttribute{
				Computed: true,
			},
			"actor_slug": resource_schema.StringAttribute{
				Computed: true,
			},
		},
	}
}

// upgradeClusterMaintainerStateV0toV1 migrates state from version 0 to version 1
func upgradeClusterMaintainerStateV0toV1(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	type clusterMaintainerResourceModelV0 struct {
		ID         types.String `tfsdk:"id"`
		ClusterID  types.String `tfsdk:"cluster_id"`
		UserID     types.String `tfsdk:"user_id"`
		TeamID     types.String `tfsdk:"team_id"`
		ActorID    types.String `tfsdk:"actor_id"`
		ActorType  types.String `tfsdk:"actor_type"`
		ActorName  types.String `tfsdk:"actor_name"`
		ActorEmail types.String `tfsdk:"actor_email"`
		ActorSlug  types.String `tfsdk:"actor_slug"`
	}

	var modelV0 clusterMaintainerResourceModelV0

	resp.Diagnostics.Append(req.State.Get(ctx, &modelV0)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Map old attribute names to new attribute names
	modelV1 := clusterMaintainerResourceModel{
		ID:          modelV0.ID,
		ClusterUUID: modelV0.ClusterID,
		UserUUID:    modelV0.UserID,
		TeamUUID:    modelV0.TeamID,
		ActorUUID:   modelV0.ActorID,
		ActorType:   modelV0.ActorType,
		ActorName:   modelV0.ActorName,
		ActorEmail:  modelV0.ActorEmail,
		ActorSlug:   modelV0.ActorSlug,
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, modelV1)...)
}
