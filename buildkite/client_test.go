package buildkite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/shurcooL/graphql"
)

// Provider settings that are unknown (left out on create) or null must not be sent, or the PATCH
// overwrites Buildkite's defaults; configured values, including false, must be.
func TestUpdatePipelineExtraInfoOmitsUnknownSettings(t *testing.T) {
	t.Parallel()

	var body map[string]map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := &Client{http: server.Client(), restURL: server.URL, organization: "test-org"}
	settings := &providerSettingsModel{
		BuildPullRequestStacks:               types.BoolUnknown(),
		GithubWorkflowAccessTokensEnabled:    types.BoolNull(),
		SkipBuildsForClosedPullRequests:      types.BoolValue(false),
		PreventCustomStatusesBuildkitePrefix: types.BoolValue(true),
	}
	if _, err := updatePipelineExtraInfo(context.Background(), "pipeline", settings, client, time.Minute); err != nil {
		t.Fatal(err)
	}

	sent := body["provider_settings"]
	for _, key := range []string{"build_pull_request_stacks", "github_workflow_access_tokens_enabled"} {
		if _, ok := sent[key]; ok {
			t.Errorf("%s: expected to be left out of the request, got %v", key, sent[key])
		}
	}
	if v, ok := sent["skip_builds_for_closed_pull_requests"]; !ok || v != false {
		t.Errorf("skip_builds_for_closed_pull_requests: expected false to be sent, got %v (present %t)", v, ok)
	}
	if v, ok := sent["prevent_custom_statuses_from_using_buildkite_prefix"]; !ok || v != true {
		t.Errorf("prevent_custom_statuses_from_using_buildkite_prefix: expected true to be sent, got %v (present %t)", v, ok)
	}
}

// A failed organization lookup must not be cached. Now that transient errors can trigger a
// retry, caching an empty ID on failure would make the next retry read it back as a successful
// empty org ID and issue mutations against "".
func TestClientGetOrganizationIDNotCachedOnError(t *testing.T) {
	t.Parallel()

	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			// Transient throttle delivered in a GraphQL 200 body.
			_, _ = w.Write([]byte(`{"errors":[{"message":"Cluster creation is currently busy, please try again."}]}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"organization": map[string]interface{}{"id": "org-abc"},
			},
		})
	}))
	defer server.Close()

	client := &Client{
		graphql:      graphql.NewClient(server.URL, server.Client()),
		organization: "test-org",
	}

	if _, err := client.GetOrganizationID(t.Context()); err == nil {
		t.Fatal("expected error from first lookup, got nil")
	}
	if client.organizationId != nil {
		t.Fatalf("organizationId was cached after a failed lookup: %q", *client.organizationId)
	}

	id, err := client.GetOrganizationID(t.Context())
	if err != nil {
		t.Fatalf("second lookup failed: %v", err)
	}
	if id == nil || *id != "org-abc" {
		got := "nil"
		if id != nil {
			got = *id
		}
		t.Fatalf("GetOrganizationID() = %q, want %q", got, "org-abc")
	}
}

// Terraform runs resource operations concurrently against a single shared Client, so the lazy
// organizationId cache must be safe under concurrent access. Run with -race to catch regressions.
func TestClientGetOrganizationIDConcurrent(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"organization": map[string]interface{}{"id": "org-abc"},
			},
		})
	}))
	defer server.Close()

	client := &Client{
		graphql:      graphql.NewClient(server.URL, server.Client()),
		organization: "test-org",
	}

	const goroutines = 32
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := client.GetOrganizationID(t.Context())
			switch {
			case err != nil:
				errs <- err
			case id == nil || *id != "org-abc":
				errs <- fmt.Errorf("GetOrganizationID() = %v, want org-abc", id)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

// A caller must stop at its own deadline even while another caller's lookup is still running, and
// the waiting caller shares that lookup rather than starting its own.
func TestClientGetOrganizationIDWaiterStopsAtItsOwnDeadline(t *testing.T) {
	t.Parallel()

	var requests atomic.Int64
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"organization":{"id":"org-abc"}}}`))
	}))
	defer server.Close()

	client := &Client{graphql: graphql.NewClient(server.URL, server.Client()), organization: "test-org"}

	// Release the holder's lookup after a while regardless, so a waiter that cannot give up early
	// fails the assertions below instead of hanging the test.
	releaseLater := time.AfterFunc(3*time.Second, func() { close(release) })

	holder := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()
		_, err := client.GetOrganizationID(ctx)
		holder <- err
	}()
	for requests.Load() == 0 {
		time.Sleep(time.Millisecond)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := client.GetOrganizationID(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("GetOrganizationID() = %v, want its own deadline", err)
	}
	// Generous against a loaded machine, but short of the holder's release.
	if waited := time.Since(started); waited > 2*time.Second {
		t.Errorf("Waited %s for a lookup it could not use, want about 100ms", waited.Round(time.Millisecond))
	}

	if releaseLater.Stop() {
		close(release)
	}
	if err := <-holder; err != nil {
		t.Fatalf("The holder's lookup failed: %v", err)
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("Made %d lookups, want the waiter to have shared the holder's", got)
	}
}

// A lookup started by a caller with a short deadline ends at that deadline. A caller with more time
// must not inherit that failure; it starts a lookup of its own under its own deadline.
func TestClientGetOrganizationIDOutlivesAShorterCallersLookup(t *testing.T) {
	t.Parallel()

	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			// The first lookup hangs until its caller's deadline cuts it off. The server only notices
			// the client going away once the request body has been read.
			_, _ = io.Copy(io.Discard, r.Body)
			<-r.Context().Done()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"organization":{"id":"org-abc"}}}`))
	}))
	defer server.Close()

	client := &Client{graphql: graphql.NewClient(server.URL, server.Client()), organization: "test-org"}

	short := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
		defer cancel()
		_, err := client.GetOrganizationID(ctx)
		short <- err
	}()
	for requests.Load() == 0 {
		time.Sleep(time.Millisecond)
	}

	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	id, err := client.GetOrganizationID(ctx)
	if err != nil {
		t.Fatalf("GetOrganizationID() with a minute to spare = %v, want the ID", err)
	}
	if *id != "org-abc" {
		t.Errorf("GetOrganizationID() = %q, want %q", *id, "org-abc")
	}
	if err := <-short; !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("The short caller's GetOrganizationID() = %v, want its deadline", err)
	}
}

// Every attempt of a step shares one deadline, the timeout plus mutationGracePeriod, rather than
// each retry getting a fresh one.
func TestRetryMutationGivesEveryAttemptOneDeadline(t *testing.T) {
	t.Parallel()

	const timeout = 10 * time.Second
	var deadlines []time.Time
	before := time.Now()
	err := retryMutation(t.Context(), timeout, func(ctx context.Context) *retry.RetryError {
		deadline, ok := ctx.Deadline()
		if !ok {
			return retry.NonRetryableError(errors.New("attempt has no deadline"))
		}
		deadlines = append(deadlines, deadline)
		if len(deadlines) == 1 {
			return retry.RetryableError(errors.New("try again"))
		}
		return nil
	})
	after := time.Now()

	if err != nil {
		t.Fatalf("retryMutation() = %v, want nil", err)
	}
	if len(deadlines) != 2 {
		t.Fatalf("retryMutation() made %d attempts, want 2", len(deadlines))
	}
	if !deadlines[0].Equal(deadlines[1]) {
		t.Errorf("Attempt deadlines = %v and %v, want the same", deadlines[0], deadlines[1])
	}
	earliest := before.Add(timeout + mutationGracePeriod)
	latest := after.Add(timeout + mutationGracePeriod)
	if deadlines[0].Before(earliest) || deadlines[0].After(latest) {
		t.Errorf("Attempt deadline = %v, want between %v and %v", deadlines[0], earliest, latest)
	}
}
