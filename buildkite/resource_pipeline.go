package buildkite

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/shurcooL/graphql"
	"log"
	"net/http"
)

// PipelineNode represents a pipeline as returned from the GraphQL API
type PipelineNode struct {
	CancelIntermediateBuilds             graphql.Boolean
	CancelIntermediateBuildsBranchFilter graphql.String
	DefaultBranch                        graphql.String
	Description                          graphql.String
	Id                                   graphql.String
	Name                                 graphql.String
	Repository                           struct {
		Url graphql.String
	}
	SkipIntermediateBuilds             graphql.Boolean
	SkipIntermediateBuildsBranchFilter graphql.String
	Slug                               graphql.String
	Steps                              struct {
		Yaml graphql.String
	}
	Teams struct {
		Edges []struct {
			Node TeamPipelineNode
		}
	} `graphql:"teams(first: 50)"`
	Uuid       graphql.String
	WebhookURL graphql.String `graphql:"webhookURL"`
}
type PipelineAccessLevels graphql.String
type TeamPipelineNode struct {
	AccessLevel PipelineAccessLevels
	Id          graphql.String
	Team        TeamNode
}

// resourcePipeline represents the terraform pipeline resource schema
func resourcePipeline() *schema.Resource {
	return &schema.Resource{
		Create: CreatePipeline,
		Read:   ReadPipeline,
		Update: UpdatePipeline,
		Delete: DeletePipeline,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"cancel_intermediate_builds": {
				Optional: true,
				Default:  false,
				Type:     schema.TypeBool,
			},
			"cancel_intermediate_builds_branch_filter": {
				Optional: true,
				Default:  "",
				Type:     schema.TypeString,
			},
			"default_branch": {
				Optional: true,
				Type:     schema.TypeString,
			},
			"description": {
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
				Optional: true,
				Default:  false,
				Type:     schema.TypeBool,
			},
			"skip_intermediate_builds_branch_filter": {
				Optional: true,
				Default:  "",
				Type:     schema.TypeString,
			},
			"slug": {
				Computed: true,
				Type:     schema.TypeString,
			},
			"steps": {
				Required: true,
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
			"webhook_url": {
				Computed: true,
				Type:     schema.TypeString,
			},
		},
	}
}

// CreatePipeline creates a Buildkite pipeline
func CreatePipeline(d *schema.ResourceData, m interface{}) error {
	client := m.(*Client)
	orgId, err := GetOrganizationID(client.organization, client.graphql)
	if err != nil {
		return err
	}

	var mutation struct {
		PipelineCreate struct {
			Pipeline PipelineNode
		} `graphql:"pipelineCreate(input: {cancelIntermediateBuilds: $cancel_intermediate_builds, cancelIntermediateBuildsBranchFilter: $cancel_intermediate_builds_branch_filter, defaultBranch: $default_branch, description: $desc, name: $name, organizationId: $org, repository: {url: $repository_url}, skipIntermediateBuilds: $skip_intermediate_builds, skipIntermediateBuildsBranchFilter: $skip_intermediate_builds_branch_filter, steps: {yaml: $steps}})"`
	}
	vars := map[string]interface{}{
		"cancel_intermediate_builds":               graphql.Boolean(d.Get("cancel_intermediate_builds").(bool)),
		"cancel_intermediate_builds_branch_filter": graphql.String(d.Get("cancel_intermediate_builds_branch_filter").(string)),
		"default_branch":                           graphql.String(d.Get("default_branch").(string)),
		"desc":                                     graphql.String(d.Get("description").(string)),
		"name":                                     graphql.String(d.Get("name").(string)),
		"org":                                      orgId,
		"repository_url":                           graphql.String(d.Get("repository").(string)),
		"skip_intermediate_builds":                 graphql.Boolean(d.Get("skip_intermediate_builds").(bool)),
		"skip_intermediate_builds_branch_filter":   graphql.String(d.Get("skip_intermediate_builds_branch_filter").(string)),
		"steps":                                    graphql.String(d.Get("steps").(string)),
	}

	log.Printf("Creating pipeline %s ...", vars["name"])
	err = client.graphql.Mutate(context.Background(), &mutation, vars)
	if err != nil {
		log.Printf("Unable to create pipeline %s", d.Get("name"))
		return err
	}
	log.Printf("Successfully created pipeline with id '%s'.", mutation.PipelineCreate.Pipeline.Id)

	teamPipelines := getTeamPipelinesFromSchema(d)
	err = reconcileTeamPipelines(teamPipelines, &mutation.PipelineCreate.Pipeline, client)
	if err != nil {
		log.Print("Unable to create team pipelines")
		return err
	}

	updatePipelineResource(d, &mutation.PipelineCreate.Pipeline)

	return ReadPipeline(d, m)
}

// ReadPipeline retrieves a Buildkite pipeline
func ReadPipeline(d *schema.ResourceData, m interface{}) error {
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
		return err
	}

	updatePipelineResource(d, &query.Node.Pipeline)

	return nil
}

// UpdatePipeline updates a Buildkite pipeline
func UpdatePipeline(d *schema.ResourceData, m interface{}) error {
	client := m.(*Client)
	var mutation struct {
		PipelineUpdate struct {
			Pipeline PipelineNode
		} `graphql:"pipelineUpdate(input: {cancelIntermediateBuilds: $cancel_intermediate_builds, cancelIntermediateBuildsBranchFilter: $cancel_intermediate_builds_branch_filter, defaultBranch: $default_branch, description: $desc, id: $id, name: $name, repository: {url: $repository_url}, skipIntermediateBuilds: $skip_intermediate_builds, skipIntermediateBuildsBranchFilter: $skip_intermediate_builds_branch_filter, steps: {yaml: $steps}})"`
	}
	vars := map[string]interface{}{
		"cancel_intermediate_builds":               graphql.Boolean(d.Get("cancel_intermediate_builds").(bool)),
		"cancel_intermediate_builds_branch_filter": graphql.String(d.Get("cancel_intermediate_builds_branch_filter").(string)),
		"default_branch":                           graphql.String(d.Get("default_branch").(string)),
		"desc":                                     graphql.String(d.Get("description").(string)),
		"id":                                       graphql.ID(d.Id()),
		"name":                                     graphql.String(d.Get("name").(string)),
		"repository_url":                           graphql.String(d.Get("repository").(string)),
		"skip_intermediate_builds":                 graphql.Boolean(d.Get("skip_intermediate_builds").(bool)),
		"skip_intermediate_builds_branch_filter":   graphql.String(d.Get("skip_intermediate_builds_branch_filter").(string)),
		"steps":                                    graphql.String(d.Get("steps").(string)),
	}

	log.Printf("Updating pipeline %s ...", vars["name"])
	err := client.graphql.Mutate(context.Background(), &mutation, vars)
	if err != nil {
		log.Printf("Unable to update pipeline %s", d.Get("name"))
		return err
	}

	teamPipelines := getTeamPipelinesFromSchema(d)
	err = reconcileTeamPipelines(teamPipelines, &mutation.PipelineUpdate.Pipeline, client)
	if err != nil {
		log.Print("Unable to reconcile team pipelines")
		return err
	}

	updatePipelineResource(d, &mutation.PipelineUpdate.Pipeline)

	return ReadPipeline(d, m)
}

// DeletePipeline removes a Buildkite pipeline
func DeletePipeline(d *schema.ResourceData, m interface{}) error {
	client := m.(*Client)

	slug := d.Get("slug").(string)
	log.Printf("Deleting pipeline %s ...", slug)
	// there is no delete mutation in graphql yet so we must use rest api
	req, err := http.NewRequest("DELETE", fmt.Sprintf("https://api.buildkite.com/v2/organizations/%s/pipelines/%s",
		client.organization, slug), nil)
	if err != nil {
		return err
	}

	// a successful response returns 204
	resp, err := client.http.Do(req)
	if err != nil && resp.StatusCode != 204 {
		log.Printf("Unable to delete pipeline %s", slug)
		return err
	}

	return nil
}

func getTeamPipelinesFromSchema(d *schema.ResourceData) []TeamPipelineNode {
	teamsInput := d.Get("team").(*schema.Set).List()
	teamPipelineNodes := make([]TeamPipelineNode, len(teamsInput))
	for i, v := range teamsInput {
		teamInput := v.(map[string]interface{})
		teamPipeline := TeamPipelineNode{
			AccessLevel: PipelineAccessLevels(teamInput["access_level"].(string)),
			Id:          "",
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
		id := teamPipeline.Node.Id

		teamPipelineIds[string(teamSlugBk)] = graphql.ID(id)

		found := false
		for _, teamPipeline := range teamPipelines {
			if teamPipeline.Team.Slug == teamSlugBk {
				found = true
				if teamPipeline.AccessLevel != accessLevelBk {
					toUpdate = append(toUpdate, TeamPipelineNode{
						AccessLevel: teamPipeline.AccessLevel,
						Id:          id,
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
				Id:          id,
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
	err := createTeamPipelines(toAdd, string(pipeline.Id), client)
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
func createTeamPipelines(teamPipelines []TeamPipelineNode, pipelineId string, client *Client) error {
	var mutation struct {
		TeamPipelineCreate struct {
			TeamPipeline struct {
				Id graphql.ID
			}
		} `graphql:"teamPipelineCreate(input: {teamID: $team, pipelineID: $pipeline, accessLevel: $accessLevel})"`
	}
	for _, teamPipeline := range teamPipelines {
		log.Printf("Granting teamPipeline %s %s access to pipeline id '%s'...", teamPipeline.Team.Slug, teamPipeline.AccessLevel, pipelineId)
		teamId, err := GetTeamID(string(teamPipeline.Team.Slug), client)
		if err != nil {
			log.Printf("Unable to get ID for team slug %s", teamPipeline.Team.Slug)
			return err
		}
		params := map[string]interface{}{
			"team":        graphql.ID(teamId),
			"pipeline":    graphql.ID(pipelineId),
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
				Id graphql.ID
			}
		} `graphql:"teamPipelineUpdate(input: {id: $id, accessLevel: $accessLevel})"`
	}
	for _, teamPipeline := range teamPipelines {
		log.Printf("Updating access to %s for teamPipeline id '%s'...", teamPipeline.AccessLevel, teamPipeline.Id)
		params := map[string]interface{}{
			"id":          graphql.ID(string(teamPipeline.Id)),
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
				Id graphql.ID
			}
		} `graphql:"teamPipelineDelete(input: {id: $id})"`
	}
	for _, teamPipeline := range teamPipelines {
		log.Printf("Removing access for teamPipeline %s (id=%s)...", teamPipeline.Team.Slug, teamPipeline.Id)
		params := map[string]interface{}{
			"id": graphql.ID(string(teamPipeline.Id)),
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
	d.SetId(string(pipeline.Id))
	d.Set("cancel_intermediate_builds", bool(pipeline.CancelIntermediateBuilds))
	d.Set("cancel_intermediate_builds_branch_filter", string(pipeline.CancelIntermediateBuildsBranchFilter))
	d.Set("default_branch", string(pipeline.DefaultBranch))
	d.Set("description", string(pipeline.Description))
	d.Set("name", string(pipeline.Name))
	d.Set("repository", string(pipeline.Repository.Url))
	d.Set("skip_intermediate_builds", bool(pipeline.SkipIntermediateBuilds))
	d.Set("skip_intermediate_builds_branch_filter", string(pipeline.SkipIntermediateBuildsBranchFilter))
	d.Set("slug", string(pipeline.Slug))
	d.Set("steps", string(pipeline.Steps.Yaml))
	d.Set("uuid", string(pipeline.Uuid))
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
