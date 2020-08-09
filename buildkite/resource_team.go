package buildkite

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/shurcooL/graphql"
)

type TeamPrivacy graphql.String
type TeamMemberRole graphql.String

type TeamNode struct {
	Description               graphql.String
	ID                        graphql.String
	IsDefaultTeam             graphql.Boolean
	Name                      graphql.String
	MembersCanCreatePipelines graphql.Boolean
	Privacy                   TeamPrivacy
	Slug                      graphql.String
	UUID                      graphql.String
}

func resourceTeam() *schema.Resource {
	return &schema.Resource{
		Create: CreateTeam,
		Read:   ReadTeam,
		Update: UpdateTeam,
		Delete: DeleteTeam,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"description": &schema.Schema{
				Optional: true,
				Type:     schema.TypeString,
			},
			"name": &schema.Schema{
				Required: true,
				Type:     schema.TypeString,
			},
			"privacy": &schema.Schema{
				Required: true,
				Type:     schema.TypeString,
				ValidateFunc: func(val interface{}, key string) (warns []string, errs []error) {
					v := val.(string)
					switch v {
					case "VISIBLE":
					case "SECRET":
						return
					default:
						errs = append(errs, fmt.Errorf("%q must be either VISIBLE or SECRET, got: %s", key, v))
						return
					}
					return
				},
			},
			"default_team": &schema.Schema{
				Required: true,
				Type:     schema.TypeBool,
			},
			"default_member_role": &schema.Schema{
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
			"members_can_create_pipelines": &schema.Schema{
				Optional: true,
				Type:     schema.TypeBool,
			},
			"slug": &schema.Schema{
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

// CreateTeam creates a Buildkite team
func CreateTeam(d *schema.ResourceData, m interface{}) error {
	client := m.(*Client)
	id, err := GetOrganizationID(client.organization, client.graphql)
	if err != nil {
		return err
	}

	var mutation struct {
		TeamCreate struct {
			TeamEdge struct {
				Node TeamNode
			}
		} `graphql:"teamCreate(input: {organizationID: $org, name: $name, description: $desc, privacy: $privacy, isDefaultTeam: $default_team, defaultMemberRole: $default_member_role, membersCanCreatePipelines: $members_can_create_pipelines})"`
	}

	vars := map[string]interface{}{
		"org":                          id,
		"name":                         graphql.String(d.Get("name").(string)),
		"desc":                         graphql.String(d.Get("description").(string)),
		"privacy":                      TeamPrivacy(d.Get("privacy").(string)),
		"default_team":                 graphql.Boolean(d.Get("default_team").(bool)),
		"default_member_role":          TeamMemberRole(d.Get("default_member_role").(string)),
		"members_can_create_pipelines": graphql.Boolean(d.Get("members_can_create_pipelines").(bool)),
	}

	err = client.graphql.Mutate(context.Background(), &mutation, vars)
	if err != nil {
		return err
	}

	updateTeam(d, &mutation.TeamCreate.TeamEdge.Node)

	return nil
}

// ReadTeam retrieves a Buildkite team
func ReadTeam(d *schema.ResourceData, m interface{}) error {
	client := m.(*Client)

	var query struct {
		Node struct {
			Team TeamNode `graphql:"... on Team"`
		} `graphql:"node(id: $id)"`
	}

	vars := map[string]interface{}{
		"id": d.Id(),
	}

	err := client.graphql.Query(context.Background(), &query, vars)
	if err != nil {
		return err
	}

	updateTeam(d, &query.Node.Team)

	return nil
}

// UpdateTeam updates a Buildkite team
func UpdateTeam(d *schema.ResourceData, m interface{}) error {
	client := m.(*Client)

	var mutation struct {
		TeamUpdate struct {
			Team TeamNode
		} `graphql:"teamUpdate(input: {id: $id, name: $name, description: $desc, privacy: $privacy, isDefaultTeam: $default_team, defaultMemberRole: $default_member_role, membersCanCreatePipelines: $members_can_create_pipelines})"`
	}

	vars := map[string]interface{}{
		"id":                           d.Id(),
		"name":                         graphql.String(d.Get("name").(string)),
		"desc":                         graphql.String(d.Get("description").(string)),
		"privacy":                      TeamPrivacy(d.Get("privacy").(string)),
		"default_team":                 graphql.Boolean(d.Get("default_team").(bool)),
		"default_member_role":          TeamMemberRole(d.Get("default_member_role").(string)),
		"members_can_create_pipelines": graphql.Boolean(d.Get("members_can_create_pipelines").(bool)),
	}

	err := client.graphql.Mutate(context.Background(), &mutation, vars)
	if err != nil {
		return err
	}

	updateTeam(d, &mutation.TeamUpdate.Team)

	return nil
}

// DeleteTeam removes a Buildkite team
func DeleteTeam(d *schema.ResourceData, m interface{}) error {
	client := m.(*Client)
	var mutation struct {
		TeamDelete struct {
			DeletedTeamId graphql.String `graphql:"deletedTeamID"`
		} `graphql:"teamDelete(input: {id: $id})"`
	}

	vars := map[string]interface{}{
		"id": graphql.ID(d.Id()),
	}

	err := client.graphql.Mutate(context.Background(), &mutation, vars)
	if err != nil {
		return err
	}

	return nil
}

func updateTeam(d *schema.ResourceData, t *TeamNode) {
	d.SetId(string(t.ID))
	d.Set("default_team", bool(t.IsDefaultTeam))
	d.Set("description", string(t.Description))
	d.Set("members_can_create_pipelines", bool(t.MembersCanCreatePipelines))
	d.Set("name", string(t.Name))
	d.Set("privacy", string(t.Privacy))
	d.Set("slug", string(t.Slug))
	d.Set("uuid", string(t.UUID))
}
