package buildkite

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	genqlient "github.com/Khan/genqlient/graphql"
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
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

// timeoutsValue is the provider's timeouts block with the given attributes set.
func timeoutsValue(set map[string]string) timeouts.Value {
	attrTypes := map[string]attr.Type{}
	values := map[string]attr.Value{}
	for name, value := range set {
		attrTypes[name] = types.StringType
		values[name] = types.StringValue(value)
	}

	return timeouts.Value{Object: types.ObjectValueMust(attrTypes, values)}
}

// newGraphQLTimeoutTestClient is newRetryTestClient with the GraphQL waits shortened as well and the
// given timeouts configured.
func newGraphQLTimeoutTestClient(t *testing.T, serverURL string, maxRetries int, wait time.Duration, configured map[string]string) *Client {
	t.Helper()

	client := newRetryTestClient(t, serverURL, maxRetries, wait)
	client.graphqlRetry.RetryWaitMin = wait
	client.graphqlRetry.RetryWaitMax = wait
	client.timeouts = timeoutsValue(configured)

	return client
}

func assertStoppedAtTheTimeout(t *testing.T, requests int64, failure string) {
	t.Helper()

	if requests < 1 {
		t.Fatal("Expected the call to reach the server at least once")
	}
	if requests > stubBoundedAttempts {
		t.Errorf("Made %d requests against a %s timeout, so the call ran the retry schedule (%d attempts) instead", requests, stubReadTimeout, stubUnboundedAttempts)
	}
	// With retries exhausted the stub's 503 would surface instead, so the deadline error is what shows
	// the call stopped at its timeout rather than for some other reason.
	if !strings.Contains(failure, "context deadline exceeded") {
		t.Errorf("Failed with %q, want a context deadline error", failure)
	}
}

func errorsText(diags diag.Diagnostics) string {
	var messages []string
	for _, d := range diags.Errors() {
		messages = append(messages, d.Summary()+": "+d.Detail())
	}

	return strings.Join(messages, "; ")
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

	client := newGraphQLTimeoutTestClient(t, server.URL, stubRetries, stubRetryWait, map[string]string{"read": stubReadTimeout.String()})

	_, err := getOrganization(context.Background(), client.genqlient, client.organization)
	if err == nil {
		t.Fatal("getOrganization succeeded against a server that only fails")
	}
	assertStoppedAtTheTimeout(t, requests.Load(), err.Error())
}

// The organization lookup runs under a mutex, so an unbounded one would also block every other
// resource waiting on the ID.
func TestGetOrganizationIDWithoutADeadlineStopsAtTheReadTimeout(t *testing.T) {
	t.Parallel()

	server, requests := newRetryStub(t, stubResponse{status: http.StatusServiceUnavailable, body: `{"message":"unavailable"}`})
	defer server.Close()

	client := newGraphQLTimeoutTestClient(t, server.URL, stubRetries, stubRetryWait, map[string]string{"read": stubReadTimeout.String()})

	_, err := client.GetOrganizationID(context.Background())
	if err == nil {
		t.Fatal("GetOrganizationID succeeded against a server that only fails")
	}
	assertStoppedAtTheTimeout(t, requests.Load(), err.Error())
}

// The data source a customer hit ran for 55m51s against a network path that kept resetting.
func TestDatasourceReadsStopAtTheReadTimeout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		datasource func(*Client) datasource.DataSource
		config     map[string]tftypes.Value
	}{
		{
			name:       "cluster",
			datasource: func(client *Client) datasource.DataSource { return &clusterDatasource{client: client} },
			config:     map[string]tftypes.Value{"name": tftypes.NewValue(tftypes.String, "some-cluster")},
		},
		{
			name:       "pipeline template",
			datasource: func(client *Client) datasource.DataSource { return &pipelineTemplateDatasource{client: client} },
			config:     map[string]tftypes.Value{"name": tftypes.NewValue(tftypes.String, "some-template")},
		},
		{
			name:       "pipelines",
			datasource: func(client *Client) datasource.DataSource { return &pipelinesDatasource{client: client} },
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server, requests := newRetryStub(t, stubResponse{status: http.StatusServiceUnavailable, body: `{"message":"unavailable"}`})
			defer server.Close()

			ctx := t.Context()
			ds := testCase.datasource(newGraphQLTimeoutTestClient(t, server.URL, stubRetries, stubRetryWait, map[string]string{"read": stubReadTimeout.String()}))
			req := datasource.ReadRequest{Config: datasourceConfigFor(ctx, t, ds, testCase.config)}
			resp := datasource.ReadResponse{State: tfsdk.State{Schema: req.Config.Schema}}

			ds.Read(ctx, req, &resp)

			if !resp.Diagnostics.HasError() {
				t.Fatal("Read succeeded against a server that only fails")
			}
			assertStoppedAtTheTimeout(t, requests.Load(), errorsText(resp.Diagnostics))
		})
	}
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
	configured := map[string]string{"read": "1m", "create": "10m", "delete": "7m"}

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
