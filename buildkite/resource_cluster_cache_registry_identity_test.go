package buildkite

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

func TestClusterCacheRegistryIdentityAndAbsence(t *testing.T) {
	for name, test := range map[string]struct {
		change    func(*cacheRegistryTestAPI, *clusterCacheRegistryResourceModel, *clusterCacheRegistryResource)
		wantError string
		absent    bool
	}{
		"registry deleted": {change: func(a *cacheRegistryTestAPI, _ *clusterCacheRegistryResourceModel, _ *clusterCacheRegistryResource) {
			a.registry = nil
		}, absent: true},
		"parent deleted": {change: func(a *cacheRegistryTestAPI, _ *clusterCacheRegistryResourceModel, _ *clusterCacheRegistryResource) {
			a.registry, a.parentDeleted = nil, true
		}, absent: true},
		"public org membership lost": {change: func(a *cacheRegistryTestAPI, _ *clusterCacheRegistryResourceModel, _ *clusterCacheRegistryResource) {
			a.denyMembership = true
		}, wantError: "authorized organization membership"},
		"live parent management denied": {change: func(a *cacheRegistryTestAPI, _ *clusterCacheRegistryResourceModel, _ *clusterCacheRegistryResource) {
			a.denyManagement = true
		}, wantError: "cluster management permission"},
		"feature disabled": {change: func(a *cacheRegistryTestAPI, _ *clusterCacheRegistryResourceModel, _ *clusterCacheRegistryResource) {
			a.featureDisabled = true
		}, wantError: "enabled feature"},
		"hidden node": {change: func(a *cacheRegistryTestAPI, _ *clusterCacheRegistryResourceModel, _ *clusterCacheRegistryResource) {
			a.hideNode = true
		}, wantError: "still exists"},
		"older state recovered": {change: func(_ *cacheRegistryTestAPI, s *clusterCacheRegistryResourceModel, _ *clusterCacheRegistryResource) {
			s.OrganizationID = types.StringNull()
		}},
		"older missing state": {change: func(a *cacheRegistryTestAPI, s *clusterCacheRegistryResourceModel, _ *clusterCacheRegistryResource) {
			s.OrganizationID = types.StringNull()
			a.registry = nil
		}, wantError: "restore access and refresh"},
		"missing import": {change: func(a *cacheRegistryTestAPI, s *clusterCacheRegistryResourceModel, _ *clusterCacheRegistryResource) {
			*s = clusterCacheRegistryResourceModel{ID: s.ID}
			a.registry = nil
		}, wantError: "ID-only import"},
		"inaccessible import": {change: func(a *cacheRegistryTestAPI, s *clusterCacheRegistryResourceModel, _ *clusterCacheRegistryResource) {
			*s = clusterCacheRegistryResourceModel{ID: s.ID}
			a.hideNode = true
		}, wantError: "ID-only import"},
		"wrong org import": {change: func(_ *cacheRegistryTestAPI, s *clusterCacheRegistryResourceModel, r *clusterCacheRegistryResource) {
			*s = clusterCacheRegistryResourceModel{ID: s.ID}
			r.client.organization = "other-org"
		}, wantError: "belongs to organization"},
		"provider org changed": {change: func(_ *cacheRegistryTestAPI, _ *clusterCacheRegistryResourceModel, r *clusterCacheRegistryResource) {
			r.client.organization = "other-org"
		}, wantError: "belongs to organization"},
		"provider org changed missing node": {change: func(a *cacheRegistryTestAPI, _ *clusterCacheRegistryResourceModel, r *clusterCacheRegistryResource) {
			r.client.organization = "other-org"
			a.registry = nil
		}, wantError: "belongs to organization"},
		"Relay API error": {change: func(a *cacheRegistryTestAPI, _ *clusterCacheRegistryResourceModel, _ *clusterCacheRegistryResource) {
			a.errorOperation, a.errorMessage = "getCacheRegistryByNode", "No cache registry found"
		}, wantError: "No cache registry found"},
		"cluster API error": {change: func(a *cacheRegistryTestAPI, _ *clusterCacheRegistryResourceModel, _ *clusterCacheRegistryResource) {
			a.hideNode = true
			a.errorOperation, a.errorMessage = "getCacheRegistryClusters", "No cluster found"
		}, wantError: "No cluster found"},
		"registry API error": {change: func(a *cacheRegistryTestAPI, _ *clusterCacheRegistryResourceModel, _ *clusterCacheRegistryResource) {
			a.hideNode = true
			a.errorOperation, a.errorMessage = "getClusterCacheRegistries", "No cache registry found"
		}, wantError: "No cache registry found"},
	} {
		for _, operation := range []string{"lookup", "read", "delete", "update"} {
			t.Run(name+"/"+operation, func(t *testing.T) {
				ctx := context.Background()
				server, api := newCacheRegistryTestAPI(t)
				r := clusterCacheRegistryResource{client: NewClient(&clientConfig{org: "test-org", apiToken: "dummy", graphqlURL: server.URL, restURL: server.URL})}
				state := createCacheRegistryCallbackState(t, &r)
				var model clusterCacheRegistryResourceModel
				if diags := state.Get(ctx, &model); diags.HasError() {
					t.Fatal(diags)
				}
				api.mu.Lock()
				api.parentPages, api.registryPages = true, true
				test.change(api, &model, &r)
				api.mu.Unlock()
				if diags := state.Set(ctx, &model); diags.HasError() {
					t.Fatal(diags)
				}
				var diagnostics string
				var hasError bool
				returned := state
				switch operation {
				case "lookup":
					values, err := r.lookupCacheRegistry(ctx, &model)
					absent := errors.Is(err, errCacheRegistryAbsent)
					if absent != test.absent || (values != nil) != (err == nil) {
						t.Fatalf("unexpected lookup result: values=%v, err=%v", values, err)
					}
					diagnostics, hasError = fmt.Sprint(err), err != nil && !absent
				case "read":
					resp := frameworkresource.ReadResponse{State: state}
					r.Read(ctx, frameworkresource.ReadRequest{State: state}, &resp)
					diagnostics, hasError, returned = fmt.Sprint(resp.Diagnostics), resp.Diagnostics.HasError(), resp.State
					if !hasError && returned.Raw.IsNull() != test.absent {
						t.Fatalf("unexpected removed state: %v", returned.Raw)
					}
					if !hasError && !test.absent {
						var got clusterCacheRegistryResourceModel
						if diags := returned.Get(ctx, &got); diags.HasError() {
							t.Fatal(diags)
						}
						if got.OrganizationID.ValueString() != "organization-id" {
							t.Fatal("ownership not persisted")
						}
					}
				case "delete":
					resp := frameworkresource.DeleteResponse{State: state}
					r.Delete(ctx, frameworkresource.DeleteRequest{State: state}, &resp)
					diagnostics, hasError, returned = fmt.Sprint(resp.Diagnostics), resp.Diagnostics.HasError(), resp.State
				case "update":
					resp := frameworkresource.UpdateResponse{State: state}
					r.Update(ctx, frameworkresource.UpdateRequest{State: state, Plan: tfsdk.Plan(state)}, &resp)
					diagnostics, hasError, returned = fmt.Sprint(resp.Diagnostics), resp.Diagnostics.HasError(), resp.State
				}
				wantError := test.wantError
				if operation == "update" && test.absent {
					wantError = "no longer exists"
				}
				if hasError != (wantError != "") || (wantError != "" && !strings.Contains(diagnostics, wantError)) {
					t.Fatalf("diagnostics = %s, want %q", diagnostics, wantError)
				}
				if hasError && !returned.Raw.Equal(state.Raw) {
					t.Fatal("failed callback changed state")
				}
				api.mu.Lock()
				defer api.mu.Unlock()
				if (hasError || test.absent) && (api.operations["deleteCacheRegistry"] != 0 || api.operations["updateCacheRegistry"] != 0) {
					t.Fatalf("unexpected mutation: %v", api.operations)
				}
				if test.absent && api.operations["getCacheRegistryClusters"] != 3 {
					t.Fatalf("parent pages not exhausted: %v", api.operations)
				}
				if test.absent && !api.parentDeleted && api.operations["getClusterCacheRegistries"] != 2 {
					t.Fatalf("registry pages not exhausted: %v", api.operations)
				}
				if api.parentDeleted && api.operations["getClusterCacheRegistries"] != 0 {
					t.Fatal("queried deleted parent")
				}
			})
		}
	}
}

func createCacheRegistryCallbackState(t *testing.T, r *clusterCacheRegistryResource) tfsdk.State {
	t.Helper()
	ctx := context.Background()
	var schema frameworkresource.SchemaResponse
	r.Schema(ctx, frameworkresource.SchemaRequest{}, &schema)
	plan := tfsdk.Plan{Schema: schema.Schema}
	if diags := plan.Set(ctx, &clusterCacheRegistryResourceModel{ClusterID: types.StringValue("cluster-id"), Name: types.StringValue("Cache"), Policy: jsontypes.NewNormalizedNull()}); diags.HasError() {
		t.Fatal(diags)
	}
	resp := frameworkresource.CreateResponse{State: tfsdk.State{Schema: schema.Schema, Raw: plan.Raw}}
	r.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	return resp.State
}

func TestClusterCacheRegistryIncompleteAbsenceProof(t *testing.T) {
	for _, field := range []string{"permissions", "membership", "allowed", "clusters", "edges", "node", "parentPageInfo", "pageInfo", "cursor", "repeatedCursor"} {
		for _, operation := range []string{"read", "delete"} {
			t.Run(field+"/"+operation, func(t *testing.T) {
				server, api := newCacheRegistryTestAPI(t)
				r := clusterCacheRegistryResource{client: NewClient(&clientConfig{org: "test-org", apiToken: "dummy", graphqlURL: server.URL, restURL: server.URL})}
				state := createCacheRegistryCallbackState(t, &r)
				api.mu.Lock()
				api.registry, api.nullField = nil, field
				api.mu.Unlock()
				ctx := context.Background()
				if operation == "read" {
					resp := frameworkresource.ReadResponse{State: state}
					r.Read(ctx, frameworkresource.ReadRequest{State: state}, &resp)
					if !resp.Diagnostics.HasError() || !resp.State.Raw.Equal(state.Raw) {
						t.Fatalf("failed proof changed state: %v", resp.Diagnostics)
					}
				} else {
					resp := frameworkresource.DeleteResponse{State: state}
					r.Delete(ctx, frameworkresource.DeleteRequest{State: state}, &resp)
					if !resp.Diagnostics.HasError() || !resp.State.Raw.Equal(state.Raw) {
						t.Fatalf("failed proof changed state: %v", resp.Diagnostics)
					}
				}
			})
		}
	}
}

func TestClusterCacheRegistryDeleteRechecksNotFound(t *testing.T) {
	for _, race := range []string{"membership", "management", "feature", "still-present", "deleted", "parent-deleted"} {
		t.Run(race, func(t *testing.T) {
			server, api := newCacheRegistryTestAPI(t)
			r := clusterCacheRegistryResource{client: NewClient(&clientConfig{org: "test-org", apiToken: "dummy", graphqlURL: server.URL, restURL: server.URL})}
			state := createCacheRegistryCallbackState(t, &r)
			api.mu.Lock()
			api.deleteRace = race
			api.mu.Unlock()
			resp := frameworkresource.DeleteResponse{State: state}
			r.Delete(context.Background(), frameworkresource.DeleteRequest{State: state}, &resp)
			if resp.Diagnostics.HasError() != (race != "deleted" && race != "parent-deleted") {
				t.Fatalf("unexpected delete result: %v", resp.Diagnostics)
			}
			if resp.Diagnostics.HasError() && !resp.State.Raw.Equal(state.Raw) {
				t.Fatal("failed deletion changed state")
			}
			api.mu.Lock()
			defer api.mu.Unlock()
			if api.operations["getCacheRegistryByNode"] != 2 || api.operations["deleteCacheRegistry"] != 1 {
				t.Fatalf("missing verification: %v", api.operations)
			}
		})
	}
}

func TestUnitClusterCacheRegistryDeletedRecreation(t *testing.T) {
	for _, parent := range []bool{false, true} {
		t.Run(fmt.Sprintf("parent=%t", parent), func(t *testing.T) {
			server, api := newCacheRegistryTestAPI(t)
			config := cacheRegistryTestConfig(server, "Cache", "")
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: protoV6ProviderFactories(), CheckDestroy: api.checkDestroyed,
				Steps: []resource.TestStep{
					{Config: config, Check: resource.TestCheckResourceAttr(cacheRegistryTestAddress, "organization_id", "organization-id")},
					{PreConfig: func() {
						api.mu.Lock()
						defer api.mu.Unlock()
						api.registry, api.parentDeleted = nil, parent
						api.parentPages, api.registryPages = true, true
					}, Config: config, PlanOnly: true, ExpectNonEmptyPlan: true,
						ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectResourceAction(cacheRegistryTestAddress, plancheck.ResourceActionCreate)}}},
					{PreConfig: func() { api.mu.Lock(); defer api.mu.Unlock(); api.parentDeleted = false }, Config: config,
						Check: resource.TestCheckResourceAttr(cacheRegistryTestAddress, "organization_id", "organization-id")},
				},
			})
		})
	}
}

func TestUnitClusterCacheRegistryAccessAndOrganization(t *testing.T) {
	t.Setenv("BUILDKITE_API_TOKEN", "environment-token-must-not-be-used")
	t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "environment-org-must-not-be-used")
	for _, failure := range []string{"membership", "management", "feature", "provider", "import", "missing import", "parent page membership", "registry page membership", "parent page identity", "registry page error"} {
		t.Run(failure, func(t *testing.T) {
			server, api := newCacheRegistryTestAPI(t)
			config := cacheRegistryTestConfig(server, "Cache", "")
			step := resource.TestStep{Config: config, PlanOnly: true, ExpectError: regexp.MustCompile("Unable to read Cache Registry")}
			step.PreConfig = func() {
				api.mu.Lock()
				defer api.mu.Unlock()
				switch failure {
				case "membership":
					api.denyMembership = true
				case "management":
					api.denyManagement = true
				case "feature":
					api.featureDisabled = true
				case "missing import":
					api.hideNode = true
				case "parent page membership":
					api.hideNode, api.parentPages, api.pageFailure = true, true, "membership"
				case "registry page membership":
					api.hideNode, api.registryPages, api.pageFailure = true, true, "membership"
				case "parent page identity":
					api.hideNode, api.parentPages, api.pageFailure = true, true, "identity"
				case "registry page error":
					api.hideNode, api.registryPages, api.pageFailure = true, true, "request failed"
				}
			}
			if failure == "provider" || failure == "import" {
				step.Config = strings.ReplaceAll(config, `"test-org"`, `"other-org"`)
			}
			if failure == "import" || failure == "missing import" {
				step.PlanOnly, step.ImportState, step.ResourceName, step.ImportStateId = false, true, cacheRegistryTestAddress, "cache-id"
			}
			if failure == "missing import" {
				step.ImportStateId = "missing-cache-id"
			}
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: protoV6ProviderFactories(), CheckDestroy: api.checkDestroyed,
				Steps: []resource.TestStep{
					{Config: config}, step,
					{PreConfig: func() {
						api.mu.Lock()
						defer api.mu.Unlock()
						api.denyMembership, api.denyManagement, api.featureDisabled, api.hideNode = false, false, false, false
						api.pageFailure = ""
					}, Config: config, PlanOnly: true,
						Check: resource.TestCheckResourceAttr(cacheRegistryTestAddress, "organization_id", "organization-id")},
				},
			})
			api.mu.Lock()
			defer api.mu.Unlock()
			if api.operations["createCacheRegistry"] != 1 || api.operations["updateCacheRegistry"] != 0 || api.operations["deleteCacheRegistry"] != 1 {
				t.Fatalf("unexpected mutation: %v", api.operations)
			}
		})
	}
}

func TestClusterCacheRegistryRetry(t *testing.T) {
	server, api := newCacheRegistryTestAPI(t)
	r := clusterCacheRegistryResource{client: NewClient(&clientConfig{org: "test-org", apiToken: "dummy", graphqlURL: server.URL, restURL: server.URL, maxRetries: 1})}
	state := createCacheRegistryCallbackState(t, &r)
	api.mu.Lock()
	api.retryOperation = "getCacheRegistryByNode"
	api.mu.Unlock()
	resp := frameworkresource.ReadResponse{State: state}
	r.Read(context.Background(), frameworkresource.ReadRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() || resp.State.Raw.IsNull() {
		t.Fatal(resp.Diagnostics)
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	if api.operations["getCacheRegistryByNode"] != 2 {
		t.Fatalf("request was not retried: %v", api.operations)
	}
}

func TestClusterCacheRegistryCreateOwnership(t *testing.T) {
	for _, denied := range []bool{false, true} {
		t.Run(fmt.Sprintf("membershipDenied=%t", denied), func(t *testing.T) {
			ctx := context.Background()
			server, api := newCacheRegistryTestAPI(t)
			api.denyMembership = denied
			r := clusterCacheRegistryResource{client: NewClient(&clientConfig{org: "other-org", apiToken: "dummy", graphqlURL: server.URL, restURL: server.URL})}
			var schema frameworkresource.SchemaResponse
			r.Schema(ctx, frameworkresource.SchemaRequest{}, &schema)
			plan := tfsdk.Plan{Schema: schema.Schema}
			if diags := plan.Set(ctx, &clusterCacheRegistryResourceModel{ClusterID: types.StringValue("cluster-id"), Name: types.StringValue("Cache"), Policy: jsontypes.NewNormalizedNull()}); diags.HasError() {
				t.Fatal(diags)
			}
			resp := frameworkresource.CreateResponse{State: tfsdk.State{Schema: schema.Schema, Raw: plan.Raw}}
			r.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &resp)
			if !resp.Diagnostics.HasError() {
				t.Fatal("accepted an unverified cluster owner")
			}
			api.mu.Lock()
			defer api.mu.Unlock()
			if api.operations["createCacheRegistry"] != 0 {
				t.Fatal("created registry before verifying ownership")
			}
		})
	}
}
