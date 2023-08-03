package buildkite

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type testSuiteTeamModel struct {
	ID          types.String `tfsdk:"id"`
	UUID        types.String `tfsdk:"uuid"`
	TestSuiteId types.String `tfsdk:"test_suite_id"`
	TeamID      types.String `tfsdk:"team_id"`
	AccessLevel types.String `tfsdk:"access_level"`
}

type testSuiteTeamResource struct {
	client *Client
}

func newTestSuiteTeamResource() resource.Resource {
	return &testSuiteTeamResource{}
}

func (*testSuiteTeamResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_test_suite_team"
}

func (tst *testSuiteTeamResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	tst.client = req.ProviderData.(*Client)
}

func (tst *testSuiteTeamResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A test suite team links a collection of tests (suite) to a particular team.",
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
			"test_suite_id": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "The GraphQL ID of the test suite.",
			},
			"team_id": schema.StringAttribute{
				
				Required: true,
				MarkdownDescription: "The GraphQL ID of the team.",
			},
			"access_level": schema.StringAttribute{
				Required: true,
				Validators: []validator.String{
					stringvalidator.OneOf("MANAGE_AND_READ", "READ_ONLY"),
				},
				MarkdownDescription: " The access level the team has on the test suite. Either `READ_ONLY` or `MANAGE_AND_READ`",
			},
		},
	}
}

func (tst *testSuiteTeamResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var state testSuiteTeamModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("Adding team %s to test suite %s ...", state.TeamID.ValueString(), state.TestSuiteId.ValueString())
	apiResponse, err := createTestSuiteTeam(
		tst.client.genqlient,
		state.TeamID.ValueString(),
		state.TestSuiteId.ValueString(),
		SuiteAccessLevels(state.AccessLevel.ValueString()),
	)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create test suite team",
			fmt.Sprintf("Unable to create test suite team: %s", err.Error()),
		)
		return
	}

	// Set ID and UUID to state
	state.ID = types.StringValue(apiResponse.TeamSuiteCreate.TeamSuite.Id)
	state.UUID = types.StringValue(apiResponse.TeamSuiteCreate.TeamSuite.TeamSuiteUuid)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (tst *testSuiteTeamResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state testSuiteTeamModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("Reading test suite team with ID %s ...", state.ID.ValueString())
	apiResponse, err := getNode(tst.client.genqlient, state.ID.ValueString())

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read test suite team",
			fmt.Sprintf("Unable to read test suite team member: %s", err.Error()),
		)
		return
	}

	// Convert fron Node to getNodeTeamMember type
	if teamSuiteNode, ok := apiResponse.GetNode().(*getNodeNodeTeamSuite); ok {
		if teamSuiteNode == nil {
			resp.Diagnostics.AddError(
				"Unable to get test suite team",
				"Error getting test suite team: nil response",
			)
			return
		}
		updateTeamSuiteTeamResource(&state, *teamSuiteNode)
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	} else {
		// Test suite team was removed - remove from state
		resp.Diagnostics.AddWarning("Test suite team not found", "Removing test suite team from state")
		resp.State.RemoveResource(ctx)
		return
	}
}

func (tst *testSuiteTeamResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (tst *testSuiteTeamResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state testSuiteTeamModel
	var testSuiteTeamAccessLevel string

	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("access_level"), &testSuiteTeamAccessLevel)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	log.Printf("Updating team %s in test suite %s to %s ...", state.TeamID.ValueString(), state.TestSuiteId.ValueString(), testSuiteTeamAccessLevel)
	apiResponse, err := updateTestSuiteTeam(
		tst.client.genqlient,
		state.ID.ValueString(),
		SuiteAccessLevels(testSuiteTeamAccessLevel),
	)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to update test suite team",
			fmt.Sprintf("Unable to update test suite team: %s", err.Error()),
		)
		return
	}

	// Update state access level
	state.AccessLevel = types.StringValue(string(apiResponse.TeamSuiteUpdate.TeamSuite.AccessLevel))

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (tst *testSuiteTeamResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state testSuiteTeamModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("Deleting team %s's access to test suite %s ...", state.TeamID.ValueString(), state.TestSuiteId.ValueString())
	_, err := deleteTestSuiteTeam(tst.client.genqlient, state.ID.ValueString())

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete test suite team",
			fmt.Sprintf("Unable to delete test suite team: %s", err.Error()),
		)
		return
	}
}

func updateTeamSuiteTeamResource(tstm *testSuiteTeamModel, tsn getNodeNodeTeamSuite) {
	tstm.ID = types.StringValue(tsn.Id)
	tstm.UUID = types.StringValue(tsn.TeamSuiteUuid)
	tstm.TeamID = types.StringValue(tsn.Team.Id)
	tstm.TestSuiteId = types.StringValue(tsn.Suite.Id)
	tstm.AccessLevel = types.StringValue(string(tsn.AccessLevel))
}
