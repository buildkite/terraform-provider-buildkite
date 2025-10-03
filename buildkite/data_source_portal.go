package buildkite

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type portalDatasource struct {
	client *Client
}

type portalDatasourceModel struct {
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

func newPortalDatasource() datasource.DataSource {
	return &portalDatasource{}
}

func (p *portalDatasource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	p.client = req.ProviderData.(*Client)
}

func (p *portalDatasource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_portal"
}

func (p *portalDatasource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config portalDatasourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := fmt.Sprintf("/v2/organizations/%s/portals/%s", p.client.organization, config.Slug.ValueString())

	var result portalAPIResponse
	err := p.client.makeRequest(ctx, http.MethodGet, path, nil, &result)
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			resp.Diagnostics.AddError(
				"Portal not found",
				fmt.Sprintf("Could not find portal with slug \"%s\"", config.Slug.ValueString()),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Unable to read portal",
			fmt.Sprintf("Unable to read portal: %s", err.Error()),
		)
		return
	}

	state := portalDatasourceModel{
		UUID:          types.StringValue(result.UUID),
		Slug:          types.StringValue(result.Slug),
		Name:          types.StringValue(result.Name),
		Query:         types.StringValue(result.Query),
		UserInvokable: types.BoolValue(result.UserInvokable),
		CreatedAt:     types.StringValue(result.CreatedAt),
	}

	if result.Description != nil {
		state.Description = types.StringValue(*result.Description)
	} else {
		state.Description = types.StringNull()
	}

	if result.AllowedIPAddresses != nil {
		state.AllowedIPAddresses = types.StringValue(*result.AllowedIPAddresses)
	} else {
		state.AllowedIPAddresses = types.StringNull()
	}

	if result.CreatedBy != nil {
		createdByMap := map[string]attr.Value{
			"uuid":  types.StringValue(result.CreatedBy.UUID),
			"name":  types.StringValue(result.CreatedBy.Name),
			"email": types.StringValue(result.CreatedBy.Email),
		}
		createdByObj, d := types.ObjectValue(map[string]attr.Type{
			"uuid":  types.StringType,
			"name":  types.StringType,
			"email": types.StringType,
		}, createdByMap)
		resp.Diagnostics.Append(d...)
		state.CreatedBy = createdByObj
	} else {
		state.CreatedBy = types.ObjectNull(map[string]attr.Type{
			"uuid":  types.StringType,
			"name":  types.StringType,
			"email": types.StringType,
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (p *portalDatasource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to retrieve a portal by slug. You can find out more about portals in the Buildkite [documentation](https://buildkite.com/docs/apis/portals).",
		Attributes: map[string]schema.Attribute{
			"uuid": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the portal.",
			},
			"slug": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The slug of the portal to retrieve.",
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
	}
}
