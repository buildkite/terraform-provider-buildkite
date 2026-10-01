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
