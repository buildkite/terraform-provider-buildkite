package buildkite

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type testSuiteTeamModel struct {
	ID            types.String `tfsdk:"id"`
	UUID          types.String `tfsdk:"uuid"`
	TestSuiteId   types.String `tfsdk:"test_suite_id"`
	team_id	      types.String `tfsdk:"team_id"`
	Name          types.String `tfsdk:"access_level"`

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
			},
			"team_id": schema.StringAttribute{
				Required: true,
			},
			"access_level": schema.StringAttribute{
				Required: true,
			},
		},
	}
}

func (tst *testSuiteTeamResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {

}

func (tst *testSuiteTeamResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {

}

func (tst *testSuiteTeamResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {

}

func (tst *testSuiteTeamResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {

}