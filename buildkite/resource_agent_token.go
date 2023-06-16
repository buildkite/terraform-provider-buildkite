package buildkite

import (
	"context"
	"errors"
	"fmt"

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
		Importer:      nil,
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
	var diags diag.Diagnostics
	client := m.(*Client)

	apiResponse, err := createAgentToken(
		client.genqlient,
		client.organizationId,
		d.Get("description").(string),
	)

	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(apiResponse.AgentTokenCreate.AgentTokenEdge.Node.Id)
	d.Set("uuid", apiResponse.AgentTokenCreate.AgentTokenEdge.Node.Uuid)
	d.Set("description", apiResponse.AgentTokenCreate.AgentTokenEdge.Node.Description)
	d.Set("token", apiResponse.AgentTokenCreate.TokenValue)

	return diags
}

// ReadToken retrieves a Buildkite agent token
func ReadToken(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	client := m.(*Client)

	agentToken, err := getAgentToken(client.genqlient, fmt.Sprintf("%s/%s", client.organization, d.Get("uuid").(string)))

	if err != nil {
		return diag.FromErr(err)
	}

	if agentToken.AgentToken.Id == "" {
		return diag.FromErr(errors.New("Agent Token not found"))
	}

	d.SetId(agentToken.AgentToken.Id)
	d.Set("uuid", agentToken.AgentToken.Uuid)
	d.Set("description", agentToken.AgentToken.Description)
	// NB: we never set the token in read context because its not available in the API after creation

	return diags
}

// DeleteToken revokes a Buildkite agent token - they cannot be completely deleted (will have a revoke)
func DeleteToken(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	client := m.(*Client)
	var err error
	
	_, err = revokeAgentToken(client.genqlient, d.Id(),"Revoked by Terraform")

	if err != nil {
		return diag.FromErr(err)
	}

	return diags
}
