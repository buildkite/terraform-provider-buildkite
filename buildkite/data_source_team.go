package buildkite

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type teamDatasourceModel struct {
	ID                        types.String `tfsdk:"id"`
	UUID                      types.String `tfsdk:"uuid"`
	Slug                      types.String `tfsdk:"slug"`
	Name                      types.String `tfsdk:"name"`
	Privacy                   types.String `tfsdk:"privacy"`
	Description               types.String `tfsdk:"description"`
	IsDefaultTeam             types.Bool   `tfsdk:"default_team"`
	DefaultMemberRole         types.String `tfsdk:"default_member_role"`
	MembersCanCreatePipelines types.Bool   `tfsdk:"members_can_create_pipelines"`
}

type teamDatasource struct {
	client *Client
}

func newTeamDatasource() datasource.DataSource {
	return &teamDatasource{}
}

func (t *teamDatasource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	t.client = req.ProviderData.(*Client)
}

func (t *teamDatasource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_team"
}

func (t *teamDatasource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "This datasource allows you to get a team from Buildkite.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the team.",
				Computed:    true,
			},
			"uuid": schema.StringAttribute{
				Description: "The UUID of the team.",
				Computed:    true,
			},
			"slug": schema.StringAttribute{
				Description: "The slug of the team.",
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the team.",
				Computed:    true,
			},
			"privacy": schema.StringAttribute{
				Description: "The privacy of the team.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "The description of the team.",
				Computed:    true,
			},
			"default_team": schema.BoolAttribute{
				Description: "Whether the team is the default team.",
				Computed:    true,
			},
			"default_member_role": schema.StringAttribute{
				Description: "The default member role of the team.",
				Computed:    true,
			},
			"members_can_create_pipelines": schema.BoolAttribute{
				Description: "Whether members can create pipelines.",
				Computed:    true,
			},
		},
	}
}

func (t *teamDatasource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state teamDatasourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	res, err := getTeam(t.client.genqlient, state.ID.ValueString())

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to get team",
			fmt.Sprintf("Error getting team: %s", err.Error()),
		)
		return
	}

	if converted, ok := res.GetNode().(*getTeamNodeTeam); ok {
		if converted == nil {
			resp.Diagnostics.AddError(
				"Unable to get team",
				"Error getting team: nil response",
			)
			return
		}
		state.ID = types.StringValue(converted.Id)
		state.UUID = types.StringValue(converted.Uuid)
		state.Slug = types.StringValue(converted.Slug)
		state.Name = types.StringValue(converted.Name)
		state.Privacy = types.StringValue(converted.Privacy)
		state.Description = types.StringValue(converted.Description)
		state.IsDefaultTeam = types.BoolValue(converted.IsDefaultTeam)
		state.DefaultMemberRole = types.StringValue(converted.DefaultMemberRole)
		state.MembersCanCreatePipelines = types.BoolValue(converted.MembersCanCreatePipelines)
	}
}
