package buildkite

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
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
	Name          types.String `tfsdk:"name"`
	Slug          types.String `tfsdk:"slug"`
	WebUrl        types.String `tfsdk:"web_url"`
	Team          []teamBlock  `tfsdk:"team"`
}

type testSuiteV6Model struct {
	ApiToken      types.String `tfsdk:"api_token"`
	DefaultBranch types.String `tfsdk:"default_branch"`
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	Slug          types.String `tfsdk:"slug"`
	WebUrl        types.String `tfsdk:"web_url"`
	Team          []teamBlock  `tfsdk:"team"`
	Teams         []teamBlock  `tfsdk:"teams"`
}

type testSuiteResponse struct {
	ApiToken      string `json:"api_token"`
	DefaultBranch string `json:"default_branch"`
	ID            string `json:"id"`
	Name          string `json:"name"`
	Slug          string `json:"slug"`
	WebUrl        string `json:"web_url"`
	TeamUuids     string `json:"team_ids"`
}

type teamBlock struct {
	ID          types.String `tfsdk:"id"`
	AccessLevel types.String `tfsdk:"access_level"`
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
	payload := map[string]interface{}{}

	if ts.isProtoV6 {
		resp.Diagnostics.Append(req.Plan.Get(ctx, &v6Plan)...)
	} else {
		resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	// The REST API requires team UUIDs but everything else in the provider uses GraphQL IDs. So we map from UUID to ID
	// here
	allTeams := append(plan.Team, v6Plan.Teams...)
	teamUuids := make([]string, len(allTeams))
	for i, team := range allTeams {
		apiResponse, err := getNode(ts.client.genqlient, team.ID.ValueString())
		if err != nil {
		}
		if team, ok := apiResponse.Node.(*getNodeNodeTeam); ok {
			teamUuids[i] = team.Uuid
		}
	}

	payload["name"] = plan.Name.ValueString()
	payload["default_branch"] = plan.DefaultBranch.ValueString()
	payload["show_api_token"] = true
	payload["team_ids"] = teamUuids

	url := fmt.Sprintf("/v2/analytics/organizations/%s/suites", ts.client.organization)
	err := ts.client.makeRequest("POST", url, payload, &response)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create test suite", err.Error())
	}

	// REST only allows adding teams to a suite with MANAGE_AND_READ so we have to manage the others manually
	apiResponse, err := findTeamSuite(ts.client.genqlient, ts.client.organization, plan.Name.ValueString(), len(allTeams))
	teams := make(map[string]string)
	for _, suite := range apiResponse.Organization.Suites.Edges {
		for _, teamSuite := range suite.Node.Teams.Edges {
			teams[teamSuite.Node.Team.Id] = teamSuite.Node.Id
		}
	}
	for _, team := range allTeams {
		// if this team is already MANAGE_AND_READ then skip it
		if team.AccessLevel.ValueString() == string(SuiteAccessLevelsManageAndRead) {
			continue
		}
		_, err := updateTestSuiteTeam(ts.client.genqlient, teams[team.ID.ValueString()], SuiteAccessLevels(team.AccessLevel.ValueString()))
		if err != nil {
			resp.Diagnostics.AddError("Failed to link team to test suite", err.Error())
			return
		}
	}

	state.ApiToken = types.StringValue(response.ApiToken)
	state.DefaultBranch = types.StringValue(response.DefaultBranch)
	state.ID = types.StringValue(response.ID)
	state.Name = types.StringValue(response.Name)
	state.Slug = types.StringValue(response.Slug)
	state.WebUrl = types.StringValue(response.WebUrl)
	state.Team = plan.Team
	if ts.isProtoV6 {
		v6State.ApiToken = types.StringValue(response.ApiToken)
		v6State.DefaultBranch = types.StringValue(response.DefaultBranch)
		v6State.ID = types.StringValue(response.ID)
		v6State.Name = types.StringValue(response.Name)
		v6State.Slug = types.StringValue(response.Slug)
		v6State.WebUrl = types.StringValue(response.WebUrl)
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

func (ts *testSuiteResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state testSuiteModel
	var response testSuiteResponse

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	url := fmt.Sprintf("/v2/analytics/organizations/%s/suites/%s", ts.client.organization, state.Slug.ValueString())
	err := ts.client.makeRequest("GET", url, nil, &response)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read test suite", err.Error())
	}

	state.DefaultBranch = types.StringValue(response.DefaultBranch)
	state.WebUrl = types.StringValue(response.WebUrl)
	state.Name = types.StringValue(response.Name)

	graphqlResponse, err := findTeamSuite(ts.client.genqlient, ts.client.organization, response.Name, 50)
	var teams []teamBlock
	for _, suite := range graphqlResponse.Organization.Suites.Edges {
		for _, teamSuite := range suite.Node.Teams.Edges {
			teams = append(teams, teamBlock{
				ID:          types.StringValue(teamSuite.Node.Team.Id),
				AccessLevel: types.StringValue(string(teamSuite.Node.AccessLevel)),
			})
		}
	}
	state.Team = teams

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (ts *testSuiteResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := map[string]schema.Attribute{
		"id": schema.StringAttribute{
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
		"web_url": schema.StringAttribute{
			Computed: true,
		},
	}

	if ts.isProtoV6 {
		attributes["teams"] = schema.ListNestedAttribute{
			Optional: true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"id": schema.StringAttribute{
						MarkdownDescription: "The GraphQL ID of the team to give access.",
						Required:            true,
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
			"team": schema.ListNestedBlock{
				Validators: []validator.List{
					listvalidator.SizeAtLeast(1),
				},
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "The GraphQL ID of the team to give access.",
							Required:            true,
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
