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
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
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

// Each operation bounds its GraphQL calls by its own timeout. Without it, the client's fallback would
// bound a create by the read timeout, and a paging walk would get a fresh read timeout for every
// page. A create or delete gets mutationGracePeriod on top, the time retry.RetryContext waits for an
// attempt still in flight at its timeout.
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
			want:      10*time.Minute + mutationGracePeriod,
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
			want:      7*time.Minute + mutationGracePeriod,
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

// Each step of a multi-step create gets its own budget. Sharing one meant a slow pipeline create
// left the archive after it too little time, and the pipeline was recorded unarchived and tainted.
func TestPipelineCreateGivesEachStepItsOwnDeadline(t *testing.T) {
	t.Parallel()

	const createDelay = 100 * time.Millisecond

	server, _ := newRetryStub(t,
		stubResponse{status: http.StatusOK, delay: createDelay, body: `{"data":{"pipelineCreate":{"pipeline":{
			"id": "pipeline-id",
			"pipelineUuid": "pipeline-uuid",
			"name": "pipeline",
			"slug": "pipeline",
			"repository": {"url": "git@github.com:org/repo.git"},
			"steps": {"yaml": "steps: []"},
			"tags": [],
			"teams": {"edges": []}
		}}}}`},
		stubResponse{status: http.StatusOK, body: `{"data":{"pipelineArchive":{"pipeline":{"id":"pipeline-id"}}}}`},
	)
	defer server.Close()

	client := newGraphQLTimeoutTestClient(t, server.URL, 0, time.Millisecond, configuredTimeouts("create", "10m"))
	organizationID := "organization-id"
	client.organizationId = &organizationID
	recorder := &deadlineRecorder{inner: client.genqlient}
	client.genqlient = recorder

	ctx := t.Context()
	p := &pipelineResource{client: client}
	schema := resourceSchema(ctx, t, p)
	raw := nullObjectWith(ctx, t, schema.Type(), map[string]tftypes.Value{
		"name":       tftypes.NewValue(tftypes.String, "pipeline"),
		"repository": tftypes.NewValue(tftypes.String, "git@github.com:org/repo.git"),
		"steps":      tftypes.NewValue(tftypes.String, "steps: []"),
		"archived":   tftypes.NewValue(tftypes.Bool, true),
	})
	resp := fwresource.CreateResponse{State: tfsdk.State{Schema: schema, Raw: tftypes.NewValue(schema.Type().TerraformType(ctx), nil)}}

	p.Create(ctx, fwresource.CreateRequest{Plan: tfsdk.Plan{Schema: schema, Raw: raw}, Config: tfsdk.Config{Schema: schema, Raw: raw}}, &resp)

	// resp.Private is nil here because only the framework can build one, which adds an unrelated
	// diagnostic, so check the step this is about rather than for any error.
	if diagnosticsContain(resp.Diagnostics, "Unable to archive pipeline") {
		t.Fatalf("Create() diagnostics = %v, want the archive to succeed", resp.Diagnostics)
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.deadlines) != 2 {
		t.Fatalf("Made %d GraphQL requests, want the create and the archive; diagnostics: %v", len(recorder.deadlines), resp.Diagnostics)
	}
	// The archive's budget starts once the create has come back, so it ends at least the create's
	// delay after the create's. A shared budget would give both the same deadline.
	if gap := recorder.deadlines[1].Sub(recorder.deadlines[0]); gap < createDelay {
		t.Errorf("Archive deadline is %s after the create's, want at least %s: the archive shared the create's budget", gap, createDelay)
	}
}
