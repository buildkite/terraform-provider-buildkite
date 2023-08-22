package buildkite

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type testSuiteModel struct {
	ApiToken      types.String `tfsdk:"api_token"`
	DefaultBranch types.String `tfsdk:"default_branch"`
	ID            types.String `tfsdk:"id"`
	UUID          types.String `tfsdk:"uuid"`
	TeamOwnerId   types.String `tfsdk:"team_owner_id"`
	Name          types.String `tfsdk:"name"`
	Slug          types.String `tfsdk:"slug"`
}

type testSuiteResponse struct {
	ApiToken      string `json:"api_token"`
	DefaultBranch string `json:"default_branch"`
	UUID          string `json:"id"`
	GraphqlID     string `json:"graphql_id"`
	Name          string `json:"name"`
	Slug          string `json:"slug"`
}

type testSuiteResource struct {
	client *Client
}

func newTestSuiteResource() resource.Resource {
	return &testSuiteResource{}
}

func (ts *testSuiteResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	ts.client = req.ProviderData.(*Client)
}

func (ts *testSuiteResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, state testSuiteModel
	var response testSuiteResponse
	payload := map[string]interface{}{}
	var teamOwnerUuid string

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The REST API requires team UUIDs but everything else in the provider uses GraphQL IDs. So we map from UUID to ID
	// here
	apiResponse, err := getNode(ctx, ts.client.genqlient, plan.TeamOwnerId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to find team", err.Error())
		return
	}
	if apiTeam, ok := apiResponse.Node.(*getNodeNodeTeam); ok {
		teamOwnerUuid = apiTeam.Uuid
	} else {
		resp.Diagnostics.AddError("Failed to parse team from graphql", err.Error())
		return
	}

	payload["name"] = plan.Name.ValueString()
	payload["default_branch"] = plan.DefaultBranch.ValueString()
	payload["show_api_token"] = true
	payload["team_ids"] = []string{teamOwnerUuid}

	url := fmt.Sprintf("/v2/analytics/organizations/%s/suites", ts.client.organization)
	err = ts.client.makeRequest(ctx, "POST", url, payload, &response)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create test suite", err.Error())
		return
	}

	state.ApiToken = types.StringValue(response.ApiToken)
	state.DefaultBranch = types.StringValue(response.DefaultBranch)
	state.ID = types.StringValue(response.GraphqlID)
	state.UUID = types.StringValue(response.UUID)
	state.Name = types.StringValue(response.Name)
	state.Slug = types.StringValue(response.Slug)
	state.TeamOwnerId = plan.TeamOwnerId

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (ts *testSuiteResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state testSuiteModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	url := fmt.Sprintf("/v2/analytics/organizations/%s/suites/%s", ts.client.organization, state.Slug.ValueString())
	err := ts.client.makeRequest(ctx, "DELETE", url, nil, nil)
	if err != nil {
		resp.Diagnostics.AddError("Failed to delete test suite", err.Error())
		return
	}
}

func (*testSuiteResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_test_suite"
}

func (ts *testSuiteResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state testSuiteModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	graphqlResponse, err := getTestSuite(ctx, ts.client.genqlient, state.ID.ValueString(), 50)
	if err != nil {
		resp.Diagnostics.AddError("Failed to load test suite from GraphQL", err.Error())
		return
	}

	teamToFind := state.TeamOwnerId.ValueString()
	// Find either the team ID from the state (if set) or the first team linked with MANAGE_AND_READ
	if suite, ok := graphqlResponse.Suite.(*getTestSuiteSuite); ok {
		var found *getTestSuiteSuiteTeamsTeamSuiteConnectionEdgesTeamSuiteEdge
		for _, teamSuite := range suite.Teams.Edges {
			if teamSuite.Node.Team.Id == teamToFind {
				found = &teamSuite
				break
			}
			if teamSuite.Node.AccessLevel == SuiteAccessLevelsManageAndRead && found == nil {
				found = &teamSuite
			}
		}
		if found != nil {
			state.TeamOwnerId = types.StringValue(string(found.Node.Team.Id))
		} else {
			// team from state doesnt exist
			// and we didnt find another team with MANAGE_AND_READ
			state.TeamOwnerId = types.StringUnknown()
		}
	}

	if state.TeamOwnerId.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("team_owner_id"), "Could not find owning team", "No team matching")
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (ts *testSuiteResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A test suite is a collection of tests. A run is to a suite what a build is to a Pipeline.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"uuid": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"team_owner_id": schema.StringAttribute{
				Required: true,
			},
			"slug": schema.StringAttribute{
				Computed: true,
			},
			"name": schema.StringAttribute{
				Required: true,
			},
			"api_token": schema.StringAttribute{
				Computed:  true,
				Sensitive: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"default_branch": schema.StringAttribute{
				Required: true,
			},
		},
	}
}

func (ts *testSuiteResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state testSuiteModel
	var response testSuiteResponse
	payload := map[string]interface{}{}

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload["name"] = plan.Name.ValueString()
	payload["default_branch"] = plan.DefaultBranch.ValueString()

	url := fmt.Sprintf("/v2/analytics/organizations/%s/suites/%s", ts.client.organization, state.Slug.ValueString())
	err := ts.client.makeRequest(ctx, "PATCH", url, payload, &response)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create test suite", err.Error())
		return
	}

	state.Name = plan.Name
	state.DefaultBranch = plan.DefaultBranch
	state.Slug = types.StringValue(response.Slug)

	// If the planned team_owner_id differs from the state, add the new one and remove the old one
	if plan.TeamOwnerId.ValueString() != state.TeamOwnerId.ValueString() {
		graphqlResponse, err := createTestSuiteTeam(ctx, ts.client.genqlient, plan.TeamOwnerId.ValueString(), state.ID.ValueString(), SuiteAccessLevelsManageAndRead)
		if err != nil {
			resp.Diagnostics.AddError("Could not add new owner team", err.Error())
			return
		}
		previousOwnerId := state.TeamOwnerId.ValueString()
		state.TeamOwnerId = types.StringValue(graphqlResponse.TeamSuiteCreate.TeamSuite.Team.Id)
		for _, team := range graphqlResponse.TeamSuiteCreate.Suite.Teams.Edges {
			if team.Node.Team.Id == previousOwnerId {
				_, err = deleteTestSuiteTeam(ctx, ts.client.genqlient, team.Node.Id)
				if err != nil {
					resp.Diagnostics.AddError("Failed to delete team owner", err.Error())
					return
				}
			}
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
