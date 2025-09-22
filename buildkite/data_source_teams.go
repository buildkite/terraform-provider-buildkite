package buildkite

import (
	"context"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type teamsDatasourceModel struct {
	Teams []teamsModel `tfsdk:"teams"`
}

type teamsModel struct {
	ID                        types.String `tfsdk:"id"`
	UUID                      types.String `tfsdk:"uuid"`
	Name                      types.String `tfsdk:"name"`
	Description               types.String `tfsdk:"description"`
	Slug                      types.String `tfsdk:"slug"`
	Privacy                   types.String `tfsdk:"privacy"`
	IsDefaultTeam             types.Bool   `tfsdk:"is_default_team"`
	DefaultMemberRole         types.String `tfsdk:"default_member_role"`
	MembersCanCreatePipelines types.Bool   `tfsdk:"members_can_create_pipelines"`
}

type teamsDatasource struct {
	client *Client
}

func newTeamsDatasource() datasource.DataSource {
	return &teamsDatasource{}
}

func (t *teamsDatasource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	t.client = req.ProviderData.(*Client)
}

func (t *teamsDatasource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_teams"
}

func (t *teamsDatasource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: heredoc.Doc(`
			Use this data source to retrieve teams of an organization. You can find out more about teams in the Buildkite
			[documentation](https://buildkite.com/docs/platform/team-management).
		`),
		Attributes: map[string]schema.Attribute{
			"teams": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "The GraphQL ID of the team.",
							Computed:            true,
						},
						"uuid": schema.StringAttribute{
							MarkdownDescription: "The UUID of the team.",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "The name of the team.",
							Computed:            true,
						},
						"description": schema.StringAttribute{
							MarkdownDescription: "The description of the team.",
							Computed:            true,
						},
						"slug": schema.StringAttribute{
							MarkdownDescription: "The slug of the team.",
							Computed:            true,
						},
						"privacy": schema.StringAttribute{
							MarkdownDescription: "The privacy setting of the team.",
							Computed:            true,
						},
						"is_default_team": schema.BoolAttribute{
							MarkdownDescription: "Whether this is the default team for the organization.",
							Computed:            true,
						},
						"default_member_role": schema.StringAttribute{
							MarkdownDescription: "The default member role for new team members.",
							Computed:            true,
						},
						"members_can_create_pipelines": schema.BoolAttribute{
							MarkdownDescription: "Whether team members can create pipelines.",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (t *teamsDatasource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state teamsDatasourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var cursor *string
	for {
		res, err := GetOrganizationTeams(ctx, t.client.genqlient, t.client.organization, cursor)
		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to get organization teams",
				fmt.Sprintf("Error getting organization teams: %s", err.Error()),
			)
			return
		}

		if len(res.Organization.Teams.Edges) == 0 {
			resp.Diagnostics.AddError(
				"No organization teams found",
				fmt.Sprintf("Error getting teams for organization: %s", t.client.organization),
			)
			return
		}

		for _, team := range res.Organization.Teams.Edges {
			updateTeamsDatasourceState(&state, team)
		}

		if !res.Organization.Teams.PageInfo.HasNextPage {
			break
		}

		cursor = &res.Organization.Teams.PageInfo.EndCursor
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func updateTeamsDatasourceState(state *teamsDatasourceModel, data GetOrganizationTeamsOrganizationTeamsTeamConnectionEdgesTeamEdge) {
	teamState := teamsModel{
		ID:                        types.StringValue(data.Node.Id),
		UUID:                      types.StringValue(data.Node.Uuid),
		Name:                      types.StringValue(data.Node.Name),
		Description:               types.StringValue(data.Node.Description),
		Slug:                      types.StringValue(data.Node.Slug),
		Privacy:                   types.StringValue(string(data.Node.Privacy)),
		IsDefaultTeam:             types.BoolValue(data.Node.IsDefaultTeam),
		DefaultMemberRole:         types.StringValue(string(data.Node.DefaultMemberRole)),
		MembersCanCreatePipelines: types.BoolValue(data.Node.MembersCanCreatePipelines),
	}

	state.Teams = append(state.Teams, teamState)
}
