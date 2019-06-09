package buildkite

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform/helper/schema"
	"github.com/shurcooL/graphql"
)

const idSeparator = "|"

type AgentTokenNode struct {
	Description graphql.String
	Id          graphql.String
	Token       graphql.String
	Uuid        graphql.String
	RevokedAt   graphql.String
}

func resourceAgentToken() *schema.Resource {
	return &schema.Resource{
		Create: CreateToken,
		Read:   ReadToken,
		Delete: DeleteToken,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"description": &schema.Schema{
				ForceNew: true,
				Optional: true,
				Type:     schema.TypeString,
			},
			"token": &schema.Schema{
				Computed: true,
				Type:     schema.TypeString,
			},
			"uuid": &schema.Schema{
				Computed: true,
				Type:     schema.TypeString,
			},
		},
	}
}

func CreateToken(d *schema.ResourceData, m interface{}) error {
	client := m.(*Client)
	id, err := GetOrganizationID(client.organization, client.graphql)
	if err != nil {
		return err
	}

	var mutation struct {
		AgentTokenCreate struct {
			AgentTokenEdge struct {
				Node AgentTokenNode
			}
		} `graphql:"agentTokenCreate(input: {organizationID: $org, description: $desc})"`
	}

	vars := map[string]interface{}{
		"org":  id,
		"desc": graphql.String(d.Get("description").(string)),
	}

	err = client.graphql.Mutate(context.Background(), &mutation, vars)
	if err != nil {
		return err
	}

	updateAgentToken(d, &mutation.AgentTokenCreate.AgentTokenEdge.Node)

	return nil
}

func ReadToken(d *schema.ResourceData, m interface{}) error {
	client := m.(*Client)
	var query struct {
		Node struct {
			AgentToken AgentTokenNode `graphql:"... on AgentToken"`
		} `graphql:"node(id: $id)"`
	}

	vars := map[string]interface{}{
		"id": d.Id(),
	}

	err := client.graphql.Query(context.Background(), &query, vars)
	if err != nil {
		return err
	}
	if query.Node.AgentToken.RevokedAt != "" {
		return errors.New("Cannot import revoked token")
	}

	updateAgentToken(d, &query.Node.AgentToken)

	return nil
}

func DeleteToken(d *schema.ResourceData, m interface{}) error {
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

func updateAgentToken(d *schema.ResourceData, t *AgentTokenNode) {
	d.SetId(string(t.Id))
	d.Set("uuid", string(t.Uuid))
	d.Set("description", string(t.Description))
	d.Set("token", string(t.Token))
}
