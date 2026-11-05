package buildkite

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	genqlient "github.com/Khan/genqlient/graphql"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/shurcooL/graphql"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

var (
	graphqlClient    *graphql.Client
	genqlientGraphql genqlient.Client
	organizationID   string
)

func init() {
	client := getTestClient()
	graphqlClient = client.graphql
	genqlientGraphql = client.genqlient
	if os.Getenv("TF_ACC") != "" {
		if id, err := client.GetOrganizationID(context.Background()); err == nil {
			organizationID = *id
		}
	}
}

func getTestClient() *Client {
	return getTestClientWithToken(os.Getenv("BUILDKITE_API_TOKEN"))
}

func getTestClientWithToken(token string) *Client {
	graphqlEndpoint := defaultGraphqlEndpoint
	if v, ok := os.LookupEnv("BUILDKITE_GRAPHQL_URL"); ok {
		graphqlEndpoint = v
	}
	restEndpoint := defaultRestEndpoint
	if v, ok := os.LookupEnv("BUILDKITE_REST_URL"); ok {
		restEndpoint = v
	}
	return NewClient(&clientConfig{
		org:        getenv("BUILDKITE_ORGANIZATION_SLUG"),
		apiToken:   token,
		graphqlURL: graphqlEndpoint,
		restURL:    restEndpoint,
		userAgent:  "terraform-provider-buildkite-test",
		maxRetries: DefaultRetryMaxAttempts,
	})
}

func TestSharedTestClientRetries(t *testing.T) {
	for _, protocol := range []string{"REST", "GraphQL", "genqlient"} {
		t.Run(protocol, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				wantPath := "/graphql"
				if protocol == "REST" {
					wantPath = "/rest/organizations/test-org"
				}
				if r.URL.Path != wantPath {
					t.Errorf("request path = %q, want %q", r.URL.Path, wantPath)
				}
				if r.Header.Get("Authorization") != "Bearer test-token" || r.Header.Get("User-Agent") != "terraform-provider-buildkite-test" {
					t.Error("acceptance client did not send its credentials and user agent")
				}
				w.Header().Set("Content-Type", "application/json")
				if requests.Add(1) == 1 {
					w.Header().Set("RateLimit-Reset", "0")
					w.WriteHeader(http.StatusTooManyRequests)
					_, _ = w.Write([]byte(`{"message":"rate limited"}`))
					return
				}
				_, _ = w.Write([]byte(`{"name":"test-org","data":{"organization":{"name":"test-org"}}}`))
			}))
			t.Cleanup(server.Close)
			t.Setenv("BUILDKITE_API_TOKEN", "test-token")
			t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "test-org")
			t.Setenv("BUILDKITE_REST_URL", server.URL+"/rest")
			t.Setenv("BUILDKITE_GRAPHQL_URL", server.URL+"/graphql")
			client := getTestClient()
			// Shorten only the waits, retaining the acceptance client's retry budget.
			if client.restRetry != nil {
				client.restRetry.RetryWaitMin = time.Millisecond
				client.restRetry.RetryWaitMax = time.Millisecond
			}
			if client.graphqlRetry != nil {
				client.graphqlRetry.RetryWaitMin = time.Millisecond
				client.graphqlRetry.RetryWaitMax = time.Millisecond
			}
			var name string
			switch protocol {
			case "REST":
				if client.restURL != server.URL+"/rest" {
					t.Fatal("acceptance client ignored BUILDKITE_REST_URL")
				}
				var result struct{ Name string }
				if err := client.makeRequest(t.Context(), http.MethodGet, "/organizations/"+client.organization, nil, &result); err != nil {
					t.Fatal(err)
				}
				name = result.Name
			case "GraphQL", "genqlient":
				var query struct {
					Organization struct{ Name graphql.String } `graphql:"organization(slug: $slug)"`
				}
				if protocol == "GraphQL" {
					if client.graphql == nil {
						t.Fatal("acceptance client has no GraphQL client")
					}
					if err := client.graphql.Query(t.Context(), &query, map[string]any{"slug": graphql.ID(client.organization)}); err != nil {
						t.Fatal(err)
					}
				} else {
					if client.genqlient == nil {
						t.Fatal("acceptance client has no genqlient client")
					}
					err := client.genqlient.MakeRequest(t.Context(), &genqlient.Request{Query: `query { organization(slug: "test-org") { name } }`}, &genqlient.Response{Data: &query})
					if err != nil {
						t.Fatal(err)
					}
				}
				name = string(query.Organization.Name)
			}
			if name != "test-org" || requests.Load() != 2 {
				t.Fatalf("name = %q after %d requests, want test-org after retrying the 429", name, requests.Load())
			}
		})
	}
}

func protoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"buildkite": providerserver.NewProtocol6WithError(New("testing")),
	}
}

func TestDestroyChecksRequireNotFound(t *testing.T) {
	checks := []struct {
		resourceType string
		path         string
		check        func(*terraform.State) error
	}{
		{"buildkite_portal", "/v2/organizations/test-org/portals/test", testAccCheckPortalResourceDestroy},
		{"buildkite_cluster_secret", "/v2/organizations/test-org/clusters/cluster/secrets/id-with-404", testAccCheckClusterSecretDestroy},
		{"buildkite_registry", "/v2/packages/organizations/test-org/registries/test", testAccCheckRegistryDestroy},
		{"buildkite_cluster_maintainer", "/v2/organizations/test-org/clusters/cluster/maintainers/id-with-404", testAccCheckClusterMaintainerDestroy},
	}
	for _, check := range checks {
		t.Run(check.resourceType, func(t *testing.T) {
			for _, status := range []int{http.StatusNotFound, http.StatusForbidden, http.StatusOK} {
				t.Run(fmt.Sprint(status), func(t *testing.T) {
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.URL.Path != check.path {
							t.Errorf("request path = %q, want %q", r.URL.Path, check.path)
						}
						w.WriteHeader(status)
						_, _ = w.Write([]byte(`{}`))
					}))
					t.Cleanup(server.Close)
					t.Setenv("BUILDKITE_API_TOKEN", "test-token")
					t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "test-org")
					t.Setenv("BUILDKITE_REST_URL", server.URL)
					s := &terraform.State{Modules: []*terraform.ModuleState{{
						Path: []string{"root"},
						Resources: map[string]*terraform.ResourceState{
							check.resourceType + ".test": {
								Type: check.resourceType,
								Primary: &terraform.InstanceState{ID: "id-with-404", Attributes: map[string]string{
									"slug": "test", "cluster_id": "cluster", "cluster_uuid": "cluster",
								}},
							},
						},
					}}}
					err := check.check(s)
					if (err == nil) != (status == http.StatusNotFound) {
						t.Fatalf("destroy check for HTTP %d = %v; only 404 should confirm deletion", status, err)
					}
					if status == http.StatusOK && !strings.Contains(err.Error(), "still exists") {
						t.Fatalf("existing resource was not reported as still existing: %v", err)
					}
				})
			}
		})
	}
}

func TestGraphQLDestroyChecksRejectFailedReads(t *testing.T) {
	t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "test-org")
	previous := genqlientGraphql
	t.Cleanup(func() { genqlientGraphql = previous })
	checks := []struct {
		resourceType string
		check        func(*terraform.State) error
	}{
		{"buildkite_pipeline", testAccCheckPipelineDestroy},
		{"buildkite_pipeline_template", testAccCheckPipelineTemplateDestroy},
		{"buildkite_organization_rule", testAccCheckOrganizationRuleDestroy},
	}
	for _, check := range checks {
		t.Run(check.resourceType, func(t *testing.T) {
			for _, tc := range []struct {
				name    string
				err     error
				wantErr bool
			}{
				{"absent", gqlerror.List{&gqlerror.Error{Message: "Resource not found"}}, false},
				{"rate limited", &apiError{StatusCode: http.StatusTooManyRequests, Attempts: 11, Body: "upstream not found"}, true},
				{"unavailable", &apiError{StatusCode: http.StatusServiceUnavailable, Attempts: 11, Body: "upstream not found"}, true},
				{"deadline", &apiError{StatusCode: http.StatusServiceUnavailable, Body: "upstream not found", Err: context.DeadlineExceeded}, true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					genqlientGraphql = &fakeGraphQLClient{handler: func(context.Context, *genqlient.Request, *genqlient.Response) error {
						return tc.err
					}}
					s := &terraform.State{Modules: []*terraform.ModuleState{{
						Path: []string{"root"},
						Resources: map[string]*terraform.ResourceState{
							check.resourceType + ".test": {
								Type:    check.resourceType,
								Primary: &terraform.InstanceState{ID: "test", Attributes: map[string]string{"slug": "test"}},
							},
						},
					}}}
					if err := check.check(s); (err != nil) != tc.wantErr {
						t.Fatalf("destroy check = %v, want error: %v", err, tc.wantErr)
					}
				})
			}
		})
	}
}

func TestOrganizationRuleDestroyPreservesReadFixture(t *testing.T) {
	previous := genqlientGraphql
	t.Cleanup(func() { genqlientGraphql = previous })
	for _, managedExists := range []bool{false, true} {
		t.Run(fmt.Sprintf("managed_exists=%t", managedExists), func(t *testing.T) {
			genqlientGraphql = &fakeGraphQLClient{handler: func(_ context.Context, req *genqlient.Request, resp *genqlient.Response) error {
				id := req.Variables.(interface{ GetId() string }).GetId()
				if id == "persistent-fixture" || managedExists {
					resp.Data.(*getNodeResponse).Node = &getNodeNodeRule{}
				}
				return nil
			}}
			s := &terraform.State{Modules: []*terraform.ModuleState{{
				Path: []string{"root"},
				Resources: map[string]*terraform.ResourceState{
					"data.buildkite_organization_rule.fixture": {
						Type:    "buildkite_organization_rule",
						Primary: &terraform.InstanceState{ID: "persistent-fixture"},
					},
					"buildkite_organization_rule.temporary": {
						Type:    "buildkite_organization_rule",
						Primary: &terraform.InstanceState{ID: "temporary-rule"},
					},
				},
			}}}
			if err := testAccCheckOrganizationRuleDestroy(s); (err != nil) != managedExists {
				t.Fatalf("destroy check = %v, managed Rule exists: %t; the persistent data source must be preserved", err, managedExists)
			}
		})
	}
}

func TestClusterQueueDestroyRequiresAbsentNode(t *testing.T) {
	previous := genqlientGraphql
	t.Cleanup(func() { genqlientGraphql = previous })
	for _, tc := range []struct {
		name    string
		exists  bool
		err     error
		wantErr bool
	}{
		{name: "absent"},
		{name: "still exists", exists: true, wantErr: true},
		{name: "forbidden", err: &apiError{StatusCode: http.StatusForbidden}, wantErr: true},
		{name: "deadline", err: context.DeadlineExceeded, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			genqlientGraphql = &fakeGraphQLClient{handler: func(_ context.Context, req *genqlient.Request, resp *genqlient.Response) error {
				id := req.Variables.(interface{ GetId() string }).GetId()
				if id == "persistent-fixture" || tc.exists {
					resp.Data.(*getClusterQueueByNodeResponse).Node = &getClusterQueueByNodeNodeClusterQueue{}
				}
				return tc.err
			}}
			s := &terraform.State{Modules: []*terraform.ModuleState{{
				Path: []string{"root"},
				Resources: map[string]*terraform.ResourceState{
					"data.buildkite_cluster_queue.fixture": {
						Type:    "buildkite_cluster_queue",
						Primary: &terraform.InstanceState{ID: "persistent-fixture"},
					},
					"buildkite_cluster_queue.temporary": {
						Type:    "buildkite_cluster_queue",
						Primary: &terraform.InstanceState{ID: "temporary-queue"},
					},
				},
			}}}
			if err := testAccCheckClusterQueueDestroy(s); (err != nil) != tc.wantErr {
				t.Fatalf("destroy check = %v, want error: %t", err, tc.wantErr)
			}
		})
	}
}

func testAccPreCheck(t *testing.T) {
	if v := getenv("BUILDKITE_ORGANIZATION_SLUG"); v == "" {
		t.Fatal("BUILDKITE_ORGANIZATION_SLUG must be set for acceptance tests")
	}
	if v := os.Getenv("BUILDKITE_API_TOKEN"); v == "" {
		t.Fatal("BUILDKITE_API_TOKEN must be set for acceptance tests")
	}

}

// The same lifecycle tests run in the admin suite. Only the dedicated non-admin
// target requires a MEMBER token, so accidentally supplying an admin cannot make
// that job green without exercising non-admin permissions.
func testAccNonAdminPreCheck(t *testing.T) {
	t.Helper()
	testAccPreCheck(t)
	if os.Getenv("BUILDKITE_ACCEPTANCE_NONADMIN") != "1" {
		return
	}
	testAccMemberPreCheck(t, graphqlClient)
}

func testAccMemberPreCheck(t *testing.T, client *graphql.Client) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var viewer struct {
		Viewer struct {
			User struct{ Email graphql.String }
		}
	}
	if err := client.Query(ctx, &viewer, nil); err != nil {
		t.Fatalf("read non-admin token owner: %v", err)
	}
	var membership struct {
		Organization struct {
			Members struct {
				Edges []struct {
					Node struct{ Role graphql.String }
				}
			} `graphql:"members(first: 1, email: $email)"`
		} `graphql:"organization(slug: $slug)"`
	}
	variables := map[string]any{
		"slug":  graphql.ID(getenv("BUILDKITE_ORGANIZATION_SLUG")),
		"email": viewer.Viewer.User.Email,
	}
	if err := client.Query(ctx, &membership, variables); err != nil {
		t.Fatalf("read non-admin organization membership: %v", err)
	}
	edges := membership.Organization.Members.Edges
	if len(edges) != 1 || edges[0].Node.Role != "MEMBER" {
		t.Fatal("non-admin acceptance tests require a token owned by an organization MEMBER, not an ADMIN")
	}
}

// These fixtures use only GraphQL. In particular, do not configure slug,
// provider_settings, or github_webhooks_enabled: those require REST permissions.
func testAccPipelineConfigWithTeam(name, description string) string {
	teamSlug := os.Getenv("BUILDKITE_NONADMIN_TEAM_SLUG")
	if teamSlug == "" {
		teamSlug = "everyone"
	}
	return fmt.Sprintf(`
		data "buildkite_team" "nonadmin" {
			slug = %q
		}

		resource "buildkite_pipeline" "pipeline" {
			name = %q
			description = %q
			repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
			default_team_id = data.buildkite_team.nonadmin.id
		}
	`, teamSlug, name, description)
}
