package buildkite

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

// defaultHostedAgentImageBuildTimeout bounds the wait for an image to build when the resource's
// timeouts block does not set create. Docker builds routinely outlast the provider's 3 minute default.
const defaultHostedAgentImageBuildTimeout = 30 * time.Minute

type hostedAgentImageResource struct {
	client *Client
}

type hostedAgentImageResourceModel struct {
	ID         types.String   `tfsdk:"id"`
	ClusterID  types.String   `tfsdk:"cluster_id"`
	Name       types.String   `tfsdk:"name"`
	Dockerfile types.String   `tfsdk:"dockerfile"`
	Status     types.String   `tfsdk:"status"`
	ImageRef   types.String   `tfsdk:"image_ref"`
	Timeouts   timeouts.Value `tfsdk:"timeouts"`
}

// hostedAgentImage is an agent image as the REST API returns it. dockerfile is the body the image was
// created with, without the managed base image FROM line the API prepends.
type hostedAgentImage struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Status         string `json:"status"`
	ImageRef       string `json:"image_ref"`
	LastBuildError string `json:"last_build_error"`
	Dockerfile     string `json:"dockerfile"`
}

func newHostedAgentImageResource() resource.Resource {
	return &hostedAgentImageResource{}
}

func (r *hostedAgentImageResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_hosted_agent_image"
}

func (r *hostedAgentImageResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(*Client)
}

func (r *hostedAgentImageResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: heredoc.Doc(`
			A custom agent image for Linux hosted agents in a cluster. Buildkite builds the image from a
			Dockerfile body on top of its managed hosted agent base image. Reference the built image from a
			hosted cluster queue with ` + "`hosted_agents.linux.agent_image_ref = buildkite_hosted_agent_image.<name>.image_ref`" + `.

			A cluster gets its hosted agents environment when its first hosted queue is created, so the cluster
			must already have a hosted queue before an image can be created in it. Use ` + "`depends_on`" + ` when that
			queue is managed in the same configuration.

			Creating an image waits for its build to finish. If the build fails, the build error is reported and
			the image is replaced on the next apply. Images cannot be changed after they are created, so changing
			any argument replaces the image. An image used by a queue cannot be deleted until those queues use a
			different image.
		`),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The ID of the agent image.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"cluster_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The UUID of the cluster the image belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the agent image. Must be unique within the cluster.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"dockerfile": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The Dockerfile instructions to build the image with. Buildkite provides the base image, so this must not contain a `FROM` instruction.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The build status of the image: `BUILDING`, `READY` or `FAILED`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"image_ref": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The reference of the built image, for a hosted queue's `hosted_agents.linux.agent_image_ref`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
		Blocks: map[string]schema.Block{
			"timeouts": timeouts.Block(ctx, timeouts.Opts{
				Create:            true,
				CreateDescription: "How long to wait for the image to build. Defaults to 30 minutes.",
			}),
		},
	}
}

func (r *hostedAgentImageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan hostedAgentImageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := r.client.createTimeout(ctx)
	resp.Diagnostics.Append(diags...)
	buildTimeout, diags := plan.Timeouts.Create(ctx, defaultHostedAgentImageBuildTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	requestCtx, cancel := mutationContext(ctx, timeout)
	defer cancel()

	clusterID := plan.ClusterID.ValueString()
	var image *hostedAgentImage
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var err error
		image, err = r.client.createHostedAgentImage(requestCtx, clusterID, plan.Name.ValueString(), plan.Dockerfile.ValueString())
		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to create hosted agent image", fmt.Sprintf("Unable to create hosted agent image: %s", err))
		return
	}

	// Record the image before waiting for its build, so one that fails or outlasts the wait is tainted
	// and replaced on the next apply rather than left behind holding its name.
	plan.ID = types.StringValue(image.ID)
	setHostedAgentImageModel(&plan, image)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	waitCtx, cancelWait := context.WithTimeout(ctx, buildTimeout)
	defer cancelWait()

	err = retry.RetryContext(waitCtx, buildTimeout, func() *retry.RetryError {
		current, err := r.client.getHostedAgentImage(waitCtx, clusterID, image.ID)
		if err != nil {
			return retryContextError(err)
		}
		image = current

		switch image.Status {
		case "READY":
			return nil
		case "FAILED":
			return retry.NonRetryableError(fmt.Errorf("the image build failed: %s", image.LastBuildError))
		default:
			return retry.RetryableError(fmt.Errorf("the image is still %s", image.Status))
		}
	})

	setHostedAgentImageModel(&plan, image)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if err != nil {
		resp.Diagnostics.AddError("Hosted agent image did not build", fmt.Sprintf("Hosted agent image %s did not build: %s", image.ID, err))
	}
}

func (r *hostedAgentImageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state hostedAgentImageResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := r.client.readTimeout(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var image *hostedAgentImage
	err := retry.RetryContext(requestCtx, timeout, func() *retry.RetryError {
		var err error
		image, err = r.client.getHostedAgentImage(requestCtx, state.ClusterID.ValueString(), state.ID.ValueString())
		return retryContextError(err)
	})
	if err != nil {
		if isAPIStatus(err, http.StatusNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read hosted agent image", fmt.Sprintf("Unable to read hosted agent image: %s", err))
		return
	}

	setHostedAgentImageModel(&state, image)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update only applies a change to the timeouts block: every other argument replaces the image.
func (r *hostedAgentImageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan hostedAgentImageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *hostedAgentImageResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state hostedAgentImageResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := r.client.deleteTimeout(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	requestCtx, cancel := mutationContext(ctx, timeout)
	defer cancel()

	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		return retryContextError(r.client.deleteHostedAgentImage(requestCtx, state.ClusterID.ValueString(), state.ID.ValueString()))
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to delete hosted agent image", fmt.Sprintf("Unable to delete hosted agent image: %s", err))
	}
}

func (r *hostedAgentImageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	clusterID, imageID, ok := strings.Cut(req.ID, "/")
	if !ok || clusterID == "" || imageID == "" || strings.Contains(imageID, "/") {
		resp.Diagnostics.AddError(
			"Invalid import ID format",
			fmt.Sprintf("Expected format: cluster_id/image_id, got: %s", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), clusterID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), imageID)...)
}

func setHostedAgentImageModel(model *hostedAgentImageResourceModel, image *hostedAgentImage) {
	model.Name = types.StringValue(image.Name)
	model.Dockerfile = types.StringValue(image.Dockerfile)
	model.Status = types.StringValue(image.Status)
	model.ImageRef = types.StringValue(image.ImageRef)
}

func (c *Client) hostedAgentImagePath(clusterID, imageID string) string {
	path := fmt.Sprintf("/v2/organizations/%s/clusters/%s/agent-images", c.organization, clusterID)
	if imageID != "" {
		path += "/" + imageID
	}
	return path
}

func (c *Client) createHostedAgentImage(ctx context.Context, clusterID, name, dockerfile string) (*hostedAgentImage, error) {
	var image hostedAgentImage
	payload := map[string]string{"name": name, "dockerfile": dockerfile}
	if err := c.makeRequest(ctx, http.MethodPost, c.hostedAgentImagePath(clusterID, ""), payload, &image); err != nil {
		return nil, err
	}
	return &image, nil
}

func (c *Client) getHostedAgentImage(ctx context.Context, clusterID, imageID string) (*hostedAgentImage, error) {
	var image hostedAgentImage
	if err := c.makeRequest(ctx, http.MethodGet, c.hostedAgentImagePath(clusterID, imageID), nil, &image); err != nil {
		return nil, err
	}
	return &image, nil
}

func (c *Client) deleteHostedAgentImage(ctx context.Context, clusterID, imageID string) error {
	return c.makeRequest(ctx, http.MethodDelete, c.hostedAgentImagePath(clusterID, imageID), nil, nil)
}
