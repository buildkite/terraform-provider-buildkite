package buildkite

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
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

type PipelineTeamAssignmentInput struct {
	Id          graphql.ID     `json:"id"`
	AccessLevel graphql.String `json:"accessLevel"`
}

// resourcePipeline represents the terraform pipeline resource schema
func resourcePipeline() *schema.Resource {
	return &schema.Resource{
		CreateContext: CreatePipeline,
		ReadContext:   ReadPipeline,
		UpdateContext: UpdatePipeline,
		DeleteContext: DeletePipeline,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"allow_rebuilds": {
				Optional: true,
				Default: true,
				Type:     schema.TypeBool,
			},
			"archive_on_delete": {
				Optional: true,
				Default:  false,
				Type:     schema.TypeBool,
			},
			"cancel_intermediate_builds": {
				Computed: true,
				Optional: true,
				Type:     schema.TypeBool,
			},
			"cancel_intermediate_builds_branch_filter": {
				Computed: true,
				Optional: true,
				Type:     schema.TypeString,
			},
			"branch_configuration": {
				Optional: true,
				Type:     schema.TypeString,
			},
			"cluster_id": {
				Optional: true,
				Type:     schema.TypeString,
			},
			"default_branch": {
				Computed: true,
				Optional: true,
				Type:     schema.TypeString,
			},
			"default_timeout_in_minutes": {
				Computed: true,
				Optional: true,
				Default:  nil,
				Type:     schema.TypeInt,
			},
			"deletion_protection": {
				Optional:    true,
				Default:     false,
				Type:        schema.TypeBool,
				Description: "If set to 'true', deletion of a pipeline via `terraform destroy` will fail, until set to 'false'.",
			},
			"maximum_timeout_in_minutes": {
				Computed: true,
				Optional: true,
				Default:  nil,
				Type:     schema.TypeInt,
			},
			"description": {
				Computed: true,
				Optional: true,
				Type:     schema.TypeString,
			},
			"name": {
				Required: true,
				Type:     schema.TypeString,
			},
			"repository": {
				Required: true,
				Type:     schema.TypeString,
			},
			"skip_intermediate_builds": {
				Computed: true,
				Optional: true,
				Type:     schema.TypeBool,
			},
			"skip_intermediate_builds_branch_filter": {
				Computed: true,
				Optional: true,
				Type:     schema.TypeString,
			},
			"slug": {
				Computed: true,
				Type:     schema.TypeString,
			},
			"steps": {
				Optional: true,
				Default:  defaultSteps,
				Type:     schema.TypeString,
			},
			"team": {
				Type:       schema.TypeSet,
				Optional:   true,
				ConfigMode: schema.SchemaConfigModeAttr,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"slug": {
							Required: true,
							Type:     schema.TypeString,
						},
						"access_level": {
							Required: true,
							Type:     schema.TypeString,
							ValidateFunc: func(val interface{}, key string) (warns []string, errs []error) {
								switch v := val.(string); v {
								case "READ_ONLY":
								case "BUILD_AND_READ":
								case "MANAGE_BUILD_AND_READ":
									return
								default:
									errs = append(errs, fmt.Errorf("%q must be one of READ_ONLY, BUILD_AND_READ or MANAGE_BUILD_AND_READ, got: %s", key, v))
									return
								}
								return
							},
						},
					},
				},
			},
			"tags": {
				Type:     schema.TypeSet,
				Optional: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
				Set:      schema.HashString,
			},
			"provider_settings": {
				Type:     schema.TypeList,
				Optional: true,
				Computed: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"trigger_mode": {
							Computed: true,
							Optional: true,
							Type:     schema.TypeString,
							ValidateFunc: func(val interface{}, key string) (warns []string, errs []error) {
								switch v := val.(string); v {
								case "code":
								case "deployment":
								case "fork":
								case "none":
									return
								default:
									errs = append(errs, fmt.Errorf("%q must be one of code, deployment, fork or none, got: %s", key, v))
									return
								}
								return
							},
						},
						"build_pull_requests": {
							Optional: true,
							Type:     schema.TypeBool,
							Default:  true,
						},
						"pull_request_branch_filter_enabled": {
							Computed: true,
							Optional: true,
							Type:     schema.TypeBool,
						},
						"pull_request_branch_filter_configuration": {
							Computed: true,
							Optional: true,
							Type:     schema.TypeString,
						},
						"skip_builds_for_existing_commits": {
							Optional: true,
							Type:     schema.TypeBool,
						},
						"skip_pull_request_builds_for_existing_commits": {
							Optional: true,
							Type:     schema.TypeBool,
							Default:  true,
						},
						"build_pull_request_ready_for_review": {
							Computed: true,
							Optional: true,
							Type:     schema.TypeBool,
						},
						"build_pull_request_labels_changed": {
							Computed: true,
							Optional: true,
							Type:     schema.TypeBool,
						},
						"build_pull_request_forks": {
							Computed: true,
							Optional: true,
							Type:     schema.TypeBool,
						},
						"prefix_pull_request_fork_branch_names": {
							Computed: true,
							Optional: true,
							Type:     schema.TypeBool,
						},
						"build_branches": {
							Optional: true,
							Type:     schema.TypeBool,
							Default:  true,
						},
						"build_tags": {
							Computed: true,
							Optional: true,
							Type:     schema.TypeBool,
						},
						"cancel_deleted_branch_builds": {
							Computed: true,
							Optional: true,
							Type:     schema.TypeBool,
						},
						"filter_enabled": {
							Computed: true,
							Optional: true,
							Type:     schema.TypeBool,
						},
						"filter_condition": {
							Computed: true,
							Optional: true,
							Type:     schema.TypeString,
						},
						"publish_commit_status": {
							Optional: true,
							Type:     schema.TypeBool,
							Default:  true,
						},
						"publish_blocked_as_pending": {
							Computed: true,
							Optional: true,
							Type:     schema.TypeBool,
						},
						"publish_commit_status_per_step": {
							Computed: true,
							Optional: true,
							Type:     schema.TypeBool,
						},
						"separate_pull_request_statuses": {
							Computed: true,
							Optional: true,
							Type:     schema.TypeBool,
						},
					},
				},
			},
			"webhook_url": {
				Computed: true,
				Type:     schema.TypeString,
			},
			"badge_url": {
				Computed: true,
				Type:     schema.TypeString,
			},
		},
	}
}

// CreatePipeline creates a Buildkite pipeline
func CreatePipeline(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*Client)
	var err error

	teamPipelines := getTeamPipelinesFromSchema(d)
	var mutation struct {
		PipelineCreate struct {
			Pipeline PipelineNode
		} `graphql:"pipelineCreate(input: {allowRebuilds: $allow_rebuilds, branchConfiguration: $branch_configuration, cancelIntermediateBuilds: $cancel_intermediate_builds, cancelIntermediateBuildsBranchFilter: $cancel_intermediate_builds_branch_filter, defaultBranch: $default_branch, defaultTimeoutInMinutes: $default_timeout_in_minutes, maximumTimeoutInMinutes: $maximum_timeout_in_minutes, description: $desc, name: $name, organizationId: $org, repository: {url: $repository_url}, skipIntermediateBuilds: $skip_intermediate_builds, skipIntermediateBuildsBranchFilter: $skip_intermediate_builds_branch_filter, steps: {yaml: $steps}, teams: $teams, tags: $tags})"`
	}

	teamsData := make([]PipelineTeamAssignmentInput, 0)
	for _, team := range teamPipelines {
		log.Printf("converting team slug '%s' to an ID", string(team.Team.Slug))
		teamID, err := GetTeamID(string(team.Team.Slug), client)
		if err != nil {
			return diag.FromErr(fmt.Errorf("Unable to get ID for team slug %s (%v)", team.Team.Slug, err))
		}
		teamsData = append(teamsData, PipelineTeamAssignmentInput{
			Id:          teamID,
			AccessLevel: graphql.String(team.AccessLevel),
		})
	}

	vars := map[string]interface{}{
		"allow_rebuilds":                           graphql.Boolean(d.Get("allow_rebuilds").(bool)),
		"branch_configuration":                     graphql.String(d.Get("branch_configuration").(string)),
		"cancel_intermediate_builds":               graphql.Boolean(d.Get("cancel_intermediate_builds").(bool)),
		"cancel_intermediate_builds_branch_filter": graphql.String(d.Get("cancel_intermediate_builds_branch_filter").(string)),
		"default_branch":                           graphql.String(d.Get("default_branch").(string)),
		"default_timeout_in_minutes":               graphql.Int(d.Get("default_timeout_in_minutes").(int)),
		"maximum_timeout_in_minutes":               graphql.Int(d.Get("maximum_timeout_in_minutes").(int)),
		"desc":                                     graphql.String(d.Get("description").(string)),
		"name":                                     graphql.String(d.Get("name").(string)),
		"org":                                      client.organizationId,
		"repository_url":                           graphql.String(d.Get("repository").(string)),
		"skip_intermediate_builds":                 graphql.Boolean(d.Get("skip_intermediate_builds").(bool)),
		"skip_intermediate_builds_branch_filter":   graphql.String(d.Get("skip_intermediate_builds_branch_filter").(string)),
		"steps":                                    graphql.String(d.Get("steps").(string)),
		"teams":                                    teamsData,
		"tags":                                     getTagsFromSchema(d),
	}

	log.Printf("Creating pipeline %s ...", vars["name"])

	// If the cluster_id key is present in the mutation, GraphQL expects a valid ID.
	// Check if cluster_id exists in the configuration before adding to mutation.
	if clusterID, ok := d.GetOk("cluster_id"); ok {
		var mutationWithClusterID struct {
			PipelineCreate struct {
				Pipeline PipelineNode
			} `graphql:"pipelineCreate(input: {allowRebuilds: $allow_rebuilds, branchConfiguration: $branch_configuration, cancelIntermediateBuilds: $cancel_intermediate_builds, cancelIntermediateBuildsBranchFilter: $cancel_intermediate_builds_branch_filter, clusterId: $cluster_id, defaultBranch: $default_branch, defaultTimeoutInMinutes: $default_timeout_in_minutes, description: $desc, maximumTimeoutInMinutes: $maximum_timeout_in_minutes, name: $name, organizationId: $org, repository: {url: $repository_url}, skipIntermediateBuilds: $skip_intermediate_builds, skipIntermediateBuildsBranchFilter: $skip_intermediate_builds_branch_filter, steps: {yaml: $steps}, teams: $teams, tags: $tags})"`
		}
		vars["cluster_id"] = graphql.ID(clusterID.(string))
		err = client.graphql.Mutate(context.Background(), &mutationWithClusterID, vars)
		mutation.PipelineCreate.Pipeline = mutationWithClusterID.PipelineCreate.Pipeline
	} else {
		err = client.graphql.Mutate(context.Background(), &mutation, vars)
	}

	if err != nil {
		log.Printf("Unable to create pipeline %s", d.Get("name"))
		return diag.FromErr(err)
	}
	log.Printf("Successfully created pipeline with id '%s'.", mutation.PipelineCreate.Pipeline.ID)

	updatePipelineResource(d, &mutation.PipelineCreate.Pipeline)

	pipelineExtraInfo, err := updatePipelineExtraInfo(d, client)
	if err != nil {
		return diag.FromErr(err)
	}
	updatePipelineResourceExtraInfo(d, &pipelineExtraInfo)

	return ReadPipeline(ctx, d, m)
}

// ReadPipeline retrieves a Buildkite pipeline
func ReadPipeline(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*Client)
	var query struct {
		Node struct {
			Pipeline PipelineNode `graphql:"... on Pipeline"`
		} `graphql:"node(id: $id)"`
	}
	vars := map[string]interface{}{
		"id": graphql.ID(d.Id()),
	}

	err := client.graphql.Query(context.Background(), &query, vars)
	if err != nil {
		return diag.FromErr(err)
	}

	updatePipelineResource(d, &query.Node.Pipeline)

	if slug, pipelineExists := d.GetOk("slug"); pipelineExists {
		pipelineExtraInfo, err := getPipelineExtraInfo(d, m, slug.(string))
		if err != nil {
			return diag.FromErr(err)
		}
		updatePipelineResourceExtraInfo(d, pipelineExtraInfo)
	}

	return nil
}

// UpdatePipeline updates a Buildkite pipeline
func UpdatePipeline(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*Client)
	var err error

	branch := d.Get("branch_configuration").(string)

	input := PipelineUpdateInput{
		AllowRebuilds:                        d.Get("allow_rebuilds").(bool),
		BranchConfiguration:                  &branch,
		CancelIntermediateBuilds:             d.Get("cancel_intermediate_builds").(bool),
		CancelIntermediateBuildsBranchFilter: d.Get("cancel_intermediate_builds_branch_filter").(string),
		DefaultBranch:                        d.Get("default_branch").(string),
		DefaultTimeoutInMinutes:              d.Get("default_timeout_in_minutes").(int),
		MaximumTimeoutInMinutes:              d.Get("maximum_timeout_in_minutes").(int),
		Description:                          d.Get("description").(string),
		Id:                                   d.Id(),
		Name:                                 d.Get("name").(string),
		Repository:                           PipelineRepositoryInput{Url: d.Get("repository").(string)},
		SkipIntermediateBuilds:               d.Get("skip_intermediate_builds").(bool),
		SkipIntermediateBuildsBranchFilter:   d.Get("skip_intermediate_builds_branch_filter").(string),
		Steps:                                PipelineStepsInput{Yaml: d.Get("steps").(string)},
		Tags:                                 getTagsFromSchema(d),
	}

	// If cluster_id exists in the schema it must be a non-empty string
	// Otherwise, if its not present it will be set to nil by default
	if clusterID, clusterIdPresent := d.GetOk("cluster_id"); clusterIdPresent {
		if value, isString := clusterID.(string); isString && value != "" {
			input.ClusterId = &value
		}
	}

	log.Printf("Updating pipeline %s ...", input.Name)

	response, err := updatePipeline(client.genqlient, input)

	if err != nil {
		log.Printf("Unable to update pipeline %s", d.Get("name"))
		return diag.FromErr(err)
	}

	// While transitioning from shurcool to genqlient, we'll map the response here to utilise existing functionality
	pipeline := PipelineNode{
		AllowRebuilds:                        graphql.Boolean(response.PipelineUpdate.Pipeline.AllowRebuilds),
		CancelIntermediateBuilds:             graphql.Boolean(response.PipelineUpdate.Pipeline.CancelIntermediateBuilds),
		CancelIntermediateBuildsBranchFilter: graphql.String(response.PipelineUpdate.Pipeline.CancelIntermediateBuildsBranchFilter),
		BranchConfiguration:                  graphql.String(response.PipelineUpdate.Pipeline.BranchConfiguration),
		Cluster:                              Cluster{ID: graphql.String(response.PipelineUpdate.Pipeline.Cluster.Id)},
		DefaultBranch:                        graphql.String(response.PipelineUpdate.Pipeline.DefaultBranch),
		DefaultTimeoutInMinutes:              graphql.Int(response.PipelineUpdate.Pipeline.DefaultTimeoutInMinutes),
		MaximumTimeoutInMinutes:              graphql.Int(response.PipelineUpdate.Pipeline.MaximumTimeoutInMinutes),
		Description:                          graphql.String(response.PipelineUpdate.Pipeline.Description),
		ID:                                   graphql.String(response.PipelineUpdate.Pipeline.Id),
		Name:                                 graphql.String(response.PipelineUpdate.Pipeline.Name),
		Repository:                           Repository{URL: graphql.String(response.PipelineUpdate.Pipeline.Repository.Url)},
		SkipIntermediateBuilds:               graphql.Boolean(response.PipelineUpdate.Pipeline.SkipIntermediateBuilds),
		SkipIntermediateBuildsBranchFilter:   graphql.String(response.PipelineUpdate.Pipeline.SkipIntermediateBuildsBranchFilter),
		Slug:                                 graphql.String(response.PipelineUpdate.Pipeline.Slug),
		Steps:                                Steps{YAML: graphql.String(response.PipelineUpdate.Pipeline.Steps.Yaml)},
		Tags:                                 mapTagsFromGenqlient(response.PipelineUpdate.Pipeline.Tags),
		Teams:                                mapTeamPipelinesFromGenqlient(response.PipelineUpdate.Pipeline.Teams.Edges),
		WebhookURL:                           graphql.String(response.PipelineUpdate.Pipeline.WebhookURL),
	}

	teamPipelines := getTeamPipelinesFromSchema(d)
	err = reconcileTeamPipelines(teamPipelines, &pipeline, client)
	if err != nil {
		log.Print("Unable to reconcile team pipelines")
		return diag.FromErr(err)
	}

	updatePipelineResource(d, &pipeline)

	pipelineExtraInfo, err := updatePipelineExtraInfo(d, client)
	if err != nil {
		log.Print("Unable to update pipeline attributes via REST API")
		return diag.FromErr(err)
	}
	updatePipelineResourceExtraInfo(d, &pipelineExtraInfo)

	return ReadPipeline(ctx, d, m)
}

// DeletePipeline removes a Buildkite pipeline
func DeletePipeline(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*Client)

	var mutation struct {
		PipelineDelete struct {
			Organization struct {
				ID graphql.ID
			}
		} `graphql:"pipelineDelete(input: {id: $id})"`
	}

	vars := map[string]interface{}{
		"id": graphql.ID(d.Id()),
	}

	if d.Get("deletion_protection") == true {
		return diag.Errorf("Deletion protection is enabled for pipeline: %s", d.Get("name"))
	}

	if d.Get("archive_on_delete") == true {
		fmt.Printf("Pipeline %s set to archive on delete. Archiving...", d.Get("name"))
		_, err := archivePipeline(client.genqlient, d.Id())
		if err != nil {
			return diag.FromErr(err)
		}
		return nil
	}

	log.Printf("Deleting pipeline %s ...", d.Get("name"))
	err := client.graphql.Mutate(context.Background(), &mutation, vars)
	if err != nil {
		log.Printf("Unable to delete pipeline %s", d.Get("name"))
		return diag.FromErr(err)
	}

	return nil
}

// As of May 21, 2021, GraphQL Pipeline is lacking support for the following properties:
// - badge_url
// - provider_settings
// We fallback to REST API

// PipelineExtraInfo is used to manage pipeline attributes that are not exposed via GraphQL API.
type PipelineExtraInfo struct {
	BadgeUrl string `json:"badge_url"`
	Provider struct {
		Settings struct {
			TriggerMode                             string `json:"trigger_mode"`
			BuildPullRequests                       bool   `json:"build_pull_requests"`
			PullRequestBranchFilterEnabled          bool   `json:"pull_request_branch_filter_enabled"`
			PullRequestBranchFilterConfiguration    string `json:"pull_request_branch_filter_configuration"`
			SkipBuildsForExistingCommits            bool   `json:"skip_builds_for_existing_commits"`
			SkipPullRequestBuildsForExistingCommits bool   `json:"skip_pull_request_builds_for_existing_commits"`
			BuildPullRequestReadyForReview          bool   `json:"build_pull_request_ready_for_review"`
			BuildPullRequestLabelsChanged           bool   `json:"build_pull_request_labels_changed"`
			BuildPullRequestForks                   bool   `json:"build_pull_request_forks"`
			PrefixPullRequestForkBranchNames        bool   `json:"prefix_pull_request_fork_branch_names"`
			BuildBranches                           bool   `json:"build_branches"`
			BuildTags                               bool   `json:"build_tags"`
			CancelDeletedBranchBuilds               bool   `json:"cancel_deleted_branch_builds"`
			FilterEnabled                           bool   `json:"filter_enabled"`
			FilterCondition                         string `json:"filter_condition"`
			PublishCommitStatus                     bool   `json:"publish_commit_status"`
			PublishBlockedAsPending                 bool   `json:"publish_blocked_as_pending"`
			PublishCommitStatusPerStep              bool   `json:"publish_commit_status_per_step"`
			SeparatePullRequestStatuses             bool   `json:"separate_pull_request_statuses"`
		} `json:"settings"`
	} `json:"provider"`
}

func getPipelineExtraInfo(d *schema.ResourceData, m interface{}, slug string) (*PipelineExtraInfo, error) {
	client := m.(*Client)
	pipelineExtraInfo := PipelineExtraInfo{}
	err := client.makeRequest("GET", fmt.Sprintf("/v2/organizations/%s/pipelines/%s", client.organization, slug), nil, &pipelineExtraInfo)
	if err != nil {
		return nil, err
	}
	return &pipelineExtraInfo, nil
}

func updatePipelineExtraInfo(d *schema.ResourceData, client *Client) (PipelineExtraInfo, error) {
	payload := map[string]interface{}{}
	if settings := d.Get("provider_settings").([]interface{}); len(settings) > 0 {
		payload["provider_settings"] = settings[0].(map[string]interface{})
	}

	slug := d.Get("slug").(string)
	pipelineExtraInfo := PipelineExtraInfo{}
	err := client.makeRequest("PATCH", fmt.Sprintf("/v2/organizations/%s/pipelines/%s", client.organization, slug), payload, &pipelineExtraInfo)
	if err != nil {
		return pipelineExtraInfo, err
	}
	return pipelineExtraInfo, nil
}

func mapTagsFromGenqlient(tags []updatePipelinePipelineUpdatePipelineUpdatePayloadPipelineTagsPipelineTag) []PipelineTag {
	newTags := make([]PipelineTag, len(tags))

	for i, v := range tags {
		newTags[i] = PipelineTag{Label: graphql.String(v.Label)}
	}
	return newTags
}

func getTagsFromSchema(d *schema.ResourceData) []PipelineTagInput {
	tagSet := d.Get("tags").(*schema.Set)
	tags := make([]PipelineTagInput, tagSet.Len())
	for i, v := range tagSet.List() {
		tags[i] = PipelineTagInput{
			Label: v.(string),
		}
	}
	return tags
}

func mapTeamPipelinesFromGenqlient(tags []updatePipelinePipelineUpdatePipelineUpdatePayloadPipelineTeamsTeamPipelineConnectionEdgesTeamPipelineEdge) struct {
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

func getTeamPipelinesFromSchema(d *schema.ResourceData) []TeamPipelineNode {
	teamsInput := d.Get("team").(*schema.Set).List()
	teamPipelineNodes := make([]TeamPipelineNode, len(teamsInput))
	for i, v := range teamsInput {
		teamInput := v.(map[string]interface{})
		teamPipeline := TeamPipelineNode{
			AccessLevel: PipelineAccessLevels(teamInput["access_level"].(string)),
			ID:          "",
			Team: TeamNode{
				Slug: graphql.String(teamInput["slug"].(string)),
			},
		}
		teamPipelineNodes[i] = teamPipeline
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

// updatePipelineResource updates the terraform resource data for the pipeline resource
func updatePipelineResource(d *schema.ResourceData, pipeline *PipelineNode) {
	d.SetId(string(pipeline.ID))
	d.Set("allow_rebuilds", bool(pipeline.AllowRebuilds))
	d.Set("branch_configuration", string(pipeline.BranchConfiguration))
	d.Set("default_timeout_in_minutes", int(pipeline.DefaultTimeoutInMinutes))
	d.Set("maximum_timeout_in_minutes", int(pipeline.MaximumTimeoutInMinutes))
	d.Set("cancel_intermediate_builds", bool(pipeline.CancelIntermediateBuilds))
	d.Set("cancel_intermediate_builds_branch_filter", string(pipeline.CancelIntermediateBuildsBranchFilter))
	d.Set("cluster_id", string(pipeline.Cluster.ID))
	d.Set("default_branch", string(pipeline.DefaultBranch))
	d.Set("description", string(pipeline.Description))
	d.Set("name", string(pipeline.Name))
	d.Set("repository", string(pipeline.Repository.URL))
	d.Set("skip_intermediate_builds", bool(pipeline.SkipIntermediateBuilds))
	d.Set("skip_intermediate_builds_branch_filter", string(pipeline.SkipIntermediateBuildsBranchFilter))
	d.Set("slug", string(pipeline.Slug))
	d.Set("steps", string(pipeline.Steps.YAML))
	d.Set("webhook_url", string(pipeline.WebhookURL))

	teams := make([]map[string]interface{}, len(pipeline.Teams.Edges))
	for i, id := range pipeline.Teams.Edges {
		team := map[string]interface{}{
			"slug":         string(id.Node.Team.Slug),
			"access_level": string(id.Node.AccessLevel),
		}
		teams[i] = team
	}
	d.Set("team", teams)
}

// updatePipelineResourceExtraInfo updates the terraform resource with data received from Buildkite REST API
func updatePipelineResourceExtraInfo(d *schema.ResourceData, pipeline *PipelineExtraInfo) {
	d.Set("badge_url", pipeline.BadgeUrl)

	s := &pipeline.Provider.Settings
	providerSettings := make([]map[string]interface{}, 1, 1)
	providerSettings[0] = map[string]interface{}{
		"trigger_mode":                                  s.TriggerMode,
		"build_pull_requests":                           s.BuildPullRequests,
		"pull_request_branch_filter_enabled":            s.PullRequestBranchFilterEnabled,
		"pull_request_branch_filter_configuration":      s.PullRequestBranchFilterConfiguration,
		"skip_builds_for_existing_commits":              s.SkipBuildsForExistingCommits,
		"skip_pull_request_builds_for_existing_commits": s.SkipPullRequestBuildsForExistingCommits,
		"build_pull_request_ready_for_review":           s.BuildPullRequestReadyForReview,
		"build_pull_request_labels_changed":             s.BuildPullRequestLabelsChanged,
		"build_pull_request_forks":                      s.BuildPullRequestForks,
		"filter_enabled":                                s.FilterEnabled,
		"filter_condition":                              s.FilterCondition,
		"prefix_pull_request_fork_branch_names":         s.PrefixPullRequestForkBranchNames,
		"build_branches":                                s.BuildBranches,
		"build_tags":                                    s.BuildTags,
		"cancel_deleted_branch_builds":                  s.CancelDeletedBranchBuilds,
		"publish_commit_status":                         s.PublishCommitStatus,
		"publish_blocked_as_pending":                    s.PublishBlockedAsPending,
		"publish_commit_status_per_step":                s.PublishCommitStatusPerStep,
		"separate_pull_request_statuses":                s.SeparatePullRequestStatuses,
	}
	d.Set("provider_settings", providerSettings)
}
