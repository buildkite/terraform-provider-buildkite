package buildkite

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"time"

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
	ApiToken        types.String `tfsdk:"api_token"`
	ApplicationName types.String `tfsdk:"application_name"`
	Color           types.String `tfsdk:"color"`
	DefaultBranch   types.String `tfsdk:"default_branch"`
	Emoji           types.String `tfsdk:"emoji"`
	ID              types.String `tfsdk:"id"`
	UUID            types.String `tfsdk:"uuid"`
	OidcPolicy      types.String `tfsdk:"oidc_policy"`
	TeamOwnerId     types.String `tfsdk:"team_owner_id"`
	Name            types.String `tfsdk:"name"`
	Slug            types.String `tfsdk:"slug"`
}

type testSuiteResponse struct {
	ApiToken        string  `json:"api_token"`
	ApplicationName *string `json:"application_name"`
	Color           *string `json:"color"`
	DefaultBranch   string  `json:"default_branch"`
	Emoji           *string `json:"emoji"`
	UUID            string  `json:"id"`
	GraphqlID       string  `json:"graphql_id"`
	Name            string  `json:"name"`
	OidcPolicy      *string `json:"oidc_policy"`
	Slug            string  `json:"slug"`
}

type testSuiteResource struct {
	client *Client
}

// optionalStringPayload returns the REST payload value for an
// optional+computed string attribute: nil (JSON null) when the value is null
// or unknown, which the API treats as "clear" on update and "unset" on create.
func optionalStringPayload(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	return v.ValueStringPointer()
}

// stateStringValue resolves the post-apply state value for an
// optional+computed string attribute: the planned value when known (Terraform
// requires the applied state to match a known plan), otherwise the value
// returned by the API.
func stateStringValue(planned types.String, fromAPI *string) types.String {
	if planned.IsUnknown() {
		return types.StringPointerValue(fromAPI)
	}
	return planned
}

// refreshedStringValue returns the state value for an optional+computed string
// attribute refreshed from the API. An explicit empty string in state is
// preserved when the API reports the attribute as null: the server coerces
// blank values (eg color) to null, and "" is the documented way to clear these
// attributes from configuration.
func refreshedStringValue(current types.String, fromAPI *string) types.String {
	if fromAPI == nil && !current.IsNull() && current.ValueString() == "" {
		return current
	}
	return types.StringPointerValue(fromAPI)
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
	payload["emoji"] = plan.Emoji.ValueStringPointer()
	payload["application_name"] = optionalStringPayload(plan.ApplicationName)
	payload["color"] = optionalStringPayload(plan.Color)
	payload["oidc_policy"] = optionalStringPayload(plan.OidcPolicy)
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
	state.ApplicationName = stateStringValue(plan.ApplicationName, response.ApplicationName)
	state.Color = stateStringValue(plan.Color, response.Color)
	state.DefaultBranch = types.StringValue(response.DefaultBranch)
	state.Emoji = plan.Emoji
	state.ID = types.StringValue(response.GraphqlID)
	state.UUID = types.StringValue(response.UUID)
	state.Name = types.StringValue(response.Name)
	state.OidcPolicy = stateStringValue(plan.OidcPolicy, response.OidcPolicy)
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
			teamPageSize, nil,
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
		return
	}

	// API Token and OIDC policy only available from REST API
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

	state.ApplicationName = refreshedStringValue(state.ApplicationName, response.ApplicationName)
	state.Color = refreshedStringValue(state.Color, response.Color)
	state.OidcPolicy = refreshedStringValue(state.OidcPolicy, response.OidcPolicy)

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
			"application_name": schema.StringAttribute{
				MarkdownDescription: "The name of the application this test suite is for. " +
					"If omitted, the value is left unmanaged by Terraform; set it to an empty string to clear it.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"color": schema.StringAttribute{
				MarkdownDescription: "The hex color code for the test suite navatar, eg #BADA55. " +
					"If omitted, the value is left unmanaged by Terraform; set it to an empty string to clear it.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"oidc_policy": schema.StringAttribute{
				MarkdownDescription: "The [OIDC policy](https://buildkite.com/docs/pipelines/configure/tests/test-collection/oidc) for the test suite, as a YAML or JSON string. " +
					"This policy defines which OIDC tokens can be exchanged for suite access, as an alternative to the suite API token. " +
					"If omitted, the policy is left unmanaged by Terraform; set it to an empty string to remove an existing policy.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
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
	payload["emoji"] = plan.Emoji.ValueStringPointer()
	payload["application_name"] = optionalStringPayload(plan.ApplicationName)
	payload["color"] = optionalStringPayload(plan.Color)
	payload["oidc_policy"] = optionalStringPayload(plan.OidcPolicy)

	// Construct URL to call to the REST API
	url := fmt.Sprintf("/v2/analytics/organizations/%s/suites/%s", ts.client.organization, state.Slug.ValueString())
	updateErr := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		err := ts.client.makeRequest(ctx, http.MethodPatch, url, payload, &response)

		return retryContextError(err)
	})

	if updateErr != nil {
		resp.Diagnostics.AddError(
			"Failed to update test suite",
			fmt.Sprintf("Failed to update test suite: %s", updateErr.Error()),
		)
		return
	}

	// The PATCH above has applied, so every path out from here has to record it. Returning without
	// setting state would drop everything it changed, and leave state naming the slug the suite
	// answered to before the rename, which is the slug the next update PATCHes: under
	// -refresh=false, where nothing re-reads the suite by ID, that request goes to a URL the suite
	// no longer has. Deferred so a new early return cannot forget it.
	defer func() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	}()

	state.Name = plan.Name
	state.DefaultBranch = plan.DefaultBranch
	state.Emoji = plan.Emoji
	state.ApplicationName = stateStringValue(plan.ApplicationName, response.ApplicationName)
	state.Color = stateStringValue(plan.Color, response.Color)
	state.OidcPolicy = stateStringValue(plan.OidcPolicy, response.OidcPolicy)
	state.Slug = types.StringValue(response.Slug)

	// If the planned team_owner_id differs from the state, add the new one and remove the old one
	if plan.TeamOwnerId.ValueString() != state.TeamOwnerId.ValueString() {
		var attachErr error
		alreadyOwned := false
		err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
			_, err := createTestSuiteTeam(ctx,
				ts.client.genqlient,
				plan.TeamOwnerId.ValueString(),
				state.ID.ValueString(),
				SuiteAccessLevelsManageAndRead,
			)

			// An owner that already owns the suite is what re-running this method looks like: the
			// previous apply attached it and then failed to detach the old one, which is the state
			// this method records so the next plan retries. Failing here would never reach that
			// detach, leaving both teams owning the suite with no way forward.
			//
			// resource_pipeline_team.go reads the same predicate as eventual-consistency noise and
			// retries it. Here the attach is the operation being re-run, so it is taken at its word
			// and confirmed against the suite below, with attachErr kept to report if it was wrong.
			if err != nil && isAlreadyExistsError(err) {
				alreadyOwned, attachErr = true, err
				return nil
			}

			return retryContextError(err)
		})
		if err != nil {
			resp.Diagnostics.AddError(
				"Could not add new owner team",
				fmt.Sprintf("Could not add new owner team: %s", err.Error()),
			)
			return
		}

		// Which edge belongs to the previous owner. Read rather than taken from the attach
		// response: a mutation's nested connection cannot be paged, so a previous owner sorting
		// past the first page would go unfound and keep owning the suite with state saying it does
		// not, and no diff left to correct it.
		teams, err := ts.suiteTeams(ctx, timeout, state.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError(
				"Could not load the suite's owner teams",
				fmt.Sprintf("Could not load the suite's owner teams: %s", err.Error()),
			)
			return
		}

		// A tolerated attach ran no mutation, so nothing yet proves the new owner can manage the
		// suite. Detaching on a message that meant something else leaves the suite with an owner
		// that cannot manage it, or none at all, and Read reports neither.
		if alreadyOwned {
			if err := ts.ensureOwnerTeamCanManage(ctx, timeout, teams, plan.TeamOwnerId.ValueString(), attachErr); err != nil {
				resp.Diagnostics.AddError(
					"Could not add new owner team",
					fmt.Sprintf("Could not add new owner team: %s", err.Error()),
				)
				return
			}
		}

		previousOwnerId := state.TeamOwnerId.ValueString()
		for _, team := range teams {
			if team.teamId == previousOwnerId {
				err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
					_, err := deleteTestSuiteTeam(ctx,
						ts.client.genqlient,
						team.id,
					)

					return retryContextError(err)
				})
				if err != nil {
					resp.Diagnostics.AddError(
						"Failed to delete team owner",
						fmt.Sprintf("Failed to remove team %s from the suite, which team %s now also owns: %s",
							previousOwnerId, plan.TeamOwnerId.ValueString(), err.Error()),
					)
					return
				}
			}
		}

		// Only once the previous owner is detached, because both own the suite until then. Recording
		// the new one first leaves the next plan comparing the config against it, finding no diff,
		// and never retrying the delete.
		state.TeamOwnerId = plan.TeamOwnerId
	}
}

// testSuiteTeamEdge is one team-suite link: the id the delete mutation takes, the team it belongs
// to, and the access that team has. The attach response and the suite query spell the same trio
// differently.
type testSuiteTeamEdge struct {
	id          string
	teamId      string
	accessLevel SuiteAccessLevels
}

// ensureOwnerTeamCanManage confirms teams really does link owner to the suite, and raises that link
// to MANAGE_AND_READ if it sits lower. attachErr is the tolerated already-exists error, reported
// when the link turns out not to be there at all: the message was then about something other than
// the previous run's attach, and the caller must not detach anything on the strength of it.
func (ts *testSuiteResource) ensureOwnerTeamCanManage(ctx context.Context, timeout time.Duration, teams []testSuiteTeamEdge, owner string, attachErr error) error {
	i := slices.IndexFunc(teams, func(team testSuiteTeamEdge) bool { return team.teamId == owner })
	if i < 0 {
		return fmt.Errorf("%w (team %s does not own suite)", attachErr, owner)
	}
	if teams[i].accessLevel == SuiteAccessLevelsManageAndRead {
		return nil
	}

	return retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		_, err := updateTestSuiteTeam(ctx, ts.client.genqlient, teams[i].id, SuiteAccessLevelsManageAndRead)

		return retryContextError(err)
	})
}

// teamPageSize is how many of a suite's teams one request asks for. Suites shared this widely are
// rare, so the paging in suiteTeams almost always completes in a single round trip.
const teamPageSize = 50

// suiteTeams reads the teams linked to a suite, paging to the end of the connection. Stopping at
// the first page would hide a previous owner whose team name sorts past it, and the caller reads
// absence as "already detached" and records the swap as done.
func (ts *testSuiteResource) suiteTeams(ctx context.Context, timeout time.Duration, suiteId string) ([]testSuiteTeamEdge, error) {
	var teams []testSuiteTeamEdge
	var cursor *string
	for {
		var r *getTestSuiteResponse
		err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
			var err error
			r, err = getTestSuite(ctx, ts.client.genqlient, suiteId, teamPageSize, cursor)

			return retryContextError(err)
		})
		if err != nil {
			return nil, err
		}

		suite, ok := r.Suite.(*getTestSuiteSuite)
		if !ok {
			return nil, fmt.Errorf("suite %s not found", suiteId)
		}

		for _, team := range suite.Teams.Edges {
			teams = append(teams, testSuiteTeamEdge{
				id:          team.Node.Id,
				teamId:      team.Node.Team.Id,
				accessLevel: team.Node.AccessLevel,
			})
		}

		// An empty cursor alongside hasNextPage would page over the same edges forever.
		if !suite.Teams.PageInfo.HasNextPage || suite.Teams.PageInfo.EndCursor == "" {
			return teams, nil
		}
		next := suite.Teams.PageInfo.EndCursor
		cursor = &next
	}
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
