package buildkite

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type testSuiteDatasourceModel struct {
	DefaultBranch types.String `tfsdk:"default_branch"`
	ID            types.String `tfsdk:"id"`
	UUID          types.String `tfsdk:"uuid"`
	Name          types.String `tfsdk:"name"`
	Slug          types.String `tfsdk:"slug"`
}

type testSuiteDatasource struct {
	client *Client
}

func newTestSuiteDatasource() datasource.DataSource {
	return &testSuiteDatasource{}
}

// Configure implements datasource.DataSourceWithConfigure.
func (t *testSuiteDatasource) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	t.client = req.ProviderData.(*Client)
}

// Metadata implements datasource.DataSource.
func (t *testSuiteDatasource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_test_suite"
}

// Read implements datasource.DataSource.
func (t *testSuiteDatasource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state testSuiteDatasourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var suite testSuiteResponse
	url := fmt.Sprintf("/v2/analytics/organizations/%s/suites/%s", t.client.organization, state.Slug.ValueString())
	err := t.client.makeRequest(ctx, "GET", url, nil, &suite)
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to read test suite",
			fmt.Sprintf("Failed to read test suite %s", err.Error()),
		)
		return
	}

	state.DefaultBranch = types.StringValue(suite.DefaultBranch)
	state.ID = types.StringValue(suite.GraphqlID)
	state.Name = types.StringValue(suite.Name)
	state.Slug = types.StringValue(suite.Slug)
	state.UUID = types.StringValue(suite.UUID)

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Schema implements datasource.DataSource.
func (t *testSuiteDatasource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A test suite is a collection of tests. A run is to a suite what a build is to a Pipeline." +
			"Use this datasource to read attributes for a [Test Suites](https://buildkite.com/docs/test-analytics) on Buildkite.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The GraphQL ID of the test suite.",
			},
			"uuid": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the test suite.",
			},
			"slug": schema.StringAttribute{
				MarkdownDescription: "The generated slug of the test suite.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The name to give the test suite.",
			},
			"default_branch": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The default branch for the repository this test suite is for.",
			},
		},
	}
}

// ensure the interface is implemented
var _ datasource.DataSourceWithConfigure = &testSuiteDatasource{}
