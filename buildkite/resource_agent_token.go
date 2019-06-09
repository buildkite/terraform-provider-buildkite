package buildkite

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	// this needs the org id (not slug) to create a token so we need to get that first
	var query struct {
		Organization struct {
			ID graphql.ID
		} `graphql:"organization(slug: $slug)"`
	}
	queryVars := map[string]interface{}{
		"slug": client.organization,
	}
	err := client.graphql.Query(context.Background(), &query, queryVars)
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
		"org":  query.Organization.ID,
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
		AgentToken AgentTokenNode `graphql:"agentToken(slug: $slug)"`
	}

	_, uuid := splitTerraformID(d.Id())

	vars := map[string]interface{}{
		"slug": fmt.Sprintf("%s/%s", client.organization, uuid),
	}

	err := client.graphql.Query(context.Background(), &query, vars)
	if err != nil {
		return err
	}
	if query.AgentToken.RevokedAt != "" {
		return errors.New("Cannot import revoked token")
	}

	updateAgentToken(d, &query.AgentToken)

	return nil
}

func DeleteToken(d *schema.ResourceData, m interface{}) error {
	client := m.(*Client)
	var mutation struct {
		AgentTokenRevoke struct {
			AgentToken AgentTokenNode
		} `graphql:"agentTokenRevoke(input: {id: $id, reason: $reason})"`
	}

	id, _ := splitTerraformID(d.Id())

	vars := map[string]interface{}{
		"id":     graphql.ID(id),
		"reason": graphql.String("Revoked by Terraform"),
	}

	err := client.graphql.Mutate(context.Background(), &mutation, vars)
	if err != nil {
		return err
	}

	return nil
}

func getTerraformID(t *AgentTokenNode) string {
	return fmt.Sprintf("%s%s%s", t.Id, idSeparator, t.Uuid)
}

func splitTerraformID(id string) (string, string) {
	split := strings.Split(id, idSeparator)
	return split[0], split[1]
}

func updateAgentToken(d *schema.ResourceData, t *AgentTokenNode) {
	d.SetId(getTerraformID(t))
	d.Set("uuid", string(t.Uuid))
	d.Set("description", string(t.Description))
	d.Set("token", string(t.Token))
}
