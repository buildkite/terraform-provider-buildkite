package buildkite

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const workloadIdentityResourceName = "buildkite_workload_identity.release"

// workloadIdentityRequestFields are the fields the API accepts. It rejects any other field with a 422,
// including the read-only fields it returns.
var workloadIdentityRequestFields = []string{
	"name", "description", "enabled", "oidc_issuer", "oidc_audience", "oidc_subject", "oidc_claims",
	"oidc_claim_conditions", "jwks", "grantable_token_scopes", "default_token_scopes", "max_token_ttl_seconds",
	"allowed_ip_addresses", "allowed_token_exchange_ip_addresses", "resource_grants",
}

// fakeWorkloadIdentityAPI serves the workload identity endpoints against identities held in memory. Like
// the API, an update only changes the fields its request carries, and claims come back re-serialized
// rather than as sent.
type fakeWorkloadIdentityAPI struct {
	t          *testing.T
	mu         sync.Mutex
	identities map[string]map[string]any
	created    int
}

func newFakeWorkloadIdentityAPI(t *testing.T) (*httptest.Server, *fakeWorkloadIdentityAPI) {
	t.Helper()

	api := &fakeWorkloadIdentityAPI{t: t, identities: map[string]map[string]any{}}
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)

	return server, api
}

func (a *fakeWorkloadIdentityAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const base = "/v2/organizations/acme/workload-identities"

	a.mu.Lock()
	defer a.mu.Unlock()

	if !strings.HasPrefix(r.URL.Path, base) {
		a.t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
		return
	}
	uuid := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, base), "/")
	identity, exists := a.identities[uuid]

	switch {
	case r.Method == http.MethodPost && uuid == "":
		body, ok := a.decode(w, r)
		if !ok {
			return
		}
		a.created++
		uuid = fmt.Sprintf("01a1b2c3-0000-7000-8000-%012d", a.created)
		identity = map[string]any{
			"uuid": uuid, "url": "https://api.buildkite.com" + base + "/" + uuid, "client_id": fmt.Sprintf("bkwi_client_%d", a.created),
			"oidc_audience": nil, "oidc_claims": map[string]any{}, "oidc_claim_conditions": nil, "jwks": nil,
			"default_token_scopes": []any{}, "max_token_ttl_seconds": 3600, "allowed_ip_addresses": nil,
			"allowed_token_exchange_ip_addresses": nil, "resource_grants": []any{}, "enabled": true,
			"created_at": "2026-11-12T00:00:00.000Z", "updated_at": "2026-11-12T00:00:00.000Z",
		}
		maps.Copy(identity, body)
		a.identities[uuid] = identity
		writeFakeJSON(w, http.StatusCreated, identity)
	case !exists:
		writeFakeJSON(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
	case r.Method == http.MethodGet:
		writeFakeJSON(w, http.StatusOK, identity)
	case r.Method == http.MethodPatch:
		body, ok := a.decode(w, r)
		if !ok {
			return
		}
		maps.Copy(identity, body)
		writeFakeJSON(w, http.StatusOK, identity)
	case r.Method == http.MethodDelete:
		delete(a.identities, uuid)
		w.WriteHeader(http.StatusNoContent)
	default:
		a.t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// decode reads a create or update body, rejecting it with a 422 where the API would.
func (a *fakeWorkloadIdentityAPI) decode(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeFakeJSON(w, http.StatusBadRequest, map[string]string{"message": "Problems parsing JSON"})
		return nil, false
	}
	reject := func(message string) (map[string]any, bool) {
		writeFakeJSON(w, http.StatusUnprocessableEntity, map[string]string{"message": message})
		return nil, false
	}

	for field := range body {
		if !slices.Contains(workloadIdentityRequestFields, field) {
			return reject("unknown field " + field)
		}
	}
	if claims, ok := body["oidc_claims"]; ok {
		if _, isObject := claims.(map[string]any); !isObject {
			return reject("oidc_claims must be an object")
		}
	}
	if grants, ok := body["resource_grants"]; ok {
		entries, _ := grants.([]any)
		for _, entry := range entries {
			grant, _ := entry.(map[string]any)
			if _, hasIDs := grant["ids"]; hasIDs != (grant["access"] == "selected") {
				return reject("Resource grants entries must not include unknown fields")
			}
		}
	}
	if conditions, ok := body["oidc_claim_conditions"]; ok && conditions == "" {
		body["oidc_claim_conditions"] = nil
	}

	return body, true
}

func (a *fakeWorkloadIdentityAPI) seed(identity map[string]any) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.identities[identity["uuid"].(string)] = identity
}

// checkStored asserts the stored values of an identity's fields, given as JSON. Arrays are compared
// as sets, because Terraform sends sets in its own order.
func (a *fakeWorkloadIdentityAPI) checkStored(want map[string]string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		a.mu.Lock()
		defer a.mu.Unlock()

		uuid := s.RootModule().Resources[workloadIdentityResourceName].Primary.Attributes["uuid"]
		identity, ok := a.identities[uuid]
		if !ok {
			return fmt.Errorf("workload identity %q does not exist", uuid)
		}
		for field, wantJSON := range want {
			var wantValue any
			if err := json.Unmarshal([]byte(wantJSON), &wantValue); err != nil {
				return fmt.Errorf("invalid expected JSON for %s: %w", field, err)
			}
			if got := identity[field]; !reflect.DeepEqual(sortedJSON(got), sortedJSON(wantValue)) {
				gotJSON, _ := json.Marshal(got)
				return fmt.Errorf("stored %s = %s, want %s", field, gotJSON, wantJSON)
			}
		}
		return nil
	}
}

func (a *fakeWorkloadIdentityAPI) checkDestroyed(*terraform.State) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if len(a.identities) > 0 {
		return fmt.Errorf("%d workload identities still exist", len(a.identities))
	}
	return nil
}

// sortedJSON round-trips a decoded JSON value with its arrays sorted, so arrays compare as sets.
func sortedJSON(value any) any {
	switch value := value.(type) {
	case []any:
		sorted := make([]any, len(value))
		for i, element := range value {
			sorted[i] = sortedJSON(element)
		}
		slices.SortFunc(sorted, func(a, b any) int {
			left, _ := json.Marshal(a)
			right, _ := json.Marshal(b)
			return strings.Compare(string(left), string(right))
		})
		return sorted
	case map[string]any:
		sorted := make(map[string]any, len(value))
		for key, element := range value {
			sorted[key] = sortedJSON(element)
		}
		return sorted
	case float64, string, bool, nil:
		return value
	default:
		// A value the provider sent is always decoded JSON; normalize anything else the fake stored.
		encoded, _ := json.Marshal(value)
		var decoded any
		_ = json.Unmarshal(encoded, &decoded)
		return sortedJSON(decoded)
	}
}

func fakeWorkloadIdentityProvider(server *httptest.Server) string {
	return fmt.Sprintf(`
	provider "buildkite" {
		organization = "acme"
		api_token    = "fake"
		rest_url     = %q
		graphql_url  = %q
	}
	`, server.URL, server.URL+"/graphql")
}

func workloadIdentityConfig(provider, attributes string) string {
	return provider + fmt.Sprintf(`
	resource "buildkite_workload_identity" "release" {
		name                   = "Release automation"
		description            = "Deploys releases"
		oidc_issuer            = "https://issuer.example.com"
		oidc_audience          = "buildkite-releases"
		oidc_subject           = "repo:acme/release:*"
		grantable_token_scopes = ["read_builds", "write_builds"]
		%s
	}
	`, attributes)
}

// workloadIdentityLifecycleSteps create an identity with every optional field set, import it, remove
// every optional field, then set an explicitly empty claims object. remoteChecks verifies the stored
// identity after each apply, which only the fake API can do.
func workloadIdentityLifecycleSteps(provider string, remoteChecks func(map[string]string) resource.TestCheckFunc) []resource.TestStep {
	emptyPlanAfterRefresh := resource.ConfigPlanChecks{
		PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
	}

	return []resource.TestStep{
		{
			// The claims are formatted differently from how the API returns them, which must not plan a change.
			Config: workloadIdentityConfig(provider, `
				enabled = false
				oidc_claims = <<-EOT
					{
					  "repository_owner": "acme",
					  "environment":      "production"
					}
				EOT
				oidc_claim_conditions               = "claims.build_branch == \"main\" || claims.build_branch.startsWith(\"release/\")"
				jwks                                = jsonencode({ keys = [{ kty = "EC", crv = "P-256", kid = "terraform-test", x = "ULk7xWeFZCEhYMlrGQ_ZUXvJLygT5-U5AHeyzZnk-GY", y = "8DUJ6Rhswji9-PrdciSIpZwz7JuYFwqtO1hRtDK9YVo" }] })
				default_token_scopes                = ["read_builds"]
				max_token_ttl_seconds               = 600
				allowed_ip_addresses                = "192.0.2.0/24"
				allowed_token_exchange_ip_addresses = "198.51.100.0/24"
				resource_grants                     = [{ type = "pipeline", access = "all" }]
			`),
			ConfigPlanChecks: emptyPlanAfterRefresh,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttrSet(workloadIdentityResourceName, "uuid"),
				resource.TestCheckResourceAttrSet(workloadIdentityResourceName, "client_id"),
				resource.TestCheckResourceAttr(workloadIdentityResourceName, "enabled", "false"),
				resource.TestCheckResourceAttr(workloadIdentityResourceName, "max_token_ttl_seconds", "600"),
				resource.TestCheckResourceAttr(workloadIdentityResourceName, "resource_grants.0.access", "all"),
				resource.TestCheckNoResourceAttr(workloadIdentityResourceName, "resource_grants.0.ids"),
				remoteChecks(map[string]string{
					"enabled":               `false`,
					"oidc_claims":           `{"repository_owner":"acme","environment":"production"}`,
					"oidc_claim_conditions": `"claims.build_branch == \"main\" || claims.build_branch.startsWith(\"release/\")"`,
					"default_token_scopes":  `["read_builds"]`,
					"resource_grants":       `[{"type":"pipeline","access":"all"}]`,
				}),
			),
		},
		{
			ResourceName:      workloadIdentityResourceName,
			ImportState:       true,
			ImportStateVerify: true,
			ImportStateIdFunc: func(s *terraform.State) (string, error) {
				return s.RootModule().Resources[workloadIdentityResourceName].Primary.Attributes["uuid"], nil
			},
			ImportStateVerifyIdentifierAttribute: "uuid",
			// Imported claims are formatted as the API returns them; the empty plans check they are equivalent.
			ImportStateVerifyIgnore: []string{"oidc_claims"},
		},
		{
			// Removing optional fields clears them in place rather than leaving the old values behind.
			Config: workloadIdentityConfig(provider, ""),
			ConfigPlanChecks: resource.ConfigPlanChecks{
				PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction(workloadIdentityResourceName, plancheck.ResourceActionUpdate)},
				PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
			},
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(workloadIdentityResourceName, "enabled", "true"),
				resource.TestCheckResourceAttr(workloadIdentityResourceName, "max_token_ttl_seconds", "3600"),
				resource.TestCheckResourceAttr(workloadIdentityResourceName, "default_token_scopes.#", "0"),
				resource.TestCheckResourceAttr(workloadIdentityResourceName, "resource_grants.#", "0"),
				resource.TestCheckNoResourceAttr(workloadIdentityResourceName, "oidc_claims"),
				resource.TestCheckNoResourceAttr(workloadIdentityResourceName, "oidc_claim_conditions"),
				resource.TestCheckNoResourceAttr(workloadIdentityResourceName, "jwks"),
				resource.TestCheckNoResourceAttr(workloadIdentityResourceName, "allowed_ip_addresses"),
				resource.TestCheckNoResourceAttr(workloadIdentityResourceName, "allowed_token_exchange_ip_addresses"),
				remoteChecks(map[string]string{
					"enabled":                             `true`,
					"oidc_claims":                         `{}`,
					"oidc_claim_conditions":               `null`,
					"jwks":                                `null`,
					"default_token_scopes":                `[]`,
					"max_token_ttl_seconds":               `3600`,
					"allowed_ip_addresses":                `null`,
					"allowed_token_exchange_ip_addresses": `null`,
					"resource_grants":                     `[]`,
				}),
			),
		},
		{
			Config: workloadIdentityConfig(provider, `
				oidc_claims     = jsonencode({})
				resource_grants = [{ type = "pipeline", access = "none" }]
			`),
			ConfigPlanChecks: emptyPlanAfterRefresh,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(workloadIdentityResourceName, "oidc_claims", "{}"),
				resource.TestCheckResourceAttr(workloadIdentityResourceName, "resource_grants.0.access", "none"),
				remoteChecks(map[string]string{
					"oidc_claims":     `{}`,
					"resource_grants": `[{"type":"pipeline","access":"none"}]`,
				}),
			),
		},
	}
}

func TestUnitBuildkiteWorkloadIdentityLifecycle(t *testing.T) {
	server, api := newFakeWorkloadIdentityAPI(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             api.checkDestroyed,
		Steps:                    workloadIdentityLifecycleSteps(fakeWorkloadIdentityProvider(server), api.checkStored),
	})
}

func TestUnitBuildkiteWorkloadIdentityValidationMessages(t *testing.T) {
	server, api := newFakeWorkloadIdentityAPI(t)
	provider := fakeWorkloadIdentityProvider(server)
	valid := workloadIdentityConfig(provider, "")
	invalid := workloadIdentityConfig(provider, `oidc_claims = "[]"`)
	validationError := regexp.MustCompile(`oidc_claims must be an object`)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             api.checkDestroyed,
		Steps: []resource.TestStep{
			{Config: invalid, ExpectError: validationError},
			{Config: valid},
			{Config: invalid, ExpectError: validationError},
			{Config: valid, PlanOnly: true},
		},
	})
}

func TestWorkloadIdentityValidationResponseIsolation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		status      int
		body        string
		wantMessage bool
	}{
		{"validation", 422, `{"message":"Claim is invalid","token":"hidden-response-field"}`, true},
		{"forbidden", 403, `{"message":"hidden-response-field"}`, false},
		{"unavailable", 503, `{"message":"hidden-response-field"}`, false},
		{"HTML", 422, "<html>hidden-response-field</html>", false},
		{"malformed JSON", 422, `{"message":"hidden-response-field"`, false},
		{"missing message", 422, `{"token":"hidden-response-field"}`, false},
		{"oversized response", 422, `{"message":"` + strings.Repeat("hidden-response-field", 200) + `"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			r := &workloadIdentityResource{client: &Client{http: server.Client(), restURL: server.URL, organization: "acme"}}
			diags := runCreate(t.Context(), t, r, nil)
			if !diagnosticsContain(diags, fmt.Sprintf("status: %d", tc.status)) {
				t.Fatalf("Create() diagnostics = %v, want status %d", diags, tc.status)
			}
			if got := diagnosticsContain(diags, "Claim is invalid"); got != tc.wantMessage {
				t.Errorf("Create() diagnostics = %v, want validation message exposed: %t", diags, tc.wantMessage)
			}
			if diagnosticsContain(diags, "hidden-response-field") {
				t.Errorf("Create() diagnostics exposed response content outside the validation message: %v", diags)
			}
		})
	}
}

func TestUnitBuildkiteWorkloadIdentitySelectedPipelines(t *testing.T) {
	server, api := newFakeWorkloadIdentityAPI(t)
	provider := fakeWorkloadIdentityProvider(server)

	// Identities created before resource grants and claims were added return an empty grants object
	// and null claims, which both mean none.
	const legacyUUID = "01a1b2c3-0000-7000-8000-999999999999"
	api.seed(map[string]any{
		"uuid": legacyUUID, "client_id": "bkwi_legacy", "name": "Release automation", "description": "Deploys releases",
		"oidc_issuer": "https://issuer.example.com", "oidc_audience": "buildkite-releases", "oidc_subject": "repo:acme/release:*",
		"oidc_claims": nil, "oidc_claim_conditions": nil, "jwks": nil, "grantable_token_scopes": []any{"read_builds", "write_builds"},
		"default_token_scopes": []any{}, "max_token_ttl_seconds": 3600, "allowed_ip_addresses": nil,
		"allowed_token_exchange_ip_addresses": nil, "resource_grants": map[string]any{}, "enabled": true,
	})

	const (
		pipelineA = "0190c3d4-1e2f-7a3b-9c4d-5e6f7a8b9c0d"
		pipelineB = "0190c3d4-1e2f-7a3b-9c4d-5e6f7a8b9c0e"
	)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             api.checkDestroyed,
		Steps: []resource.TestStep{
			{
				Config:             workloadIdentityConfig(provider, ""),
				ResourceName:       workloadIdentityResourceName,
				ImportState:        true,
				ImportStateId:      legacyUUID,
				ImportStatePersist: true,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					attributes := states[0].Attributes
					if attributes["resource_grants.#"] != "0" || attributes["oidc_claims"] != "" {
						return fmt.Errorf("legacy grants and claims were not imported as none: %v", attributes)
					}
					return nil
				},
			},
			{
				Config:   workloadIdentityConfig(provider, ""),
				PlanOnly: true,
			},
			{
				Config: workloadIdentityConfig(provider, fmt.Sprintf(`
					resource_grants = [{ type = "pipeline", access = "selected", ids = [%q, %q] }]
				`, pipelineB, pipelineA)),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckTypeSetElemAttr(workloadIdentityResourceName, "resource_grants.0.ids.*", pipelineA),
					resource.TestCheckTypeSetElemAttr(workloadIdentityResourceName, "resource_grants.0.ids.*", pipelineB),
					api.checkStored(map[string]string{
						"resource_grants": fmt.Sprintf(`[{"type":"pipeline","access":"selected","ids":[%q,%q]}]`, pipelineA, pipelineB),
					}),
				),
			},
			{
				// The API rejects ids on any other access, so they must not be sent.
				Config: workloadIdentityConfig(provider, `
					resource_grants = [{ type = "pipeline", access = "all" }]
				`),
				Check: api.checkStored(map[string]string{
					"resource_grants": `[{"type":"pipeline","access":"all"}]`,
				}),
			},
		},
	})
}

// TestAccBuildkiteWorkloadIdentity runs the lifecycle against the live API. Workload identities must be
// enabled for the organization, and the token's user must be an organization administrator with the
// read_workload_identities, write_workload_identities, and delete_workload_identities scopes.
func TestAccBuildkiteWorkloadIdentity(t *testing.T) {
	if os.Getenv("BUILDKITE_WORKLOAD_IDENTITIES_ACCEPTANCE") != "1" {
		t.Skip("live workload identities are untested; set BUILDKITE_WORKLOAD_IDENTITIES_ACCEPTANCE=1 against an organization with workload identities enabled")
	}

	noRemoteChecks := func(map[string]string) resource.TestCheckFunc {
		return func(*terraform.State) error { return nil }
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckWorkloadIdentityDestroy,
		Steps:                    workloadIdentityLifecycleSteps("", noRemoteChecks),
	})
}

func testAccCheckWorkloadIdentityDestroy(s *terraform.State) error {
	client := getTestClient()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_workload_identity" {
			continue
		}
		var identity workloadIdentity
		err := client.makeRequest(context.Background(), http.MethodGet, client.workloadIdentityPath(rs.Primary.Attributes["uuid"]), nil, &identity)
		if err == nil {
			return fmt.Errorf("workload identity %s still exists", rs.Primary.Attributes["uuid"])
		}
		if !isAPIStatus(err, http.StatusNotFound) {
			return fmt.Errorf("checking workload identity %s was deleted: %w", rs.Primary.Attributes["uuid"], err)
		}
	}
	return nil
}
