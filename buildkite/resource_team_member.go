package buildkite

import (
	"context"
	"fmt"
	"log"

	//"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	//"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	//"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
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

type TeamMemberResourceModel struct {
	Id     types.String `tfsdk:"id"`
	Uuid   types.String `tfsdk:"uuid"`
	Role   types.String `tfsdk:"role"`
	TeamId types.String `tfsdk:"team_id"`
	UserId types.String `tfsdk:"user_id"`
}

type TeamMemberResource struct {
	client *Client
}

func NewTeamMemberResource() resource.Resource {
	return &TeamMemberResource{}
}

func (TeamMemberResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_team_member"
}

func (tm *TeamMemberResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	tm.client = req.ProviderData.(*Client)
}

/*
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
*/

func (TeamMemberResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resource_schema.Schema{
		MarkdownDescription: "A team member resourc allows for the management of team team membership for existing organization users.",
		Attributes: map[string]resource_schema.Attribute{
			"id": resource_schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"uuid": resource_schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"team_id": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The GraphQL ID of the team to add/remove the user to/from.",
			},
			"user_id": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The GraphQL ID of the user to add/remove.",
			},
			"role": resource_schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The role for the user. Either MEMBER or MAINTAINER.",
			},
		},
	}
}

func (tm *TeamMemberResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, state TeamMemberResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("Creating team member into team %s ...", plan.TeamId.ValueString())
	apiResponse, err := createTeamMember(
		tm.client.genqlient,
		plan.TeamId.ValueString(),
		plan.UserId.ValueString(),
		TeamMemberRole(*plan.Role.ValueStringPointer()),
	)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create team member",
			fmt.Sprintf("Unable to create team member: %s", err.Error()),
		)
		return
	}

	// Update state with values from API response/plan
	state.Id = types.StringValue(apiResponse.TeamMemberCreate.TeamMemberEdge.Node.Id)
	state.Uuid = types.StringValue(apiResponse.TeamMemberCreate.TeamMemberEdge.Node.Uuid)
	state.TeamId = plan.TeamId
	state.UserId = plan.UserId
	// The role of the user will by default be "MEMBER" if none was entered in the plan
	state.Role = types.StringValue(string(apiResponse.TeamMemberCreate.TeamMemberEdge.Node.Role))

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (tm *TeamMemberResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state TeamMemberResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("Getting team member with ID %s ...", state.Id.ValueString())
	teamMember, err := getNode(tm.client.genqlient, state.Id.ValueString())

	test:= teamMember.GetNode().implementsGraphQLInterfacegetNodeNode().

	if err != nil {
		resp.Diagnostics.AddError(err.Error(), err.Error())
	}
	if teamMember == nil {
		resp.Diagnostics.AddError("Team member not found", "Removing from state")
		resp.State.RemoveResource(ctx)
		return
	}

	state.Id = t
}

func (tm *TeamMemberResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {

}

func (tm *TeamMemberResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var id, role string

	// Obtain team member's ID from state, new role from plan
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &id)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("role"), &role)...)

	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("Updating team member %s with role %s ...", id, role)
	apiResponse, err := updateTeamMember(tm.client.genqlient, id, TeamMemberRole(role))

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to update Team member",
			fmt.Sprintf("Unable to update Team member: %s", err.Error()),
		)
		return
	}

	// Update state with revised role
	newRole := types.StringValue(string(apiResponse.TeamMemberUpdate.TeamMember.Role))
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("role"), newRole)...)
}

func (tm *TeamMemberResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var id string

	// Obtain team member's ID from state
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &id)...)

	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("Deleting team member with ID %s ...", id)
	_, err := deleteTeamMember(tm.client.genqlient, id)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete team member",
			fmt.Sprintf("Unable to delete team member: %s", err.Error()),
		)
		return
	}
}

/*
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
	d.Set("user_id", t.User.ID)
}
*/
