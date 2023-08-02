package buildkite

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
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
	Teams                                struct {
		Edges []struct {
			Node TeamPipelineNode
		}
	} `graphql:"teams(first: 50)"`
	WebhookURL graphql.String `graphql:"webhookURL"`
}

type PipelineTag struct {
	Label graphql.String
}

// TeamPipelineNode represents a team pipeline as returned from the GraphQL API
type TeamPipelineNode struct {
	AccessLevel PipelineAccessLevels
	ID          graphql.String
	Team        TeamNode
}

type pipelineResourceModel struct {
	AllowRebuilds                        types.Bool               `tfsdk:"allow_rebuilds"`
	ArchiveOnDelete                      types.Bool               `tfsdk:"archive_on_delete"`
	BadgeUrl                             types.String             `tfsdk:"badge_url"`
	BranchConfiguration                  types.String             `tfsdk:"branch_configuration"`
	CancelIntermediateBuilds             types.Bool               `tfsdk:"cancel_intermediate_builds"`
	CancelIntermediateBuildsBranchFilter types.String             `tfsdk:"cancel_intermediate_builds_branch_filter"`
	ClusterId                            types.String             `tfsdk:"cluster_id"`
	DefaultBranch                        types.String             `tfsdk:"default_branch"`
	DefaultTimeoutInMinutes              types.Int64              `tfsdk:"default_timeout_in_minutes"`
	DeletionProtection                   types.Bool               `tfsdk:"deletion_protection"`
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
	Teams                                []pipelineTeamModel      `tfsdk:"team"`
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

type pipelineTeamModel struct {
	Slug        types.String `tfsdk:"slug"`
	AccessLevel types.String `tfsdk:"access_level"`
}

type pipelineResource struct {
	client *Client
}

type pipelineResponse interface {
	GetId() string
	GetAllowRebuilds() bool
	GetBranchConfiguration() *string
	GetCancelIntermediateBuilds() bool
	GetCancelIntermediateBuildsBranchFilter() string
	GetCluster() PipelineValuesCluster
	GetDefaultBranch() string
	GetDefaultTimeoutInMinutes() int
	GetMaximumTimeoutInMinutes() int
	GetDescription() string
	GetName() string
	GetRepository() PipelineValuesRepository
	GetSkipIntermediateBuilds() bool
	GetSkipIntermediateBuildsBranchFilter() string
	GetSlug() string
	GetSteps() PipelineValuesStepsPipelineSteps
	GetTags() []PipelineValuesTagsPipelineTag
	GetTeams() PipelineValuesTeamsTeamPipelineConnection
	GetWebhookURL() string
}

func newPipelineResource() resource.Resource {
	return &pipelineResource{}
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

	var teamsInput []PipelineTeamAssignmentInput
	if len(plan.Teams) > 0 {
		teamsInput = p.getTeamPipelinesFromSchema(&plan)
	}
	if len(teamsInput) != len(plan.Teams) {
		resp.Diagnostics.AddError("Could not resolve all team IDs", "Could not resolve all team IDs")
		return
	}

	input := PipelineCreateInput{
		AllowRebuilds:                        plan.AllowRebuilds.ValueBool(),
		BranchConfiguration:                  plan.BranchConfiguration.ValueStringPointer(),
		CancelIntermediateBuilds:             plan.CancelIntermediateBuilds.ValueBool(),
		CancelIntermediateBuildsBranchFilter: plan.CancelIntermediateBuildsBranchFilter.ValueString(),
		ClusterId:                            plan.ClusterId.ValueStringPointer(),
		DefaultBranch:                        plan.DefaultBranch.ValueString(),
		DefaultTimeoutInMinutes:              int(plan.DefaultTimeoutInMinutes.ValueInt64()),
		MaximumTimeoutInMinutes:              int(plan.MaximumTimeoutInMinutes.ValueInt64()),
		Description:                          plan.Description.ValueString(),
		Name:                                 plan.Name.ValueString(),
		OrganizationId:                       p.client.organizationId,
		Repository:                           PipelineRepositoryInput{Url: plan.Repository.ValueString()},
		SkipIntermediateBuilds:               plan.SkipIntermediateBuilds.ValueBool(),
		SkipIntermediateBuildsBranchFilter:   plan.SkipIntermediateBuildsBranchFilter.ValueString(),
		Steps:                                PipelineStepsInput{Yaml: plan.Steps.ValueString()},
		Teams:                                teamsInput,
		Tags:                                 getTagsFromSchema(&plan),
	}

	log.Printf("Creating pipeline %s ...", plan.Name.ValueString())
	response, err := createPipeline(p.client.genqlient, input)

	if err != nil {
		resp.Diagnostics.AddError("Failed to create pipeline", err.Error())
		return
	}
	log.Printf("Successfully created pipeline with id '%s'.", response.PipelineCreate.Pipeline.Id)

	setPipelineModel(&state, &response.PipelineCreate.Pipeline)
	state.DeletionProtection = plan.DeletionProtection
	state.ArchiveOnDelete = plan.ArchiveOnDelete

	if len(plan.ProviderSettings) > 0 {
		pipelineExtraInfo, err := updatePipelineExtraInfo(response.PipelineCreate.Pipeline.Slug, plan.ProviderSettings[0], p.client)
		if err != nil {
			resp.Diagnostics.AddError("Unable to set pipeline info from REST", err.Error())
			return
		}

		updatePipelineResourceExtraInfo(&state, &pipelineExtraInfo)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (p *pipelineResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pipelineResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.ArchiveOnDelete.ValueBool() {
		log.Printf("Pipeline %s set to archive on delete. Archiving...", state.Name.ValueString())
		_, err := archivePipeline(p.client.genqlient, state.Id.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Could not archive pipeline", err.Error())
		}
		return
	}

	log.Printf("Deleting pipeline %s ...", state.Name.ValueString())
	_, err := deletePipeline(p.client.genqlient, state.Id.ValueString())
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

	response, err := getNode(p.client.genqlient, state.Id.ValueString())
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

		extraInfo, err := getPipelineExtraInfo(p.client, pipelineNode.Slug)
		if err != nil {
			resp.Diagnostics.AddError("Unable to read pipeline info from REST", err.Error())
			return
		}

		setPipelineModel(&state, pipelineNode)
		if len(state.ProviderSettings) > 0 {
			updatePipelineResourceExtraInfo(&state, extraInfo)
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	} else {
		// no pipeline was found so set an empty state
		resp.Diagnostics.Append(resp.State.Set(ctx, pipelineResourceModel{})...)
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
			"archive_on_delete": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
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
			"deletion_protection": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
				Description: "If set to 'true', deletion of a pipeline via `terraform destroy` will fail, until set to 'false'.",
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
			},
			"steps": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString(defaultSteps),
			},
			"tags": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
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
			"team": schema.SetNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"slug": schema.StringAttribute{
							Required: true,
						},
						"access_level": schema.StringAttribute{
							Required: true,
							Validators: []validator.String{
								stringvalidator.OneOf(string(PipelineAccessLevelsReadOnly), string(PipelineAccessLevelsBuildAndRead), string(PipelineAccessLevelsManageBuildAndRead)),
							},
						},
					},
				},
			},
			"provider_settings": schema.ListNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"trigger_mode": schema.StringAttribute{
							Computed: true,
							Optional: true,
							Validators: []validator.String{
								stringvalidator.OneOf("code", "deployment", "fork", "none"),
							},
						},
						"build_pull_requests": schema.BoolAttribute{
							Optional: true,
							Computed: true,
							Default:  booldefault.StaticBool(true),
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
							Default:  booldefault.StaticBool(true),
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
							Default:  booldefault.StaticBool(true),
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
							Default:  booldefault.StaticBool(true),
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

func (p *pipelineResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state pipelineResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := PipelineUpdateInput{
		AllowRebuilds:                        plan.AllowRebuilds.ValueBool(),
		BranchConfiguration:                  plan.BranchConfiguration.ValueStringPointer(),
		CancelIntermediateBuilds:             plan.CancelIntermediateBuilds.ValueBool(),
		CancelIntermediateBuildsBranchFilter: plan.CancelIntermediateBuildsBranchFilter.ValueString(),
		ClusterId:                            plan.ClusterId.ValueStringPointer(),
		DefaultBranch:                        plan.DefaultBranch.ValueString(),
		DefaultTimeoutInMinutes:              int(plan.DefaultTimeoutInMinutes.ValueInt64()),
		MaximumTimeoutInMinutes:              int(plan.MaximumTimeoutInMinutes.ValueInt64()),
		Description:                          plan.Description.ValueString(),
		Id:                                   plan.Id.ValueString(),
		Name:                                 plan.Name.ValueString(),
		Repository:                           PipelineRepositoryInput{Url: plan.Repository.ValueString()},
		SkipIntermediateBuilds:               plan.SkipIntermediateBuilds.ValueBool(),
		SkipIntermediateBuildsBranchFilter:   plan.SkipIntermediateBuildsBranchFilter.ValueString(),
		Steps:                                PipelineStepsInput{Yaml: plan.Steps.ValueString()},
		Tags:                                 getTagsFromSchema(&plan),
	}

	log.Printf("Updating pipeline %s ...", input.Name)
	response, err := updatePipeline(p.client.genqlient, input)

	if err != nil {
		resp.Diagnostics.AddError("Unable to update pipeline %s", state.Name.ValueString())
		return
	}

	setPipelineModel(&state, &response.PipelineUpdate.Pipeline)
	state.DeletionProtection = plan.DeletionProtection
	state.ArchiveOnDelete = plan.ArchiveOnDelete

	if len(plan.ProviderSettings) > 0 {
		pipelineExtraInfo, err := updatePipelineExtraInfo(response.PipelineUpdate.Pipeline.Slug, plan.ProviderSettings[0], p.client)
		if err != nil {
			resp.Diagnostics.AddError("Unable to set pipeline info from REST", err.Error())
			return
		}

		updatePipelineResourceExtraInfo(&state, &pipelineExtraInfo)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (*pipelineResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func setPipelineModel(model *pipelineResourceModel, data pipelineResponse) {
	model.AllowRebuilds = types.BoolValue(data.GetAllowRebuilds())
	model.BranchConfiguration = types.StringPointerValue(data.GetBranchConfiguration())
	model.CancelIntermediateBuilds = types.BoolValue(data.GetCancelIntermediateBuilds())
	model.CancelIntermediateBuildsBranchFilter = types.StringValue(data.GetCancelIntermediateBuildsBranchFilter())
	model.ClusterId = types.StringPointerValue(data.GetCluster().Id)
	model.DefaultBranch = types.StringValue(data.GetDefaultBranch())
	model.DefaultTimeoutInMinutes = types.Int64Value(int64(data.GetDefaultTimeoutInMinutes()))
	model.Description = types.StringValue(data.GetDescription())
	model.Id = types.StringValue(data.GetId())
	model.MaximumTimeoutInMinutes = types.Int64Value(int64(data.GetMaximumTimeoutInMinutes()))
	model.Name = types.StringValue(data.GetName())
	model.Repository = types.StringValue(data.GetRepository().Url)
	model.SkipIntermediateBuilds = types.BoolValue(data.GetSkipIntermediateBuilds())
	model.SkipIntermediateBuildsBranchFilter = types.StringValue(data.GetSkipIntermediateBuildsBranchFilter())
	model.Slug = types.StringValue(data.GetSlug())
	model.Steps = types.StringValue(data.GetSteps().Yaml)
	model.WebhookUrl = types.StringValue(data.GetWebhookURL())

	var tags []types.String
	if len(data.GetTags()) > 0 {
		tags = make([]types.String, len(data.GetTags()))
		for i, tag := range data.GetTags() {
			tags[i] = types.StringValue(tag.Label)
		}
	} else {
		tags = nil
	}
	model.Tags = tags
	teams := make([]pipelineTeamModel, len(data.GetTeams().Edges))
	for i, teamEdge := range data.GetTeams().Edges {
		teams[i] = pipelineTeamModel{
			Slug:        types.StringValue(teamEdge.Node.Team.Slug),
			AccessLevel: types.StringValue(string(teamEdge.Node.AccessLevel)),
		}
	}
	model.Teams = teams
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

func getPipelineExtraInfo(client *Client, slug string) (*PipelineExtraInfo, error) {
	pipelineExtraInfo := PipelineExtraInfo{}
	err := client.makeRequest("GET", fmt.Sprintf("/v2/organizations/%s/pipelines/%s", client.organization, slug), nil, &pipelineExtraInfo)
	if err != nil {
		return nil, err
	}
	return &pipelineExtraInfo, nil
}
func updatePipelineExtraInfo(slug string, settings *providerSettingsModel, client *Client) (PipelineExtraInfo, error) {
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
	err := client.makeRequest("PATCH", fmt.Sprintf("/v2/organizations/%s/pipelines/%s", client.organization, slug), payload, &pipelineExtraInfo)
	if err != nil {
		return pipelineExtraInfo, err
	}
	return pipelineExtraInfo, nil
}

func mapTagsFromGenqlient(tags []PipelineValuesTagsPipelineTag) []PipelineTag {
	newTags := make([]PipelineTag, len(tags))

	for i, v := range tags {
		newTags[i] = PipelineTag{Label: graphql.String(v.Label)}
	}
	return newTags
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

func mapTeamPipelinesFromGenqlient(tags []PipelineValuesTeamsTeamPipelineConnectionEdgesTeamPipelineEdge) struct {
	Edges []struct{ Node TeamPipelineNode }
} {
	teamPipelineNodes := make([]struct{ Node TeamPipelineNode }, len(tags))
	for i, v := range tags {
		teamPipelineNodes[i] = struct{ Node TeamPipelineNode }{
			Node: TeamPipelineNode{
				AccessLevel: v.Node.AccessLevel,
				ID:          graphql.String(v.Node.Id),
				Team: TeamNode{
					Slug: graphql.String(v.Node.Team.Slug),
				}},
		}
	}
	return struct {
		Edges []struct{ Node TeamPipelineNode }
	}{Edges: teamPipelineNodes}
}

func (p *pipelineResource) getTeamPipelinesFromSchema(plan *pipelineResourceModel) []PipelineTeamAssignmentInput {
	teamPipelineNodes := make([]PipelineTeamAssignmentInput, len(plan.Teams))
	for i, team := range plan.Teams {
		log.Printf("converting team slug '%s' to an ID", string(team.Slug.ValueString()))
		teamID, err := GetTeamID(string(team.Slug.ValueString()), p.client)
		if err != nil {
			log.Printf("Unable to get ID for team slug")
			return []PipelineTeamAssignmentInput{}
		}
		teamPipelineNodes[i] = PipelineTeamAssignmentInput{
			Id:          teamID,
			AccessLevel: PipelineAccessLevels(team.AccessLevel.ValueString()),
		}
	}
	return teamPipelineNodes
}

// reconcileTeamPipelines adds/updates/deletes the teamPipelines on buildkite to match the teams in terraform resource data
func reconcileTeamPipelines(teamPipelines []TeamPipelineNode, pipeline *PipelineNode, client *Client) error {
	teamPipelineIds := make(map[string]graphql.ID)

	var toAdd []TeamPipelineNode
	var toUpdate []TeamPipelineNode
	var toDelete []TeamPipelineNode

	// Look for teamPipelines on buildkite that need updated or removed
	for _, teamPipeline := range pipeline.Teams.Edges {
		teamSlugBk := teamPipeline.Node.Team.Slug
		accessLevelBk := teamPipeline.Node.AccessLevel
		id := teamPipeline.Node.ID

		teamPipelineIds[string(teamSlugBk)] = graphql.ID(id)

		found := false
		for _, teamPipeline := range teamPipelines {
			if teamPipeline.Team.Slug == teamSlugBk {
				found = true
				if teamPipeline.AccessLevel != accessLevelBk {
					toUpdate = append(toUpdate, TeamPipelineNode{
						AccessLevel: teamPipeline.AccessLevel,
						ID:          id,
						Team: TeamNode{
							Slug: teamPipeline.Team.Slug,
						},
					})
				}
			}
		}
		if !found {
			toDelete = append(toDelete, TeamPipelineNode{
				AccessLevel: accessLevelBk,
				ID:          id,
				Team: TeamNode{
					Slug: teamSlugBk,
				},
			})
		}
	}

	// Look for new teamsInput that need added to buildkite
	for _, teamPipeline := range teamPipelines {
		if _, found := teamPipelineIds[string(teamPipeline.Team.Slug)]; !found {
			toAdd = append(toAdd, teamPipeline)
		}
	}

	log.Printf("EXISTING_BUILDKITE_TEAMS: %s", teamPipelineIds)

	// Add any teamsInput that don't already exist
	err := createTeamPipelines(toAdd, string(pipeline.ID), client)
	if err != nil {
		return err
	}

	// Update any teamsInput access levels that need updating
	err = updateTeamPipelines(toUpdate, client)
	if err != nil {
		return err
	}

	// Remove any teamsInput that shouldn't exist
	err = deleteTeamPipelines(toDelete, client)
	if err != nil {
		return err
	}

	return nil
}

// createTeamPipelines grants access to a pipeline for the teams specified
func createTeamPipelines(teamPipelines []TeamPipelineNode, pipelineID string, client *Client) error {
	var mutation struct {
		TeamPipelineCreate struct {
			TeamPipeline struct {
				ID graphql.ID
			}
		} `graphql:"teamPipelineCreate(input: {teamID: $team, pipelineID: $pipeline, accessLevel: $accessLevel})"`
	}
	for _, teamPipeline := range teamPipelines {
		log.Printf("Granting teamPipeline %s %s access to pipeline id '%s'...", teamPipeline.Team.Slug, teamPipeline.AccessLevel, pipelineID)
		teamID, err := GetTeamID(string(teamPipeline.Team.Slug), client)
		if err != nil {
			return fmt.Errorf("Unable to get ID for team slug %s (%v)", teamPipeline.Team.Slug, err)
		}
		params := map[string]interface{}{
			"team":        graphql.ID(teamID),
			"pipeline":    graphql.ID(pipelineID),
			"accessLevel": teamPipeline.AccessLevel,
		}
		err = client.graphql.Mutate(context.Background(), &mutation, params)
		if err != nil {
			log.Printf("Unable to create team pipeline %s", teamPipeline.Team.Slug)
			return err
		}
	}
	return nil
}

// Update access levels for the given teamPipelines
func updateTeamPipelines(teamPipelines []TeamPipelineNode, client *Client) error {
	var mutation struct {
		TeamPipelineUpdate struct {
			TeamPipeline struct {
				ID graphql.ID
			}
		} `graphql:"teamPipelineUpdate(input: {id: $id, accessLevel: $accessLevel})"`
	}
	for _, teamPipeline := range teamPipelines {
		log.Printf("Updating access to %s for teamPipeline id '%s'...", teamPipeline.AccessLevel, teamPipeline.ID)
		params := map[string]interface{}{
			"id":          graphql.ID(string(teamPipeline.ID)),
			"accessLevel": teamPipeline.AccessLevel,
		}
		err := client.graphql.Mutate(context.Background(), &mutation, params)
		if err != nil {
			log.Printf("Unable to update team pipeline")
			return err
		}
	}
	return nil
}

func deleteTeamPipelines(teamPipelines []TeamPipelineNode, client *Client) error {
	var mutation struct {
		TeamPipelineDelete struct {
			Team struct {
				ID graphql.ID
			}
		} `graphql:"teamPipelineDelete(input: {id: $id})"`
	}
	for _, teamPipeline := range teamPipelines {
		log.Printf("Removing access for teamPipeline %s (id=%s)...", teamPipeline.Team.Slug, teamPipeline.ID)
		params := map[string]interface{}{
			"id": graphql.ID(string(teamPipeline.ID)),
		}
		err := client.graphql.Mutate(context.Background(), &mutation, params)
		if err != nil {
			log.Printf("Unable to delete team pipeline")
			return err
		}
	}

	return nil
}

// updatePipelineResourceExtraInfo updates the terraform resource with data received from Buildkite REST API
func updatePipelineResourceExtraInfo(state *pipelineResourceModel, pipeline *PipelineExtraInfo) {
	state.BadgeUrl = types.StringValue(pipeline.BadgeUrl)
	s := pipeline.Provider.Settings
	state.ProviderSettings = []*providerSettingsModel{
		&providerSettingsModel{
			TriggerMode:                             types.StringValue(*s.TriggerMode),
			BuildPullRequests:                       types.BoolValue(*s.BuildPullRequests),
			PullRequestBranchFilterEnabled:          types.BoolValue(*s.PullRequestBranchFilterEnabled),
			PullRequestBranchFilterConfiguration:    types.StringValue(*s.PullRequestBranchFilterConfiguration),
			SkipBuildsForExistingCommits:            types.BoolValue(*s.SkipBuildsForExistingCommits),
			SkipPullRequestBuildsForExistingCommits: types.BoolValue(*s.SkipPullRequestBuildsForExistingCommits),
			BuildPullRequestReadyForReview:          types.BoolValue(*s.BuildPullRequestReadyForReview),
			BuildPullRequestLabelsChanged:           types.BoolValue(*s.BuildPullRequestLabelsChanged),
			BuildPullRequestForks:                   types.BoolValue(*s.BuildPullRequestForks),
			PrefixPullRequestForkBranchNames:        types.BoolValue(*s.PrefixPullRequestForkBranchNames),
			BuildBranches:                           types.BoolValue(*s.BuildBranches),
			BuildTags:                               types.BoolValue(*s.BuildTags),
			CancelDeletedBranchBuilds:               types.BoolValue(*s.CancelDeletedBranchBuilds),
			FilterEnabled:                           types.BoolValue(*s.FilterEnabled),
			FilterCondition:                         types.StringValue(*s.FilterCondition),
			PublishCommitStatus:                     types.BoolValue(*s.PublishCommitStatus),
			PublishBlockedAsPending:                 types.BoolValue(*s.PublishBlockedAsPending),
			PublishCommitStatusPerStep:              types.BoolValue(*s.PublishCommitStatusPerStep),
			SeparatePullRequestStatuses:             types.BoolValue(*s.SeparatePullRequestStatuses),
		},
	}
}
