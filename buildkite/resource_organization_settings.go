package buildkite

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

var deprecationMessage = `This resource has been deprecated in favour of the newer buildkite_organization resource. 
Please visit the provider's documentation at https://registry.terraform.io/providers/buildkite/buildkite/latest/docs for more details.`

func resourceOrganizationSettings() *schema.Resource {
	return &schema.Resource{
		CreateContext: CreateUpdateDeleteOrganizationSettings,
		ReadContext:   ReadOrganizationSettings,
		UpdateContext: CreateUpdateDeleteOrganizationSettings,
		DeleteContext: DeleteOrganizationSettings,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},
		DeprecationMessage: deprecationMessage,
		Schema: map[string]*schema.Schema{
			"uuid": {
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

	allowedIpAddresses := d.Get("allowed_api_ip_addresses").([]interface{})
	cidrs := make([]string, len(allowedIpAddresses))
	for i, v := range allowedIpAddresses {
		cidrs[i] = v.(string)
	}

	apiResponse, err := setApiIpAddresses(client.genqlient, response.Organization.Id, strings.Join(cidrs, " "))

	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(response.Organization.Id)
	d.Set("uuid", response.Organization.Uuid)
	d.Set("allowed_api_ip_addresses", strings.Split(apiResponse.OrganizationApiIpAllowlistUpdate.Organization.AllowedApiIpAddresses, " "))

	return diags
}

// DeleteOrganizationSettings is used for the deleting of a Buildkite organization's settings
func DeleteOrganizationSettings(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	client := m.(*Client)

	response, err := getOrganization(client.genqlient, client.organization)

	if err != nil {
		return diag.FromErr(err)
	}

	if response.Organization.Id == "" {
		return diag.FromErr(fmt.Errorf("organization not found: '%s'", client.organization))
	}

	_, err = setApiIpAddresses(client.genqlient, response.Organization.Id, "")

	if err != nil {
		return diag.FromErr(err)
	}

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
	d.Set("allowed_api_ip_addresses", strings.Split(response.Organization.AllowedApiIpAddresses, " "))

	return diags
}
