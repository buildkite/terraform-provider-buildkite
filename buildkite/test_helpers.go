package buildkite

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

var (
	resourceTrackingMutex sync.Mutex
	resourceTracking      = make(map[string]struct{})
)

func TrackResource(resourceType, resourceID string) {
	resourceTrackingMutex.Lock()
	defer resourceTrackingMutex.Unlock()

	key := fmt.Sprintf("%s:%s", resourceType, resourceID)
	resourceTracking[key] = struct{}{}
}

func UntrackResource(resourceType, resourceID string) {
	resourceTrackingMutex.Lock()
	defer resourceTrackingMutex.Unlock()

	key := fmt.Sprintf("%s:%s", resourceType, resourceID)
	delete(resourceTracking, key)
}

func CleanupResources(t *testing.T) {
	t.Helper()

	resourceTrackingMutex.Lock()
	defer resourceTrackingMutex.Unlock()

	ctx := context.Background()

	if len(resourceTracking) > 0 {
		t.Logf("Cleaning up %d tracked resources that were not automatically destroyed", len(resourceTracking))
	}

	for key := range resourceTracking {
		parts := strings.SplitN(key, ":", 2)
		if len(parts) != 2 {
			continue
		}

		resourceType := parts[0]
		resourceID := parts[1]

		t.Logf("Forcing cleanup of %s with ID: %s", resourceType, resourceID)

		var err error
		switch resourceType {
		case "buildkite_pipeline":
			_, err = deletePipeline(ctx, genqlientGraphql, resourceID)
		case "buildkite_team":
			_, err = teamDelete(ctx, genqlientGraphql, resourceID)
		case "buildkite_team_member":
			_, err = deleteTeamMember(ctx, genqlientGraphql, resourceID)
		case "buildkite_cluster":
			_, err = deleteCluster(ctx, genqlientGraphql, resourceID)
		case "buildkite_test_suite":
			_, err = deleteTestSuite(ctx, genqlientGraphql, resourceID)
		case "buildkite_pipeline_template":
			_, err = deletePipelineTemplate(ctx, genqlientGraphql, resourceID)
		case "buildkite_pipeline_schedule":
			_, err = deletePipelineSchedule(ctx, genqlientGraphql, resourceID)
		case "buildkite_organization_rule":
			_, err = deleteOrganizationRule(ctx, genqlientGraphql, resourceID)
		case "buildkite_cluster_queue":
			_, err = deleteClusterQueue(ctx, genqlientGraphql, resourceID)
		case "buildkite_agent_token":
			_, err = revokeAgentToken(ctx, genqlientGraphql, resourceID, "Revoked by cleanup")
		}

		if err != nil {
			t.Logf("Error cleaning up %s: %v", key, err)
		} else {
			delete(resourceTracking, key)
		}
	}
}

func RegisterResourceTracking(t *testing.T, s *terraform.State, resourceType string) {
	t.Helper()

	t.Cleanup(func() {
		CleanupResources(t)
	})

	for _, rs := range s.RootModule().Resources {
		if rs.Type == resourceType {
			TrackResource(resourceType, rs.Primary.ID)
		}
	}
}

func testAccCheckPipelineDestroyFunc(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_pipeline" {
			continue
		}

		pipelineSlug := rs.Primary.Attributes["slug"]
		if pipelineSlug == "" {
			pipelineName := rs.Primary.Attributes["name"]
			pipelineSlug = fmt.Sprintf("%s/%s", getenv("BUILDKITE_ORGANIZATION_SLUG"), strings.ToLower(pipelineName))
		}

		resp, err := getPipeline(context.Background(), genqlientGraphql, pipelineSlug)
		if err != nil {
			if strings.Contains(err.Error(), "not found") ||
				strings.Contains(err.Error(), "pipeline not found") {
				UntrackResource("buildkite_pipeline", rs.Primary.ID)
				continue
			}
			return fmt.Errorf("error checking if pipeline exists: %v", err)
		}

		if resp.Pipeline.Id != "" {
			return fmt.Errorf("pipeline still exists: %s (ID: %s)", pipelineSlug, resp.Pipeline.Id)
		}

		UntrackResource("buildkite_pipeline", rs.Primary.ID)
	}

	return nil
}

func testAccCheckTestSuiteDestroyFunc(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_test_suite" {
			continue
		}

		testSuiteID := rs.Primary.Attributes["id"]

		suite, err := getTestSuite(context.Background(), genqlientGraphql, testSuiteID, 1)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				UntrackResource("buildkite_test_suite", testSuiteID)
				continue
			}
			return fmt.Errorf("error checking if test suite exists: %v", err)
		}

		if suite.Suite != nil {
			return fmt.Errorf("test suite still exists: %v", suite)
		}

		UntrackResource("buildkite_test_suite", testSuiteID)
	}

	return nil
}

func testAccCheckTeamDestroyFunc(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_team" {
			continue
		}

		teamID := rs.Primary.ID

		r, err := getNode(context.Background(), genqlientGraphql, teamID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				UntrackResource("buildkite_team", teamID)
				continue
			}
			return fmt.Errorf("error checking if team exists: %v", err)
		}

		if teamNode, ok := r.GetNode().(*getNodeNodeTeam); ok {
			if teamNode != nil {
				return fmt.Errorf("team still exists: %v", teamNode)
			}
		}

		UntrackResource("buildkite_team", teamID)
	}

	return nil
}

func testAccCheckClusterDestroyFunc(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_cluster" {
			continue
		}

		clusterID := rs.Primary.ID

		r, err := getNode(context.Background(), genqlientGraphql, clusterID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				UntrackResource("buildkite_cluster", clusterID)
				continue
			}
			return fmt.Errorf("error checking if cluster exists: %v", err)
		}

		if clusterNode, ok := r.GetNode().(*getNodeNodeCluster); ok {
			if clusterNode != nil {
				return fmt.Errorf("cluster still exists: %v", clusterNode)
			}
		}

		UntrackResource("buildkite_cluster", clusterID)
	}

	return nil
}

func testAccCheckPipelineTemplateDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_pipeline_template" {
			continue
		}

		templateID := rs.Primary.ID

		template, err := getPipelineTemplate(context.Background(), genqlientGraphql, templateID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				UntrackResource("buildkite_pipeline_template", templateID)
				continue
			}
			return fmt.Errorf("error checking if pipeline template exists: %v", err)
		}

		if template.PipelineTemplate.Id != "" {
			return fmt.Errorf("pipeline template still exists: %v", template)
		}

		UntrackResource("buildkite_pipeline_template", templateID)
	}

	return nil
}

func testAccCheckOrganizationRuleDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_organization_rule" {
			continue
		}

		ruleID := rs.Primary.ID

		rule, err := getOrganizationRule(context.Background(), genqlientGraphql, ruleID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				UntrackResource("buildkite_organization_rule", ruleID)
				continue
			}
			return fmt.Errorf("error checking if organization rule exists: %v", err)
		}

		if rule.OrganizationRule.Id != "" {
			return fmt.Errorf("organization rule still exists: %v", rule)
		}

		UntrackResource("buildkite_organization_rule", ruleID)
	}

	return nil
}

func testAccCheckPipelineScheduleDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_pipeline_schedule" {
			continue
		}

		scheduleID := rs.Primary.ID

		_, err := getPipelineSchedule(context.Background(), genqlientGraphql, scheduleID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				UntrackResource("buildkite_pipeline_schedule", scheduleID)
				continue
			}
			return fmt.Errorf("error checking if pipeline schedule exists: %v", err)
		}

		return fmt.Errorf("pipeline schedule still exists: %s", scheduleID)
	}

	return nil
}

func testAccCheckAgentTokenDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_agent_token" {
			continue
		}

		tokenID := rs.Primary.ID
		tokenUUID := rs.Primary.Attributes["uuid"]
		organizationSlug := getenv("BUILDKITE_ORGANIZATION_SLUG")

		if tokenUUID != "" {
			_, err := getAgentToken(context.Background(), genqlientGraphql, fmt.Sprintf("%s/%s", organizationSlug, tokenUUID))
			if err != nil {
				if strings.Contains(err.Error(), "not found") {
					UntrackResource("buildkite_agent_token", tokenID)
					continue
				}
				return fmt.Errorf("error checking if agent token exists: %v", err)
			}

			return fmt.Errorf("agent token still exists: %s", tokenID)
		}

		UntrackResource("buildkite_agent_token", tokenID)
	}

	return nil
}

func testAccCheckClusterQueueDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_cluster_queue" {
			continue
		}

		queueID := rs.Primary.ID

		r, err := getNode(context.Background(), genqlientGraphql, queueID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				UntrackResource("buildkite_cluster_queue", queueID)
				continue
			}
			return fmt.Errorf("error checking if cluster queue exists: %v", err)
		}

		if queueNode, ok := r.GetNode().(*getNodeNodeClusterQueue); ok {
			if queueNode != nil {
				return fmt.Errorf("cluster queue still exists: %v", queueNode)
			}
		}

		UntrackResource("buildkite_cluster_queue", queueID)
	}

	return nil
}

func testAccCheckTeamMemberDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_team_member" {
			continue
		}

		teamMemberID := rs.Primary.ID

		r, err := getNode(context.Background(), genqlientGraphql, teamMemberID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				UntrackResource("buildkite_team_member", teamMemberID)
				continue
			}
			return fmt.Errorf("error checking if team member exists: %v", err)
		}

		if teamMemberNode, ok := r.GetNode().(*getNodeNodeTeamMember); ok {
			if teamMemberNode != nil {
				return fmt.Errorf("team member still exists: %v", teamMemberNode)
			}
		}

		UntrackResource("buildkite_team_member", teamMemberID)
	}

	return nil
}
