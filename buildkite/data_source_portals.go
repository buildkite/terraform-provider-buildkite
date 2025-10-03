package buildkite

import (
	"context"
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type portalsDatasourceModel struct {
	Portals []portalsModel `tfsdk:"portals"`
}

type portalsModel struct {
	UUID               types.String `tfsdk:"uuid"`
	Slug               types.String `tfsdk:"slug"`
	Name               types.String `tfsdk:"name"`
	Description        types.String `tfsdk:"description"`
	Query              types.String `tfsdk:"query"`
	AllowedIPAddresses types.String `tfsdk:"allowed_ip_addresses"`
	UserInvokable      types.Bool   `tfsdk:"user_invokable"`
	CreatedAt          types.String `tfsdk:"created_at"`
	CreatedBy          types.Object `tfsdk:"created_by"`
}

type portalsDatasource struct {
	client *Client
}

func newPortalsDatasource() datasource.DataSource {
	return &portalsDatasource{}
}

func (p *portalsDatasource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	p.client = req.ProviderData.(*Client)
}

func (p *portalsDatasource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_portals"
}

func (p *portalsDatasource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: heredoc.Doc(`
			Use this data source to retrieve all portals for an organization.
		`),
		Attributes: map[string]schema.Attribute{
			"portals": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"uuid": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The UUID of the portal.",
						},
						"slug": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The slug of the portal.",
						},
						"name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The name of the portal.",
						},
						"description": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The description of the portal.",
						},
						"query": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The GraphQL query that the portal executes.",
						},
						"allowed_ip_addresses": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Space-delimited list of IP addresses (in CIDR notation) allowed to invoke this portal.",
						},
						"user_invokable": schema.BoolAttribute{
							Computed:            true,
							MarkdownDescription: "Whether users can invoke the portal.",
						},
						"created_at": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The time when the portal was created.",
						},
						"created_by": schema.SingleNestedAttribute{
							Computed:            true,
							MarkdownDescription: "Information about the user who created the portal.",
							Attributes: map[string]schema.Attribute{
								"uuid": schema.StringAttribute{
									Computed:            true,
									MarkdownDescription: "The UUID of the user.",
								},
								"name": schema.StringAttribute{
									Computed:            true,
									MarkdownDescription: "The name of the user.",
								},
								"email": schema.StringAttribute{
									Computed:            true,
									MarkdownDescription: "The email of the user.",
								},
							},
						},
					},
				},
			},
		},
	}
}

func (p *portalsDatasource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state portalsDatasourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := fmt.Sprintf("/v2/organizations/%s/portals", p.client.organization)

	var results []portalAPIResponse
	err := p.client.makeRequest(ctx, http.MethodGet, path, nil, &results)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to get organization portals",
			fmt.Sprintf("Error getting organization portals: %s", err.Error()),
		)
		return
	}

	for _, data := range results {
		model := portalsModel{
			UUID:          types.StringValue(data.UUID),
			Slug:          types.StringValue(data.Slug),
			Name:          types.StringValue(data.Name),
			Query:         types.StringValue(data.Query),
			UserInvokable: types.BoolValue(data.UserInvokable),
			CreatedAt:     types.StringValue(data.CreatedAt),
		}

		if data.Description != nil {
			model.Description = types.StringValue(*data.Description)
		} else {
			model.Description = types.StringNull()
		}

		if data.AllowedIPAddresses != nil {
			model.AllowedIPAddresses = types.StringValue(*data.AllowedIPAddresses)
		} else {
			model.AllowedIPAddresses = types.StringNull()
		}

		if data.CreatedBy != nil {
			createdByMap := map[string]attr.Value{
				"uuid":  types.StringValue(data.CreatedBy.UUID),
				"name":  types.StringValue(data.CreatedBy.Name),
				"email": types.StringValue(data.CreatedBy.Email),
			}
			createdByObj, d := types.ObjectValue(map[string]attr.Type{
				"uuid":  types.StringType,
				"name":  types.StringType,
				"email": types.StringType,
			}, createdByMap)
			resp.Diagnostics.Append(d...)
			model.CreatedBy = createdByObj
		} else {
			model.CreatedBy = types.ObjectNull(map[string]attr.Type{
				"uuid":  types.StringType,
				"name":  types.StringType,
				"email": types.StringType,
			})
		}

		state.Portals = append(state.Portals, model)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
