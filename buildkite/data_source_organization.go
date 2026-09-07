package buildkite

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type organizationDatasourceModel struct {
	AllowedApiIpAddresses types.List   `tfsdk:"allowed_api_ip_addresses"`
	ID                    types.String `tfsdk:"id"`
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

	response, err := getOrganization(ctx, o.client.genqlient, o.client.organization)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read organization settings",
			fmt.Sprintf("Unable to read organization: %s", err.Error()),
		)
		return
	}

	if response.Organization.Id == "" {
		resp.Diagnostics.AddError(
			"Unable to find organization",
			fmt.Sprintf("Could not find organization with slug \"%s\"", o.client.organization),
		)
		return
	}

	state.ID = types.StringValue(response.Organization.Id)
	state.UUID = types.StringValue(response.Organization.Uuid)

	// the allowlist is served by api-settings, which only answers an organization administrator.
	// A lookup that cannot see it still reports the identifiers most callers came for.
	settings, err := o.client.getOrganizationAPISettings(ctx)
	if err != nil {
		if !isAPIStatus(err, http.StatusForbidden) {
			resp.Diagnostics.AddError(
				"Unable to read organization API settings",
				fmt.Sprintf("Unable to read organization API settings: %s", err.Error()),
			)
			return
		}
		resp.Diagnostics.AddWarning(
			"Unable to read the allowed API IP addresses",
			fmt.Sprintf("Leaving allowed_api_ip_addresses unset: reading it needs an API token with the read_organization_settings scope, whose user is an organization administrator: %s", err.Error()),
		)
		state.AllowedApiIpAddresses = types.ListNull(types.StringType)
	} else {
		ips, diag := types.ListValueFrom(ctx, types.StringType, strings.Split(settings.AllowedIpAddresses, " "))
		if diag.HasError() {
			resp.Diagnostics.Append(diag...)
			return
		}
		state.AllowedApiIpAddresses = ips
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (*organizationDatasource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to look up the organization settings.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The GraphQL ID of the organization.",
			},
			"uuid": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the organization.",
			},
			"allowed_api_ip_addresses": schema.ListAttribute{
				Computed:    true,
				ElementType: types.StringType,
				MarkdownDescription: "List of IP addresses in CIDR format that are allowed to access the Buildkite API for this organization. " +
					"Reading it requires an API token with the `read_organization_settings` scope, whose user is an organization administrator; it is left unset otherwise.",
			},
		},
	}
}
