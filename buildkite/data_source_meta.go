package buildkite

import (
	"context"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

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
	meta := MetaResponse{}
	err := m.client.makeRequest("GET", "/v2/meta", nil, &meta)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read meta", err.Error())
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
	state.ID = types.StringValue("https://api.buildkite.com/v2/meta")

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (*metaDatasource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "", // TODO
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"webhook_ips": schema.ListAttribute{
				Computed:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func newMetaDatasource() datasource.DataSource {
	return &metaDatasource{}
}
