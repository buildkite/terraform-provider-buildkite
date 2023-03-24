package buildkite

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceOrganizationSettings() *schema.Resource {
	return &schema.Resource{
		CreateContext: CreateUpdateDeleteOrganizationSettings,
		ReadContext:   ReadOrganizationSettings,
		UpdateContext: CreateUpdateDeleteOrganizationSettings,
		DeleteContext: CreateUpdateDeleteOrganizationSettings,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},
		Schema: map[string]*schema.Schema{
			"uuid": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"name": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"allowed_api_ip_addresses": {
				Optional: true,
				Type:     schema.TypeList,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
		},
	}
}

// CreateUpdateDeleteOrganizationSettings is used for the creation, updating and deleting of a Buildkite organization's settings
func CreateUpdateDeleteOrganizationSettings(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	client := m.(*Client)

	response, err := getOrganization(client.genqlient, client.organization)

	if err != nil {
		return diag.FromErr(err)
	}

	if response.Organization.Id == "" {
		return diag.FromErr(fmt.Errorf("organization not found: '%s'", client.organization))
	}

	cidrs := strings.Join(d.Get("allowed_api_ip_addresses").([]string), " ")
	apiResponse, err := setApiIpAddresses(client.genqlient, response.Organization.Id, cidrs)

	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(response.Organization.Id)
	d.Set("uuid", response.Organization.Uuid)
	d.Set("name", response.Organization.Name)
	d.Set("allowed_api_ip_addresses", strings.Split(apiResponse.OrganizationApiIpAllowlistUpdate.Organization.AllowedApiIpAddresses, " "))

	return diags
}

// ReadOrganizationSettings retrieves a Buildkite organization
func ReadOrganizationSettings(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
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
	d.Set("allowed_api_ip_addresses", strings.Split(response.Organization.AllowedApiIpAddresses, " "))


	return diags
}
