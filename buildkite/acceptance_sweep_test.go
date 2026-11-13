package buildkite

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	genqlient "github.com/Khan/genqlient/graphql"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/shurcooL/graphql"
)

func TestMain(m *testing.M) {
	resource.TestMain(m)
}

func init() {
	resource.AddTestSweepers("buildkite", &resource.Sweeper{
		Name: "buildkite",
		F: func(organization string) error {
			if organization == "" || organization != os.Getenv("BUILDKITE_ORGANIZATION_SLUG") || os.Getenv("BUILDKITE_API_TOKEN") == "" {
				return fmt.Errorf("sweep requires BUILDKITE_API_TOKEN and a -sweep organization matching BUILDKITE_ORGANIZATION_SLUG")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			defer cancel()
			return sweepAcceptanceResources(ctx, getTestClient(), time.Now().Add(-24*time.Hour))
		},
	})
}

func testAccResourceName() string {
	build, commit := os.Getenv("BUILDKITE_BUILD_NUMBER"), os.Getenv("BUILDKITE_COMMIT")
	if build == "" || commit == "" {
		return "tfacc-local-" + acctest.RandString(12)
	}
	if len(commit) > 7 {
		commit = commit[:7]
	}
	return fmt.Sprintf("tfacc-%s-%s-%s", build, commit, acctest.RandString(12))
}

func testAccResourceKey() string {
	return strings.ToUpper(strings.ReplaceAll(testAccResourceName(), "-", "_"))
}

// Only discovery metadata is decoded. In particular, sweepers never fetch secret values.
type acceptanceSweepResource struct {
	ID          string `json:"id"`
	GraphQLID   string `json:"graphql_id"`
	ClusterID   string `json:"cluster_id"`
	UUID        string `json:"uuid"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Key         string `json:"key"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
	SourceUUID  string `json:"source_uuid"`
	TargetUUID  string `json:"target_uuid"`
	Default     bool   `json:"default"`
}

func (r acceptanceSweepResource) marked() bool {
	name := r.Name
	if name == "" {
		name = r.Key
	}
	if name == "" {
		name = r.Description
	}
	return strings.HasPrefix(name, "tfacc-") || strings.HasPrefix(name, "TFACC_")
}

func (r acceptanceSweepResource) old(cutoff time.Time) (bool, error) {
	if r.CreatedAt == "" {
		return false, nil // No reliable creation time: never guess based on an ID or name.
	}
	created, err := time.Parse(time.RFC3339Nano, r.CreatedAt)
	if err != nil {
		return false, fmt.Errorf("invalid creation time for %s: %w", r.ID, err)
	}
	return created.Before(cutoff), nil
}

// Discover every page before deleting anything, so deletions cannot shift page boundaries.
// Rules and pipelines precede their clusters/templates; teams are removed last.
func sweepAcceptanceResources(ctx context.Context, client *Client, cutoff time.Time) error {
	base := "/v2/organizations/" + url.PathEscape(client.organization)
	log.Printf("Sweeping marked acceptance resources in %s created before %s", client.organization, cutoff.Format(time.RFC3339))
	pipelines, err := getAllPages[acceptanceSweepResource](ctx, client, base+"/pipelines")
	if err != nil {
		return err
	}
	clusters, err := getAllPages[acceptanceSweepResource](ctx, client, base+"/clusters")
	if err != nil {
		return err
	}

	var paths []string
	markedPipelineIDs := make(map[string]bool)
	pipelineIDs := make(map[string]bool)
	for _, p := range pipelines {
		if !p.marked() || p.ID == testAccFixtureHiddenPipelineUUID {
			continue
		}
		markedPipelineIDs[p.ID] = true
		old, err := p.old(cutoff)
		if err != nil {
			return err
		}
		if old {
			pipelineIDs[p.ID] = true
		}
	}

	rules, err := getAllPages[acceptanceSweepResource](ctx, client, base+"/rules")
	if err != nil {
		return err
	}
	for _, rule := range rules {
		if rule.UUID == testAccFixtureReadableRuleUUID || rule.UUID == testAccFixtureSourceOnlyRuleUUID || rule.UUID == testAccFixtureTargetOnlyRuleUUID {
			continue
		}
		old, err := rule.old(cutoff)
		if err != nil {
			return err
		}
		if !old {
			// Keep endpoint ownership discoverable until this Rule can also be swept.
			delete(pipelineIDs, rule.SourceUUID)
			delete(pipelineIDs, rule.TargetUUID)
			continue
		}
		// Some lifecycle tests deliberately clear the description. Their temporary
		// pipelines still identify the Rule as test-owned.
		if rule.marked() || markedPipelineIDs[rule.SourceUUID] || markedPipelineIDs[rule.TargetUUID] {
			paths = append(paths, base+"/rules/"+url.PathEscape(rule.UUID))
		}
	}

	retainedClusters := make(map[string]bool)
	webhooks := make(map[string]*webhookInfo)
	for _, p := range pipelines {
		if pipelineIDs[p.ID] {
			response, err := getPipelineWebhook(ctx, client.genqlient, p.GraphQLID)
			if err != nil {
				return err
			}
			pipeline, _ := response.GetNode().(*getPipelineWebhookNodePipeline)
			info, err := extractWebhookFromPipeline(pipeline)
			if err != nil && !errors.Is(err, ErrNoWebhook) && !errors.Is(err, ErrProviderUnknown) {
				return err
			}
			if info != nil {
				webhooks[p.GraphQLID] = info
			}
			paths = append(paths, base+"/pipelines/"+url.PathEscape(p.Slug))
		} else {
			retainedClusters[p.ClusterID] = true
		}
	}

	for _, endpoint := range []struct{ path, id string }{
		{base + "/portals", "slug"},
		{base + "/pipeline-templates", "uuid"},
	} {
		selected, err := acceptanceSweepPaths(ctx, client, endpoint.path, endpoint.id, cutoff)
		if err != nil {
			return err
		}
		paths = append(paths, selected...)
	}

	for _, endpoint := range []struct{ field, path string }{
		{"suites", "/v2/analytics/organizations/" + url.PathEscape(client.organization) + "/suites"},
		{"registries", "/v2/packages/organizations/" + url.PathEscape(client.organization) + "/registries"},
	} {
		selected, err := acceptanceGraphQLSweepPaths(ctx, client, endpoint.field, endpoint.path, cutoff)
		if err != nil {
			return err
		}
		paths = append(paths, selected...)
	}

	for _, cluster := range clusters {
		clusterPath := base + "/clusters/" + url.PathEscape(cluster.ID)
		for _, child := range []struct{ path, id string }{
			{"/queues", "id"}, {"/tokens", "id"}, {"/secrets", "id"}, {"/cache-registries", "uuid"},
		} {
			selected, err := acceptanceSweepPaths(ctx, client, clusterPath+child.path, child.id, cutoff)
			if err != nil {
				return err
			}
			paths = append(paths, selected...)
		}
		if !cluster.marked() || cluster.ID == testAccFixtureClusterUUID || retainedClusters[cluster.ID] {
			continue
		}
		old, err := cluster.old(cutoff)
		if err != nil {
			return err
		}
		if old {
			// Cluster deletion removes queues, tokens, cache registries, and the hosted
			// tenant (including images, whose API does not expose creation timestamps).
			paths = append(paths, clusterPath)
		}
	}

	for _, endpoint := range []struct{ path, id string }{
		{base + "/services", "id"}, {base + "/teams", "id"},
	} {
		selected, err := acceptanceSweepPaths(ctx, client, endpoint.path, endpoint.id, cutoff)
		if err != nil {
			return err
		}
		paths = append(paths, selected...)
	}

	// Legacy organization-wide tokens are GraphQL-only and have no cursor argument.
	var query struct {
		Organization struct {
			AgentTokens struct {
				Count int
				Edges []struct {
					Node struct {
						ID          string
						Description string
						CreatedAt   string
					}
				}
			} `graphql:"agentTokens(first: 500, revoked: false)"`
		} `graphql:"organization(slug: $slug)"`
	}
	if err := client.graphql.Query(ctx, &query, map[string]any{"slug": graphql.ID(client.organization)}); err != nil {
		return err
	}
	tokens := query.Organization.AgentTokens
	if tokens.Count != len(tokens.Edges) {
		return fmt.Errorf("agent token list is truncated: got %d of %d", len(tokens.Edges), tokens.Count)
	}
	var tokenIDs []string
	for _, edge := range tokens.Edges {
		token := acceptanceSweepResource{ID: edge.Node.ID, Description: edge.Node.Description, CreatedAt: edge.Node.CreatedAt}
		if !token.marked() {
			continue
		}
		old, err := token.old(cutoff)
		if err != nil {
			return err
		}
		if old {
			tokenIDs = append(tokenIDs, token.ID)
		}
	}

	// Repository hooks live outside Buildkite and are not cascaded by pipeline deletion.
	for id, info := range webhooks {
		log.Printf("Sweeping repository webhook for pipeline %s", id)
		if _, err := deletePipelineWebhook(ctx, client.genqlient, id, &info.Repository); err != nil && !gqlErrorContains(err, "No Buildkite webhook found for this repository") {
			return fmt.Errorf("sweep pipeline webhook %s: %w", id, err)
		}
	}
	for _, path := range paths {
		log.Printf("Sweeping %s", path)
		if err := client.makeRequest(ctx, http.MethodDelete, path, nil, nil); err != nil && !isResourceNotFoundError(err) {
			return fmt.Errorf("sweep %s: %w", path, err)
		}
	}
	for _, id := range tokenIDs {
		log.Printf("Revoking acceptance agent token %s", id)
		if _, err := revokeAgentToken(ctx, client.genqlient, id, "Revoked by acceptance test sweeper"); err != nil && !gqlErrorContains(err, "This agent registration token was already revoked") {
			return err
		}
	}
	return nil
}

// Suite and registry REST responses omit created_at. Both GraphQL connections
// expose creation times and use the same cursor-based metadata discovery.
func acceptanceGraphQLSweepPaths(ctx context.Context, client *Client, field, path string, cutoff time.Time) ([]string, error) {
	var cursor *string
	var paths []string
	for {
		var result struct {
			Organization struct {
				Resources struct {
					PageInfo struct {
						HasNextPage bool
						EndCursor   string
					}
					Edges []struct {
						Node struct {
							Name      string
							Slug      string
							CreatedAt string
						}
					}
				}
			}
		}
		// field is a fixed connection name supplied above, never user input.
		query := fmt.Sprintf(`query SweepMetadata($slug: ID!, $cursor: String) {
			organization(slug: $slug) {
				resources: %s(first: 100, after: $cursor) {
					pageInfo { hasNextPage endCursor }
					edges { node { name slug createdAt } }
				}
			}
		}`, field)
		if err := client.genqlient.MakeRequest(ctx, &genqlient.Request{
			OpName: "SweepMetadata", Query: query, Variables: map[string]any{"slug": client.organization, "cursor": cursor},
		}, &genqlient.Response{Data: &result}); err != nil {
			return nil, err
		}
		for _, edge := range result.Organization.Resources.Edges {
			item := acceptanceSweepResource{Name: edge.Node.Name, Slug: edge.Node.Slug, CreatedAt: edge.Node.CreatedAt}
			if !item.marked() {
				continue
			}
			old, err := item.old(cutoff)
			if err != nil {
				return nil, err
			}
			if old {
				paths = append(paths, path+"/"+url.PathEscape(item.Slug))
			}
		}
		page := result.Organization.Resources.PageInfo
		if !page.HasNextPage {
			break
		}
		if page.EndCursor == "" || (cursor != nil && *cursor == page.EndCursor) {
			return nil, fmt.Errorf("%s pagination did not advance", field)
		}
		cursor = &page.EndCursor
	}
	return paths, nil
}

func acceptanceSweepPaths(ctx context.Context, client *Client, path, idField string, cutoff time.Time) ([]string, error) {
	items, err := getAllPages[acceptanceSweepResource](ctx, client, path)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", path, err)
	}
	var paths []string
	for _, item := range items {
		if !item.marked() || item.Slug == testAccFixturePipelineTeamSlug || item.UUID == testAccFixtureTemplateUUID {
			continue
		}
		// The API forbids deleting a cluster's default cache registry. Temporary
		// clusters remove it during cluster deletion; persistent defaults stay intact.
		if item.Default && strings.HasSuffix(path, "/cache-registries") {
			log.Printf("Preserving default cache registry %s in %s", item.UUID, path)
			continue
		}
		old, err := item.old(cutoff)
		if err != nil {
			return nil, err
		}
		if !old {
			continue
		}
		id := item.ID
		switch idField {
		case "slug":
			id = item.Slug
		case "uuid":
			id = item.UUID
		}
		if id == "" {
			return nil, fmt.Errorf("missing %s on marked resource from %s", idField, path)
		}
		paths = append(paths, path+"/"+url.PathEscape(id))
	}
	return paths, nil
}
