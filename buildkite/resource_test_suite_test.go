package buildkite

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
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
	_suite, err := getTestSuite(context.Background(), genqlientGraphql, id, 1)
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

// The REST PATCH applies name, default_branch, emoji, application_name, color, oidc_policy and the
// slug they derive from, and two team mutations run after it that can each fail. Update has to
// record the PATCH either way. Dropping it is not merely a re-plan: state goes on naming the slug
// the suite answered to before the rename, and that is the slug the URL above is built from, so
// under -refresh=false the next update PATCHes a suite that is no longer there.
func TestTestSuiteUpdatePersistsThePatchWhenATeamStepFails(t *testing.T) {
	t.Parallel()

	const (
		priorName      = "original-suite"
		updatedName    = "renamed-suite"
		priorSlug      = "original-suite"
		updatedSlug    = "renamed-suite"
		previousTeamID = "previous-team-id"
		newTeamID      = "new-team-id"
	)

	patched := stubResponse{status: http.StatusOK, body: fmt.Sprintf(`{
		"id": "suite-uuid", "graphql_id": "suite-id", "api_token": "token",
		"name": %q, "slug": %q, "default_branch": "main"
	}`, updatedName, updatedSlug)}
	teamAttached := stubResponse{status: http.StatusOK, body: fmt.Sprintf(`{"data":{"teamSuiteCreate":{
		"suite": {"teams": {"edges": [{"node": {"id": "team-suite-id", "uuid": "team-suite-uuid", "team": {"id": %q}}}]}},
		"teamSuite": {"id": "team-suite-id", "teamSuiteUuid": "team-suite-uuid", "accessLevel": "MANAGE_AND_READ",
			"team": {"id": %q}, "suite": {"id": "suite-id"}}
	}}}`, previousTeamID, newTeamID)}
	graphQLFails := stubResponse{status: http.StatusOK, body: `{"errors":[{"message":"team mutation exploded"}]}`}

	tests := []struct {
		name      string
		responses []stubResponse
		wantError string
		// The owner the next plan has to work from. Both teams own the suite once the attach has
		// applied, so the previous one is what leaves a diff to retry the delete from.
		wantOwner string
	}{
		{
			name:      "attaching the new owner team fails",
			responses: []stubResponse{patched, graphQLFails},
			wantError: "Could not add new owner team",
			wantOwner: previousTeamID,
		},
		{
			name:      "detaching the previous owner team fails",
			responses: []stubResponse{patched, teamAttached, graphQLFails},
			wantError: "Failed to delete team owner",
			wantOwner: previousTeamID,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server, requests := newRetryStub(t, testCase.responses...)
			defer server.Close()

			ts := &testSuiteResource{client: newRetryTestClient(t, server.URL, 0, time.Millisecond)}

			ctx := t.Context()
			var schemaResp fwresource.SchemaResponse
			ts.Schema(ctx, fwresource.SchemaRequest{}, &schemaResp)
			if schemaResp.Diagnostics.HasError() {
				t.Fatalf("Schema() diagnostics = %v", schemaResp.Diagnostics)
			}
			schema := schemaResp.Schema

			shared := map[string]tftypes.Value{
				"id":             tftypes.NewValue(tftypes.String, "suite-id"),
				"uuid":           tftypes.NewValue(tftypes.String, "suite-uuid"),
				"default_branch": tftypes.NewValue(tftypes.String, "main"),
			}
			prior := map[string]tftypes.Value{
				"name":          tftypes.NewValue(tftypes.String, priorName),
				"slug":          tftypes.NewValue(tftypes.String, priorSlug),
				"team_owner_id": tftypes.NewValue(tftypes.String, previousTeamID),
			}
			planned := map[string]tftypes.Value{
				"name":          tftypes.NewValue(tftypes.String, updatedName),
				"slug":          tftypes.NewValue(tftypes.String, updatedSlug),
				"team_owner_id": tftypes.NewValue(tftypes.String, newTeamID),
			}
			maps.Copy(prior, shared)
			maps.Copy(planned, shared)

			priorRaw := nullObjectWith(ctx, t, schema.Type(), prior)
			req := fwresource.UpdateRequest{
				Plan:   tfsdk.Plan{Schema: schema, Raw: nullObjectWith(ctx, t, schema.Type(), planned)},
				State:  tfsdk.State{Schema: schema, Raw: priorRaw},
				Config: tfsdk.Config{Schema: schema, Raw: nullObjectWith(ctx, t, schema.Type(), planned)},
			}
			resp := fwresource.UpdateResponse{State: tfsdk.State{Schema: schema, Raw: priorRaw}}

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
			if got := persisted.Name.ValueString(); got != updatedName {
				t.Errorf("Persisted name = %q, want %q: the PATCH applied, so dropping it leaves Terraform planning the same change again", got, updatedName)
			}
			if got := persisted.Slug.ValueString(); got != updatedSlug {
				t.Errorf("Persisted slug = %q, want %q: the next update builds its URL from this", got, updatedSlug)
			}
			if got := persisted.TeamOwnerId.ValueString(); got != testCase.wantOwner {
				t.Errorf("Persisted team_owner_id = %q, want %q", got, testCase.wantOwner)
			}
		})
	}
}

// The mirror of the pipeline case. When the delete fails, Update keeps the previous owner in state
// so the next plan still shows a diff. That next apply re-attaches the new owner, which already
// owns the suite from the run before, and the attach response is then empty, so the previous
// owner's edge has to be read back or the delete this apply exists to retry is silently skipped
// and both teams keep owning the suite.
func TestTestSuiteUpdateRetriesTheDeleteWhenTheNewOwnerAlreadyOwnsTheSuite(t *testing.T) {
	t.Parallel()

	const (
		previousTeamID = "previous-team-id"
		newTeamID      = "new-team-id"
		previousEdgeID = "previous-team-suite-id"
	)

	server, requests, bodies := newRecordingRetryStub(t,
		// The PATCH applies.
		stubResponse{status: http.StatusOK, body: `{
			"id": "suite-uuid", "graphql_id": "suite-id", "api_token": "token",
			"name": "renamed-suite", "slug": "renamed-suite", "default_branch": "main"
		}`},
		// The new owner was attached by the previous, partly failed apply.
		stubResponse{status: http.StatusOK, body: `{"errors":[{"message":"This team has already been added to this suite"}]}`},
		// getTestSuite, reached only if the attach stopped being fatal. Both teams own the suite.
		stubResponse{status: http.StatusOK, body: fmt.Sprintf(`{"data":{"suite":{
			"__typename": "Suite",
			"id": "suite-id", "uuid": "suite-uuid", "name": "renamed-suite", "slug": "renamed-suite",
			"defaultBranch": "main", "emoji": null,
			"teams": {"edges": [
				{"node": {"id": %q, "accessLevel": "MANAGE_AND_READ", "team": {"id": %q}}},
				{"node": {"id": "new-team-suite-id", "accessLevel": "MANAGE_AND_READ", "team": {"id": %q}}}
			]}
		}}}`, previousEdgeID, previousTeamID, newTeamID)},
		// deleteTestSuiteTeam for the previous owner.
		stubResponse{status: http.StatusOK, body: `{"data":{"teamSuiteDelete":{"deletedTeamSuiteID":"previous-team-suite-id","clientMutationId":""}}}`},
	)
	defer server.Close()

	ts := &testSuiteResource{client: newRetryTestClient(t, server.URL, 0, time.Millisecond)}

	ctx := t.Context()
	var schemaResp fwresource.SchemaResponse
	ts.Schema(ctx, fwresource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics = %v", schemaResp.Diagnostics)
	}
	schema := schemaResp.Schema

	shared := map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "suite-id"),
		"uuid":           tftypes.NewValue(tftypes.String, "suite-uuid"),
		"default_branch": tftypes.NewValue(tftypes.String, "main"),
	}
	prior := map[string]tftypes.Value{
		"name":          tftypes.NewValue(tftypes.String, "original-suite"),
		"slug":          tftypes.NewValue(tftypes.String, "original-suite"),
		"team_owner_id": tftypes.NewValue(tftypes.String, previousTeamID),
	}
	planned := map[string]tftypes.Value{
		"name":          tftypes.NewValue(tftypes.String, "renamed-suite"),
		"slug":          tftypes.NewValue(tftypes.String, "renamed-suite"),
		"team_owner_id": tftypes.NewValue(tftypes.String, newTeamID),
	}
	maps.Copy(prior, shared)
	maps.Copy(planned, shared)

	priorRaw := nullObjectWith(ctx, t, schema.Type(), prior)
	req := fwresource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: schema, Raw: nullObjectWith(ctx, t, schema.Type(), planned)},
		State:  tfsdk.State{Schema: schema, Raw: priorRaw},
		Config: tfsdk.Config{Schema: schema, Raw: nullObjectWith(ctx, t, schema.Type(), planned)},
	}
	resp := fwresource.UpdateResponse{State: tfsdk.State{Schema: schema, Raw: priorRaw}}

	ts.Update(ctx, req, &resp)

	if diagnosticsContain(resp.Diagnostics, "Could not add new owner team") {
		t.Fatalf("Update() failed on an attach that had already applied, so the delete it exists to retry was never reached: %v", resp.Diagnostics)
	}
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update() diagnostics = %v, want the swap to have completed", resp.Diagnostics)
	}
	if got := requests.Load(); got < 4 {
		t.Fatalf("Made %d requests, want the previous owner to have been read back and deleted; bodies were %v", got, bodies())
	}

	var deleted bool
	for _, body := range bodies() {
		if strings.Contains(body, "teamSuiteDelete") && strings.Contains(body, previousEdgeID) {
			deleted = true
		}
	}
	if !deleted {
		t.Errorf("No teamSuiteDelete for the previous owner edge %q; bodies were %v", previousEdgeID, bodies())
	}

	var persisted testSuiteModel
	if diags := resp.State.Get(ctx, &persisted); diags.HasError() {
		t.Fatalf("Reading the persisted state = %v", diags)
	}
	if got := persisted.TeamOwnerId.ValueString(); got != newTeamID {
		t.Errorf("Persisted team_owner_id = %q, want %q: the delete succeeded, so the swap is complete", got, newTeamID)
	}
}
