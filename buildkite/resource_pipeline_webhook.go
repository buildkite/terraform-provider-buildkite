package buildkite

import (
	"context"
	"fmt"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

type pipelineWebhook struct {
	client *Client
}

type pipelineWebhookResourceModel struct {
	Id            types.String `tfsdk:"id"`
	PipelineId    types.String `tfsdk:"pipeline_id"`
	Provider      types.String `tfsdk:"provider_name"`
	RepositoryUrl types.String `tfsdk:"repository_url"`
}

func newPipelineWebhookResource() resource.Resource {
	return &pipelineWebhook{}
}

func (pw *pipelineWebhook) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pipeline_webhook"
}

func (pw *pipelineWebhook) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pw.client = req.ProviderData.(*Client)
}

func (pw *pipelineWebhook) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resource_schema.Schema{
		MarkdownDescription: heredoc.Doc(`
			This resource manages a webhook for a Buildkite pipeline's source repository.

			The webhook enables automatic build triggering when changes are pushed to the repository.
			Only one webhook can exist per pipeline - if a webhook already exists, it will be adopted into state.
		`),
		Attributes: map[string]resource_schema.Attribute{
			"id": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The ID of the webhook.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"pipeline_id": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The GraphQL ID of the pipeline.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"provider_name": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The SCM provider for the webhook (e.g., `github`, `github_enterprise`).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"repository_url": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The repository URL the webhook is configured for.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (pw *pipelineWebhook) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pipelineWebhookResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeouts, diags := pw.client.timeouts.Create(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state pipelineWebhookResourceModel
	state.PipelineId = plan.PipelineId

	err := retry.RetryContext(ctx, timeouts, func() *retry.RetryError {
		apiResponse, err := createPipelineWebhook(ctx, pw.client.genqlient, plan.PipelineId.ValueString())
		if err != nil {
			if strings.Contains(err.Error(), "A webhook already exists for this repository") {
				readResp, readErr := getPipelineWebhook(ctx, pw.client.genqlient, plan.PipelineId.ValueString())
				if readErr != nil {
					return retry.NonRetryableError(fmt.Errorf("webhook exists but failed to read: %w", readErr))
				}
				if pipeline, ok := readResp.GetNode().(*getPipelineWebhookNodePipeline); ok && pipeline != nil && pipeline.RepositoryWebhook.Id != "" {
					state.Id = types.StringValue(pipeline.RepositoryWebhook.Id)
					state.Provider = types.StringValue(pipeline.RepositoryWebhook.Provider)
					state.RepositoryUrl = types.StringValue(pipeline.RepositoryWebhook.RepositoryUrl)
				}
				return nil
			}
			return retryContextError(err)
		}

		if apiResponse.PipelineCreateWebhook.Webhook.Id != "" {
			state.Id = types.StringValue(apiResponse.PipelineCreateWebhook.Webhook.Id)
			state.Provider = types.StringValue(apiResponse.PipelineCreateWebhook.Webhook.Provider)
			state.RepositoryUrl = types.StringValue(apiResponse.PipelineCreateWebhook.Webhook.RepositoryUrl)
		}
		return nil
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create pipeline webhook",
			fmt.Sprintf("Unable to create pipeline webhook: %s", err.Error()),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (pw *pipelineWebhook) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pipelineWebhookResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeouts, diags := pw.client.timeouts.Read(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var apiResponse *getPipelineWebhookResponse
	err := retry.RetryContext(ctx, timeouts, func() *retry.RetryError {
		var err error
		apiResponse, err = getPipelineWebhook(ctx, pw.client.genqlient, state.PipelineId.ValueString())
		return retryContextError(err)
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read pipeline webhook",
			fmt.Sprintf("Unable to read pipeline webhook: %s", err.Error()),
		)
		return
	}

	if pipeline, ok := apiResponse.GetNode().(*getPipelineWebhookNodePipeline); ok && pipeline != nil {
		if pipeline.RepositoryWebhook.Id == "" {
			resp.Diagnostics.AddWarning(
				"Pipeline webhook not found",
				"Removing pipeline webhook from state...",
			)
			resp.State.RemoveResource(ctx)
			return
		}
		state.Id = types.StringValue(pipeline.RepositoryWebhook.Id)
		state.Provider = types.StringValue(pipeline.RepositoryWebhook.Provider)
		state.RepositoryUrl = types.StringValue(pipeline.RepositoryWebhook.RepositoryUrl)
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	} else {
		resp.Diagnostics.AddWarning(
			"Pipeline not found",
			"Removing pipeline webhook from state...",
		)
		resp.State.RemoveResource(ctx)
	}
}

func (pw *pipelineWebhook) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Webhooks have no mutable attributes, so update is a no-op
	var state pipelineWebhookResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (pw *pipelineWebhook) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pipelineWebhookResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := pw.client.timeouts.Delete(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		_, err := deletePipelineWebhook(ctx, pw.client.genqlient, state.PipelineId.ValueString())
		if err != nil && isResourceNotFoundError(err) {
			return nil
		}
		return retryContextError(err)
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete pipeline webhook",
			fmt.Sprintf("Unable to delete pipeline webhook: %s", err.Error()),
		)
		return
	}
}

func (pw *pipelineWebhook) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("pipeline_id"), req, resp)
}
