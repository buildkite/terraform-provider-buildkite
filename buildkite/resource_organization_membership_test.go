package buildkite

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

const (
	membershipUserUUID = "00000000-0000-4000-8000-000000000002"
	membershipUserID   = "VXNlci0tLTAwMDAwMDAwLTAwMDAtNDAwMC04MDAwLTAwMDAwMDAwMDAwMg=="
	membershipAddress  = "buildkite_organization_membership.jane"
)

// The wire fixtures deliberately use JSON field names independently of the
// production structs. Terraform's actual CLI drives the provider against this API.
type membershipAPI struct {
	t                 *testing.T
	mu                sync.Mutex
	member            map[string]any
	invitation        map[string]any
	created           int
	revoked           int
	deleted           int
	patches           []map[string]string
	acceptOnRevoke    bool
	status            int
	patchStatus       int
	createStatus      int
	memberPageTwo     bool
	invitationPageTwo bool
}

func newMembershipAPI(t *testing.T) (*httptest.Server, *membershipAPI) {
	t.Helper()
	a := &membershipAPI{t: t}
	s := httptest.NewServer(a)
	t.Cleanup(s.Close)
	return s, a
}

func (a *membershipAPI) accept() {
	a.member = map[string]any{
		"id": membershipUserUUID, "email": "jane@example.com",
		"role": a.invitation["role"], "sso_mode": a.invitation["sso_mode"],
	}
	a.invitation["state"] = "accepted"
	a.invitation["accepted_by"] = map[string]any{"id": membershipUserUUID}
}

func (a *membershipAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if a.status != 0 {
		http.Error(w, `{"message":"not found is only prose, not a 404 status"}`, a.status)
		return
	}
	const base = "/v2/organizations/acme/"
	var result any
	switch {
	case r.Method == "GET" && r.URL.Path == base+"members":
		members := []map[string]any{}
		if a.memberPageTwo && r.URL.Query().Get("page") == "1" {
			for range 100 {
				members = append(members, map[string]any{"id": "other", "email": "other@example.com"})
			}
		} else if a.member != nil {
			members = append(members, a.member)
		}
		result = members
	case r.Method == "GET" && r.URL.Path == base+"members/"+membershipUserUUID:
		if a.member == nil {
			http.NotFound(w, r)
			return
		}
		result = a.member
	case r.Method == "PATCH" && r.URL.Path == base+"members/"+membershipUserUUID:
		if a.patchStatus != 0 {
			http.Error(w, `{"message":"cannot update member"}`, a.patchStatus)
			return
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			a.t.Error(err)
		}
		if a.member == nil {
			http.NotFound(w, r)
			return
		}
		for k, v := range payload {
			if (k == "role" && v != "admin" && v != "member") || (k == "sso_mode" && v != "required" && v != "optional") {
				a.t.Errorf("unexpected patch: %v", payload)
			}
			a.member[k] = v
		}
		a.patches = append(a.patches, payload)
		result = a.member
	case r.Method == "DELETE" && r.URL.Path == base+"members/"+membershipUserUUID:
		a.member = nil
		a.deleted++
		w.WriteHeader(http.StatusNoContent)
		return
	case r.Method == "GET" && r.URL.Path == base+"invitations":
		items := []map[string]any{}
		links := map[string]string{}
		if a.invitationPageTwo && r.URL.Query().Get("after") == "" {
			items = append(items, map[string]any{"id": "other", "email": "other@example.com", "state": "pending"})
			links["next"] = "http://" + r.Host + base + "invitations?per_page=100&after=opaque%2Bcursor"
		} else if a.invitation != nil && a.invitation["state"] == "pending" {
			if a.invitationPageTwo && r.URL.Query().Get("after") != "opaque+cursor" {
				a.t.Errorf("cursor changed: %s", r.URL.RawQuery)
			}
			items = append(items, a.invitation)
		}
		result = map[string]any{"items": items, "links": links}
	case r.Method == "POST" && r.URL.Path == base+"invitations":
		if a.createStatus != 0 {
			http.Error(w, `{"message":"cannot create invitation"}`, a.createStatus)
			return
		}
		var payload struct {
			Emails  []string `json:"emails"`
			Role    string   `json:"role"`
			SSOMode string   `json:"sso_mode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			a.t.Error(err)
		}
		if len(payload.Emails) != 1 || payload.Emails[0] != "jane@example.com" ||
			(payload.Role != "admin" && payload.Role != "member") ||
			(payload.SSOMode != "required" && payload.SSOMode != "optional") {
			a.t.Errorf("invalid invitation payload: %+v", payload)
		}
		if a.member != nil || (a.invitation != nil && a.invitation["state"] == "pending") {
			http.Error(w, `{"message":"already a member or invited"}`, http.StatusUnprocessableEntity)
			return
		}
		a.created++
		a.invitation = map[string]any{
			"id":    fmt.Sprintf("00000000-0000-4000-8001-%012d", a.created),
			"email": payload.Emails[0], "state": "pending", "role": payload.Role, "sso_mode": payload.SSOMode,
		}
		result = []map[string]any{a.invitation}
		w.WriteHeader(http.StatusCreated)
	case strings.HasPrefix(r.URL.Path, base+"invitations/") && (r.Method == "GET" || r.Method == "DELETE"):
		if a.invitation == nil || strings.TrimPrefix(r.URL.Path, base+"invitations/") != a.invitation["id"] {
			http.NotFound(w, r)
			return
		}
		if r.Method == "DELETE" {
			if a.acceptOnRevoke {
				a.acceptOnRevoke = false
				a.accept()
			}
			if a.invitation["state"] != "pending" {
				http.Error(w, `{"message":"invitation is not pending"}`, http.StatusUnprocessableEntity)
				return
			}
			a.invitation["state"] = "revoked"
			a.revoked++
			w.WriteHeader(http.StatusNoContent)
			return
		}
		result = a.invitation
	default:
		a.t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		http.NotFound(w, r)
		return
	}
	if err := json.NewEncoder(w).Encode(result); err != nil {
		a.t.Error(err)
	}
}

func membershipConfig(server, attributes string) string {
	return fmt.Sprintf(`
provider "buildkite" {
  organization = "acme"
  api_token = "fake"
  rest_url = %q
  max_retries = 0
}
resource "buildkite_organization_membership" "jane" {
  %s
}
`, server, attributes)
}

func TestOrganizationMembershipAdoptUpdateImport(t *testing.T) {
	for _, identity := range []string{`email = "jane@example.com"`, `uuid = "` + membershipUserUUID + `"`} {
		t.Run(identity, func(t *testing.T) {
			s, api := newMembershipAPI(t)
			api.member = map[string]any{"id": membershipUserUUID, "email": "jane@example.com", "role": "member", "sso_mode": "required"}
			api.memberPageTwo = true
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				Steps: []resource.TestStep{
					{
						Config:           membershipConfig(s.URL, identity+"\nrole = \"ADMIN\"\nsso_mode = \"OPTIONAL\""),
						ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr(membershipAddress, "id", membershipUserUUID),
							resource.TestCheckResourceAttr(membershipAddress, "state", "active"),
							resource.TestCheckResourceAttr(membershipAddress, "user_id", membershipUserID),
							resource.TestCheckResourceAttr(membershipAddress, "role", "ADMIN"),
							resource.TestCheckResourceAttr(membershipAddress, "sso_mode", "OPTIONAL"),
						),
					},
					{ResourceName: membershipAddress, ImportState: true, ImportStateVerify: true},
					{
						Config: membershipConfig(s.URL, identity+"\nrole = \"MEMBER\"\nsso_mode = \"REQUIRED\""),
						ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction(membershipAddress, plancheck.ResourceActionUpdate),
							// Unknown identity values would force replacement of dependent team memberships.
							plancheck.ExpectKnownValue(membershipAddress, tfjsonpath.New("user_id"), knownvalue.StringExact(membershipUserID)),
							plancheck.ExpectKnownValue(membershipAddress, tfjsonpath.New("uuid"), knownvalue.StringExact(membershipUserUUID)),
						}},
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr(membershipAddress, "role", "MEMBER"),
							resource.TestCheckResourceAttr(membershipAddress, "sso_mode", "REQUIRED"),
						),
					},
				},
			})
			api.mu.Lock()
			defer api.mu.Unlock()
			if api.created != 0 || api.deleted != 1 || api.member != nil || len(api.patches) != 2 {
				t.Fatalf("adoption lifecycle: created=%d deleted=%d member=%v patches=%v", api.created, api.deleted, api.member, api.patches)
			}
		})
	}
}

func TestOrganizationMembershipInvitationLifecycle(t *testing.T) {
	s, api := newMembershipAPI(t)
	const firstID = "00000000-0000-4000-8001-000000000001"
	initial := membershipConfig(s.URL, `email = "jane@example.com"
role = "ADMIN"
sso_mode = "OPTIONAL"
send_invitation = true`)
	updated := membershipConfig(s.URL, `email = "jane@example.com"
role = "MEMBER"
sso_mode = "REQUIRED"
send_invitation = true`)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: initial,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(membershipAddress, "state", "pending"),
					resource.TestCheckResourceAttr(membershipAddress, "id", firstID),
					resource.TestCheckNoResourceAttr(membershipAddress, "user_id"),
					resource.TestCheckNoResourceAttr(membershipAddress, "uuid"),
				),
			},
			{
				ResourceName: membershipAddress, ImportState: true, ImportStateId: "invitation/" + firstID,
				ImportStateVerify: true, ImportStateVerifyIgnore: []string{"send_invitation"},
			},
			{
				Config: updated,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(membershipAddress, "id", firstID),
					resource.TestCheckResourceAttr(membershipAddress, "invitation_id", "00000000-0000-4000-8001-000000000002"),
					resource.TestCheckResourceAttr(membershipAddress, "role", "MEMBER"),
					resource.TestCheckResourceAttr(membershipAddress, "sso_mode", "REQUIRED"),
				),
			},
			{
				PreConfig: func() {
					api.mu.Lock()
					defer api.mu.Unlock()
					api.accept()
					// An invitation may be accepted by a user whose primary email differs.
					api.member["email"] = "jane.primary@example.com"
				},
				Config:           updated,
				ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(membershipAddress, "id", firstID),
					resource.TestCheckResourceAttr(membershipAddress, "email", "jane@example.com"),
					resource.TestCheckResourceAttr(membershipAddress, "state", "active"),
					resource.TestCheckResourceAttr(membershipAddress, "uuid", membershipUserUUID),
					resource.TestCheckResourceAttr(membershipAddress, "user_id", membershipUserID),
				),
			},
		},
	})
	api.mu.Lock()
	defer api.mu.Unlock()
	if api.created != 2 || api.revoked != 1 || api.deleted != 1 {
		t.Fatalf("invitation lifecycle: created=%d revoked=%d deleted=%d", api.created, api.revoked, api.deleted)
	}
}

func TestOrganizationMembershipDowngradeOnDestroy(t *testing.T) {
	s, api := newMembershipAPI(t)
	api.member = map[string]any{"id": membershipUserUUID, "email": "jane@example.com", "role": "admin", "sso_mode": "optional"}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{{Config: membershipConfig(s.URL, `email = "jane@example.com"
role = "ADMIN"
sso_mode = "OPTIONAL"
downgrade_on_destroy = true`)}},
	})
	api.mu.Lock()
	defer api.mu.Unlock()
	if api.deleted != 0 || api.member["role"] != "member" || api.member["sso_mode"] != "optional" || len(api.patches) != 1 || len(api.patches[0]) != 1 {
		t.Fatalf("destroy must only demote, retaining SSO mode: member=%v patches=%v deleted=%d", api.member, api.patches, api.deleted)
	}
}

func TestOrganizationMembershipAdoptPendingAndExpire(t *testing.T) {
	s, api := newMembershipAPI(t)
	api.invitationPageTwo = true
	api.invitation = map[string]any{"id": "00000000-0000-4000-8001-000000000099", "email": "JANE@example.com", "state": "pending", "role": "member", "sso_mode": "required"}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{Config: membershipConfig(s.URL, `email = "jane@example.com"
role = "MEMBER"
sso_mode = "REQUIRED"
downgrade_on_destroy = true`), Check: resource.TestCheckResourceAttr(membershipAddress, "invitation_id", "00000000-0000-4000-8001-000000000099")},
			{
				PreConfig: func() {
					api.mu.Lock()
					defer api.mu.Unlock()
					if api.created != 0 {
						t.Error("adoption sent a duplicate invitation")
					}
					api.invitation["state"] = "expired"
				},
				Config: membershipConfig(s.URL, `email = "jane@example.com"
role = "MEMBER"
sso_mode = "REQUIRED"
send_invitation = true
downgrade_on_destroy = true`),
				Check: resource.TestCheckResourceAttr(membershipAddress, "invitation_id", "00000000-0000-4000-8001-000000000001"),
			},
		},
	})
	api.mu.Lock()
	defer api.mu.Unlock()
	if api.created != 1 || api.revoked != 1 || api.deleted != 0 {
		t.Fatalf("pending destroy must revoke even with downgrade: created=%d revoked=%d deleted=%d", api.created, api.revoked, api.deleted)
	}
}

func TestOrganizationMembershipMissingDoesNotInvite(t *testing.T) {
	s, api := newMembershipAPI(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{{
			Config: membershipConfig(s.URL, `email = "jane@example.com"
role = "MEMBER"
sso_mode = "REQUIRED"`),
			ExpectError: regexp.MustCompile("organization member not found"),
		}},
	})
	api.mu.Lock()
	defer api.mu.Unlock()
	if api.created != 0 {
		t.Fatal("sent an invitation without opt-in")
	}
}

func TestOrganizationMembershipFailedAdoptionDoesNotTaintMember(t *testing.T) {
	s, api := newMembershipAPI(t)
	api.member = map[string]any{"id": membershipUserUUID, "email": "jane@example.com", "role": "member", "sso_mode": "required"}
	api.patchStatus = http.StatusForbidden
	config := membershipConfig(s.URL, `email = "jane@example.com"
role = "ADMIN"
sso_mode = "OPTIONAL"`)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{Config: config, ExpectError: regexp.MustCompile("status: 403")},
			{
				PreConfig: func() { api.mu.Lock(); defer api.mu.Unlock(); api.patchStatus = 0 },
				Config:    config,
				Check:     resource.TestCheckResourceAttr(membershipAddress, "role", "ADMIN"),
			},
		},
	})
	api.mu.Lock()
	defer api.mu.Unlock()
	if api.deleted != 1 || len(api.patches) != 1 {
		t.Fatalf("retry must adopt, not destroy a tainted member: deleted=%d patches=%v", api.deleted, api.patches)
	}
}

func TestOrganizationMembershipFailedInvitationReplacementRecovers(t *testing.T) {
	s, api := newMembershipAPI(t)
	initial := membershipConfig(s.URL, `email = "jane@example.com"
role = "MEMBER"
sso_mode = "REQUIRED"
send_invitation = true`)
	updated := membershipConfig(s.URL, `email = "jane@example.com"
role = "ADMIN"
sso_mode = "OPTIONAL"
send_invitation = true`)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{Config: initial},
			{
				PreConfig: func() { api.mu.Lock(); defer api.mu.Unlock(); api.createStatus = http.StatusForbidden },
				Config:    updated, ExpectError: regexp.MustCompile("status: 403"),
			},
			{
				PreConfig: func() { api.mu.Lock(); defer api.mu.Unlock(); api.createStatus = 0 },
				Config:    updated,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(membershipAddress, "invitation_id", "00000000-0000-4000-8001-000000000002"),
					resource.TestCheckResourceAttr(membershipAddress, "role", "ADMIN"),
					resource.TestCheckResourceAttr(membershipAddress, "sso_mode", "OPTIONAL"),
				),
			},
		},
	})
	api.mu.Lock()
	defer api.mu.Unlock()
	if api.created != 2 || api.revoked != 2 {
		t.Fatalf("unexpected replacement recovery: created=%d revoked=%d", api.created, api.revoked)
	}
}

func TestOrganizationMembershipPreservesPendingTeamAssignments(t *testing.T) {
	s, api := newMembershipAPI(t)
	api.invitation = map[string]any{
		"id": "invite", "email": "jane@example.com", "state": "pending", "role": "member", "sso_mode": "required",
		"teams": []map[string]any{{"id": "engineering"}},
	}
	r := &organizationMembershipResource{client: &Client{organization: "acme", restURL: s.URL, http: s.Client()}}
	state := organizationMembershipResourceModel{
		Email: types.StringValue("jane@example.com"), Role: types.StringValue("ADMIN"),
		SSOMode: types.StringValue("OPTIONAL"), SendInvitation: types.BoolValue(true),
	}
	err := r.apply(context.Background(), &state)
	if err == nil || !strings.Contains(err.Error(), "team assignments") {
		t.Fatalf("expected refusal to discard team assignments, got %v", err)
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	if api.revoked != 0 || api.created != 0 || api.invitation["state"] != "pending" {
		t.Fatal("invitation with unmanaged teams was replaced")
	}
}

func TestOrganizationMembershipAcceptanceRaces(t *testing.T) {
	for _, operation := range []string{"update", "remove", "downgrade"} {
		t.Run(operation, func(t *testing.T) {
			s, api := newMembershipAPI(t)
			api.invitation = map[string]any{"id": "invite", "email": "jane@example.com", "state": "pending", "role": "admin", "sso_mode": "optional"}
			api.acceptOnRevoke = true
			r := &organizationMembershipResource{client: &Client{organization: "acme", restURL: s.URL, http: s.Client()}}
			state := organizationMembershipResourceModel{
				ID: types.StringValue("invite"), Email: types.StringValue("jane@example.com"), InvitationID: types.StringValue("invite"),
				Role: types.StringValue("MEMBER"), SSOMode: types.StringValue("REQUIRED"), SendInvitation: types.BoolValue(true),
				DowngradeOnDestroy: types.BoolValue(operation == "downgrade"),
			}
			var err error
			if operation == "update" {
				err = r.apply(context.Background(), &state)
			} else {
				err = r.remove(context.Background(), &state)
			}
			if err != nil {
				t.Fatal(err)
			}
			api.mu.Lock()
			defer api.mu.Unlock()
			if api.created != 0 || api.revoked != 0 {
				t.Fatal("accepted invitation was replaced or revoked")
			}
			if operation == "remove" {
				if api.deleted != 1 || api.member != nil {
					t.Fatal("accepted member was left behind")
				}
			} else {
				wantSSO := "optional"
				if operation == "update" {
					wantSSO = "required"
				}
				if api.deleted != 0 || api.member["role"] != "member" || api.member["sso_mode"] != wantSSO {
					t.Fatalf("wrong result after acceptance race: %v", api.member)
				}
			}
		})
	}
}

func TestOrganizationMembershipReadErrorsPreserveState(t *testing.T) {
	for _, status := range []int{403, 404, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			s, api := newMembershipAPI(t)
			api.status = status
			r := &organizationMembershipResource{client: &Client{organization: "acme", restURL: s.URL, http: s.Client()}}
			var schemaResp frameworkresource.SchemaResponse
			r.Schema(context.Background(), frameworkresource.SchemaRequest{}, &schemaResp)
			state := tfsdk.State{Schema: schemaResp.Schema}
			model := organizationMembershipResourceModel{ID: types.StringValue(membershipUserUUID), UUID: types.StringValue(membershipUserUUID)}
			if diags := state.Set(context.Background(), &model); diags.HasError() {
				t.Fatal(diags)
			}
			response := frameworkresource.ReadResponse{State: state}
			r.Read(context.Background(), frameworkresource.ReadRequest{State: state}, &response)
			if status == 404 {
				if response.Diagnostics.HasError() || !response.State.Raw.IsNull() {
					t.Fatal("404 must remove the resource")
				}
			} else if !response.Diagnostics.HasError() || !response.State.Raw.Equal(state.Raw) {
				t.Fatal("API error must be reported without losing state")
			}
		})
	}
}

type membershipDeadlineTransport struct {
	http.RoundTripper
	deadlines []time.Time
}

func (t *membershipDeadlineTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	deadline, _ := req.Context().Deadline()
	t.deadlines = append(t.deadlines, deadline)
	return t.RoundTripper.RoundTrip(req)
}

func TestOrganizationMembershipOperationDeadlines(t *testing.T) {
	// Distinct budgets catch accidentally using the read timeout for every operation.
	budgets := map[string]time.Duration{"create": time.Hour, "read": 2 * time.Hour, "update": 3 * time.Hour, "delete": 4 * time.Hour}
	attributeTypes := map[string]attr.Type{}
	attributeValues := map[string]attr.Value{}
	for operation, budget := range budgets {
		attributeTypes[operation] = types.StringType
		attributeValues[operation] = types.StringValue(budget.String())
	}

	for operation, budget := range budgets {
		t.Run(operation, func(t *testing.T) {
			s, api := newMembershipAPI(t)
			api.member = map[string]any{"id": membershipUserUUID, "email": "jane@example.com", "role": "member", "sso_mode": "required"}
			transport := &membershipDeadlineTransport{RoundTripper: s.Client().Transport}
			r := &organizationMembershipResource{client: &Client{
				organization: "acme", restURL: s.URL, http: &http.Client{Transport: transport},
				timeouts: timeouts.Value{Object: types.ObjectValueMust(attributeTypes, attributeValues)},
			}}
			model := organizationMembershipResourceModel{
				ID: types.StringValue(membershipUserUUID), UUID: types.StringValue(membershipUserUUID),
				Email: types.StringValue("jane@example.com"), Role: types.StringValue("ADMIN"), SSOMode: types.StringValue("REQUIRED"),
			}
			if operation == "read" {
				// Pending refresh reads both the invitation and members collection.
				api.member = nil
				api.invitation = map[string]any{"id": "invite", "email": "jane@example.com", "state": "pending", "role": "member", "sso_mode": "required"}
				model.ID, model.InvitationID = types.StringValue("invite"), types.StringValue("invite")
				model.UUID = types.StringNull()
			}
			ctx := context.Background()
			var schemaResp frameworkresource.SchemaResponse
			r.Schema(ctx, frameworkresource.SchemaRequest{}, &schemaResp)
			state := tfsdk.State{Schema: schemaResp.Schema}
			if diags := state.Set(ctx, &model); diags.HasError() {
				t.Fatal(diags)
			}
			plan := tfsdk.Plan{Schema: schemaResp.Schema, Raw: state.Raw}
			var diagnostics diag.Diagnostics
			started := time.Now()
			switch operation {
			case "create":
				response := frameworkresource.CreateResponse{State: state}
				r.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &response)
				diagnostics = response.Diagnostics
			case "read":
				response := frameworkresource.ReadResponse{State: state}
				r.Read(ctx, frameworkresource.ReadRequest{State: state}, &response)
				diagnostics = response.Diagnostics
			case "update":
				response := frameworkresource.UpdateResponse{State: state}
				r.Update(ctx, frameworkresource.UpdateRequest{State: state, Plan: plan}, &response)
				diagnostics = response.Diagnostics
			case "delete":
				response := frameworkresource.DeleteResponse{State: state}
				r.Delete(ctx, frameworkresource.DeleteRequest{State: state}, &response)
				diagnostics = response.Diagnostics
			}
			finished := time.Now()
			if diagnostics.HasError() {
				t.Fatal(diagnostics)
			}
			if len(transport.deadlines) != 2 {
				t.Fatalf("expected two requests sharing an operation deadline, got %v", transport.deadlines)
			}
			for _, deadline := range transport.deadlines {
				if deadline.Before(started.Add(budget)) || deadline.After(finished.Add(budget)) {
					t.Errorf("request deadline %s does not use the %s budget of %s", deadline, operation, budget)
				}
				if !deadline.Equal(transport.deadlines[0]) {
					t.Error("each request received a fresh deadline instead of sharing the operation budget")
				}
			}
		})
	}
}
