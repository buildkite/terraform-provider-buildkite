package buildkite

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type organizationDatasourceModel struct {
	AllowedApiIpAddresses types.List   `tfsdk:"allowed_api_ip_addresses"`
	UUID                  types.String `tfsdk:"uuid"`
}

type organizationDatasource struct {
	client *Client
}

func newOrganizationDatasource() datasource.DataSource {
	return &organizationDatasource{}
}

func (c *organizationDatasource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	c.client = req.ProviderData.(*Client)
}

func (*organizationDatasource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization"
}

func (o *organizationDatasource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state organizationDatasourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	response, err := getOrganization(o.client.genqlient, o.client.organization)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read organization settings",
			fmt.Sprintf("Unable to read organization: %s", err.Error()),
		)
		return
	}

	state.UUID = types.StringValue(response.Organization.Uuid)
	ips, diag := types.ListValueFrom(ctx, types.StringType, strings.Split(response.Organization.AllowedApiIpAddresses, " "))
	state.AllowedApiIpAddresses = ips

	if diag.HasError() {
		resp.Diagnostics.Append(diag...)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (*organizationDatasource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a cluster by name.",
		Attributes: map[string]schema.Attribute{
			"uuid": schema.StringAttribute{
				Computed: true,
			},
			"allowed_api_ip_addresses": schema.ListAttribute{
				Computed:    true,
				ElementType: types.StringType,
			},
		},
	}
}
