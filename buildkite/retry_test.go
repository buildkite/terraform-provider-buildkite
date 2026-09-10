package buildkite

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vektah/gqlparser/v2/gqlerror"
)

// This tests retry logic in general. Both REST and GraphQL clients are wrapped with `retryablehttp.Client`.
func TestClientConfig_MaxRetries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		maxRetries  int
		expectedMax int
	}{
		{
			name:        "default value",
			maxRetries:  DefaultRetryMaxAttempts,
			expectedMax: DefaultRetryMaxAttempts,
		},
		{
			name:        "custom value",
			maxRetries:  5,
			expectedMax: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			config := &clientConfig{
				apiToken:   "test",
				graphqlURL: "https://example.com",
				restURL:    "https://example.com",
				org:        "test",
				userAgent:  "test",
				maxRetries: tt.maxRetries,
			}

			client := NewClient(config)
			if client == nil {
				t.Fatal("NewClient() returned nil")
			}

			if got := client.restRetry.RetryMax; got != tt.expectedMax {
				t.Errorf("rest RetryMax = %d, want %d", got, tt.expectedMax)
			}
			if got := client.graphqlRetry.RetryMax; got != tt.expectedMax {
				t.Errorf("graphql RetryMax = %d, want %d", got, tt.expectedMax)
			}
		})
	}
}

func TestClientCreation(t *testing.T) {
	t.Parallel()

	config := &clientConfig{
		apiToken:   "test",
		graphqlURL: "https://example.com",
		restURL:    "https://example.com",
		org:        "test",
		userAgent:  "test",
		maxRetries: 3,
	}

	client := NewClient(config)
	if client == nil {
		t.Error("NewClient() returned nil")
	}
}

// retryContextError must retry the transient "currently busy / please try again" backend
// throttle (which arrives in a GraphQL 200 body) and leave every other error non-retryable.
func TestRetryContextError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		err           error
		wantNil       bool
		wantRetryable bool
	}{
		{name: "nil error", err: nil, wantNil: true},
		{
			name:          "cluster creation busy is retryable",
			err:           errors.New("input:3:2: clusterCreate Cluster creation is currently busy, please try again."),
			wantRetryable: true,
		},
		{
			name:          "generic please try again is not retryable",
			err:           errors.New("something went wrong, please try again"),
			wantRetryable: false,
		},
		{
			name:          "transient gqlerror is retryable",
			err:           gqlerror.List{{Message: "Cluster creation is currently busy, please try again."}},
			wantRetryable: true,
		},
		{
			name:          "gqlerror list retries when any element is transient",
			err:           gqlerror.List{{Message: "No cluster found"}, {Message: "Cluster creation is currently busy, please try again."}},
			wantRetryable: true,
		},
		{
			name:          "generic error is not retryable",
			err:           errors.New("invalid input: name is required"),
			wantRetryable: false,
		},
		{
			// The http client has already retried the 5xx; matching the phrase out of its body would
			// re-run the whole operation on top of those attempts.
			name: "rest failure is not retryable even when the body reads as transient",
			err: &apiError{
				Method:     http.MethodPost,
				URL:        "https://example.com/v2/organizations/test-org/clusters",
				StatusCode: http.StatusServiceUnavailable,
				Body:       `{"message":"Cluster creation is currently busy, please try again."}`,
			},
			wantRetryable: false,
		},
		{
			// The transport-failure shape carries no status but must be classified the same way, and
			// its message can quote what an earlier attempt's body said.
			name: "rest transport failure is not retryable even when an earlier body reads as transient",
			err: fmt.Errorf("reading cluster: %w", &apiError{
				Method:        http.MethodPost,
				URL:           "https://example.com/v2/organizations/test-org/clusters",
				earlierStatus: http.StatusServiceUnavailable,
				earlierBody:   `{"message":"Cluster creation is currently busy, please try again."}`,
				Err:           errors.New("EOF"),
			}),
			wantRetryable: false,
		},
		{
			name:          "not-found gqlerror is not retryable",
			err:           gqlerror.List{{Message: "No cluster found"}},
			wantRetryable: false,
		},
		{
			name:          "empty gqlerror list is not retryable",
			err:           gqlerror.List{},
			wantRetryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := retryContextError(tt.err)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("retryContextError(nil) = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("retryContextError(%v) = nil, want non-nil", tt.err)
			}
			if got.Retryable != tt.wantRetryable {
				t.Errorf("Retryable = %v, want %v", got.Retryable, tt.wantRetryable)
			}
		})
	}
}

type stubResponse struct {
	status int
	body   string
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// newRetryStub serves responses in order and repeats the last one once they run out, counting
// requests so tests can assert how many attempts the client made.
func newRetryStub(t *testing.T, responses ...stubResponse) (*httptest.Server, *atomic.Int64) {
	t.Helper()

	server, requests, _ := newRecordingRetryStub(t, responses...)

	return server, requests
}

// newRecordingRetryStub is newRetryStub plus the request bodies it received, for tests that need to
// assert what the provider sent rather than only how many times it sent something.
func newRecordingRetryStub(t *testing.T, responses ...stubResponse) (*httptest.Server, *atomic.Int64, func() []string) {
	t.Helper()

	var (
		requests atomic.Int64
		mu       sync.Mutex
		bodies   []string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
		}
		mu.Lock()
		bodies = append(bodies, r.URL.Path+" "+string(body))
		mu.Unlock()

		response := responses[len(responses)-1]
		if n := int(requests.Add(1)); n <= len(responses) {
			response = responses[n-1]
		}

		w.WriteHeader(response.status)
		if _, err := w.Write([]byte(response.body)); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))

	return server, &requests, func() []string {
		mu.Lock()
		defer mu.Unlock()

		return slices.Clone(bodies)
	}
}

// newRetryTestClient goes through NewClient so the tests exercise the real retry configuration,
// then replaces the waits: DefaultRetryWaitMinSeconds is 15, so a test using the configured bounds
// would spend at least 15 seconds per retry. The wait is fixed rather than doubling so that tests
// which need the deadline to land in a known gap can predict it.
func newRetryTestClient(t *testing.T, serverURL string, maxRetries int, wait time.Duration) *Client {
	t.Helper()

	client := NewClient(&clientConfig{
		apiToken:   "test",
		graphqlURL: serverURL,
		restURL:    serverURL,
		org:        "test-org",
		userAgent:  "test",
		maxRetries: maxRetries,
	})
	client.restRetry.RetryWaitMin = wait
	client.restRetry.RetryWaitMax = wait

	return client
}

func TestMakeRequestUsesCallerDeadline(t *testing.T) {
	t.Parallel()

	var gotDeadline time.Time
	client := &Client{
		restURL: "https://example.com",
		http: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			gotDeadline, _ = req.Context().Deadline()
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Body:       http.NoBody,
				Request:    req,
			}, nil
		})},
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	t.Cleanup(cancel)
	wantDeadline, _ := ctx.Deadline()

	if err := client.makeRequest(ctx, http.MethodPost, "/notification-services", nil, nil); err != nil {
		t.Fatalf("makeRequest() error = %v", err)
	}
	if !gotDeadline.Equal(wantDeadline) {
		t.Errorf("request deadline = %v, want caller deadline %v", gotDeadline, wantDeadline)
	}
}

// A sustained 5xx has to reach the user as a status code and the API's own message. retryablehttp
// discards the final response by default, which reduced this to "giving up after N attempt(s)" and
// hid explanations the API had gone to the trouble of returning.
func TestMakeRequestSurfacesFinalErrorAfterRetries(t *testing.T) {
	t.Parallel()

	server, requests := newRetryStub(t, stubResponse{
		status: http.StatusServiceUnavailable,
		body:   `{"message":"Could not load network ranges: tenant is not configured"}`,
	})
	defer server.Close()

	client := newRetryTestClient(t, server.URL, 2, time.Millisecond)

	err := client.makeRequest(context.Background(), http.MethodGet, "/v2/organizations/test-org/clusters", nil, nil)
	if err == nil {
		t.Fatal("Expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "status: 503") {
		t.Errorf("Expected the error to name the status code, got %s", err)
	}
	if !strings.Contains(err.Error(), "tenant is not configured") {
		t.Errorf("Expected the error to carry the API message, got %s", err)
	}
	// Distinguishes a retried failure from a single-shot one.
	if !strings.Contains(err.Error(), "after 3 attempts") {
		t.Errorf("Expected the error to report the attempt count, got %s", err)
	}
	// The library's own exhaustion message means the ErrorHandler is gone and the body went with it.
	if strings.Contains(err.Error(), "giving up after") {
		t.Errorf("Expected the client's message rather than retryablehttp's, got %s", err)
	}
	if got := requests.Load(); got != 3 {
		t.Errorf("Expected 3 attempts (1 initial + 2 retries), got %d", got)
	}
}

// The common production failure: with the configured 15s minimum wait the deadline lands during a
// backoff wait long before the retries run out, and retryablehttp returns the context error and
// drops the response instead of calling ErrorHandler. The status and message still have to survive,
// otherwise a three minute wait ends in a bare "context deadline exceeded".
func TestMakeRequestSurfacesFinalErrorWhenDeadlineExpires(t *testing.T) {
	t.Parallel()

	server, requests := newRetryStub(t, stubResponse{
		status: http.StatusServiceUnavailable,
		body:   `{"message":"Could not load network ranges: tenant is not configured"}`,
	})
	defer server.Close()

	// Waits long enough, and a retry count high enough, that the deadline is what ends the request:
	// attempts land at roughly 0ms, 250ms and 500ms, and the deadline falls in the third wait.
	client := newRetryTestClient(t, server.URL, 10, 250*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()

	err := client.makeRequest(ctx, http.MethodGet, "/v2/organizations/test-org/clusters", nil, nil)
	if err == nil {
		t.Fatal("Expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "status: 503") {
		t.Errorf("Expected the error to name the status code, got %s", err)
	}
	if !strings.Contains(err.Error(), "tenant is not configured") {
		t.Errorf("Expected the error to carry the API message, got %s", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Expected the deadline to stay in the error chain, got %s", err)
	}
	// The point of the case is that the deadline, not the retry budget of 10, ended the request, so
	// the count has to show at least one retry and far fewer attempts than were allowed.
	if got := requests.Load(); got < 2 || got > 4 {
		t.Errorf("Expected 2 to 4 attempts before the deadline, got %d", got)
	}
}

// 429 is retried on the same path as 5xx, so it loses its body the same way.
func TestMakeRequestSurfacesRateLimitErrorAfterRetries(t *testing.T) {
	t.Parallel()

	server, requests := newRetryStub(t, stubResponse{
		status: http.StatusTooManyRequests,
		body:   `{"message":"Number of requests exceeded"}`,
	})
	defer server.Close()

	client := newRetryTestClient(t, server.URL, 1, time.Millisecond)

	err := client.makeRequest(context.Background(), http.MethodGet, "/v2/organizations/test-org/clusters", nil, nil)
	if err == nil {
		t.Fatal("Expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "status: 429") {
		t.Errorf("Expected the error to name the status code, got %s", err)
	}
	if !strings.Contains(err.Error(), "Number of requests exceeded") {
		t.Errorf("Expected the error to carry the API message, got %s", err)
	}
	if got := requests.Load(); got != 2 {
		t.Errorf("Expected 2 attempts (1 initial + 1 retry), got %d", got)
	}
}

// Returning the final response to the caller must not break the ordinary retry-then-succeed path.
func TestMakeRequestRetriesThenSucceeds(t *testing.T) {
	t.Parallel()

	server, requests := newRetryStub(t,
		stubResponse{status: http.StatusServiceUnavailable, body: `{"message":"try later"}`},
		stubResponse{status: http.StatusOK, body: `{"name":"a-cluster"}`},
	)
	defer server.Close()

	client := newRetryTestClient(t, server.URL, 3, time.Millisecond)

	var response struct {
		Name string `json:"name"`
	}
	if err := client.makeRequest(context.Background(), http.MethodGet, "/v2/organizations/test-org/clusters", nil, &response); err != nil {
		t.Fatalf("makeRequest failed: %v", err)
	}

	if response.Name != "a-cluster" {
		t.Errorf("Expected the retried request to decode, got name %q", response.Name)
	}
	if got := requests.Load(); got != 2 {
		t.Errorf("Expected 2 attempts, got %d", got)
	}
}

// A status the retry policy does not cover is answered once and reported with its body, which is
// the behaviour the retryable statuses are being brought into line with.
func TestMakeRequestDoesNotRetryClientErrors(t *testing.T) {
	t.Parallel()

	server, requests := newRetryStub(t, stubResponse{
		status: http.StatusForbidden,
		body:   `{"message":"You're not allowed to do that"}`,
	})
	defer server.Close()

	client := newRetryTestClient(t, server.URL, 3, time.Millisecond)

	err := client.makeRequest(context.Background(), http.MethodGet, "/v2/organizations/test-org/clusters", nil, nil)
	if err == nil {
		t.Fatal("Expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "status: 403") {
		t.Errorf("Expected the error to name the status code, got %s", err)
	}
	if !strings.Contains(err.Error(), "You're not allowed to do that") {
		t.Errorf("Expected the error to carry the API message, got %s", err)
	}
	// A single-attempt failure reads the same as it always has. Resources match on this wording to
	// recognise permission and not-found errors, so the unretried shape must not drift.
	if strings.Contains(err.Error(), "attempts") {
		t.Errorf("Expected no attempt count on an unretried failure, got %s", err)
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("Expected 1 attempt with no retries, got %d", got)
	}
}

// The ErrorHandler is REST-only on purpose: see restErrorHandler in client.go for why handing a
// non-200 back to genqlient is unsafe.
func TestGraphQLClientHasNoErrorHandler(t *testing.T) {
	t.Parallel()

	client := NewClient(&clientConfig{
		apiToken:   "test",
		graphqlURL: "https://example.com",
		restURL:    "https://example.com",
		org:        "test",
		userAgent:  "test",
		maxRetries: 3,
	})

	if client.graphqlRetry.ErrorHandler != nil {
		t.Error("Expected no ErrorHandler on the GraphQL client, so retry exhaustion keeps returning an error rather than a 5xx response")
	}
	if client.restRetry.ErrorHandler == nil {
		t.Error("Expected an ErrorHandler on the REST client")
	}
}

// A status captured from an earlier attempt must not be reported as the cause of a later transport
// failure, which would send an operator looking for a Buildkite 503 when the connection was dying.
// It is still worth reporting as context, so the two have to be distinguishable in the message.
func TestMakeRequestReportsEarlierStatusAsContextAfterTransportFailure(t *testing.T) {
	t.Parallel()

	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			if _, err := w.Write([]byte(`{"message":"tenant is not configured"}`)); err != nil {
				t.Errorf("failed to write response: %v", err)
			}
			return
		}

		// Every later attempt dies mid-connection, which is what the client has to report.
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("failed to hijack connection: %v", err)
			return
		}
		conn.Close()
	}))
	defer server.Close()

	client := newRetryTestClient(t, server.URL, 3, time.Millisecond)

	err := client.makeRequest(context.Background(), http.MethodGet, "/v2/organizations/test-org/clusters", nil, nil)
	if err == nil {
		t.Fatal("Expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to send request") {
		t.Errorf("Expected the transport failure to be reported as the cause, got %s", err)
	}
	if isAPIStatus(err, http.StatusServiceUnavailable) {
		t.Errorf("Expected the earlier status not to be claimed as the request's status, got %s", err)
	}
	if !strings.Contains(err.Error(), "an earlier attempt returned status 503") {
		t.Errorf("Expected the earlier status to survive as context, got %s", err)
	}
}

// A transport failure that survives every retry keeps the attempt count, which is the only clue
// that the provider spent the backoff schedule before giving up.
func TestMakeRequestReportsAttemptsWhenTransportKeepsFailing(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.NotFoundHandler())
	serverURL := server.URL
	server.Close()

	client := newRetryTestClient(t, serverURL, 2, time.Millisecond)

	err := client.makeRequest(context.Background(), http.MethodGet, "/v2/organizations/test-org/clusters", nil, nil)
	if err == nil {
		t.Fatal("Expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "after 3 attempts") {
		t.Errorf("Expected the error to report the attempts, got %s", err)
	}
}

// The registry resources and data source call client.http.Do without going through makeRequest, so
// they keep no capture and cannot rebuild the attempt count themselves. It has to survive on the
// error, or a retried connection failure reads to them as a single-shot one.
func TestDirectRESTCallerKeepsTheAttemptCount(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.NotFoundHandler())
	serverURL := server.URL
	server.Close()

	client := newRetryTestClient(t, serverURL, 2, time.Millisecond)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, serverURL+"/v2/registries", nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}

	if _, err = client.http.Do(req); err == nil {
		t.Fatal("Expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "after 3 attempts") {
		t.Errorf("Expected the error to report the attempts, got %s", err)
	}
}

// The same call must not claim a retry it never made. A non-retryable transport error, a bad
// certificate for instance, gives up on the first attempt.
func TestDirectRESTCallerOmitsTheCountOnASingleAttempt(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.NotFoundHandler())
	defer server.Close()

	client := newRetryTestClient(t, server.URL, 3, time.Millisecond)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/v2/registries", nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}

	if _, err = client.http.Do(req); err == nil {
		t.Fatal("Expected an error, got nil")
	}
	if strings.Contains(err.Error(), "attempts") {
		t.Errorf("Expected no attempt count on an unretried failure, got %s", err)
	}
	if !strings.Contains(err.Error(), "certificate") {
		t.Errorf("Expected the cause to survive, got %s", err)
	}
}

// The carried count is for callers that build no message of their own. makeRequest builds one and
// renders the count beside the request, so it has to strip the carried copy.
func TestMakeRequestDoesNotRepeatTheAttemptCount(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.NotFoundHandler())
	serverURL := server.URL
	server.Close()

	client := newRetryTestClient(t, serverURL, 2, time.Millisecond)

	err := client.makeRequest(context.Background(), http.MethodGet, "/v2/organizations/test-org/clusters", nil, nil)
	if err == nil {
		t.Fatal("Expected an error, got nil")
	}
	if got := strings.Count(err.Error(), "attempts"); got != 1 {
		t.Errorf("Expected the attempt count once, got %d in %s", got, err)
	}
}

// The body is read under a limit on both paths that produce an error message, so an unexpected
// payload cannot be pasted wholesale into a Terraform diagnostic.
func TestMakeRequestTruncatesBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status int
	}{
		{name: "captured from a retried response", status: http.StatusServiceUnavailable},
		{name: "read from a returned response", status: http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server, _ := newRetryStub(t, stubResponse{
				status: tt.status,
				body:   strings.Repeat("a", maxCapturedBodyBytes) + "TAIL",
			})
			defer server.Close()

			client := newRetryTestClient(t, server.URL, 1, time.Millisecond)

			err := client.makeRequest(context.Background(), http.MethodGet, "/v2/organizations/test-org/clusters", nil, nil)
			if err == nil {
				t.Fatal("Expected an error, got nil")
			}
			if strings.Contains(err.Error(), "TAIL") {
				t.Errorf("Expected the body to be truncated, got %d bytes of error", len(err.Error()))
			}
			if !strings.Contains(err.Error(), fmt.Sprintf("status: %d", tt.status)) {
				t.Errorf("Expected the error to name the status code, got %s", err)
			}
		})
	}
}

// An empty body must not leave a dangling separator where the message would go.
func TestMakeRequestOmitsEmptyBody(t *testing.T) {
	t.Parallel()

	server, _ := newRetryStub(t, stubResponse{status: http.StatusServiceUnavailable, body: ""})
	defer server.Close()

	client := newRetryTestClient(t, server.URL, 1, time.Millisecond)

	err := client.makeRequest(context.Background(), http.MethodGet, "/v2/organizations/test-org/clusters", nil, nil)
	if err == nil {
		t.Fatal("Expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "status: 503") {
		t.Errorf("Expected the error to name the status code, got %s", err)
	}
	if strings.Contains(err.Error(), "503):") {
		t.Errorf("Expected no separator after the status when there is no body, got %s", err)
	}
}

// Resources treat a 404 as "gone" and drop the resource from state, so a 404 mentioned anywhere in
// an error body must not read as one. Carrying the status as a field rather than leaving callers to
// find it in the message is what keeps an API-supplied body from deciding a resource's fate.
func TestMakeRequestDoesNotDisguiseABodyAsAStatus(t *testing.T) {
	t.Parallel()

	server, _ := newRetryStub(t, stubResponse{
		status: http.StatusServiceUnavailable,
		body:   `{"message":"upstream returned 404 while loading this page"}`,
	})
	defer server.Close()

	client := newRetryTestClient(t, server.URL, 1, time.Millisecond)

	err := client.makeRequest(context.Background(), http.MethodGet, "/v2/organizations/test-org/clusters", nil, nil)
	if err == nil {
		t.Fatal("Expected an error, got nil")
	}
	if isAPIStatus(err, http.StatusNotFound) {
		t.Errorf("Expected a 404 in the body not to read as the response status, got %s", err)
	}
	if !isAPIStatus(err, http.StatusServiceUnavailable) {
		t.Errorf("Expected the error to carry the real status code, got %s", err)
	}
	if !strings.Contains(err.Error(), "upstream returned 404") {
		t.Errorf("Expected the body to still reach the reader, got %s", err)
	}
}

// The message is user-facing, so its shape is worth pinning: it has to name the request, the status
// and the API's own words, and it has to stay quiet about parts that do not apply.
func TestAPIErrorMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  *apiError
		want string
	}{
		{
			name: "status response",
			err: &apiError{
				Method: http.MethodGet, URL: "https://api.buildkite.com/v2/clusters",
				StatusCode: http.StatusForbidden, Attempts: 1,
				Body: `{"message":"forbidden"}`,
			},
			want: `the Buildkite API request failed: GET https://api.buildkite.com/v2/clusters (status: 403): {"message":"forbidden"}`,
		},
		{
			name: "status response with no body and a single attempt says neither",
			err: &apiError{
				Method: http.MethodGet, URL: "https://api.buildkite.com/v2/clusters",
				StatusCode: http.StatusForbidden, Attempts: 1,
			},
			want: "the Buildkite API request failed: GET https://api.buildkite.com/v2/clusters (status: 403)",
		},
		{
			name: "retried status response ending in a deadline",
			err: &apiError{
				Method: http.MethodGet, URL: "https://api.buildkite.com/v2/clusters",
				StatusCode: http.StatusServiceUnavailable, Attempts: 3,
				Body: `{"message":"try later"}`, Err: context.DeadlineExceeded,
			},
			want: `the Buildkite API request failed: GET https://api.buildkite.com/v2/clusters (status: 503): {"message":"try later"} (after 3 attempts): context deadline exceeded`,
		},
		{
			// A non-retryable transport error, a bad certificate for instance, gives up on the first
			// attempt. Saying so would read as "after 1 attempts" and imply a retry that never happened.
			name: "transport failure on a single attempt names the request and no count",
			err: &apiError{
				Method: http.MethodGet, URL: "https://api.buildkite.com/v2/clusters", Attempts: 1,
				Err: errors.New("tls: failed to verify certificate"),
			},
			want: "failed to send request: GET https://api.buildkite.com/v2/clusters: tls: failed to verify certificate",
		},
		{
			name: "retried transport failure",
			err: &apiError{
				Method: http.MethodGet, URL: "https://api.buildkite.com/v2/clusters", Attempts: 3,
				Err: errors.New("connection refused"),
			},
			want: "failed to send request: GET https://api.buildkite.com/v2/clusters: connection refused (after 3 attempts)",
		},
		{
			name: "transport failure after an earlier status",
			err: &apiError{
				Method: http.MethodGet, URL: "https://api.buildkite.com/v2/clusters", Attempts: 4,
				earlierStatus: http.StatusServiceUnavailable, earlierBody: `{"message":"try later"}`,
				Err: errors.New("EOF"),
			},
			want: `failed to send request: GET https://api.buildkite.com/v2/clusters: EOF (after 4 attempts) (an earlier attempt returned status 503: {"message":"try later"})`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() =\n  %s\nwant\n  %s", got, tt.want)
			}
		})
	}
}

// isAPIStatus has to see through the wrapping the resources add on the way out, and must not answer
// for an error that never carried a status.
func TestIsAPIStatus(t *testing.T) {
	t.Parallel()

	forbidden := &apiError{Method: http.MethodGet, URL: "https://example.com", StatusCode: http.StatusForbidden}

	tests := []struct {
		name   string
		err    error
		status int
		want   bool
	}{
		{name: "nil", err: nil, status: http.StatusNotFound, want: false},
		{name: "matching status", err: forbidden, status: http.StatusForbidden, want: true},
		{name: "different status", err: forbidden, status: http.StatusNotFound, want: false},
		{
			name:   "wrapped matching status",
			err:    fmt.Errorf("listing cluster maintainers: %w", forbidden),
			status: http.StatusForbidden,
			want:   true,
		},
		{
			name:   "transport failure carries no status",
			err:    &apiError{Method: http.MethodGet, URL: "https://example.com", Err: errors.New("EOF")},
			status: http.StatusNotFound,
			want:   false,
		},
		{
			name:   "an earlier attempt's status is not the request's status",
			err:    &apiError{Method: http.MethodGet, URL: "https://example.com", earlierStatus: http.StatusNotFound, Err: errors.New("EOF")},
			status: http.StatusNotFound,
			want:   false,
		},
		{name: "unrelated error", err: errors.New("status: 404"), status: http.StatusNotFound, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := isAPIStatus(tt.err, tt.status); got != tt.want {
				t.Errorf("isAPIStatus(%v, %d) = %v, want %v", tt.err, tt.status, got, tt.want)
			}
		})
	}
}

// When retries end on a status the policy does not cover, that status is the one to report: the
// earlier 503 is history, and a resource matching on "status: 403" has to see it.
func TestMakeRequestReportsTheStatusThatEndedTheRequest(t *testing.T) {
	t.Parallel()

	server, requests := newRetryStub(t,
		stubResponse{status: http.StatusServiceUnavailable, body: `{"message":"try later"}`},
		stubResponse{status: http.StatusForbidden, body: `{"message":"You're not allowed to do that"}`},
	)
	defer server.Close()

	client := newRetryTestClient(t, server.URL, 3, time.Millisecond)

	err := client.makeRequest(context.Background(), http.MethodGet, "/v2/organizations/test-org/clusters", nil, nil)
	if err == nil {
		t.Fatal("Expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "status: 403") {
		t.Errorf("Expected the final status, got %s", err)
	}
	if strings.Contains(err.Error(), "try later") {
		t.Errorf("Expected the earlier 503 body to be replaced, got %s", err)
	}
	if got := requests.Load(); got != 2 {
		t.Errorf("Expected 2 attempts, got %d", got)
	}
}

// A capture must not outlive the request that made it. One client serves every resource operation,
// so anything scoped to the client rather than the request context would follow the next request
// and describe it with the previous one's failure.
func TestMakeRequestCaptureDoesNotOutliveItsRequest(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/flaky" {
			w.WriteHeader(http.StatusServiceUnavailable)
			if _, err := w.Write([]byte(`{"message":"stale detail"}`)); err != nil {
				t.Errorf("failed to write response: %v", err)
			}
			return
		}

		// No body, so anything the message says about one had to come from the earlier request.
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := newRetryTestClient(t, server.URL, 2, time.Millisecond)

	if err := client.makeRequest(context.Background(), http.MethodGet, "/flaky", nil, nil); err == nil {
		t.Fatal("Expected the first request to fail, got nil")
	}

	err := client.makeRequest(context.Background(), http.MethodGet, "/forbidden", nil, nil)
	if err == nil {
		t.Fatal("Expected the second request to fail, got nil")
	}
	if !strings.Contains(err.Error(), "status: 403") {
		t.Errorf("Expected the second request to report its own status, got %s", err)
	}
	if strings.Contains(err.Error(), "stale detail") {
		t.Errorf("Expected no detail from the earlier request, got %s", err)
	}
	if strings.Contains(err.Error(), "attempts") {
		t.Errorf("Expected no attempt count from the earlier request, got %s", err)
	}
}

// Concurrent requests must not read each other's captured response. Terraform runs resource
// operations in parallel, so the shared CheckRetry closure sees interleaved requests. One client
// against one server, because the capture lives per request and nothing else may scope it.
func TestMakeRequestCapturesAreIndependent(t *testing.T) {
	t.Parallel()

	statuses := []int{http.StatusServiceUnavailable, http.StatusBadGateway, http.StatusTooManyRequests}

	// Holding every response until all of them are ready forces the captures to be written at the
	// same time. Without it the requests finish in turn and a shared capture would go unnoticed.
	var arrived sync.WaitGroup
	arrived.Add(len(statuses))

	// The path names the status so one handler can answer every request differently.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/"))
		if err != nil {
			t.Errorf("unexpected request path %q", r.URL.Path)
			return
		}

		arrived.Done()
		arrived.Wait()

		w.WriteHeader(status)
		if _, err := fmt.Fprintf(w, `{"message":"body for %d"}`, status); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	// No retries, so the barrier above sees exactly one request per status.
	client := newRetryTestClient(t, server.URL, 0, time.Millisecond)

	errs := make(map[int]error, len(statuses))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, status := range statuses {
		wg.Add(1)
		go func() {
			defer wg.Done()

			err := client.makeRequest(context.Background(), http.MethodGet, fmt.Sprintf("/%d", status), nil, nil)

			mu.Lock()
			defer mu.Unlock()
			errs[status] = err
		}()
	}
	wg.Wait()

	for _, status := range statuses {
		err := errs[status]
		if err == nil {
			t.Fatalf("Expected an error for %d, got nil", status)
		}
		if !strings.Contains(err.Error(), fmt.Sprintf("status: %d", status)) {
			t.Errorf("Expected the %d request to report its own status, got %s", status, err)
		}
		if !strings.Contains(err.Error(), fmt.Sprintf("body for %d", status)) {
			t.Errorf("Expected the %d request to report its own body, got %s", status, err)
		}
	}
}

func TestUnwrapURLError(t *testing.T) {
	t.Parallel()

	cause := errors.New("connection refused")

	tests := []struct {
		name string
		err  error
		want error
	}{
		{name: "nil", err: nil, want: nil},
		{name: "plain error is returned as is", err: cause, want: cause},
		{
			name: "url error is unwrapped to its cause",
			err:  &url.Error{Op: "Get", URL: "https://example.com", Err: cause},
			want: cause,
		},
		{
			name: "wrapped url error is unwrapped to its cause",
			err:  fmt.Errorf("sending: %w", &url.Error{Op: "Get", URL: "https://example.com", Err: cause}),
			want: cause,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := unwrapURLError(tt.err); got != tt.want {
				t.Errorf("unwrapURLError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
