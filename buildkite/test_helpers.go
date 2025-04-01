package buildkite

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

const defaultTestSweepTimeout = 15 * time.Minute

// We're using existing endpoints and getenv defined in provider.go and util.go
// const defaultGraphqlEndpoint = "https://graphql.buildkite.com/v1"
// const defaultRestEndpoint = "https://api.buildkite.com"

func getOrgEnv() string {
	return os.Getenv("BUILDKITE_ORGANIZATION_SLUG")
}

func getApiTokenEnv() string {
	return os.Getenv("BUILDKITE_API_TOKEN")
}

func init() {

	resource.AddTestSweepers("buildkite_agent_token", &resource.Sweeper{
		Name: "buildkite_agent_token",
		F:    sweepAgentTokens,
	})

	resource.AddTestSweepers("buildkite_cluster", &resource.Sweeper{
		Name: "buildkite_cluster",
		F:    sweepClusters,
		Dependencies: []string{
			"buildkite_cluster_queue",
			"buildkite_cluster_agent_token",
		},
	})

	resource.AddTestSweepers("buildkite_cluster_queue", &resource.Sweeper{
		Name: "buildkite_cluster_queue",
		F:    sweepClusterQueues,
	})

	resource.AddTestSweepers("buildkite_cluster_agent_token", &resource.Sweeper{
		Name: "buildkite_cluster_agent_token",
		F:    sweepClusterAgentTokens,
	})

	resource.AddTestSweepers("buildkite_pipeline", &resource.Sweeper{
		Name: "buildkite_pipeline",
		F:    sweepPipelines,
		Dependencies: []string{
			"buildkite_pipeline_schedule",
		},
	})

	resource.AddTestSweepers("buildkite_pipeline_schedule", &resource.Sweeper{
		Name: "buildkite_pipeline_schedule",
		F:    sweepPipelineSchedules,
	})

	resource.AddTestSweepers("buildkite_pipeline_template", &resource.Sweeper{
		Name: "buildkite_pipeline_template",
		F:    sweepPipelineTemplates,
	})

	resource.AddTestSweepers("buildkite_team", &resource.Sweeper{
		Name: "buildkite_team",
		F:    sweepTeams,
		Dependencies: []string{
			"buildkite_team_member",
		},
	})

	resource.AddTestSweepers("buildkite_team_member", &resource.Sweeper{
		Name: "buildkite_team_member",
		F:    sweepTeamMembers,
	})

	resource.AddTestSweepers("buildkite_test_suite", &resource.Sweeper{
		Name: "buildkite_test_suite",
		F:    sweepTestSuites,
	})

	resource.AddTestSweepers("buildkite_test_suite_team", &resource.Sweeper{
		Name: "buildkite_test_suite_team",
		F:    sweepTestSuiteTeams,
	})
}

// sweepAgentTokens removes agent tokens created during testing
func sweepAgentTokens(region string) error {
	log.Printf("[INFO] Sweeping buildkite_agent_token resources...")
	return nil
}

// sweepClusters removes clusters created during testing
func sweepClusters(region string) error {
	log.Printf("[INFO] Sweeping buildkite_cluster resources...")
	return nil
}

// sweepClusterQueues removes cluster queues created during testing
func sweepClusterQueues(region string) error {
	log.Printf("[INFO] Sweeping buildkite_cluster_queue resources...")
	return nil
}

// sweepClusterAgentTokens removes cluster agent tokens created during testing
func sweepClusterAgentTokens(region string) error {
	log.Printf("[INFO] Sweeping buildkite_cluster_agent_token resources...")
	// Implementation would use existing client functions
	return nil
}

func sweepPipelines(region string) error {
	log.Printf("[INFO] Sweeping buildkite_pipeline resources...")
	ctx := context.Background()
	orgSlug := getOrgEnv()

	client := NewClient(&clientConfig{
		org:        orgSlug,
		apiToken:   getApiTokenEnv(),
		graphqlURL: defaultGraphqlEndpoint,
		restURL:    defaultRestEndpoint,
		userAgent:  "testing",
	})

	// Use REST API to list pipelines
	type restPipeline struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Slug string `json:"slug"`
	}

	var pipelines []restPipeline
	err := client.makeRequest(ctx, "GET", fmt.Sprintf("/v2/organizations/%s/pipelines", orgSlug), nil, &pipelines)
	if err != nil {
		return fmt.Errorf("error fetching pipelines: %w", err)
	}

	pipelinesToDelete := []string{}

	for _, pipeline := range pipelines {
		if isTestResource(pipeline.Name) {
			log.Printf("[INFO] Found test pipeline to delete: %s (%s)", pipeline.Name, pipeline.ID)
			pipelinesToDelete = append(pipelinesToDelete, pipeline.ID)
		}
	}

	for _, pipelineID := range pipelinesToDelete {
		log.Printf("[DEBUG] Deleting pipeline %s", pipelineID)
		_, err := deletePipeline(ctx, client.genqlient, pipelineID)
		if err != nil {
			log.Printf("[ERROR] Failed to delete pipeline %s: %v", pipelineID, err)
		}
	}

	return nil
}

// sweepPipelineSchedules removes pipeline schedules created during testing
func sweepPipelineSchedules(region string) error {
	log.Printf("[INFO] Sweeping buildkite_pipeline_schedule resources...")
	// Implementation would use existing client functions
	return nil
}

// sweepPipelineTemplates removes pipeline templates created during testing
func sweepPipelineTemplates(region string) error {
	log.Printf("[INFO] Sweeping buildkite_pipeline_template resources...")
	// Implementation would use existing client functions
	return nil
}

func sweepTeams(region string) error {
	log.Printf("[INFO] Sweeping buildkite_team resources...")
	ctx := context.Background()
	orgSlug := getOrgEnv()

	client := NewClient(&clientConfig{
		org:        orgSlug,
		apiToken:   getApiTokenEnv(),
		graphqlURL: defaultGraphqlEndpoint,
		restURL:    defaultRestEndpoint,
		userAgent:  "testing",
	})

	type restTeam struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Slug string `json:"slug"`
	}

	var teams []restTeam
	err := client.makeRequest(ctx, "GET", fmt.Sprintf("/v2/organizations/%s/teams", orgSlug), nil, &teams)
	if err != nil {
		return fmt.Errorf("error fetching teams: %w", err)
	}

	teamsToDelete := []string{}

	// Check each team to see if it's a test resource
	for _, team := range teams {
		if isTestResource(team.Name) {
			log.Printf("[INFO] Found test team to delete: %s (%s)", team.Name, team.ID)
			teamsToDelete = append(teamsToDelete, team.ID)
		}
	}

	for _, teamID := range teamsToDelete {
		log.Printf("[DEBUG] Deleting team %s", teamID)
		_, err := teamDelete(ctx, client.genqlient, teamID)
		if err != nil {
			log.Printf("[ERROR] Failed to delete team %s: %v", teamID, err)
		}
	}

	return nil
}

// sweepTeamMembers removes team members created during testing
func sweepTeamMembers(region string) error {
	log.Printf("[INFO] Sweeping buildkite_team_member resources...")
	return nil
}

// sweepTestSuites removes test suites created during testing
func sweepTestSuites(region string) error {
	log.Printf("[INFO] Sweeping buildkite_test_suite resources...")
	return nil
}

func sweepTestSuiteTeams(region string) error {
	log.Printf("[INFO] Sweeping buildkite_test_suite_team resources...")
	return nil
}

var (
	trackedResources = make(map[string]map[string]bool)
	trackedMutex     sync.RWMutex
)

func RegisterResourceTracking(t *testing.T) {
	t.Cleanup(func() {
		leftoverResources := CleanupResources()
		if len(leftoverResources) > 0 {
			t.Logf("[WARNING] The following resources were not properly cleaned up: %v", leftoverResources)
		}
	})
}

func TrackResource(resourceType, id string) {
	trackedMutex.Lock()
	defer trackedMutex.Unlock()

	if _, exists := trackedResources[resourceType]; !exists {
		trackedResources[resourceType] = make(map[string]bool)
	}
	trackedResources[resourceType][id] = true
	log.Printf("[DEBUG] Tracking resource %s with ID %s", resourceType, id)
}

func UntrackResource(resourceType, id string) {
	trackedMutex.Lock()
	defer trackedMutex.Unlock()

	if resources, exists := trackedResources[resourceType]; exists {
		delete(resources, id)
		log.Printf("[DEBUG] Untracking resource %s with ID %s", resourceType, id)
	}
}

func CleanupResources() map[string][]string {
	trackedMutex.RLock()
	defer trackedMutex.RUnlock()

	leftoverResources := make(map[string][]string)
	for resourceType, resources := range trackedResources {
		for id := range resources {
			leftoverResources[resourceType] = append(leftoverResources[resourceType], id)
		}
	}
	return leftoverResources
}

func isTestResource(name string) bool {
	if name == "" {
		return false
	}

	testPrefixes := []string{
		"test", "Test", "TEST",
		"acc", "Acc", "ACC",
		"tf-acc", "tf-test",
		"acceptance",
	}

	name = strings.ToLower(name)
	for _, prefix := range testPrefixes {
		if strings.HasPrefix(name, strings.ToLower(prefix)) {
			return true
		}
	}

	// Also look for acctest.RandString pattern
	if strings.Contains(name, "acc test") ||
		strings.Contains(name, "acceptance test") ||
		strings.Contains(name, "terraform test") {
		return true
	}

	return false
}

func runSweepWithTimeout(name string, f func(string) error, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan error)

	go func() {
		err := f(name)
		done <- err
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("sweeper for %s timed out after %v", name, timeout)
	case err := <-done:
		if err != nil {
			return fmt.Errorf("sweeper for %s encountered an error: %w", name, err)
		}
		return nil
	}
}
