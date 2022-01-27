package buildkite

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/shurcooL/graphql"
)

type TeamMemberNode struct {
	ID   graphql.String
	Role TeamMemberRole
	UUID graphql.String
	Team TeamNode
	User struct {
		ID graphql.ID
	}
}

func resourceTeamMember() *schema.Resource {
	return &schema.Resource{
		CreateContext: CreateTeamMember,
		ReadContext:   ReadTeamMember,
		UpdateContext: UpdateTeamMember,
		DeleteContext: DeleteTeamMember,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"role": &schema.Schema{
				Required: true,
				Type:     schema.TypeString,
				ValidateFunc: func(val interface{}, key string) (warns []string, errs []error) {
					v := val.(string)
					switch v {
					case "MEMBER":
					case "MAINTAINER":
						return
					default:
						errs = append(errs, fmt.Errorf("%q must be either MEMBER or MAINTAINER, got: %s", key, v))
						return
					}
					return
				},
			},
			"team_id": &schema.Schema{
				Required: true,
				Type:     schema.TypeString,
			},
			"user_id": &schema.Schema{
				Required: true,
				Type:     schema.TypeString,
			},
			"uuid": &schema.Schema{
				Computed: true,
				Type:     schema.TypeString,
			},
		},
	}
}

// CreateTeamMember adds a user to a Buildkite team
func CreateTeamMember(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	originalRole := d.Get("role").(string)
	client := m.(*Client)

	var mutation struct {
		TeamMemberCreate struct {
			TeamMemberEdge struct {
				Node TeamMemberNode
			}
		} `graphql:"teamMemberCreate(input: {teamID: $teamId, userID: $userId})"`
	}

	vars := map[string]interface{}{
		"teamId": graphql.ID(d.Get("team_id").(string)),
		"userId": graphql.ID(d.Get("user_id").(string)),
	}

	err := client.graphql.Mutate(context.Background(), &mutation, vars)
	if err != nil {
		return diag.FromErr(err)
	}

	updateTeamMember(d, &mutation.TeamMemberCreate.TeamMemberEdge.Node)

	// theres a bug in teamMemberCreate that always sets role to MEMBER
	// so if using MAINTAINER, make a separate call to change the role if necessary
	if originalRole == "MAINTAINER" {
		d.Set("role", "MAINTAINER")
		UpdateTeamMember(ctx, d, m)
	}

	return nil
}

// ReadTeamMember retrieves a Buildkite team
func ReadTeamMember(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*Client)

	var query struct {
		Node struct {
			TeamMember TeamMemberNode `graphql:"... on TeamMember"`
		} `graphql:"node(id: $id)"`
	}

	vars := map[string]interface{}{
		"id": d.Id(),
	}

	err := client.graphql.Query(context.Background(), &query, vars)
	if err != nil {
		return diag.FromErr(err)
	}

	updateTeamMember(d, &query.Node.TeamMember)

	return nil
}

// UpdateTeamMember a team members role within a Buildkite team
func UpdateTeamMember(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*Client)

	var mutation struct {
		TeamMemberUpdate struct {
			TeamMember TeamMemberNode
		} `graphql:"teamMemberUpdate(input: {id: $id, role: $role})"`
	}

	vars := map[string]interface{}{
		"id":   d.Id(),
		"role": TeamMemberRole(d.Get("role").(string)),
	}

	err := client.graphql.Mutate(context.Background(), &mutation, vars)
	if err != nil {
		return diag.FromErr(err)
	}

	updateTeamMember(d, &mutation.TeamMemberUpdate.TeamMember)

	return nil
}

// DeleteTeamMember removes a user from a Buildkite team
func DeleteTeamMember(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*Client)
	var mutation struct {
		TeamDelete struct {
			DeletedTeamMemberId graphql.String `graphql:"deletedTeamMemberID"`
		} `graphql:"teamMemberDelete(input: {id: $id})"`
	}

	vars := map[string]interface{}{
		"id": graphql.ID(d.Id()),
	}

	err := client.graphql.Mutate(context.Background(), &mutation, vars)
	if err != nil {
		return diag.FromErr(err)
	}

	return nil
}

func updateTeamMember(d *schema.ResourceData, t *TeamMemberNode) {
	d.SetId(string(t.ID))
	d.Set("role", string(t.Role))
	d.Set("uuid", string(t.UUID))
	d.Set("team_id", string(t.Team.ID))
	d.Set("user_id", t.User.ID.(string))
}
