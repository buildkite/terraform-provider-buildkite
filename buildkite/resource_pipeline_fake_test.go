package buildkite

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

const (
	fakePipelineGraphQLID  = "pipeline-graphql-id"
	fakePipelineUUID       = "9d1b2c3e-4444-5555-6666-777788889999"
	fakePipelineWebhookURL = "https://webhook.buildkite.com/deliver/fake"
)

// fakePipelineAPI models one pipeline behind the GraphQL operations the pipeline resource makes,
// echoing the fields each create or update sends. What it varies is whether the API lets the caller
// see the pipeline's webhook URL, which Buildkite withholds from a token that may not edit the
// pipeline: a plan refreshed with such a token must not hold the apply, made with one that sees the
// URL, to an empty one.
type fakePipelineAPI struct {
	t      *testing.T
	mu     sync.Mutex
	exists bool
	// input holds the fields of the last create or update, under the names the API takes
	input map[string]json.RawMessage
	// hideWebhookURL withholds the URL from every operation, as the API does from a read-only token;
	// hideWebhookURLOnRead from reads only, as it does when a read-only token refreshes and a write
	// token applies.
	hideWebhookURL       bool
	hideWebhookURLOnRead bool
}

func newFakePipelineAPI(t *testing.T) (*httptest.Server, *fakePipelineAPI) {
	t.Helper()

	api := &fakePipelineAPI{t: t, input: map[string]json.RawMessage{}}
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)

	return server, api
}

func (a *fakePipelineAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if r.URL.Path != "/graphql" {
		a.t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
		return
	}

	var body struct {
		OperationName string                     `json:"operationName"`
		Variables     map[string]json.RawMessage `json:"variables"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		a.t.Errorf("unable to read the GraphQL request: %v", err)
		return
	}

	// the organization id is fetched with an anonymous query
	response := `{"data":{"organization":{"id":"organization-graphql-id"}}}`
	switch body.OperationName {
	case "createPipeline", "updatePipeline":
		var input map[string]json.RawMessage
		if err := json.Unmarshal(body.Variables["input"], &input); err != nil {
			a.t.Errorf("input = %s, want an object", body.Variables["input"])
		}
		for key, value := range input {
			a.input[key] = value
		}
		a.exists = true
		field := "pipelineUpdate"
		if body.OperationName == "createPipeline" {
			field = "pipelineCreate"
		}
		response = fmt.Sprintf(`{"data":{%q:{"pipeline":%s}}}`, field, a.pipelineJSON(a.hideWebhookURL))
	case "getNode":
		node := "null"
		if a.exists {
			node = a.pipelineJSON(a.hideWebhookURL || a.hideWebhookURLOnRead)
		}
		response = fmt.Sprintf(`{"data":{"node":%s}}`, node)
	case "deletePipeline":
		a.exists = false
		response = `{"data":{"pipelineDelete":{"clientMutationId":null}}}`
	}

	if _, err := io.WriteString(w, response); err != nil {
		a.t.Errorf("unable to write the GraphQL response: %v", err)
	}
}

// pipelineJSON answers with the fields the last write sent, the API's defaults for the rest, and the
// webhook URL unless withheld
func (a *fakePipelineAPI) pipelineJSON(hideWebhookURL bool) string {
	sent := func(key string, fallback any) any {
		raw, ok := a.input[key]
		if !ok {
			return fallback
		}
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			a.t.Errorf("%s = %s, want JSON", key, raw)
		}
		return value
	}
	var name string
	if raw, ok := a.input["name"]; ok {
		if err := json.Unmarshal(raw, &name); err != nil {
			a.t.Errorf("name = %s, want a string", raw)
		}
	}
	var webhookURL any = fakePipelineWebhookURL
	if hideWebhookURL {
		webhookURL = nil
	}
	pipeline, err := json.Marshal(map[string]any{
		"__typename":                           "Pipeline",
		"id":                                   fakePipelineGraphQLID,
		"pipelineUuid":                         fakePipelineUUID,
		"allowRebuilds":                        sent("allowRebuilds", true),
		"badgeURL":                             "https://badge.buildkite.com/fake.svg",
		"branchConfiguration":                  sent("branchConfiguration", nil),
		"cancelIntermediateBuilds":             sent("cancelIntermediateBuilds", false),
		"cancelIntermediateBuildsBranchFilter": sent("cancelIntermediateBuildsBranchFilter", ""),
		"cloneMirrorUrl":                       sent("cloneMirrorUrl", nil),
		"cluster":                              nil,
		"color":                                sent("color", nil),
		"defaultBranch":                        sent("defaultBranch", ""),
		"defaultTimeoutInMinutes":              sent("defaultTimeoutInMinutes", nil),
		"emoji":                                sent("emoji", nil),
		"maximumTimeoutInMinutes":              sent("maximumTimeoutInMinutes", nil),
		"description":                          sent("description", ""),
		"name":                                 name,
		"repository":                           sent("repository", map[string]any{"url": ""}),
		"pipelineTemplate":                     nil,
		"skipIntermediateBuilds":               sent("skipIntermediateBuilds", false),
		"skipIntermediateBuildsBranchFilter":   sent("skipIntermediateBuildsBranchFilter", ""),
		"slug":                                 strings.ToLower(name),
		"steps":                                sent("steps", map[string]any{"yaml": ""}),
		"tags":                                 sent("tags", []any{}),
		"teams":                                map[string]any{"pageInfo": map[string]any{"endCursor": "", "hasNextPage": false}, "count": 0, "edges": []any{}},
		"archived":                             sent("archived", false),
		"visibility":                           sent("visibility", "PRIVATE"),
		"webhookURL":                           webhookURL,
	})
	if err != nil {
		a.t.Fatalf("unable to encode the pipeline: %v", err)
	}
	return string(pipeline)
}

// TestUnitBuildkitePipelineResourceWebhookURLWithheldFromRefresh follows a pipeline whose webhook
// URL the API withholds from the token that refreshes it, as Buildkite does from a read-only one,
// while the token that applies sees it. The apply must not be held to the empty value such a refresh
// gets, and must keep the URL once an update has stored it.
func TestUnitBuildkitePipelineResourceWebhookURLWithheldFromRefresh(t *testing.T) {
	server, api := newFakePipelineAPI(t)

	config := func(description string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			organization = "acme"
			api_token    = "fake"
			rest_url     = %q
			graphql_url  = %q
		}

		resource "buildkite_pipeline" "pipeline" {
			name        = "fake"
			repository  = "https://github.com/acme/fake.git"
			description = %q
		}
		`, server.URL, server.URL+"/graphql", description)
	}

	const name = "buildkite_pipeline.pipeline"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				// a pipeline every operation withholds the URL from, which is what state holds after
				// a read-only token's refresh was applied
				PreConfig: func() { api.hideWebhookURL = true },
				Config:    config("one"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.TestCheckResourceAttr(name, "webhook_url", ""),
			},
			{
				// an update planned from that state leaves the URL unknown, so the apply, which sees
				// it, is consistent with the plan and stores it
				PreConfig: func() { api.hideWebhookURL = false; api.hideWebhookURLOnRead = true },
				Config:    config("two"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             []plancheck.PlanCheck{plancheck.ExpectUnknownValue(name, tfjsonpath.New("webhook_url"))},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.TestCheckResourceAttr(name, "webhook_url", fakePipelineWebhookURL),
			},
			{
				// a refresh withheld the URL again, which keeps the one stored: the plan holds it, and
				// the apply returns the same
				Config: config("three"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             []plancheck.PlanCheck{plancheck.ExpectKnownValue(name, tfjsonpath.New("webhook_url"), knownvalue.StringExact(fakePipelineWebhookURL))},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.TestCheckResourceAttr(name, "webhook_url", fakePipelineWebhookURL),
			},
		},
	})
}
