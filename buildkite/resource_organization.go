package buildkite

import (
	"context"
	"fmt"
	"strings"
	
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type OrganizationResourceModel struct {
	AllowedApiIpAddresses types.List   `tfsdk:"allowed_api_ip_addresses"`
	ID                    types.String `tfsdk:"id"`
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
			"id": resource_schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"uuid": resource_schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"allowed_api_ip_addresses": resource_schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (o *OrganizationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, state OrganizationResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Get Organization
	response, err := getOrganization(o.client.genqlient, o.client.organization)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to obtain Organization",
			fmt.Sprintf("Unable to obtain Organization: %s", err.Error()),
		)
		return
	}

	if response.Organization.Id == "" {
		resp.Diagnostics.AddError(
			"Organization not found",
			fmt.Sprintf("Organization not found: %s", err.Error()),
		)
		return
	}

	allowedIpAddresses := plan.AllowedApiIpAddresses
	cidrs := make([]string, len(allowedIpAddresses.Elements()))
	for i, v := range allowedIpAddresses.Elements() {
		cidrs[i] = strings.Trim(v.String(), "\"")
	}

	apiResponse, err := setApiIpAddresses(
		o.client.genqlient,
		o.client.organizationId,
		strings.Join(cidrs, " "),
	)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create Organization settings",
			fmt.Sprintf("Unable to create Organization settings: %s", err.Error()),
		)
		return
	}

	state.ID = types.StringValue(response.Organization.Id)
	state.UUID = types.StringValue(response.Organization.Uuid)
	ips, diag := types.ListValueFrom(ctx, types.StringType, strings.Split(apiResponse.OrganizationApiIpAllowlistUpdate.Organization.AllowedApiIpAddresses, " "))
	state.AllowedApiIpAddresses = ips

	if diag.HasError() {
		resp.Diagnostics.Append(diag...)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (o *OrganizationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state OrganizationResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Get Organization
	response, err := getOrganization(o.client.genqlient, o.client.organization)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to obtain Organization",
			fmt.Sprintf("Unable to obtain Organization: %s", err.Error()),
		)
		return
	}

	if response.Organization.Id == "" {
		resp.Diagnostics.AddError(
			"Organization not found",
			fmt.Sprintf("Organization not found: %s", err.Error()),
		)
		return
	}

	state.ID = types.StringValue(response.Organization.Id)
	state.UUID = types.StringValue(response.Organization.Uuid)
	ips, diag := types.ListValueFrom(ctx, types.StringType, strings.Split(response.Organization.AllowedApiIpAddresses, " "))
	state.AllowedApiIpAddresses = ips

	if diag.HasError() {
		resp.Diagnostics.Append(diag...)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (o *OrganizationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
    resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (o *OrganizationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state OrganizationResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Get Organization
	response, err := getOrganization(o.client.genqlient, o.client.organization)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to obtain Organization",
			fmt.Sprintf("Unable to obtain Organization: %s", err.Error()),
		)
		return
	}

	if response.Organization.Id == "" {
		resp.Diagnostics.AddError(
			"Organization not found",
			fmt.Sprintf("Organization not found: %s", err.Error()),
		)
		return
	}

	allowedIpAddresses := plan.AllowedApiIpAddresses
	cidrs := make([]string, len(allowedIpAddresses.Elements()))
	for i, v := range allowedIpAddresses.Elements() {
		cidrs[i] = strings.Trim(v.String(), "\"")
	}

	apiResponse, err := setApiIpAddresses(
		o.client.genqlient,
		o.client.organizationId,
		strings.Join(cidrs, " "),
	)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to update Organization settings",
			fmt.Sprintf("Unable to update Organization settings: %s", err.Error()),
		)
		return
	}

	state.ID = types.StringValue(response.Organization.Id)
	state.UUID = types.StringValue(response.Organization.Uuid)
	ips, diag := types.ListValueFrom(ctx, types.StringType, strings.Split(apiResponse.OrganizationApiIpAllowlistUpdate.Organization.AllowedApiIpAddresses, " "))
	state.AllowedApiIpAddresses = ips

	if diag.HasError() {
		resp.Diagnostics.Append(diag...)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (o *OrganizationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	response, err := getOrganization(o.client.genqlient, o.client.organization)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to obtain Organization",
			fmt.Sprintf("Unable to obtain Organization: %s", err.Error()),
		)
		return
	}

	if response.Organization.Id == "" {
		resp.Diagnostics.AddError(
			"Organization not found",
			fmt.Sprintf("Organization not found: %s", err.Error()),
		)
		return
	}
	_, err = setApiIpAddresses(o.client.genqlient, response.Organization.Id, "")

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete Organization settings",
			fmt.Sprintf("Unable to delete Organization settings: %s", err.Error()),
		)
		return
	}
}
