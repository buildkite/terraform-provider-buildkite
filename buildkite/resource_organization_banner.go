package buildkite

import (
	"context"
	//"fmt"
	//"log"
	//"strings"

	"github.com/MakeNowJust/heredoc"
	//"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	//"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

type organizationBannerResourceModel struct {
	ID          types.String `tfsdk:"id"`
	UUID        types.String `tfsdk:"uuid"`
	Message     types.String `tfsdk:"message"`

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
	
}

func (ob *organizationBannerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	
}

func (ob *organizationBannerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {

}

func (ob *organizationBannerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {

}

func (ob *organizationBannerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {

}