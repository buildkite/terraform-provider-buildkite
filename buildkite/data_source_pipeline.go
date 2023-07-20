package buildkite

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type pipelineDataSourceModel struct {
	ID                    types.String `tfsdk:"id"`
	UUID                  types.String `tfsdk:"uuid"`
	DefaultBranch  		  types.String `tfsdk:"default_branch"`
	Description  		  types.String `tfsdk:"description"`
	Repository  		  types.String `tfsdk:"repository"`
	Slug  		 		  types.String `tfsdk:"slug"`
	WebhookUrl  		  types.String `tfsdk:"webhook_url"`
}

type pipelineDatasource struct {
	client *Client
}

func newPipelineDatasource() datasource.DataSource {
	return &clusterDatasource{}
}

func (p *pipelineDatasource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	p.client = req.ProviderData.(*Client)
}

func (*pipelineDatasource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pipeline"
}

func (*pipelineDatasource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"uuid": schema.StringAttribute{
				Computed: true,
			},
			"default_branch": schema.StringAttribute{
				Computed:            true,
			},
			"description": schema.StringAttribute{
				Computed:            true,
			},
			"repository": schema.StringAttribute{
				Computed:            true,
			},
			"slug": schema.StringAttribute{
				Required:            true,
			},
			"webhook_url": schema.StringAttribute{
				Computed:            true,
			},
		},
	}
}

func (c *pipelineDatasource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state pipelineDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	
	if resp.Diagnostics.HasError() {
		return
	}

	orgPipelineSlug := fmt.Sprintf("%s/%s", c.client.organization, state.Slug.String())
	pipeline, err := getPipeline(c.client.genqlient, orgPipelineSlug)

	if err != nil {
		return diag.FromErr(err)
	}

	if pipeline.Pipeline.Id == "" {
		return diag.FromErr(errors.New("Pipeline not found"))
	}


}

/*
func dataSourcePipeline() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourcePipelineRead,
		Schema: map[string]*schema.Schema{
			"default_branch": {
				Computed: true,
				Type:     schema.TypeString,
			},
			"description": {
				Computed: true,
				Type:     schema.TypeString,
			},
			"name": {
				Computed: true,
				Type:     schema.TypeString,
			},
			"repository": {
				Computed: true,
				Type:     schema.TypeString,
			},
			"slug": {
				Required: true,
				Type:     schema.TypeString,
			},
			"webhook_url": {
				Computed: true,
				Type:     schema.TypeString,
			},
		},
	}
}

// ReadPipeline retrieves a Buildkite pipeline
func dataSourcePipelineRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	// Warning or errors can be collected in a slice type
	var diags diag.Diagnostics

	client := m.(*Client)

	orgPipelineSlug := fmt.Sprintf("%s/%s", client.organization, d.Get("slug").(string))
	pipeline, err := getPipeline(client.genqlient, orgPipelineSlug)

	if err != nil {
		return diag.FromErr(err)
	}

	if pipeline.Pipeline.Id == "" {
		return diag.FromErr(errors.New("Pipeline not found"))
	}

	d.SetId(pipeline.Pipeline.Id)
	d.Set("default_branch", pipeline.Pipeline.DefaultBranch)
	d.Set("description", pipeline.Pipeline.Description)
	d.Set("name", pipeline.Pipeline.Name)
	d.Set("repository", pipeline.Pipeline.Repository.Url)
	d.Set("slug", pipeline.Pipeline.Slug)
	d.Set("webhook_url", pipeline.Pipeline.WebhookURL)

	return diags
}
*/