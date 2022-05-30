package buildkite

import (
	"context"
	"sort"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

type MetaResponse struct {
	WebhookIps []string `json:"webhook_ips"`
}

func dataSourceMeta() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceMetaRead,
		Schema: map[string]*schema.Schema{
			"webhook_ips": {
				Computed: true,
				Type:     schema.TypeList,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
		},
	}
}

func dataSourceMetaRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*Client)
	meta := MetaResponse{}
	err := client.makeRequest("GET", "/v2/meta", nil, &meta)

	if err != nil {
		return diag.FromErr(err)
	}

	// a consistent order will ensure a change in ordering from the server won't trigger
	// changes in a terraform plan
	sort.Strings(meta.WebhookIps)

	if err := d.Set("webhook_ips", meta.WebhookIps); err != nil {
		return diag.FromErr(err)
	}

	// It seems we need to set an ID for a data source, so pick a stable one that won't change
	d.SetId("https://api.buildkite.com/v2/meta")

	return nil
}
