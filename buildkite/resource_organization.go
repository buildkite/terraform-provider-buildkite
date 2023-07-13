package buildkite

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type OrganizationResourceModel struct {
	AllowedApiIpAddresses types.List   `tfsdk:"allowed_api_ip_addresses"`
	UUID                  types.String `tfsdk:"uuid"`
}

type OrganizationResource struct {
	client *Client
}

func NewOrganizationResource() resource.Resource {
	return &OrganizationResource{}
}

func (OrganizationResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization"
}

func (o *OrganizationResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	o.client = req.ProviderData.(*Client)
}

func (*OrganizationResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resource_schema.Schema{
		Attributes: map[string]resource_schema.Attribute{
			"uuid": resource_schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"allowed_api_ip_addresses": resource_schema.ListAttribute{
				Optional: true,
				ElementType: types.StringType,
			},
		},
	}
}

func (o *OrganizationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
}

func (o *OrganizationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
}

func (o *OrganizationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
}

func (o *OrganizationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
}

func (o *OrganizationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
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
