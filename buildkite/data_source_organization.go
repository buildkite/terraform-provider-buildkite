package buildkite

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceOrganization() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceOrganizationRead,
		Schema: map[string]*schema.Schema{
			"uuid": {
				Computed: true,
				Type:     schema.TypeString,
			},
			"name": {
				Computed: true,
				Type:     schema.TypeString,
			},
			"allowed_api_ip_addresses": {
				Computed: true,
				Type:     schema.TypeString,
			},
		},
	}
}

// ReadOrganization retrieves a Buildkite organization
func dataSourceOrganizationRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	// Warning or errors can be collected in a slice type
	var diags diag.Diagnostics

	client := m.(*Client)

	response, err := getOrganization(client.genqlient, client.organization)

	if err != nil {
		return diag.FromErr(err)
	}

	if response.Organization.Id == "" {
		return diag.FromErr(fmt.Errorf("organization not found: '%s'", client.organization))
	}

	d.SetId(response.Organization.Id)
	d.Set("uuid", response.Organization.Uuid)
	d.Set("name", response.Organization.Name)
	d.Set("allowed_api_ip_addresses", response.Organization.AllowedApiIpAddresses)

	return diags
}
