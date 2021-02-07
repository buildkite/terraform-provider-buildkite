package buildkite

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/shurcooL/graphql"
)

// PipelineScheduleNode represents a pipeline schedule as returned from the GraphQL API
type PipelineScheduleNode struct {
	Branch   graphql.String
	Commit   graphql.String
	Cronline graphql.String
	Enabled  graphql.Boolean
	Env      []graphql.String
	ID       graphql.String
	Label    graphql.String
	Message  graphql.String
	Pipeline struct {
		ID graphql.String
	}
}

// resourcePipelineSchedule represents the terraform pipeline_schedule resource schema
func resourcePipelineSchedule() *schema.Resource {
	return &schema.Resource{
		Create: CreatePipelineSchedule,
		Read:   ReadPipelineSchedule,
		Update: UpdatePipelineSchedule,
		Delete: DeletePipelineSchedule,
		Importer: &schema.ResourceImporter{
			State: setPipelineScheduleIDFromSlug,
		},
		Schema: map[string]*schema.Schema{
			"pipeline_id": {
				Required: true,
				Type:     schema.TypeString,
			},
			"label": {
				Required: true,
				Type:     schema.TypeString,
			},
			"cronline": {
				Required: true,
				Type:     schema.TypeString,
			},
			"commit": {
				Optional: true,
				Default:  "HEAD",
				Type:     schema.TypeString,
			},
			"branch": {
				Required: true,
				Type:     schema.TypeString,
			},
			"message": {
				Optional: true,
				Computed: true,
				Type:     schema.TypeString,
			},
			"env": {
				Optional: true,
				Type:     schema.TypeMap,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"enabled": {
				Optional: true,
				Default:  true,
				Type:     schema.TypeBool,
			},
		},
	}
}

func setPipelineScheduleIDFromSlug(d *schema.ResourceData, m interface{}) ([]*schema.ResourceData, error) {
	client := m.(*Client)

	var query struct {
		PipelineSchedule struct {
			ID graphql.String
		} `graphql:"pipelineSchedule(slug: $slug)"`
	}

	// d.Id() here is the last argument passed to the `terraform import buildkite_pipeline_schedule.NAME SCHEDULE_SLUG` command
	vars := map[string]interface{}{
		"slug": graphql.ID(d.Id()),
	}

	err := client.graphql.Query(context.Background(), &query, vars)
	if err != nil {
		return nil, err
	}

	d.SetId(string(query.PipelineSchedule.ID))

	return []*schema.ResourceData{d}, nil
}

// CreatePipelineSchedule creates a Buildkite pipeline schedule
func CreatePipelineSchedule(d *schema.ResourceData, m interface{}) error {
	client := m.(*Client)

	var mutation struct {
		PipelineScheduleCreatePayload struct {
			Pipeline             PipelineNode
			PipelineScheduleEdge struct {
				Node PipelineScheduleNode
			}
		} `graphql:"pipelineScheduleCreate(input: {pipelineID: $pipeline_id, branch: $branch, commit: $commit, cronline: $cronline, enabled: $enabled, env: $env, label: $label, message: $message})"`
	}
	vars := map[string]interface{}{
		"pipeline_id": graphql.ID(d.Get("pipeline_id").(string)),
		"label":       graphql.String(d.Get("label").(string)),
		"cronline":    graphql.String(d.Get("cronline").(string)),
		"commit":      graphql.String(d.Get("commit").(string)),
		"branch":      graphql.String(d.Get("branch").(string)),
		"message":     graphql.String(d.Get("message").(string)),
		"env":         graphql.String(envVarsToString(d.Get("env").(map[string]interface{}))),
		"enabled":     graphql.Boolean(d.Get("enabled").(bool)),
	}

	log.Printf("Creating pipeline %s ...", vars["label"])
	var err = client.graphql.Mutate(context.Background(), &mutation, vars)
	if err != nil {
		log.Printf("Unable to create pipeline schedule %s", d.Get("label"))
		return err
	}
	log.Printf("Successfully created pipeline schedule with id '%s'.", mutation.PipelineScheduleCreatePayload.PipelineScheduleEdge.Node.ID)

	updatePipelineScheduleResource(d, &mutation.PipelineScheduleCreatePayload.PipelineScheduleEdge.Node)
	return ReadPipelineSchedule(d, m)
}

// ReadPipelineSchedule retrieves a Buildkite pipeline schedule
func ReadPipelineSchedule(d *schema.ResourceData, m interface{}) error {
	client := m.(*Client)
	var query struct {
		Node struct {
			PipelineSchedule PipelineScheduleNode `graphql:"... on PipelineSchedule"`
		} `graphql:"node(id: $id)"`
	}
	vars := map[string]interface{}{
		"id": graphql.ID(d.Id()),
	}

	err := client.graphql.Query(context.Background(), &query, vars)
	if err != nil {
		return err
	}

	updatePipelineScheduleResource(d, &query.Node.PipelineSchedule)
	return nil
}

// UpdatePipelineSchedule updates a Buildkite pipeline schedule
func UpdatePipelineSchedule(d *schema.ResourceData, m interface{}) error {
	client := m.(*Client)
	var mutation struct {
		PipelineScheduleUpdate struct {
			PipelineSchedule PipelineScheduleNode
		} `graphql:"pipelineScheduleUpdate(input: {id: $id, branch: $branch, commit: $commit, cronline: $cronline, enabled: $enabled, env: $env, label: $label, message: $message})"`
	}
	vars := map[string]interface{}{
		"id":       graphql.ID(d.Id()),
		"label":    graphql.String(d.Get("label").(string)),
		"cronline": graphql.String(d.Get("cronline").(string)),
		"commit":   graphql.String(d.Get("commit").(string)),
		"branch":   graphql.String(d.Get("branch").(string)),
		"message":  graphql.String(d.Get("message").(string)),
		"env":      graphql.String(envVarsToString(d.Get("env").(map[string]interface{}))),
		"enabled":  graphql.Boolean(d.Get("enabled").(bool)),
	}

	log.Printf("Updating pipeline schedule %s ...", vars["label"])
	err := client.graphql.Mutate(context.Background(), &mutation, vars)
	if err != nil {
		log.Printf("Unable to update pipeline schedule %s", d.Get("label"))
		return err
	}

	updatePipelineScheduleResource(d, &mutation.PipelineScheduleUpdate.PipelineSchedule)
	return ReadPipelineSchedule(d, m)
}

// DeletePipelineSchedule removes a Buildkite pipeline schedule
func DeletePipelineSchedule(d *schema.ResourceData, m interface{}) error {
	client := m.(*Client)

	var mutation struct {
		PipelineScheduleDelete struct {
			Pipeline PipelineNode
		} `graphql:"pipelineScheduleDelete(input: {id: $id})"`
	}
	vars := map[string]interface{}{
		"id": graphql.ID(d.Id()),
	}

	log.Printf("Deleting pipeline schedule %s ...", d.Get("label"))
	err := client.graphql.Mutate(context.Background(), &mutation, vars)
	if err != nil {
		log.Printf("Unable to delete pipeline %s", d.Get("label"))
		return err
	}

	return nil
}

// updatePipelineScheduleResource updates the terraform resource data for the pipeline_schedule resource
func updatePipelineScheduleResource(d *schema.ResourceData, pipelineSchedule *PipelineScheduleNode) {
	d.SetId(string(pipelineSchedule.ID))
	d.Set("pipeline_id", string(pipelineSchedule.Pipeline.ID))
	d.Set("label", string(pipelineSchedule.Label))
	d.Set("cronline", string(pipelineSchedule.Cronline))
	d.Set("message", string(pipelineSchedule.Message))
	d.Set("commit", string(pipelineSchedule.Commit))
	d.Set("branch", string(pipelineSchedule.Branch))
	d.Set("env", envVarsArrayToMap(pipelineSchedule.Env))
	d.Set("enabled", bool(pipelineSchedule.Enabled))
}

// converts env vars map to a newline-separated string
func envVarsToString(m map[string]interface{}) string {
	b := new(bytes.Buffer)
	for key, value := range m {
		fmt.Fprintf(b, "%s=%s\n", key, value.(string))
	}
	return b.String()
}

// converts env vars array of Strings to a map
func envVarsArrayToMap(envVarsArray []graphql.String) map[string]string {
	result := make(map[string]string)
	for _, envVar := range envVarsArray {
		tuple := strings.Split(string(envVar), "=")
		result[tuple[0]] = tuple[1]
	}
	return result
}
