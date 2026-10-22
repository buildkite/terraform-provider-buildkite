package buildkite

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// GraphQL requests do not pass through makeRequest, and the retry client's HTTPClient.Timeout only
// bounds each attempt, so a GraphQL call with no deadline ran the whole retry schedule: just under
// 56 minutes whatever the timeouts block said. These tests drive calls against a server that never
// recovers and assert they stop at the configured timeout.

// The stub shortens the waits so the two outcomes are far apart but the test runs in about a
// second. Outcomes are told apart by request count rather than wall clock, because a loaded machine
// only makes the bounded call slower, so fewer attempts fit, while the unbounded call reaches
// stubUnboundedAttempts regardless. 30 retries at 250ms is 7.5s of waiting against a 1s timeout;
// stubBoundedAttempts sits well above the four or five that fit in the timeout and well below that.
const (
	stubRetryWait         = 250 * time.Millisecond
	stubReadTimeout       = 1 * time.Second
	stubUnboundedAttempts = 31
	stubRetries           = stubUnboundedAttempts - 1
	stubBoundedAttempts   = 10
)

// newGraphQLTimeoutTestClient is newRetryTestClient with the GraphQL waits shortened as well and the
// given timeouts configured.
func newGraphQLTimeoutTestClient(t *testing.T, serverURL string, maxRetries int, wait time.Duration, configured timeouts.Value) *Client {
	t.Helper()

	client := newRetryTestClient(t, serverURL, maxRetries, wait)
	client.graphqlRetry.RetryWaitMin = wait
	client.graphqlRetry.RetryWaitMax = wait
	client.timeouts = configured

	return client
}

func assertStoppedAtTheTimeout(t *testing.T, requests int64, hitDeadline bool, failure any) {
	t.Helper()

	if requests < 1 {
		t.Fatal("Expected the call to reach the server at least once")
	}
	if requests > stubBoundedAttempts {
		t.Errorf("Made %d requests against a %s timeout, so the call ran the retry schedule (%d attempts) instead", requests, stubReadTimeout, stubUnboundedAttempts)
	}
	// With retries exhausted the stub's 503 would surface instead, so the deadline error is what shows
	// the call stopped at its timeout rather than for some other reason.
	if !hitDeadline {
		t.Errorf("Failed with %v, want a context deadline error", failure)
	}
}

// datasourceConfigFor builds a config for d in which every attribute is null except those in set.
func datasourceConfigFor(ctx context.Context, t *testing.T, d datasource.DataSource, set map[string]tftypes.Value) tfsdk.Config {
	t.Helper()

	datasourceSchema := datasourceSchema(ctx, t, d)

	return tfsdk.Config{Schema: datasourceSchema, Raw: nullObjectWith(ctx, t, datasourceSchema.Type(), set)}
}

// The floor under every GraphQL request: one made with no deadline at all still stops at the read
// timeout.
func TestGraphQLRequestWithoutADeadlineStopsAtTheReadTimeout(t *testing.T) {
	t.Parallel()

	server, requests := newRetryStub(t, stubResponse{status: http.StatusServiceUnavailable, body: `{"errors":[{"message":"unavailable"}]}`})
	defer server.Close()

	client := newGraphQLTimeoutTestClient(t, server.URL, stubRetries, stubRetryWait, configuredTimeouts("read", stubReadTimeout.String()))

	_, err := getOrganization(context.Background(), client.genqlient, client.organization)
	if err == nil {
		t.Fatal("getOrganization succeeded against a server that only fails")
	}
	assertStoppedAtTheTimeout(t, requests.Load(), strings.Contains(err.Error(), "context deadline exceeded"), err)
}

// The organization lookup runs under a mutex, so an unbounded one would also block every other
// resource waiting on the ID.
func TestGetOrganizationIDWithoutADeadlineStopsAtTheReadTimeout(t *testing.T) {
	t.Parallel()

	server, requests := newRetryStub(t, stubResponse{status: http.StatusServiceUnavailable, body: `{"message":"unavailable"}`})
	defer server.Close()

	client := newGraphQLTimeoutTestClient(t, server.URL, stubRetries, stubRetryWait, configuredTimeouts("read", stubReadTimeout.String()))

	_, err := client.GetOrganizationID(context.Background())
	if err == nil {
		t.Fatal("GetOrganizationID succeeded against a server that only fails")
	}
	assertStoppedAtTheTimeout(t, requests.Load(), strings.Contains(err.Error(), "context deadline exceeded"), err)
}

// Every data source stops at the read timeout against a server that never recovers. The cluster
// data source is the one a customer hit, which ran for 55m51s against a network path that kept
// resetting. Each case fails on its first request, so the client's own read-timeout fallback would
// also stop it; whether a multi-call read shares one budget is checked by
// TestOperationsGiveEachRequestItsBudget.
func TestDatasourceReadsStopAtTheReadTimeout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source func(*Client) datasource.DataSource
		config map[string]tftypes.Value
	}{
		{
			name:   "cluster",
			source: func(c *Client) datasource.DataSource { return &clusterDatasource{client: c} },
			config: map[string]tftypes.Value{"name": tftypes.NewValue(tftypes.String, "some-cluster")},
		},
		{
			name:   "clusters",
			source: func(c *Client) datasource.DataSource { return &clustersDatasource{client: c} },
		},
		{
			name:   "organization",
			source: func(c *Client) datasource.DataSource { return &organizationDatasource{client: c} },
		},
		{
			name:   "organization member",
			source: func(c *Client) datasource.DataSource { return &organizationMemberDatasource{client: c} },
			config: map[string]tftypes.Value{"email": tftypes.NewValue(tftypes.String, "someone@example.com")},
		},
		{
			name:   "organization members",
			source: func(c *Client) datasource.DataSource { return &organizationMembersDatasource{client: c} },
		},
		{
			name:   "pipeline",
			source: func(c *Client) datasource.DataSource { return &pipelineDatasource{client: c} },
			config: map[string]tftypes.Value{"slug": tftypes.NewValue(tftypes.String, "some-pipeline")},
		},
		{
			name:   "team by id",
			source: func(c *Client) datasource.DataSource { return &teamDatasource{client: c} },
			config: map[string]tftypes.Value{"id": tftypes.NewValue(tftypes.String, "team-id")},
		},
		{
			name:   "team by slug",
			source: func(c *Client) datasource.DataSource { return &teamDatasource{client: c} },
			config: map[string]tftypes.Value{"slug": tftypes.NewValue(tftypes.String, "some-team")},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server, requests := newRetryStub(t, stubResponse{status: http.StatusServiceUnavailable, body: `{"message":"unavailable"}`})
			defer server.Close()

			ctx := t.Context()
			client := newGraphQLTimeoutTestClient(t, server.URL, stubRetries, stubRetryWait, configuredTimeouts("read", stubReadTimeout.String()))
			// Primed so the organization lookup some of these make first is not what times out.
			organizationID := "organization-id"
			client.organizationId = &organizationID
			ds := testCase.source(client)
			req := datasource.ReadRequest{Config: datasourceConfigFor(ctx, t, ds, testCase.config)}
			resp := datasource.ReadResponse{State: tfsdk.State{Schema: req.Config.Schema}}

			ds.Read(ctx, req, &resp)

			if !resp.Diagnostics.HasError() {
				t.Fatal("Read succeeded against a server that only fails")
			}
			assertStoppedAtTheTimeout(t, requests.Load(), diagnosticsContain(resp.Diagnostics, "context deadline exceeded"), resp.Diagnostics)
		})
	}
}

// A create whose response arrives after the create timeout, but within the grace period
// retry.RetryContext waits for the attempt in flight, still records state. Cancelling it at the
// timeout instead left the team in Buildkite with nothing in state, and the next apply failed on
// the name being taken.
func TestResourceCreateRecordsAMutationThatLandsJustAfterTheTimeout(t *testing.T) {
	t.Parallel()

	const createTimeout = 500 * time.Millisecond

	server, requests := newRetryStub(t, stubResponse{
		status: http.StatusOK,
		body:   `{"data":{"teamCreate":{"teamEdge":{"node":{"id":"team-id","uuid":"team-uuid","slug":"team"}}}}}`,
		delay:  3 * createTimeout,
	})
	defer server.Close()

	client := newGraphQLTimeoutTestClient(t, server.URL, 0, time.Millisecond, configuredTimeouts("create", createTimeout.String()))
	organizationID := "organization-id"
	client.organizationId = &organizationID

	ctx := t.Context()
	r := &teamResource{client: client}
	schema := resourceSchema(ctx, t, r)
	plan := nullObjectWith(ctx, t, schema.Type(), map[string]tftypes.Value{
		"name":                tftypes.NewValue(tftypes.String, "team"),
		"privacy":             tftypes.NewValue(tftypes.String, "VISIBLE"),
		"default_member_role": tftypes.NewValue(tftypes.String, "MEMBER"),
		"default_team":        tftypes.NewValue(tftypes.Bool, false),
	})
	resp := fwresource.CreateResponse{State: tfsdk.State{Schema: schema, Raw: tftypes.NewValue(schema.Type().TerraformType(ctx), nil)}}

	r.Create(ctx, fwresource.CreateRequest{Plan: tfsdk.Plan{Schema: schema, Raw: plan}}, &resp)

	if got := requests.Load(); got != 1 {
		t.Fatalf("Made %d requests, want 1", got)
	}
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create() diagnostics = %v, want the team that was created recorded", resp.Diagnostics)
	}
	var state teamResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("Reading the recorded state = %v", diags)
	}
	if got := state.ID.ValueString(); got != "team-id" {
		t.Errorf("Recorded id = %q, want %q", got, "team-id")
	}
}

// A cache registry create that outlasts the read timeout but not its own create timeout is
// recorded. Its calls used to carry no deadline, so the client's read-timeout fallback cancelled the
// mutation at the read timeout, after it had applied in Buildkite, and left nothing in state.
func TestClusterCacheRegistryCreateIsBoundedByTheCreateTimeout(t *testing.T) {
	t.Parallel()

	const (
		readTimeout   = time.Second
		createTimeout = 10 * time.Second
		retryWait     = 600 * time.Millisecond
	)

	_, api := newCacheRegistryTestAPI(t)
	api.retryOperation = "createCacheRegistry"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
		}
		// The 503 comes back at once and the retry's success takes retryWait, so the mutation lands
		// at about twice retryWait: past the read timeout and well inside the create timeout.
		api.mu.Lock()
		retried := api.retries > 0
		api.mu.Unlock()
		if retried && strings.Contains(string(body), `"operationName":"createCacheRegistry"`) {
			time.Sleep(retryWait)
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer dummy")
		api.ServeHTTP(w, r)
	}))
	defer server.Close()

	client := newGraphQLTimeoutTestClient(t, server.URL, 1, retryWait, configuredTimeouts("read", readTimeout.String(), "create", createTimeout.String()))

	ctx := t.Context()
	r := &clusterCacheRegistryResource{client: client}
	schema := resourceSchema(ctx, t, r)
	plan := nullObjectWith(ctx, t, schema.Type(), map[string]tftypes.Value{
		"cluster_id": tftypes.NewValue(tftypes.String, "cluster-id"),
		"name":       tftypes.NewValue(tftypes.String, "Cache"),
	})
	resp := fwresource.CreateResponse{State: tfsdk.State{Schema: schema, Raw: tftypes.NewValue(schema.Type().TerraformType(ctx), nil)}}

	r.Create(ctx, fwresource.CreateRequest{Plan: tfsdk.Plan{Schema: schema, Raw: plan}}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Create() diagnostics = %v, want the registry that was created recorded", resp.Diagnostics)
	}
	var state clusterCacheRegistryResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("Reading the recorded state = %v", diags)
	}
	if state.ID.ValueString() == "" {
		t.Error("Recorded no id, so the registry is orphaned and the next apply creates a second one")
	}

	api.mu.Lock()
	defer api.mu.Unlock()
	if got := api.operations["createCacheRegistry"]; got != 2 {
		t.Errorf("Sent createCacheRegistry %d times, want the 503 and its retry", got)
	}
}

// recordedRequest is one HTTP attempt as the transport saw it: the deadline its context carried,
// when it was sent, and when its response came back.
type recordedRequest struct {
	method, path   string
	deadline       time.Time
	sent, returned time.Time
}

type requestLog struct {
	mu       sync.Mutex
	requests []recordedRequest
}

type recordingTransport struct {
	next http.RoundTripper
	log  *requestLog
}

func (r recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	sent := time.Now()
	deadline, _ := req.Context().Deadline()
	resp, err := r.next.RoundTrip(req)

	r.log.mu.Lock()
	defer r.log.mu.Unlock()
	r.log.requests = append(r.log.requests, recordedRequest{method: req.Method, path: req.URL.Path, deadline: deadline, sent: sent, returned: time.Now()})

	return resp, err
}

// recordRequests logs every GraphQL and REST attempt the client makes, in order. It sits under each
// retry client's per-attempt HTTPClient.Timeout, which clips the deadline it sees to that timeout
// (DefaultTimeout, since the test clients configure timeouts after NewClient), so the budgets a test
// expects have to stay below it.
func recordRequests(client *Client) *requestLog {
	log := &requestLog{}
	client.graphqlRetry.HTTPClient.Transport = recordingTransport{next: client.graphqlRetry.HTTPClient.Transport, log: log}
	client.restRetry.HTTPClient.Transport = recordingTransport{next: client.restRetry.HTTPClient.Transport, log: log}

	return log
}

// budget is the deadline one request should carry: how long it was given, and whether that time
// was started for its own step or carried over from the request before it.
type budget struct {
	length time.Duration
	fresh  bool
}

// assertBudgets checks each request's deadline against what it should have been given. A step's
// context is created before its first request is sent and after the previous step's response has
// come back, so a fresh budget ends between those two points plus its length, and a carried one
// ends exactly where the previous request's did. Both checks are exact rather than timing tolerant.
func assertBudgets(t *testing.T, started time.Time, requests []recordedRequest, want []budget) {
	t.Helper()

	if len(requests) != len(want) {
		var made []string
		for _, r := range requests {
			made = append(made, r.method+" "+r.path)
		}
		t.Fatalf("Made %d requests %v, want %d", len(requests), made, len(want))
	}

	previousReturned := started
	for i, request := range requests {
		name := fmt.Sprintf("Request %d (%s %s)", i+1, request.method, request.path)
		switch {
		case request.deadline.IsZero():
			t.Errorf("%s had no deadline", name)
		case request.deadline.After(request.sent.Add(want[i].length)):
			t.Errorf("%s had %s left when sent, more than its %s budget", name, request.deadline.Sub(request.sent).Round(time.Second), want[i].length)
		case want[i].fresh && request.deadline.Before(previousReturned.Add(want[i].length)):
			t.Errorf("%s had a budget that started before the previous request came back, want a fresh %s", name, want[i].length)
		case !want[i].fresh && !request.deadline.Equal(requests[i-1].deadline):
			t.Errorf("%s had its own deadline, want the one request %d had", name, i)
		}
		previousReturned = request.returned
	}
}

// Every request runs under its operation's own timeout. Reads share one budget across their calls;
// each step of a create, update or delete that changes something gets a fresh budget with
// mutationGracePeriod on top, so a slow step cannot leave the steps after it short; and a step that
// only looks something up gets the plain timeout.
func TestOperationsGiveEachRequestItsBudget(t *testing.T) {
	t.Parallel()

	// Distinct, so a deadline shows which timeout it came from, and below DefaultTimeout once the
	// grace period is added (see recordRequests).
	const (
		readTimeout   = 40 * time.Second
		createTimeout = 90 * time.Second
		updateTimeout = 100 * time.Second
		deleteTimeout = 70 * time.Second
	)
	configured := configuredTimeouts("read", readTimeout.String(), "create", createTimeout.String(), "update", updateTimeout.String(), "delete", deleteTimeout.String())

	read := func(fresh bool) budget { return budget{readTimeout, fresh} }
	mutation := func(timeout time.Duration, fresh bool) budget { return budget{timeout + mutationGracePeriod, fresh} }
	lookup := func(timeout time.Duration) budget { return budget{timeout, true} }

	ok := func(body string) stubResponse { return stubResponse{status: http.StatusOK, body: body} }
	str := func(s string) tftypes.Value { return tftypes.NewValue(tftypes.String, s) }
	boolean := func(b bool) tftypes.Value { return tftypes.NewValue(tftypes.Bool, b) }
	with := func(base map[string]tftypes.Value, set map[string]tftypes.Value) map[string]tftypes.Value {
		merged := maps.Clone(base)
		maps.Copy(merged, set)
		return merged
	}

	pipeline := `{"pipeline":{"id":"pipeline-id","pipelineUuid":"pipeline-uuid","name":"pipeline","slug":"pipeline",
		"repository":{"url":"git@github.com:org/repo.git"},"steps":{"yaml":"steps: []"},"tags":[],"teams":{"edges":[]}}}`
	pipelinePlan := map[string]tftypes.Value{
		"id":         str("pipeline-id"),
		"name":       str("pipeline"),
		"repository": str("git@github.com:org/repo.git"),
		"steps":      str("steps: []"),
		"archived":   boolean(false),
	}
	teamFound := ok(`{"data":{"node":{"__typename":"Team","id":"team-id"}}}`)
	apiSettings := ok(`{"allowed_ip_addresses":"","restrict_user_api_token_creation":false,"features":{"api_ip_allow_list":true}}`)
	allowlist := func(ctx context.Context, t *testing.T, r fwresource.Resource, cidr string) tftypes.Value {
		listType := resourceSchema(ctx, t, r).Type().TerraformType(ctx).(tftypes.Object).AttributeTypes["allowed_api_ip_addresses"]
		return tftypes.NewValue(listType, []tftypes.Value{str(cidr)})
	}
	queueCreated := ok(`{"data":{"clusterQueueCreate":{"clusterQueue":{"id":"queue-id","uuid":"queue-uuid","key":"queue","cluster":{"uuid":"cluster-uuid"}}}}}`)

	tests := []struct {
		name      string
		responses []stubResponse
		want      []budget
		run       func(context.Context, *testing.T, *Client) diag.Diagnostics
		// Leave the organization ID uncached, so the lookup is one of the requests checked.
		lookupOrganization bool
	}{
		{
			name:      "cluster create",
			responses: []stubResponse{ok(`{"errors":[{"message":"refused"}]}`)},
			want:      []budget{mutation(createTimeout, true)},
			run: func(ctx context.Context, t *testing.T, c *Client) diag.Diagnostics {
				return runCreate(ctx, t, &clusterResource{client: c}, map[string]tftypes.Value{"name": str("cluster")})
			},
		},
		{
			name:      "cluster delete",
			responses: []stubResponse{ok(`{"errors":[{"message":"refused"}]}`)},
			want:      []budget{mutation(deleteTimeout, true)},
			run: func(ctx context.Context, t *testing.T, c *Client) diag.Diagnostics {
				return runDelete(ctx, t, &clusterResource{client: c}, map[string]tftypes.Value{"id": str("cluster-id")})
			},
		},
		{
			name: "paging data source",
			responses: []stubResponse{
				ok(`{"data":{"organization":{"teams":{"pageInfo":{"endCursor":"cursor","hasNextPage":true},"edges":[{"node":{"id":"team-id","name":"team","slug":"team"}}]}}}}`),
				ok(`{"data":{"organization":{"teams":{"pageInfo":{"endCursor":"cursor","hasNextPage":false},"edges":[{"node":{"id":"team-id","name":"team","slug":"team"}}]}}}}`),
			},
			want: []budget{read(true), read(false)},
			run: func(ctx context.Context, t *testing.T, c *Client) diag.Diagnostics {
				ds := &teamsDatasource{client: c}
				req := datasource.ReadRequest{Config: datasourceConfigFor(ctx, t, ds, nil)}
				resp := datasource.ReadResponse{State: tfsdk.State{Schema: req.Config.Schema, Raw: tftypes.NewValue(req.Config.Schema.Type().TerraformType(ctx), nil)}}
				ds.Read(ctx, req, &resp)
				return resp.Diagnostics
			},
		},
		{
			// Every REST helper Create calls, and the archive after them.
			name: "pipeline create",
			responses: []stubResponse{
				ok(`{"data":{"pipelineCreate":` + pipeline + `}}`),
				ok(`{"slug":"chosen"}`), // slug
				ok(`{}`),                // provider settings
				ok(`{"enabled":true}`),  // GitHub webhooks read
				ok(`{}`),                // GitHub webhooks disable
				ok(`{"data":{"pipelineArchive":{"pipeline":{"id":"pipeline-id"}}}}`),
			},
			want: []budget{
				mutation(createTimeout, true),
				mutation(createTimeout, true),
				mutation(createTimeout, true),
				lookup(createTimeout),
				mutation(createTimeout, true),
				mutation(createTimeout, true),
			},
			run: func(ctx context.Context, t *testing.T, c *Client) diag.Diagnostics {
				p := &pipelineResource{client: c}
				return runCreate(ctx, t, p, with(pipelinePlan, map[string]tftypes.Value{
					"slug":                    str("chosen"),
					"provider_settings":       nullAttribute(ctx, t, p, "provider_settings"),
					"github_webhooks_enabled": boolean(false),
					"archived":                boolean(true),
				}))
			},
		},
		{
			// Removing the default team pages through the pipeline's teams and deletes the edge in one
			// step, then archives in another.
			name: "pipeline update",
			responses: []stubResponse{
				ok(`{"data":{"pipelineUpdate":` + pipeline + `}}`),
				ok(`{"data":{"pipeline":{"teams":{"pageInfo":{"hasNextPage":false},"edges":[{"node":{"id":"edge-id","accessLevel":"MANAGE_BUILD_AND_READ","team":{"id":"old-team"}}}]}}}}`),
				ok(`{"data":{"teamPipelineDelete":{"deletedTeamPipelineID":"edge-id"}}}`),
				ok(`{"data":{"pipelineArchive":{"pipeline":{"id":"pipeline-id"}}}}`),
			},
			want: []budget{
				mutation(updateTimeout, true),
				mutation(updateTimeout, true),
				mutation(updateTimeout, false),
				mutation(updateTimeout, true),
			},
			run: func(ctx context.Context, t *testing.T, c *Client) diag.Diagnostics {
				prior := with(pipelinePlan, map[string]tftypes.Value{"slug": str("pipeline"), "default_team_id": str("old-team")})
				return runUpdate(ctx, t, &pipelineResource{client: c}, prior, with(pipelinePlan, map[string]tftypes.Value{"archived": boolean(true)}))
			},
		},
		{
			// The new default team is already attached with less access. Attaching it, raising its access
			// (a walk and a mutation), and removing the old team (a walk and a delete) are each a step.
			name: "pipeline update replacing the default team",
			responses: []stubResponse{
				ok(`{"data":{"pipelineUpdate":` + pipeline + `}}`),
				ok(`{"errors":[{"message":"This pipeline has already been added to this team"}]}`),
				ok(`{"data":{"pipeline":{"teams":{"pageInfo":{"hasNextPage":false},"edges":[
					{"node":{"id":"new-edge","accessLevel":"READ_ONLY","team":{"id":"new-team"}}}]}}}}`),
				ok(`{"data":{"teamPipelineUpdate":{"teamPipeline":{"id":"new-edge"}}}}`),
				ok(`{"data":{"pipeline":{"teams":{"pageInfo":{"hasNextPage":false},"edges":[
					{"node":{"id":"old-edge","accessLevel":"MANAGE_BUILD_AND_READ","team":{"id":"old-team"}}}]}}}}`),
				ok(`{"data":{"teamPipelineDelete":{"deletedTeamPipelineID":"old-edge"}}}`),
			},
			want: []budget{
				mutation(updateTimeout, true),
				mutation(updateTimeout, true),
				mutation(updateTimeout, true),
				mutation(updateTimeout, false),
				mutation(updateTimeout, true),
				mutation(updateTimeout, false),
			},
			run: func(ctx context.Context, t *testing.T, c *Client) diag.Diagnostics {
				prior := with(pipelinePlan, map[string]tftypes.Value{"slug": str("pipeline"), "default_team_id": str("old-team")})
				return runUpdate(ctx, t, &pipelineResource{client: c}, prior, with(pipelinePlan, map[string]tftypes.Value{"default_team_id": str("new-team")}))
			},
		},
		{
			// The default team is not on the first page of the pipeline's teams, so Read checks the team
			// exists, pages on, and checks it again for the next page, all under the one read budget.
			name: "pipeline read",
			responses: []stubResponse{
				ok(`{"data":{"node":{"__typename":"Pipeline","id":"pipeline-id","slug":"pipeline","teams":{"pageInfo":{"endCursor":"cursor","hasNextPage":true},"edges":[]}}}}`),
				teamFound,
				ok(`{"data":{"pipeline":{"teams":{"pageInfo":{"hasNextPage":false},"edges":[{"node":{"id":"edge-id","accessLevel":"MANAGE_BUILD_AND_READ","team":{"id":"team-id"}}}]}}}}`),
				teamFound,
			},
			want: []budget{read(true), read(false), read(false), read(false)},
			run: func(ctx context.Context, t *testing.T, c *Client) diag.Diagnostics {
				return runRead(ctx, t, &pipelineResource{client: c}, with(pipelinePlan, map[string]tftypes.Value{"slug": str("pipeline"), "default_team_id": str("team-id")}))
			},
		},
		{
			// The repository check only looks, so it stops at the timeout; the create gets its own budget.
			name: "pipeline webhook create",
			responses: []stubResponse{
				ok(`{"data":{"node":{"__typename":"Pipeline","id":"pipeline-id","repository":{"url":"git@github.com:org/repo.git"}}}}`),
				ok(`{"data":{"pipelineCreateWebhook":{"pipeline":{"id":"pipeline-id","repository":{"url":"git@github.com:org/repo.git"}},"webhook":{"externalId":"hook","url":"https://example.com"}}}}`),
			},
			want: []budget{lookup(createTimeout), mutation(createTimeout, true)},
			run: func(ctx context.Context, t *testing.T, c *Client) diag.Diagnostics {
				return runCreate(ctx, t, &pipelineWebhook{client: c}, map[string]tftypes.Value{"pipeline_id": str("pipeline-id"), "repository": str("git@github.com:org/repo.git")})
			},
		},
		{
			// The settings read and write are one step; enforcing 2FA is another.
			name: "organization create",
			responses: []stubResponse{
				ok(`{"data":{"organization":{"id":"organization-id","uuid":"organization-uuid","membersRequireTwoFactorAuthentication":false}}}`),
				apiSettings,
				ok(`{"allowed_ip_addresses":"10.0.0.0/8"}`),
				ok(`{"data":{"organizationEnforceTwoFactorAuthenticationForMembersUpdate":{"organization":{"membersRequireTwoFactorAuthentication":true}}}}`),
			},
			want: []budget{mutation(createTimeout, true), mutation(createTimeout, false), mutation(createTimeout, false), mutation(createTimeout, true)},
			run: func(ctx context.Context, t *testing.T, c *Client) diag.Diagnostics {
				o := &organizationResource{client: c}
				return runCreate(ctx, t, o, map[string]tftypes.Value{"enforce_2fa": boolean(true), "allowed_api_ip_addresses": allowlist(ctx, t, o, "10.0.0.0/8")})
			},
		},
		{
			name: "organization update",
			responses: []stubResponse{
				apiSettings,
				ok(`{"allowed_ip_addresses":"10.0.0.0/8"}`),
				ok(`{"data":{"organizationEnforceTwoFactorAuthenticationForMembersUpdate":{"organization":{"membersRequireTwoFactorAuthentication":true}}}}`),
			},
			want: []budget{mutation(updateTimeout, true), mutation(updateTimeout, false), mutation(updateTimeout, true)},
			run: func(ctx context.Context, t *testing.T, c *Client) diag.Diagnostics {
				o := &organizationResource{client: c}
				prior := map[string]tftypes.Value{"id": str("organization-id"), "enforce_2fa": boolean(false)}
				return runUpdate(ctx, t, o, prior, map[string]tftypes.Value{"id": str("organization-id"), "enforce_2fa": boolean(true), "allowed_api_ip_addresses": allowlist(ctx, t, o, "10.0.0.0/8")})
			},
		},
		{
			name: "organization read",
			responses: []stubResponse{
				ok(`{"data":{"organization":{"id":"organization-id","uuid":"organization-uuid","membersRequireTwoFactorAuthentication":false}}}`),
				apiSettings,
			},
			want: []budget{read(true), read(false)},
			run: func(ctx context.Context, t *testing.T, c *Client) diag.Diagnostics {
				return runRead(ctx, t, &organizationResource{client: c}, map[string]tftypes.Value{"id": str("organization-id")})
			},
		},
		{
			name:      "organization delete",
			responses: []stubResponse{ok(`{"allowed_ip_addresses":"10.0.0.0/8"}`), ok(`{"allowed_ip_addresses":""}`)},
			want:      []budget{mutation(deleteTimeout, true), mutation(deleteTimeout, false)},
			run: func(ctx context.Context, t *testing.T, c *Client) diag.Diagnostics {
				return runDelete(ctx, t, &organizationResource{client: c}, map[string]tftypes.Value{"id": str("organization-id")})
			},
		},
		{
			// The create, setting retry_agent_affinity over REST, and pausing dispatch are each a step.
			name:      "cluster queue create",
			responses: []stubResponse{queueCreated, ok(`{}`), ok(`{"data":{}}`)},
			want:      []budget{mutation(createTimeout, true), mutation(createTimeout, true), mutation(createTimeout, true)},
			run: func(ctx context.Context, t *testing.T, c *Client) diag.Diagnostics {
				return runCreate(ctx, t, &clusterQueueResource{client: c}, map[string]tftypes.Value{
					"cluster_id":           str("cluster-id"),
					"key":                  str("queue"),
					"retry_agent_affinity": str(RetryAgentAffinityPreferDifferent),
					"dispatch_paused":      boolean(true),
				})
			},
		},
		{
			// Pausing dispatch, the update, and setting retry_agent_affinity over REST are each a step.
			name:      "cluster queue update",
			responses: []stubResponse{ok(`{"data":{}}`), ok(`{"data":{"clusterQueueUpdate":{"clusterQueue":{"id":"queue-id","dispatchPaused":true}}}}`), ok(`{}`)},
			want:      []budget{mutation(updateTimeout, true), mutation(updateTimeout, true), mutation(updateTimeout, true)},
			run: func(ctx context.Context, t *testing.T, c *Client) diag.Diagnostics {
				prior := map[string]tftypes.Value{
					"id":                   str("queue-id"),
					"uuid":                 str("queue-uuid"),
					"cluster_id":           str("cluster-id"),
					"cluster_uuid":         str("cluster-uuid"),
					"key":                  str("queue"),
					"retry_agent_affinity": str(RetryAgentAffinityPreferWarmest),
					"dispatch_paused":      boolean(false),
				}
				return runUpdate(ctx, t, &clusterQueueResource{client: c}, prior, with(prior, map[string]tftypes.Value{
					"retry_agent_affinity": str(RetryAgentAffinityPreferDifferent),
					"dispatch_paused":      boolean(true),
				}))
			},
		},
		{
			// The organization lookup only looks, so it stops at the timeout; the update and resuming
			// dispatch are each a step.
			name:               "cluster queue update resuming dispatch",
			lookupOrganization: true,
			responses: []stubResponse{
				ok(`{"data":{"organization":{"id":"organization-id"}}}`),
				ok(`{"data":{"clusterQueueUpdate":{"clusterQueue":{"id":"queue-id","dispatchPaused":true}}}}`),
				ok(`{"data":{}}`),
			},
			want: []budget{lookup(updateTimeout), mutation(updateTimeout, true), mutation(updateTimeout, true)},
			run: func(ctx context.Context, t *testing.T, c *Client) diag.Diagnostics {
				prior := map[string]tftypes.Value{
					"id":                   str("queue-id"),
					"uuid":                 str("queue-uuid"),
					"cluster_id":           str("cluster-id"),
					"cluster_uuid":         str("cluster-uuid"),
					"key":                  str("queue"),
					"retry_agent_affinity": str(RetryAgentAffinityPreferWarmest),
					"dispatch_paused":      boolean(true),
				}
				return runUpdate(ctx, t, &clusterQueueResource{client: c}, prior, with(prior, map[string]tftypes.Value{
					"dispatch_paused": boolean(false),
				}))
			},
		},
		{
			name: "cluster queue read",
			responses: []stubResponse{
				ok(`{"data":{"node":{"__typename":"ClusterQueue","id":"queue-id","uuid":"queue-uuid","key":"queue","cluster":{"id":"cluster-id","uuid":"cluster-uuid"}}}}`),
				ok(`{"retry_agent_affinity":"prefer-warmest"}`),
			},
			want: []budget{read(true), read(false)},
			run: func(ctx context.Context, t *testing.T, c *Client) diag.Diagnostics {
				return runRead(ctx, t, &clusterQueueResource{client: c}, map[string]tftypes.Value{
					"id": str("queue-id"), "uuid": str("queue-uuid"), "cluster_uuid": str("cluster-uuid"), "key": str("queue"),
				})
			},
		},
		{
			name:      "cluster queue delete",
			responses: []stubResponse{ok(`{"data":{}}`)},
			want:      []budget{mutation(deleteTimeout, true)},
			run: func(ctx context.Context, t *testing.T, c *Client) diag.Diagnostics {
				return runDelete(ctx, t, &clusterQueueResource{client: c}, map[string]tftypes.Value{"id": str("queue-id"), "key": str("queue")})
			},
		},
		{
			// The PATCH, attaching the new owner, and detaching the old owner are each a step. Reading
			// the suite's teams in between is a lookup, so its pages share one plain timeout.
			name: "test suite update",
			responses: []stubResponse{
				ok(`{"slug":"suite"}`),
				ok(`{"data":{"teamSuiteCreate":{"teamSuite":{"id":"new-edge"}}}}`),
				ok(`{"data":{"suite":{"__typename":"Suite","id":"suite-id","teams":{"pageInfo":{"endCursor":"cursor","hasNextPage":true},"edges":[
					{"node":{"id":"new-edge","accessLevel":"MANAGE_AND_READ","team":{"id":"new-team"}}}]}}}}`),
				ok(`{"data":{"suite":{"__typename":"Suite","id":"suite-id","teams":{"pageInfo":{"hasNextPage":false},"edges":[
					{"node":{"id":"old-edge","accessLevel":"MANAGE_AND_READ","team":{"id":"old-team"}}}]}}}}`),
				ok(`{"data":{"teamSuiteDelete":{"deletedTeamSuiteID":"old-edge"}}}`),
			},
			want: []budget{mutation(updateTimeout, true), mutation(updateTimeout, true), lookup(updateTimeout), {updateTimeout, false}, mutation(updateTimeout, true)},
			run: func(ctx context.Context, t *testing.T, c *Client) diag.Diagnostics {
				prior := map[string]tftypes.Value{"id": str("suite-id"), "slug": str("suite"), "name": str("suite"), "default_branch": str("main"), "team_owner_id": str("old-team")}
				return runUpdate(ctx, t, &testSuiteResource{client: c}, prior, with(prior, map[string]tftypes.Value{"team_owner_id": str("new-team")}))
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server, _ := newRetryStub(t, testCase.responses...)
			defer server.Close()

			client := newGraphQLTimeoutTestClient(t, server.URL, 0, time.Millisecond, configured)
			if !testCase.lookupOrganization {
				// Primed so the organization lookup does not consume a stubbed response of its own.
				organizationID := "organization-id"
				client.organizationId = &organizationID
			}
			log := recordRequests(client)

			started := time.Now()
			diags := testCase.run(t.Context(), t, client)

			log.mu.Lock()
			defer log.mu.Unlock()
			if len(log.requests) != len(testCase.want) {
				t.Logf("diagnostics: %v", diags)
			}
			assertBudgets(t, started, log.requests, testCase.want)
		})
	}
}

// The organization rule's plan modifier resolves pipeline slugs under the read timeout, shared
// across both lookups, since plan has no timeout of its own.
func TestOrganizationRulePlanSharesTheReadTimeoutAcrossLookups(t *testing.T) {
	t.Parallel()

	const readTimeout = 40 * time.Second
	pipelineFound := stubResponse{status: http.StatusOK, body: `{"data":{"pipeline":{"id":"pipeline-id","pipelineUuid":"pipeline-uuid"}}}`}
	server, _ := newRetryStub(t, pipelineFound, pipelineFound)
	defer server.Close()

	client := newGraphQLTimeoutTestClient(t, server.URL, 0, time.Millisecond, configuredTimeouts("read", readTimeout.String()))
	log := recordRequests(client)

	ctx := t.Context()
	r := &organizationRuleResource{client: client}
	schema := resourceSchema(ctx, t, r)
	config := nullObjectWith(ctx, t, schema.Type(), map[string]tftypes.Value{
		"type":  tftypes.NewValue(tftypes.String, "pipeline.trigger_build.pipeline"),
		"value": tftypes.NewValue(tftypes.String, `{"source_pipeline":"source","target_pipeline":"target"}`),
	})
	resp := fwresource.ModifyPlanResponse{Plan: tfsdk.Plan{Schema: schema, Raw: config}}

	started := time.Now()
	r.ModifyPlan(ctx, fwresource.ModifyPlanRequest{
		Config: tfsdk.Config{Schema: schema, Raw: config},
		Plan:   tfsdk.Plan{Schema: schema, Raw: config},
		State:  tfsdk.State{Schema: schema, Raw: tftypes.NewValue(schema.Type().TerraformType(ctx), nil)},
	}, &resp)

	log.mu.Lock()
	defer log.mu.Unlock()
	if len(log.requests) != 2 {
		t.Logf("diagnostics: %v", resp.Diagnostics)
	}
	assertBudgets(t, started, log.requests, []budget{{readTimeout, true}, {readTimeout, false}})
}

// runCreate, runRead, runUpdate and runDelete drive one CRUD method with every attribute null except
// those given. Config is the plan, which is what an operation that reads it expects to find.
func runCreate(ctx context.Context, t *testing.T, r fwresource.Resource, plan map[string]tftypes.Value) diag.Diagnostics {
	t.Helper()

	schema := resourceSchema(ctx, t, r)
	raw := nullObjectWith(ctx, t, schema.Type(), plan)
	resp := fwresource.CreateResponse{State: tfsdk.State{Schema: schema, Raw: tftypes.NewValue(schema.Type().TerraformType(ctx), nil)}}
	r.Create(ctx, fwresource.CreateRequest{Plan: tfsdk.Plan{Schema: schema, Raw: raw}, Config: tfsdk.Config{Schema: schema, Raw: raw}}, &resp)

	return resp.Diagnostics
}

func runRead(ctx context.Context, t *testing.T, r fwresource.Resource, state map[string]tftypes.Value) diag.Diagnostics {
	t.Helper()

	schema := resourceSchema(ctx, t, r)
	raw := nullObjectWith(ctx, t, schema.Type(), state)
	resp := fwresource.ReadResponse{State: tfsdk.State{Schema: schema, Raw: raw}}
	r.Read(ctx, fwresource.ReadRequest{State: tfsdk.State{Schema: schema, Raw: raw}}, &resp)

	return resp.Diagnostics
}

func runUpdate(ctx context.Context, t *testing.T, r fwresource.Resource, prior, plan map[string]tftypes.Value) diag.Diagnostics {
	t.Helper()

	req, resp := updateRequestFor(ctx, t, resourceSchema(ctx, t, r), prior, plan)
	r.Update(ctx, req, &resp)

	return resp.Diagnostics
}

func runDelete(ctx context.Context, t *testing.T, r fwresource.Resource, state map[string]tftypes.Value) diag.Diagnostics {
	t.Helper()

	schema := resourceSchema(ctx, t, r)
	raw := nullObjectWith(ctx, t, schema.Type(), state)
	resp := fwresource.DeleteResponse{State: tfsdk.State{Schema: schema, Raw: raw}}
	r.Delete(ctx, fwresource.DeleteRequest{State: tfsdk.State{Schema: schema, Raw: raw}}, &resp)

	return resp.Diagnostics
}

// nullAttribute is a known value of the named nested attribute whose own attributes are all null.
func nullAttribute(ctx context.Context, t *testing.T, r fwresource.Resource, name string) tftypes.Value {
	t.Helper()

	objectType, ok := resourceSchema(ctx, t, r).Type().TerraformType(ctx).(tftypes.Object).AttributeTypes[name].(tftypes.Object)
	if !ok {
		t.Fatalf("Attribute %q is not a nested object", name)
	}
	attributes := make(map[string]tftypes.Value, len(objectType.AttributeTypes))
	for attribute, attributeType := range objectType.AttributeTypes {
		attributes[attribute] = tftypes.NewValue(attributeType, nil)
	}

	return tftypes.NewValue(objectType, attributes)
}

// retryablehttp returns only the context error when the deadline lands during a backoff wait, or
// "giving up after N attempt(s)" when the retries run out, and the response it retried is gone. REST
// kept it through makeRequest; GraphQL now keeps it the same way, so the failure says what the API
// was answering.
func TestGraphQLRequestReportsTheResponseItRetried(t *testing.T) {
	t.Parallel()

	unavailable := stubResponse{status: http.StatusServiceUnavailable, body: `{"message":"Service Unavailable"}`}

	tests := []struct {
		name       string
		retries    int
		wantPhrase string
		deadline   bool
	}{
		{name: "deadline lands during a backoff wait", retries: stubRetries, wantPhrase: "context deadline exceeded", deadline: true},
		{name: "retries run out", retries: 2, wantPhrase: "after 3 attempts"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server, _ := newRetryStub(t, unavailable)
			defer server.Close()

			client := newGraphQLTimeoutTestClient(t, server.URL, testCase.retries, stubRetryWait, configuredTimeouts("read", stubReadTimeout.String()))

			_, err := getOrganization(context.Background(), client.genqlient, client.organization)
			if err == nil {
				t.Fatal("getOrganization succeeded against a server that only fails")
			}
			if !isAPIStatus(err, http.StatusServiceUnavailable) {
				t.Errorf("getOrganization() = %q, want the 503 it retried reported as the status", err)
			}
			for _, want := range []string{"Service Unavailable", testCase.wantPhrase} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("getOrganization() = %q, want it to mention %q", err, want)
				}
			}
			if got := errors.Is(err, context.DeadlineExceeded); got != testCase.deadline {
				t.Errorf("errors.Is(err, context.DeadlineExceeded) = %v, want %v", got, testCase.deadline)
			}
		})
	}
}

// A response that did arrive is reported by genqlient itself and passes through untouched, so
// callers that inspect GraphQL errors see what they did before.
func TestGraphQLResponseErrorsPassThrough(t *testing.T) {
	t.Parallel()

	server, _ := newRetryStub(t, stubResponse{status: http.StatusOK, body: `{"errors":[{"message":"No Organization found"}]}`})
	defer server.Close()

	client := newGraphQLTimeoutTestClient(t, server.URL, 0, time.Millisecond, configuredTimeouts("read", stubReadTimeout.String()))

	_, err := getOrganization(context.Background(), client.genqlient, client.organization)
	var errList gqlerror.List
	if !errors.As(err, &errList) {
		t.Fatalf("getOrganization() = %v (%T), want the GraphQL error list", err, err)
	}
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		t.Errorf("getOrganization() = %q, want a response error left unwrapped", err)
	}
	if !isResourceNotFoundError(err) {
		t.Errorf("isResourceNotFoundError(%q) = false, want true", err)
	}
}
