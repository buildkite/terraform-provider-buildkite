package buildkite

import (
	"context"
	"fmt"
	"log"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

type organizationBannerResourceModel struct {
	ID      types.String `tfsdk:"id"`
	UUID    types.String `tfsdk:"uuid"`
	Message types.String `tfsdk:"message"`
}

type organizationBannerResource struct {
	client *Client
}

func newOrganizationBannerResource() resource.Resource {
	return &organizationBannerResource{}
}

func (organizationBannerResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_banner"
}

func (ob *organizationBannerResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	ob.client = req.ProviderData.(*Client)
}

func (organizationBannerResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resource_schema.Schema{
		MarkdownDescription: heredoc.Doc(`
		This resource allows you to create and manage banners for specific organizations, displayed to all members at the top of each page in Buildkite's UI.

		More information on organization/system banners can be found in the [documentation](https://buildkite.com/docs/team-management/system-banners).
	`),
		Attributes: map[string]resource_schema.Attribute{
			"id": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The GraphQL ID of the organization banner. ",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"uuid": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the organization banner. ",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"message": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The organization banner's message. ",
			},
		},
	}
}

func (ob *organizationBannerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, state organizationBannerResourceModel

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := ob.client.timeouts.Create(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var r *upsertBannerResponse
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		org, err := ob.client.GetOrganizationID()
		if err == nil {
			log.Printf("Creating organization banner ...")
			r, err = upsertBanner(ctx,
				ob.client.genqlient,
				*org,
				plan.Message.ValueString(),
			)
		}

		return retryContextError(err)
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create organization banner",
			fmt.Sprintf("Unable to create organization banner %s", err.Error()),
		)
		return
	}

	state.ID = types.StringValue(r.OrganizationBannerUpsert.Banner.Id)
	state.UUID = types.StringValue(r.OrganizationBannerUpsert.Banner.Uuid)
	state.Message = types.StringValue(r.OrganizationBannerUpsert.Banner.Message)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (ob *organizationBannerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state organizationBannerResourceModel

	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := ob.client.timeouts.Read(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var r *getOrganiztionBannerResponse
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var err error

		log.Printf("Getting organization banner %s ...", state.ID.ValueString())
		r, err = getOrganiztionBanner(ctx,
			ob.client.genqlient,
			ob.client.organization,
		)

		return retryContextError(err)
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read organization banner",
			fmt.Sprintf("Unable to read organization banner: %s", err.Error()),
		)
		return
	}

	log.Printf("Found organization banner %s", state.ID.ValueString())
	// Update organizationBannerResourceModel with the first element in the response Edges (only one banner can exist)
	updateOrganizationBannerResource(r.Organization.Banners.Edges[0].Node, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (ob *organizationBannerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (ob *organizationBannerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state organizationBannerResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := ob.client.timeouts.Update(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var r *upsertBannerResponse
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		org, err := ob.client.GetOrganizationID()
		if err == nil {
			log.Printf("Updating organization banner %s ...", state.ID.ValueString())
			r, err = upsertBanner(ctx,
				ob.client.genqlient,
				*org,
				plan.Message.ValueString(),
			)
		}

		return retryContextError(err)
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to update organization banner",
			fmt.Sprintf("Unable to update organization banner %s", err.Error()),
		)
		return
	}

	state.Message = types.StringValue(r.OrganizationBannerUpsert.Banner.Message)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (ob *organizationBannerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state organizationBannerResourceModel

	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := ob.client.timeouts.Delete(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		org, err := ob.client.GetOrganizationID()
		if err == nil {
			log.Printf("Deleting organization banner %s ...", state.ID.ValueString())
			_, err = deleteBanner(ctx,
				ob.client.genqlient,
				*org,
			)
		}

		return retryContextError(err)
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete organization banner",
			fmt.Sprintf("Unable to delete organization banner %s", err.Error()),
		)
		return
	}
}

func updateOrganizationBannerResource(obn getOrganiztionBannerOrganizationBannersOrganizationBannerConnectionEdgesOrganizationBannerEdgeNodeOrganizationBanner, ob *organizationBannerResourceModel) {
	ob.ID = types.StringValue(obn.Id)
	ob.UUID = types.StringValue(obn.Uuid)
	ob.Message = types.StringValue(obn.Message)
}
