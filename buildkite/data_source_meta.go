package buildkite

import (
	"context"
	"fmt"
	"sort"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const metaId = "https://api.buildkite.com/v2/meta"

type MetaResponse struct {
	WebhookIps []string `json:"webhook_ips"`
}

type metaDatasource struct {
	client *Client
}

type metaDatasourceModel struct {
	WebhookIps types.List   `tfsdk:"webhook_ips"`
	ID         types.String `tfsdk:"id"`
}

func (c *metaDatasource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	c.client = req.ProviderData.(*Client)
}

func (*metaDatasource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_meta"
}

func (m *metaDatasource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state metaDatasourceModel
	var meta MetaResponse
	err := m.client.makeRequest(ctx, "GET", "/v2/meta", nil, &meta)
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to read meta",
			fmt.Sprintf("Failed to read meta %s", err.Error()),
		)
		return
	}

	// a consistent order will ensure a change in ordering from the server won't trigger
	// changes in a terraform plan
	sort.Strings(meta.WebhookIps)

	ips, diag := types.ListValueFrom(ctx, types.StringType, meta.WebhookIps)
	if diag.HasError() {
		resp.Diagnostics.Append(diag...)
		return
	}

	state.WebhookIps = ips
	state.ID = types.StringValue(metaId)

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (*metaDatasource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: heredoc.Doc(`
		Use this data source to look up the source IP addresses that Buildkite may use to send external requests,
		including webhooks and API calls to source control systems (like GitHub Enterprise Server and BitBucket Server).

		More info in the Buildkite [documentation](https://buildkite.com/docs/apis/rest-api/meta).
		`),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Fixed value: `https://api.buildkite.com/v2/meta`.",
			},
			"webhook_ips": schema.ListAttribute{
				MarkdownDescription: "List of IPs in CIDR format.",
				Computed:            true,
				ElementType:         types.StringType,
			},
		},
	}
}

func newMetaDatasource() datasource.DataSource {
	return &metaDatasource{}
}
