package buildkite

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

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
