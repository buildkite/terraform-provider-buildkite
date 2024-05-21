package buildkite

import (
	"context"
	"fmt"
	"log"
	"time"
	"unsafe"

	"github.com/MakeNowJust/heredoc"
	custom_modifier "github.com/buildkite/terraform-provider-buildkite/internal/planmodifier"
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
	PipelineUuid                         graphql.String
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
	AllowRebuilds                        types.Bool             `tfsdk:"allow_rebuilds"`
	BadgeUrl                             types.String           `tfsdk:"badge_url"`
	BranchConfiguration                  types.String           `tfsdk:"branch_configuration"`
	CancelIntermediateBuilds             types.Bool             `tfsdk:"cancel_intermediate_builds"`
	CancelIntermediateBuildsBranchFilter types.String           `tfsdk:"cancel_intermediate_builds_branch_filter"`
	Color                                types.String           `tfsdk:"color"`
	ClusterId                            types.String           `tfsdk:"cluster_id"`
	DefaultTeamId                        types.String           `tfsdk:"default_team_id"`
	DefaultBranch                        types.String           `tfsdk:"default_branch"`
	DefaultTimeoutInMinutes              types.Int64            `tfsdk:"default_timeout_in_minutes"`
	Description                          types.String           `tfsdk:"description"`
	Emoji                                types.String           `tfsdk:"emoji"`
	Id                                   types.String           `tfsdk:"id"`
	MaximumTimeoutInMinutes              types.Int64            `tfsdk:"maximum_timeout_in_minutes"`
	Name                                 types.String           `tfsdk:"name"`
	PipelineTemplateId                   types.String           `tfsdk:"pipeline_template_id"`
	ProviderSettings                     *providerSettingsModel `tfsdk:"provider_settings"`
	Repository                           types.String           `tfsdk:"repository"`
	SkipIntermediateBuilds               types.Bool             `tfsdk:"skip_intermediate_builds"`
	SkipIntermediateBuildsBranchFilter   types.String           `tfsdk:"skip_intermediate_builds_branch_filter"`
	Slug                                 types.String           `tfsdk:"slug"`
	Steps                                types.String           `tfsdk:"steps"`
	Tags                                 []types.String         `tfsdk:"tags"`
	UUID                                 types.String           `tfsdk:"uuid"`
	WebhookUrl                           types.String           `tfsdk:"webhook_url"`
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
	archiveOnDelete *bool
}

type pipelineResponse interface {
	GetId() string
	GetPipelineUuid() string
	GetAllowRebuilds() bool
	GetBranchConfiguration() *string
	GetCancelIntermediateBuilds() bool
	GetCancelIntermediateBuildsBranchFilter() string
	GetCluster() PipelineFieldsCluster
	GetColor() *string
	GetDefaultBranch() string
	GetDefaultTimeoutInMinutes() *int
	GetMaximumTimeoutInMinutes() *int
	GetDescription() string
	GetEmoji() *string
	GetName() string
	GetRepository() PipelineFieldsRepository
	GetPipelineTemplate() PipelineFieldsPipelineTemplate
	GetSkipIntermediateBuilds() bool
	GetSkipIntermediateBuildsBranchFilter() string
	GetSlug() string
	GetSteps() PipelineFieldsStepsPipelineSteps
	GetTags() []PipelineFieldsTagsPipelineTag
}

func newPipelineResource(archiveOnDelete *bool) func() resource.Resource {
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

	timeouts, diags := p.client.timeouts.Create(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var response *createPipelineResponse
	log.Printf("Creating pipeline %s ...", plan.Name.ValueString())
	err := retry.RetryContext(ctx, timeouts, func() *retry.RetryError {
		org, err := p.client.GetOrganizationID()
		if err == nil {
			input := PipelineCreateInput{
				AllowRebuilds:                        plan.AllowRebuilds.ValueBool(),
				BranchConfiguration:                  plan.BranchConfiguration.ValueStringPointer(),
				CancelIntermediateBuilds:             plan.CancelIntermediateBuilds.ValueBool(),
				CancelIntermediateBuildsBranchFilter: plan.CancelIntermediateBuildsBranchFilter.ValueString(),
				ClusterId:                            plan.ClusterId.ValueStringPointer(),
				Color:                                plan.Color.ValueStringPointer(),
				DefaultBranch:                        plan.DefaultBranch.ValueString(),
				DefaultTimeoutInMinutes:              defaultTimeoutInMinutes,
				Emoji:                                plan.Emoji.ValueStringPointer(),
				MaximumTimeoutInMinutes:              maxTimeoutInMinutes,
				Description:                          plan.Description.ValueString(),
				Name:                                 plan.Name.ValueString(),
				OrganizationId:                       *org,
				PipelineTemplateId:                   plan.PipelineTemplateId.ValueString(),
				Repository:                           PipelineRepositoryInput{Url: plan.Repository.ValueString()},
				SkipIntermediateBuilds:               plan.SkipIntermediateBuilds.ValueBool(),
				SkipIntermediateBuildsBranchFilter:   plan.SkipIntermediateBuildsBranchFilter.ValueString(),
				Steps:                                PipelineStepsInput{Yaml: plan.Steps.ValueString()},
				Tags:                                 getTagsFromSchema(&plan),
			}

			// if a team has been specified, add that to the graphql payload
			if !plan.DefaultTeamId.IsNull() {
				input.Teams = []PipelineTeamAssignmentInput{
					{
						Id:          plan.DefaultTeamId.ValueString(),
						AccessLevel: PipelineAccessLevelsManageBuildAndRead,
					},
				}
			}

			response, err = createPipeline(ctx, p.client.genqlient, input)
		}
		return retryContextError(err)
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to create pipeline",
			fmt.Sprintf("Failed to create pipeline: %s", err.Error()),
		)
		return
	}
	log.Printf("Successfully created pipeline with id '%s'.", response.PipelineCreate.Pipeline.Id)

	setPipelineModel(&state, &response.PipelineCreate.Pipeline)
	state.WebhookUrl = types.StringValue(response.PipelineCreate.Pipeline.GetWebhookURL())
	state.DefaultTeamId = plan.DefaultTeamId

	if plan.ProviderSettings != nil {
		pipelineExtraInfo, err := updatePipelineExtraInfo(ctx, response.PipelineCreate.Pipeline.Slug, plan.ProviderSettings, p.client, timeouts)
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

	if *p.archiveOnDelete {
		log.Printf("Pipeline %s set to archive on delete. Archiving...", state.Name.ValueString())

		err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
			_, err := archivePipeline(ctx, p.client.genqlient, state.Id.ValueString())
			return retryContextError(err)
		})
		if err != nil {
			resp.Diagnostics.AddError(
				"Could not archive pipeline",
				fmt.Sprintf("Could not archive pipeline %s", err.Error()),
			)
		}
		return
	}

	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		log.Printf("Deleting pipeline %s ...", state.Name.ValueString())
		_, err := deletePipeline(ctx, p.client.genqlient, state.Id.ValueString())
		return retryContextError(err)
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Could not delete pipeline",
			fmt.Sprintf("Could not delete pipeline: %s", err.Error()),
		)
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

		// set the webhook url if its not empty
		// the value can be empty if not using a token with appropriate permissions. in this case, we just leave the
		// state value alone assuming it was previously set correctly
		if extraInfo.Provider.WebhookUrl != "" {
			state.WebhookUrl = types.StringValue(extraInfo.Provider.WebhookUrl)
		}

		if state.ProviderSettings != nil {
			updatePipelineResourceExtraInfo(&state, extraInfo)
		}

		// pipeline default team is a terraform concept only so it takes some coercing
		err = p.setDefaultTeamIfExists(ctx, &state, &pipelineNode.Teams.PipelineTeam)
		if err != nil {
			resp.Diagnostics.AddError("Error encountered trying to read teams for pipeline", err.Error())
		}

		state.BadgeUrl = types.StringValue(extraInfo.BadgeUrl)

		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	} else {
		// no pipeline was found so remove it from state
		resp.Diagnostics.AddWarning("Pipeline not found", "Removing pipeline from state")
		resp.State.RemoveResource(ctx)
	}
}

// setDefaultTeamIfExists will try to find a team for the pipeline to set as default
// if we got here from a terraform import, we will have no idea what (if any) team to assign as the default, it
// will need to be done manually by the user
// however, if this is a normal read operation, we will have access to the previous state which is a reliable
// source of default team ID. so if it is set in state, we need to ensure the permission level is correct,
// otherwise it cannot be the default owner if it has lower permissions
func (p *pipelineResource) setDefaultTeamIfExists(ctx context.Context, state *pipelineResourceModel, pipelineTeam *PipelineTeam) error {
	if !state.DefaultTeamId.IsNull() {
		var foundAccessLevel *PipelineAccessLevels
		// loop over all attached teams to ensure its connected with the correct permissions
		for _, team := range pipelineTeam.Edges {
			if state.DefaultTeamId.ValueString() == team.Node.Team.Id {
				foundAccessLevel = &team.Node.AccessLevel
				break
			}
		}

		// if the team was not found, and there are more to load, then load more and recurse to find a matching one
		if foundAccessLevel == nil && pipelineTeam.PageInfo.HasNextPage {
			resp, err := getPipelineTeams(ctx, p.client.genqlient, state.Slug.ValueString(), pipelineTeam.PageInfo.EndCursor)
			if err != nil {
				return err
			}
			pt := resp.Pipeline.Teams.PipelineTeam
			return p.setDefaultTeamIfExists(ctx, state, &pt)
		}

		// after checking all teams, if a matching one was still not found or the permission was wrong, then update
		// the state
		if foundAccessLevel == nil || *foundAccessLevel != PipelineAccessLevelsManageBuildAndRead {
			state.DefaultTeamId = types.StringUnknown()
		}
	}

	return nil
}

func (*pipelineResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Version: 1,
		MarkdownDescription: heredoc.Doc(`
			This resource allows you to create and manage pipelines for repositories.

			More information on pipelines can be found in the [documentation](https://buildkite.com/docs/pipelines).
		`),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The GraphQL ID of the pipeline.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"allow_rebuilds": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Whether rebuilds are allowed for this pipeline.",
			},
			"branch_configuration": schema.StringAttribute{
				MarkdownDescription: "Configure the pipeline to only build on this branch conditional.",
				Optional:            true,
			},
			"cancel_intermediate_builds": schema.BoolAttribute{
				Computed:            true,
				Optional:            true,
				MarkdownDescription: "Whether to cancel builds when a new commit is pushed to a matching branch.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"cancel_intermediate_builds_branch_filter": schema.StringAttribute{
				Computed:            true,
				Optional:            true,
				MarkdownDescription: "Filter the `cancel_intermediate_builds` setting based on this branch condition.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"color": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "A color hex code to represent this pipeline.",
			},
			"cluster_id": schema.StringAttribute{
				MarkdownDescription: "Attach this pipeline to the given cluster GraphQL ID.",
				Optional:            true,
			},
			"default_team_id": schema.StringAttribute{
				MarkdownDescription: "The GraphQL ID of the team to use as the default owner of the pipeline.",
				Optional:            true,
			},
			"default_branch": schema.StringAttribute{
				Computed:            true,
				Optional:            true,
				MarkdownDescription: "Default branch of the pipeline.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"default_timeout_in_minutes": schema.Int64Attribute{
				Computed:            true,
				Optional:            true,
				Default:             nil,
				MarkdownDescription: "Set pipeline wide timeout for command steps.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"emoji": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "An emoji that represents this pipeline.",
			},
			"maximum_timeout_in_minutes": schema.Int64Attribute{
				Computed:            true,
				Optional:            true,
				Default:             nil,
				MarkdownDescription: "Set pipeline wide maximum timeout for command steps.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"description": schema.StringAttribute{
				Computed:            true,
				Optional:            true,
				MarkdownDescription: "Description for the pipeline. Can include emoji 🙌.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name to give the pipeline.",
			},
			"pipeline_template_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The GraphQL ID of the pipeline template applied to this pipeline.",
			},
			"repository": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "URL to the repository this pipeline is configured for.",
			},
			"skip_intermediate_builds": schema.BoolAttribute{
				Computed:            true,
				Optional:            true,
				MarkdownDescription: "Whether to skip queued builds if a new commit is pushed to a matching branch.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"skip_intermediate_builds_branch_filter": schema.StringAttribute{
				Computed:            true,
				Optional:            true,
				MarkdownDescription: "Filter the `skip_intermediate_builds` setting based on this branch condition.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"slug": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The slug generated for the pipeline.",
				PlanModifiers: []planmodifier.String{
					custom_modifier.UseStateIfUnchanged("name"),
				},
			},
			"steps": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The YAML steps to configure for the pipeline. Defaults to `buildkite-agent pipeline upload`.",
				Validators: []validator.String{
					stringvalidator.ConflictsWith(path.Expressions{
						path.MatchRoot("pipeline_template_id"),
					}...),
				},
			},
			"tags": schema.SetAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				Default:             setdefault.StaticValue(types.SetValueMust(types.StringType, []attr.Value{})),
				MarkdownDescription: "Tags to attribute to the pipeline. Useful for searching by in the UI.",
			},
			"uuid": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the pipeline.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"webhook_url": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The webhook URL used to trigger builds from VCS providers.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"badge_url": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The badge URL showing build state.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"provider_settings": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Control settings depending on the VCS provider used in `repository`.",
				Attributes: map[string]schema.Attribute{
					"trigger_mode": schema.StringAttribute{
						Computed: true,
						Optional: true,
						MarkdownDescription: heredoc.Docf(`
							What type of event to trigger builds on. Must be one of:
								- %s
								- %s
								- %s
								- %s

								-> %s is only valid if the pipeline uses a GitHub repository.
								-> If not set, the default value is %s and other provider settings defaults are applied.
						`,
							"`code` will create builds when code is pushed to GitHub.",
							"`deployment` will create builds when a deployment is created in GitHub.",
							"`fork` will create builds when the GitHub repository is forked.",
							"`none` will not create any builds based on GitHub activity.",
							"`trigger_mode`",
							"`code`",
						),
					},
					"build_pull_requests": schema.BoolAttribute{
						Optional:            true,
						Computed:            true,
						MarkdownDescription: "Whether to create builds for commits that are part of a pull request. Defaults to `true` when `trigger_mode` is set to `code`.",
					},
					"pull_request_branch_filter_enabled": schema.BoolAttribute{
						Computed:            true,
						Optional:            true,
						MarkdownDescription: "Filter pull request builds.",
					},
					"pull_request_branch_filter_configuration": schema.StringAttribute{
						Computed:            true,
						Optional:            true,
						MarkdownDescription: "Filter pull requests builds by the branch filter.",
					},
					"skip_builds_for_existing_commits": schema.BoolAttribute{
						Optional:            true,
						Computed:            true,
						MarkdownDescription: "Whether to skip creating a new build if an existing build for the commit and branch already exists. This option is only valid if the pipeline uses a GitHub repository.",
					},
					"skip_pull_request_builds_for_existing_commits": schema.BoolAttribute{
						Optional:            true,
						Computed:            true,
						MarkdownDescription: "Whether to skip creating a new build for a pull request if an existing build for the commit and branch already exists.  Defaults to `true` when `trigger_mode` is set to `code`.",
					},
					"build_pull_request_ready_for_review": schema.BoolAttribute{
						Computed:            true,
						Optional:            true,
						MarkdownDescription: "Whether to create a build when a pull request changes to \"Ready for review\".",
					},
					"build_pull_request_labels_changed": schema.BoolAttribute{
						Computed:            true,
						Optional:            true,
						MarkdownDescription: "Whether to create builds for pull requests when labels are added or removed.",
					},
					"build_pull_request_forks": schema.BoolAttribute{
						Computed:            true,
						Optional:            true,
						MarkdownDescription: "Whether to create builds for pull requests from third-party forks.",
					},
					"prefix_pull_request_fork_branch_names": schema.BoolAttribute{
						Computed: true,
						Optional: true,
						MarkdownDescription: "Prefix branch names for third-party fork builds to ensure they don't trigger branch conditions." +
							" For example, the main branch from some-user will become some-user:main.",
					},
					"build_branches": schema.BoolAttribute{
						Optional:            true,
						Computed:            true,
						MarkdownDescription: "Whether to create builds when branches are pushed. Defaults to `true` when `trigger_mode` is set to `code`.",
					},
					"build_tags": schema.BoolAttribute{
						Computed:            true,
						Optional:            true,
						MarkdownDescription: "Whether to create builds when tags are pushed.",
					},
					"cancel_deleted_branch_builds": schema.BoolAttribute{
						Computed:            true,
						Optional:            true,
						MarkdownDescription: "Automatically cancel running builds for a branch if the branch is deleted.",
					},
					"filter_enabled": schema.BoolAttribute{
						Computed:            true,
						Optional:            true,
						MarkdownDescription: "Whether to filter builds to only run when the condition in `filter_condition` is true.",
					},
					"filter_condition": schema.StringAttribute{
						Computed: true,
						Optional: true,
						MarkdownDescription: "The condition to evaluate when deciding if a build should run." +
							" More details available in [the documentation](https://buildkite.com/docs/pipelines/conditionals#conditionals-in-pipelines).",
					},
					"publish_commit_status": schema.BoolAttribute{
						Optional:            true,
						Computed:            true,
						MarkdownDescription: "Whether to update the status of commits in Bitbucket or GitHub. Defaults to `true` when `trigger_mode` is set to `code`.",
					},
					"publish_blocked_as_pending": schema.BoolAttribute{
						Computed: true,
						Optional: true,
						MarkdownDescription: "The status to use for blocked builds. Pending can be used with [required status checks](https://help.github.com/en/articles/enabling-required-status-checks)" +
							" to prevent merging pull requests with blocked builds.",
					},
					"publish_commit_status_per_step": schema.BoolAttribute{
						Computed:            true,
						Optional:            true,
						MarkdownDescription: "Whether to create a separate status for each job in a build, allowing you to see the status of each job directly in Bitbucket or GitHub.",
					},
					"separate_pull_request_statuses": schema.BoolAttribute{
						Computed: true,
						Optional: true,
						MarkdownDescription: "Whether to create a separate status for pull request builds, allowing you to require a passing pull request" +
							" build in your [required status checks](https://help.github.com/en/articles/enabling-required-status-checks) in GitHub.",
					},
				},
			},
		},
	}
}

func (p *pipelineResource) UpgradeState(ctx context.Context) map[int64]resource.StateUpgrader {
	pipelineV0 := pipelineSchemaV0()

	return map[int64]resource.StateUpgrader{
		// State upgrade implementation from Version 0 (prior Pipeline state version) to 1 (Schema.Version)
		0: {
			PriorSchema:   &pipelineV0,
			StateUpgrader: upgradePipelineStateV0toV1,
		},
	}
}

func (p *pipelineResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Don't modify on destroy, but otherwise make sure the steps default is preserved
	if !req.Plan.Raw.IsNull() {
		var template, steps types.String

		// Load pipeline_template_id and steps (if defined)
		req.Plan.GetAttribute(ctx, path.Root("pipeline_template_id"), &template)
		req.Plan.GetAttribute(ctx, path.Root("steps"), &steps)

		// Set default steps only if there is no template or defined steps
		if template.IsNull() && (steps.IsUnknown() || steps.IsNull()) {
			resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("steps"), defaultSteps)...)
		}
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
		Color:                                plan.Color.ValueStringPointer(),
		ClusterId:                            plan.ClusterId.ValueStringPointer(),
		DefaultBranch:                        plan.DefaultBranch.ValueString(),
		DefaultTimeoutInMinutes:              defaultTimeoutInMinutes,
		Emoji:                                plan.Emoji.ValueStringPointer(),
		MaximumTimeoutInMinutes:              maxTimeoutInMinutes,
		Description:                          plan.Description.ValueString(),
		Id:                                   plan.Id.ValueString(),
		Name:                                 plan.Name.ValueString(),
		PipelineTemplateId:                   plan.PipelineTemplateId.ValueString(),
		Repository:                           PipelineRepositoryInput{Url: plan.Repository.ValueString()},
		SkipIntermediateBuilds:               plan.SkipIntermediateBuilds.ValueBool(),
		SkipIntermediateBuildsBranchFilter:   plan.SkipIntermediateBuildsBranchFilter.ValueString(),
		Steps:                                PipelineStepsInput{Yaml: plan.Steps.ValueString()},
		Tags:                                 getTagsFromSchema(&plan),
	}

	timeouts, diags := p.client.timeouts.Update(ctx, DefaultTimeout)
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
		resp.Diagnostics.AddError(
			"Unable to update Pipeline",
			fmt.Sprintf("Unable to update Pipeline: %s", err.Error()),
		)
		return
	}

	setPipelineModel(&state, &response.PipelineUpdate.Pipeline)

	if plan.DefaultTeamId.IsNull() && !state.DefaultTeamId.IsNull() {
		// if the plan is empty but was previously set, just remove the team
		err = p.findAndRemoveTeam(ctx, state.DefaultTeamId.ValueString(), state.Slug.ValueString(), "")
		if err != nil {
			resp.Diagnostics.AddError("Could not remove default team", err.Error())
			return
		}

		state.DefaultTeamId = types.StringNull()
	} else if plan.DefaultTeamId.ValueString() != state.DefaultTeamId.ValueString() {
		// If the planned default_team_id differs from the state, add the new one and remove the old one
		var r *createTeamPipelineResponse
		err := retry.RetryContext(ctx, timeouts, func() *retry.RetryError {
			var err error
			r, err = createTeamPipeline(ctx, p.client.genqlient, plan.DefaultTeamId.ValueString(), state.Id.ValueString(), PipelineAccessLevelsManageBuildAndRead)
			return retryContextError(err)
		})

		if err != nil {
			resp.Diagnostics.AddError("Could not attach new default team to pipeline", err.Error())
			return
		}

		// update default team in state
		previousTeamID := state.DefaultTeamId.ValueString()
		state.DefaultTeamId = types.StringValue(r.TeamPipelineCreate.TeamPipelineEdge.Node.Team.Id)

		// remove the old team
		err = p.findAndRemoveTeam(ctx, previousTeamID, state.Slug.ValueString(), "")
		if err != nil {
			resp.Diagnostics.AddError("Could not remove previous default team", err.Error())
			return
		}
	}

	if plan.ProviderSettings != nil {
		pipelineExtraInfo, err := updatePipelineExtraInfo(ctx, response.PipelineUpdate.Pipeline.Slug, plan.ProviderSettings, p.client, timeouts)
		if err != nil {
			resp.Diagnostics.AddError("Unable to set pipeline info from REST", err.Error())
			return
		}

		updatePipelineResourceExtraInfo(&state, &pipelineExtraInfo)
		// set the webhook url if its not empty
		// the value can be empty if not using a token with appropriate permissions. in this case, we just leave the
		// state value alone assuming it was previously set correctly
		if pipelineExtraInfo.Provider.WebhookUrl != "" {
			state.WebhookUrl = types.StringValue(pipelineExtraInfo.Provider.WebhookUrl)
		}
	} else {
		// no provider_settings provided, but we still need to read in the badge url
		extraInfo, err := getPipelineExtraInfo(ctx, p.client, response.PipelineUpdate.Pipeline.Slug, timeouts)
		if err != nil {
			resp.Diagnostics.AddError("Unable to read pipeline info from REST", err.Error())
			return
		}
		state.BadgeUrl = types.StringValue(extraInfo.BadgeUrl)
		// set the webhook url if its not empty
		// the value can be empty if not using a token with appropriate permissions. in this case, we just leave the
		// state value alone assuming it was previously set correctly
		if extraInfo.Provider.WebhookUrl != "" {
			state.WebhookUrl = types.StringValue(extraInfo.Provider.WebhookUrl)
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// findAndRemoveTeam will try to find a team and remove its access from the pipeline
// we only know the teams ID but the API request to remove access requies the pipeline team connection ID, so we need to
// query all connected teams and check their ID matches
func (p *pipelineResource) findAndRemoveTeam(ctx context.Context, teamID string, pipelineSlug string, cursor string) error {
	slug := fmt.Sprintf("%s/%s", p.client.organization, pipelineSlug)
	teams, err := getPipelineTeams(ctx, p.client.genqlient, slug, cursor)
	if err != nil {
		return err
	}

	for _, team := range teams.Pipeline.Teams.Edges {
		if team.Node.Team.Id == teamID {
			_, err := deleteTeamPipeline(ctx, p.client.genqlient, team.Node.Id)
			if err != nil {
				return err
			}
			break
		}
	}

	// if there are more teams, recurse again with the next page
	if teams.Pipeline.Teams.PageInfo.HasNextPage {
		return p.findAndRemoveTeam(ctx, teamID, pipelineSlug, teams.Pipeline.Teams.PageInfo.EndCursor)
	}
	return nil
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
	model.Color = types.StringPointerValue(data.GetColor())
	model.DefaultBranch = types.StringValue(data.GetDefaultBranch())
	model.DefaultTimeoutInMinutes = types.Int64PointerValue(defaultTimeoutInMinutes)
	model.Description = types.StringValue(data.GetDescription())
	model.Emoji = types.StringPointerValue(data.GetEmoji())
	model.Id = types.StringValue(data.GetId())
	model.MaximumTimeoutInMinutes = types.Int64PointerValue(maximumTimeoutInMinutes)
	model.Name = types.StringValue(data.GetName())
	model.Repository = types.StringValue(data.GetRepository().Url)
	model.PipelineTemplateId = types.StringPointerValue(data.GetPipelineTemplate().Id)
	model.SkipIntermediateBuilds = types.BoolValue(data.GetSkipIntermediateBuilds())
	model.SkipIntermediateBuildsBranchFilter = types.StringValue(data.GetSkipIntermediateBuildsBranchFilter())
	model.Slug = types.StringValue(data.GetSlug())
	model.Steps = types.StringValue(data.GetSteps().Yaml)
	model.UUID = types.StringValue(data.GetPipelineUuid())

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
		WebhookUrl string                `json:"webhook_url"`
		Settings   PipelineExtraSettings `json:"settings"`
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

	state.ProviderSettings = &providerSettingsModel{
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
	}
}

func upgradePipelineStateV0toV1(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	type pipelineResourceModelV0 struct {
		AllowRebuilds                        types.Bool               `tfsdk:"allow_rebuilds"`
		BadgeUrl                             types.String             `tfsdk:"badge_url"`
		BranchConfiguration                  types.String             `tfsdk:"branch_configuration"`
		CancelIntermediateBuilds             types.Bool               `tfsdk:"cancel_intermediate_builds"`
		CancelIntermediateBuildsBranchFilter types.String             `tfsdk:"cancel_intermediate_builds_branch_filter"`
		ClusterId                            types.String             `tfsdk:"cluster_id"`
		DefaultBranch                        types.String             `tfsdk:"default_branch"`
		DefaultTeamId                        types.String             `tfsdk:"default_team_id"`
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

	var priorPipelineStateData pipelineResourceModelV0

	resp.Diagnostics.Append(req.State.Get(ctx, &priorPipelineStateData)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Create a new pipelineResourceModel instance with fields from state
	upgradedPipelineStateData := pipelineResourceModel{
		AllowRebuilds:                        priorPipelineStateData.AllowRebuilds,
		BadgeUrl:                             priorPipelineStateData.BadgeUrl,
		BranchConfiguration:                  priorPipelineStateData.BranchConfiguration,
		CancelIntermediateBuilds:             priorPipelineStateData.CancelIntermediateBuilds,
		CancelIntermediateBuildsBranchFilter: priorPipelineStateData.CancelIntermediateBuildsBranchFilter,
		ClusterId:                            priorPipelineStateData.ClusterId,
		DefaultBranch:                        priorPipelineStateData.DefaultBranch,
		DefaultTeamId:                        priorPipelineStateData.DefaultTeamId,
		DefaultTimeoutInMinutes:              priorPipelineStateData.DefaultTimeoutInMinutes,
		Description:                          priorPipelineStateData.Description,
		Id:                                   priorPipelineStateData.Id,
		MaximumTimeoutInMinutes:              priorPipelineStateData.MaximumTimeoutInMinutes,
		Name:                                 priorPipelineStateData.Name,
		Repository:                           priorPipelineStateData.Repository,
		SkipIntermediateBuilds:               priorPipelineStateData.SkipIntermediateBuilds,
		SkipIntermediateBuildsBranchFilter:   priorPipelineStateData.SkipIntermediateBuildsBranchFilter,
		Slug:                                 priorPipelineStateData.Slug,
		Steps:                                priorPipelineStateData.Steps,
		Tags:                                 priorPipelineStateData.Tags,
		WebhookUrl:                           priorPipelineStateData.WebhookUrl,
	}

	// If the existing pipelines' state had ProviderSettings - set it as part of the V1 pipelineResourceModel
	if len(priorPipelineStateData.ProviderSettings) == 1 {
		upgradedPipelineStateData.ProviderSettings = priorPipelineStateData.ProviderSettings[0]
	}

	// Upgrade pipeline in state
	diags := resp.State.Set(ctx, upgradedPipelineStateData)
	resp.Diagnostics.Append(diags...)
}

func pipelineSchemaV0() schema.Schema {
	return schema.Schema{
		MarkdownDescription: heredoc.Doc(`
			This resource allows you to create and manage pipelines for repositories.

			More information on pipelines can be found in the documentation](https://buildkite.com/docs/pipelines).
		`),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"allow_rebuilds": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
			"cancel_intermediate_builds": schema.BoolAttribute{
				Computed: true,
				Optional: true,
			},
			"cancel_intermediate_builds_branch_filter": schema.StringAttribute{
				Computed: true,
				Optional: true,
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
			},
			"default_team_id": schema.StringAttribute{
				MarkdownDescription: "The GraphQL ID of the team to use as the default owner of the pipeline.",
				Optional:            true,
			},
			"default_timeout_in_minutes": schema.Int64Attribute{
				Computed: true,
				Optional: true,
				Default:  nil,
			},
			"maximum_timeout_in_minutes": schema.Int64Attribute{
				Computed: true,
				Optional: true,
				Default:  nil,
			},
			"description": schema.StringAttribute{
				Computed: true,
				Optional: true,
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
			},
			"skip_intermediate_builds_branch_filter": schema.StringAttribute{
				Computed: true,
				Optional: true,
			},
			"slug": schema.StringAttribute{
				Computed: true,
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
			},
			"webhook_url": schema.StringAttribute{
				Computed: true,
			},
			"badge_url": schema.StringAttribute{
				Computed: true,
			},
		},
		Blocks: map[string]schema.Block{
			"provider_settings": schema.ListNestedBlock{
				MarkdownDescription: "Control settings depending on the VCS provider used in `repository`.",
				Validators: []validator.List{
					listvalidator.SizeBetween(0, 1),
				},
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"trigger_mode": schema.StringAttribute{
							Computed: true,
							Optional: true,
						},
						"build_pull_requests": schema.BoolAttribute{
							Optional: true,
							Computed: true,
						},
						"pull_request_branch_filter_enabled": schema.BoolAttribute{
							Computed: true,
							Optional: true,
						},
						"pull_request_branch_filter_configuration": schema.StringAttribute{
							Computed: true,
							Optional: true,
						},
						"skip_builds_for_existing_commits": schema.BoolAttribute{
							Optional: true,
							Computed: true,
						},
						"skip_pull_request_builds_for_existing_commits": schema.BoolAttribute{
							Optional: true,
							Computed: true,
						},
						"build_pull_request_ready_for_review": schema.BoolAttribute{
							Computed: true,
							Optional: true,
						},
						"build_pull_request_labels_changed": schema.BoolAttribute{
							Computed: true,
							Optional: true,
						},
						"build_pull_request_forks": schema.BoolAttribute{
							Computed: true,
							Optional: true,
						},
						"prefix_pull_request_fork_branch_names": schema.BoolAttribute{
							Computed: true,
							Optional: true,
						},
						"build_branches": schema.BoolAttribute{
							Optional: true,
							Computed: true,
						},
						"build_tags": schema.BoolAttribute{
							Computed: true,
							Optional: true,
						},
						"cancel_deleted_branch_builds": schema.BoolAttribute{
							Computed: true,
							Optional: true,
						},
						"filter_enabled": schema.BoolAttribute{
							Computed: true,
							Optional: true,
						},
						"filter_condition": schema.StringAttribute{
							Computed: true,
							Optional: true,
						},
						"publish_commit_status": schema.BoolAttribute{
							Optional: true,
							Computed: true,
						},
						"publish_blocked_as_pending": schema.BoolAttribute{
							Computed: true,
							Optional: true,
						},
						"publish_commit_status_per_step": schema.BoolAttribute{
							Computed: true,
							Optional: true,
						},
						"separate_pull_request_statuses": schema.BoolAttribute{
							Computed: true,
							Optional: true,
						},
					},
				},
			},
		},
	}
}
