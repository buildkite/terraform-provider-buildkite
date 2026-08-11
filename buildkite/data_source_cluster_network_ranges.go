package buildkite

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The REST path segment must be a cluster UUID; the API rejects anything else with an opaque 404.
// Matched case-insensitively to agree with the API.
var clusterUUIDRegex = regexp.MustCompile(`(?i)^[a-f0-9]{8}(-[a-f0-9]{4}){3}-[a-f0-9]{12}$`)

type clusterNetworkRangesDatasourceModel struct {
	ClusterUUID types.String               `tfsdk:"cluster_uuid"`
	Ranges      []clusterNetworkRangeModel `tfsdk:"ranges"`
}

type clusterNetworkRangeModel struct {
	CidrRange types.String `tfsdk:"cidr_range"`
	Kind      types.String `tfsdk:"kind"`
}

// Pointers so a null from the API stays null in state rather than decoding to an empty string,
// which would silently reach a firewall rule as "".
type clusterNetworkRangeAPIResponse struct {
	CidrRange *string `json:"cidr_range"`
	Kind      *string `json:"kind"`
}

type clusterNetworkRangesDatasource struct {
	client *Client
}

func newClusterNetworkRangesDatasource() datasource.DataSource {
	return &clusterNetworkRangesDatasource{}
}

func (c *clusterNetworkRangesDatasource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	c.client = req.ProviderData.(*Client)
}

func (*clusterNetworkRangesDatasource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_network_ranges"
}

func (*clusterNetworkRangesDatasource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: heredoc.Doc(`
			Use this data source to retrieve the egress network ranges that a cluster's hosted agents
			connect out from. This is useful for allowing Buildkite hosted agents through a firewall or
			security group from the same Terraform configuration.

			The API token must have the read_clusters scope, and the user it belongs to must be able to
			manage the cluster. A token that can only read clusters will receive a 403.

			Clusters that do not have a hosted agents queue have no egress ranges and return an empty list.
		`),
		Attributes: map[string]schema.Attribute{
			"cluster_uuid": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The UUID of the cluster to read network ranges for. Note this is the cluster's `uuid`, not its GraphQL `id`.",
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						clusterUUIDRegex,
						"must be a cluster UUID, for example buildkite_cluster.example.uuid (not the GraphQL id)",
					),
				},
			},
			"ranges": schema.SetNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The egress network ranges for the cluster. Unordered, since the ranges are reported by an upstream service that does not guarantee ordering.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"cidr_range": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The range in CIDR notation.",
						},
						"kind": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The kind of range, for example `NAMESPACE_MANAGED`. Values originate upstream and are not a fixed set.",
						},
					},
				},
			},
		},
	}
}

func (c *clusterNetworkRangesDatasource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state clusterNetworkRangesDatasourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterUUID := state.ClusterUUID.ValueString()

	ranges, err := c.getNetworkRanges(ctx, clusterUUID)
	if err != nil {
		detail := fmt.Sprintf("Unable to read network ranges for cluster %s: %s", clusterUUID, err.Error())
		if strings.Contains(err.Error(), "status: 403") {
			detail += "\n\nThis endpoint needs the read_clusters scope and permission to manage the cluster. A token that can only read clusters is not sufficient."
		}
		resp.Diagnostics.AddError("Unable to read cluster network ranges", detail)
		return
	}

	// Clusters without hosted agents return no ranges. Build a non-nil slice so the attribute is a
	// known empty set rather than null, which would break configurations iterating over it.
	state.Ranges = make([]clusterNetworkRangeModel, 0, len(ranges))
	for _, r := range ranges {
		state.Ranges = append(state.Ranges, clusterNetworkRangeModel{
			CidrRange: types.StringPointerValue(r.CidrRange),
			Kind:      types.StringPointerValue(r.Kind),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// getNetworkRanges reads a cluster's egress ranges. The endpoint returns them all in one response,
// so there is no pagination to follow. Transient failures are retried by the shared REST client in
// client.go, which backs off on 5xx and 429.
func (c *clusterNetworkRangesDatasource) getNetworkRanges(ctx context.Context, clusterUUID string) ([]clusterNetworkRangeAPIResponse, error) {
	path := fmt.Sprintf("/v2/organizations/%s/clusters/%s/network_ranges", c.client.organization, clusterUUID)

	var ranges []clusterNetworkRangeAPIResponse
	if err := c.client.makeRequest(ctx, http.MethodGet, path, nil, &ranges); err != nil {
		return nil, err
	}

	return ranges, nil
}
