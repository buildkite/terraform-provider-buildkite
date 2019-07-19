package buildkite

import (
	"context"

	"github.com/hashicorp/terraform/helper/schema"
	"github.com/shurcooL/graphql"
)

type PipelineNode struct {
	DefaultBranch graphql.String
	Description   graphql.String
	Id            graphql.String
	Name          graphql.String
	Uuid          graphql.String
	WebhookURL    graphql.String `graphql:"webhookURL"`
}

func resourcePipeline() *schema.Resource {
	return &schema.Resource{
		Create: CreatePipeline,
		Read:   ReadPipeline,
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
			// "steps": &schema.Schema{
			//     Computed: true,
			//     Type:     schema.TypeString,
			// },
		},
	}
}

func CreatePipeline(d *schema.ResourceData, m interface{}) error {
	client := m.(*Client)
	id, err := GetOrganizationID(client.organization, client.graphql)
	if err != nil {
		return err
	}

	var mutation struct {
		PipelineCreate struct {
			Pipeline PipelineNode
		} `graphql:"pipelineCreate(input: {organizationID: $org, name: $name, description: $desc, repository: {url: $repository_url}, steps: {yaml: $steps}})"`
	}

	vars := map[string]interface{}{
		"desc":           graphql.String(d.Get("description").(string)),
		"name":           d.Get("name").(string),
		"org":            id,
		"repository_url": d.Get("repository").(string),
		"steps":          "steps:\n  - command: \"buildkite-agent pipeline upload\"\n    label: \":pipeline:\"",
	}

	err = client.graphql.Mutate(context.Background(), &mutation, vars)
	if err != nil {
		return err
	}

	updatePipeline(d, &mutation.PipelineCreate.Pipeline)

	return nil
}

func ReadPipeline(d *schema.ResourceData, m interface{}) error {
	client := m.(*Client)
	var query struct {
		Node struct {
			Pipeline PipelineNode `graphql:"... on Pipeline"`
		} `graphql:"node(id: $id)"`
	}

	vars := map[string]interface{}{
		"id": d.Id(),
	}

	err := client.graphql.Query(context.Background(), &query, vars)
	if err != nil {
		return err
	}

	updatePipeline(d, &query.Node.Pipeline)

	return nil
}

func DeletePipeline(d *schema.ResourceData, m interface{}) error {
	client := m.(*Client)
	var mutation struct {
		AgentTokenRevoke struct {
			AgentToken AgentTokenNode
		} `graphql:"agentTokenRevoke(input: {id: $id, reason: $reason})"`
	}

	vars := map[string]interface{}{
		"id":     graphql.ID(d.Id()),
		"reason": graphql.String("Revoked by Terraform"),
	}

	err := client.graphql.Mutate(context.Background(), &mutation, vars)
	if err != nil {
		return err
	}

	return nil
}

func updatePipeline(d *schema.ResourceData, t *PipelineNode) {
	d.SetId(string(t.Id))
	d.Set("description", string(t.Description))
	d.Set("name", string(t.Name))
	d.Set("uuid", string(t.Uuid))
	d.Set("webhook_url", string(t.WebhookURL))
}
