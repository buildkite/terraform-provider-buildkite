package buildkite

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	khanGraphQL "github.com/Khan/genqlient/graphql"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

type SweeperClusterNode struct {
	Id   string
	Name string
}

// sweeperAgentTokenNode is used to represent an agent token in the sweeper
// It mirrors the fields needed from the GraphQL `AgentTokenEdge.Node`.
// Note: RevokedAt is a pointer to a string to handle null values from GraphQL.
// If it's nil or an empty string, the token is considered not revoked.
type sweeperAgentTokenNode struct {
	ID          string
	UUID        string
	Description *string
	RevokedAt   *string
}

const listOrgAgentTokensQuery = `
query ListOrgAgentTokens($orgSlug: String!, $first: Int!, $after: String) {
  organization(slug: $orgSlug) {
    agentTokens(first: $first, after: $after, revoked: false) { # Querying only non-revoked tokens might be more efficient
      edges {
        node {
          id
          uuid
          description
          revokedAt
        }
      }
      pageInfo {
        hasNextPage
        endCursor
      }
    }
  }
}
`

// listOrgAgentTokensInternal fetches all agent tokens for a given organization slug.
// It handles pagination and returns a slice of sweeperAgentTokenNode.
func listOrgAgentTokensInternal(ctx context.Context, client khanGraphQL.Client, orgSlug string) ([]*sweeperAgentTokenNode, error) {
	log.Printf("[DEBUG] Fetching agent tokens for organization slug: %s", orgSlug)

	var allTokens []*sweeperAgentTokenNode
	var endCursor *string
	const pageSize = 100

	type GraphQLAgentTokenNode struct {
		ID          string  `json:"id"`
		UUID        string  `json:"uuid"`
		Description *string `json:"description"`
		RevokedAt   *string `json:"revokedAt"`
	}
	type ListOrgAgentTokensResponse struct {
		Organization struct {
			AgentTokens struct {
				Edges []struct {
					Node GraphQLAgentTokenNode `json:"node"`
				} `json:"edges"`
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
			} `json:"agentTokens"`
		} `json:"organization"`
	}

	for {
		vars := map[string]interface{}{
			"orgSlug": orgSlug,
			"first":   pageSize,
			"after":   endCursor,
		}

		req := &khanGraphQL.Request{
			Query:     listOrgAgentTokensQuery,
			Variables: vars,
		}
		respData := &ListOrgAgentTokensResponse{}

		if err := client.MakeRequest(ctx, req, &khanGraphQL.Response{Data: respData}); err != nil {
			return nil, fmt.Errorf("failed to query organization agent tokens for orgSlug %s: %w", orgSlug, err)
		}

		if respData.Organization.AgentTokens.Edges == nil {
			log.Printf("[DEBUG] No agent token edges found for orgSlug %s, or organization not found/accessible.", orgSlug)
			break
		}

		for _, edge := range respData.Organization.AgentTokens.Edges {
			allTokens = append(allTokens, &sweeperAgentTokenNode{
				ID:          edge.Node.ID,
				UUID:        edge.Node.UUID,
				Description: edge.Node.Description,
				RevokedAt:   edge.Node.RevokedAt,
			})
		}

		if !respData.Organization.AgentTokens.PageInfo.HasNextPage || respData.Organization.AgentTokens.PageInfo.EndCursor == "" {
			break
		}
		tempCursor := respData.Organization.AgentTokens.PageInfo.EndCursor
		endCursor = &tempCursor
		log.Printf("[DEBUG] Paginating agent token list for orgSlug %s, next cursor: %s", orgSlug, *endCursor)
	}

	log.Printf("[INFO] Found %d agent tokens in organization %s for sweeping.", len(allTokens), orgSlug)
	return allTokens, nil
}

type sweeperPipelineScheduleNode struct {
	ID    string
	UUID  string
	Label string
}

type sweeperPipelineInfoForScheduleSweeping struct {
	ID        string
	Name      string
	Schedules []*sweeperPipelineScheduleNode
}

const deletePipelineScheduleMutation = `
mutation DeletePipelineSchedule($scheduleID: ID!) {
  pipelineScheduleDelete(input: {id: $scheduleID}) {
    clientMutationId
  }
}
`

type sweeperUserNode struct {
	ID    string
	Name  string
	Email string
}

type sweeperTeamMemberNode struct {
	ID   string
	Role string
	User sweeperUserNode
}

type sweeperTeamInfoForMemberSweeping struct {
	ID      string
	Name    string
	Members []*sweeperTeamMemberNode
}

const listOrgTeamsAndMembersQuery = `
query ListOrgTeamsAndMembers($orgSlug: String!, $firstTeams: Int!, $afterTeam: String, $firstMembers: Int!, $afterMember: String) {
  organization(slug: $orgSlug) {
    teams(first: $firstTeams, after: $afterTeam) {
      pageInfo {
        hasNextPage
        endCursor
      }
      edges {
        node {
          id
          name
          members(first: $firstMembers, after: $afterMember) {
            pageInfo {
              hasNextPage
              endCursor
            }
            edges {
              node {
                id
                role
                user {
                  id
                  name
                  email
                }
              }
            }
          }
        }
      }
    }
  }
}
`

const listMembersForTeamQuery = `
query ListMembersForTeam($teamID: ID!, $firstMembers: Int!, $afterMember: String) {
  node(id: $teamID) {
    ... on Team {
      members(first: $firstMembers, after: $afterMember) {
        pageInfo {
          hasNextPage
          endCursor
        }
        edges {
          node {
            id
            role
            user {
              id
              name
              email
            }
          }
        }
      }
    }
  }
}
`

// sweeperTestSuiteRESTNode represents a test suite from the REST API for sweeping
type sweeperTestSuiteRESTNode struct {
	ID            string `json:"id"`
	GraphqlID     string `json:"graphql_id"`
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	URL           string `json:"url"`
	WebURL        string `json:"web_url"`
	DefaultBranch string `json:"default_branch"`
}

// sweeperTeamRESTNode represents a team from the REST API for sweeping
type sweeperTeamRESTNode struct {
	ID   string `json:"id"` // This is the Team UUID
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// sweeperTeamSuiteLinkRESTNode represents a link between a team and a test suite from the REST API
type sweeperTeamSuiteLinkRESTNode struct {
	SuiteID     string   `json:"suite_id"` // This is the Suite UUID
	AccessLevel []string `json:"access_level"`
	CreatedAt   string   `json:"created_at"`
	SuiteURL    string   `json:"suite_url"`
}

const deleteTeamMemberMutation = `
mutation DeleteTeamMember($teamMemberID: ID!) {
  teamMemberDelete(input: {id: $teamMemberID}) {
    clientMutationId
  }
}
`

const listOrgPipelinesAndSchedulesQuery = `
query ListOrgPipelinesAndSchedules($orgSlug: String!, $firstPipelines: Int!, $afterPipeline: String, $firstSchedules: Int!, $afterSchedule: String) {
  organization(slug: $orgSlug) {
    pipelines(first: $firstPipelines, after: $afterPipeline) {
      pageInfo {
        hasNextPage
        endCursor
      }
      edges {
        node {
          id
          name
          schedules(first: $firstSchedules, after: $afterSchedule) {
            pageInfo {
              hasNextPage
              endCursor
            }
            edges {
              node {
                id
                uuid
                label
              }
            }
          }
        }
      }
    }
  }
}
`

const revokeAgentTokenMutation = `
mutation RevokeAgentToken($tokenID: ID!, $reason: String!) {
  agentTokenRevoke(input: {id: $tokenID, reason: $reason}) {
    agentToken {
      id
      revokedAt
    }
    clientMutationId
  }
}
`

// revokeAgentTokenInternal revokes an agent token given its GraphQL ID.
func revokeAgentTokenInternal(ctx context.Context, client khanGraphQL.Client, tokenID string, reason string) error {
	log.Printf("[DEBUG] Revoking agent token ID: %s with reason: %s", tokenID, reason)

	type RevokeAgentTokenResponse struct {
		AgentTokenRevoke struct {
			AgentToken struct {
				ID        string  `json:"id"`
				RevokedAt *string `json:"revokedAt"`
			} `json:"agentToken"`
			ClientMutationID *string `json:"clientMutationId"`
		} `json:"agentTokenRevoke"`
	}

	vars := map[string]interface{}{
		"tokenID": tokenID,
		"reason":  reason,
	}

	req := &khanGraphQL.Request{
		Query:     revokeAgentTokenMutation,
		Variables: vars,
	}
	respData := &RevokeAgentTokenResponse{}

	if err := client.MakeRequest(ctx, req, &khanGraphQL.Response{Data: respData}); err != nil {
		return fmt.Errorf("failed to revoke agent token ID %s: %w", tokenID, err)
	}

	if respData.AgentTokenRevoke.AgentToken.ID == "" {
		return fmt.Errorf("failed to revoke agent token ID %s, received empty token ID in response", tokenID)
	}

	log.Printf("[INFO] Successfully revoked agent token ID: %s. Revoked at: %v", tokenID, respData.AgentTokenRevoke.AgentToken.RevokedAt)
	return nil
}

func listOrgClusters(ctx context.Context, client khanGraphQL.Client, orgID string) ([]*SweeperClusterNode, error) {
	log.Printf("[DEBUG] Fetching clusters for organization GraphQL ID: %s", orgID)

	var allClusters []*SweeperClusterNode
	var endCursor *string
	const pageSize = 100

	const listOrgClustersQuery = `
		query ListOrgClustersQuery($orgID: ID!, $first: Int!, $after: String) {
			organization(id: $orgID) {
				clusters(first: $first, after: $after) {
					edges {
						node {
							id
							name
						}
					}
					pageInfo {
						hasNextPage
						endCursor
					}
				}
			}
		}
	`

	type GraphQLClusterNode struct {
		Id   string `json:"id"`
		Name string `json:"name"`
	}
	type ListOrgClustersResponse struct {
		Organization struct {
			Clusters struct {
				Edges []struct {
					Node GraphQLClusterNode `json:"node"`
				} `json:"edges"`
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
			} `json:"clusters"`
		} `json:"organization"`
	}

	for {
		vars := map[string]interface{}{
			"orgID": orgID,
			"first": pageSize,
			"after": endCursor,
		}

		req := &khanGraphQL.Request{
			Query:     listOrgClustersQuery,
			Variables: vars,
		}
		respData := &ListOrgClustersResponse{}

		if err := client.MakeRequest(ctx, req, &khanGraphQL.Response{Data: respData}); err != nil {
			return nil, fmt.Errorf("failed to query organization clusters for orgID %s: %w", orgID, err)
		}

		if respData.Organization.Clusters.Edges == nil {
			log.Printf("[DEBUG] No cluster edges found for orgID %s, or organization not found/accessible.", orgID)
			break
		}

		for _, edge := range respData.Organization.Clusters.Edges {
			allClusters = append(allClusters, &SweeperClusterNode{
				Id:   edge.Node.Id,
				Name: edge.Node.Name,
			})
		}

		if !respData.Organization.Clusters.PageInfo.HasNextPage || respData.Organization.Clusters.PageInfo.EndCursor == "" {
			break
		}
		tempCursor := respData.Organization.Clusters.PageInfo.EndCursor
		endCursor = &tempCursor
		log.Printf("[DEBUG] Paginating cluster list for orgID %s, next cursor: %s", orgID, *endCursor)
	}

	log.Printf("[INFO] Found %d clusters in organization %s for sweeping.", len(allClusters), orgID)
	return allClusters, nil
}

func getOrgEnv() string {
	return os.Getenv("BUILDKITE_ORGANIZATION_SLUG")
}

// We're using existing endpoints and getenv defined in provider.go and util.go
// const defaultGraphqlEndpoint = "https://graphql.buildkite.com/v1"
// const defaultRestEndpoint = "https://api.buildkite.com"

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
			"buildkite_pipeline_team",
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
			"buildkite_test_suite_team",
			"buildkite_pipeline_team",
		},
	})

	resource.AddTestSweepers("buildkite_team_member", &resource.Sweeper{
		Name: "buildkite_team_member",
		F:    sweepTeamMembers,
	})

	resource.AddTestSweepers("buildkite_test_suite", &resource.Sweeper{
		Name: "buildkite_test_suite",
		F:    sweepTestSuites,
		Dependencies: []string{
			"buildkite_test_suite_team",
		},
	})

	resource.AddTestSweepers("buildkite_test_suite_team", &resource.Sweeper{
		Name: "buildkite_test_suite_team",
		F:    sweepTestSuiteTeams,
	})

	resource.AddTestSweepers("buildkite_organization_banner", &resource.Sweeper{
		Name: "buildkite_organization_banner",
		F:    sweepOrganizationBanners,
	})

	resource.AddTestSweepers("buildkite_organization_rule", &resource.Sweeper{
		Name: "buildkite_organization_rule",
		F:    sweepOrganizationRules,
	})

	resource.AddTestSweepers("buildkite_pipeline_team", &resource.Sweeper{
		Name: "buildkite_pipeline_team",
		F:    sweepPipelineTeams,
	})

	resource.AddTestSweepers("buildkite_registry", &resource.Sweeper{
		Name: "buildkite_registry",
		F:    sweepRegistries,
	})

	resource.AddTestSweepers("buildkite_cluster_default_queue", &resource.Sweeper{
		Name: "buildkite_cluster_default_queue",
		F:    sweepClusterDefaultQueues,
	})
}

// sweepAgentTokens removes agent tokens created during testing
// sweepAgentTokens removes agent tokens created during testing
func sweepAgentTokens(region string) error {
	log.Printf("[INFO] Sweeping buildkite_agent_token resources...")
	ctx := context.Background()

	orgSlug := getOrgEnv()
	apiToken := getApiTokenEnv()

	if orgSlug == "" || apiToken == "" {
		log.Printf("[WARN] BUILDKITE_ORGANIZATION_SLUG or BUILDKITE_API_TOKEN not set. Skipping sweep of agent tokens.")
		return nil
	}

	cfg := &clientConfig{
		org:        orgSlug,
		apiToken:   apiToken,
		graphqlURL: defaultGraphqlEndpoint, // defined in provider.go
		restURL:    defaultRestEndpoint,    // defined in provider.go
		userAgent:  "terraform-provider-buildkite-sweeper",
	}
	bkClient := NewClient(cfg) // NewClient is defined in client.go

	tokens, err := listOrgAgentTokensInternal(ctx, bkClient.genqlient, bkClient.organization)
	if err != nil {
		return fmt.Errorf("failed to list agent tokens for sweeping (org: %s): %w", bkClient.organization, err)
	}

	var sweepErrors []string
	for _, token := range tokens {
		isAlreadyRevoked := token.RevokedAt != nil && *token.RevokedAt != ""
		tokenDescription := ""
		if token.Description != nil {
			tokenDescription = *token.Description
		}

		if isTestResource(tokenDescription) && !isAlreadyRevoked {
			log.Printf("[INFO] Revoking test agent token (ID: %s, Description: '%s', UUID: %s) in org %s...", token.ID, tokenDescription, token.UUID, bkClient.organization)
			if err := revokeAgentTokenInternal(ctx, bkClient.genqlient, token.ID, "Terraform Acceptance Test Sweeper"); err != nil {
				log.Printf("[ERROR] Failed to revoke agent token ID %s (UUID: %s) in org %s: %v", token.ID, token.UUID, bkClient.organization, err)
				sweepErrors = append(sweepErrors, err.Error())
			} else {
				log.Printf("[INFO] Successfully revoked test agent token (ID: %s, UUID: %s) in org %s.", token.ID, token.UUID, bkClient.organization)
			}
		} else if isTestResource(tokenDescription) && isAlreadyRevoked {
			log.Printf("[DEBUG] Test agent token (ID: %s, Description: '%s', UUID: %s) in org %s is already revoked.", token.ID, tokenDescription, token.UUID, bkClient.organization)
		}
	}

	if len(sweepErrors) > 0 {
		return fmt.Errorf("encountered errors while sweeping agent tokens for org %s:\n- %s", bkClient.organization, strings.Join(sweepErrors, "\n- "))
	}

	log.Printf("[INFO] Finished sweeping buildkite_agent_token resources for org %s.", bkClient.organization)
	return nil
}

func sweepClusters(region string) error {
	log.Printf("[INFO] Sweeping buildkite_cluster resources...")
	ctx := context.Background()
	orgSlug := getOrgEnv()
	apiToken := getApiTokenEnv()

	if orgSlug == "" || apiToken == "" {
		log.Printf("[WARN] BUILDKITE_ORGANIZATION_SLUG or BUILDKITE_API_TOKEN not set. Skipping sweep of clusters.")
		return nil
	}

	cfg := &clientConfig{
		org:        orgSlug,
		apiToken:   apiToken,
		graphqlURL: defaultGraphqlEndpoint,
		restURL:    defaultRestEndpoint,
		userAgent:  "terraform-provider-buildkite-sweeper",
	}
	client := NewClient(cfg)

	orgIDPtr, err := client.GetOrganizationID()
	if err != nil {
		return fmt.Errorf("failed to get organization GraphQL ID for sweeping clusters: %w", err)
	}
	if orgIDPtr == nil || *orgIDPtr == "" {
		return fmt.Errorf("organization GraphQL ID is nil or empty")
	}
	orgID := *orgIDPtr
	log.Printf("[DEBUG] Sweeping clusters in organization %s (GraphQL ID: %s)", orgSlug, orgID)

	clusters, err := listOrgClusters(ctx, client.genqlient, orgID)
	if err != nil {
		return fmt.Errorf("error fetching clusters for sweep in org %s (GraphQL ID: %s): %w", orgSlug, orgID, err)
	}

	if len(clusters) == 0 {
		log.Printf("[INFO] No clusters found in organization %s (GraphQL ID: %s) for sweeping.", orgSlug, orgID)
		return nil
	}

	var sweepErrors []string
	for _, clusterNode := range clusters {
		if clusterNode == nil {
			continue
		}

		if isTestResource(clusterNode.Name) {
			log.Printf("[INFO] Found test cluster to delete: %s (ID: %s)", clusterNode.Name, clusterNode.Id)

			_, err := deleteCluster(ctx, client.genqlient, clusterNode.Id, orgID)
			if err != nil {
				errMsg := fmt.Sprintf("failed to delete cluster %s (ID: %s) in org %s: %v", clusterNode.Name, clusterNode.Id, orgID, err)
				log.Printf("[ERROR] %s", errMsg)
				sweepErrors = append(sweepErrors, errMsg)
			} else {
				log.Printf("[DEBUG] Successfully deleted test cluster: %s (ID: %s) in org %s", clusterNode.Name, clusterNode.Id, orgID)
			}
		}
	}

	if len(sweepErrors) > 0 {
		return fmt.Errorf("errors during cluster sweep: %s", strings.Join(sweepErrors, "; "))
	}

	log.Printf("[INFO] Finished sweeping buildkite_cluster resources for organization %s.", orgSlug)
	return nil
}

func sweepClusterQueues(region string) error {
	log.Printf("[INFO] Sweeping buildkite_cluster_queue resources...")
	return nil
}

func sweepClusterAgentTokens(region string) error {
	log.Printf("[INFO] Sweeping buildkite_cluster_agent_token resources...")
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
		userAgent:  "terraform-provider-buildkite-sweeper",
	})

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

// listOrgPipelinesAndSchedulesInternal fetches all pipelines and their schedules for an organization.
// It handles pagination for both pipelines and schedules within each pipeline.
func listOrgPipelinesAndSchedulesInternal(ctx context.Context, client khanGraphQL.Client, orgSlug string) ([]*sweeperPipelineInfoForScheduleSweeping, error) {
	allPipelinesWithSchedules := make([]*sweeperPipelineInfoForScheduleSweeping, 0)
	pipelinePageSize := 100
	schedulePageSize := 100
	var pipelineEndCursor *string

	for {
		var resp struct {
			Organization struct {
				Pipelines struct {
					PageInfo struct {
						HasNextPage bool
						EndCursor   string
					}
					Edges []struct {
						Node struct {
							ID        string
							Name      string
							Schedules struct {
								PageInfo struct {
									HasNextPage bool
									EndCursor   string
								}
								Edges []struct {
									Node sweeperPipelineScheduleNode
								}
							}
						}
					}
				}
			}
		}

		req := &khanGraphQL.Request{
			Query: listOrgPipelinesAndSchedulesQuery,
			Variables: map[string]interface{}{
				"orgSlug":        orgSlug,
				"firstPipelines": pipelinePageSize,
				"afterPipeline":  pipelineEndCursor,
				"firstSchedules": schedulePageSize,
			},
		}

		if err := client.MakeRequest(ctx, req, &khanGraphQL.Response{Data: &resp}); err != nil {
			return nil, fmt.Errorf("failed to query pipelines and schedules for org %s: %w", orgSlug, err)
		}

		for _, pipelineEdge := range resp.Organization.Pipelines.Edges {
			pipelineNode := pipelineEdge.Node
			currentPipelineInfo := &sweeperPipelineInfoForScheduleSweeping{
				ID:   pipelineNode.ID,
				Name: pipelineNode.Name,
			}

			currentPipelineSchedules := make([]*sweeperPipelineScheduleNode, 0)
			for _, scheduleEdge := range pipelineNode.Schedules.Edges {
				nodeCopy := scheduleEdge.Node // Make a copy to take its address
				currentPipelineSchedules = append(currentPipelineSchedules, &nodeCopy)
			}

			var scheduleEndCursor *string
			if pipelineNode.Schedules.PageInfo.HasNextPage {
				scheduleEndCursor = &pipelineNode.Schedules.PageInfo.EndCursor
			}

			for scheduleEndCursor != nil {

				log.Printf("[DEBUG] Pipeline %s has more schedules, inner pagination for schedules not fully implemented in this pass.", pipelineNode.ID)
				scheduleEndCursor = nil
			}
			currentPipelineInfo.Schedules = currentPipelineSchedules
			allPipelinesWithSchedules = append(allPipelinesWithSchedules, currentPipelineInfo)
		}

		if !resp.Organization.Pipelines.PageInfo.HasNextPage {
			break // No more pipelines
		}
		pipelineEndCursor = &resp.Organization.Pipelines.PageInfo.EndCursor
	}

	return allPipelinesWithSchedules, nil
}

func deletePipelineScheduleInternal(ctx context.Context, client khanGraphQL.Client, scheduleID string) error {
	log.Printf("[DEBUG] Deleting pipeline schedule with ID: %s", scheduleID)
	var resp struct {
		PipelineScheduleDelete struct {
			ClientMutationId string
		}
	}

	req := &khanGraphQL.Request{
		Query: deletePipelineScheduleMutation,
		Variables: map[string]interface{}{
			"scheduleID": scheduleID,
		},
	}

	if err := client.MakeRequest(ctx, req, &khanGraphQL.Response{Data: &resp}); err != nil {
		return fmt.Errorf("failed to delete pipeline schedule ID %s: %w", scheduleID, err)
	}

	log.Printf("[DEBUG] Successfully initiated deletion of pipeline schedule ID %s.", scheduleID)
	return nil
}

func sweepPipelineSchedules(region string) error {
	log.Printf("[INFO] Sweeping buildkite_pipeline_schedule resources...")
	ctx := context.Background()

	orgSlug := getOrgEnv()
	apiToken := getApiTokenEnv()

	if orgSlug == "" || apiToken == "" {
		log.Printf("[WARN] BUILDKITE_ORGANIZATION_SLUG or BUILDKITE_API_TOKEN not set. Skipping sweep of pipeline schedules.")
		return nil
	}

	cfg := &clientConfig{
		org:        orgSlug,
		apiToken:   apiToken,
		graphqlURL: defaultGraphqlEndpoint,
		restURL:    defaultRestEndpoint,
		userAgent:  "terraform-provider-buildkite-sweeper",
	}
	bkClient := NewClient(cfg)

	pipelinesWithSchedules, err := listOrgPipelinesAndSchedulesInternal(ctx, bkClient.genqlient, bkClient.organization)
	if err != nil {
		return fmt.Errorf("failed to list pipelines and schedules for sweeping (org: %s): %w", bkClient.organization, err)
	}

	var sweepErrors []string
	for _, pipelineInfo := range pipelinesWithSchedules {
		for _, schedule := range pipelineInfo.Schedules {
			// Determine if the schedule is a test resource
			// Option 1: Check the schedule's label
			isTestSched := isTestResource(schedule.Label)
			// Option 2: Check the parent pipeline's name (if label is not definitive)
			if !isTestSched && pipelineInfo.Name != "" {
				isTestSched = isTestResource(pipelineInfo.Name)
				// If identified via pipeline, log it differently or add context
				if isTestSched {
					log.Printf("[DEBUG] Schedule ID %s (Label: '%s', UUID: %s) on pipeline '%s' (ID: %s) identified as test resource via parent pipeline.", schedule.ID, schedule.Label, schedule.UUID, pipelineInfo.Name, pipelineInfo.ID)
				}
			}

			if isTestSched {
				log.Printf("[INFO] Deleting test pipeline schedule (ID: %s, Label: '%s', UUID: %s) on pipeline '%s' (ID: %s)...", schedule.ID, schedule.Label, schedule.UUID, pipelineInfo.Name, pipelineInfo.ID)
				if err := deletePipelineScheduleInternal(ctx, bkClient.genqlient, schedule.ID); err != nil {
					log.Printf("[ERROR] Failed to delete pipeline schedule ID %s (UUID: %s, Label: '%s'): %v", schedule.ID, schedule.UUID, schedule.Label, err)
					sweepErrors = append(sweepErrors, err.Error())
				} else {
					log.Printf("[INFO] Successfully deleted test pipeline schedule (ID: %s, UUID: %s, Label: '%s').", schedule.ID, schedule.UUID, schedule.Label)
				}
			}
		}
	}

	if len(sweepErrors) > 0 {
		return fmt.Errorf("encountered errors while sweeping pipeline schedules for org %s:\n- %s", bkClient.organization, strings.Join(sweepErrors, "\n- "))
	}

	log.Printf("[INFO] Finished sweeping buildkite_pipeline_schedule resources for org %s.", bkClient.organization)
	return nil
}

func sweepPipelineTemplates(region string) error {
	log.Printf("[INFO] Sweeping buildkite_pipeline_template resources...")
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
		userAgent:  "terraform-provider-buildkite-sweeper",
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

// listOrgTeamsAndMembersInternal fetches all teams and their members for an organization.
func listOrgTeamsAndMembersInternal(ctx context.Context, client khanGraphQL.Client, orgSlug string) ([]*sweeperTeamInfoForMemberSweeping, error) {
	allTeamsWithMembers := make([]*sweeperTeamInfoForMemberSweeping, 0)
	teamPageSize := 50
	memberPageSize := 100
	var teamEndCursor *string

	for {
		var resp struct {
			Organization struct {
				Teams struct {
					PageInfo struct {
						HasNextPage bool
						EndCursor   string
					}
					Edges []struct {
						Node struct {
							ID      string
							Name    string
							Members struct {
								PageInfo struct {
									HasNextPage bool
									EndCursor   string
								}
								Edges []struct {
									Node sweeperTeamMemberNode
								}
							}
						}
					}
				}
			}
		}

		req := &khanGraphQL.Request{
			Query: listOrgTeamsAndMembersQuery,
			Variables: map[string]interface{}{
				"orgSlug":      orgSlug,
				"firstTeams":   teamPageSize,
				"afterTeam":    teamEndCursor,
				"firstMembers": memberPageSize,
			},
		}

		if err := client.MakeRequest(ctx, req, &khanGraphQL.Response{Data: &resp}); err != nil {
			return nil, fmt.Errorf("failed to query teams and members for org %s: %w", orgSlug, err)
		}

		for _, teamEdge := range resp.Organization.Teams.Edges {
			teamNode := teamEdge.Node
			currentTeamInfo := &sweeperTeamInfoForMemberSweeping{
				ID:   teamNode.ID,
				Name: teamNode.Name,
			}

			currentTeamMembers := make([]*sweeperTeamMemberNode, 0)
			for _, memberEdge := range teamNode.Members.Edges {
				nodeCopy := memberEdge.Node
				currentTeamMembers = append(currentTeamMembers, &nodeCopy)
			}

			var memberEndCursor *string
			if teamNode.Members.PageInfo.HasNextPage {
				memberEndCursor = &teamNode.Members.PageInfo.EndCursor
			}

			// Inner loop for paginating all members for the current teamNode
			for memberEndCursor != nil {
				log.Printf("[DEBUG] Fetching more members for team %s (ID: %s), after cursor: %s", teamNode.Name, teamNode.ID, *memberEndCursor)
				var memberResp struct {
					Node struct {
						// TeamSpecificFields holds fields from the '... on Team' inline fragment
						TeamSpecificFields struct {
							Members struct {
								PageInfo struct {
									HasNextPage bool
									EndCursor   string
								}
								Edges []struct {
									Node sweeperTeamMemberNode
								}
							}
						} `graphql:"... on Team"`
					}
				}

				memberReq := &khanGraphQL.Request{
					Query: listMembersForTeamQuery,
					Variables: map[string]interface{}{
						"teamID":       teamNode.ID,
						"firstMembers": memberPageSize, // Reuse existing page size
						"afterMember":  memberEndCursor,
					},
				}

				if err := client.MakeRequest(ctx, memberReq, &khanGraphQL.Response{Data: &memberResp}); err != nil {
					log.Printf("[WARN] Failed to query additional members for team %s (ID: %s): %v. Proceeding with already fetched members for this team.", teamNode.Name, teamNode.ID, err)
					break
				}

				for _, memberEdge := range memberResp.Node.TeamSpecificFields.Members.Edges {
					nodeCopy := memberEdge.Node
					currentTeamMembers = append(currentTeamMembers, &nodeCopy)
				}

				if memberResp.Node.TeamSpecificFields.Members.PageInfo.HasNextPage {
					memberEndCursor = &memberResp.Node.TeamSpecificFields.Members.PageInfo.EndCursor
				} else {
					memberEndCursor = nil
				}
			}
			currentTeamInfo.Members = currentTeamMembers
			allTeamsWithMembers = append(allTeamsWithMembers, currentTeamInfo)
		}

		if !resp.Organization.Teams.PageInfo.HasNextPage {
			break
		}
		teamEndCursor = &resp.Organization.Teams.PageInfo.EndCursor
	}

	return allTeamsWithMembers, nil
}

func deleteTeamMemberInternal(ctx context.Context, client khanGraphQL.Client, teamMemberID string) error {
	log.Printf("[DEBUG] Deleting team member with ID: %s", teamMemberID)
	var resp struct {
		TeamMemberDelete struct {
			ClientMutationId string
		}
	}

	req := &khanGraphQL.Request{
		Query: deleteTeamMemberMutation,
		Variables: map[string]interface{}{
			"teamMemberID": teamMemberID,
		},
	}

	if err := client.MakeRequest(ctx, req, &khanGraphQL.Response{Data: &resp}); err != nil {
		return fmt.Errorf("failed to delete team member ID %s: %w", teamMemberID, err)
	}

	log.Printf("[DEBUG] Successfully initiated deletion of team member ID %s.", teamMemberID)
	return nil
}

func sweepTeamMembers(region string) error {
	log.Printf("[INFO] Sweeping buildkite_team_member resources...")
	ctx := context.Background()

	orgSlug := getOrgEnv()
	apiToken := getApiTokenEnv()

	if orgSlug == "" || apiToken == "" {
		log.Printf("[WARN] BUILDKITE_ORGANIZATION_SLUG or BUILDKITE_API_TOKEN not set. Skipping sweep of team members.")
		return nil
	}

	cfg := &clientConfig{
		org:        orgSlug,
		apiToken:   apiToken,
		graphqlURL: defaultGraphqlEndpoint,
		restURL:    defaultRestEndpoint,
		userAgent:  "terraform-provider-buildkite-sweeper",
	}
	bkClient := NewClient(cfg)

	teamsAndMembers, err := listOrgTeamsAndMembersInternal(ctx, bkClient.genqlient, bkClient.organization)
	if err != nil {
		return fmt.Errorf("failed to list teams and members for sweeping (org: %s): %w", bkClient.organization, err)
	}

	var sweepErrors []string
	for _, teamInfo := range teamsAndMembers {
		if isTestResource(teamInfo.Name) {
			log.Printf("[DEBUG] Team '%s' (ID: %s) is a test resource. Sweeping its members.", teamInfo.Name, teamInfo.ID)
			for _, member := range teamInfo.Members {
				log.Printf("[INFO] Deleting test team member (ID: %s, Role: %s, User: %s/%s) from team '%s' (ID: %s)...", member.ID, member.Role, member.User.Name, member.User.Email, teamInfo.Name, teamInfo.ID)
				if err := deleteTeamMemberInternal(ctx, bkClient.genqlient, member.ID); err != nil {
					log.Printf("[ERROR] Failed to delete team member ID %s (User: %s) from team '%s': %v", member.ID, member.User.Name, teamInfo.Name, err)
					sweepErrors = append(sweepErrors, err.Error())
				} else {
					log.Printf("[INFO] Successfully deleted team member ID %s (User: %s) from team '%s'.", member.ID, member.User.Name, teamInfo.Name)
				}
			}

		}
	}

	if len(sweepErrors) > 0 {
		return fmt.Errorf("encountered errors while sweeping team members for org %s:\n- %s", bkClient.organization, strings.Join(sweepErrors, "\n- "))
	}

	log.Printf("[INFO] Finished sweeping buildkite_team_member resources for org %s.", bkClient.organization)
	return nil
}

// sweeperPipelineRESTNode represents a pipeline as returned by the Buildkite REST API (subset of fields)
type sweeperPipelineRESTNode struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"` // Name can also be used if slug is not definitive
}

// sweeperTeamPipelineLinkRESTNode represents a link between a team and a pipeline from the REST API
type sweeperTeamPipelineLinkRESTNode struct {
	PipelineID string `json:"pipeline_id"`
	// AccessLevel string `json:"access_level"` // Not needed for sweeping deletion
}

// listOrgPipelinesRESTInternal fetches all pipelines for an organization using the REST API.
func listOrgPipelinesRESTInternal(ctx context.Context, client *Client, orgSlug, apiToken string) ([]sweeperPipelineRESTNode, error) {
	var allPipelines []sweeperPipelineRESTNode
	page := 1
	for {
		// Use client.restURL to construct the full URL
		url := fmt.Sprintf("%s/v2/organizations/%s/pipelines?page=%d&per_page=100", client.restURL, orgSlug, page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request for listing pipelines (org: %s, page: %d): %w", orgSlug, page, err)
		}
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiToken))
		// User-Agent is set by the client's transport

		resp, err := client.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to list pipelines (org: %s, page: %d): %w", orgSlug, page, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("failed to list pipelines (org: %s, page: %d): status %d, body: %s", orgSlug, page, resp.StatusCode, string(bodyBytes))
		}

		var pipelines []sweeperPipelineRESTNode
		if err := json.NewDecoder(resp.Body).Decode(&pipelines); err != nil {
			return nil, fmt.Errorf("failed to decode pipelines list (org: %s, page: %d): %w", orgSlug, page, err)
		}

		allPipelines = append(allPipelines, pipelines...)

		if len(pipelines) < 100 { // Check if this was the last page
			break
		}
		page++
	}
	log.Printf("[DEBUG] Found %d pipelines in organization %s via REST API.", len(allPipelines), orgSlug)
	return allPipelines, nil
}

// listPipelinesForTeamRESTInternal fetches all pipeline links for a specific team using the REST API.
func listPipelinesForTeamRESTInternal(ctx context.Context, client *Client, orgSlug, teamID, apiToken string) ([]sweeperTeamPipelineLinkRESTNode, error) {
	var allPipelineLinks []sweeperTeamPipelineLinkRESTNode
	page := 1
	for {
		url := fmt.Sprintf("%s/v2/organizations/%s/teams/%s/pipelines?page=%d&per_page=100", client.restURL, orgSlug, teamID, page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request to list pipelines for team %s (org %s, page %d): %w", teamID, orgSlug, page, err)
		}
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiToken))

		resp, err := client.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to list pipelines for team %s (org %s, page %d): %w", teamID, orgSlug, page, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("failed to list pipelines for team %s (org %s, page %d): status %d, body: %s", teamID, orgSlug, page, resp.StatusCode, string(bodyBytes))
		}

		var links []sweeperTeamPipelineLinkRESTNode
		if err := json.NewDecoder(resp.Body).Decode(&links); err != nil {
			return nil, fmt.Errorf("failed to decode pipeline links for team %s (org %s, page %d): %w", teamID, orgSlug, page, err)
		}

		allPipelineLinks = append(allPipelineLinks, links...)

		if len(links) < 100 { // Check if this was the last page
			break
		}
		page++
	}
	log.Printf("[DEBUG] Found %d pipeline links for team %s (org %s) via REST API.", len(allPipelineLinks), teamID, orgSlug)
	return allPipelineLinks, nil
}

// deleteTeamPipelineLinkRESTInternal deletes a link between a team and a pipeline using the REST API.
func deleteTeamPipelineLinkRESTInternal(ctx context.Context, client *Client, orgSlug, teamID, pipelineID, apiToken string) error {
	url := fmt.Sprintf("%s/v2/organizations/%s/teams/%s/pipelines/%s", client.restURL, orgSlug, teamID, pipelineID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request to delete team-pipeline link (team: %s, pipeline: %s, org: %s): %w", teamID, pipelineID, orgSlug, err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiToken))

	resp, err := client.http.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete team-pipeline link (team: %s, pipeline: %s, org: %s): %w", teamID, pipelineID, orgSlug, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete team-pipeline link (team: %s, pipeline: %s, org: %s): status %d, body: %s", teamID, pipelineID, orgSlug, resp.StatusCode, string(bodyBytes))
	}

	log.Printf("[DEBUG] Successfully deleted team-pipeline link (team: %s, pipeline: %s, org: %s) via REST API.", teamID, pipelineID, orgSlug)
	return nil
}

// listOrgTestSuitesRESTInternal fetches all test suites for an organization using the REST API.
func listOrgTestSuitesRESTInternal(ctx context.Context, client *Client, orgSlug string, apiToken string) ([]sweeperTestSuiteRESTNode, error) {
	url := fmt.Sprintf("%s/v2/analytics/organizations/%s/suites", client.restURL, orgSlug)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request to list test suites for org %s: %w", orgSlug, err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiToken))

	resp, err := client.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list test suites for org %s: %w", orgSlug, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list test suites for org %s: status %d, body: %s", orgSlug, resp.StatusCode, string(bodyBytes))
	}

	var suites []sweeperTestSuiteRESTNode
	if err := json.NewDecoder(resp.Body).Decode(&suites); err != nil {
		return nil, fmt.Errorf("failed to decode test suites list for org %s: %w", orgSlug, err)
	}
	log.Printf("[DEBUG] Found %d test suites for org %s via REST API.", len(suites), orgSlug)
	return suites, nil
}

// deleteTestSuiteRESTInternal deletes a test suite using the REST API.
func deleteTestSuiteRESTInternal(ctx context.Context, client *Client, orgSlug, suiteSlug string, apiToken string) error {
	url := fmt.Sprintf("%s/v2/analytics/organizations/%s/suites/%s", client.restURL, orgSlug, suiteSlug)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request to delete test suite %s/%s: %w", orgSlug, suiteSlug, err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiToken))

	resp, err := client.http.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete test suite %s/%s: %w", orgSlug, suiteSlug, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete test suite %s/%s: status %d, body: %s", orgSlug, suiteSlug, resp.StatusCode, string(bodyBytes))
	}

	log.Printf("[DEBUG] Successfully deleted test suite %s/%s via REST API.", orgSlug, suiteSlug)
	return nil
}

// sweepTestSuites removes test suites created during testing using the REST API
// listOrgTeamsRESTInternal fetches all teams for an organization using the REST API.
func listOrgTeamsRESTInternal(ctx context.Context, client *Client, orgSlug string, apiToken string) ([]sweeperTeamRESTNode, error) {
	url := fmt.Sprintf("%s/v2/organizations/%s/teams", client.restURL, orgSlug)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request to list teams for org %s: %w", orgSlug, err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiToken))

	resp, err := client.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list teams for org %s: %w", orgSlug, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list teams for org %s: status %d, body: %s", orgSlug, resp.StatusCode, string(bodyBytes))
	}

	var teams []sweeperTeamRESTNode
	if err := json.NewDecoder(resp.Body).Decode(&teams); err != nil {
		return nil, fmt.Errorf("failed to decode teams list for org %s: %w", orgSlug, err)
	}
	log.Printf("[DEBUG] Found %d teams for org %s via REST API.", len(teams), orgSlug)
	return teams, nil
}

// listSuitesForTeamRESTInternal fetches all test suite links for a specific team using the REST API.
func listSuitesForTeamRESTInternal(ctx context.Context, client *Client, orgSlug, teamID, apiToken string) ([]sweeperTeamSuiteLinkRESTNode, error) {
	url := fmt.Sprintf("%s/v2/organizations/%s/teams/%s/suites", client.restURL, orgSlug, teamID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request to list suites for team %s (org %s): %w", teamID, orgSlug, err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiToken))

	resp, err := client.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list suites for team %s (org %s): %w", teamID, orgSlug, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list suites for team %s (org %s): status %d, body: %s", teamID, orgSlug, resp.StatusCode, string(bodyBytes))
	}

	var links []sweeperTeamSuiteLinkRESTNode
	if err := json.NewDecoder(resp.Body).Decode(&links); err != nil {
		return nil, fmt.Errorf("failed to decode suite links for team %s (org %s): %w", teamID, orgSlug, err)
	}
	log.Printf("[DEBUG] Found %d suite links for team %s (org %s) via REST API.", len(links), teamID, orgSlug)
	return links, nil
}

// deleteTeamSuiteLinkRESTInternal deletes a link between a team and a test suite using the REST API.
func deleteTeamSuiteLinkRESTInternal(ctx context.Context, client *Client, orgSlug, teamID, suiteID, apiToken string) error {
	url := fmt.Sprintf("%s/v2/organizations/%s/teams/%s/suites/%s", client.restURL, orgSlug, teamID, suiteID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request to delete team-suite link (team: %s, suite: %s, org: %s): %w", teamID, suiteID, orgSlug, err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiToken))

	resp, err := client.http.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete team-suite link (team: %s, suite: %s, org: %s): %w", teamID, suiteID, orgSlug, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete team-suite link (team: %s, suite: %s, org: %s): status %d, body: %s", teamID, suiteID, orgSlug, resp.StatusCode, string(bodyBytes))
	}

	log.Printf("[DEBUG] Successfully deleted team-suite link (team: %s, suite: %s, org: %s) via REST API.", teamID, suiteID, orgSlug)
	return nil
}

func sweepTestSuites(region string) error { // region param is unused but kept for sweeper func signature consistency for now
	log.Printf("[INFO] Sweeping buildkite_test_suite resources (using REST API)...")
	ctx := context.Background()

	orgSlug := getOrgEnv()
	apiToken := getApiTokenEnv()

	if orgSlug == "" || apiToken == "" {
		log.Printf("[WARN] BUILDKITE_ORGANIZATION_SLUG or BUILDKITE_API_TOKEN not set. Skipping sweep of test suites.")
		return nil
	}

	cfg := &clientConfig{
		org:        orgSlug,
		apiToken:   apiToken,
		graphqlURL: defaultGraphqlEndpoint,
		restURL:    defaultRestEndpoint,
		userAgent:  "terraform-provider-buildkite-sweeper",
	}
	bkClient := NewClient(cfg)

	testSuites, err := listOrgTestSuitesRESTInternal(ctx, bkClient, orgSlug, apiToken)
	if err != nil {
		return fmt.Errorf("failed to list test suites for sweeping (org: %s): %w", orgSlug, err)
	}

	var sweepErrors []string
	for _, suite := range testSuites {
		if isTestResource(suite.Name) {
			log.Printf("[INFO] Deleting test suite '%s' (Slug: %s, ID: %s) via REST API...", suite.Name, suite.Slug, suite.ID)
			if err := deleteTestSuiteRESTInternal(ctx, bkClient, orgSlug, suite.Slug, apiToken); err != nil {
				log.Printf("[ERROR] Failed to delete test suite '%s' (Slug: %s): %v", suite.Name, suite.Slug, err)
				sweepErrors = append(sweepErrors, err.Error())
			} else {
				log.Printf("[INFO] Successfully deleted test suite '%s' (Slug: %s).", suite.Name, suite.Slug)
			}
		}
	}

	if len(sweepErrors) > 0 {
		return fmt.Errorf("encountered errors while sweeping test suites for org %s (REST API):\n- %s", orgSlug, strings.Join(sweepErrors, "\n- "))
	}

	log.Printf("[INFO] Finished sweeping buildkite_test_suite resources for org %s (REST API).", orgSlug)
	return nil
}

// sweepTestSuiteTeams removes associations between test teams/suites created during testing using the REST API
func sweepTestSuiteTeams(region string) error { // region param is unused but kept for sweeper func signature consistency
	log.Printf("[INFO] Sweeping buildkite_test_suite_team resources (using REST API)...")
	ctx := context.Background()

	orgSlug := getOrgEnv()
	apiToken := getApiTokenEnv()

	if orgSlug == "" || apiToken == "" {
		log.Printf("[WARN] BUILDKITE_ORGANIZATION_SLUG or BUILDKITE_API_TOKEN not set. Skipping sweep of test suite teams.")
		return nil
	}

	cfg := &clientConfig{
		org:        orgSlug,
		apiToken:   apiToken,
		graphqlURL: defaultGraphqlEndpoint,
		restURL:    defaultRestEndpoint,
		userAgent:  "terraform-provider-buildkite-sweeper",
	}
	bkClient := NewClient(cfg)

	allTeams, err := listOrgTeamsRESTInternal(ctx, bkClient, orgSlug, apiToken)
	if err != nil {
		return fmt.Errorf("failed to list teams for sweeping test_suite_teams (org: %s): %w", orgSlug, err)
	}

	allTestSuites, err := listOrgTestSuitesRESTInternal(ctx, bkClient, orgSlug, apiToken)
	if err != nil {
		return fmt.Errorf("failed to list test suites for sweeping test_suite_teams (org: %s): %w", orgSlug, err)
	}
	suiteIDToNameMap := make(map[string]string)
	for _, ts := range allTestSuites {
		suiteIDToNameMap[ts.ID] = ts.Name
	}

	var sweepErrors []string

	for _, team := range allTeams {
		log.Printf("[DEBUG] Checking team '%s' (ID: %s) for test suite associations...", team.Name, team.ID)
		suiteLinks, err := listSuitesForTeamRESTInternal(ctx, bkClient, orgSlug, team.ID, apiToken)
		if err != nil {
			log.Printf("[ERROR] Failed to list suite associations for team '%s' (ID: %s): %v", team.Name, team.ID, err)
			sweepErrors = append(sweepErrors, err.Error())
			continue // Move to the next team
		}

		for _, link := range suiteLinks {
			suiteName, suiteExists := suiteIDToNameMap[link.SuiteID]
			if !suiteExists {
				log.Printf("[WARN] Test suite with ID '%s' (linked to team '%s') not found in initial list. Skipping link.", link.SuiteID, team.Name)
				continue
			}

			isTeamTestResource := isTestResource(team.Name)
			isSuiteTestResource := isTestResource(suiteName)

			if isTeamTestResource || isSuiteTestResource {
				log.Printf("[INFO] Deleting test suite team link: Team '%s' (Test: %t) <-> Suite '%s' (Test: %t) (SuiteID: %s, TeamID: %s)",
					team.Name, isTeamTestResource, suiteName, isSuiteTestResource, link.SuiteID, team.ID)
				if err := deleteTeamSuiteLinkRESTInternal(ctx, bkClient, orgSlug, team.ID, link.SuiteID, apiToken); err != nil {
					log.Printf("[ERROR] Failed to delete test suite team link (TeamID: %s, SuiteID: %s): %v", team.ID, link.SuiteID, err)
					sweepErrors = append(sweepErrors, err.Error())
				} else {
					log.Printf("[INFO] Successfully deleted test suite team link (TeamID: %s, SuiteID: %s).", team.ID, link.SuiteID)
				}
			}
		}
	}

	if len(sweepErrors) > 0 {
		return fmt.Errorf("encountered errors while sweeping test_suite_team resources for org %s (REST API):\n- %s", orgSlug, strings.Join(sweepErrors, "\n- "))
	}

	log.Printf("[INFO] Finished sweeping buildkite_test_suite_team resources for org %s (REST API).", orgSlug)
	return nil
}

// sweepOrganizationBanners removes organization banners created during testing
func sweepOrganizationBanners(region string) error {
	log.Printf("[INFO] Sweeping buildkite_organization_banner resources...")
	ctx := context.Background()

	client := NewClient(&clientConfig{
		org:        getOrgEnv(),
		apiToken:   getApiTokenEnv(),
		graphqlURL: defaultGraphqlEndpoint,
		restURL:    defaultRestEndpoint,
		userAgent:  "terraform-provider-buildkite-sweeper",
	})

	// Get the current organization banner
	resp, err := getOrganiztionBanner(ctx, client.genqlient, getOrgEnv())
	if err != nil {
		return fmt.Errorf("error fetching organization banner: %w", err)
	}

	// Only delete if the banner exists and is a test resource
	if len(resp.Organization.Banners.Edges) > 0 {
		bannerNode := resp.Organization.Banners.Edges[0].Node
		// Check if the banner message is a test resource
		if isTestResource(bannerNode.Message) {
			log.Printf("[INFO] Deleting test organization banner")
			// To delete a banner, we can set an empty message
			_, err = upsertBanner(ctx, client.genqlient, getOrgEnv(), "")
			if err != nil {
				return fmt.Errorf("error deleting organization banner: %w", err)
			}
		}
	}

	return nil
}

// sweepOrganizationRules removes organization rules created during testing
func sweepOrganizationRules(region string) error {
	log.Printf("[INFO] Sweeping buildkite_organization_rule resources...")
	log.Printf("[WARN] Organization rules sweeping is not yet implemented")
	return nil
}

// sweepPipelineTeams removes pipeline team associations created during testing using the REST API
func sweepPipelineTeams(region string) error { // region param is unused but kept for sweeper func signature consistency
	log.Printf("[INFO] Sweeping buildkite_pipeline_team resources (using REST API)...")
	ctx := context.Background()

	orgSlug := getOrgEnv()
	apiToken := getApiTokenEnv()

	if orgSlug == "" || apiToken == "" {
		log.Printf("[WARN] BUILDKITE_ORGANIZATION_SLUG or BUILDKITE_API_TOKEN not set. Skipping sweep of pipeline-team associations.")
		return nil
	}

	cfg := &clientConfig{
		org:        orgSlug,
		apiToken:   apiToken,
		graphqlURL: defaultGraphqlEndpoint, // Not strictly needed for this sweeper, but good for consistency
		restURL:    defaultRestEndpoint,
		userAgent:  "terraform-provider-buildkite-sweeper",
	}
	bkClient := NewClient(cfg)

	// 1. List all teams
	allTeams, err := listOrgTeamsRESTInternal(ctx, bkClient, orgSlug, apiToken)
	if err != nil {
		return fmt.Errorf("failed to list teams for sweeping pipeline-team associations (org: %s): %w", orgSlug, err)
	}
	log.Printf("[DEBUG] Found %d teams in organization %s for pipeline-team sweep.", len(allTeams), orgSlug)

	// 2. List all pipelines and create a map for easy lookup (ID to Node)
	allPipelines, err := listOrgPipelinesRESTInternal(ctx, bkClient, orgSlug, apiToken)
	if err != nil {
		return fmt.Errorf("failed to list pipelines for sweeping pipeline-team associations (org: %s): %w", orgSlug, err)
	}
	pipelineInfoMap := make(map[string]sweeperPipelineRESTNode)
	for _, p := range allPipelines {
		pipelineInfoMap[p.ID] = p
	}
	log.Printf("[DEBUG] Found %d pipelines in organization %s and mapped them for pipeline-team sweep.", len(allPipelines), orgSlug)

	var sweepErrors []string

	// 3. Iterate through teams, then their pipeline associations
	for _, team := range allTeams {
		isTestTeam := isTestResource(team.Name) || isTestResource(team.Slug)
		log.Printf("[DEBUG] Checking team '%s' (ID: %s, Slug: %s, IsTest: %t) for pipeline associations...", team.Name, team.ID, team.Slug, isTestTeam)

		pipelineLinks, err := listPipelinesForTeamRESTInternal(ctx, bkClient, orgSlug, team.ID, apiToken)
		if err != nil {
			errMessage := fmt.Sprintf("failed to list pipeline associations for team %s (ID: %s, org: %s): %v", team.Name, team.ID, orgSlug, err)
			log.Printf("[ERROR] %s", errMessage)
			sweepErrors = append(sweepErrors, errMessage)
			continue // Move to the next team
		}

		if len(pipelineLinks) == 0 && isTestTeam {
			log.Printf("[DEBUG] Test team '%s' (ID: %s) has no pipeline associations to check.", team.Name, team.ID)
		}

		for _, link := range pipelineLinks {
			pipelineNode, ok := pipelineInfoMap[link.PipelineID]
			if !ok {
				log.Printf("[WARN] Pipeline ID %s (associated with team %s) not found in pipeline list. Skipping.", link.PipelineID, team.Name)
				continue
			}

			isTestPipeline := isTestResource(pipelineNode.Name) || isTestResource(pipelineNode.Slug)
			log.Printf("[DEBUG] ... found association with pipeline '%s' (ID: %s, Slug: %s, IsTest: %t)", pipelineNode.Name, pipelineNode.ID, pipelineNode.Slug, isTestPipeline)

			if isTestTeam || isTestPipeline {
				log.Printf("[INFO] Deleting pipeline-team association: Team '%s' (ID: %s) <-> Pipeline '%s' (ID: %s) because TestTeam=%t, TestPipeline=%t",
					team.Name, team.ID, pipelineNode.Name, pipelineNode.ID, isTestTeam, isTestPipeline)
				err := deleteTeamPipelineLinkRESTInternal(ctx, bkClient, orgSlug, team.ID, pipelineNode.ID, apiToken)
				if err != nil {
					errMessage := fmt.Sprintf("failed to delete pipeline-team association (team ID: %s, pipeline ID: %s, org: %s): %v", team.ID, pipelineNode.ID, orgSlug, err)
					log.Printf("[ERROR] %s", errMessage)
					sweepErrors = append(sweepErrors, errMessage)
				} else {
					log.Printf("[DEBUG] Successfully deleted pipeline-team association: Team '%s' <-> Pipeline '%s'", team.Name, pipelineNode.Name)
				}
			}
		}
	}

	if len(sweepErrors) > 0 {
		return fmt.Errorf("encountered errors while sweeping pipeline-team associations for org %s (REST API):\n- %s", orgSlug, strings.Join(sweepErrors, "\n- "))
	}

	log.Printf("[INFO] Finished sweeping buildkite_pipeline_team resources for org %s (REST API).", orgSlug)
	return nil
}

type Registry struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func sweepRegistries(region string) error {
	log.Printf("[INFO] Sweeping buildkite_registry resources...")

	apiToken := getApiTokenEnv()
	orgSlug := getOrgEnv()
	baseURL := os.Getenv("BUILDKITE_API_URL")
	if baseURL == "" {
		baseURL = "https://api.buildkite.com" // Default REST API endpoint
	}

	if apiToken == "" || orgSlug == "" {
		log.Printf("[WARN] BUILDKITE_API_TOKEN or BUILDKITE_ORGANIZATION_SLUG not set. Skipping sweep of registries.")
		return nil
	}

	client := &http.Client{Timeout: 20 * time.Second} // Increased timeout for potentially many registries
	listURL := fmt.Sprintf("%s/v2/packages/organizations/%s/registries", baseURL, orgSlug)

	req, err := http.NewRequest(http.MethodGet, listURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create list registries request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiToken))
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to list registries: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to list registries: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read list registries response body: %w", err)
	}

	var registries []Registry
	if err := json.Unmarshal(bodyBytes, &registries); err != nil {
		if len(bodyBytes) > 0 && string(bodyBytes) != "[]" {
			log.Printf("[WARN] Failed to unmarshal registries list. Body: %s. Error: %v", string(bodyBytes), err)
		}
	}

	var sweepErrors []string
	for _, registry := range registries {
		if isTestResource(registry.Name) {
			log.Printf("[INFO] Deleting test registry: %s (slug: %s)", registry.Name, registry.Slug)
			deleteURL := fmt.Sprintf("%s/v2/packages/organizations/%s/registries/%s", baseURL, orgSlug, registry.Slug)
			delReq, err := http.NewRequest(http.MethodDelete, deleteURL, nil)
			if err != nil {
				sweepErrors = append(sweepErrors, fmt.Sprintf("failed to create delete request for registry %s: %v", registry.Slug, err))
				continue
			}
			delReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiToken))

			delResp, err := client.Do(delReq)
			if err != nil {
				sweepErrors = append(sweepErrors, fmt.Sprintf("failed to delete registry %s: %v", registry.Slug, err))
				continue
			}
			delResp.Body.Close()

			if delResp.StatusCode != http.StatusNoContent && delResp.StatusCode != http.StatusNotFound {
				sweepErrors = append(sweepErrors, fmt.Sprintf("failed to delete registry %s: status %d", registry.Slug, delResp.StatusCode))
			} else {
				log.Printf("[INFO] Successfully deleted test registry: %s (slug: %s)", registry.Name, registry.Slug)
			}
		}
	}

	if len(sweepErrors) > 0 {
		return fmt.Errorf("errors during registry sweep: %s", strings.Join(sweepErrors, "; "))
	}

	log.Printf("[INFO] Finished sweeping buildkite_registry resources.")
	return nil
}

// TODO: Implement sweepClusterDefaultQueues

func sweepClusterDefaultQueues(region string) error {
	log.Printf("[INFO] Sweeping buildkite_cluster_default_queue resources...")
	log.Printf("[WARN] Cluster default queues sweeping is not yet implemented")
	return nil
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

	if strings.Contains(name, "acc test") ||
		strings.Contains(name, "acceptance test") ||
		strings.Contains(name, "terraform test") {
		return true
	}

	return false
}
