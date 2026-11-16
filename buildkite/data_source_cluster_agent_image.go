package buildkite

import (
	"context"
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type clusterAgentImageDatasource struct {
	client *Client
}

type clusterAgentImageDatasourceModel struct {
	ID         types.String `tfsdk:"id"`
	ClusterID  types.String `tfsdk:"cluster_id"`
	Name       types.String `tfsdk:"name"`
	Dockerfile types.String `tfsdk:"dockerfile"`
	Status     types.String `tfsdk:"status"`
	ImageRef   types.String `tfsdk:"image_ref"`
}

func newClusterAgentImageDatasource() datasource.DataSource {
	return &clusterAgentImageDatasource{}
}

func (*clusterAgentImageDatasource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_agent_image"
}

func (d *clusterAgentImageDatasource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	d.client = req.ProviderData.(*Client)
}

func (*clusterAgentImageDatasource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: heredoc.Doc(`
			Use this data source to retrieve a custom agent image for Linux hosted agents by its ID or exact
			name within a cluster. Configure exactly one of ` + "`id` or `name`" + `, alongside ` + "`cluster_id`" + `.

			The API token requires the ` + "`read_clusters`" + ` scope, and its user must have permission to
			manage the cluster. Read access to the cluster alone is not sufficient.

			This data source reports the current build status without waiting for the image to finish
			building. Only use ` + "`image_ref`" + ` for a hosted queue when the image status is ` + "`READY`" + `.
		`),
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The UUID of the cluster the image belongs to. Use the cluster's `uuid`, not its GraphQL `id`.",
			},
			"id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The ID of the agent image. Configure either `id` or `name`, but not both.",
				Validators: []validator.String{
					stringvalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
					stringvalidator.LengthAtLeast(1),
				},
			},
			"name": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The name of the agent image, matched exactly and case-sensitively within the cluster. Configure either `name` or `id`, but not both.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"dockerfile": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The Dockerfile instructions the image was created with, without the managed base image's `FROM` instruction.",
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The current build status of the image: `BUILDING`, `READY` or `FAILED`. This data source does not wait for the build to finish.",
			},
			"image_ref": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The reference of the built image, for a hosted queue's `hosted_agents.linux.agent_image_ref`. May be empty before the image is ready.",
			},
		},
	}
}

func (d *clusterAgentImageDatasource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state clusterAgentImageDatasourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := d.client.readTimeout(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var image *clusterAgentImage
	var err error
	if !state.ID.IsNull() {
		image, err = d.client.getClusterAgentImage(requestCtx, state.ClusterID.ValueString(), state.ID.ValueString())
	} else {
		// The index endpoint returns the complete list without pagination.
		var images []clusterAgentImage
		err = d.client.makeRequest(requestCtx, http.MethodGet, d.client.clusterAgentImagePath(state.ClusterID.ValueString(), ""), nil, &images)
		if err == nil {
			for i := range images {
				if images[i].Name == state.Name.ValueString() {
					image = &images[i]
					break
				}
			}
			if image == nil {
				resp.Diagnostics.AddError("Unable to find cluster agent image",
					fmt.Sprintf("Could not find a cluster agent image with name %q in cluster %s", state.Name.ValueString(), state.ClusterID.ValueString()))
				return
			}
		}
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to read cluster agent image", fmt.Sprintf("Unable to read cluster agent image: %s", err))
		return
	}

	state.ID = types.StringValue(image.ID)
	state.Name = types.StringValue(image.Name)
	state.Dockerfile = types.StringValue(image.Dockerfile)
	state.Status = types.StringValue(image.Status)
	state.ImageRef = types.StringValue(image.ImageRef)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
