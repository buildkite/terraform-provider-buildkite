package buildkite

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccBuildkiteTestSuiteResource(t *testing.T) {
	basicTestSuite := func(name string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_team" "team" {
			name = "test suite team %s"
			default_team = false
			privacy = "VISIBLE"
			default_member_role = "MAINTAINER"
		}
		resource "buildkite_test_suite" "suite" {
			name = "test suite %s"
			default_branch = "main"
			team_owner_id = resource.buildkite_team.team.id
		}
		`, name, name)
	}

	basicTestSuiteWithEmoji := func(name string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_team" "team" {
			name = "test suite team %s"
			default_team = false
			privacy = "VISIBLE"
			default_member_role = "MAINTAINER"
		}
		resource "buildkite_test_suite" "suite" {
			name = "test suite %s"
			default_branch = "main"
			emoji = ":buildkite:"
			team_owner_id = resource.buildkite_team.team.id
		}
		`, name, name)
	}

	testSuiteWithAllAttributes := func(name, applicationName, color, oidcPolicy string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_team" "team" {
			name = "test suite team %s"
			default_team = false
			privacy = "VISIBLE"
			default_member_role = "MAINTAINER"
		}
		resource "buildkite_test_suite" "suite" {
			name = "test suite %s"
			default_branch = "main"
			emoji = ":buildkite:"
			application_name = "%s"
			color = "%s"
			oidc_policy = %q
			team_owner_id = resource.buildkite_team.team.id
		}
		`, name, name, applicationName, color, oidcPolicy)
	}

	testSuiteWithTwoTeams := func(name string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_team" "ateam" {
			name = "a team %s-a"
			default_team = false
			privacy = "VISIBLE"
			default_member_role = "MAINTAINER"
		}
		resource "buildkite_team" "bteam" {
			name = "b team %s-b"
			default_team = false
			privacy = "VISIBLE"
			default_member_role = "MAINTAINER"
		}
		resource "buildkite_test_suite" "suite" {
			name = "test suite update %s"
			default_branch = "main"
			team_owner_id = resource.buildkite_team.bteam.id
		}
		`, name, name, name)
	}

	testSuiteTeamAddition := func(name string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_team" "ateam" {
			name = "a team %s-a"
			default_team = false
			privacy = "VISIBLE"
			default_member_role = "MAINTAINER"
		}
		resource "buildkite_team" "bteam" {
			name = "b team %s-b"
			default_team = false
			privacy = "VISIBLE"
			default_member_role = "MAINTAINER"
		}
		resource "buildkite_test_suite" "suite" {
			name = "test suite update %s"
			default_branch = "main"
			team_owner_id = resource.buildkite_team.bteam.id
		}
		resource "buildkite_test_suite_team" "team-a" {
			test_suite_id = buildkite_test_suite.suite.id
			team_id = buildkite_team.ateam.id
			access_level = "MANAGE_AND_READ"
		}
		`, name, name, name)
	}

	t.Run("creates a test suite", func(t *testing.T) {
		var suite getTestSuiteSuite
		randName := acctest.RandString(10)

		check := resource.ComposeAggregateTestCheckFunc(
			checkTestSuiteExists("buildkite_test_suite.suite", &suite),
			checkTestSuiteRemoteValue(&suite, "Name", fmt.Sprintf("test suite %s", randName)),
			checkTestSuiteRemoteValue(&suite, "DefaultBranch", "main"),
			resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "id"),
			resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "api_token"),
			resource.TestCheckResourceAttr("buildkite_test_suite.suite", "default_branch", "main"),
			resource.TestCheckResourceAttr("buildkite_test_suite.suite", "name", fmt.Sprintf("test suite %s", randName)),
			resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "team_owner_id"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testTestSuiteDestroy,
			Steps: []resource.TestStep{
				{
					Config: basicTestSuite(randName),
					Check:  check,
				},
			},
		})
	})

	t.Run("creates a test suite with an emoji set", func(t *testing.T) {
		var suite getTestSuiteSuite
		randName := acctest.RandString(10)

		check := resource.ComposeAggregateTestCheckFunc(
			checkTestSuiteExists("buildkite_test_suite.suite", &suite),
			checkTestSuiteRemoteValue(&suite, "Name", fmt.Sprintf("test suite %s", randName)),
			checkTestSuiteRemoteValue(&suite, "DefaultBranch", "main"),
			resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "id"),
			resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "api_token"),
			resource.TestCheckResourceAttr("buildkite_test_suite.suite", "default_branch", "main"),
			resource.TestCheckResourceAttr("buildkite_test_suite.suite", "emoji", ":buildkite:"),
			resource.TestCheckResourceAttr("buildkite_test_suite.suite", "name", fmt.Sprintf("test suite %s", randName)),
			resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "team_owner_id"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testTestSuiteDestroy,
			Steps: []resource.TestStep{
				{
					Config: basicTestSuiteWithEmoji(randName),
					Check:  check,
				},
			},
		})
	})

	t.Run("creates and updates a test suite with all attributes set", func(t *testing.T) {
		var suite getTestSuiteSuite
		randName := acctest.RandString(10)

		oidcPolicy := "- iss: https://agent.buildkite.com\n  claims:\n    organization_slug: my-org\n    pipeline_slug: my-pipeline\n  scopes:\n    - write_uploads\n"
		updatedOidcPolicy := "- iss: https://agent.buildkite.com\n  claims:\n    organization_slug: my-org\n    pipeline_slug: another-pipeline\n  scopes:\n    - read_suites\n    - write_uploads\n"

		check := func(applicationName, color, oidcPolicy string) resource.TestCheckFunc {
			return resource.ComposeAggregateTestCheckFunc(
				checkTestSuiteExists("buildkite_test_suite.suite", &suite),
				checkTestSuiteRemoteValue(&suite, "Name", fmt.Sprintf("test suite %s", randName)),
				checkTestSuiteRemoteValue(&suite, "DefaultBranch", "main"),
				resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "id"),
				resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "api_token"),
				resource.TestCheckResourceAttr("buildkite_test_suite.suite", "default_branch", "main"),
				resource.TestCheckResourceAttr("buildkite_test_suite.suite", "emoji", ":buildkite:"),
				resource.TestCheckResourceAttr("buildkite_test_suite.suite", "application_name", applicationName),
				resource.TestCheckResourceAttr("buildkite_test_suite.suite", "color", color),
				resource.TestCheckResourceAttr("buildkite_test_suite.suite", "oidc_policy", oidcPolicy),
				resource.TestCheckResourceAttr("buildkite_test_suite.suite", "name", fmt.Sprintf("test suite %s", randName)),
				resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "team_owner_id"),
			)
		}

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testTestSuiteDestroy,
			Steps: []resource.TestStep{
				{
					Config: testSuiteWithAllAttributes(randName, "My App", "#BADA55", oidcPolicy),
					Check:  check("My App", "#BADA55", oidcPolicy),
				},
				{
					Config: testSuiteWithAllAttributes(randName, "My Updated App", "#B2ECF7", updatedOidcPolicy),
					Check:  check("My Updated App", "#B2ECF7", updatedOidcPolicy),
				},
				{
					// removing the optional+computed attributes from config
					// leaves the server values unmanaged and untouched
					Config: basicTestSuite(randName),
					Check: resource.ComposeAggregateTestCheckFunc(
						checkTestSuiteExists("buildkite_test_suite.suite", &suite),
						resource.TestCheckResourceAttr("buildkite_test_suite.suite", "application_name", "My Updated App"),
						resource.TestCheckResourceAttr("buildkite_test_suite.suite", "color", "#B2ECF7"),
						resource.TestCheckResourceAttr("buildkite_test_suite.suite", "oidc_policy", updatedOidcPolicy),
					),
				},
				{
					// explicitly empty attributes clear the server values
					Config: testSuiteWithAllAttributes(randName, "", "", ""),
					Check: resource.ComposeAggregateTestCheckFunc(
						checkTestSuiteExists("buildkite_test_suite.suite", &suite),
						resource.TestCheckResourceAttr("buildkite_test_suite.suite", "application_name", ""),
						resource.TestCheckResourceAttr("buildkite_test_suite.suite", "color", ""),
						resource.TestCheckResourceAttr("buildkite_test_suite.suite", "oidc_policy", ""),
					),
				},
			},
		})
	})

	t.Run("updates a test suite", func(t *testing.T) {
		var suite getTestSuiteSuite
		randName := acctest.RandString(10)

		check := resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "id"),
			resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "api_token"),
			resource.TestCheckResourceAttr("buildkite_test_suite.suite", "default_branch", "main"),
			resource.TestCheckResourceAttr("buildkite_test_suite.suite", "name", fmt.Sprintf("test suite %s", randName)),
			resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "team_owner_id"),
			checkTestSuiteExists("buildkite_test_suite.suite", &suite),
			checkTestSuiteRemoteValue(&suite, "Name", fmt.Sprintf("test suite %s", randName)),
			checkTestSuiteRemoteValue(&suite, "DefaultBranch", "main"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testTestSuiteDestroy,
			Steps: []resource.TestStep{
				{
					Config: basicTestSuite(randName),
					Check:  check,
				},
				{
					Config: basicTestSuite(randName),
					Taint:  []string{"buildkite_team.team"},
					Check:  check,
				},
			},
		})
	})

	t.Run("creates and handles test suite team owner resolution", func(t *testing.T) {
		var suite getTestSuiteSuite
		randName := acctest.RandString(10)

		check := resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "id"),
			resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "api_token"),
			resource.TestCheckResourceAttr("buildkite_test_suite.suite", "default_branch", "main"),
			resource.TestCheckResourceAttr("buildkite_test_suite.suite", "name", fmt.Sprintf("test suite update %s", randName)),
			resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "team_owner_id"),
			resource.TestCheckResourceAttrPair("buildkite_test_suite.suite", "team_owner_id", "buildkite_team.bteam", "id"),
			checkTestSuiteExists("buildkite_test_suite.suite", &suite),
			checkTestSuiteRemoteValue(&suite, "Name", fmt.Sprintf("test suite update %s", randName)),
			checkTestSuiteRemoteValue(&suite, "DefaultBranch", "main"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testTestSuiteDestroy,
			Steps: []resource.TestStep{
				{
					Config: testSuiteWithTwoTeams(randName),
					Check:  check,
				},
				{
					Config: testSuiteTeamAddition(randName),
					Check:  check,
				},
			},
		})
	})

	t.Run("import a test suite", func(t *testing.T) {
		var suite getTestSuiteSuite
		resName := acctest.RandString(10)

		check := resource.ComposeAggregateTestCheckFunc(
			checkTestSuiteExists("buildkite_test_suite.suite", &suite),
			resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "id"),
			resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "uuid"),
			resource.TestCheckResourceAttr("buildkite_test_suite.suite", "name", fmt.Sprintf("test suite %s", resName)),
			resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "slug"),
			resource.TestCheckResourceAttr("buildkite_test_suite.suite", "default_branch", "main"),
			resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "team_owner_id"),
			resource.TestCheckNoResourceAttr("buildkite_test_suite.suite", "emoji"),
			resource.TestCheckResourceAttrSet("buildkite_test_suite.suite", "api_token"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testTestSuiteDestroy,
			Steps: []resource.TestStep{
				{
					Config: basicTestSuite(resName),
					Check:  check,
				},
				{
					ResourceName:      "buildkite_test_suite.suite",
					ImportState:       true,
					ImportStateVerify: true,
				},
			},
		})
	})
}

func checkTestSuiteRemoteValue(suite *getTestSuiteSuite, property, value string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if obj := reflect.ValueOf(*suite).FieldByName(property).String(); obj != value {
			return fmt.Errorf("%s property on test suite does not match \"%s\" (\"%s\")", property, value, obj)
		}

		return nil
	}
}

func loadRemoteTestSuite(id string) *getTestSuiteSuite {
	_suite, err := getTestSuite(context.Background(), genqlientGraphql, id, 1, nil)
	if err != nil {
		return nil
	}
	if suite, ok := _suite.Suite.(*getTestSuiteSuite); ok {
		return suite
	}

	return nil
}

func checkTestSuiteExists(name string, suite *getTestSuiteSuite) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]
		if !ok {
			return errors.New("Test suite not found in state")
		}
		_suite := loadRemoteTestSuite(rs.Primary.Attributes["id"])

		if _suite == nil {
			return errors.New("Test suite does not exist on server")
		}

		suite.Id = _suite.Id
		suite.Uuid = _suite.Uuid
		suite.DefaultBranch = _suite.DefaultBranch
		suite.Name = _suite.Name
		suite.Slug = _suite.Slug
		suite.Teams = _suite.Teams

		return nil
	}
}

func testTestSuiteDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_test_suite" {
			continue
		}
	}
	return nil
}

// The attributes these tests drive Update with. The schema has a dozen more, and nullObjectWith
// leaves those null, which keeps each case to the attributes that actually steer the code path.
func testSuiteAttrs(name, slug, teamOwnerID string) map[string]tftypes.Value {
	return map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "suite-id"),
		"uuid":           tftypes.NewValue(tftypes.String, "suite-uuid"),
		"default_branch": tftypes.NewValue(tftypes.String, "main"),
		"name":           tftypes.NewValue(tftypes.String, name),
		"slug":           tftypes.NewValue(tftypes.String, slug),
		"team_owner_id":  tftypes.NewValue(tftypes.String, teamOwnerID),
	}
}

const (
	priorSuiteName    = "original-suite"
	updatedSuiteName  = "renamed-suite"
	previousSuiteTeam = "previous-team-id"
	newSuiteTeam      = "new-team-id"
	previousSuiteEdge = "previous-team-suite-id"
	newSuiteEdge      = "new-team-suite-id"
)

// suitePatched is the REST PATCH answering with the renamed suite.
var suitePatched = stubResponse{status: http.StatusOK, body: fmt.Sprintf(`{
	"id": "suite-uuid", "graphql_id": "suite-id", "api_token": "token",
	"name": %q, "slug": %q, "default_branch": "main"
}`, updatedSuiteName, updatedSuiteName)}

var (
	suiteTeamAttached = stubResponse{status: http.StatusOK, body: fmt.Sprintf(`{"data":{"teamSuiteCreate":{
		"teamSuite": {"id": %q, "teamSuiteUuid": "new-team-suite-uuid", "accessLevel": "MANAGE_AND_READ",
			"team": {"id": %q}, "suite": {"id": "suite-id"}}
	}}}`, newSuiteEdge, newSuiteTeam)}
	suiteTeamAlreadyAttached = stubResponse{status: http.StatusOK, body: `{"errors":[{"message":"This suite has already been added to this team"}]}`}
	suiteTeamDetached        = stubResponse{status: http.StatusOK, body: fmt.Sprintf(`{"data":{"teamSuiteDelete":{"deletedTeamSuiteID":%q,"clientMutationId":""}}}`, previousSuiteEdge)}
	graphQLFails             = stubResponse{status: http.StatusOK, body: `{"errors":[{"message":"the mutation exploded"}]}`}
)

// suiteTeamEdge is one edge of the teams connection getTestSuite answers with.
func suiteTeamEdge(edgeID, teamID, accessLevel string) string {
	return fmt.Sprintf(`{"node": {"id": %q, "accessLevel": %q, "team": {"id": %q}}}`, edgeID, accessLevel, teamID)
}

// suiteWithTeams is a getTestSuite response listing edges, optionally reporting a further page.
func suiteWithTeams(nextCursor string, edges ...string) stubResponse {
	pageInfo := `{"hasNextPage": false, "endCursor": ""}`
	if nextCursor != "" {
		pageInfo = fmt.Sprintf(`{"hasNextPage": true, "endCursor": %q}`, nextCursor)
	}

	return stubResponse{status: http.StatusOK, body: fmt.Sprintf(`{"data":{"suite":{
		"__typename": "Suite",
		"id": "suite-id", "uuid": "suite-uuid", "name": %q, "slug": %q,
		"defaultBranch": "main", "emoji": null,
		"teams": {"pageInfo": %s, "edges": [%s]}
	}}}`, updatedSuiteName, updatedSuiteName, pageInfo, strings.Join(edges, ","))}
}

// bothTeamsOwn is the suite mid-swap: the new owner attached, the previous one not yet detached.
func bothTeamsOwn() stubResponse {
	return suiteWithTeams("",
		suiteTeamEdge(previousSuiteEdge, previousSuiteTeam, "MANAGE_AND_READ"),
		suiteTeamEdge(newSuiteEdge, newSuiteTeam, "MANAGE_AND_READ"),
	)
}

// The REST PATCH applies name, default_branch, emoji, application_name, color, oidc_policy and the
// slug they derive from, and the team swap that runs after it can fail at any of three steps.
// Update has to record the PATCH either way. Dropping it is not merely a re-plan: state goes on
// naming the slug the suite answered to before the rename, and that is the slug the PATCH URL is
// built from, so under -refresh=false the next update PATCHes a suite that is no longer there.
func TestTestSuiteUpdatePersistsThePatchWhenATeamStepFails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		responses []stubResponse
		wantError string
	}{
		{
			name:      "attaching the new owner team fails",
			responses: []stubResponse{suitePatched, graphQLFails},
			wantError: "Could not add new owner team",
		},
		{
			name:      "reading the suite's teams fails",
			responses: []stubResponse{suitePatched, suiteTeamAttached, graphQLFails},
			wantError: "Could not load the suite's owner teams",
		},
		{
			name:      "detaching the previous owner team fails",
			responses: []stubResponse{suitePatched, suiteTeamAttached, bothTeamsOwn(), graphQLFails},
			wantError: "Failed to delete team owner",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server, requests := newRetryStub(t, testCase.responses...)
			defer server.Close()

			ts := &testSuiteResource{client: newRetryTestClient(t, server.URL, 0, time.Millisecond)}
			ctx := t.Context()
			schema := resourceSchema(ctx, t, ts)

			req, resp := updateRequestFor(ctx, t, schema,
				testSuiteAttrs(priorSuiteName, priorSuiteName, previousSuiteTeam),
				testSuiteAttrs(updatedSuiteName, updatedSuiteName, newSuiteTeam))
			ts.Update(ctx, req, &resp)

			if got := requests.Load(); got < int64(len(testCase.responses)) {
				t.Fatalf("Made %d requests, want %d: the PATCH has to have applied before the failure", got, len(testCase.responses))
			}
			if !diagnosticsContain(resp.Diagnostics, testCase.wantError) {
				t.Fatalf("Update() diagnostics = %v, want %q", resp.Diagnostics, testCase.wantError)
			}

			var persisted testSuiteModel
			if diags := resp.State.Get(ctx, &persisted); diags.HasError() {
				t.Fatalf("Reading the persisted state = %v", diags)
			}
			if got := persisted.Name.ValueString(); got != updatedSuiteName {
				t.Errorf("Persisted name = %q, want %q: the PATCH applied, so dropping it leaves Terraform planning the same change again", got, updatedSuiteName)
			}
			if got := persisted.Slug.ValueString(); got != updatedSuiteName {
				t.Errorf("Persisted slug = %q, want %q: the next update builds its URL from this", got, updatedSuiteName)
			}
			// Both teams own the suite once the attach has applied, so keeping the previous one is
			// what leaves a diff for the next plan to retry the detach from.
			if got := persisted.TeamOwnerId.ValueString(); got != previousSuiteTeam {
				t.Errorf("Persisted team_owner_id = %q, want %q", got, previousSuiteTeam)
			}
		})
	}
}

// The mirror of the case above: nothing applied, so nothing may be recorded. The defer that
// persists the PATCH is registered after it, and hoisting it above would start reporting a rename
// that never happened.
func TestTestSuiteUpdateKeepsPriorStateWhenThePatchFails(t *testing.T) {
	t.Parallel()

	server, _ := newRetryStub(t, stubResponse{status: http.StatusInternalServerError, body: `{"message":"boom"}`})
	defer server.Close()

	ts := &testSuiteResource{client: newRetryTestClient(t, server.URL, 0, time.Millisecond)}
	ctx := t.Context()
	schema := resourceSchema(ctx, t, ts)

	req, resp := updateRequestFor(ctx, t, schema,
		testSuiteAttrs(priorSuiteName, priorSuiteName, previousSuiteTeam),
		testSuiteAttrs(updatedSuiteName, updatedSuiteName, newSuiteTeam))
	ts.Update(ctx, req, &resp)

	if !diagnosticsContain(resp.Diagnostics, "Failed to update test suite") {
		t.Fatalf("Update() diagnostics = %v, want %q", resp.Diagnostics, "Failed to update test suite")
	}

	var persisted testSuiteModel
	if diags := resp.State.Get(ctx, &persisted); diags.HasError() {
		t.Fatalf("Reading the persisted state = %v", diags)
	}
	if got := persisted.Name.ValueString(); got != priorSuiteName {
		t.Errorf("Persisted name = %q, want %q: the PATCH never applied", got, priorSuiteName)
	}
	if got := persisted.Slug.ValueString(); got != priorSuiteName {
		t.Errorf("Persisted slug = %q, want %q: the PATCH never applied", got, priorSuiteName)
	}
	if got := persisted.TeamOwnerId.ValueString(); got != previousSuiteTeam {
		t.Errorf("Persisted team_owner_id = %q, want %q", got, previousSuiteTeam)
	}
}

// The common update: a rename with the owner left alone. The whole team block is skipped, so the
// PATCH is recorded by the defer and nothing else, and no GraphQL call is made at all.
func TestTestSuiteUpdateRecordsARenameWithNoOwnerChange(t *testing.T) {
	t.Parallel()

	server, requests, bodies := newRecordingRetryStub(t, suitePatched)
	defer server.Close()

	ts := &testSuiteResource{client: newRetryTestClient(t, server.URL, 0, time.Millisecond)}
	ctx := t.Context()
	schema := resourceSchema(ctx, t, ts)

	req, resp := updateRequestFor(ctx, t, schema,
		testSuiteAttrs(priorSuiteName, priorSuiteName, previousSuiteTeam),
		testSuiteAttrs(updatedSuiteName, updatedSuiteName, previousSuiteTeam))
	ts.Update(ctx, req, &resp)

	recorded := bodies()
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update() diagnostics = %v, want the rename to have applied", resp.Diagnostics)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("Made %d requests, want 1: only the PATCH; bodies were %v", got, recorded)
	}

	var persisted testSuiteModel
	if diags := resp.State.Get(ctx, &persisted); diags.HasError() {
		t.Fatalf("Reading the persisted state = %v", diags)
	}
	if got := persisted.Slug.ValueString(); got != updatedSuiteName {
		t.Errorf("Persisted slug = %q, want %q", got, updatedSuiteName)
	}
	if got := persisted.TeamOwnerId.ValueString(); got != previousSuiteTeam {
		t.Errorf("Persisted team_owner_id = %q, want %q", got, previousSuiteTeam)
	}
}

// The ordinary owner swap: attach the new owner, read the suite's teams, detach the previous one.
// Sending the wrong edge id to teamSuiteDelete detaches the wrong team, or nothing at all, and the
// swap looks successful either way.
func TestTestSuiteUpdateSwapsTheOwnerTeam(t *testing.T) {
	t.Parallel()

	server, requests, bodies := newRecordingRetryStub(t,
		suitePatched, suiteTeamAttached, bothTeamsOwn(), suiteTeamDetached,
	)
	defer server.Close()

	ts := &testSuiteResource{client: newRetryTestClient(t, server.URL, 0, time.Millisecond)}
	ctx := t.Context()
	schema := resourceSchema(ctx, t, ts)

	req, resp := updateRequestFor(ctx, t, schema,
		testSuiteAttrs(priorSuiteName, priorSuiteName, previousSuiteTeam),
		testSuiteAttrs(updatedSuiteName, updatedSuiteName, newSuiteTeam))
	ts.Update(ctx, req, &resp)

	recorded := bodies()
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update() diagnostics = %v, want the swap to have completed", resp.Diagnostics)
	}
	if got := requests.Load(); got != 4 {
		t.Fatalf("Made %d requests, want 4: the PATCH, the attach, the teams read and the detach; bodies were %v", got, recorded)
	}

	var deleted []string
	for _, body := range recorded {
		if strings.Contains(body, "teamSuiteDelete") {
			deleted = append(deleted, body)
		}
	}
	if len(deleted) != 1 {
		t.Fatalf("Made %d teamSuiteDelete requests, want 1; bodies were %v", len(deleted), recorded)
	}
	if !strings.Contains(deleted[0], previousSuiteEdge) {
		t.Errorf("teamSuiteDelete = %s\nwant the previous owner's edge %q", deleted[0], previousSuiteEdge)
	}
	if strings.Contains(deleted[0], newSuiteEdge) {
		t.Errorf("teamSuiteDelete = %s\nwant the new owner's edge %q left attached", deleted[0], newSuiteEdge)
	}

	var persisted testSuiteModel
	if diags := resp.State.Get(ctx, &persisted); diags.HasError() {
		t.Fatalf("Reading the persisted state = %v", diags)
	}
	if got := persisted.TeamOwnerId.ValueString(); got != newSuiteTeam {
		t.Errorf("Persisted team_owner_id = %q, want %q", got, newSuiteTeam)
	}
	if got := persisted.Slug.ValueString(); got != updatedSuiteName {
		t.Errorf("Persisted slug = %q, want %q: the next PATCH addresses the suite by it", got, updatedSuiteName)
	}
}

// The suite's teams are paged, and the previous owner is only on the second page. Stopping at the
// first would detach nothing, record the swap as done, and leave the previous team owning the suite
// with no diff left to correct it.
func TestTestSuiteUpdateFindsAPreviousOwnerPastTheFirstPage(t *testing.T) {
	t.Parallel()

	const cursor = "page-1-end"

	server, _, bodies := newRecordingRetryStub(t,
		suitePatched,
		suiteTeamAttached,
		suiteWithTeams(cursor, suiteTeamEdge(newSuiteEdge, newSuiteTeam, "MANAGE_AND_READ")),
		suiteWithTeams("", suiteTeamEdge(previousSuiteEdge, previousSuiteTeam, "MANAGE_AND_READ")),
		suiteTeamDetached,
	)
	defer server.Close()

	ts := &testSuiteResource{client: newRetryTestClient(t, server.URL, 0, time.Millisecond)}
	ctx := t.Context()
	schema := resourceSchema(ctx, t, ts)

	req, resp := updateRequestFor(ctx, t, schema,
		testSuiteAttrs(priorSuiteName, priorSuiteName, previousSuiteTeam),
		testSuiteAttrs(updatedSuiteName, updatedSuiteName, newSuiteTeam))
	ts.Update(ctx, req, &resp)

	recorded := bodies()
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update() diagnostics = %v, want the swap to have completed", resp.Diagnostics)
	}

	var paged, deleted bool
	for _, body := range recorded {
		if strings.Contains(body, "getTestSuite") && strings.Contains(body, cursor) {
			paged = true
		}
		if strings.Contains(body, "teamSuiteDelete") && strings.Contains(body, previousSuiteEdge) {
			deleted = true
		}
	}
	if !paged {
		t.Errorf("No getTestSuite carried the cursor %q, so the second page was never read; bodies were %v", cursor, recorded)
	}
	if !deleted {
		t.Errorf("No teamSuiteDelete for the previous owner edge %q; bodies were %v", previousSuiteEdge, recorded)
	}

	var persisted testSuiteModel
	if diags := resp.State.Get(ctx, &persisted); diags.HasError() {
		t.Fatalf("Reading the persisted state = %v", diags)
	}
	if got := persisted.TeamOwnerId.ValueString(); got != newSuiteTeam {
		t.Errorf("Persisted team_owner_id = %q, want %q", got, newSuiteTeam)
	}
}

// When the delete fails, Update keeps the previous owner in state so the next plan still shows a
// diff. That next apply re-attaches the new owner, which already owns the suite from the run
// before, so the attach errors and the delete this apply exists to retry has to still be reached.
func TestTestSuiteUpdateRetriesTheDeleteWhenTheNewOwnerAlreadyOwnsTheSuite(t *testing.T) {
	t.Parallel()

	server, _, bodies := newRecordingRetryStub(t,
		suitePatched, suiteTeamAlreadyAttached, bothTeamsOwn(), suiteTeamDetached,
	)
	defer server.Close()

	ts := &testSuiteResource{client: newRetryTestClient(t, server.URL, 0, time.Millisecond)}
	ctx := t.Context()
	schema := resourceSchema(ctx, t, ts)

	req, resp := updateRequestFor(ctx, t, schema,
		testSuiteAttrs(priorSuiteName, priorSuiteName, previousSuiteTeam),
		testSuiteAttrs(updatedSuiteName, updatedSuiteName, newSuiteTeam))
	ts.Update(ctx, req, &resp)

	recorded := bodies()
	if diagnosticsContain(resp.Diagnostics, "Could not add new owner team") {
		t.Fatalf("Update() failed on an attach that had already applied, so the delete it exists to retry was never reached: %v", resp.Diagnostics)
	}
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update() diagnostics = %v, want the swap to have completed", resp.Diagnostics)
	}

	var deleted bool
	for _, body := range recorded {
		if strings.Contains(body, "teamSuiteDelete") && strings.Contains(body, previousSuiteEdge) {
			deleted = true
		}
	}
	if !deleted {
		t.Errorf("No teamSuiteDelete for the previous owner edge %q; bodies were %v", previousSuiteEdge, recorded)
	}

	var persisted testSuiteModel
	if diags := resp.State.Get(ctx, &persisted); diags.HasError() {
		t.Fatalf("Reading the persisted state = %v", diags)
	}
	if got := persisted.TeamOwnerId.ValueString(); got != newSuiteTeam {
		t.Errorf("Persisted team_owner_id = %q, want %q: the delete succeeded, so the swap is complete", got, newSuiteTeam)
	}
}

// Tolerating the already-added attach means no mutation ran to prove the new owner can manage the
// suite, and the message it was tolerated on is only matched by regex. So the suite is asked before
// the previous owner is detached. Without that check the suite ends up with an owner that cannot
// manage it, or with none at all, and Read surfaces neither: it matches the team named in state and
// never looks at the access level.
func TestTestSuiteUpdateChecksTheToleratedAttachBeforeDetaching(t *testing.T) {
	t.Parallel()

	promoted := stubResponse{status: http.StatusOK, body: fmt.Sprintf(`{"data":{"teamSuiteUpdate":{"teamSuite":{
		"id": %q, "uuid": "new-team-suite-uuid", "accessLevel": "MANAGE_AND_READ"}}}}`, newSuiteEdge)}

	tests := []struct {
		name          string
		responses     []stubResponse
		wantError     string
		wantPromotion bool
		wantDetach    bool
		// The owner the next plan has to work from.
		wantOwner string
	}{
		{
			// The new owner is linked, but only at READ_ONLY, which is not ownership. Recording it
			// as owner without raising it leaves the suite with nobody who can manage it.
			name: "the new owner team is linked below manage access",
			responses: []stubResponse{suitePatched, suiteTeamAlreadyAttached, suiteWithTeams("",
				suiteTeamEdge(previousSuiteEdge, previousSuiteTeam, "MANAGE_AND_READ"),
				suiteTeamEdge(newSuiteEdge, newSuiteTeam, "READ_ONLY"),
			), promoted, suiteTeamDetached},
			wantPromotion: true,
			wantDetach:    true,
			wantOwner:     newSuiteTeam,
		},
		{
			// The message matched but the new owner is not linked at all, so it was never about a
			// previous run's attach. Detaching here strips the suite's only owner, and Read then
			// fails outright with "Could not find owning team".
			name: "the new owner team does not own the suite",
			responses: []stubResponse{suitePatched, suiteTeamAlreadyAttached, suiteWithTeams("",
				suiteTeamEdge(previousSuiteEdge, previousSuiteTeam, "MANAGE_AND_READ"),
			)},
			wantError:  "Could not add new owner team",
			wantDetach: false,
			wantOwner:  previousSuiteTeam,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server, _, bodies := newRecordingRetryStub(t, testCase.responses...)
			defer server.Close()

			ts := &testSuiteResource{client: newRetryTestClient(t, server.URL, 0, time.Millisecond)}
			ctx := t.Context()
			schema := resourceSchema(ctx, t, ts)

			req, resp := updateRequestFor(ctx, t, schema,
				testSuiteAttrs(priorSuiteName, priorSuiteName, previousSuiteTeam),
				testSuiteAttrs(updatedSuiteName, updatedSuiteName, newSuiteTeam))
			ts.Update(ctx, req, &resp)

			recorded := bodies()
			if testCase.wantError == "" {
				if resp.Diagnostics.HasError() {
					t.Fatalf("Update() diagnostics = %v, want the swap to have completed", resp.Diagnostics)
				}
			} else if !diagnosticsContain(resp.Diagnostics, testCase.wantError) {
				t.Fatalf("Update() diagnostics = %v, want %q; bodies were %v", resp.Diagnostics, testCase.wantError, recorded)
			}

			var promotedTeam, detached bool
			for _, body := range recorded {
				if strings.Contains(body, "teamSuiteUpdate") && strings.Contains(body, newSuiteEdge) && strings.Contains(body, "MANAGE_AND_READ") {
					promotedTeam = true
				}
				if strings.Contains(body, "teamSuiteDelete") {
					detached = true
				}
			}
			if promotedTeam != testCase.wantPromotion {
				t.Errorf("Promoted the new owner to MANAGE_AND_READ = %v, want %v; bodies were %v", promotedTeam, testCase.wantPromotion, recorded)
			}
			if detached != testCase.wantDetach {
				t.Errorf("Detached the previous owner = %v, want %v; bodies were %v", detached, testCase.wantDetach, recorded)
			}

			var persisted testSuiteModel
			if diags := resp.State.Get(ctx, &persisted); diags.HasError() {
				t.Fatalf("Reading the persisted state = %v", diags)
			}
			if got := persisted.TeamOwnerId.ValueString(); got != testCase.wantOwner {
				t.Errorf("Persisted team_owner_id = %q, want %q", got, testCase.wantOwner)
			}
			if got := persisted.Slug.ValueString(); got != updatedSuiteName {
				t.Errorf("Persisted slug = %q, want %q: the PATCH applied either way", got, updatedSuiteName)
			}
		})
	}
}

// The suite is deleted out of band between the PATCH and the read-back, so node(id:) answers with
// something that is not a Suite. The swap cannot proceed, but the PATCH still has to be recorded
// and the previous owner kept, so the next plan has somewhere to go.
func TestTestSuiteUpdateReportsASuiteThatDisappearsDuringTheSwap(t *testing.T) {
	t.Parallel()

	server, _, bodies := newRecordingRetryStub(t,
		suitePatched,
		suiteTeamAttached,
		stubResponse{status: http.StatusOK, body: `{"data":{"suite":null}}`},
	)
	defer server.Close()

	ts := &testSuiteResource{client: newRetryTestClient(t, server.URL, 0, time.Millisecond)}
	ctx := t.Context()
	schema := resourceSchema(ctx, t, ts)

	req, resp := updateRequestFor(ctx, t, schema,
		testSuiteAttrs(priorSuiteName, priorSuiteName, previousSuiteTeam),
		testSuiteAttrs(updatedSuiteName, updatedSuiteName, newSuiteTeam))
	ts.Update(ctx, req, &resp)

	recorded := bodies()
	if !diagnosticsContain(resp.Diagnostics, "Could not load the suite's owner teams") {
		t.Fatalf("Update() diagnostics = %v, want %q; bodies were %v", resp.Diagnostics, "Could not load the suite's owner teams", recorded)
	}
	for _, body := range recorded {
		if strings.Contains(body, "teamSuiteDelete") {
			t.Errorf("Detached a team without knowing the suite's owners: %s", body)
		}
	}

	var persisted testSuiteModel
	if diags := resp.State.Get(ctx, &persisted); diags.HasError() {
		t.Fatalf("Reading the persisted state = %v", diags)
	}
	if got := persisted.Slug.ValueString(); got != updatedSuiteName {
		t.Errorf("Persisted slug = %q, want %q: the PATCH applied", got, updatedSuiteName)
	}
	if got := persisted.TeamOwnerId.ValueString(); got != previousSuiteTeam {
		t.Errorf("Persisted team_owner_id = %q, want %q", got, previousSuiteTeam)
	}
}
