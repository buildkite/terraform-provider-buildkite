package buildkite

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/shurcooL/graphql"
)

// AgentTokenNode represents a pipeline as returned from the GraphQL API
type AgentTokenNode struct {
	Description graphql.String
	ID          graphql.String
	Token       graphql.String
	UUID        graphql.String
	RevokedAt   graphql.String
}

func resourceAgentToken() *schema.Resource {
	return &schema.Resource{
		CreateContext: CreateToken,
		ReadContext:   ReadToken,
		// NB: there is no updating a token, changes force a new one to be creaated
		DeleteContext: DeleteToken,
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

// CreateToken creates a Buildkite agent token
func CreateToken(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*Client)
	id, err := GetOrganizationID(client.organization, client.graphql)
	if err != nil {
		return diag.FromErr(err)
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
		return diag.FromErr(err)
	}

	updateAgentToken(d, &mutation.AgentTokenCreate.AgentTokenEdge.Node)

	return nil
}

// ReadToken retrieves a Buildkite agent token
func ReadToken(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
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
		return diag.FromErr(err)
	}
	if query.Node.AgentToken.RevokedAt != "" {
		return diag.FromErr(errors.New("Cannot import revoked token"))
	}

	updateAgentToken(d, &query.Node.AgentToken)

	return nil
}

// DeleteToken revokes a Buildkite agent token - they cannot be completely deleted
func DeleteToken(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
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
		return diag.FromErr(err)
	}

	return nil
}

func updateAgentToken(d *schema.ResourceData, t *AgentTokenNode) {
	d.SetId(string(t.ID))
	d.Set("uuid", string(t.UUID))
	d.Set("description", string(t.Description))
	d.Set("token", string(t.Token))
}
