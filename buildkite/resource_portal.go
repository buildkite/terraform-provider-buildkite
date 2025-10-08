package buildkite

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type portalResourceModel struct {
	UUID               types.String `tfsdk:"uuid"`
	Slug               types.String `tfsdk:"slug"`
	Name               types.String `tfsdk:"name"`
	Description        types.String `tfsdk:"description"`
	Query              types.String `tfsdk:"query"`
	AllowedIPAddresses types.String `tfsdk:"allowed_ip_addresses"`
	UserInvokable      types.Bool   `tfsdk:"user_invokable"`
	Token              types.String `tfsdk:"token"`
	CreatedAt          types.String `tfsdk:"created_at"`
	CreatedBy          types.Object `tfsdk:"created_by"`
}

type portalAPIResponse struct {
	UUID               string  `json:"uuid"`
	Slug               string  `json:"slug"`
	OrganizationUUID   string  `json:"organization_uuid"`
	Name               string  `json:"name"`
	Description        *string `json:"description"`
	Query              string  `json:"query"`
	AllowedIPAddresses *string `json:"allowed_ip_addresses"`
	UserInvokable      bool    `json:"user_invokable"`
	Token              *string `json:"token,omitempty"`
	CreatedAt          string  `json:"created_at"`
	CreatedBy          *struct {
		UUID  string `json:"uuid"`
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"created_by"`
}

type portalCreateUpdateRequest struct {
	Name               string  `json:"name"`
	Slug               string  `json:"slug"`
	Description        *string `json:"description,omitempty"`
	Query              string  `json:"query"`
	AllowedIPAddresses *string `json:"allowed_ip_addresses,omitempty"`
	UserInvokable      bool    `json:"user_invokable"`
}

type portalResource struct {
	client *Client
}

func newPortalResource() resource.Resource {
	return &portalResource{}
}

func (p *portalResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_portal"
}

func (p *portalResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	p.client = req.ProviderData.(*Client)
}

func (p *portalResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resource_schema.Schema{
		MarkdownDescription: heredoc.Doc(`
			This resource allows you to manage portals in Buildkite. Portals allow you to expose GraphQL queries
			that can be invoked via a REST API endpoint. Find out more information in our
			[documentation](https://buildkite.com/docs/apis/portals).
		`),
		Attributes: map[string]resource_schema.Attribute{
			"uuid": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the portal.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"slug": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The slug of the portal. Used in the portal's URL path.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the portal.",
			},
			"description": resource_schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "A description of the portal.",
			},
			"query": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The GraphQL query that the portal executes.",
			},
			"allowed_ip_addresses": resource_schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Space-delimited list of IP addresses (in CIDR notation) allowed to invoke this portal. If not specified, all IP addresses are allowed.",
			},
			"user_invokable": resource_schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Whether users can invoke the portal. Defaults to false.",
			},
			"token": resource_schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "The token used to invoke the portal. Only returned on creation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"created_at": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The time when the portal was created.",
			},
			"created_by": resource_schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Information about the user who created the portal.",
				Attributes: map[string]resource_schema.Attribute{
					"uuid": resource_schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "The UUID of the user.",
					},
					"name": resource_schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "The name of the user.",
					},
					"email": resource_schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "The email of the user.",
					},
				},
			},
		},
	}
}

func (p *portalResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan portalResourceModel

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := p.createPortal(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create portal",
			fmt.Sprintf("Unable to create portal: %s", err.Error()),
		)
		return
	}

	plan.UUID = types.StringValue(result.UUID)
	plan.Slug = types.StringValue(result.Slug)
	plan.Name = types.StringValue(result.Name)
	plan.Query = types.StringValue(result.Query)
	plan.UserInvokable = types.BoolValue(result.UserInvokable)
	plan.CreatedAt = types.StringValue(result.CreatedAt)

	if result.Description != nil {
		plan.Description = types.StringValue(*result.Description)
	} else {
		plan.Description = types.StringNull()
	}

	if result.Token != nil {
		plan.Token = types.StringValue(*result.Token)
	}

	if result.AllowedIPAddresses != nil {
		plan.AllowedIPAddresses = types.StringValue(*result.AllowedIPAddresses)
	} else {
		plan.AllowedIPAddresses = types.StringNull()
	}

	if result.CreatedBy != nil {
		createdByMap := map[string]attr.Value{
			"uuid":  types.StringValue(result.CreatedBy.UUID),
			"name":  types.StringValue(result.CreatedBy.Name),
			"email": types.StringValue(result.CreatedBy.Email),
		}
		createdByObj, d := types.ObjectValue(map[string]attr.Type{
			"uuid":  types.StringType,
			"name":  types.StringType,
			"email": types.StringType,
		}, createdByMap)
		resp.Diagnostics.Append(d...)
		plan.CreatedBy = createdByObj
	} else {
		plan.CreatedBy = types.ObjectNull(map[string]attr.Type{
			"uuid":  types.StringType,
			"name":  types.StringType,
			"email": types.StringType,
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (p *portalResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state portalResourceModel

	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := p.getPortal(ctx, state.Slug.ValueString())
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			resp.Diagnostics.AddWarning(
				"Portal not found",
				"Removing portal from state...",
			)
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Unable to read portal",
			fmt.Sprintf("Unable to read portal: %s", err.Error()),
		)
		return
	}

	state.UUID = types.StringValue(result.UUID)
	state.Slug = types.StringValue(result.Slug)
	state.Name = types.StringValue(result.Name)
	state.Query = types.StringValue(result.Query)
	state.UserInvokable = types.BoolValue(result.UserInvokable)
	state.CreatedAt = types.StringValue(result.CreatedAt)

	if result.Description != nil {
		state.Description = types.StringValue(*result.Description)
	} else {
		state.Description = types.StringNull()
	}

	if result.Token != nil {
		state.Token = types.StringValue(*result.Token)
	}

	if result.AllowedIPAddresses != nil {
		state.AllowedIPAddresses = types.StringValue(*result.AllowedIPAddresses)
	} else {
		state.AllowedIPAddresses = types.StringNull()
	}

	if result.CreatedBy != nil {
		createdByMap := map[string]attr.Value{
			"uuid":  types.StringValue(result.CreatedBy.UUID),
			"name":  types.StringValue(result.CreatedBy.Name),
			"email": types.StringValue(result.CreatedBy.Email),
		}
		createdByObj, d := types.ObjectValue(map[string]attr.Type{
			"uuid":  types.StringType,
			"name":  types.StringType,
			"email": types.StringType,
		}, createdByMap)
		resp.Diagnostics.Append(d...)
		state.CreatedBy = createdByObj
	} else {
		state.CreatedBy = types.ObjectNull(map[string]attr.Type{
			"uuid":  types.StringType,
			"name":  types.StringType,
			"email": types.StringType,
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (p *portalResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan portalResourceModel

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := p.updatePortal(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to update portal",
			fmt.Sprintf("Unable to update portal: %s", err.Error()),
		)
		return
	}

	plan.UUID = types.StringValue(result.UUID)
	plan.Slug = types.StringValue(result.Slug)
	plan.Name = types.StringValue(result.Name)
	plan.Query = types.StringValue(result.Query)
	plan.UserInvokable = types.BoolValue(result.UserInvokable)
	plan.CreatedAt = types.StringValue(result.CreatedAt)

	if result.Description != nil {
		plan.Description = types.StringValue(*result.Description)
	} else {
		plan.Description = types.StringNull()
	}

	if result.Token != nil {
		plan.Token = types.StringValue(*result.Token)
	}

	if result.AllowedIPAddresses != nil {
		plan.AllowedIPAddresses = types.StringValue(*result.AllowedIPAddresses)
	} else {
		plan.AllowedIPAddresses = types.StringNull()
	}

	if result.CreatedBy != nil {
		createdByMap := map[string]attr.Value{
			"uuid":  types.StringValue(result.CreatedBy.UUID),
			"name":  types.StringValue(result.CreatedBy.Name),
			"email": types.StringValue(result.CreatedBy.Email),
		}
		createdByObj, d := types.ObjectValue(map[string]attr.Type{
			"uuid":  types.StringType,
			"name":  types.StringType,
			"email": types.StringType,
		}, createdByMap)
		resp.Diagnostics.Append(d...)
		plan.CreatedBy = createdByObj
	} else {
		plan.CreatedBy = types.ObjectNull(map[string]attr.Type{
			"uuid":  types.StringType,
			"name":  types.StringType,
			"email": types.StringType,
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (p *portalResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state portalResourceModel

	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := p.deletePortal(ctx, state.Slug.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete portal",
			fmt.Sprintf("Unable to delete portal: %s", err.Error()),
		)
		return
	}
}

func (p *portalResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("slug"), req.ID)...)
}

func (p *portalResource) createPortal(ctx context.Context, plan *portalResourceModel) (*portalAPIResponse, error) {
	path := fmt.Sprintf("/v2/organizations/%s/portals", p.client.organization)

	reqBody := portalCreateUpdateRequest{
		Name:          plan.Name.ValueString(),
		Slug:          plan.Slug.ValueString(),
		Query:         plan.Query.ValueString(),
		UserInvokable: plan.UserInvokable.ValueBool(),
	}

	if !plan.Description.IsNull() {
		desc := plan.Description.ValueString()
		reqBody.Description = &desc
	}

	if !plan.AllowedIPAddresses.IsNull() {
		ipString := plan.AllowedIPAddresses.ValueString()
		reqBody.AllowedIPAddresses = &ipString
	}

	var result portalAPIResponse
	err := p.client.makeRequest(ctx, http.MethodPost, path, reqBody, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func (p *portalResource) getPortal(ctx context.Context, slug string) (*portalAPIResponse, error) {
	path := fmt.Sprintf("/v2/organizations/%s/portals/%s", p.client.organization, slug)

	var result portalAPIResponse
	err := p.client.makeRequest(ctx, http.MethodGet, path, nil, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func (p *portalResource) updatePortal(ctx context.Context, plan *portalResourceModel) (*portalAPIResponse, error) {
	path := fmt.Sprintf("/v2/organizations/%s/portals/%s", p.client.organization, plan.Slug.ValueString())

	reqBody := portalCreateUpdateRequest{
		Name:          plan.Name.ValueString(),
		Slug:          plan.Slug.ValueString(),
		Query:         plan.Query.ValueString(),
		UserInvokable: plan.UserInvokable.ValueBool(),
	}

	if !plan.Description.IsNull() {
		desc := plan.Description.ValueString()
		reqBody.Description = &desc
	}

	if !plan.AllowedIPAddresses.IsNull() {
		ipString := plan.AllowedIPAddresses.ValueString()
		reqBody.AllowedIPAddresses = &ipString
	}

	var result portalAPIResponse
	err := p.client.makeRequest(ctx, http.MethodPut, path, reqBody, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func (p *portalResource) deletePortal(ctx context.Context, slug string) error {
	path := fmt.Sprintf("/v2/organizations/%s/portals/%s", p.client.organization, slug)

	err := p.client.makeRequest(ctx, http.MethodDelete, path, nil, nil)
	if err != nil {
		return err
	}

	return nil
}
