package buildkite

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"time"
	"unsafe"

	"github.com/MakeNowJust/heredoc"
	custom_modifier "github.com/buildkite/terraform-provider-buildkite/internal/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
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
	BadgeUrl                             graphql.String `graphql:"badgeURL"`
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
	ClusterName                          types.String           `tfsdk:"cluster_name"`
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
	IgnoreDefaultBranchPullRequests         types.Bool   `tfsdk:"ignore_default_branch_pull_requests"`
}

type pipelineResource struct {
	client          *Client
	archiveOnDelete *bool
}

type pipelineResponse interface {
	GetId() string
	GetPipelineUuid() string
	GetAllowRebuilds() bool
	GetBadgeURL() string
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
	GetWebhookURL() string
}

func newPipelineResource(archiveOnDelete *bool) func() resource.Resource {
	return func() resource.Resource {
		return &pipelineResource{
			archiveOnDelete: archiveOnDelete,
		}
	}
}

// validateFilterConditionWithTriggerMode checks if filter_condition or filter_enabled is set
// when trigger_mode is "none" and adds a warning if so
func validateFilterConditionWithTriggerMode(providerSettings *providerSettingsModel, diagnostics *diag.Diagnostics) {
	if providerSettings == nil {
		return
	}

	filterConditionSet := !providerSettings.FilterCondition.IsNull() && providerSettings.FilterCondition.ValueString() != ""
	filterEnabledSet := !providerSettings.FilterEnabled.IsNull() && providerSettings.FilterEnabled.ValueBool()

	if filterConditionSet || filterEnabledSet {
		if providerSettings.TriggerMode.IsNull() || providerSettings.TriggerMode.ValueString() == "none" {
			diagnostics.AddWarning(
				"`filter_condition` requires `trigger_mode` to be `code`",
				"The `filter_condition` and `filter_enabled` attributes are only applicable when `trigger_mode` is set to `code`. They will be ignored when `trigger_mode` is `none` or not configured.",
			)
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

	validateFilterConditionWithTriggerMode(plan.ProviderSettings, &resp.Diagnostics)

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
	state.DefaultTeamId = plan.DefaultTeamId

	useSlugValue := response.PipelineCreate.Pipeline.Slug
	resp.Diagnostics.Append(resp.Private.SetKey(ctx, "slugSource", []byte(`{"source": "api"}`))...)
	if len(plan.Slug.ValueString()) > 0 {
		useSlugValue = plan.Slug.ValueString()

		pipelineExtraInfo, err := updatePipelineSlug(ctx, response.PipelineCreate.Pipeline.Slug, useSlugValue, p.client, timeouts)
		if err != nil {
			resp.Diagnostics.AddError("Unable to set pipeline slug from REST", err.Error())
			return
		}

		updatePipelineResourceExtraInfo(&state, &pipelineExtraInfo)
		resp.Diagnostics.Append(resp.Private.SetKey(ctx, "slugSource", []byte(`{"source": "user"}`))...)
	}

	if plan.ProviderSettings != nil {
		pipelineExtraInfo, err := updatePipelineExtraInfo(ctx, useSlugValue, plan.ProviderSettings, p.client, timeouts)
		if err != nil {
			resp.Diagnostics.AddError("Unable to set pipeline info from REST", err.Error())
			return
		}

		updatePipelineResourceExtraInfo(&state, &pipelineExtraInfo)
	} else {
		// no provider_settings provided
		state.ProviderSettings = plan.ProviderSettings
	}

	state.Slug = types.StringValue(useSlugValue)
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

		if err != nil && isResourceNotFoundError(err) {
			return nil
		}

		if err != nil && isActiveJobsError(err) {
			log.Printf("Pipeline %s has active jobs, retrying deletion...", state.Name.ValueString())
			return retry.RetryableError(err)
		}

		return retryContextError(err)
	})

	if err != nil {
		errorMsg := fmt.Sprintf("Could not delete pipeline: %s", err.Error())
		if isActiveJobsError(err) {
			errorMsg = fmt.Sprintf("Could not delete pipeline %s due to active jobs/builds: %s. Please wait for jobs to complete or cancel them manually before retrying.",
				state.Name.ValueString(), err.Error())
		}
		resp.Diagnostics.AddError("Could not delete pipeline", errorMsg)
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

		if state.ProviderSettings != nil {
			updatePipelineResourceExtraInfo(&state, extraInfo)
		}

		// pipeline default team is a terraform concept only so it takes some coercing
		teamResult, err := p.setDefaultTeamIfExists(ctx, &state, &pipelineNode.Teams.PipelineTeam)
		if err != nil {
			resp.Diagnostics.AddError("Error with default team configuration", err.Error())
			return
		}

		// Add warning if team was added back to enforce Terraform configuration
		if teamResult != nil && teamResult.teamAddedBack {
			resp.Diagnostics.AddWarning(
				"Default team missing from pipeline",
				fmt.Sprintf("The default team (ID: %s) is missing from the pipeline but exists in your organization. The team has been removed from state - run 'terraform apply' to add it back with Full Access.",
					teamResult.originalTeamId),
			)
		}

		// Add warning if team permissions were reduced
		if teamResult != nil && teamResult.reducedPermission {
			resp.Diagnostics.AddWarning(
				"Default team permission level reduced",
				fmt.Sprintf("The default team (ID: %s) no longer has Full Access (current level: %s). This may cause issues with pipeline updates, team management, or build triggering. Consider updating the team's permissions in the Buildkite UI or using a different team with Full Access as the default.",
					teamResult.originalTeamId, teamResult.accessLevel),
			)
		}

		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	} else {
		// no pipeline was found so remove it from state
		resp.Diagnostics.AddWarning("Pipeline not found", "Removing pipeline from state")
		resp.State.RemoveResource(ctx)
	}
}

type teamResult struct {
	reducedPermission bool
	teamAddedBack     bool
	originalTeamId    string
	accessLevel       string
}

// setDefaultTeamIfExists enforces the terraform configuration as the source of truth for team assignments
// - If no team is configured (null or unset), it does nothing, preserving user choice
// - If the configured team doesn't exist globally, it explicity fails
// - If the team exists but was removed from the pipeline, it sets state to null to trigger update
// - If the team exists on the pipeline but with reduced permissions, it warns but keeps the team
func (p *pipelineResource) setDefaultTeamIfExists(ctx context.Context, state *pipelineResourceModel, pipelineTeam *PipelineTeam) (*teamResult, error) {
	return p.setDefaultTeamIfExistsWithCandidate(ctx, state, pipelineTeam)
}

// setDefaultTeamIfExistsWithCandidate handles the logic for enforcing the Terraform configuration as source of truth
func (p *pipelineResource) setDefaultTeamIfExistsWithCandidate(ctx context.Context, state *pipelineResourceModel, pipelineTeam *PipelineTeam) (*teamResult, error) {
	result := &teamResult{}

	// Only enforce team validation if the user has explicitly configured a team ID
	// This preserves cases where users have set default_team_id = null or haven't set it at all
	if !state.DefaultTeamId.IsNull() {
		result.originalTeamId = state.DefaultTeamId.ValueString()

		// First, check if the team exists globally using getNode
		teamResponse, err := getNode(ctx, p.client.genqlient, result.originalTeamId)
		if err != nil {
			return nil, fmt.Errorf("failed to check if team exists: %w", err)
		}

		// If the team doesn't exist globally, fail the operation
		if teamResponse.Node == nil {
			return nil, fmt.Errorf("team with ID %s does not exist in the organization", result.originalTeamId)
		}

		// Check if it's actually a team node
		if _, ok := teamResponse.Node.(*getNodeNodeTeam); !ok {
			return nil, fmt.Errorf("ID %s does not refer to a team", result.originalTeamId)
		}

		// Check if the team is attached to the pipeline
		var foundAccessLevel *PipelineAccessLevels

		// Loop over all attached teams to find the current default team
		for _, team := range pipelineTeam.Edges {
			if state.DefaultTeamId.ValueString() == team.Node.Team.Id {
				foundAccessLevel = &team.Node.AccessLevel
				break
			}
		}

		// If there are multiple pages of teams, keep checking until we find the team or exhaust all pages
		if foundAccessLevel == nil && pipelineTeam.PageInfo.HasNextPage {
			resp, err := getPipelineTeams(ctx, p.client.genqlient, state.Slug.ValueString(), pipelineTeam.PageInfo.EndCursor)
			if err != nil {
				return nil, err
			}
			pt := resp.Pipeline.Teams.PipelineTeam
			return p.setDefaultTeamIfExistsWithCandidate(ctx, state, &pt)
		}

		// Handle the different scenarios based on what we found
		if foundAccessLevel == nil {
			// Team exists globally but is not attached to the pipeline, mark state as requiring drift correction
			state.DefaultTeamId = types.StringNull()
			result.teamAddedBack = true
		} else if *foundAccessLevel != PipelineAccessLevelsManageBuildAndRead {
			// The team exists on the pipeline but no longer has Full Access, so we will warn the user and respect the UI configuration
			result.reducedPermission = true
			result.accessLevel = string(*foundAccessLevel)
		}
		// No changes needed
	}

	return result, nil
}

func (*pipelineResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Version: 1,
		MarkdownDescription: heredoc.Doc(`
			This resource allows you to create and manage pipelines for repositories.

			More information on pipelines can be found in the [documentation](https://buildkite.com/docs/pipelines).

			-> **Note:** When creating a new pipeline, the Buildkite API requires at least one team to be associated with it. You must use the 'default_team_id' attribute to specify this initial team. The 'buildkite_pipeline_team' resource can then be used to manage team access for existing pipelines.
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
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^\#[a-zA-Z0-9]{6}$`),
						"must be a valid color hex code (#000000)",
					),
				},
			},
			"cluster_id": schema.StringAttribute{
				MarkdownDescription: "Attach this pipeline to the given cluster GraphQL ID.",
				Optional:            true,
			},
			"cluster_name": schema.StringAttribute{
				MarkdownDescription: "The name of the cluster the pipeline is (optionally) attached to.",
				Computed:            true,
			},
			"default_team_id": schema.StringAttribute{
				MarkdownDescription: "The GraphQL ID of a team to initially assign to the pipeline. This is required by the Buildkite API when creating a new pipeline. The team assigned here will be given 'Manage Build and Read' access. Further team associations can be managed with the `buildkite_pipeline_team` resource after the pipeline is created.",
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
				Computed:            true,
				Default:             stringdefault.StaticString(""),
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
				Optional:            true,
				MarkdownDescription: "A custom identifier for the pipeline. If provided, this slug will be used as the pipeline's URL path instead of automatically converting the pipeline name. If not provided, the slug will be [derived](https://buildkite.com/docs/apis/graphql/cookbooks/pipelines#create-a-pipeline-deriving-a-pipeline-slug-from-the-pipelines-name) from the pipeline `name`.",
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 100),
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^[a-z0-9\-]+$`),
						"can only contain lowercase characters, numbers and hyphens",
					),
				},
				PlanModifiers: []planmodifier.String{
					custom_modifier.UseDerivedPipelineSlug(),
				},
			},
			"steps": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The YAML steps to configure for the pipeline. Can also accept the `steps` attribute from the [`buildkite_signed_pipeline_steps`](/docs/data-sources/signed_pipeline_steps) data source to enable a signed pipeline. Defaults to `buildkite-agent pipeline upload`.",
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
						MarkdownDescription: "Whether to create builds for commits that are part of a pull request.",
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
						MarkdownDescription: "Whether to skip creating a new build for a pull request if an existing build for the commit and branch already exists.",
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
						MarkdownDescription: "Whether to create builds when branches are pushed.",
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
						MarkdownDescription: "The condition to evaluate when deciding if a build should run. This is only valid when `trigger_mode` is `code`. " +
							"More details available in [the documentation](https://buildkite.com/docs/pipelines/conditionals#conditionals-in-pipelines).",
					},
					"publish_commit_status": schema.BoolAttribute{
						Optional:            true,
						Computed:            true,
						MarkdownDescription: "Whether to update the status of commits in Bitbucket, GitHub, or GitLab.",
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
					"ignore_default_branch_pull_requests": schema.BoolAttribute{
						Computed:            true,
						Optional:            true,
						MarkdownDescription: "Whether to prevent caching pull requests with the source branch matching the default branch.",
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

// ModifyPlan will modify the plan for the pipeline resource to handle setting the steps and pipeline_template_id
// attributes.
//
// These attributes are mutually exclusive and steps has a default value which must be handled.
// The mutal exclusion is already validated on the schema, so this function needs to determine which mode is being used;
// either "template" mode or "steps" mode. "template" mode is only ever enabled explicitly if the value for the
// pipeline_template_id is non-null, whereas "steps" mode can be implied with both attributes being null or explicit
// with "steps" being non-null.
//
// To further complicate things, this function is called twice per run: first during the planning phase before TF prints
// out a diff to the user for confirmation. The req.Config may contain unknown values at this point that derive from
// other unknowns (think string interpolation, etc.). If the user accepts the plan, this is called again with
// wholly-known req.Config values with any unknowns that can be resolved. Note: there may still be unknowns.
//
// Reference: https://github.com/hashicorp/terraform/blob/55600d815e0cde1a19e9cd319f52e1247033b8e0/docs/resource-instance-change-lifecycle.md
func (p *pipelineResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// if the entire plan is null, the resource is planned for destruction and we dont need to do anything
	if req.Plan.Raw.IsNull() {
		return
	}

	var configTemplate, configSteps types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("pipeline_template_id"), &configTemplate)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("steps"), &configSteps)...)

	// if we don't know either, return early
	if configTemplate.IsUnknown() || configSteps.IsUnknown() {
		return
	}

	// we are in "template" mode if the value is known (not null, ie a literal string) or unknown (derived from another
	// resource)
	templateMode := !configTemplate.IsNull()
	// explict steps if the value is not null
	explicitStepsMode := !templateMode && (!configSteps.IsNull())

	// "template" mode is enabled explicitly, but the value can be derived from other resources, meaning its value could be
	// "unknown". But if it is "null", we know the user has elected not to use a template. This means we are in one of the
	// "steps" modes.
	if !templateMode {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("pipeline_template_id"), types.StringNull())...)
		// if steps is not supplied, we are in "implicit steps mode" and we can set the value to the default
		if !explicitStepsMode {
			log.Println("`steps` and `pipeline_template_id` are both null. Using implicit steps mode.")
			resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("steps"), defaultSteps)...)
			return
		}
		return
	}

	// reaching here, we know we are in "template" mode. the value could be known or unknown (derived). either way, we
	// do not need to change it. but we do need to empty out the steps
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("steps"), types.StringNull())...)
}

func (p *pipelineResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state pipelineResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	validateFilterConditionWithTriggerMode(plan.ProviderSettings, &resp.Diagnostics)

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
		PipelineTemplateId:                   plan.PipelineTemplateId.ValueStringPointer(),
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

	useSlugValue := response.PipelineUpdate.Pipeline.Slug
	resp.Diagnostics.Append(resp.Private.SetKey(ctx, "slugSource", []byte(`{"source": "api"}`))...)
	if len(plan.Slug.ValueString()) > 0 {
		useSlugValue = plan.Slug.ValueString()
		_, err := updatePipelineSlug(ctx, response.PipelineUpdate.Pipeline.Slug, useSlugValue, p.client, timeouts)
		if err != nil {
			resp.Diagnostics.AddError("Unable to set pipeline slug from REST", err.Error())
			return
		}

		resp.Diagnostics.Append(resp.Private.SetKey(ctx, "slugSource", []byte(`{"source": "user"}`))...)
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
		pipelineExtraInfo, err := updatePipelineExtraInfo(ctx, useSlugValue, plan.ProviderSettings, p.client, timeouts)
		if err != nil {
			resp.Diagnostics.AddError("Unable to set pipeline info from REST", err.Error())
			return
		}

		updatePipelineResourceExtraInfo(&state, &pipelineExtraInfo)
	} else {
		// no provider_settings provided
		state.ProviderSettings = plan.ProviderSettings
	}

	state.Slug = types.StringValue(useSlugValue)
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
	model.BadgeUrl = types.StringValue(data.GetBadgeURL())
	model.BranchConfiguration = types.StringPointerValue(data.GetBranchConfiguration())
	model.CancelIntermediateBuilds = types.BoolValue(data.GetCancelIntermediateBuilds())
	model.CancelIntermediateBuildsBranchFilter = types.StringValue(data.GetCancelIntermediateBuildsBranchFilter())
	model.ClusterId = types.StringPointerValue(data.GetCluster().Id)
	model.ClusterName = types.StringPointerValue(data.GetCluster().Name)
	model.Color = types.StringPointerValue(data.GetColor())
	model.DefaultBranch = types.StringValue(data.GetDefaultBranch())
	model.DefaultTimeoutInMinutes = types.Int64PointerValue(defaultTimeoutInMinutes)
	model.Description = types.StringValue(data.GetDescription())
	// Normalize null emoji from API to empty string to match schema default
	if emoji := data.GetEmoji(); emoji != nil {
		model.Emoji = types.StringValue(*emoji)
	} else {
		model.Emoji = types.StringValue("")
	}
	model.Id = types.StringValue(data.GetId())
	model.MaximumTimeoutInMinutes = types.Int64PointerValue(maximumTimeoutInMinutes)
	model.Name = types.StringValue(data.GetName())
	model.Repository = types.StringValue(data.GetRepository().Url)
	model.SkipIntermediateBuilds = types.BoolValue(data.GetSkipIntermediateBuilds())
	model.SkipIntermediateBuildsBranchFilter = types.StringValue(data.GetSkipIntermediateBuildsBranchFilter())
	model.UUID = types.StringValue(data.GetPipelineUuid())
	model.WebhookUrl = types.StringValue(data.GetWebhookURL())

	// only set template or steps. steps is always updated even if using a template, but its redundant and creates
	// complications later
	if data.GetPipelineTemplate().Id != nil {
		model.PipelineTemplateId = types.StringPointerValue(data.GetPipelineTemplate().Id)
		model.Steps = types.StringNull()
	} else {
		model.Steps = types.StringValue(data.GetSteps().Yaml)
		model.PipelineTemplateId = types.StringNull()
	}

	tags := make([]types.String, len(data.GetTags()))
	for i, tag := range data.GetTags() {
		tags[i] = types.StringValue(tag.Label)
	}
	model.Tags = tags
}

// As of December 23, 2024, `pipelineCreate` and `pipelineUpdate` GraphQL Mutations are lacking support the following properties:
// - provider_settings
// - slug
// We fallback to REST API for secondary calls to set/update these properties.

// PipelineExtraInfo is used to manage pipeline attributes that are not exposed via GraphQL API.
type PipelineExtraInfo struct {
	Provider struct {
		Settings PipelineExtraSettings `json:"settings"`
	} `json:"provider"`
	Slug string `json:"slug"`
}

type PipelineSlug struct {
	Slug string `json:"slug"`
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
	IgnoreDefaultBranchPullRequests         *bool   `json:"ignore_default_branch_pull_requests"`
}

func getPipelineExtraInfo(ctx context.Context, client *Client, slug string, timeouts time.Duration) (*PipelineExtraInfo, error) {
	var pipelineExtraInfo PipelineExtraInfo

	err := retry.RetryContext(ctx, timeouts, func() *retry.RetryError {
		err := client.makeRequest(ctx, "GET", fmt.Sprintf("/v2/organizations/%s/pipelines/%s", client.organization, slug), nil, &pipelineExtraInfo)
		return retryContextError(err)
	})
	if err != nil {
		return nil, err
	}

	return &pipelineExtraInfo, nil
}

func updatePipelineSlug(ctx context.Context, slug string, updatedSlug string, client *Client, timeouts time.Duration) (PipelineExtraInfo, error) {
	payload := PipelineSlug{
		Slug: updatedSlug,
	}

	var pipelineExtraInfo PipelineExtraInfo

	if len(updatedSlug) > 0 {
		err := retry.RetryContext(ctx, timeouts, func() *retry.RetryError {
			err := client.makeRequest(ctx, "PATCH", fmt.Sprintf("/v2/organizations/%s/pipelines/%s", client.organization, slug), payload, &pipelineExtraInfo)
			return retryContextError(err)
		})
		if err != nil {
			return pipelineExtraInfo, err
		}
	}
	return pipelineExtraInfo, nil
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
			IgnoreDefaultBranchPullRequests:         settings.IgnoreDefaultBranchPullRequests.ValueBoolPointer(),
		},
	}

	var pipelineExtraInfo PipelineExtraInfo
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
		IgnoreDefaultBranchPullRequests:         types.BoolPointerValue(s.IgnoreDefaultBranchPullRequests),
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
						"ignore_default_branch_pull_requests": schema.BoolAttribute{
							Computed: true,
							Optional: true,
						},
					},
				},
			},
		},
	}
}
