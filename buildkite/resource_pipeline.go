package buildkite

import (
	"context"
	"fmt"
	"log"
	"time"
	"unsafe"

	"github.com/buildkite/terraform-provider-buildkite/internal/boolvalidation"
	"github.com/buildkite/terraform-provider-buildkite/internal/pipelinevalidation"
	custom_modifier "github.com/buildkite/terraform-provider-buildkite/internal/planmodifier"
	"github.com/buildkite/terraform-provider-buildkite/internal/stringvalidation"
	"github.com/hashicorp/terraform-plugin-framework-validators/boolvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/shurcooL/graphql"
)

const defaultSteps = `steps:
- label: ':pipeline: Pipeline Upload'
  command: buildkite-agent pipeline upload`

// PipelineNode represents a pipeline as returned from the GraphQL API
type Cluster struct {
	ID graphql.String
}
type Repository struct {
	URL graphql.String
}
type Steps struct {
	YAML graphql.String
}
type PipelineNode struct {
	AllowRebuilds                        graphql.Boolean
	BranchConfiguration                  graphql.String
	CancelIntermediateBuilds             graphql.Boolean
	CancelIntermediateBuildsBranchFilter graphql.String
	Cluster                              Cluster
	DefaultBranch                        graphql.String
	DefaultTimeoutInMinutes              graphql.Int
	MaximumTimeoutInMinutes              graphql.Int
	Description                          graphql.String
	ID                                   graphql.String
	Name                                 graphql.String
	Repository                           Repository
	SkipIntermediateBuilds               graphql.Boolean
	SkipIntermediateBuildsBranchFilter   graphql.String
	Slug                                 graphql.String
	Steps                                Steps
	Tags                                 []PipelineTag
	WebhookURL                           graphql.String `graphql:"webhookURL"`
}

type PipelineTag struct {
	Label graphql.String
}

type pipelineResourceModel struct {
	AllowRebuilds                        types.Bool               `tfsdk:"allow_rebuilds"`
	BadgeUrl                             types.String             `tfsdk:"badge_url"`
	BranchConfiguration                  types.String             `tfsdk:"branch_configuration"`
	CancelIntermediateBuilds             types.Bool               `tfsdk:"cancel_intermediate_builds"`
	CancelIntermediateBuildsBranchFilter types.String             `tfsdk:"cancel_intermediate_builds_branch_filter"`
	ClusterId                            types.String             `tfsdk:"cluster_id"`
	DefaultBranch                        types.String             `tfsdk:"default_branch"`
	DefaultTimeoutInMinutes              types.Int64              `tfsdk:"default_timeout_in_minutes"`
	Description                          types.String             `tfsdk:"description"`
	Id                                   types.String             `tfsdk:"id"`
	MaximumTimeoutInMinutes              types.Int64              `tfsdk:"maximum_timeout_in_minutes"`
	Name                                 types.String             `tfsdk:"name"`
	ProviderSettings                     []*providerSettingsModel `tfsdk:"provider_settings"`
	Repository                           types.String             `tfsdk:"repository"`
	SkipIntermediateBuilds               types.Bool               `tfsdk:"skip_intermediate_builds"`
	SkipIntermediateBuildsBranchFilter   types.String             `tfsdk:"skip_intermediate_builds_branch_filter"`
	Slug                                 types.String             `tfsdk:"slug"`
	Steps                                types.String             `tfsdk:"steps"`
	Tags                                 []types.String           `tfsdk:"tags"`
	WebhookUrl                           types.String             `tfsdk:"webhook_url"`
}

type providerSettingsModel struct {
	TriggerMode                             types.String `tfsdk:"trigger_mode"`
	BuildPullRequests                       types.Bool   `tfsdk:"build_pull_requests"`
	PullRequestBranchFilterEnabled          types.Bool   `tfsdk:"pull_request_branch_filter_enabled"`
	PullRequestBranchFilterConfiguration    types.String `tfsdk:"pull_request_branch_filter_configuration"`
	SkipBuildsForExistingCommits            types.Bool   `tfsdk:"skip_builds_for_existing_commits"`
	SkipPullRequestBuildsForExistingCommits types.Bool   `tfsdk:"skip_pull_request_builds_for_existing_commits"`
	BuildPullRequestReadyForReview          types.Bool   `tfsdk:"build_pull_request_ready_for_review"`
	BuildPullRequestLabelsChanged           types.Bool   `tfsdk:"build_pull_request_labels_changed"`
	BuildPullRequestForks                   types.Bool   `tfsdk:"build_pull_request_forks"`
	PrefixPullRequestForkBranchNames        types.Bool   `tfsdk:"prefix_pull_request_fork_branch_names"`
	BuildBranches                           types.Bool   `tfsdk:"build_branches"`
	BuildTags                               types.Bool   `tfsdk:"build_tags"`
	CancelDeletedBranchBuilds               types.Bool   `tfsdk:"cancel_deleted_branch_builds"`
	FilterEnabled                           types.Bool   `tfsdk:"filter_enabled"`
	FilterCondition                         types.String `tfsdk:"filter_condition"`
	PublishCommitStatus                     types.Bool   `tfsdk:"publish_commit_status"`
	PublishBlockedAsPending                 types.Bool   `tfsdk:"publish_blocked_as_pending"`
	PublishCommitStatusPerStep              types.Bool   `tfsdk:"publish_commit_status_per_step"`
	SeparatePullRequestStatuses             types.Bool   `tfsdk:"separate_pull_request_statuses"`
}

type pipelineResource struct {
	client          *Client
	archiveOnDelete bool
}

type pipelineResponse interface {
	GetId() string
	GetAllowRebuilds() bool
	GetBranchConfiguration() *string
	GetCancelIntermediateBuilds() bool
	GetCancelIntermediateBuildsBranchFilter() string
	GetCluster() PipelineValuesCluster
	GetDefaultBranch() string
	GetDefaultTimeoutInMinutes() *int
	GetMaximumTimeoutInMinutes() *int
	GetDescription() string
	GetName() string
	GetRepository() PipelineValuesRepository
	GetSkipIntermediateBuilds() bool
	GetSkipIntermediateBuildsBranchFilter() string
	GetSlug() string
	GetSteps() PipelineValuesStepsPipelineSteps
	GetTags() []PipelineValuesTagsPipelineTag
	GetWebhookURL() string
}

func newPipelineResource(archiveOnDelete bool) func() resource.Resource {
	return func() resource.Resource {
		return &pipelineResource{
			archiveOnDelete: archiveOnDelete,
		}
	}
}

func (p *pipelineResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	p.client = req.ProviderData.(*Client)
}

func (p *pipelineResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, state pipelineResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// use the unsafe module to convert to an int. this is fine because the absolute max accepted by the API is much
	// less than an int
	defaultTimeoutInMinutes := (*int)(unsafe.Pointer(plan.DefaultTimeoutInMinutes.ValueInt64Pointer()))
	maxTimeoutInMinutes := (*int)(unsafe.Pointer(plan.MaximumTimeoutInMinutes.ValueInt64Pointer()))

	input := PipelineCreateInput{
		AllowRebuilds:                        plan.AllowRebuilds.ValueBool(),
		BranchConfiguration:                  plan.BranchConfiguration.ValueStringPointer(),
		CancelIntermediateBuilds:             plan.CancelIntermediateBuilds.ValueBool(),
		CancelIntermediateBuildsBranchFilter: plan.CancelIntermediateBuildsBranchFilter.ValueString(),
		ClusterId:                            plan.ClusterId.ValueStringPointer(),
		DefaultBranch:                        plan.DefaultBranch.ValueString(),
		DefaultTimeoutInMinutes:              defaultTimeoutInMinutes,
		MaximumTimeoutInMinutes:              maxTimeoutInMinutes,
		Description:                          plan.Description.ValueString(),
		Name:                                 plan.Name.ValueString(),
		OrganizationId:                       p.client.organizationId,
		Repository:                           PipelineRepositoryInput{Url: plan.Repository.ValueString()},
		SkipIntermediateBuilds:               plan.SkipIntermediateBuilds.ValueBool(),
		SkipIntermediateBuildsBranchFilter:   plan.SkipIntermediateBuildsBranchFilter.ValueString(),
		Steps:                                PipelineStepsInput{Yaml: plan.Steps.ValueString()},
		Tags:                                 getTagsFromSchema(&plan),
	}

	timeouts, diags := p.client.timeouts.Create(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var response *createPipelineResponse
	log.Printf("Creating pipeline %s ...", plan.Name.ValueString())
	err := retry.RetryContext(ctx, timeouts, func() *retry.RetryError {
		var err error
		response, err = createPipeline(ctx, p.client.genqlient, input)
		return retryContextError(err)
	})

	if err != nil {
		resp.Diagnostics.AddError("Failed to create pipeline", err.Error())
		return
	}
	log.Printf("Successfully created pipeline with id '%s'.", response.PipelineCreate.Pipeline.Id)

	setPipelineModel(&state, &response.PipelineCreate.Pipeline)

	if len(plan.ProviderSettings) > 0 {
		pipelineExtraInfo, err := updatePipelineExtraInfo(ctx, response.PipelineCreate.Pipeline.Slug, plan.ProviderSettings[0], p.client, timeouts)
		if err != nil {
			resp.Diagnostics.AddError("Unable to set pipeline info from REST", err.Error())
			return
		}

		updatePipelineResourceExtraInfo(&state, &pipelineExtraInfo)
	} else {
		// no provider_settings provided, but we still need to read in the badge url
		extraInfo, err := getPipelineExtraInfo(ctx, p.client, response.PipelineCreate.Pipeline.Slug, timeouts)
		if err != nil {
			resp.Diagnostics.AddError("Unable to read pipeline info from REST", err.Error())
			return
		}
		state.BadgeUrl = types.StringValue(extraInfo.BadgeUrl)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (p *pipelineResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pipelineResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := p.client.timeouts.Delete(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if p.archiveOnDelete {
		log.Printf("Pipeline %s set to archive on delete. Archiving...", state.Name.ValueString())

		err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
			_, err := archivePipeline(ctx, p.client.genqlient, state.Id.ValueString())
			return retryContextError(err)
		})
		if err != nil {
			resp.Diagnostics.AddError("Could not archive pipeline", err.Error())
		}
		return
	}

	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		log.Printf("Deleting pipeline %s ...", state.Name.ValueString())
		_, err := deletePipeline(ctx, p.client.genqlient, state.Id.ValueString())
		return retryContextError(err)
	})

	if err != nil {
		resp.Diagnostics.AddError("Could not delete pipeline", err.Error())
	}
}

func (*pipelineResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pipeline"
}

func (p *pipelineResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pipelineResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeouts, diags := p.client.timeouts.Read(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var response *getNodeResponse
	err := retry.RetryContext(ctx, timeouts, func() *retry.RetryError {
		var err error
		response, err = getNode(ctx, p.client.genqlient, state.Id.ValueString())
		return retryContextError(err)
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read pipeline",
			fmt.Sprintf("Unable to pipeline: %s", err.Error()),
		)
		return
	}

	if pipelineNode, ok := response.Node.(*getNodeNodePipeline); ok {
		// no pipeline with given ID found, set empty state
		if pipelineNode == nil {
			resp.Diagnostics.AddError("Unable to get pipeline", fmt.Sprintf("Unable to get pipeline with ID %s (%v)", state.Id.ValueString(), err))
			return
		}

		extraInfo, err := getPipelineExtraInfo(ctx, p.client, pipelineNode.Slug, timeouts)
		if err != nil {
			resp.Diagnostics.AddError("Unable to read pipeline info from REST", err.Error())
			return
		}

		setPipelineModel(&state, pipelineNode)

		if len(state.ProviderSettings) > 0 {
			updatePipelineResourceExtraInfo(&state, extraInfo)
		}
		state.BadgeUrl = types.StringValue(extraInfo.BadgeUrl)
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	} else {
		// no pipeline was found so remove it from state
		resp.Diagnostics.AddWarning("Pipeline not found", "Removing pipeline from state")
		resp.State.RemoveResource(ctx)
	}
}

func (*pipelineResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"allow_rebuilds": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
			"cancel_intermediate_builds": schema.BoolAttribute{
				Computed: true,
				Optional: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"cancel_intermediate_builds_branch_filter": schema.StringAttribute{
				Computed: true,
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"branch_configuration": schema.StringAttribute{
				Optional: true,
			},
			"cluster_id": schema.StringAttribute{
				Optional: true,
			},
			"default_branch": schema.StringAttribute{
				Computed: true,
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"default_timeout_in_minutes": schema.Int64Attribute{
				Computed: true,
				Optional: true,
				Default:  nil,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"maximum_timeout_in_minutes": schema.Int64Attribute{
				Computed: true,
				Optional: true,
				Default:  nil,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"description": schema.StringAttribute{
				Computed: true,
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
			},
			"repository": schema.StringAttribute{
				Required: true,
			},
			"skip_intermediate_builds": schema.BoolAttribute{
				Computed: true,
				Optional: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"skip_intermediate_builds_branch_filter": schema.StringAttribute{
				Computed: true,
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"slug": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					custom_modifier.UseStateIfUnchanged("name"),
				},
			},
			"steps": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString(defaultSteps),
			},
			"tags": schema.SetAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Default:     setdefault.StaticValue(types.SetValueMust(types.StringType, []attr.Value{})),
			},
			"webhook_url": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"badge_url": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
		Blocks: map[string]schema.Block{
			"provider_settings": schema.ListNestedBlock{
				Validators: []validator.List{
					listvalidator.SizeBetween(0, 1),
				},
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"trigger_mode": schema.StringAttribute{
							Computed: true,
							Optional: true,
							Validators: []validator.String{
								pipelinevalidation.WhenRepositoryProviderIs(pipelinevalidation.RepositoryProviderGitHub),
								stringvalidator.OneOf("code", "deployment", "fork", "none"),
							},
						},
						"build_pull_requests": schema.BoolAttribute{
							Optional: true,
							Computed: true,
							Validators: []validator.Bool{
								boolvalidator.Any(
									pipelinevalidation.WhenRepositoryProviderIs(pipelinevalidation.RepositoryProviderBitbucket),
									boolvalidator.All(
										pipelinevalidation.WhenRepositoryProviderIs(pipelinevalidation.RepositoryProviderGitHub),
										boolvalidation.WhenString(path.MatchRoot("provider_settings").AtAnyListIndex().AtName("trigger_mode"), "code"),
									),
								),
							},
						},
						"pull_request_branch_filter_enabled": schema.BoolAttribute{
							Computed: true,
							Optional: true,
							Validators: []validator.Bool{
								boolvalidation.WhenBool(path.MatchRoot("provider_settings").AtAnyListIndex().AtName("build_pull_requests"), true),
							},
						},
						"pull_request_branch_filter_configuration": schema.StringAttribute{
							Computed: true,
							Optional: true,
							Validators: []validator.String{
								stringvalidation.WhenBool(path.MatchRoot("provider_settings").AtAnyListIndex().AtName("pull_request_branch_filter_enabled"), true),
							},
						},
						"skip_builds_for_existing_commits": schema.BoolAttribute{
							Optional: true,
							Computed: true,
							Validators: []validator.Bool{
								pipelinevalidation.WhenRepositoryProviderIs(pipelinevalidation.RepositoryProviderGitHub),
							},
						},
						"skip_pull_request_builds_for_existing_commits": schema.BoolAttribute{
							Optional: true,
							Computed: true,
							Validators: []validator.Bool{
								boolvalidation.WhenBool(path.MatchRoot("provider_settings").AtAnyListIndex().AtName("build_pull_requests"), true),
							},
						},
						"build_pull_request_ready_for_review": schema.BoolAttribute{
							Computed: true,
							Optional: true,
							Validators: []validator.Bool{
								pipelinevalidation.WhenRepositoryProviderIs(pipelinevalidation.RepositoryProviderGitHub),
								boolvalidation.WhenString(path.MatchRoot("provider_settings").AtAnyListIndex().AtName("trigger_mode"), "code"),
								boolvalidation.WhenBool(path.MatchRoot("provider_settings").AtAnyListIndex().AtName("build_pull_requests"), true),
							},
						},
						"build_pull_request_labels_changed": schema.BoolAttribute{
							Computed: true,
							Optional: true,
							Validators: []validator.Bool{
								pipelinevalidation.WhenRepositoryProviderIs(pipelinevalidation.RepositoryProviderGitHub),
								boolvalidation.WhenString(path.MatchRoot("provider_settings").AtAnyListIndex().AtName("trigger_mode"), "code"),
								boolvalidation.WhenBool(path.MatchRoot("provider_settings").AtAnyListIndex().AtName("build_pull_requests"), true),
							},
						},
						"build_pull_request_forks": schema.BoolAttribute{
							Computed: true,
							Optional: true,
							Validators: []validator.Bool{
								pipelinevalidation.WhenRepositoryProviderIs(pipelinevalidation.RepositoryProviderGitHub),
								boolvalidation.WhenString(path.MatchRoot("provider_settings").AtAnyListIndex().AtName("trigger_mode"), "code"),
								boolvalidation.WhenBool(path.MatchRoot("provider_settings").AtAnyListIndex().AtName("build_pull_requests"), true),
							},
						},
						"prefix_pull_request_fork_branch_names": schema.BoolAttribute{
							Computed: true,
							Optional: true,
							Validators: []validator.Bool{
								boolvalidation.WhenBool(path.MatchRoot("provider_settings").AtAnyListIndex().AtName("build_pull_request_forks"), true),
							},
						},
						"build_branches": schema.BoolAttribute{
							Optional: true,
							Computed: true,
							Validators: []validator.Bool{
								pipelinevalidation.WhenRepositoryProviderIs(pipelinevalidation.RepositoryProviderGitHub, pipelinevalidation.RepositoryProviderBitbucket),
							},
						},
						"build_tags": schema.BoolAttribute{
							Computed: true,
							Optional: true,
							Validators: []validator.Bool{
								pipelinevalidation.WhenRepositoryProviderIs(pipelinevalidation.RepositoryProviderGitHub, pipelinevalidation.RepositoryProviderBitbucket),
							},
						},
						"cancel_deleted_branch_builds": schema.BoolAttribute{
							Computed: true,
							Optional: true,
							Validators: []validator.Bool{
								pipelinevalidation.WhenRepositoryProviderIs(pipelinevalidation.RepositoryProviderGitHub),
								boolvalidation.WhenString(path.MatchRoot("provider_settings").AtAnyListIndex().AtName("trigger_mode"), "code"),
							},
						},
						"filter_enabled": schema.BoolAttribute{
							Computed: true,
							Optional: true,
							Validators: []validator.Bool{
								pipelinevalidation.WhenRepositoryProviderIs(pipelinevalidation.RepositoryProviderGitHub),
								boolvalidation.WhenString(path.MatchRoot("provider_settings").AtAnyListIndex().AtName("trigger_mode"), "code"),
							},
						},
						"filter_condition": schema.StringAttribute{
							Computed: true,
							Optional: true,
							Validators: []validator.String{
								stringvalidation.WhenBool(path.MatchRoot("provider_settings").AtAnyListIndex().AtName("filter_enabled"), true),
							},
						},
						"publish_commit_status": schema.BoolAttribute{
							Optional: true,
							Computed: true,
							Validators: []validator.Bool{
								boolvalidator.Any(
									pipelinevalidation.WhenRepositoryProviderIs(pipelinevalidation.RepositoryProviderBitbucket),
									boolvalidator.All(
										pipelinevalidation.WhenRepositoryProviderIs(pipelinevalidation.RepositoryProviderGitHub),
										boolvalidation.WhenString(path.MatchRoot("provider_settings").AtAnyListIndex().AtName("trigger_mode"), "code"),
									),
								),
							},
						},
						"publish_blocked_as_pending": schema.BoolAttribute{
							Computed: true,
							Optional: true,
							Validators: []validator.Bool{
								pipelinevalidation.WhenRepositoryProviderIs(pipelinevalidation.RepositoryProviderGitHub),
								boolvalidation.WhenBool(path.MatchRoot("provider_settings").AtAnyListIndex().AtName("publish_commit_status"), true),
							},
						},
						"publish_commit_status_per_step": schema.BoolAttribute{
							Computed: true,
							Optional: true,
							Validators: []validator.Bool{
								pipelinevalidation.WhenRepositoryProviderIs(pipelinevalidation.RepositoryProviderGitHub, pipelinevalidation.RepositoryProviderBitbucket),
								boolvalidation.WhenBool(path.MatchRoot("provider_settings").AtAnyListIndex().AtName("publish_commit_status"), true),
							},
						},
						"separate_pull_request_statuses": schema.BoolAttribute{
							Computed: true,
							Optional: true,
							Validators: []validator.Bool{
								pipelinevalidation.WhenRepositoryProviderIs(pipelinevalidation.RepositoryProviderGitHub),
								boolvalidation.WhenBool(path.MatchRoot("provider_settings").AtAnyListIndex().AtName("publish_commit_status"), true),
							},
						},
					},
				},
			},
		},
	}
}

func (p *pipelineResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state pipelineResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	defaultTimeoutInMinutes := (*int)(unsafe.Pointer(plan.DefaultTimeoutInMinutes.ValueInt64Pointer()))
	maxTimeoutInMinutes := (*int)(unsafe.Pointer(plan.MaximumTimeoutInMinutes.ValueInt64Pointer()))

	input := PipelineUpdateInput{
		AllowRebuilds:                        plan.AllowRebuilds.ValueBool(),
		BranchConfiguration:                  plan.BranchConfiguration.ValueStringPointer(),
		CancelIntermediateBuilds:             plan.CancelIntermediateBuilds.ValueBool(),
		CancelIntermediateBuildsBranchFilter: plan.CancelIntermediateBuildsBranchFilter.ValueString(),
		ClusterId:                            plan.ClusterId.ValueStringPointer(),
		DefaultBranch:                        plan.DefaultBranch.ValueString(),
		DefaultTimeoutInMinutes:              defaultTimeoutInMinutes,
		MaximumTimeoutInMinutes:              maxTimeoutInMinutes,
		Description:                          plan.Description.ValueString(),
		Id:                                   plan.Id.ValueString(),
		Name:                                 plan.Name.ValueString(),
		Repository:                           PipelineRepositoryInput{Url: plan.Repository.ValueString()},
		SkipIntermediateBuilds:               plan.SkipIntermediateBuilds.ValueBool(),
		SkipIntermediateBuildsBranchFilter:   plan.SkipIntermediateBuildsBranchFilter.ValueString(),
		Steps:                                PipelineStepsInput{Yaml: plan.Steps.ValueString()},
		Tags:                                 getTagsFromSchema(&plan),
	}

	timeouts, diags := p.client.timeouts.Read(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var response *updatePipelineResponse
	err := retry.RetryContext(ctx, timeouts, func() *retry.RetryError {
		var err error
		log.Printf("Updating pipeline %s ...", input.Name)
		response, err = updatePipeline(ctx, p.client.genqlient, input)
		return retryContextError(err)
	})

	if err != nil {
		resp.Diagnostics.AddError("Unable to update pipeline %s", state.Name.ValueString())
		return
	}

	setPipelineModel(&state, &response.PipelineUpdate.Pipeline)

	if len(plan.ProviderSettings) > 0 {
		pipelineExtraInfo, err := updatePipelineExtraInfo(ctx, response.PipelineUpdate.Pipeline.Slug, plan.ProviderSettings[0], p.client, timeouts)
		if err != nil {
			resp.Diagnostics.AddError("Unable to set pipeline info from REST", err.Error())
			return
		}

		updatePipelineResourceExtraInfo(&state, &pipelineExtraInfo)
	} else {
		// no provider_settings provided, but we still need to read in the badge url
		extraInfo, err := getPipelineExtraInfo(ctx, p.client, response.PipelineUpdate.Pipeline.Slug, timeouts)
		if err != nil {
			resp.Diagnostics.AddError("Unable to read pipeline info from REST", err.Error())
			return
		}
		state.BadgeUrl = types.StringValue(extraInfo.BadgeUrl)
		state.ProviderSettings = make([]*providerSettingsModel, 0)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (*pipelineResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func setPipelineModel(model *pipelineResourceModel, data pipelineResponse) {
	defaultTimeoutInMinutes := (*int64)(unsafe.Pointer(data.GetDefaultTimeoutInMinutes()))
	maximumTimeoutInMinutes := (*int64)(unsafe.Pointer(data.GetMaximumTimeoutInMinutes()))

	model.AllowRebuilds = types.BoolValue(data.GetAllowRebuilds())
	model.BranchConfiguration = types.StringPointerValue(data.GetBranchConfiguration())
	model.CancelIntermediateBuilds = types.BoolValue(data.GetCancelIntermediateBuilds())
	model.CancelIntermediateBuildsBranchFilter = types.StringValue(data.GetCancelIntermediateBuildsBranchFilter())
	model.ClusterId = types.StringPointerValue(data.GetCluster().Id)
	model.DefaultBranch = types.StringValue(data.GetDefaultBranch())
	model.DefaultTimeoutInMinutes = types.Int64PointerValue(defaultTimeoutInMinutes)
	model.Description = types.StringValue(data.GetDescription())
	model.Id = types.StringValue(data.GetId())
	model.MaximumTimeoutInMinutes = types.Int64PointerValue(maximumTimeoutInMinutes)
	model.Name = types.StringValue(data.GetName())
	model.Repository = types.StringValue(data.GetRepository().Url)
	model.SkipIntermediateBuilds = types.BoolValue(data.GetSkipIntermediateBuilds())
	model.SkipIntermediateBuildsBranchFilter = types.StringValue(data.GetSkipIntermediateBuildsBranchFilter())
	model.Slug = types.StringValue(data.GetSlug())
	model.Steps = types.StringValue(data.GetSteps().Yaml)
	model.WebhookUrl = types.StringValue(data.GetWebhookURL())

	tags := make([]types.String, len(data.GetTags()))
	for i, tag := range data.GetTags() {
		tags[i] = types.StringValue(tag.Label)
	}
	model.Tags = tags
}

// As of May 21, 2021, GraphQL Pipeline is lacking support for the following properties:
// - badge_url
// - provider_settings
// We fallback to REST API

// PipelineExtraInfo is used to manage pipeline attributes that are not exposed via GraphQL API.
type PipelineExtraInfo struct {
	BadgeUrl string `json:"badge_url"`
	Provider struct {
		Settings PipelineExtraSettings `json:"settings"`
	} `json:"provider"`
}
type PipelineExtraSettings struct {
	TriggerMode                             *string `json:"trigger_mode,omitempty"`
	BuildPullRequests                       *bool   `json:"build_pull_requests,omitempty"`
	PullRequestBranchFilterEnabled          *bool   `json:"pull_request_branch_filter_enabled,omitempty"`
	PullRequestBranchFilterConfiguration    *string `json:"pull_request_branch_filter_configuration,omitempty"`
	SkipBuildsForExistingCommits            *bool   `json:"skip_builds_for_existing_commits,omitempty"`
	SkipPullRequestBuildsForExistingCommits *bool   `json:"skip_pull_request_builds_for_existing_commits,omitempty"`
	BuildPullRequestReadyForReview          *bool   `json:"build_pull_request_ready_for_review,omitempty"`
	BuildPullRequestLabelsChanged           *bool   `json:"build_pull_request_labels_changed,omitempty"`
	BuildPullRequestForks                   *bool   `json:"build_pull_request_forks,omitempty"`
	PrefixPullRequestForkBranchNames        *bool   `json:"prefix_pull_request_fork_branch_names,omitempty"`
	BuildBranches                           *bool   `json:"build_branches,omitempty"`
	BuildTags                               *bool   `json:"build_tags,omitempty"`
	CancelDeletedBranchBuilds               *bool   `json:"cancel_deleted_branch_builds,omitempty"`
	FilterEnabled                           *bool   `json:"filter_enabled,omitempty"`
	FilterCondition                         *string `json:"filter_condition,omitempty"`
	PublishCommitStatus                     *bool   `json:"publish_commit_status,omitempty"`
	PublishBlockedAsPending                 *bool   `json:"publish_blocked_as_pending,omitempty"`
	PublishCommitStatusPerStep              *bool   `json:"publish_commit_status_per_step,omitempty"`
	SeparatePullRequestStatuses             *bool   `json:"separate_pull_request_statuses,omitempty"`
}

func getPipelineExtraInfo(ctx context.Context, client *Client, slug string, timeouts time.Duration) (*PipelineExtraInfo, error) {
	pipelineExtraInfo := PipelineExtraInfo{}

	err := retry.RetryContext(ctx, timeouts, func() *retry.RetryError {
		err := client.makeRequest(ctx, "GET", fmt.Sprintf("/v2/organizations/%s/pipelines/%s", client.organization, slug), nil, &pipelineExtraInfo)
		return retryContextError(err)
	})

	if err != nil {
		return nil, err
	}

	return &pipelineExtraInfo, nil
}
func updatePipelineExtraInfo(ctx context.Context, slug string, settings *providerSettingsModel, client *Client, timeouts time.Duration) (PipelineExtraInfo, error) {
	payload := map[string]any{
		"provider_settings": PipelineExtraSettings{
			TriggerMode:                             settings.TriggerMode.ValueStringPointer(),
			BuildPullRequests:                       settings.BuildPullRequests.ValueBoolPointer(),
			PullRequestBranchFilterEnabled:          settings.PullRequestBranchFilterEnabled.ValueBoolPointer(),
			PullRequestBranchFilterConfiguration:    settings.PullRequestBranchFilterConfiguration.ValueStringPointer(),
			SkipBuildsForExistingCommits:            settings.SkipBuildsForExistingCommits.ValueBoolPointer(),
			SkipPullRequestBuildsForExistingCommits: settings.SkipPullRequestBuildsForExistingCommits.ValueBoolPointer(),
			BuildPullRequestReadyForReview:          settings.BuildPullRequestReadyForReview.ValueBoolPointer(),
			BuildPullRequestLabelsChanged:           settings.BuildPullRequestLabelsChanged.ValueBoolPointer(),
			BuildPullRequestForks:                   settings.BuildPullRequestForks.ValueBoolPointer(),
			PrefixPullRequestForkBranchNames:        settings.PrefixPullRequestForkBranchNames.ValueBoolPointer(),
			BuildBranches:                           settings.BuildBranches.ValueBoolPointer(),
			BuildTags:                               settings.BuildTags.ValueBoolPointer(),
			CancelDeletedBranchBuilds:               settings.CancelDeletedBranchBuilds.ValueBoolPointer(),
			FilterEnabled:                           settings.FilterEnabled.ValueBoolPointer(),
			FilterCondition:                         settings.FilterCondition.ValueStringPointer(),
			PublishCommitStatus:                     settings.PublishCommitStatus.ValueBoolPointer(),
			PublishBlockedAsPending:                 settings.PublishBlockedAsPending.ValueBoolPointer(),
			PublishCommitStatusPerStep:              settings.PublishCommitStatusPerStep.ValueBoolPointer(),
			SeparatePullRequestStatuses:             settings.SeparatePullRequestStatuses.ValueBoolPointer(),
		},
	}

	pipelineExtraInfo := PipelineExtraInfo{}
	err := retry.RetryContext(ctx, timeouts, func() *retry.RetryError {
		err := client.makeRequest(ctx, "PATCH", fmt.Sprintf("/v2/organizations/%s/pipelines/%s", client.organization, slug), payload, &pipelineExtraInfo)
		return retryContextError(err)
	})

	if err != nil {
		return pipelineExtraInfo, err
	}
	return pipelineExtraInfo, nil
}

func getTagsFromSchema(plan *pipelineResourceModel) []PipelineTagInput {
	tags := make([]PipelineTagInput, len(plan.Tags))
	for i, tag := range plan.Tags {
		tags[i] = PipelineTagInput{
			Label: tag.ValueString(),
		}
	}
	return tags
}

// updatePipelineResourceExtraInfo updates the terraform resource with data received from Buildkite REST API
func updatePipelineResourceExtraInfo(state *pipelineResourceModel, pipeline *PipelineExtraInfo) {
	state.BadgeUrl = types.StringValue(pipeline.BadgeUrl)
	s := pipeline.Provider.Settings
	state.ProviderSettings = []*providerSettingsModel{
		{
			TriggerMode:                             types.StringPointerValue(s.TriggerMode),
			BuildPullRequests:                       types.BoolPointerValue(s.BuildPullRequests),
			PullRequestBranchFilterEnabled:          types.BoolPointerValue(s.PullRequestBranchFilterEnabled),
			PullRequestBranchFilterConfiguration:    types.StringPointerValue(s.PullRequestBranchFilterConfiguration),
			SkipBuildsForExistingCommits:            types.BoolPointerValue(s.SkipBuildsForExistingCommits),
			SkipPullRequestBuildsForExistingCommits: types.BoolPointerValue(s.SkipPullRequestBuildsForExistingCommits),
			BuildPullRequestReadyForReview:          types.BoolPointerValue(s.BuildPullRequestReadyForReview),
			BuildPullRequestLabelsChanged:           types.BoolPointerValue(s.BuildPullRequestLabelsChanged),
			BuildPullRequestForks:                   types.BoolPointerValue(s.BuildPullRequestForks),
			PrefixPullRequestForkBranchNames:        types.BoolPointerValue(s.PrefixPullRequestForkBranchNames),
			BuildBranches:                           types.BoolPointerValue(s.BuildBranches),
			BuildTags:                               types.BoolPointerValue(s.BuildTags),
			CancelDeletedBranchBuilds:               types.BoolPointerValue(s.CancelDeletedBranchBuilds),
			FilterEnabled:                           types.BoolPointerValue(s.FilterEnabled),
			FilterCondition:                         types.StringPointerValue(s.FilterCondition),
			PublishCommitStatus:                     types.BoolPointerValue(s.PublishCommitStatus),
			PublishBlockedAsPending:                 types.BoolPointerValue(s.PublishBlockedAsPending),
			PublishCommitStatusPerStep:              types.BoolPointerValue(s.PublishCommitStatusPerStep),
			SeparatePullRequestStatuses:             types.BoolPointerValue(s.SeparatePullRequestStatuses),
		},
	}
}
