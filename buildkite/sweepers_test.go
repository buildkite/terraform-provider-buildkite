package buildkite

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestTemporaryResourceNames(t *testing.T) {
	t.Setenv("BUILDKITE_BUILD_NUMBER", "5319")
	t.Setenv("BUILDKITE_COMMIT", "80c886dbb8e47fffd1a4d2e171687b56a68b8607")
	first, second := testAccResourceName(), testAccResourceName()
	if !regexp.MustCompile(`^tfacc-5319-80c886d-[a-z0-9]{12}$`).MatchString(first) || first == second {
		t.Fatalf("expected identifiable, distinct CI resource names, got %q and %q", first, second)
	}
	if key := testAccResourceKey(); !regexp.MustCompile(`^TFACC_5319_80C886D_[A-Z0-9]{12}$`).MatchString(key) {
		t.Fatalf("secret key is not identifiable or API-compatible: %q", key)
	}
	t.Setenv("BUILDKITE_BUILD_NUMBER", "")
	t.Setenv("BUILDKITE_COMMIT", "")
	if name := testAccResourceName(); !regexp.MustCompile(`^tfacc-local-[a-z0-9]{12}$`).MatchString(name) {
		t.Fatalf("local acceptance name = %q", name)
	}
}

func TestSweepAcceptanceResources(t *testing.T) {
	cutoff := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	old := cutoff.Add(-time.Second).Format(time.RFC3339)
	boundary := cutoff.Format(time.RFC3339)
	fresh := cutoff.Add(time.Second).Format(time.RFC3339)
	base := "/v2/organizations/test-org"
	marked := "tfacc-5319-80c886d-abcdefghijkl"
	for _, failure := range []string{"", "list", "delete", "partial-delete", "timestamp", "truncated-tokens", "webhook-delete", "webhook-absent", "revoked-token"} {
		t.Run("failure="+failure, func(t *testing.T) {
			var deleted []string
			removed := make(map[string]bool)
			deleteFailed := false
			webhookRemoved := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodDelete {
					if failure == "delete" || (failure == "partial-delete" && len(deleted) == 1 && !deleteFailed) {
						deleteFailed = true
						http.Error(w, `{"message":"delete forbidden"}`, http.StatusForbidden)
						return
					}
					if r.URL.Path == base+"/clusters/old-cluster" && (!removed[base+"/pipelines/old"] || !removed[base+"/pipelines/old2"]) {
						http.Error(w, `{"message":"Cluster has one or more pipelines"}`, http.StatusUnprocessableEntity)
						return
					}
					if removed[r.URL.Path] {
						http.Error(w, `{"message":"Not found"}`, http.StatusNotFound)
						return
					}
					removed[r.URL.Path] = true
					deleted = append(deleted, r.URL.Path)
					w.WriteHeader(http.StatusNoContent)
					return
				}
				if r.URL.Path == "/graphql" {
					var request struct {
						Query     string         `json:"query"`
						Variables map[string]any `json:"variables"`
					}
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
						return
					}
					if strings.Contains(request.Query, "first:1000") || strings.Contains(request.Query, "first: 1000") {
						fmt.Fprint(w, `{"errors":[{"message":"Maximum value of first is 500"}]}`)
						return
					}
					if strings.Contains(request.Query, "getPipelineWebhook") {
						if request.Variables["id"] == "pipeline-old" {
							var webhook any
							if !webhookRemoved {
								webhook = map[string]string{"externalId": "hook", "url": "https://webhook.buildkite.com/example"}
							}
							_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"node": map[string]any{
								"__typename": "Pipeline", "repository": map[string]any{"url": "https://github.com/example/repo", "provider": map[string]any{"__typename": "RepositoryProviderGithub", "webhook": webhook}},
							}}})
						} else {
							fmt.Fprint(w, `{"data":{"node":{"__typename":"Pipeline","repository":{"url":"https://gitlab.com/example/repo","provider":{"__typename":"RepositoryProviderGitlab"}}}}}`)
						}
						return
					}
					if strings.Contains(request.Query, "pipelineDeleteWebhook") {
						if removed[base+"/pipelines/old"] {
							fmt.Fprint(w, `{"errors":[{"message":"No pipeline found"}]}`)
							return
						}
						if request.Variables["pipelineId"] != "pipeline-old" || request.Variables["repository"] != "https://github.com/example/repo" {
							t.Errorf("unexpected webhook deletion: %v", request.Variables)
						}
						if failure == "webhook-delete" {
							fmt.Fprint(w, `{"errors":[{"message":"Repository not found or not accessible by the GitHub App."}]}`)
							return
						}
						webhookRemoved = true
						if failure == "webhook-absent" {
							fmt.Fprint(w, `{"errors":[{"message":"No Buildkite webhook found for this repository"}]}`)
						} else {
							fmt.Fprint(w, `{"data":{"pipelineDeleteWebhook":{"pipeline":{"id":"pipeline-old"},"deletedWebhookExternalId":"hook"}}}`)
						}
						return
					}
					if strings.Contains(request.Query, "suites(") {
						_, _ = fmt.Fprintf(w, `{"data":{"organization":{"resources":{"pageInfo":{"hasNextPage":false,"endCursor":""},"edges":[{"node":{"slug":"old-suite","name":%q,"createdAt":%q}}]}}}}`, marked, old)
						return
					}
					if strings.Contains(request.Query, "registries(") {
						nextPage := request.Variables["cursor"] == nil
						slug := "old-registry"
						if !nextPage {
							if request.Variables["cursor"] != "registry-page-2" {
								t.Errorf("unexpected registry cursor: %v", request.Variables["cursor"])
							}
							slug = "old-registry-2"
						}
						_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"organization": map[string]any{"resources": map[string]any{
							"pageInfo": map[string]any{"hasNextPage": nextPage, "endCursor": "registry-page-2"},
							"edges":    []any{map[string]any{"node": map[string]any{"slug": slug, "name": marked, "createdAt": old}}},
						}}}})
						return
					}
					if strings.Contains(request.Query, "agentTokenRevoke") {
						if request.Variables["id"] != "old-token" {
							t.Errorf("revoked unexpected token: %v", request.Variables)
						}
						if removed["token:old-token"] {
							fmt.Fprint(w, `{"errors":[{"message":"This agent registration token was already revoked by Test User"}]}`)
							return
						}
						removed["token:old-token"] = true
						deleted = append(deleted, "token:old-token")
						if failure == "revoked-token" {
							http.Error(w, `{"message":"upstream response lost"}`, http.StatusServiceUnavailable)
							return
						}
						fmt.Fprint(w, `{"data":{"agentTokenRevoke":{"agentToken":{"id":"old-token"}}}}`)
						return
					}
					count := 2
					if failure == "truncated-tokens" {
						count = 3
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"organization": map[string]any{"agentTokens": map[string]any{
						"count": count, "edges": []any{
							map[string]any{"node": map[string]any{"id": "old-token", "description": marked, "createdAt": old}},
							map[string]any{"node": map[string]any{"id": "fresh-token", "description": marked, "createdAt": fresh}},
						},
					}}}})
					return
				}
				if r.Method != http.MethodGet {
					t.Errorf("unexpected %s %s", r.Method, r.URL)
				}
				if r.URL.Path == base+"/pipelines" {
					if failure == "list" {
						http.Error(w, `{"message":"list forbidden"}`, http.StatusForbidden)
						return
					}
					if r.URL.Query().Get("page") == "2" {
						_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "old2", "graphql_id": "pipeline-old2", "slug": "old2", "name": marked + "-second", "created_at": old}})
						return
					}
					w.Header().Set("Link", "<"+r.URL.Path+"?page=2&per_page=100>; rel=\"next\"")
					timestamp := old
					if failure == "timestamp" {
						timestamp = "not-a-date"
					}
					_ = json.NewEncoder(w).Encode([]map[string]any{
						{"id": "old", "graphql_id": "pipeline-old", "slug": "old", "name": marked, "created_at": timestamp},
						{"id": "boundary", "slug": "boundary", "name": marked + "-boundary", "created_at": boundary},
						{"id": "fresh", "slug": "fresh", "name": marked + "-fresh", "created_at": fresh},
						{"id": "unknown-age", "slug": "unknown-age", "name": marked + "-unknown-age"},
						{"id": testAccFixtureHiddenPipelineUUID, "slug": "renamed-fixture", "name": marked, "created_at": old},
						{"slug": "legacy", "name": "abcdefghijkl", "created_at": old},
						{"slug": "fixture", "name": "Terraform Provider Non-admin Rules Source", "created_at": old},
					})
					return
				}
				if r.URL.Path == base+"/clusters" {
					_ = json.NewEncoder(w).Encode([]map[string]any{
						{"id": testAccFixtureClusterUUID, "name": marked + "-renamed-fixture", "created_at": old},
						{"id": "old-cluster", "name": marked + "-cluster", "created_at": old},
					})
					return
				}
				fixtures := map[string][]map[string]any{
					base + "/rules": {
						{"uuid": "old-rule", "source_uuid": "old", "created_at": old},
						{"uuid": "marked-rule", "description": marked, "created_at": old},
						{"uuid": testAccFixtureReadableRuleUUID, "description": marked, "created_at": old},
						{"uuid": "persistent-rule", "description": "Persistent rule", "created_at": old},
					},
					base + "/portals": {{"slug": "old-portal", "name": marked, "created_at": old}},
					base + "/pipeline-templates": {
						{"uuid": "old-template", "name": marked, "created_at": old},
						{"uuid": testAccFixtureTemplateUUID, "name": marked, "created_at": old},
					},
					"/v2/analytics/organizations/test-org/suites": {{"slug": "old-suite", "name": marked}},
					base + "/services": {
						{"id": "old-service", "description": marked, "created_at": old},
					},
					base + "/teams": {
						{"id": "old-team", "slug": "not-the-id", "name": marked, "created_at": old},
						{"id": "fixture-team", "slug": testAccFixturePipelineTeamSlug, "name": marked, "created_at": old},
					},
					base + "/clusters/" + testAccFixtureClusterUUID + "/queues": {{"id": "old-queue", "key": marked, "created_at": old}},
					base + "/clusters/" + testAccFixtureClusterUUID + "/tokens": {{"id": "old-cluster-token", "description": marked, "created_at": old}},
				}
				if rows, ok := fixtures[r.URL.Path]; ok {
					if r.URL.Path == base+"/services" {
						_ = json.NewEncoder(w).Encode(map[string]any{"items": rows, "links": map[string]string{"self": r.URL.String()}})
					} else {
						_ = json.NewEncoder(w).Encode(rows)
					}
					return
				}
				if r.URL.Path == base+"/clusters/"+testAccFixtureClusterUUID+"/secrets" {
					_ = json.NewEncoder(w).Encode([]map[string]any{
						{"id": "old-secret", "key": "TFACC_5319_80C886D_ABCDEFGHIJKL", "created_at": old},
						{"id": "fixture-secret", "key": "PERSISTENT_SECRET", "created_at": old},
						{"id": "fresh-secret", "key": "TFACC_5319_80C886D_FRESH", "created_at": fresh},
					})
					return
				}
				if strings.HasSuffix(r.URL.Path, "/cache-registries") {
					items := []map[string]any{}
					links := map[string]string{"self": r.URL.String()}
					if strings.Contains(r.URL.Path, testAccFixtureClusterUUID) {
						if r.URL.Query().Get("after") == "cache-page-2" {
							items = append(items, map[string]any{"uuid": "old-cache", "name": marked, "created_at": old})
						} else {
							items = append(items, map[string]any{"uuid": "default-cache", "name": marked, "default": true, "created_at": old})
							links["next"] = r.URL.Path + "?after=cache-page-2&per_page=100"
							w.Header().Set("Link", "<"+links["next"]+">; rel=\"next\"")
						}
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"items": items, "links": links})
					return
				}
				fmt.Fprint(w, `[]`)
			}))
			defer server.Close()
			t.Setenv("BUILDKITE_REST_URL", server.URL)
			t.Setenv("BUILDKITE_GRAPHQL_URL", server.URL+"/graphql")
			t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "test-org")
			client := getTestClientWithToken("test-token")
			if client.graphqlRetry != nil {
				client.graphqlRetry.RetryWaitMin = time.Millisecond
				client.graphqlRetry.RetryWaitMax = time.Millisecond
			}
			err := sweepAcceptanceResources(context.Background(), client, cutoff)
			if failure == "partial-delete" {
				if err == nil || !strings.Contains(err.Error(), "delete forbidden") || len(deleted) != 1 {
					t.Fatalf("expected one deletion before failure, got deletions %v and error %v", deleted, err)
				}
				err = sweepAcceptanceResources(context.Background(), client, cutoff)
			}
			wantError := map[string]string{"list": "list forbidden", "delete": "delete forbidden", "timestamp": "not-a-date", "truncated-tokens": "truncated", "webhook-delete": "Repository not found"}[failure]
			if wantError != "" {
				if err == nil || !strings.Contains(err.Error(), wantError) {
					t.Fatalf("sweep error = %v, expected %s failure", err, failure)
				}
				if len(deleted) != 0 {
					t.Fatalf("sweep mutated resources despite failed discovery or first deletion: %v", deleted)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !webhookRemoved {
				t.Fatal("pipeline repository webhook was orphaned")
			}
			want := []string{
				base + "/rules/old-rule", base + "/rules/marked-rule",
				base + "/pipelines/old", base + "/pipelines/old2",
				base + "/portals/old-portal", base + "/pipeline-templates/old-template",
				"/v2/analytics/organizations/test-org/suites/old-suite",
				"/v2/packages/organizations/test-org/registries/old-registry",
				"/v2/packages/organizations/test-org/registries/old-registry-2",
				base + "/clusters/" + testAccFixtureClusterUUID + "/queues/old-queue",
				base + "/clusters/" + testAccFixtureClusterUUID + "/tokens/old-cluster-token",
				base + "/clusters/" + testAccFixtureClusterUUID + "/secrets/old-secret",
				base + "/clusters/" + testAccFixtureClusterUUID + "/cache-registries/old-cache",
				base + "/clusters/old-cluster", base + "/services/old-service", base + "/teams/old-team", "token:old-token",
			}
			sort.Strings(deleted)
			sort.Strings(want)
			if !reflect.DeepEqual(deleted, want) {
				t.Fatalf("deleted = %v, want only old marked objects %v", deleted, want)
			}
		})
	}
}

func TestSweepPreservesYoungRuleOwnership(t *testing.T) {
	cutoff := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	old := cutoff.Add(-time.Second).Format(time.RFC3339)
	base := "/v2/organizations/test-org"
	var deleted []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodDelete {
			deleted = append(deleted, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		switch r.URL.Path {
		case base + "/pipelines":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": "source", "graphql_id": "pipeline-source", "slug": "source", "name": "tfacc-source", "cluster_id": "cluster", "created_at": old},
				{"id": "target", "graphql_id": "pipeline-target", "slug": "target", "name": "tfacc-target", "cluster_id": "cluster", "created_at": old},
			})
		case base + "/clusters":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "cluster", "name": "tfacc-cluster", "created_at": old}})
		case base + "/rules":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"uuid": "rule", "source_uuid": "source", "target_uuid": "target", "created_at": cutoff.Add(time.Second).Format(time.RFC3339)}})
		case "/graphql":
			var request struct{ Query string }
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
				return
			}
			switch {
			case strings.Contains(request.Query, "agentTokens"):
				fmt.Fprint(w, `{"data":{"organization":{"agentTokens":{"count":0,"edges":[]}}}}`)
			case strings.Contains(request.Query, "getPipelineWebhook"):
				fmt.Fprint(w, `{"data":{"node":{"__typename":"Pipeline","repository":{"url":"https://github.com/example/repo","provider":{"__typename":"RepositoryProviderGithub","webhook":null}}}}}`)
			default:
				fmt.Fprint(w, `{"data":{"organization":{"resources":{"pageInfo":{"hasNextPage":false},"edges":[]}}}}`)
			}
		default:
			if strings.HasSuffix(r.URL.Path, "/cache-registries") || r.URL.Path == base+"/services" {
				fmt.Fprint(w, `{"items":[],"links":{}}`)
			} else {
				fmt.Fprint(w, `[]`)
			}
		}
	}))
	defer server.Close()
	t.Setenv("BUILDKITE_REST_URL", server.URL)
	t.Setenv("BUILDKITE_GRAPHQL_URL", server.URL+"/graphql")
	t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "test-org")
	client := getTestClientWithToken("test-token")
	if err := sweepAcceptanceResources(context.Background(), client, cutoff); err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 0 {
		t.Fatalf("young Rule lost its pipeline or cluster ownership: %v", deleted)
	}
	if err := sweepAcceptanceResources(context.Background(), client, cutoff.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	want := []string{base + "/rules/rule", base + "/pipelines/source", base + "/pipelines/target", base + "/clusters/cluster"}
	sort.Strings(deleted)
	sort.Strings(want)
	if !reflect.DeepEqual(deleted, want) {
		t.Fatalf("later sweep deleted %v, want complete Rule and parent cleanup %v", deleted, want)
	}
}
