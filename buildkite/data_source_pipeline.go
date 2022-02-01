package buildkite

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/shurcooL/graphql"
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
	var query struct {
		Pipeline PipelineNode `graphql:"pipeline(slug: $slug)"`
	}
	orgPipelineSlug := fmt.Sprintf("%s/%s", client.organization, d.Get("slug").(string))
	vars := map[string]interface{}{
		"slug": graphql.ID(orgPipelineSlug),
	}

	err := client.graphql.Query(context.Background(), &query, vars)
	if err != nil {
		return diag.FromErr(err)
	}

	if query.Pipeline.ID == "" {
		return diag.FromErr(errors.New("Pipeline not found"))
	}

	d.SetId(query.Pipeline.ID.(string))
	d.Set("default_branch", string(query.Pipeline.DefaultBranch))
	d.Set("description", string(query.Pipeline.Description))
	d.Set("name", string(query.Pipeline.Name))
	d.Set("repository", string(query.Pipeline.Repository.URL))
	d.Set("slug", string(query.Pipeline.Slug))
	d.Set("webhook_url", string(query.Pipeline.WebhookURL))

	return diags
}
