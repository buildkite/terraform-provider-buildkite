package buildkite

import (
	"context"
	"errors"
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
	RepositoryUrl types.String `tfsdk:"repository_url"`
	WebhookUrl    types.String `tfsdk:"webhook_url"`
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
			"repository_url": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The repository URL the webhook is configured for.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"webhook_url": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The Buildkite webhook URL that receives events from the repository.",
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
				if pipeline, ok := readResp.GetNode().(*getPipelineWebhookNodePipeline); ok {
					info, err := extractWebhookFromPipeline(pipeline)
					if err != nil {
						return retry.NonRetryableError(err)
					}
					state.Id = types.StringValue(info.ExternalId)
					state.RepositoryUrl = types.StringValue(info.RepositoryUrl)
					state.WebhookUrl = types.StringValue(info.Url)
				} else {
					return retry.NonRetryableError(fmt.Errorf("unable to read existing webhook for pipeline"))
				}
				return nil
			}
			return retryContextError(err)
		}

		webhook := apiResponse.PipelineCreateWebhook.Webhook
		if webhook != nil && webhook.GetExternalId() != "" {
			state.Id = types.StringValue(webhook.GetExternalId())
			state.RepositoryUrl = types.StringValue(apiResponse.PipelineCreateWebhook.Pipeline.Repository.Url)
			state.WebhookUrl = types.StringValue(webhook.GetUrl())
		} else {
			return retry.NonRetryableError(fmt.Errorf("unable to read existing webhook for pipeline"))
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
		resp.Diagnostics.AddWarning(
			"Unable to read pipeline webhook",
			fmt.Sprintf("Removing webhook from state: %s", err.Error()),
		)
		resp.State.RemoveResource(ctx)
		return
	}

	pipeline, ok := apiResponse.GetNode().(*getPipelineWebhookNodePipeline)
	if !ok || pipeline == nil {
		resp.Diagnostics.AddWarning(
			"Pipeline not found",
			"Removing pipeline webhook from state...",
		)
		resp.State.RemoveResource(ctx)
		return
	}

	info, err := extractWebhookFromPipeline(pipeline)
	if err != nil {
		if errors.Is(err, ErrNoWebhook) {
			resp.Diagnostics.AddWarning(
				"Pipeline webhook not found",
				"Removing pipeline webhook from state...",
			)
			resp.State.RemoveResource(ctx)
			return
		}
		// Provider changed to an unsupported type - remove from state
		resp.Diagnostics.AddWarning(
			"Pipeline repository provider changed",
			fmt.Sprintf("%s. Removing pipeline webhook from state...", err.Error()),
		)
		resp.State.RemoveResource(ctx)
		return
	}

	state.Id = types.StringValue(info.ExternalId)
	state.RepositoryUrl = types.StringValue(info.RepositoryUrl)
	state.WebhookUrl = types.StringValue(info.Url)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
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

// webhookInfo holds the extracted webhook information from a pipeline
type webhookInfo struct {
	ExternalId    string
	Url           string
	RepositoryUrl string
}

// repositoryProviderDisplayName returns a human-readable name for a repository provider typename.
func repositoryProviderDisplayName(typename string) string {
	switch typename {
	case "RepositoryProviderGithub":
		return "GitHub"
	case "RepositoryProviderGithubEnterprise":
		return "GitHub Enterprise"
	case "RepositoryProviderGitlab":
		return "GitLab"
	case "RepositoryProviderGitlabCommunity":
		return "GitLab Community"
	case "RepositoryProviderGitlabEnterprise":
		return "GitLab Enterprise"
	case "RepositoryProviderBitbucket":
		return "Bitbucket"
	case "RepositoryProviderBitbucketServer":
		return "Bitbucket Server"
	case "RepositoryProviderBeanstalk":
		return "Beanstalk"
	case "RepositoryProviderCodebase":
		return "Codebase"
	case "RepositoryProviderUnknown":
		return "Unknown"
	default:
		return typename
	}
}

// ErrNoWebhook is returned when a pipeline has no webhook configured.
var ErrNoWebhook = errors.New("no webhook configured")

// extractWebhookFromPipeline extracts webhook information from a pipeline response.
// Returns ErrNoWebhook if no webhook exists, or an error if the provider is unsupported.
func extractWebhookFromPipeline(pipeline *getPipelineWebhookNodePipeline) (*webhookInfo, error) {
	if pipeline == nil {
		return nil, ErrNoWebhook
	}

	repositoryUrl := pipeline.Repository.Url
	provider := pipeline.Repository.Provider

	switch p := provider.(type) {
	case *getPipelineWebhookNodePipelineRepositoryProviderRepositoryProviderGithub:
		webhook := p.Webhook
		if webhook.GetExternalId() == "" {
			return nil, ErrNoWebhook
		}
		return &webhookInfo{
			ExternalId:    webhook.GetExternalId(),
			Url:           webhook.GetUrl(),
			RepositoryUrl: repositoryUrl,
		}, nil
	case *getPipelineWebhookNodePipelineRepositoryProviderRepositoryProviderGithubEnterprise:
		webhook := p.Webhook
		if webhook.GetExternalId() == "" {
			return nil, ErrNoWebhook
		}
		return &webhookInfo{
			ExternalId:    webhook.GetExternalId(),
			Url:           webhook.GetUrl(),
			RepositoryUrl: repositoryUrl,
		}, nil
	default:
		providerName := "unknown"
		if provider != nil {
			providerName = repositoryProviderDisplayName(provider.GetTypename())
		}
		return nil, fmt.Errorf("webhooks are not supported for repository provider %q", providerName)
	}
}
