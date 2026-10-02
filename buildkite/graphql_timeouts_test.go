package buildkite

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	genqlient "github.com/Khan/genqlient/graphql"
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

// The data source a customer hit, which ran for 55m51s against a network path that kept resetting.
func TestClusterDatasourceReadStopsAtTheReadTimeout(t *testing.T) {
	t.Parallel()

	server, requests := newRetryStub(t, stubResponse{status: http.StatusServiceUnavailable, body: `{"message":"unavailable"}`})
	defer server.Close()

	ctx := t.Context()
	ds := &clusterDatasource{client: newGraphQLTimeoutTestClient(t, server.URL, stubRetries, stubRetryWait, configuredTimeouts("read", stubReadTimeout.String()))}
	req := datasource.ReadRequest{Config: datasourceConfigFor(ctx, t, ds, map[string]tftypes.Value{"name": tftypes.NewValue(tftypes.String, "some-cluster")})}
	resp := datasource.ReadResponse{State: tfsdk.State{Schema: req.Config.Schema}}

	ds.Read(ctx, req, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("Read succeeded against a server that only fails")
	}
	assertStoppedAtTheTimeout(t, requests.Load(), diagnosticsContain(resp.Diagnostics, "context deadline exceeded"), resp.Diagnostics)
}

// deadlineRecorder records the deadline each GraphQL request arrives with, before the client's own
// fallback adds one, so a test can see what the calling method passed down.
type deadlineRecorder struct {
	inner genqlient.Client

	mu        sync.Mutex
	deadlines []time.Time
}

func (d *deadlineRecorder) MakeRequest(ctx context.Context, req *genqlient.Request, resp *genqlient.Response) error {
	deadline, _ := ctx.Deadline()
	d.mu.Lock()
	d.deadlines = append(d.deadlines, deadline)
	d.mu.Unlock()

	return d.inner.MakeRequest(ctx, req, resp)
}

// Every GraphQL call in an operation shares one deadline derived from that operation's own timeout.
// Without it, the client's fallback would bound a create by the read timeout, and a paging walk
// would get a fresh read timeout for every page.
func TestOperationsBoundTheirGraphQLCallsByTheirOwnTimeout(t *testing.T) {
	t.Parallel()

	// Distinct values so the recorded deadline shows which one was used.
	configured := configuredTimeouts("read", "1m", "create", "10m", "delete", "7m")

	teamsPage := func(hasNextPage bool) stubResponse {
		next := "false"
		if hasNextPage {
			next = "true"
		}
		return stubResponse{status: http.StatusOK, body: `{"data":{"organization":{"teams":{
			"pageInfo": {"endCursor": "cursor", "hasNextPage": ` + next + `},
			"edges": [{"node": {"id": "team-id", "uuid": "team-uuid", "name": "team", "slug": "team", "privacy": "VISIBLE", "defaultMemberRole": "MEMBER"}}]
		}}}}`}
	}
	refused := stubResponse{status: http.StatusOK, body: `{"errors":[{"message":"refused"}]}`}

	tests := []struct {
		name      string
		responses []stubResponse
		want      time.Duration
		requests  int
		run       func(context.Context, *testing.T, *Client) diag.Diagnostics
	}{
		{
			name:      "resource create uses the create timeout",
			responses: []stubResponse{refused},
			want:      10 * time.Minute,
			requests:  1,
			run: func(ctx context.Context, t *testing.T, client *Client) diag.Diagnostics {
				r := &clusterResource{client: client}
				schema := resourceSchema(ctx, t, r)
				plan := nullObjectWith(ctx, t, schema.Type(), map[string]tftypes.Value{"name": tftypes.NewValue(tftypes.String, "cluster")})
				resp := fwresource.CreateResponse{State: tfsdk.State{Schema: schema, Raw: tftypes.NewValue(schema.Type().TerraformType(ctx), nil)}}
				r.Create(ctx, fwresource.CreateRequest{Plan: tfsdk.Plan{Schema: schema, Raw: plan}}, &resp)
				return resp.Diagnostics
			},
		},
		{
			name:      "resource delete uses the delete timeout",
			responses: []stubResponse{refused},
			want:      7 * time.Minute,
			requests:  1,
			run: func(ctx context.Context, t *testing.T, client *Client) diag.Diagnostics {
				r := &clusterResource{client: client}
				schema := resourceSchema(ctx, t, r)
				state := nullObjectWith(ctx, t, schema.Type(), map[string]tftypes.Value{"id": tftypes.NewValue(tftypes.String, "cluster-id")})
				resp := fwresource.DeleteResponse{State: tfsdk.State{Schema: schema, Raw: state}}
				r.Delete(ctx, fwresource.DeleteRequest{State: tfsdk.State{Schema: schema, Raw: state}}, &resp)
				return resp.Diagnostics
			},
		},
		{
			name:      "paging data source shares one read timeout across pages",
			responses: []stubResponse{teamsPage(true), teamsPage(false)},
			want:      time.Minute,
			requests:  2,
			run: func(ctx context.Context, t *testing.T, client *Client) diag.Diagnostics {
				ds := &teamsDatasource{client: client}
				req := datasource.ReadRequest{Config: datasourceConfigFor(ctx, t, ds, nil)}
				resp := datasource.ReadResponse{State: tfsdk.State{Schema: req.Config.Schema, Raw: tftypes.NewValue(req.Config.Schema.Type().TerraformType(ctx), nil)}}
				ds.Read(ctx, req, &resp)
				return resp.Diagnostics
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server, _ := newRetryStub(t, testCase.responses...)
			defer server.Close()

			client := newGraphQLTimeoutTestClient(t, server.URL, 0, time.Millisecond, configured)
			organizationID := "organization-id"
			client.organizationId = &organizationID
			recorder := &deadlineRecorder{inner: client.genqlient}
			client.genqlient = recorder

			started := time.Now()
			diags := testCase.run(t.Context(), t, client)
			finished := time.Now()

			recorder.mu.Lock()
			defer recorder.mu.Unlock()
			if len(recorder.deadlines) != testCase.requests {
				t.Fatalf("Made %d GraphQL requests, want %d; diagnostics: %v", len(recorder.deadlines), testCase.requests, diags)
			}
			for i, deadline := range recorder.deadlines {
				if deadline.IsZero() {
					t.Fatalf("Request %d had no deadline, so only the client's read-timeout fallback bounded it", i+1)
				}
				// The deadline was set once the method started, so it lies within want of the window it ran in.
				if deadline.Before(started.Add(testCase.want)) || deadline.After(finished.Add(testCase.want)) {
					t.Errorf("Request %d had %s to run, want %s", i+1, deadline.Sub(started).Round(time.Second), testCase.want)
				}
				if !deadline.Equal(recorder.deadlines[0]) {
					t.Errorf("Request %d had deadline %s, want the operation's single deadline %s", i+1, deadline, recorder.deadlines[0])
				}
			}
		})
	}
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
