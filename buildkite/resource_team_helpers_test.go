package buildkite

import (
	"encoding/base64"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestUUIDFromGraphQLID(t *testing.T) {
	tests := []struct {
		name      string
		graphqlID string
		wantUUID  string
		wantErr   bool
	}{
		{
			name:      "valid team GraphQL ID",
			graphqlID: base64.StdEncoding.EncodeToString([]byte("Team---c5e09619-8648-4896-a936-9d0b8b7b3fe9")),
			wantUUID:  "c5e09619-8648-4896-a936-9d0b8b7b3fe9",
		},
		{
			name:      "valid team GraphQL ID with different UUID",
			graphqlID: base64.StdEncoding.EncodeToString([]byte("Team---8497e6e1-8a4d-492e-9ff5-7dc97978fcb8")),
			wantUUID:  "8497e6e1-8a4d-492e-9ff5-7dc97978fcb8",
		},
		{
			name:      "invalid base64",
			graphqlID: "not-valid-base64!@#",
			wantErr:   true,
		},
		{
			name:      "valid base64 but no separator",
			graphqlID: base64.StdEncoding.EncodeToString([]byte("Team_no_separator")),
			wantErr:   true,
		},
		{
			name:      "empty string",
			graphqlID: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := uuidFromGraphQLID(tt.graphqlID)
			if (err != nil) != tt.wantErr {
				t.Errorf("uuidFromGraphQLID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.wantUUID {
				t.Errorf("uuidFromGraphQLID() = %q, want %q", got, tt.wantUUID)
			}
		})
	}
}

func TestTeamUUIDFromState(t *testing.T) {
	tests := []struct {
		name     string
		state    teamResourceModel
		wantUUID string
		wantErr  bool
	}{
		{
			name: "UUID is set",
			state: teamResourceModel{
				UUID: types.StringValue("c5e09619-8648-4896-a936-9d0b8b7b3fe9"),
				ID:   types.StringValue("some-graphql-id"),
			},
			wantUUID: "c5e09619-8648-4896-a936-9d0b8b7b3fe9",
		},
		{
			name: "UUID is empty but GraphQL ID is set",
			state: teamResourceModel{
				UUID: types.StringValue(""),
				ID:   types.StringValue(base64.StdEncoding.EncodeToString([]byte("Team---8497e6e1-8a4d-492e-9ff5-7dc97978fcb8"))),
			},
			wantUUID: "8497e6e1-8a4d-492e-9ff5-7dc97978fcb8",
		},
		{
			name: "UUID is null but GraphQL ID is set",
			state: teamResourceModel{
				UUID: types.StringNull(),
				ID:   types.StringValue(base64.StdEncoding.EncodeToString([]byte("Team---8497e6e1-8a4d-492e-9ff5-7dc97978fcb8"))),
			},
			wantUUID: "8497e6e1-8a4d-492e-9ff5-7dc97978fcb8",
		},
		{
			name: "both null",
			state: teamResourceModel{
				UUID: types.StringNull(),
				ID:   types.StringNull(),
			},
			wantErr: true,
		},
		{
			name: "both empty",
			state: teamResourceModel{
				UUID: types.StringValue(""),
				ID:   types.StringValue(""),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := teamUUIDFromState(&tt.state)
			if (err != nil) != tt.wantErr {
				t.Errorf("teamUUIDFromState() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.wantUUID {
				t.Errorf("teamUUIDFromState() = %q, want %q", got, tt.wantUUID)
			}
		})
	}
}

func TestUpdateTeamResourceStateFromREST(t *testing.T) {
	res := &teamAPIResponse{
		ID:                          "8497e6e1-8a4d-492e-9ff5-7dc97978fcb8",
		GraphQLID:                   "VGVhbS0tLTg0OTdlNmUxLThhNGQtNDkyZS05ZmY1LTdkYzk3OTc4ZmNiOA==",
		Name:                        "Agent Contractors",
		Slug:                        "agent-contractors",
		Description:                 "Limited access for contractors",
		Privacy:                     "visible",
		Default:                     false,
		DefaultMemberRole:           "member",
		MembersCanCreatePipelines:   true,
		MembersCanCreateSuites:      false,
		MembersCanCreateRegistries:  true,
		MembersCanDestroyRegistries: false,
		MembersCanDestroyPackages:   false,
	}

	var state teamResourceModel
	updateTeamResourceStateFromREST(&state, res)

	// Verify GraphQL ID goes to state.ID
	if state.ID.ValueString() != res.GraphQLID {
		t.Errorf("ID = %q, want %q", state.ID.ValueString(), res.GraphQLID)
	}

	// Verify REST id (UUID) goes to state.UUID
	if state.UUID.ValueString() != res.ID {
		t.Errorf("UUID = %q, want %q", state.UUID.ValueString(), res.ID)
	}

	if state.Name.ValueString() != "Agent Contractors" {
		t.Errorf("Name = %q, want %q", state.Name.ValueString(), "Agent Contractors")
	}

	if state.Slug.ValueString() != "agent-contractors" {
		t.Errorf("Slug = %q, want %q", state.Slug.ValueString(), "agent-contractors")
	}

	if state.Description.ValueString() != "Limited access for contractors" {
		t.Errorf("Description = %q, want %q", state.Description.ValueString(), "Limited access for contractors")
	}

	// Verify lowercase -> uppercase conversion
	if state.Privacy.ValueString() != "VISIBLE" {
		t.Errorf("Privacy = %q, want %q", state.Privacy.ValueString(), "VISIBLE")
	}

	if state.DefaultMemberRole.ValueString() != "MEMBER" {
		t.Errorf("DefaultMemberRole = %q, want %q", state.DefaultMemberRole.ValueString(), "MEMBER")
	}

	if state.IsDefaultTeam.ValueBool() != false {
		t.Errorf("IsDefaultTeam = %v, want false", state.IsDefaultTeam.ValueBool())
	}

	if state.MembersCanCreatePipelines.ValueBool() != true {
		t.Errorf("MembersCanCreatePipelines = %v, want true", state.MembersCanCreatePipelines.ValueBool())
	}

	if state.MembersCanCreateSuites.ValueBool() != false {
		t.Errorf("MembersCanCreateSuites = %v, want false", state.MembersCanCreateSuites.ValueBool())
	}

	if state.MembersCanCreateRegistries.ValueBool() != true {
		t.Errorf("MembersCanCreateRegistries = %v, want true", state.MembersCanCreateRegistries.ValueBool())
	}

	if state.MembersCanDestroyRegistries.ValueBool() != false {
		t.Errorf("MembersCanDestroyRegistries = %v, want false", state.MembersCanDestroyRegistries.ValueBool())
	}

	if state.MembersCanDestroyPackages.ValueBool() != false {
		t.Errorf("MembersCanDestroyPackages = %v, want false", state.MembersCanDestroyPackages.ValueBool())
	}
}

func TestUpdateTeamResourceStateFromREST_SecretMaintainer(t *testing.T) {
	res := &teamAPIResponse{
		Privacy:           "secret",
		DefaultMemberRole: "maintainer",
	}

	var state teamResourceModel
	updateTeamResourceStateFromREST(&state, res)

	if state.Privacy.ValueString() != "SECRET" {
		t.Errorf("Privacy = %q, want %q", state.Privacy.ValueString(), "SECRET")
	}

	if state.DefaultMemberRole.ValueString() != "MAINTAINER" {
		t.Errorf("DefaultMemberRole = %q, want %q", state.DefaultMemberRole.ValueString(), "MAINTAINER")
	}
}

func TestUpdateTeamResourceStateFromREST_EmptyDescription(t *testing.T) {
	res := &teamAPIResponse{
		Description: "",
	}

	var state teamResourceModel
	updateTeamResourceStateFromREST(&state, res)

	if state.Description.ValueString() != "" {
		t.Errorf("Description = %q, want empty string", state.Description.ValueString())
	}
}

func TestUpdateTeamResourceStateFromREST_EmptyGraphQLID(t *testing.T) {
	existingID := "VGVhbS0tLWV4aXN0aW5n"
	state := teamResourceModel{
		ID: types.StringValue(existingID),
	}
	res := &teamAPIResponse{
		GraphQLID: "",
		ID:        "some-uuid",
	}

	updateTeamResourceStateFromREST(&state, res)

	if state.ID.ValueString() != existingID {
		t.Errorf("ID was overwritten to %q, want existing value %q", state.ID.ValueString(), existingID)
	}
	if state.UUID.ValueString() != "some-uuid" {
		t.Errorf("UUID = %q, want %q", state.UUID.ValueString(), "some-uuid")
	}
}

func TestBuildTeamRequest(t *testing.T) {
	state := &teamResourceModel{
		Name:                        types.StringValue("My Team"),
		Description:                 types.StringValue("A description"),
		Privacy:                     types.StringValue("VISIBLE"),
		IsDefaultTeam:               types.BoolValue(true),
		DefaultMemberRole:           types.StringValue("MEMBER"),
		MembersCanCreatePipelines:   types.BoolValue(true),
		MembersCanCreateSuites:      types.BoolValue(false),
		MembersCanCreateRegistries:  types.BoolValue(true),
		MembersCanDestroyRegistries: types.BoolValue(false),
		MembersCanDestroyPackages:   types.BoolValue(false),
	}

	req := buildTeamRequest(state)

	if req.Name != "My Team" {
		t.Errorf("Name = %q, want %q", req.Name, "My Team")
	}

	// Verify uppercase -> lowercase conversion for REST API
	if req.Privacy != "visible" {
		t.Errorf("Privacy = %q, want %q", req.Privacy, "visible")
	}

	if req.DefaultMemberRole != "member" {
		t.Errorf("DefaultMemberRole = %q, want %q", req.DefaultMemberRole, "member")
	}

	if req.IsDefaultTeam != true {
		t.Errorf("IsDefaultTeam = %v, want true", req.IsDefaultTeam)
	}

	if req.MembersCanCreatePipelines != true {
		t.Errorf("MembersCanCreatePipelines = %v, want true", req.MembersCanCreatePipelines)
	}

	if req.MembersCanCreateSuites != false {
		t.Errorf("MembersCanCreateSuites = %v, want false", req.MembersCanCreateSuites)
	}
}

func TestBuildTeamRequest_SecretMaintainer(t *testing.T) {
	state := &teamResourceModel{
		Name:              types.StringValue("Secret Team"),
		Description:       types.StringValue(""),
		Privacy:           types.StringValue("SECRET"),
		IsDefaultTeam:     types.BoolValue(false),
		DefaultMemberRole: types.StringValue("MAINTAINER"),
	}

	req := buildTeamRequest(state)

	if req.Privacy != "secret" {
		t.Errorf("Privacy = %q, want %q", req.Privacy, "secret")
	}

	if req.DefaultMemberRole != "maintainer" {
		t.Errorf("DefaultMemberRole = %q, want %q", req.DefaultMemberRole, "maintainer")
	}
}
