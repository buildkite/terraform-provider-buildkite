package buildkite

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	fakeClusterUUID      = "0b6f6f35-1111-2222-3333-444455556666"
	fakeTracingServiceA  = "7c3e5a91-aaaa-4bbb-8ccc-000000000001"
	fakeTracingServiceB  = "7c3e5a91-aaaa-4bbb-8ccc-000000000002"
	fakeClusterGraphQLID = "cluster-graphql-id"
)

// fakeClusterAPI models one cluster behind the GraphQL operations the cluster resource and data
// sources make. Like the API, it treats agentTracingServiceUuid by whether the key is present: an
// absent key keeps the selection, and any key, even null, is refused while tracingDisabled is set.
type fakeClusterAPI struct {
	t               *testing.T
	mu              sync.Mutex
	exists          bool
	name            string
	description     *string
	tracingService  *string
	tracingDisabled bool
	// writes holds every cluster create or update, in order
	writes []fakeClusterWrite
	// hosted is whether the cluster has had a hosted queue, which the REST API requires before it
	// changes either hosted cache setting
	hosted         bool
	gitMirror      bool
	containerCache bool
	// unmanaged answers REST reads the way the API does for a token that cannot manage the cluster:
	// without the hosted cache settings
	unmanaged bool
	// readStatus, when set, fails REST reads with that status: 403 is a token without the read_clusters
	// scope, and 404 a cluster outside the provider's organization
	readStatus int
	// patches holds the body of every REST cluster update, in order
	patches []string
}

// fakeClusterWrite records how a write treated agentTracingServiceUuid: tracingService is the JSON it
// sent, "null" included, or "" when the key was left out
type fakeClusterWrite struct {
	operation      string
	tracingService string
}

func newFakeClusterAPI(t *testing.T) (*httptest.Server, *fakeClusterAPI) {
	t.Helper()

	api := &fakeClusterAPI{t: t}
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)

	return server, api
}

func (a *fakeClusterAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	switch r.URL.Path {
	case "/graphql":
		a.graphql(w, r)
	case "/v2/organizations/acme/clusters/" + fakeClusterUUID:
		a.rest(w, r)
	case "/v2/organizations/acme/clusters/" + fakeClusterUUID + "/maintainers":
		_, _ = io.WriteString(w, `[]`)
	default:
		a.t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	}
}

func (a *fakeClusterAPI) graphql(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OperationName string                     `json:"operationName"`
		Query         string                     `json:"query"`
		Variables     map[string]json.RawMessage `json:"variables"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		a.t.Errorf("unable to read the GraphQL request: %v", err)
		return
	}

	// the organization id is fetched with an anonymous query
	response := `{"data":{"organization":{"id":"organization-graphql-id"}}}`
	switch body.OperationName {
	case "createCluster", "updateCluster", "updateClusterClearingAgentTracingService":
		raw, ok := body.Variables["agentTracingServiceUuid"]
		// genqlient sends the operation with its whitespace removed
		if strings.Contains(strings.ReplaceAll(body.Query, " ", ""), "agentTracingServiceUuid:null") {
			raw, ok = json.RawMessage("null"), true
		}
		a.writes = append(a.writes, fakeClusterWrite{operation: body.OperationName, tracingService: string(raw)})
		if ok {
			if a.tracingDisabled {
				response = `{"data":null,"errors":[{"message":"Agent tracing configuration is not enabled for this organization"}]}`
				break
			}
			a.tracingService = nil
			if err := json.Unmarshal(raw, &a.tracingService); err != nil {
				a.t.Errorf("agentTracingServiceUuid = %s, want a string or null", raw)
			}
		}
		a.exists = true
		if err := json.Unmarshal(body.Variables["name"], &a.name); err != nil {
			a.t.Errorf("name = %s, want a string", body.Variables["name"])
		}
		a.description = nil
		if err := json.Unmarshal(body.Variables["description"], &a.description); err != nil {
			a.t.Errorf("description = %s, want a string or null", body.Variables["description"])
		}
		field := "clusterUpdate"
		if body.OperationName == "createCluster" {
			field = "clusterCreate"
		}
		response = fmt.Sprintf(`{"data":{%q:{"clientMutationId":null,"cluster":%s}}}`, field, a.clusterJSON())
	case "getNode":
		node := "null"
		if a.exists {
			node = a.clusterJSON()
		}
		response = fmt.Sprintf(`{"data":{"node":%s}}`, node)
	case "getClusterByName", "GetOrganizationClusters":
		response = fmt.Sprintf(`{"data":{"organization":{"clusters":{`+
			`"pageInfo":{"endCursor":"","hasNextPage":false},"edges":[{"node":%s}]}}}}`, a.clusterJSON())
	case "deleteCluster":
		a.exists = false
		response = `{"data":{"clusterDelete":{"clientMutationId":null}}}`
	}

	if _, err := io.WriteString(w, response); err != nil {
		a.t.Errorf("unable to write the GraphQL response: %v", err)
	}
}

// rest serves the REST cluster, which carries the hosted cache settings that GraphQL does not
func (a *fakeClusterAPI) rest(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && a.readStatus != 0 {
		http.Error(w, `{"message":"refused"}`, a.readStatus)
		return
	}
	if r.Method == http.MethodPatch {
		body, _ := io.ReadAll(r.Body)
		a.patches = append(a.patches, string(body))

		var settings map[string]bool
		if err := json.Unmarshal(body, &settings); err != nil {
			a.t.Errorf("PATCH body = %s, want booleans", body)
		}
		for key, value := range settings {
			current := map[string]*bool{"hosted_git_mirror_enabled": &a.gitMirror, "hosted_container_cache_enabled": &a.containerCache}[key]
			if current == nil {
				a.t.Errorf("PATCH sent %s, want only hosted cache settings", key)
				continue
			}
			if !a.hosted && value != *current {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = fmt.Fprintf(w, `{"message":"%s can only be changed on hosted clusters"}`, key)
				return
			}
			*current = value
		}
	}

	cluster := map[string]any{"id": fakeClusterUUID, "name": a.name}
	if !a.unmanaged {
		cluster["hosted_git_mirror_enabled"] = a.gitMirror
		cluster["hosted_container_cache_enabled"] = a.containerCache
	}
	if err := json.NewEncoder(w).Encode(cluster); err != nil {
		a.t.Errorf("unable to write the REST response: %v", err)
	}
}

// addHostedQueue does what creating the cluster's first hosted queue does to it
func (a *fakeClusterAPI) addHostedQueue() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.hosted = true
	a.containerCache = true
}

// checkPatches checks the body of every REST cluster update made so far
func (a *fakeClusterAPI) checkPatches(want ...string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		a.mu.Lock()
		defer a.mu.Unlock()

		if fmt.Sprint(a.patches) != fmt.Sprint(want) {
			return fmt.Errorf("REST cluster updates = %q, want %q", a.patches, want)
		}
		return nil
	}
}

// clusterJSON answers the way the API reads: no selection unless agent tracing is enabled
func (a *fakeClusterAPI) clusterJSON() string {
	tracingService := a.tracingService
	if a.tracingDisabled {
		tracingService = nil
	}
	cluster, err := json.Marshal(map[string]any{
		"__typename":              "Cluster",
		"id":                      fakeClusterGraphQLID,
		"uuid":                    fakeClusterUUID,
		"name":                    a.name,
		"description":             a.description,
		"emoji":                   nil,
		"color":                   nil,
		"agentTracingServiceUuid": tracingService,
		"defaultQueue":            nil,
	})
	if err != nil {
		a.t.Fatalf("unable to encode the cluster: %v", err)
	}
	return string(cluster)
}

// seed gives the fake a cluster made outside Terraform, for a test to import
func (a *fakeClusterAPI) seed(tracingService string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.exists = true
	a.name = "test"
	if tracingService != "" {
		a.tracingService = &tracingService
	}
}

func (a *fakeClusterAPI) setTracingService(uuid string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.tracingService = &uuid
}

// checkLastWrite checks the operation of the last write and the agentTracingServiceUuid it sent: a UUID,
// "null", or "" for the key left out
func (a *fakeClusterAPI) checkLastWrite(operation, want string) resource.TestCheckFunc {
	if want != "" && want != "null" {
		want = fmt.Sprintf("%q", want)
	}
	return func(*terraform.State) error {
		a.mu.Lock()
		defer a.mu.Unlock()

		if len(a.writes) == 0 {
			return fmt.Errorf("no cluster write was made, want %s", operation)
		}
		write := a.writes[len(a.writes)-1]
		switch {
		case write.operation != operation:
			return fmt.Errorf("the last write was %s, want %s", write.operation, operation)
		case write.tracingService != want && want == "":
			return fmt.Errorf("%s sent agentTracingServiceUuid = %s, want the key left out", operation, write.tracingService)
		case write.tracingService != want:
			return fmt.Errorf("%s sent agentTracingServiceUuid = %q, want %s", operation, write.tracingService, want)
		}
		return nil
	}
}

// checkWrites checks how many cluster writes have been made, so a step can show it wrote nothing
func (a *fakeClusterAPI) checkWrites(want int) resource.TestCheckFunc {
	return func(*terraform.State) error {
		a.mu.Lock()
		defer a.mu.Unlock()

		if len(a.writes) != want {
			return fmt.Errorf("%d cluster writes were made, want %d: %v", len(a.writes), want, a.writes)
		}
		return nil
	}
}

func (a *fakeClusterAPI) checkTracingService(want string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		a.mu.Lock()
		defer a.mu.Unlock()

		got := ""
		if a.tracingService != nil {
			got = *a.tracingService
		}
		if got != want {
			return fmt.Errorf("the cluster's agent tracing service = %q, want %q", got, want)
		}
		return nil
	}
}

func fakeClusterConfig(server *httptest.Server, attributes string) string {
	return fmt.Sprintf(`
	provider "buildkite" {
		organization = "acme"
		api_token    = "fake"
		rest_url     = %q
		graphql_url  = %q
	}

	resource "buildkite_cluster" "test" {
		name = "test"
		%s
	}
	`, server.URL, server.URL+"/graphql", attributes)
}

func TestUnitBuildkiteClusterAgentTracingServiceAgainstFakeAPI(t *testing.T) {
	server, api := newFakeClusterAPI(t)

	const name = "buildkite_cluster.test"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fakeClusterConfig(server, ``),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(name, "agent_tracing_service_uuid"),
					api.checkLastWrite("createCluster", ""),
				),
			},
			{
				// a selection made outside Terraform is adopted, and other changes leave it out of the write
				PreConfig: func() { api.setTracingService(fakeTracingServiceA) },
				Config:    fakeClusterConfig(server, `description = "changed"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "agent_tracing_service_uuid", fakeTracingServiceA),
					api.checkLastWrite("updateCluster", ""),
					api.checkTracingService(fakeTracingServiceA),
				),
			},
			{
				Config: fakeClusterConfig(server, fmt.Sprintf(`agent_tracing_service_uuid = %q`, fakeTracingServiceB)),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "agent_tracing_service_uuid", fakeTracingServiceB),
					api.checkLastWrite("updateCluster", fakeTracingServiceB),
					api.checkTracingService(fakeTracingServiceB),
				),
			},
			{
				ResourceName:      name,
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				// removing the attribute keeps the selection: there is nothing to plan, so nothing is written
				Config: fakeClusterConfig(server, ``),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "agent_tracing_service_uuid", fakeTracingServiceB),
					api.checkTracingService(fakeTracingServiceB),
				),
			},
			{
				Config: fakeClusterConfig(server, `description = "changed again"`) + `
				data "buildkite_cluster" "test" {
					name       = "test"
					depends_on = [buildkite_cluster.test]
				}

				data "buildkite_clusters" "all" {
					depends_on = [buildkite_cluster.test]
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					api.checkLastWrite("updateCluster", ""),
					resource.TestCheckResourceAttr("data.buildkite_cluster.test", "agent_tracing_service_uuid", fakeTracingServiceB),
					resource.TestCheckResourceAttr("data.buildkite_clusters.all", "clusters.0.agent_tracing_service_uuid", fakeTracingServiceB),
				),
			},
		},
	})
}

func TestUnitBuildkiteClusterCreatesWithAnAgentTracingService(t *testing.T) {
	server, api := newFakeClusterAPI(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fakeClusterConfig(server, fmt.Sprintf(`agent_tracing_service_uuid = %q`, fakeTracingServiceA)),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("buildkite_cluster.test", "agent_tracing_service_uuid", fakeTracingServiceA),
					api.checkLastWrite("createCluster", fakeTracingServiceA),
				),
			},
		},
	})
}

// An organization without agent tracing refuses the key even as null, so a cluster that never
// configures it has to be creatable and updatable without sending it.
func TestUnitBuildkiteClusterWithoutAgentTracingEnabled(t *testing.T) {
	server, api := newFakeClusterAPI(t)
	api.tracingDisabled = true

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fakeClusterConfig(server, ``),
				Check:  api.checkLastWrite("createCluster", ""),
			},
			{
				Config: fakeClusterConfig(server, `description = "changed"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("buildkite_cluster.test", "agent_tracing_service_uuid"),
					api.checkLastWrite("updateCluster", ""),
				),
			},
			{
				Config:      fakeClusterConfig(server, fmt.Sprintf(`agent_tracing_service_uuid = %q`, fakeTracingServiceA)),
				ExpectError: regexp.MustCompile(`Agent tracing configuration is not enabled`),
			},
		},
	})
}

func TestUnitBuildkiteClusterRejectsAnAgentTracingServiceThatIsNotALowercaseUUID(t *testing.T) {
	server, _ := newFakeClusterAPI(t)

	for _, value := range []string{
		strings.ToUpper(fakeTracingServiceA),
		// a notification service's graphql_id rather than its id
		"U2VydmljZS0tLTdjM2U1YTkxLWFhYWEtNGJiYi04Y2NjLTAwMDAwMDAwMDAwMQ==",
	} {
		t.Run(value, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				Steps: []resource.TestStep{
					{
						Config:      fakeClusterConfig(server, fmt.Sprintf(`agent_tracing_service_uuid = %q`, value)),
						PlanOnly:    true,
						ExpectError: regexp.MustCompile(`must be a lowercase UUID`),
					},
				},
			})
		})
	}
}

func TestUnitBuildkiteClusterClearsTheAgentTracingServiceWithAnEmptyString(t *testing.T) {
	server, api := newFakeClusterAPI(t)

	const name = "buildkite_cluster.test"
	cleared := `agent_tracing_service_uuid = ""`

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fakeClusterConfig(server, fmt.Sprintf(`agent_tracing_service_uuid = %q`, fakeTracingServiceA)),
				Check:  api.checkLastWrite("createCluster", fakeTracingServiceA),
			},
			{
				// "" clears with an explicit null, and is kept although the API reads it back as null
				Config: fakeClusterConfig(server, cleared),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "agent_tracing_service_uuid", ""),
					api.checkLastWrite("updateClusterClearingAgentTracingService", "null"),
					api.checkTracingService(""),
				),
			},
			{
				// once cleared, other changes leave the key out again
				Config: fakeClusterConfig(server, cleared+"\ndescription = \"changed\""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "agent_tracing_service_uuid", ""),
					api.checkLastWrite("updateCluster", ""),
				),
			},
			{
				// removing the attribute after clearing plans nothing
				Config: fakeClusterConfig(server, `description = "changed"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "agent_tracing_service_uuid", ""),
					api.checkWrites(3),
				),
			},
			{
				// a selection made outside Terraform is cleared again while the configuration says ""
				PreConfig: func() { api.setTracingService(fakeTracingServiceB) },
				Config:    fakeClusterConfig(server, cleared+"\ndescription = \"changed\""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "agent_tracing_service_uuid", ""),
					api.checkLastWrite("updateClusterClearingAgentTracingService", "null"),
					api.checkTracingService(""),
				),
			},
		},
	})
}

// A new cluster has no selection to clear, so "" is created without the key, which an organization
// without agent tracing would refuse
func TestUnitBuildkiteClusterCreatesWithAnEmptyAgentTracingService(t *testing.T) {
	server, api := newFakeClusterAPI(t)
	api.tracingDisabled = true

	cleared := `agent_tracing_service_uuid = ""`

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fakeClusterConfig(server, cleared),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("buildkite_cluster.test", "agent_tracing_service_uuid", ""),
					api.checkLastWrite("createCluster", ""),
				),
			},
			{
				Config: fakeClusterConfig(server, cleared+"\ndescription = \"changed\""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: api.checkLastWrite("updateCluster", ""),
			},
		},
	})
}

func TestUnitBuildkiteClusterImportedThenClearedWithAnEmptyString(t *testing.T) {
	const name = "buildkite_cluster.test"
	cleared := `agent_tracing_service_uuid = ""`

	t.Run("with a selection", func(t *testing.T) {
		server, api := newFakeClusterAPI(t)
		api.seed(fakeTracingServiceA)

		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config:             fakeClusterConfig(server, cleared),
					ResourceName:       name,
					ImportState:        true,
					ImportStateId:      fakeClusterGraphQLID,
					ImportStatePersist: true,
				},
				{
					Config: fakeClusterConfig(server, cleared),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
					},
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr(name, "agent_tracing_service_uuid", ""),
						api.checkLastWrite("updateClusterClearingAgentTracingService", "null"),
						api.checkTracingService(""),
					),
				},
			},
		})
	})

	t.Run("without a selection", func(t *testing.T) {
		server, api := newFakeClusterAPI(t)
		api.seed("")
		api.tracingDisabled = true

		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config:             fakeClusterConfig(server, cleared),
					ResourceName:       name,
					ImportState:        true,
					ImportStateId:      fakeClusterGraphQLID,
					ImportStatePersist: true,
				},
				{
					// the import reads null, so "" is one update that has nothing to clear and leaves the key out
					Config: fakeClusterConfig(server, cleared),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
					},
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr(name, "agent_tracing_service_uuid", ""),
						api.checkLastWrite("updateCluster", ""),
					),
				},
			},
		})
	})
}

func TestUnitBuildkiteClusterHostedCacheSettingsAgainstFakeAPI(t *testing.T) {
	server, api := newFakeClusterAPI(t)

	const name = "buildkite_cluster.test"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fakeClusterConfig(server, ``),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "hosted_git_mirror_enabled", "false"),
					resource.TestCheckResourceAttr(name, "hosted_container_cache_enabled", "false"),
					api.checkPatches(),
				),
			},
			{
				// the first hosted queue enables the container cache, which is adopted while unconfigured,
				// and only the setting that changes is sent
				PreConfig: api.addHostedQueue,
				Config:    fakeClusterConfig(server, `hosted_git_mirror_enabled = true`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "hosted_git_mirror_enabled", "true"),
					resource.TestCheckResourceAttr(name, "hosted_container_cache_enabled", "true"),
					api.checkPatches(`{"hosted_git_mirror_enabled":true}`),
				),
			},
			{
				Config: fakeClusterConfig(server, "hosted_git_mirror_enabled = true\nhosted_container_cache_enabled = false"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "hosted_git_mirror_enabled", "true"),
					resource.TestCheckResourceAttr(name, "hosted_container_cache_enabled", "false"),
					api.checkPatches(`{"hosted_git_mirror_enabled":true}`, `{"hosted_container_cache_enabled":false}`),
				),
			},
			{
				ResourceName:      name,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// The API refuses the hosted cache settings until the cluster has a hosted queue, which needs the
// cluster first, so they are refused when creating it and explained when updating it too early.
func TestUnitBuildkiteClusterHostedCacheSettingsWithoutAHostedQueue(t *testing.T) {
	server, api := newFakeClusterAPI(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      fakeClusterConfig(server, `hosted_git_mirror_enabled = true`),
				ExpectError: regexp.MustCompile(`Cannot enable hosted_git_mirror_enabled when creating or replacing a Cluster`),
			},
			{
				Config: fakeClusterConfig(server, ``),
				Check:  api.checkWrites(1),
			},
			{
				// the GraphQL change applies although the hosted cache setting is refused
				Config:      fakeClusterConfig(server, "description = \"changed\"\nhosted_git_mirror_enabled = true"),
				ExpectError: regexp.MustCompile(`(?s)status: 422.*at least one hosted queue`),
			},
			{
				Config:   fakeClusterConfig(server, `description = "changed"`),
				PlanOnly: true,
			},
		},
	})
}

// The API leaves the hosted cache settings out for a token that cannot manage the cluster, which
// must not fail the cluster
func TestUnitBuildkiteClusterWithoutHostedCacheSettingsInTheAPI(t *testing.T) {
	server, api := newFakeClusterAPI(t)
	api.unmanaged = true

	const name = "buildkite_cluster.test"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fakeClusterConfig(server, ``),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(name, "hosted_git_mirror_enabled"),
					resource.TestCheckNoResourceAttr(name, "hosted_container_cache_enabled"),
				),
			},
			{
				Config: fakeClusterConfig(server, `description = "changed"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(name, "hosted_git_mirror_enabled"),
					api.checkPatches(),
				),
			},
		},
	})
}

// A REST read of the hosted cache settings that is refused must not fail the cluster: a token without the
// read_clusters scope, as a GraphQL-only token is, gets a 403, and a cluster outside the provider's
// organization a 404. With write_clusters the token can still change them.
func TestUnitBuildkiteClusterWithTheHostedCacheSettingsReadRefused(t *testing.T) {
	const name = "buildkite_cluster.test"

	for _, status := range []int{http.StatusForbidden, http.StatusNotFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server, api := newFakeClusterAPI(t)
			api.readStatus = status

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				Steps: []resource.TestStep{
					{
						Config: fakeClusterConfig(server, ``),
						ConfigPlanChecks: resource.ConfigPlanChecks{
							PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
						},
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckNoResourceAttr(name, "hosted_git_mirror_enabled"),
							resource.TestCheckNoResourceAttr(name, "hosted_container_cache_enabled"),
						),
					},
					{
						// the setting left unconfigured stays null in state, rather than taking the API's value
						PreConfig: api.addHostedQueue,
						Config:    fakeClusterConfig(server, `hosted_git_mirror_enabled = true`),
						ConfigPlanChecks: resource.ConfigPlanChecks{
							PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
						},
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr(name, "hosted_git_mirror_enabled", "true"),
							resource.TestCheckNoResourceAttr(name, "hosted_container_cache_enabled"),
							api.checkPatches(`{"hosted_git_mirror_enabled":true}`),
							api.checkWrites(1),
						),
					},
				},
			})
		})
	}
}

// A new cluster has both settings disabled, so false is accepted when creating it and needs no write.
// The first hosted queue then enables the container cache, which a configured false turns back off.
func TestUnitBuildkiteClusterCreatesWithHostedCacheSettingsDisabled(t *testing.T) {
	server, api := newFakeClusterAPI(t)

	const name = "buildkite_cluster.test"
	disabled := "hosted_git_mirror_enabled = false\nhosted_container_cache_enabled = false"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fakeClusterConfig(server, disabled),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "hosted_git_mirror_enabled", "false"),
					resource.TestCheckResourceAttr(name, "hosted_container_cache_enabled", "false"),
					api.checkPatches(),
				),
			},
			{
				PreConfig: api.addHostedQueue,
				Config:    fakeClusterConfig(server, disabled),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction(name, plancheck.ResourceActionUpdate)},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "hosted_container_cache_enabled", "false"),
					api.checkPatches(`{"hosted_container_cache_enabled":false}`),
					api.checkWrites(1),
				),
			},
		},
	})
}

func TestUnitBuildkiteClusterRefusesAnUnknownHostedCacheSettingWhenCreating(t *testing.T) {
	server, _ := newFakeClusterAPI(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				// terraform_data's output is not known until apply, so it could be true
				Config: fakeClusterConfig(server, `hosted_git_mirror_enabled = terraform_data.flag.output`) + `
				resource "terraform_data" "flag" {
					input = false
				}
				`,
				ExpectError: regexp.MustCompile(`Cannot enable hosted_git_mirror_enabled when creating or replacing a Cluster`),
			},
		},
	})
}
