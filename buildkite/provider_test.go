package buildkite

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	genqlient "github.com/Khan/genqlient/graphql"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/shurcooL/graphql"
)

var graphqlClient *graphql.Client
var genqlientGraphql genqlient.Client
var organizationID string
var testResourcePrefix string

const defaultApiEndpoint = "https://api.buildkite.com"

// Helper function to determine if a resource name matches our test patterns
func isTestResource(name string) bool {
	return strings.HasPrefix(name, testResourcePrefix) || // Current test run resources
		strings.HasPrefix(name, "tfacc-") || // Previous test run resources
		strings.Contains(name, "test") || // Generic test resources
		strings.Contains(name, "Test") ||
		strings.Contains(name, "-team") ||
		strings.Contains(name, "cluster")
}

// TestMain runs before all tests in the package and sets up global fixtures
func TestMain(m *testing.M) {
	// Run all tests
	exitCode := m.Run()

	// Execute cleanup after all tests have completed
	log.Printf("[INFO] Running final cleanup from TestMain")
	cleanupTestResources(nil)

	// Exit with the status code from the tests
	os.Exit(exitCode)
}

func init() {
	rt := http.DefaultTransport
	header := make(http.Header)
	header.Set("Authorization", "Bearer "+os.Getenv("BUILDKITE_API_TOKEN"))
	header.Set("User-Agent", "testing")
	rt = newHeaderRoundTripper(rt, header)

	httpClient := &http.Client{
		Transport: rt,
	}

	graphqlClient = graphql.NewClient(defaultGraphqlEndpoint, httpClient)
	genqlientGraphql = genqlient.NewClient(defaultGraphqlEndpoint, httpClient)
	organizationID, _ = GetOrganizationID(getenv("BUILDKITE_ORGANIZATION_SLUG"), graphqlClient)

	testResourcePrefix = fmt.Sprintf("tfacc-%d-", time.Now().Unix())
}

func protoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"buildkite": providerserver.NewProtocol6WithError(New("testing")),
	}
}

func testAccPreCheck(t *testing.T) {
	if v := getenv("BUILDKITE_ORGANIZATION_SLUG"); v == "" {
		t.Fatal("BUILDKITE_ORGANIZATION_SLUG must be set for acceptance tests")
	}
	if v := os.Getenv("BUILDKITE_API_TOKEN"); v == "" {
		t.Fatal("BUILDKITE_API_TOKEN must be set for acceptance tests")
	}
	// Cleanup will happen automatically after all tests in TestMain
}

type ResourceCounts struct {
	Suites    int
	Pipelines int
	Teams     int
	Clusters  int
}

func (r ResourceCounts) TotalCount() int {
	return r.Suites + r.Pipelines + r.Teams + r.Clusters
}

func countTestResources(ctx context.Context) ResourceCounts {
	var counts ResourceCounts
	orgSlug := getenv("BUILDKITE_ORGANIZATION_SLUG")

	// Count test suites
	var getTestSuitesQuery struct {
		Organization struct {
			TestSuites struct {
				Edges []struct {
					Node struct {
						Name string
					}
				}
			} `graphql:"testSuites(first: 100)"`
		} `graphql:"organization(slug: $slug)"`
	}

	vars := map[string]interface{}{
		"slug": orgSlug,
	}

	if err := graphqlClient.Query(ctx, &getTestSuitesQuery, vars); err == nil {
		for _, edge := range getTestSuitesQuery.Organization.TestSuites.Edges {
			if isTestResource(edge.Node.Name) {
				counts.Suites++
			}
		}
	} else {
		log.Printf("[WARN] Failed to count test suites: %v", err)
	}

	var getPipelinesQuery struct {
		Organization struct {
			Pipelines struct {
				Edges []struct {
					Node struct {
						Name string
					}
				}
			} `graphql:"pipelines(first: 100)"`
		} `graphql:"organization(slug: $slug)"`
	}

	if err := graphqlClient.Query(ctx, &getPipelinesQuery, vars); err == nil {
		for _, edge := range getPipelinesQuery.Organization.Pipelines.Edges {
			if isTestResource(edge.Node.Name) {
				counts.Pipelines++
			}
		}
	} else {
		log.Printf("[WARN] Failed to count pipelines: %v", err)
	}

	var getTeamsQuery struct {
		Organization struct {
			Teams struct {
				Edges []struct {
					Node struct {
						Name string
					}
				}
			} `graphql:"teams(first: 100)"`
		} `graphql:"organization(slug: $slug)"`
	}

	if err := graphqlClient.Query(ctx, &getTeamsQuery, vars); err == nil {
		for _, edge := range getTeamsQuery.Organization.Teams.Edges {
			if isTestResource(edge.Node.Name) ||
				strings.HasPrefix(edge.Node.Name, "a team") ||
				strings.HasPrefix(edge.Node.Name, "b team") ||
				strings.HasPrefix(edge.Node.Name, "acc_tests") {
				counts.Teams++
			}
		}
	} else {
		log.Printf("[WARN] Failed to count teams: %v", err)
	}

	var getClustersQuery struct {
		Organization struct {
			Clusters struct {
				Edges []struct {
					Node struct {
						Name string
					}
				}
			} `graphql:"clusters(first: 100)"`
		} `graphql:"organization(slug: $slug)"`
	}

	if err := graphqlClient.Query(ctx, &getClustersQuery, vars); err == nil {
		for _, edge := range getClustersQuery.Organization.Clusters.Edges {
			if isTestResource(edge.Node.Name) {
				counts.Clusters++
			}
		}
	} else {
		log.Printf("[WARN] Failed to count clusters: %v", err)
	}

	return counts
}

func cleanupTestResources(t *testing.T) {
	ctx := context.Background()
	orgSlug := getenv("BUILDKITE_ORGANIZATION_SLUG")

	// Set a timeout for the cleanup
	cleanupTimeout := 5 * time.Minute
	ctx, cancel := context.WithTimeout(ctx, cleanupTimeout)
	defer cancel()

	// First, scan and count all test resources before cleanup
	beforeCounts := countTestResources(ctx)

	fmt.Println("\n============================================================")
	fmt.Println("CLEANUP VERIFICATION: STARTING")
	fmt.Printf("Test resources before cleanup: %+v\n", beforeCounts)

	// Clean up in reverse dependency order
	cleanupTestSuites(ctx, t)
	cleanupTestPipelines(ctx, t, orgSlug)
	cleanupTestTeams(ctx, t)
	cleanupTestClusters(ctx, t)

	afterCounts := countTestResources(ctx)

	cleanedUp := ResourceCounts{
		Suites:    beforeCounts.Suites - afterCounts.Suites,
		Pipelines: beforeCounts.Pipelines - afterCounts.Pipelines,
		Teams:     beforeCounts.Teams - afterCounts.Teams,
		Clusters:  beforeCounts.Clusters - afterCounts.Clusters,
	}

	fmt.Println("============================================================")
	fmt.Println("CLEANUP VERIFICATION RESULTS:")
	fmt.Printf("Resources before cleanup: %+v\n", beforeCounts)
	fmt.Printf("Resources after cleanup:  %+v\n", afterCounts)
	fmt.Printf("Resources cleaned up:     %d suites, %d pipelines, %d teams, %d clusters\n",
		cleanedUp.Suites, cleanedUp.Pipelines, cleanedUp.Teams, cleanedUp.Clusters)

	if afterCounts.TotalCount() > 0 {
		fmt.Println("\n⚠️  WARNING: SOME TEST RESOURCES WERE NOT CLEANED UP!")
		if afterCounts.Suites > 0 {
			fmt.Printf("- %d test suites remain\n", afterCounts.Suites)
		}
		if afterCounts.Pipelines > 0 {
			fmt.Printf("- %d pipelines remain\n", afterCounts.Pipelines)
		}
		if afterCounts.Teams > 0 {
			fmt.Printf("- %d teams remain\n", afterCounts.Teams)
		}
		if afterCounts.Clusters > 0 {
			fmt.Printf("- %d clusters remain\n", afterCounts.Clusters)
		}
	} else {
		fmt.Println("\n✅ SUCCESS: ALL TEST RESOURCES WERE CLEANED UP SUCCESSFULLY")
	}
	fmt.Println("============================================================")
}

func testSuiteDelete(ctx context.Context, client genqlient.Client, id string) (*struct{}, error) {
	r, err := getTestSuite(ctx, client, id, 1)
	if err != nil {
		return nil, fmt.Errorf("failed to get test suite details: %v", err)
	}

	var slug string
	if suite, ok := r.Suite.(*getTestSuiteSuite); ok {
		slug = suite.Slug
	} else {
		return nil, fmt.Errorf("test suite not found or invalid response")
	}

	url := fmt.Sprintf("/v2/analytics/organizations/%s/suites/%s", getenv("BUILDKITE_ORGANIZATION_SLUG"), slug)
	apiEndpoint := fmt.Sprintf("%s%s", defaultApiEndpoint, url)

	req, err := http.NewRequestWithContext(ctx, "DELETE", apiEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create delete request: %v", err)
	}

	// Set up HTTP client with auth headers (same as we do elsewhere)
	rt := http.DefaultTransport
	header := make(http.Header)
	header.Set("Authorization", "Bearer "+os.Getenv("BUILDKITE_API_TOKEN"))
	rt = newHeaderRoundTripper(rt, header)

	httpClient := &http.Client{
		Transport: rt,
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute delete request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("delete request failed with status code %d", resp.StatusCode)
	}

	return &struct{}{}, nil
}

// cleanupTestSuites cleans up test suites created during acceptance tests
func cleanupTestSuites(ctx context.Context, t *testing.T) {
	// Query for test suites that match our test naming pattern
	var getTestSuitesQuery struct {
		Organization struct {
			TestSuites struct {
				Edges []struct {
					Node struct {
						ID   string
						Name string
					}
				}
			} `graphql:"testSuites(first: 100)"`
		} `graphql:"organization(slug: $slug)"`
	}

	vars := map[string]interface{}{
		"slug": getenv("BUILDKITE_ORGANIZATION_SLUG"),
	}

	err := graphqlClient.Query(ctx, &getTestSuitesQuery, vars)
	if err != nil {
		log.Printf("[ERROR] Failed to fetch test suites for cleanup: %v", err)
		return
	}

	// Delete any test suites that look like they were created by tests
	for _, edge := range getTestSuitesQuery.Organization.TestSuites.Edges {
		testSuite := edge.Node
		if isTestResource(testSuite.Name) {
			log.Printf("[INFO] Cleaning up test suite: %s (%s)", testSuite.Name, testSuite.ID)
			_, err := testSuiteDelete(ctx, genqlientGraphql, testSuite.ID)
			if err != nil {
				log.Printf("[ERROR] Failed to delete test suite %s: %v", testSuite.Name, err)
			}
		}
	}
}

// cleanupTestPipelines cleans up test pipelines created during acceptance tests
func cleanupTestPipelines(ctx context.Context, t *testing.T, orgSlug string) {
	var getPipelinesQuery struct {
		Organization struct {
			Pipelines struct {
				Edges []struct {
					Node struct {
						ID   string
						Name string
						Slug string
					}
				}
			} `graphql:"pipelines(first: 100)"`
		} `graphql:"organization(slug: $slug)"`
	}

	vars := map[string]interface{}{
		"slug": orgSlug,
	}

	err := graphqlClient.Query(ctx, &getPipelinesQuery, vars)
	if err != nil {
		log.Printf("[ERROR] Failed to fetch pipelines for cleanup: %v", err)
		return
	}

	// Delete any pipelines that look like they were created by tests
	for _, edge := range getPipelinesQuery.Organization.Pipelines.Edges {
		pipeline := edge.Node
		if isTestResource(pipeline.Name) {
			log.Printf("[INFO] Cleaning up test pipeline: %s (%s)", pipeline.Name, pipeline.ID)
			_, err := deletePipeline(ctx, genqlientGraphql, pipeline.ID)
			if err != nil {
				log.Printf("[ERROR] Failed to delete pipeline %s: %v", pipeline.Name, err)
			}
		}
	}
}

func cleanupTestTeams(ctx context.Context, t *testing.T) {
	var getTeamsQuery struct {
		Organization struct {
			Teams struct {
				Edges []struct {
					Node struct {
						ID   string
						Name string
					}
				}
			} `graphql:"teams(first: 100)"`
		} `graphql:"organization(slug: $slug)"`
	}

	vars := map[string]interface{}{
		"slug": getenv("BUILDKITE_ORGANIZATION_SLUG"),
	}

	err := graphqlClient.Query(ctx, &getTeamsQuery, vars)
	if err != nil {
		log.Printf("[ERROR] Failed to fetch teams for cleanup: %v", err)
		return
	}

	for _, edge := range getTeamsQuery.Organization.Teams.Edges {
		team := edge.Node
		if isTestResource(team.Name) ||
			strings.HasPrefix(team.Name, "a team") ||
			strings.HasPrefix(team.Name, "b team") ||
			strings.HasPrefix(team.Name, "acc_tests") {
			log.Printf("[INFO] Cleaning up test team: %s (%s)", team.Name, team.ID)
			_, err := teamDelete(ctx, genqlientGraphql, team.ID)
			if err != nil {
				log.Printf("[ERROR] Failed to delete team %s: %v", team.Name, err)
			}
		}
	}
}

// cleanupTestClusters cleans up test clusters created during acceptance tests
func cleanupTestClusters(ctx context.Context, t *testing.T) {
	var getClustersQuery struct {
		Organization struct {
			Clusters struct {
				Edges []struct {
					Node struct {
						ID   string
						Name string
					}
				}
			} `graphql:"clusters(first: 100)"`
		} `graphql:"organization(slug: $slug)"`
	}

	vars := map[string]interface{}{
		"slug": getenv("BUILDKITE_ORGANIZATION_SLUG"),
	}

	err := graphqlClient.Query(ctx, &getClustersQuery, vars)
	if err != nil {
		log.Printf("[ERROR] Failed to fetch clusters for cleanup: %v", err)
		return
	}

	for _, edge := range getClustersQuery.Organization.Clusters.Edges {
		cluster := edge.Node
		if isTestResource(cluster.Name) {
			log.Printf("[INFO] Cleaning up test cluster: %s (%s)", cluster.Name, cluster.ID)
			_, err := deleteCluster(ctx, genqlientGraphql, organizationID, cluster.ID)
			if err != nil {
				log.Printf("[ERROR] Failed to delete cluster %s: %v", cluster.Name, err)
			}
		}
	}
}
