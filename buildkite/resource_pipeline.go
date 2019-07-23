package buildkite

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform/helper/schema"
	"github.com/shurcooL/graphql"
)

// PipelineNode represents a pipeline as returned from the GraphQL API
type PipelineNode struct {
	DefaultBranch graphql.String
	Description   graphql.String
	Id            graphql.String
	Name          graphql.String
	Repository    struct {
		Url graphql.String
	}
	Slug  graphql.String
	Steps struct {
		Yaml graphql.String
	}
	Uuid       graphql.String
	WebhookURL graphql.String `graphql:"webhookURL"`
}

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
			"name": &schema.Schema{
				Required: true,
				Type:     schema.TypeString,
			},
			"description": &schema.Schema{
				Optional: true,
				Type:     schema.TypeString,
			},
			"repository": &schema.Schema{
				Required: true,
				Type:     schema.TypeString,
			},
			"webhook_url": &schema.Schema{
				Computed: true,
				Type:     schema.TypeString,
			},
			"slug": &schema.Schema{
				Computed: true,
				Type:     schema.TypeString,
			},
			"steps": &schema.Schema{
				Required: true,
				Type:     schema.TypeString,
			},
		},
	}
}

// CreatePipeline creates a Buildkite pipeline
func CreatePipeline(d *schema.ResourceData, m interface{}) error {
	client := m.(*Client)
	id, err := GetOrganizationID(client.organization, client.graphql)
	if err != nil {
		return err
	}

	var mutation struct {
		PipelineCreate struct {
			Pipeline PipelineNode
		} `graphql:"pipelineCreate(input: {organizationId: $org, name: $name, description: $desc, repository: {url: $repository_url}, steps: {yaml: $steps}})"`
	}

	vars := map[string]interface{}{
		"desc":           graphql.String(d.Get("description").(string)),
		"name":           graphql.String(d.Get("name").(string)),
		"org":            id,
		"repository_url": graphql.String(d.Get("repository").(string)),
		"steps":          graphql.String(d.Get("steps").(string)),
	}

	err = client.graphql.Mutate(context.Background(), &mutation, vars)
	if err != nil {
		return err
	}

	updatePipeline(d, &mutation.PipelineCreate.Pipeline)

	return nil
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

	updatePipeline(d, &query.Node.Pipeline)

	return nil
}

// UpdatePipeline updates a Buildkite pipeline
func UpdatePipeline(d *schema.ResourceData, m interface{}) error {
	client := m.(*Client)

	var mutation struct {
		PipelineUpdate struct {
			Pipeline PipelineNode
		} `graphql:"pipelineUpdate(input: {id: $id, name: $name, description: $desc, repository: {url: $repository_url}, steps: {yaml: $steps}})"`
	}

	vars := map[string]interface{}{
		"desc":           graphql.String(d.Get("description").(string)),
		"id":             graphql.ID(d.Id()),
		"name":           graphql.String(d.Get("name").(string)),
		"repository_url": graphql.String(d.Get("repository").(string)),
		"steps":          graphql.String("steps:\n  - command: \"buildkite-agent pipeline upload\"\n    label: \":pipeline:\""),
	}

	err := client.graphql.Mutate(context.Background(), &mutation, vars)
	if err != nil {
		return err
	}

	updatePipeline(d, &mutation.PipelineUpdate.Pipeline)

	return nil
}

// DeletePipeline removes a Buildkite pipeline
func DeletePipeline(d *schema.ResourceData, m interface{}) error {
	client := m.(*Client)

	// there is no delete mutation in graphql yet so we must use rest api
	req, err := http.NewRequest("DELETE", fmt.Sprintf("https://api.buildkite.com/v2/organizations/%s/pipelines/%s",
		client.organization, d.Get("slug").(string)), strings.NewReader(""))
	if err != nil {
		return err
	}

	// a successful response returns 204
	resp, err := client.http.Do(req)
	if err != nil && resp.StatusCode != 204 {
		return err
	}

	return nil
}

func updatePipeline(d *schema.ResourceData, t *PipelineNode) {
	d.SetId(string(t.Id))
	d.Set("description", string(t.Description))
	d.Set("name", string(t.Name))
	d.Set("repository", string(t.Repository.Url))
	d.Set("slug", string(t.Slug))
	d.Set("steps", string(t.Steps.Yaml))
	d.Set("uuid", string(t.Uuid))
	d.Set("webhook_url", string(t.WebhookURL))
}
