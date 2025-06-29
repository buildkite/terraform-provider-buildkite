package buildkite

import (
	"context"
	"fmt"

	custom_modifier "github.com/buildkite/terraform-provider-buildkite/internal/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

type testSuiteModel struct {
	ApiToken      types.String `tfsdk:"api_token"`
	DefaultBranch types.String `tfsdk:"default_branch"`
	Emoji         types.String `tfsdk:"emoji"`
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

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Use the Read timeout for obtaining a Test suite's UUID
	timeout, diags := ts.client.timeouts.Read(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	// The REST API requires team UUIDs but everything else in the provider uses GraphQL IDs. So we map from UUID to ID
	// here
	var r *getNodeResponse
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var err error
		r, err = getNode(ctx,
			ts.client.genqlient,
			plan.TeamOwnerId.ValueString(),
		)

		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to find owning team",
			fmt.Sprintf("Failed to find owning team: %s", err.Error()),
		)
		return
	}

	if apiTeam, ok := r.Node.(*getNodeNodeTeam); ok {
		teamOwnerUuid = apiTeam.Uuid
	} else {
		resp.Diagnostics.AddError("Failed to parse team from graphql", err.Error())
		return
	}

	payload["name"] = plan.Name.ValueString()
	payload["default_branch"] = plan.DefaultBranch.ValueString()
	payload["emoji"] = plan.Emoji.ValueString()
	payload["show_api_token"] = true
	payload["team_ids"] = []string{teamOwnerUuid}

	// Construct URL to call to the REST API
	url := fmt.Sprintf("/v2/analytics/organizations/%s/suites", ts.client.organization)

	// Use the Create timeout for test suite creation
	timeout, diags = ts.client.timeouts.Create(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	createErr := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		err = ts.client.makeRequest(ctx, "POST", url, payload, &response)

		return retryContextError(err)
	})

	if createErr != nil {
		resp.Diagnostics.AddError(
			"Failed to create test suite",
			fmt.Sprintf("Failed to create test suite: %s", createErr.Error()),
		)
		return
	}

	state.ApiToken = types.StringValue(response.ApiToken)
	state.DefaultBranch = types.StringValue(response.DefaultBranch)
	state.Emoji = plan.Emoji
	state.ID = types.StringValue(response.GraphqlID)
	state.UUID = types.StringValue(response.UUID)
	state.Name = types.StringValue(response.Name)
	state.Slug = types.StringValue(response.Slug)
	state.TeamOwnerId = plan.TeamOwnerId

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (ts *testSuiteResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state testSuiteModel

	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := ts.client.timeouts.Delete(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	// Construct URL to call to the REST API
	url := fmt.Sprintf("/v2/analytics/organizations/%s/suites/%s", ts.client.organization, state.Slug.ValueString())
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		err := ts.client.makeRequest(ctx, "DELETE", url, nil, nil)

		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to delete test suite",
			fmt.Sprintf("Failed to delete test suite: %s", err.Error()),
		)
		return
	}
}

func (*testSuiteResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_test_suite"
}

func (ts *testSuiteResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state testSuiteModel

	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := ts.client.timeouts.Read(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var r *getTestSuiteResponse
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var err error
		r, err = getTestSuite(ctx,
			ts.client.genqlient, state.ID.ValueString(),
			50,
		)

		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to load test suite from GraphQL",
			fmt.Sprintf("Failed to load test suite from GraphQL: %s", err.Error()),
		)
		return
	}

	teamToFind := state.TeamOwnerId.ValueString()
	// Find either the team ID from the state (if set) or the first team linked with MANAGE_AND_READ
	if suite, ok := r.Suite.(*getTestSuiteSuite); ok {
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

		if state.TeamOwnerId.IsUnknown() {
			resp.Diagnostics.AddAttributeError(path.Root("team_owner_id"), "Could not find owning team", "No team matching")
			return
		}

		setTestSuiteModel(&state, suite)

	} else {
		// Test suite was removed - remove from state
		resp.Diagnostics.AddWarning("Test suite not found", "Removing test suite from state")
		resp.State.RemoveResource(ctx)
	}

	// API Token only available from REST API
	var response testSuiteResponse

	// Construct URL to call to the REST API to get the API Token
	url := fmt.Sprintf("/v2/analytics/organizations/%s/suites/%s?show_api_token=true", ts.client.organization, state.Slug.ValueString())
	err = retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		err := ts.client.makeRequest(ctx, "GET", url, nil, &response)
		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to read test suite API token",
			fmt.Sprintf("Failed to read test suite API token: %s", err.Error()),
		)
		return
	}

	// Update API Token in State if it has changed or if it is null from importing into State
	if response.ApiToken != state.ApiToken.ValueString() || state.ApiToken.IsNull() {
		// The API Token can be regenerated, but 'terraform refresh' or 'terraform apply' is required to update State
		// don't need a warning if it is null from importing into State
		if !state.ApiToken.IsNull() {
			resp.Diagnostics.AddAttributeWarning(path.Root("api_token"), "Test Suite API Token has changed", "Test Suite API Token has changed, \"run terraform refresh\" or \"terraform apply\" to update State")
		}
		state.ApiToken = types.StringValue(response.ApiToken)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (ts *testSuiteResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A test suite is a collection of tests. A run is to a suite what a build is to a Pipeline." +
			"Use this resource to manage [Test Suites](https://buildkite.com/docs/test-analytics) on Buildkite.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The GraphQL ID of the test suite.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"uuid": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the test suite.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"team_owner_id": schema.StringAttribute{
				MarkdownDescription: "The GraphQL ID of the team to mark as the owner/admin of the test suite.",
				Required:            true,
			},
			"slug": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The generated slug of the test suite.",
				PlanModifiers: []planmodifier.String{
					custom_modifier.UseStateIfUnchanged("name"),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name to give the test suite.",
			},
			"api_token": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "The API token to use to send test run data to the API.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"default_branch": schema.StringAttribute{
				MarkdownDescription: "The default branch for the repository this test suite is for.",
				Required:            true,
			},
			"emoji": schema.StringAttribute{
				MarkdownDescription: "The emoji associated with this test suite, eg :buildkite:",
				Optional:            true,
			},
		},
	}
}

func (ts *testSuiteResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state testSuiteModel
	var response testSuiteResponse
	payload := map[string]interface{}{}

	diagsPlan := req.Plan.Get(ctx, &plan)
	diagsState := req.State.Get(ctx, &state)

	resp.Diagnostics.Append(diagsPlan...)
	resp.Diagnostics.Append(diagsState...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := ts.client.timeouts.Update(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	payload["name"] = plan.Name.ValueString()
	payload["default_branch"] = plan.DefaultBranch.ValueString()
	payload["emoji"] = plan.Emoji.ValueString()

	// Construct URL to call to the REST API
	url := fmt.Sprintf("/v2/analytics/organizations/%s/suites/%s", ts.client.organization, state.Slug.ValueString())
	updateErr := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		err := ts.client.makeRequest(ctx, "PATCH", url, payload, &response)

		return retryContextError(err)
	})

	if updateErr != nil {
		resp.Diagnostics.AddError(
			"Failed to update test suite",
			fmt.Sprintf("Failed to update test suite: %s", updateErr.Error()),
		)
		return
	}

	state.Name = plan.Name
	state.DefaultBranch = plan.DefaultBranch
	state.Emoji = plan.Emoji
	state.Slug = types.StringValue(response.Slug)

	// If the planned team_owner_id differs from the state, add the new one and remove the old one
	if plan.TeamOwnerId.ValueString() != state.TeamOwnerId.ValueString() {
		var r *createTestSuiteTeamResponse
		err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
			var err error
			r, err = createTestSuiteTeam(ctx,
				ts.client.genqlient,
				plan.TeamOwnerId.ValueString(),
				state.ID.ValueString(),
				SuiteAccessLevelsManageAndRead,
			)

			return retryContextError(err)
		})
		if err != nil {
			resp.Diagnostics.AddError(
				"Could not add new owner team",
				fmt.Sprintf("Could not add new owner team: %s", err.Error()),
			)
			return
		}
		previousOwnerId := state.TeamOwnerId.ValueString()
		state.TeamOwnerId = types.StringValue(r.TeamSuiteCreate.TeamSuite.Team.Id)
		for _, team := range r.TeamSuiteCreate.Suite.Teams.Edges {
			if team.Node.Team.Id == previousOwnerId {
				err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
					_, err := deleteTestSuiteTeam(ctx,
						ts.client.genqlient,
						team.Node.Id,
					)

					return retryContextError(err)
				})
				if err != nil {
					resp.Diagnostics.AddError(
						"Failed to delete team owner",
						fmt.Sprintf("Failed to delete team owner: %s", err.Error()),
					)
					return
				}
			}
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (ts *testSuiteResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func setTestSuiteModel(testSuiteModel *testSuiteModel, suite *getTestSuiteSuite) {
	testSuiteModel.Name = types.StringValue(suite.Name)
	testSuiteModel.Slug = types.StringValue(suite.Slug)
	testSuiteModel.UUID = types.StringValue(suite.Uuid)
	testSuiteModel.DefaultBranch = types.StringValue(suite.DefaultBranch)
	testSuiteModel.Emoji = types.StringPointerValue(suite.Emoji)
}
