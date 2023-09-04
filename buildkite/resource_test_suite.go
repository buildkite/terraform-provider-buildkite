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

		if err != nil {
			if isRetryableError(err) {
				return retry.RetryableError(err)
			}
			return retry.NonRetryableError(err)
		}

		return nil
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to find team",
			err.Error(),
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
	payload["show_api_token"] = true
	payload["team_ids"] = []string{teamOwnerUuid}

	// Construct URL to call to the REST API
	url := fmt.Sprintf("/v2/analytics/organizations/%s/suites", ts.client.organization)

	// Use the Create timeout for test suite creation
	timeout, diags = ts.client.timeouts.Create(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	createErr := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		err = ts.client.makeRequest(ctx, "POST", url, payload, &response)

		if err != nil {
			if isRetryableError(err) {
				return retry.RetryableError(err)
			}
			return retry.NonRetryableError(err)
		}

		return nil
	})

	if createErr != nil {
		resp.Diagnostics.AddError(
			"Failed to create test suite",
			createErr.Error(),
		)
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

		if err != nil {
			if isRetryableError(err) {
				return retry.RetryableError(err)
			}
			return retry.NonRetryableError(err)
		}

		return nil
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to delete test suite",
			err.Error(),
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
		if err != nil {
			if isRetryableError(err) {
				return retry.RetryableError(err)
			}
			return retry.NonRetryableError(err)
		}

		return nil
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to load test suite from GraphQL",
			err.Error(),
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
	} else {
		// Test suite was removed - remove from state
		resp.Diagnostics.AddWarning("Test suite not found", "Removing test suite from state")
		resp.State.RemoveResource(ctx)
		return
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
				PlanModifiers: []planmodifier.String{
					custom_modifier.UseStateIfUnchanged("name"),
				},
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

	// Construct URL to call to the REST API
	url := fmt.Sprintf("/v2/analytics/organizations/%s/suites/%s", ts.client.organization, state.Slug.ValueString())
	updateErr := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		err := ts.client.makeRequest(ctx, "PATCH", url, payload, &response)

		if err != nil {
			if isRetryableError(err) {
				return retry.RetryableError(err)
			}
			return retry.NonRetryableError(err)
		}

		return nil
	})

	if updateErr != nil {
		resp.Diagnostics.AddError(
			"Failed to update test suite",
			updateErr.Error(),
		)
		return
	}

	state.Name = plan.Name
	state.DefaultBranch = plan.DefaultBranch
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

			if err != nil {
				if isRetryableError(err) {
					return retry.RetryableError(err)
				}
				return retry.NonRetryableError(err)
			}

			return nil
		})

		if err != nil {
			resp.Diagnostics.AddError(
				"Could not add new owner team",
				err.Error(),
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

					if err != nil {
						if isRetryableError(err) {
							return retry.RetryableError(err)
						}
						return retry.NonRetryableError(err)
					}
					return nil
				})

				if err != nil {
					resp.Diagnostics.AddError(
						"Failed to delete team owner",
						err.Error(),
					)
					return
				}
			}
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
