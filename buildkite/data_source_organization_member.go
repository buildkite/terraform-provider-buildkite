package buildkite

import (
	"context"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type organizationMemberDatasourceModel struct {
	ID    types.String `tfsdk:"id"`
	UUID  types.String `tfsdk:"uuid"`
	Name  types.String `tfsdk:"name"`
	Email types.String `tfsdk:"email"`
}

type organizationMemberDatasource struct {
	client *Client
}

func newOrganizationMemberDatasource() datasource.DataSource {
	return &organizationMemberDatasource{}
}

func (o *organizationMemberDatasource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	o.client = req.ProviderData.(*Client)
}

func (o *organizationMemberDatasource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_member"
}

func (o *organizationMemberDatasource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: heredoc.Doc(`
			Use this data source to retrieve a specific organization member, using their email. You can find out more about organization members in the Buildkite
			[documentation](https://buildkite.com/docs/platform/team-management).
		`),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The GraphQL ID of the organization member.",
				Computed:            true,
			},
			"uuid": schema.StringAttribute{
				MarkdownDescription: "The UUID of the organization member.",
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the organization member.",
				Computed:            true,
			},
			"email": schema.StringAttribute{
				MarkdownDescription: "The email address of the organization member.",
				Required:            true,
			},
		},
	}
}

func (o *organizationMemberDatasource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state organizationMemberDatasourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	res, err := GetOrganizationMemberByEmail(ctx, o.client.genqlient, o.client.organization, state.Email.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to get organization member",
			fmt.Sprintf("Error getting organization member: %s", err.Error()),
		)
		return
	}

	if len(res.Organization.Members.Edges) == 0 {
		resp.Diagnostics.AddError(
			"No organization member found",
			fmt.Sprintf("Organization member not found: %s", state.Email.ValueString()),
		)
		return
	}

	for _, member := range res.Organization.Members.Edges {
		updateOrganizationMemberDatasourceState(&state, member)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func updateOrganizationMemberDatasourceState(state *organizationMemberDatasourceModel, data GetOrganizationMemberByEmailOrganizationMembersOrganizationMemberConnectionEdgesOrganizationMemberEdge) {
	state.ID = types.StringValue(data.Node.User.Id)
	state.UUID = types.StringValue(data.Node.User.Uuid)
	state.Name = types.StringValue(data.Node.User.Name)
	state.Email = types.StringValue(data.Node.User.Email)
}
