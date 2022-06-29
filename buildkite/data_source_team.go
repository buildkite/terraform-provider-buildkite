package buildkite

import (
	"context"
	"fmt"

	"github.com/buildkite/terraform-provider-buildkite/buildkite/graphql"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceTeam() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceTeamRead,
		Schema: map[string]*schema.Schema{
			"id": {
				Computed: true,
				Type:     schema.TypeString,
			},
			"slug": {
				Required: true,
				Type:     schema.TypeString,
			},
			"uuid": {
				Computed: true,
				Type:     schema.TypeString,
			},
			"name": {
				Computed: true,
				Type:     schema.TypeString,
			},
			"privacy": {
				Computed: true,
				Type:     schema.TypeString,
			},
			"description": {
				Computed: true,
				Type:     schema.TypeString,
			},
			"default_team": {
				Computed: true,
				Type:     schema.TypeBool,
			},
			"default_member_role": {
				Computed: true,
				Type:     schema.TypeString,
			},
			"members_can_create_pipelines": {
				Computed: true,
				Type:     schema.TypeBool,
			},
		},
	}
}

// ReadTeam retrieves a Buildkite team
func dataSourceTeamRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	// Warning or errors can be collected in a slice type
	var diags diag.Diagnostics

	client := m.(*Client)
	orgTeamSlug := fmt.Sprintf("%s/%s", client.organization, d.Get("slug").(string))

	response, err := graphql.GetTeam(client.genqlient, orgTeamSlug)

	if err != nil {
		return diag.FromErr(err)
	}

	if response.Team.Id == "" {
		d.SetId("")
		return nil
	}

	setTeamSchema(d, &response.Team.Team)

	return diags
}
