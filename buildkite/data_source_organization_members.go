package buildkite

import (
	"context"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type organizationMembersDatasourceModel struct {
	Members []organizationMembersModel `tfsdk:"members"`
}

type organizationMembersModel struct {
	ID    types.String `tfsdk:"id"`
	UUID  types.String `tfsdk:"uuid"`
	Name  types.String `tfsdk:"name"`
	Email types.String `tfsdk:"email"`
}

type organizationMembersDatasource struct {
	client *Client
}

func newOrganizationMembersDatasource() datasource.DataSource {
	return &organizationMembersDatasource{}
}

func (o *organizationMembersDatasource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	o.client = req.ProviderData.(*Client)
}

func (o *organizationMembersDatasource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_members"
}

func (o *organizationMembersDatasource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: heredoc.Doc(`
			Use this data source to retrieve a members of an organization. You can find out more about organization members in the Buildkite
			[documentation](https://buildkite.com/docs/platform/team-management).
		`),
		Attributes: map[string]schema.Attribute{
			"members": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "The GraphQL ID of the to find.",
							Computed:            true,
						},
						"uuid": schema.StringAttribute{
							MarkdownDescription: "The UUID of the organization members.",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "The name of the organization members.",
							Computed:            true,
						},
						"email": schema.StringAttribute{
							MarkdownDescription: "The email address of the organization members.",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (o *organizationMembersDatasource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state organizationMembersDatasourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var cursor *string
	for {
		res, err := GetOrganizationMembers(ctx, o.client.genqlient, o.client.organization, cursor)
		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to get organization members",
				fmt.Sprintf("Error getting members: %s", err.Error()),
			)
			return
		}

		if len(res.Organization.Members.Edges) == 0 {
			resp.Diagnostics.AddError(
				"No organization members found",
				fmt.Sprintf("Error getting members for organization: %s", o.client.organization),
			)
			return
		}

		for _, member := range res.Organization.Members.Edges {
			updateOrganizationMembersDatasourceState(&state, member)
		}

		if !res.Organization.Members.PageInfo.HasNextPage {
			break
		}

		cursor = &res.Organization.Members.PageInfo.EndCursor
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func updateOrganizationMembersDatasourceState(state *organizationMembersDatasourceModel, data GetOrganizationMembersOrganizationMembersOrganizationMemberConnectionEdgesOrganizationMemberEdge) {
	memberState := organizationMembersModel{
		ID:    types.StringValue(data.Node.User.Id),
		UUID:  types.StringValue(data.Node.User.Uuid),
		Name:  types.StringValue(data.Node.User.Name),
		Email: types.StringValue(data.Node.User.Email),
	}

	state.Members = append(state.Members, memberState)
}
