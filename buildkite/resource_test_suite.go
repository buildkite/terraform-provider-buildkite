package buildkite

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type testSuiteModel struct {
	ApiToken      types.String `tfsdk:"api_token"`
	DefaultBranch types.String `tfsdk:"default_branch"`
	ID            types.String `tfsdk:"id"`
	UUID          types.String `tfsdk:"uuid"`
	Name          types.String `tfsdk:"name"`
	Slug          types.String `tfsdk:"slug"`
	Team          []teamBlock  `tfsdk:"team"`
}

type testSuiteV6Model struct {
	ApiToken      types.String `tfsdk:"api_token"`
	DefaultBranch types.String `tfsdk:"default_branch"`
	ID            types.String `tfsdk:"id"`
	UUID          types.String `tfsdk:"uuid"`
	Name          types.String `tfsdk:"name"`
	Slug          types.String `tfsdk:"slug"`
	Team          []teamBlock  `tfsdk:"team"`
	Teams         []teamBlock  `tfsdk:"teams"`
}

type testSuiteResponse struct {
	ApiToken      string `json:"api_token"`
	DefaultBranch string `json:"default_branch"`
	UUID          string `json:"id"`
	GraphqlID     string `json:"graphql_id"`
	Name          string `json:"name"`
	Slug          string `json:"slug"`
	TeamUuids     string `json:"team_ids"`
}

type teamBlock struct {
	AccessLevel types.String `tfsdk:"access_level"`
	ID          types.String `tfsdk:"id"`
	TeamSuiteID types.String `tfsdk:"team_suite_id"`
}

type teamMap struct {
	id, uuid string
}

type testSuiteResource struct {
	client    *Client
	isProtoV6 bool
}

func newTestSuiteResource(isProtoV6 bool) func() resource.Resource {
	return func () resource.Resource{
		return &testSuiteResource{
			isProtoV6: isProtoV6,
		}
	}
}

func (ts *testSuiteResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	ts.client = req.ProviderData.(*Client)
}

func (ts *testSuiteResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, state testSuiteModel
	var v6Plan, v6State testSuiteV6Model
	var response testSuiteResponse
	var allTeams []teamBlock
	var manageTeams, readTeams []teamMap
	payload := map[string]interface{}{}

	if ts.isProtoV6 {
		resp.Diagnostics.Append(req.Plan.Get(ctx, &v6Plan)...)
		if len(v6Plan.Teams) > 0 {
			allTeams = v6Plan.Teams
		} else if len(v6Plan.Team) > 0 {
			allTeams = v6Plan.Team
		}
	} else {
		resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
		allTeams = plan.Team
	}
	if resp.Diagnostics.HasError() {
		return
	}

	// The REST API requires team UUIDs but everything else in the provider uses GraphQL IDs. So we map from UUID to ID
	// here
	manageTeamUuids := make([]string, len(allTeams))
	for i, team := range allTeams {
		apiResponse, err := getNode(ts.client.genqlient, team.ID.ValueString())
		if err != nil {
			// TODO
		}
		if apiTeam, ok := apiResponse.Node.(*getNodeNodeTeam); ok {
			t := teamMap{id: apiTeam.Id, uuid: apiTeam.Uuid}
			if team.AccessLevel.ValueString() == string(SuiteAccessLevelsManageAndRead) {
				manageTeams = append(manageTeams, t)
				manageTeamUuids[i] = apiTeam.Uuid
			} else if team.AccessLevel.ValueString() == string(SuiteAccessLevelsReadOnly) {
				readTeams = append(readTeams, t)
			} else {
				// TODO: error? unknown permission level
			}
		}
	}

	payload["name"] = plan.Name.ValueString()
	payload["default_branch"] = plan.DefaultBranch.ValueString()
	payload["show_api_token"] = true
	payload["team_ids"] = manageTeamUuids

	url := fmt.Sprintf("/v2/analytics/organizations/%s/suites", ts.client.organization)
	err := ts.client.makeRequest("POST", url, payload, &response)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create test suite", err.Error())
	}

	// REST only allows adding teams to a suite with MANAGE_AND_READ so we have to manage the others manually
	graphqlResponse, err := listTeamSuites(ts.client.genqlient, ts.client.organization, plan.Name.ValueString(), 50)
	var suiteId string
	if err != nil {
		resp.Diagnostics.AddError("Failed to link team to test suite", err.Error())
	}
	for _, resp := range graphqlResponse.Organization.Suites.Edges {
		if resp.Node.Uuid == response.UUID {
			suiteId = resp.Node.Id
			break
		}
	}
	if suiteId == "" {
		resp.Diagnostics.AddError("Could not find the created suite in Graphql", "This may have left a dangling suite. Please check the UI")
		return
	}
	// Now we can add the READ_ONLY teams to the suite
	for _, readTeam := range readTeams {
		_, err := createTestSuiteTeam(ts.client.genqlient, readTeam.id, suiteId, SuiteAccessLevelsReadOnly)
		if err != nil {
			resp.Diagnostics.AddError("Failed to link team to test suite", err.Error())
		}
	}

	state.ApiToken = types.StringValue(response.ApiToken)
	state.DefaultBranch = types.StringValue(response.DefaultBranch)
	state.ID = types.StringValue(response.UUID)
	state.UUID = types.StringValue(suiteId)
	state.Name = types.StringValue(response.Name)
	state.Slug = types.StringValue(response.Slug)
	state.Team = plan.Team
	if ts.isProtoV6 {
		v6State.ApiToken = types.StringValue(response.ApiToken)
		v6State.DefaultBranch = types.StringValue(response.DefaultBranch)
		v6State.ID = types.StringValue(response.UUID)
		v6State.UUID = types.StringValue(suiteId)
		v6State.Name = types.StringValue(response.Name)
		v6State.Slug = types.StringValue(response.Slug)
		v6State.Team = plan.Team
		v6State.Teams = v6Plan.Teams
		resp.Diagnostics.Append(resp.State.Set(ctx, v6State)...)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (ts *testSuiteResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state testSuiteModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	url := fmt.Sprintf("/v2/analytics/organizations/%s/suites/%s", ts.client.organization, state.Slug.ValueString())
	err := ts.client.makeRequest("DELETE", url, nil, nil)
	if err != nil {
		resp.Diagnostics.AddError("Failed to delete test suite", err.Error())
	}
}

func (*testSuiteResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_test_suite"
}

func findTeamSuite(uuid string, suites []listTeamSuitesOrganizationSuitesSuiteConnectionEdgesSuiteEdge) *listTeamSuitesOrganizationSuitesSuiteConnectionEdgesSuiteEdgeNodeSuite {
	for _, suite := range suites {
		if suite.Node.Uuid == uuid {
			return &suite.Node
		}
	}

	return nil
}

func (ts *testSuiteResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state testSuiteModel
	var v6State testSuiteV6Model
	var usingTeams bool

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if ts.isProtoV6 {
		resp.Diagnostics.Append(req.State.Get(ctx, &v6State)...)
	}
	if resp.Diagnostics.HasError() {
		return
	}
	if len(v6State.Teams) > 0 {
		usingTeams = true
	}

	graphqlResponse, err := getTestSuite(ts.client.genqlient, state.ID.ValueString(), 50)
	if err != nil {
		return
	}

	var teams []teamBlock
	if suite, ok := graphqlResponse.Suite.(*getTestSuiteSuite); ok {
		for _, teamSuite := range suite.Teams.Edges {
			teams = append(teams, teamBlock{
				AccessLevel: types.StringValue(string(teamSuite.Node.AccessLevel)),
				ID:          types.StringValue(teamSuite.Node.Team.Id),
				TeamSuiteID: types.StringValue(teamSuite.Node.Id),
			})
		}
	}

	if usingTeams {
		v6State.Teams = teams
	} else if ts.isProtoV6 {
		v6State.Team = teams
	}
	if ts.isProtoV6 {
		resp.Diagnostics.Append(resp.State.Set(ctx, v6State)...)
	} else {
		state.Team = teams
		resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
	}

}

func (ts *testSuiteResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	var deprecationMessage string
	attributes := map[string]schema.Attribute{
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
	}

	if ts.isProtoV6 {
		deprecationMessage = ``
		attributes["teams"] = schema.SetNestedAttribute{
			Optional: true,
			Validators: []validator.Set{
				setvalidator.ExactlyOneOf(path.Expressions{
					path.MatchRoot("team"),
					path.MatchRoot("teams"),
				}...),
				setvalidator.SizeAtLeast(1),
				// TODO: need to validate that at least one entry is marked as MANAGE_AND READ
			},
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"id": schema.StringAttribute{
						MarkdownDescription: "The GraphQL ID of the team to give access.",
						Required:            true,
					},
					"team_suite_id": schema.StringAttribute{
						Computed:            true,
					},
					"access_level": schema.StringAttribute{
						MarkdownDescription: "The access level of the team.",
						Required:            true,
						Validators: []validator.String{
							stringvalidator.OneOf(string(SuiteAccessLevelsReadOnly), string(SuiteAccessLevelsManageAndRead)),
						},
					},
				},
			},
		}
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: "A test suite is a collection of tests. A run is to a suite what a build is to a Pipeline.",
		Blocks: map[string]schema.Block{
			"team": schema.SetNestedBlock{
				DeprecationMessage: deprecationMessage,
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
				},
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "The GraphQL ID of the team to give access.",
							Required:            true,
						},
						"team_suite_id": schema.StringAttribute{
							Computed:            true,
						},
						"access_level": schema.StringAttribute{
							MarkdownDescription: "The access level of the team.",
							Required:            true,
							Validators: []validator.String{
								stringvalidator.OneOf(string(SuiteAccessLevelsReadOnly), string(SuiteAccessLevelsManageAndRead)),
							},
						},
					},
				},
			},
		},
		Attributes: attributes,
	}
}

func (ts *testSuiteResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state testSuiteModel
	payload := map[string]interface{}{}

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload["name"] = plan.Name.ValueString()
	payload["default_branch"] = plan.DefaultBranch.ValueString()

	url := fmt.Sprintf("/v2/analytics/organizations/%s/suites/%s", ts.client.organization, plan.Slug.ValueString())
	err := ts.client.makeRequest("PATCH", url, payload, &state)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create test suite", err.Error())
	}

	state.Name = plan.Name
	state.DefaultBranch = plan.DefaultBranch

	// TODO: update teams. involves adding, removing, updating

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}
