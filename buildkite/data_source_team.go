package buildkite

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/shurcooL/graphql"
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
	var query struct {
		Team TeamNode `graphql:"team(slug: $slug)"`
	}
	orgTeamSlug := fmt.Sprintf("%s/%s", client.organization, d.Get("slug").(string))
	vars := map[string]interface{}{
		"slug": graphql.ID(orgTeamSlug),
	}

	err := client.graphql.Query(context.Background(), &query, vars)
	if err != nil {
		return diag.FromErr(err)
	}

	if query.Team.ID == "" {
		return diag.FromErr(errors.New("Team not found"))
	}

	d.SetId(string(query.Team.ID))
	d.Set("slug", string(query.Team.Slug))
	d.Set("uuid", string(query.Team.UUID))
	d.Set("name", string(query.Team.Name))
	d.Set("privacy", string(query.Team.Privacy))
	d.Set("default_team", bool(query.Team.IsDefaultTeam))
	d.Set("description", string(query.Team.Description))
	d.Set("default_member_role", string(query.Team.DefaultMemberRole))
	d.Set("members_can_create_pipelines", bool(query.Team.MembersCanCreatePipelines))

	return diags
}
