package buildkite

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	custom_modifier "github.com/buildkite/terraform-provider-buildkite/internal/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/shurcooL/graphql"
)

type teamResource struct {
	client *Client
}

type teamResourceModel struct {
	ID                          types.String `tfsdk:"id"`
	UUID                        types.String `tfsdk:"uuid"`
	Name                        types.String `tfsdk:"name"`
	Description                 types.String `tfsdk:"description"`
	Privacy                     types.String `tfsdk:"privacy"`
	IsDefaultTeam               types.Bool   `tfsdk:"default_team"`
	DefaultMemberRole           types.String `tfsdk:"default_member_role"`
	Slug                        types.String `tfsdk:"slug"`
	MembersCanCreatePipelines   types.Bool   `tfsdk:"members_can_create_pipelines"`
	MembersCanCreateSuites      types.Bool   `tfsdk:"members_can_create_suites"`
	MembersCanCreateRegistries  types.Bool   `tfsdk:"members_can_create_registries"`
	MembersCanDestroyRegistries types.Bool   `tfsdk:"members_can_destroy_registries"`
	MembersCanDestroyPackages   types.Bool   `tfsdk:"members_can_destroy_packages"`
}

// This is required due to the getTeam function not using Genqlient
type TeamNode struct {
	Description               graphql.String
	ID                        graphql.String
	IsDefaultTeam             graphql.Boolean
	DefaultMemberRole         graphql.String
	Name                      graphql.String
	MembersCanCreatePipelines graphql.Boolean
	Privacy                   graphql.String
	Slug                      graphql.String
	UUID                      graphql.String
}

type teamAPIResponse struct {
	ID                          string `json:"id"`                            // UUID
	GraphQLID                   string `json:"graphql_id"`                    // base64 GraphQL ID
	Name                        string `json:"name"`
	Slug                        string `json:"slug"`
	Description                 string `json:"description"`
	Privacy                     string `json:"privacy"`                      // "visible" or "secret"
	Default                     bool   `json:"default"`                      // response uses "default"
	DefaultMemberRole           string `json:"default_member_role"`          // "member" or "maintainer"
	MembersCanCreatePipelines   bool   `json:"members_can_create_pipelines"`
	MembersCanCreateSuites      bool   `json:"members_can_create_suites"`
	MembersCanCreateRegistries  bool   `json:"members_can_create_registries"`
	MembersCanDestroyRegistries bool   `json:"members_can_destroy_registries"`
	MembersCanDestroyPackages   bool   `json:"members_can_destroy_packages"`
}

type teamCreateUpdateRequest struct {
	Name                        string `json:"name"`
	Description                 string `json:"description"`
	Privacy                     string `json:"privacy"`            // "visible" or "secret"
	IsDefaultTeam               bool   `json:"is_default_team"`    // request uses "is_default_team"
	DefaultMemberRole           string `json:"default_member_role"` // "member" or "maintainer"
	MembersCanCreatePipelines   bool   `json:"members_can_create_pipelines"`
	MembersCanCreateSuites      bool   `json:"members_can_create_suites"`
	MembersCanCreateRegistries  bool   `json:"members_can_create_registries"`
	MembersCanDestroyRegistries bool   `json:"members_can_destroy_registries"`
	MembersCanDestroyPackages   bool   `json:"members_can_destroy_packages"`
}

func newTeamResource() resource.Resource {
	return &teamResource{}
}

func (t *teamResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_team"
}

func (t *teamResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	t.client = req.ProviderData.(*Client)
}

func (t *teamResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resource_schema.Schema{
		MarkdownDescription: "A Team is a group of users that can be given permissions for using Pipelines." +
			"This feature is only available to Business and Enterprise customers.  You can find out more about Teams in the Buildkite [documentation](https://buildkite.com/docs/team-management/permissions).",
		Attributes: map[string]resource_schema.Attribute{
			"id": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The GraphQL ID of the team.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"uuid": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the team.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": resource_schema.StringAttribute{
				MarkdownDescription: "The name of the team.",
				Required:            true,
			},
			"description": resource_schema.StringAttribute{
				Computed:            true,
				Default:             stringdefault.StaticString(""),
				Optional:            true,
				MarkdownDescription: "A description for the team. This is displayed in the Buildkite UI.",
			},
			"privacy": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The privacy setting for the team. This can be either `VISIBLE` or `SECRET`.",
				Validators: []validator.String{
					stringvalidator.OneOf("VISIBLE", "SECRET"),
				},
			},
			"default_team": resource_schema.BoolAttribute{
				Required:            true,
				MarkdownDescription: "Whether this is the default team for the organization.",
			},
			"default_member_role": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The default role for new members of the team. This can be either `MEMBER` or `MAINTAINER`.",
				Validators: []validator.String{
					stringvalidator.OneOf("MEMBER", "MAINTAINER"),
				},
			},
			"slug": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The generated slug for the team.",
				PlanModifiers: []planmodifier.String{
					custom_modifier.UseStateIfUnchanged("name"),
				},
			},
			"members_can_create_pipelines": resource_schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Whether members of the team can create Pipelines.",
			},
			"members_can_create_suites": resource_schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Whether members of the team can create test suites.",
			},
			"members_can_create_registries": resource_schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Whether members of the team can create registries.",
			},
			"members_can_destroy_registries": resource_schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Whether members of the team can destroy registries.",
			},
			"members_can_destroy_packages": resource_schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Whether members of the team can destroy packages.",
			},
		},
	}
}

func (t *teamResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if isUUID(req.ID) {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("uuid"), req.ID)...)
	} else {
		uuid, err := uuidFromGraphQLID(req.ID)
		if err != nil {
			resp.Diagnostics.AddError(
				"Invalid import ID",
				fmt.Sprintf("Expected a UUID or GraphQL ID: %s", err.Error()),
			)
			return
		}
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("uuid"), uuid)...)
	}
}

func (t *teamResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var state teamResourceModel

	diags := req.Plan.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := t.client.timeouts.Create(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var result *teamAPIResponse
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var err error
		result, err = t.createTeam(ctx, &state)
		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create team.",
			fmt.Sprintf("Unable to create team: %s", err.Error()),
		)
		return
	}

	// If the response lacks graphql_id, do a GET fallback
	if result.GraphQLID == "" {
		getResult, err := t.getTeam(ctx, result.ID)
		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to read team after create.",
				fmt.Sprintf("Unable to read team after create: %s", err.Error()),
			)
			return
		}
		result = getResult
	}

	updateTeamResourceStateFromREST(&state, result)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (t *teamResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state teamResourceModel

	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := t.client.timeouts.Read(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	uuid, err := teamUUIDFromState(&state)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to determine team UUID.",
			fmt.Sprintf("Unable to determine team UUID: %s", err.Error()),
		)
		return
	}

	var result *teamAPIResponse
	var notFound bool
	err = retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var err error
		result, err = t.getTeam(ctx, uuid)
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
			"Unable to read team.",
			fmt.Sprintf("Unable to read team: %s", err.Error()),
		)
		return
	}

	if notFound {
		resp.Diagnostics.AddWarning("Team not found", "Removing team from state")
		resp.State.RemoveResource(ctx)
		return
	}

	updateTeamResourceStateFromREST(&state, result)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (t *teamResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state, plan teamResourceModel
	diagsState := req.State.Get(ctx, &state)
	diagsPlan := req.Plan.Get(ctx, &plan)

	resp.Diagnostics.Append(diagsPlan...)
	resp.Diagnostics.Append(diagsState...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := t.client.timeouts.Update(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	uuid, err := teamUUIDFromState(&state)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to determine team UUID.",
			fmt.Sprintf("Unable to determine team UUID: %s", err.Error()),
		)
		return
	}

	var result *teamAPIResponse
	err = retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var err error
		result, err = t.updateTeam(ctx, uuid, &plan)
		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to update Team",
			fmt.Sprintf("Unable to update Team: %s", err.Error()),
		)
		return
	}

	// If the response lacks graphql_id, do a GET fallback
	if result.GraphQLID == "" {
		getResult, err := t.getTeam(ctx, result.ID)
		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to read team after update.",
				fmt.Sprintf("Unable to read team after update: %s", err.Error()),
			)
			return
		}
		result = getResult
	}

	updateTeamResourceStateFromREST(&plan, result)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (t *teamResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state teamResourceModel

	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := t.client.timeouts.Delete(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	uuid, err := teamUUIDFromState(&state)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to determine team UUID.",
			fmt.Sprintf("Unable to determine team UUID: %s", err.Error()),
		)
		return
	}

	err = retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		err := t.deleteTeam(ctx, uuid)
		if err != nil && strings.Contains(err.Error(), "404") {
			return nil
		}
		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete Team",
			fmt.Sprintf("Unable to delete Team: %s", err.Error()),
		)
		return
	}
}

func updateTeamResourceState(state *teamResourceModel, res getNodeNodeTeam) {
	state.ID = types.StringValue(res.Id)
	state.UUID = types.StringValue(res.Uuid)
	state.Slug = types.StringValue(res.Slug)
	state.Name = types.StringValue(res.Name)
	state.Privacy = types.StringValue(string(res.GetPrivacy()))
	state.Description = types.StringValue(res.Description)
	state.IsDefaultTeam = types.BoolValue(res.IsDefaultTeam)
	state.DefaultMemberRole = types.StringValue(string(res.GetDefaultMemberRole()))
	state.MembersCanCreatePipelines = types.BoolValue(res.MembersCanCreatePipelines)
	state.MembersCanCreateSuites = types.BoolValue(res.MembersCanCreateSuites)
	state.MembersCanCreateRegistries = types.BoolValue(res.MembersCanCreateRegistries)
	state.MembersCanDestroyRegistries = types.BoolValue(res.MembersCanDestroyRegistries)
	state.MembersCanDestroyPackages = types.BoolValue(res.MembersCanDestroyPackages)
}

func updateTeamResourceStateFromREST(state *teamResourceModel, res *teamAPIResponse) {
	state.ID = types.StringValue(res.GraphQLID)
	state.UUID = types.StringValue(res.ID)
	state.Slug = types.StringValue(res.Slug)
	state.Name = types.StringValue(res.Name)
	state.Description = types.StringValue(res.Description)
	state.Privacy = types.StringValue(strings.ToUpper(res.Privacy))
	state.IsDefaultTeam = types.BoolValue(res.Default)
	state.DefaultMemberRole = types.StringValue(strings.ToUpper(res.DefaultMemberRole))
	state.MembersCanCreatePipelines = types.BoolValue(res.MembersCanCreatePipelines)
	state.MembersCanCreateSuites = types.BoolValue(res.MembersCanCreateSuites)
	state.MembersCanCreateRegistries = types.BoolValue(res.MembersCanCreateRegistries)
	state.MembersCanDestroyRegistries = types.BoolValue(res.MembersCanDestroyRegistries)
	state.MembersCanDestroyPackages = types.BoolValue(res.MembersCanDestroyPackages)
}

// REST API helper methods

func (t *teamResource) createTeam(ctx context.Context, state *teamResourceModel) (*teamAPIResponse, error) {
	apiPath := fmt.Sprintf("/v2/organizations/%s/teams", t.client.organization)
	reqBody := buildTeamRequest(state)
	var result teamAPIResponse
	err := t.client.makeRequest(ctx, http.MethodPost, apiPath, reqBody, &result)
	if err != nil {
		return nil, fmt.Errorf("error creating team: %w", err)
	}
	return &result, nil
}

func (t *teamResource) getTeam(ctx context.Context, uuid string) (*teamAPIResponse, error) {
	apiPath := fmt.Sprintf("/v2/organizations/%s/teams/%s", t.client.organization, uuid)
	var result teamAPIResponse
	err := t.client.makeRequest(ctx, http.MethodGet, apiPath, nil, &result)
	if err != nil {
		return nil, fmt.Errorf("error reading team: %w", err)
	}
	return &result, nil
}

func (t *teamResource) updateTeam(ctx context.Context, uuid string, plan *teamResourceModel) (*teamAPIResponse, error) {
	apiPath := fmt.Sprintf("/v2/organizations/%s/teams/%s", t.client.organization, uuid)
	reqBody := buildTeamRequest(plan)
	var result teamAPIResponse
	err := t.client.makeRequest(ctx, http.MethodPatch, apiPath, reqBody, &result)
	if err != nil {
		return nil, fmt.Errorf("error updating team: %w", err)
	}
	return &result, nil
}

func (t *teamResource) deleteTeam(ctx context.Context, uuid string) error {
	apiPath := fmt.Sprintf("/v2/organizations/%s/teams/%s", t.client.organization, uuid)
	err := t.client.makeRequest(ctx, http.MethodDelete, apiPath, nil, nil)
	if err != nil {
		return fmt.Errorf("error deleting team: %w", err)
	}
	return nil
}

func buildTeamRequest(state *teamResourceModel) teamCreateUpdateRequest {
	return teamCreateUpdateRequest{
		Name:                        state.Name.ValueString(),
		Description:                 state.Description.ValueString(),
		Privacy:                     strings.ToLower(state.Privacy.ValueString()),
		IsDefaultTeam:               state.IsDefaultTeam.ValueBool(),
		DefaultMemberRole:           strings.ToLower(state.DefaultMemberRole.ValueString()),
		MembersCanCreatePipelines:   state.MembersCanCreatePipelines.ValueBool(),
		MembersCanCreateSuites:      state.MembersCanCreateSuites.ValueBool(),
		MembersCanCreateRegistries:  state.MembersCanCreateRegistries.ValueBool(),
		MembersCanDestroyRegistries: state.MembersCanDestroyRegistries.ValueBool(),
		MembersCanDestroyPackages:   state.MembersCanDestroyPackages.ValueBool(),
	}
}

func teamUUIDFromState(state *teamResourceModel) (string, error) {
	if !state.UUID.IsNull() && state.UUID.ValueString() != "" {
		return state.UUID.ValueString(), nil
	}
	if !state.ID.IsNull() && state.ID.ValueString() != "" {
		return uuidFromGraphQLID(state.ID.ValueString())
	}
	return "", fmt.Errorf("team state is missing both uuid and id")
}

func uuidFromGraphQLID(graphqlID string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(graphqlID)
	if err != nil {
		return "", fmt.Errorf("invalid GraphQL ID: %w", err)
	}
	parts := strings.SplitN(string(decoded), "---", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("unexpected GraphQL ID format: %s", string(decoded))
	}
	return parts[1], nil
}
