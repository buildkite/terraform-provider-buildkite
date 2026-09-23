package buildkite

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

const (
	cacheRegistryTestAddress        = "buildkite_cluster_cache_registry.test"
	cacheRegistryEmptyPolicy        = `{"save":{"scopes":{}},"restore":{"scopes":[]},"rules":[]}`
	cacheRegistryDefaultPolicy      = `{"save":{"scopes":{}},"restore":{"scopes":[{}]},"rules":[{"effect":"allow","action":["save"]},{"effect":"allow","action":["restore"]}]}`
	cacheRegistryNullableNamePolicy = `{"rules":[{"name":null,"effect":"allow","action":"save"}]}`
	cacheRegistryNullableWhenPolicy = `{"rules":[{"when":null,"effect":"allow","action":"save"}]}`
	cacheRegistryNullablePolicy     = `{"rules":[{"name":null,"when":null,"effect":"allow","action":"save"}]}`
	cacheRegistrySavePolicy         = `{"save":{"scopes":{}},"restore":{"scopes":[]},"rules":[{"effect":"allow","action":["save"]}]}`
)

type cacheRegistryTestAPI struct {
	t               *testing.T
	mu              sync.Mutex
	registry        map[string]any
	operations      map[string]int
	mutations       []map[string]any
	overridePolicy  bool
	responsePolicy  *string
	errorOperation  string
	errorMessage    string
	hideNode        bool
	parentDeleted   bool
	denyMembership  bool
	denyManagement  bool
	featureDisabled bool
	ownerID         string
	parentPages     bool
	registryPages   bool
	pageFailure     string
	nullField       string
	deleteRace      string
	retryOperation  string
	retries         int
}

func newCacheRegistryTestAPI(t *testing.T) (*httptest.Server, *cacheRegistryTestAPI) {
	t.Helper()
	api := &cacheRegistryTestAPI{t: t, operations: map[string]int{}, ownerID: "organization-id"}
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)
	return server, api
}

func (a *cacheRegistryTestAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if r.Header.Get("Authorization") != "Bearer dummy" {
		a.t.Errorf("expected the explicitly configured dummy API token")
	}
	var request struct {
		OperationName string         `json:"operationName"`
		Query         string         `json:"query"`
		Variables     map[string]any `json:"variables"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		a.t.Errorf("decode GraphQL request: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	operation := request.OperationName
	if operation == "" && strings.Contains(request.Query, "organization(slug:") {
		operation = "getOrganization"
	}
	a.operations[operation]++
	if operation == a.retryOperation && a.retries == 0 {
		a.retries++
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	write := func(value any) {
		if err := json.NewEncoder(w).Encode(value); err != nil {
			a.t.Errorf("encode GraphQL response: %v", err)
		}
	}
	fail := func(message string) {
		write(map[string]any{"errors": []any{map[string]any{"message": message}}})
	}
	if operation == a.errorOperation {
		fail(a.errorMessage)
		return
	}
	var data any
	organizationID := "organization-id"
	if request.Variables["orgSlug"] == "other-org" {
		organizationID = "other-organization-id"
	}
	organization := map[string]any{"id": organizationID, "permissions": map[string]any{
		"organizationMemberView": map[string]any{"allowed": !a.denyMembership},
	}}
	if a.nullField == "permissions" {
		organization["permissions"] = nil
	}
	if a.nullField == "membership" {
		organization["permissions"] = map[string]any{"organizationMemberView": nil}
	}
	if a.nullField == "allowed" {
		organization["permissions"] = map[string]any{"organizationMemberView": map[string]any{"allowed": nil}}
	}
	if a.pageFailure != "" && request.Variables["cursor"] != nil {
		switch a.pageFailure {
		case "membership":
			organization["permissions"] = map[string]any{"organizationMemberView": map[string]any{"allowed": false}}
		case "identity":
			organization["id"] = "changed-organization-id"
		default:
			fail(a.pageFailure)
			return
		}
	}
	switch operation {
	case "getCacheRegistryOrganization":
		data = map[string]any{"organization": organization}
	case "getCacheRegistryClusters":
		edges := []any{}
		next := a.parentPages && request.Variables["cursor"] == nil
		if next {
			edges = append(edges, map[string]any{"node": map[string]any{"id": "other-cluster", "uuid": "other-uuid"}})
		} else if !a.parentDeleted && !a.denyMembership && organizationID == a.ownerID {
			edges = append(edges, map[string]any{"node": map[string]any{"id": "cluster-id", "uuid": "cluster-uuid"}})
		}
		organization["clusters"] = map[string]any{"edges": edges, "pageInfo": map[string]any{"hasNextPage": next, "endCursor": "parent-page-2"}}
		if a.nullField == "clusters" {
			organization["clusters"] = nil
		}
		if a.nullField == "parentPageInfo" {
			organization["clusters"].(map[string]any)["pageInfo"] = nil
		}
		data = map[string]any{"organization": organization}
	case "getOrganization":
		data = map[string]any{"organization": map[string]any{"id": "organization-id"}}
	case "createCacheRegistry", "updateCacheRegistry":
		variables := request.Variables
		if variables["organizationId"] != "organization-id" {
			a.t.Errorf("unexpected organization: %v", variables)
		}
		policy := cacheRegistryDefaultPolicy
		if operation == "updateCacheRegistry" {
			if a.registry == nil || variables["id"] != a.registry["id"] {
				fail("No cache registry found")
				return
			}
			policy, _ = a.registry["policy"].(string)
		} else if a.registry != nil {
			a.t.Error("create would orphan the existing cache registry")
			fail("Cache registry already exists")
			return
		}
		if value, ok := variables["policy"].(string); ok {
			fixtures := map[string]string{
				`{}`:                            cacheRegistryEmptyPolicy,
				cacheRegistryEmptyPolicy:        cacheRegistryEmptyPolicy,
				cacheRegistryDefaultPolicy:      cacheRegistryDefaultPolicy,
				cacheRegistryNullableNamePolicy: cacheRegistrySavePolicy,
				cacheRegistryNullableWhenPolicy: cacheRegistrySavePolicy,
				cacheRegistryNullablePolicy:     cacheRegistrySavePolicy,
				cacheRegistrySavePolicy:         cacheRegistrySavePolicy,
			}
			found := false
			for input, output := range fixtures {
				if cacheRegistryTestJSONEqual(value, input) {
					policy, found = output, true
					break
				}
			}
			if !found {
				fail("Invalid policy: unsupported policy fixture")
				return
			}
		}
		name, _ := variables["name"].(string)
		a.registry = map[string]any{
			"__typename": "CacheRegistry", "id": "cache-id", "uuid": "cache-uuid",
			"name": name, "slug": strings.ReplaceAll(strings.ToLower(name), " ", "-"),
			"description": variables["description"], "emoji": variables["emoji"], "color": variables["color"],
			"policy": policy, "createdAt": "2026-01-02T03:04:05Z", "updatedAt": "2026-01-02T04:04:05Z",
			"cacheRegistryCluster": map[string]any{"id": "cluster-id", "uuid": "cluster-uuid", "organization": map[string]any{"id": a.ownerID}},
		}
		if a.overridePolicy {
			a.registry["policy"] = a.responsePolicy
		}
		a.mutations = append(a.mutations, variables)
		field := "cacheRegistryCreate"
		if operation == "updateCacheRegistry" {
			field = "cacheRegistryUpdate"
		}
		data = map[string]any{field: map[string]any{"cacheRegistry": a.registry}}
	case "getCacheRegistryByNode":
		var node any
		if !a.hideNode && !a.denyMembership && !a.denyManagement && !a.featureDisabled && a.registry != nil && request.Variables["id"] == a.registry["id"] {
			node = a.registry
		}
		data = map[string]any{"node": node, "organization": organization}
	case "getClusterCacheRegistries":
		if a.parentDeleted {
			fail("No cluster found")
			return
		}
		if a.denyManagement || a.featureDisabled {
			fail("Cache registries require enabled feature and cluster management permission")
			return
		}
		edges := []any{}
		next := a.registryPages && request.Variables["cursor"] == nil
		if next {
			edges = append(edges, map[string]any{"node": map[string]any{"id": "other-registry"}})
		} else if a.registry != nil {
			edges = append(edges, map[string]any{"node": map[string]any{"id": a.registry["id"]}})
		}
		connection := map[string]any{"edges": edges, "pageInfo": map[string]any{"hasNextPage": next, "endCursor": "registry-page-2"}}
		if a.nullField == "edges" {
			connection["edges"] = nil
		}
		if a.nullField == "node" {
			connection["edges"] = []any{map[string]any{"node": nil}}
		}
		if a.nullField == "pageInfo" {
			connection["pageInfo"] = nil
		}
		if a.nullField == "cursor" {
			connection["pageInfo"] = map[string]any{"hasNextPage": true, "endCursor": nil}
		}
		if a.nullField == "repeatedCursor" {
			connection["pageInfo"] = map[string]any{"hasNextPage": true, "endCursor": "same"}
		}
		organization["cluster"] = map[string]any{"id": "cluster-id", "cacheRegistries": connection}
		data = map[string]any{"organization": organization}
	case "deleteCacheRegistry":
		if a.deleteRace != "" {
			switch a.deleteRace {
			case "membership":
				a.denyMembership = true
			case "management":
				a.denyManagement = true
			case "feature":
				a.featureDisabled = true
			case "deleted":
				a.registry = nil
			case "parent-deleted":
				a.registry, a.parentDeleted = nil, true
			}
			fail("No cache registry found")
			return
		}
		if a.registry == nil || request.Variables["id"] != a.registry["id"] {
			fail("No cache registry found")
			return
		}
		a.registry = nil
		data = map[string]any{"cacheRegistryDelete": map[string]any{"deletedCacheRegistryId": "cache-id"}}
	default:
		a.t.Errorf("unexpected GraphQL operation %q: %s", operation, request.Query)
		fail("Unexpected operation")
		return
	}
	write(map[string]any{"data": data})
}

func cacheRegistryTestJSONEqual(left, right string) bool {
	var l, r any
	return json.Unmarshal([]byte(left), &l) == nil && json.Unmarshal([]byte(right), &r) == nil && reflect.DeepEqual(l, r)
}

func cacheRegistryTestConfig(server *httptest.Server, name, attributes string) string {
	return fmt.Sprintf(`
provider "buildkite" {
  organization = "test-org"
  api_token = "dummy"
  graphql_url = %q
  rest_url = %q
}
resource "buildkite_cluster_cache_registry" "test" {
  cluster_id = "cluster-id"
  name = %q
  %s
}
`, server.URL, server.URL, name, attributes)
}

func (a *cacheRegistryTestAPI) checkDestroyed(*terraform.State) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.registry != nil {
		return fmt.Errorf("cache registry still exists: %v", a.registry)
	}
	return nil
}

func cacheRegistryTestPolicyCheck(want string) resource.TestCheckFunc {
	return resource.TestCheckResourceAttrWith(cacheRegistryTestAddress, "policy", func(got string) error {
		if !cacheRegistryTestJSONEqual(got, want) {
			return fmt.Errorf("policy = %s, want %s", got, want)
		}
		return nil
	})
}

func TestUnitClusterCacheRegistryNullablePolicy(t *testing.T) {
	for name, policy := range map[string]string{
		"name": cacheRegistryNullableNamePolicy,
		"when": cacheRegistryNullableWhenPolicy,
		"both": cacheRegistryNullablePolicy,
	} {
		t.Run(name, func(t *testing.T) {
			server, api := newCacheRegistryTestAPI(t)
			config := cacheRegistryTestConfig(server, "Cache", fmt.Sprintf("policy = %q", policy))
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             api.checkDestroyed,
				Steps: []resource.TestStep{
					{Config: config, Check: cacheRegistryTestPolicyCheck(policy)},
					{Config: config, PlanOnly: true},
				},
			})
		})
	}
}

func TestUnitClusterCacheRegistryPolicyLifecycle(t *testing.T) {
	server, api := newCacheRegistryTestAPI(t)
	config := func(name, attributes string) string { return cacheRegistryTestConfig(server, name, attributes) }
	configured := fmt.Sprintf("policy = %q", cacheRegistryNullablePolicy)
	empty := fmt.Sprintf("policy = %q", cacheRegistryEmptyPolicy)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             api.checkDestroyed,
		Steps: []resource.TestStep{
			{Config: config("Cache", ""), Check: cacheRegistryTestPolicyCheck(cacheRegistryDefaultPolicy)},
			{RefreshState: true, Check: cacheRegistryTestPolicyCheck(cacheRegistryDefaultPolicy)},
			{Config: config("Cache", `description = "  Shared cache  "
emoji = ":package:"
color = "#BADA55"`), Check: resource.ComposeAggregateTestCheckFunc(
				cacheRegistryTestPolicyCheck(cacheRegistryDefaultPolicy),
				resource.TestCheckResourceAttr(cacheRegistryTestAddress, "description", "  Shared cache  "),
				resource.TestCheckResourceAttr(cacheRegistryTestAddress, "color", "#BADA55"),
			)},
			{Config: config("Renamed Cache", configured), Check: resource.ComposeAggregateTestCheckFunc(
				cacheRegistryTestPolicyCheck(cacheRegistryNullablePolicy),
				resource.TestCheckResourceAttr(cacheRegistryTestAddress, "slug", "renamed-cache"),
				resource.TestCheckNoResourceAttr(cacheRegistryTestAddress, "description"),
				resource.TestCheckNoResourceAttr(cacheRegistryTestAddress, "emoji"),
				resource.TestCheckNoResourceAttr(cacheRegistryTestAddress, "color"),
			)},
			{ResourceName: cacheRegistryTestAddress, ImportState: true, ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{"policy"},
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 || !cacheRegistryTestJSONEqual(states[0].Attributes["policy"], cacheRegistrySavePolicy) {
						return fmt.Errorf("import did not return the complete normalized save policy: %v", states)
					}
					return nil
				},
			},
			{Config: config("Renamed Cache", configured), PlanOnly: true},
			{Config: config("Renamed Cache", `description = "policy removed"`), Check: cacheRegistryTestPolicyCheck(cacheRegistryNullablePolicy)},
			{Config: config("Renamed Cache", empty), Check: cacheRegistryTestPolicyCheck(cacheRegistryEmptyPolicy)},
			{Config: config("Renamed Cache", empty), PlanOnly: true},
			{PreConfig: func() {
				api.mu.Lock()
				defer api.mu.Unlock()
				api.registry["policy"] = cacheRegistryDefaultPolicy
			}, Config: config("Renamed Cache", empty),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(cacheRegistryTestAddress, plancheck.ResourceActionUpdate),
				}},
				Check: cacheRegistryTestPolicyCheck(cacheRegistryEmptyPolicy),
			},
		},
	})
	api.mu.Lock()
	defer api.mu.Unlock()
	if api.operations["createCacheRegistry"] != 1 || api.operations["deleteCacheRegistry"] != 1 {
		t.Fatalf("unexpected lifecycle operations: %v", api.operations)
	}
	if api.mutations[0]["policy"] != nil {
		t.Fatalf("omitted policy sent on create: %v", api.mutations[0])
	}
}

func TestUnitClusterCacheRegistryEmptyStrings(t *testing.T) {
	server, api := newCacheRegistryTestAPI(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             api.checkDestroyed,
		Steps: []resource.TestStep{
			{Config: cacheRegistryTestConfig(server, "Cache", `description = ""
emoji = ""
color = ""`), Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(cacheRegistryTestAddress, "description", ""),
				resource.TestCheckResourceAttr(cacheRegistryTestAddress, "emoji", ""),
				resource.TestCheckResourceAttr(cacheRegistryTestAddress, "color", ""),
			)},
			{Config: cacheRegistryTestConfig(server, "Cache", ""), Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckNoResourceAttr(cacheRegistryTestAddress, "description"),
				resource.TestCheckNoResourceAttr(cacheRegistryTestAddress, "emoji"),
				resource.TestCheckNoResourceAttr(cacheRegistryTestAddress, "color"),
			)},
		},
	})
}

func TestUnitClusterCacheRegistryValidation(t *testing.T) {
	server, api := newCacheRegistryTestAPI(t)
	api.errorOperation = "createCacheRegistry"
	api.errorMessage = "Invalid rule at index 0: action must be save or restore"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             api.checkDestroyed,
		Steps: []resource.TestStep{
			{Config: cacheRegistryTestConfig(server, "Cache", `policy = "null"`), ExpectError: regexp.MustCompile("Invalid cache registry policy")},
			{Config: cacheRegistryTestConfig(server, "Cache", `policy = "{broken"`), ExpectError: regexp.MustCompile("Invalid")},
			{Config: cacheRegistryTestConfig(server, "Cache", `policy = "{\"rules\":[{\"effect\":\"allow\",\"action\":\"invalid\"}]}"`), ExpectError: regexp.MustCompile("Invalid rule at index 0")},
		},
	})
	api.mu.Lock()
	defer api.mu.Unlock()
	if api.operations["createCacheRegistry"] != 1 || api.operations["deleteCacheRegistry"] != 0 || api.registry != nil {
		t.Fatalf("rejected create changed remote state: %v", api.operations)
	}
}

func TestUnitClusterCacheRegistryUnknownPolicy(t *testing.T) {
	server, api := newCacheRegistryTestAPI(t)
	config := cacheRegistryTestConfig(server, "Cache", "policy = terraform_data.policy.output") + fmt.Sprintf(`
resource "terraform_data" "policy" {
  input = %q
}
`, cacheRegistryNullablePolicy)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             api.checkDestroyed,
		Steps: []resource.TestStep{
			{Config: config, Check: cacheRegistryTestPolicyCheck(cacheRegistryNullablePolicy),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectUnknownValue(cacheRegistryTestAddress, tfjsonpath.New("policy")),
				}},
			},
			{Config: config, PlanOnly: true},
		},
	})
}

func TestUnitClusterCacheRegistryApplyErrorPreservesIdentity(t *testing.T) {
	for _, operation := range []string{"create", "update"} {
		for name, policy := range map[string]*string{
			"different": ptr(cacheRegistryDefaultPolicy),
			"null":      nil,
			"malformed": ptr("{broken"),
		} {
			t.Run(operation+"/"+name, func(t *testing.T) {
				server, api := newCacheRegistryTestAPI(t)
				config := cacheRegistryTestConfig(server, "Cache", fmt.Sprintf("policy = %q", cacheRegistryNullablePolicy))
				steps := []resource.TestStep{}
				if operation == "update" {
					steps = append(steps, resource.TestStep{Config: config})
					config = cacheRegistryTestConfig(server, "Renamed Cache", fmt.Sprintf("policy = %q", cacheRegistryNullablePolicy))
				}
				steps = append(steps, resource.TestStep{
					PreConfig: func() {
						api.mu.Lock()
						defer api.mu.Unlock()
						api.overridePolicy, api.responsePolicy = true, policy
					},
					Config:      config,
					ExpectError: regexp.MustCompile("Invalid Cache Registry " + operation + " response"),
				}, resource.TestStep{
					PreConfig: func() {
						api.mu.Lock()
						defer api.mu.Unlock()
						if api.registry == nil {
							t.Fatal("expected the mutation to have created a remote registry")
						}
						api.registry["policy"] = cacheRegistryDefaultPolicy
					},
					Config:  config,
					Destroy: true,
				})
				resource.UnitTest(t, resource.TestCase{
					ProtoV6ProviderFactories: protoV6ProviderFactories(),
					CheckDestroy:             api.checkDestroyed,
					Steps:                    steps,
				})
				api.mu.Lock()
				defer api.mu.Unlock()
				if api.operations["createCacheRegistry"] != 1 || api.operations["deleteCacheRegistry"] != 1 {
					t.Fatalf("failed apply lost or replaced the remote identity: %v", api.operations)
				}
			})
		}
	}
}

func TestUnitClusterCacheRegistryUpdateRejected(t *testing.T) {
	server, api := newCacheRegistryTestAPI(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             api.checkDestroyed,
		Steps: []resource.TestStep{
			{Config: cacheRegistryTestConfig(server, "Cache", "")},
			{PreConfig: func() {
				api.mu.Lock()
				defer api.mu.Unlock()
				api.errorOperation = "updateCacheRegistry"
				api.errorMessage = "Invalid policy: must be an object"
			}, Config: cacheRegistryTestConfig(server, "Renamed Cache", ""), ExpectError: regexp.MustCompile("Unable to update Cache Registry")},
			{Config: cacheRegistryTestConfig(server, "Cache", ""), PlanOnly: true, Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(cacheRegistryTestAddress, "name", "Cache"),
				cacheRegistryTestPolicyCheck(cacheRegistryDefaultPolicy),
			)},
		},
	})
}

func TestUnitClusterCacheRegistryPartialMutation(t *testing.T) {
	for _, operation := range []string{"create", "update"} {
		for _, payload := range []string{"organization null", "different organization", "policy null", "identity only"} {
			t.Run(operation+"/"+payload, func(t *testing.T) {
				_, api := newCacheRegistryTestAPI(t)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					recorder := httptest.NewRecorder()
					api.ServeHTTP(recorder, r)
					var body map[string]any
					if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
						t.Error(err)
						return
					}
					field := "cacheRegistryCreate"
					if operation == "update" {
						field = "cacheRegistryUpdate"
					}
					if data, ok := body["data"].(map[string]any); ok {
						if mutation, ok := data[field].(map[string]any); ok {
							registry := mutation["cacheRegistry"].(map[string]any)
							switch payload {
							case "organization null":
								registry["cacheRegistryCluster"].(map[string]any)["organization"] = nil
							case "different organization":
								registry["cacheRegistryCluster"] = map[string]any{"id": "untrusted-cluster", "uuid": "untrusted-uuid", "organization": map[string]any{"id": "untrusted-organization"}}
							case "policy null":
								registry["policy"] = nil
							case "identity only":
								mutation["cacheRegistry"] = map[string]any{"id": registry["id"]}
							}
							body["errors"] = []any{map[string]any{"message": "Organization lookup is currently busy, please try again", "path": []any{field, "cacheRegistry", "cacheRegistryCluster", "organization"}}}
						}
					}
					w.Header().Set("Content-Type", "application/json")
					if err := json.NewEncoder(w).Encode(body); err != nil {
						t.Error(err)
					}
				}))
				defer server.Close()
				config := cacheRegistryTestConfig(server, "Cache", "")
				steps := []resource.TestStep{}
				if operation == "update" {
					steps = append(steps, resource.TestStep{Config: config})
					config = cacheRegistryTestConfig(server, "Renamed Cache", "")
				}
				steps = append(steps,
					resource.TestStep{Config: config, ExpectError: regexp.MustCompile("Unable to " + operation + " Cache Registry")},
					resource.TestStep{RefreshState: true, ExpectNonEmptyPlan: operation == "create", Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr(cacheRegistryTestAddress, "id", "cache-id"),
						resource.TestCheckResourceAttr(cacheRegistryTestAddress, "organization_id", "organization-id"),
						resource.TestCheckResourceAttr(cacheRegistryTestAddress, "cluster_id", "cluster-id"),
					)},
					resource.TestStep{Config: config, Destroy: true},
				)
				resource.UnitTest(t, resource.TestCase{
					ProtoV6ProviderFactories: protoV6ProviderFactories(),
					CheckDestroy:             api.checkDestroyed,
					Steps:                    steps,
				})
				api.mu.Lock()
				defer api.mu.Unlock()
				if api.operations[operation+"CacheRegistry"] != 1 || api.operations["createCacheRegistry"] != 1 || api.operations["deleteCacheRegistry"] != 1 {
					t.Fatalf("partial mutation lost or repeated the remote identity: %v", api.operations)
				}
			})
		}
	}
}
