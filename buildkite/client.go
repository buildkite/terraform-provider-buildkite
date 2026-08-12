package buildkite

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	genqlient "github.com/Khan/genqlient/graphql"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/shurcooL/graphql"
)

// Client can be used to interact with the Buildkite API
type Client struct {
	graphql          *graphql.Client
	genqlient        genqlient.Client
	http             *http.Client
	organization     string
	organizationId   *string
	organizationIdMu sync.Mutex
	restURL          string
	timeouts         timeouts.Value

	// Retained so tests can assert the retry configuration and shorten the waits.
	restRetry    *retryablehttp.Client
	graphqlRetry *retryablehttp.Client
}

type clientConfig struct {
	org        string
	apiToken   string
	graphqlURL string
	restURL    string
	userAgent  string
	timeouts   timeouts.Value
	maxRetries int
}

type headerRoundTripper struct {
	next   http.RoundTripper
	Header http.Header
}

func (client *Client) GetOrganizationID() (*string, error) {
	client.organizationIdMu.Lock()
	defer client.organizationIdMu.Unlock()
	if client.organizationId != nil {
		return client.organizationId, nil
	}
	orgId, err := GetOrganizationID(client.organization, client.graphql)
	if err != nil {
		return nil, err
	}
	// Cache only on success; a cached empty ID would be served on later retries.
	client.organizationId = &orgId

	return client.organizationId, nil
}

// NewClient creates a client for interacting with the Buildkite API.
//
// https://buildkite.com/docs/apis/rest-api/limits
//
// For REST API calls:
//  1. Uses hashicorp/go-retryablehttp to provide automatic retries with smart backoff
//  2. Maximum of 10 retry attempts for requests that fail with retryable errors
//  3. Rate limiting strategy:
//     - Checks RateLimit-Reset header (seconds until reset) to determine how long to wait
//     - Waits for the specified duration plus a small buffer before retrying
//     - Falls back to Retry-After header if reset time isn't available
//  4. Also retries server errors (HTTP 500-599), doubling the wait after each attempt
//  5. Retryable requests wait a minimum of 15 seconds and a maximum of 180 seconds, except that a
//     Retry-After this code does not handle itself is passed through to retryablehttp and honoured
//     as given, which can fall below the minimum or exceed the maximum
//  6. The deadline makeRequest derives from the read timeout bounds the whole request including the
//     waits between attempts, so on REST it, not max_retries, is usually what ends a sustained
//     failure. GraphQL calls are not bounded this way, since they never pass through makeRequest.
func NewClient(config *clientConfig) *Client {
	readTimeout, diags := config.timeouts.Read(context.Background(), DefaultTimeout)

	commonHeaders := make(http.Header)
	commonHeaders.Set("Authorization", "Bearer "+config.apiToken)
	commonHeaders.Set("User-Agent", config.userAgent)

	// Common Backoff strategy for retryable clients
	sharedBackoff := func(min, max time.Duration, attemptNum int, resp *http.Response) time.Duration {
		if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
			// Try RateLimit-Reset first (seconds until reset)
			if resetHeader := resp.Header.Get("RateLimit-Reset"); resetHeader != "" {
				if resetSeconds, err := strconv.ParseInt(resetHeader, 10, 64); err == nil {
					// Add a 2-second buffer to ensure we're past the reset time
					waitTime := time.Duration(resetSeconds)*time.Second + (2 * time.Second)
					tflog.Debug(context.Background(), fmt.Sprintf("Rate limit hit, retry after: %v", waitTime))
					if waitTime < min {
						return min
					}
					if waitTime > max {
						return max
					}
					return waitTime
				}
			}
			// Fall back to Retry-After header if available
			if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
				if seconds, err := strconv.ParseInt(retryAfter, 10, 64); err == nil {
					waitTime := time.Duration(seconds)*time.Second + (2 * time.Second)
					tflog.Debug(context.Background(), fmt.Sprintf("Rate limit hit, retry after: %v", waitTime))
					if waitTime < min {
						return min
					}
					if waitTime > max {
						return max
					}
					return waitTime
				}
			}
		}

		// Use exponential backoff for server errors (min * 2^attempt, capped at max)
		return retryablehttp.DefaultBackoff(min, max, attemptNum, resp)
	}

	// Common CheckRetry policy for retryable clients
	sharedCheckRetry := func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		capture := captureFrom(ctx)

		if err != nil {
			capture.noteAttempt()
			return retryablehttp.DefaultRetryPolicy(ctx, resp, err)
		}
		if resp.StatusCode == http.StatusTooManyRequests || (resp.StatusCode >= 500 && resp.StatusCode < 600) {
			// Only the retried statuses can lose their response, so only they are worth keeping.
			capture.noteResponse(resp)
			tflog.Debug(ctx, fmt.Sprintf("Buildkite API returned %d - retrying (RateLimit-Remaining: %s, RateLimit-Reset: %s)",
				resp.StatusCode, resp.Header.Get("RateLimit-Remaining"), resp.Header.Get("RateLimit-Reset")))
			return true, nil
		}

		capture.noteAttempt()
		return false, nil
	}

	// By default retryablehttp drains the final response and reports only how many attempts it made,
	// so a request that exhausts its retries loses the status code and whatever the API said was
	// wrong. Hand the response back and let the caller report it; the caller closes the body as it
	// does for any other response.
	//
	// This is deliberately only installed on the REST client. genqlient turns a non-200 into an
	// *HTTPError carrying the raw body and no Unwrap, which drops isResourceNotFoundError (util.go)
	// into a regex fallback over that body, so a 5xx page containing "not found" would remove a live
	// resource from state. Enabling it here means every caller-side matcher has to key off the status
	// rather than the prose around it, which is why the REST callers anchor on "status: 404" and why
	// isTransientError ignores errors carrying a status at all.
	restErrorHandler := func(resp *http.Response, err error, numTries int) (*http.Response, error) {
		if resp != nil && err == nil {
			return resp, nil
		}
		if resp != nil {
			// net/http hands back a response alongside an error when it stops following redirects, and
			// has already closed the body; a second Close is harmless.
			logCtx := context.Background()
			if resp.Request != nil {
				logCtx = resp.Request.Context()
			}
			if closeErr := resp.Body.Close(); closeErr != nil {
				tflog.Warn(logCtx, "Failed to close response body", map[string]interface{}{"error": closeErr.Error()})
			}
		}
		if err == nil {
			// Unreachable with the current library, which only gives up without an error when it has a
			// response. Guarded anyway: returning (nil, nil) would nil-dereference in the caller.
			err = errors.New("no attempt produced a response")
		}

		// The count is carried rather than formatted in. makeRequest renders its own from the capture,
		// alongside the request itself, and strips this one; the registry resources call client.http.Do
		// directly and keep no capture, so for them this is the only place the count survives.
		return nil, &attemptsError{attempts: numTries, err: unwrapURLError(err)}
	}

	// REST Client Setup
	restRetryClient := retryablehttp.NewClient()
	restRetryClient.RetryMax = config.maxRetries
	// Hardcode wait times following AWS provider pattern
	restRetryClient.RetryWaitMin = DefaultRetryWaitMinSeconds * time.Second
	restRetryClient.RetryWaitMax = DefaultRetryWaitMaxSeconds * time.Second
	restRetryClient.Logger = nil // Using tflog directly
	restRetryClient.Backoff = sharedBackoff
	restRetryClient.CheckRetry = sharedCheckRetry
	restRetryClient.ErrorHandler = restErrorHandler
	if !diags.HasError() && readTimeout > 0 {
		restRetryClient.HTTPClient.Timeout = readTimeout
	}
	// Add auth headers to the underlying transport of the REST retry client
	restRetryClient.HTTPClient.Transport = newHeaderRoundTripper(restRetryClient.HTTPClient.Transport, commonHeaders)
	restHttpClient := restRetryClient.StandardClient()

	// GraphQL Client Setup. Note it gets no ErrorHandler: see restErrorHandler above before adding one.
	graphqlRetryClient := retryablehttp.NewClient()
	graphqlRetryClient.RetryMax = config.maxRetries // Same retry policy as REST
	graphqlRetryClient.RetryWaitMin = DefaultRetryWaitMinSeconds * time.Second
	graphqlRetryClient.RetryWaitMax = DefaultGraphQLWaitMaxSeconds * time.Second
	graphqlRetryClient.Logger = nil // Using tflog directly
	graphqlRetryClient.Backoff = sharedBackoff
	graphqlRetryClient.CheckRetry = sharedCheckRetry
	if !diags.HasError() && readTimeout > 0 {
		graphqlRetryClient.HTTPClient.Timeout = readTimeout
	}
	// Add auth headers to the underlying transport of the GraphQL retry client
	graphqlRetryClient.HTTPClient.Transport = newHeaderRoundTripper(graphqlRetryClient.HTTPClient.Transport, commonHeaders)
	graphqlHttpClient := graphqlRetryClient.StandardClient()

	graphqlClient := graphql.NewClient(config.graphqlURL, graphqlHttpClient)

	return &Client{
		graphql:        graphqlClient,
		genqlient:      genqlient.NewClient(config.graphqlURL, graphqlHttpClient),
		http:           restHttpClient,
		organization:   config.org,
		organizationId: nil,
		restURL:        config.restURL,
		timeouts:       config.timeouts,
		restRetry:      restRetryClient,
		graphqlRetry:   graphqlRetryClient,
	}
}

func newHeaderRoundTripper(next http.RoundTripper, header http.Header) *headerRoundTripper {
	if next == nil {
		next = http.DefaultTransport
	}
	return &headerRoundTripper{
		next:   next,
		Header: header,
	}
}

func (rt *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if rt.Header != nil {
		for k, v := range rt.Header {
			req.Header[k] = v
		}
	}
	return rt.next.RoundTrip(req)
}

// Enough to carry an API error message without letting an unexpected payload into an error string.
const maxCapturedBodyBytes = 2048

// apiRequestFailedPrefix opens every error describing an HTTP status response.
const apiRequestFailedPrefix = "the Buildkite API request failed:"

type lastResponseKey struct{}

// lastResponseCapture records what the most recent attempt saw. When the read deadline expires
// during a backoff wait, retryablehttp returns the context error and discards the response, so
// without this the caller cannot say what the API was actually complaining about.
type lastResponseCapture struct {
	attempts int
	status   int
	body     string
	// stale marks a stored response as belonging to an earlier attempt than the one that ended the
	// request, so it is reported as context rather than as the cause.
	stale bool
}

// reopenedBody hands a body that has already been read back to the next reader while still closing
// the original.
type reopenedBody struct {
	io.Reader
	io.Closer
}

// captureFrom returns the capture carried by a request context, or nil for requests that do not
// install one. Only makeRequest installs one, so GraphQL calls and the registry resources that use
// client.http directly get nil, and the methods below are no-ops for them.
func captureFrom(ctx context.Context) *lastResponseCapture {
	capture, _ := ctx.Value(lastResponseKey{}).(*lastResponseCapture)
	return capture
}

// noteAttempt counts an attempt whose response cannot stand in for the failure, either because the
// request never produced one or because the caller is about to receive it directly. Marking any
// stored response stale matters as much as the count: a status held over from an earlier attempt
// reported as the cause of a later, unrelated failure sends the reader after the wrong problem.
func (c *lastResponseCapture) noteAttempt() {
	if c == nil {
		return
	}

	c.attempts++
	c.stale = true
}

// noteResponse counts an attempt and keeps a bounded prefix of its body, putting the prefix back so
// the response stays readable for whoever consumes it next. retryablehttp passes CheckRetry the
// response before draining it, which is the last point the body is available. io.ReadAll returns
// what it managed to read alongside any error, so the prefix is stored either way rather than
// leaving this attempt's status paired with an earlier attempt's message.
func (c *lastResponseCapture) noteResponse(resp *http.Response) {
	if c == nil {
		return
	}

	c.attempts++
	c.status = resp.StatusCode
	c.stale = false

	prefix, _ := io.ReadAll(io.LimitReader(resp.Body, maxCapturedBodyBytes))
	c.body = string(prefix)
	if len(prefix) == maxCapturedBodyBytes {
		c.body += " (truncated)"
	}
	resp.Body = reopenedBody{Reader: io.MultiReader(bytes.NewReader(prefix), resp.Body), Closer: resp.Body}
}

// apiError describes a failed REST request, whether the response is in hand or only what the retry
// loop kept of it. Callers switch on StatusCode rather than reading the message: Body is whatever
// the API, or a proxy in front of it, chose to return, and a 5xx page is free to mention "404" or
// "not found" without meaning either.
type apiError struct {
	Method string
	URL    string
	// StatusCode is the status of the response that ended the request, or 0 when a transport or
	// context error ended it instead.
	StatusCode int
	Attempts   int
	Body       string
	// earlierStatus and earlierBody carry what a previous attempt saw when a later one died in
	// flight. They are context rather than the cause, so they stay out of StatusCode.
	earlierStatus int
	earlierBody   string
	// Err is the transport or context error that ended the request, where one did.
	Err error
}

func (e *apiError) Error() string {
	var message string
	if e.StatusCode == 0 {
		message = fmt.Sprintf("failed to send request: %s %s: %s", e.Method, e.URL, e.Err)
	} else {
		message = fmt.Sprintf("%s %s %s (status: %d)", apiRequestFailedPrefix, e.Method, e.URL, e.StatusCode)
		if e.Body != "" {
			message += ": " + e.Body
		}
	}

	if e.Attempts > 1 {
		message += fmt.Sprintf(" (after %d attempts)", e.Attempts)
	}
	if e.StatusCode != 0 && e.Err != nil {
		message += ": " + e.Err.Error()
	}
	if e.earlierStatus != 0 {
		message += fmt.Sprintf(" (an earlier attempt returned status %d: %s)", e.earlierStatus, e.earlierBody)
	}

	return message
}

// Unwrap keeps the deadline or transport error reachable, so errors.Is(err, context.DeadlineExceeded)
// still answers for a request that ran out of time part way through the retry schedule.
func (e *apiError) Unwrap() error { return e.Err }

// isAPIStatus reports whether err describes a REST response carrying the given status.
func isAPIStatus(err error, status int) bool {
	var apiErr *apiError
	return errors.As(err, &apiErr) && apiErr.StatusCode == status
}

// attemptsError reports how many attempts a request took. retryablehttp does not pass the request to
// its ErrorHandler, so a caller's own state is out of reach there and the count has to travel with
// the error instead. A single attempt says nothing worth saying, so it says nothing.
type attemptsError struct {
	attempts int
	err      error
}

func (e *attemptsError) Error() string {
	if e.attempts > 1 {
		return fmt.Sprintf("after %d attempts: %s", e.attempts, e.err)
	}

	return e.err.Error()
}

func (e *attemptsError) Unwrap() error { return e.err }

// requestCause reduces the error the http client returned to the part worth reporting. The
// *url.Error repeats a request the message names already, and the attempt count belongs to whoever
// is building the message rather than to what restErrorHandler wrote for callers building none.
func requestCause(err error) error {
	var attemptsErr *attemptsError
	if errors.As(err, &attemptsErr) {
		return attemptsErr.err
	}

	return unwrapURLError(err)
}

// unwrapURLError returns the cause net/http wrapped in a *url.Error, so a message that already
// names the request does not repeat the URL.
func unwrapURLError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}

func (client *Client) makeRequest(ctx context.Context, method string, path string, postData interface{}, responseObject interface{}) error {
	readTimeout, diags := client.timeouts.Read(ctx, DefaultTimeout)
	if !diags.HasError() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, readTimeout)
		defer cancel()
	}

	lastResponse := &lastResponseCapture{}
	ctx = context.WithValue(ctx, lastResponseKey{}, lastResponse)

	bodyBytes := io.Reader(nil)
	if postData != nil {
		jsonPayload, err := json.Marshal(postData)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
		bodyBytes = bytes.NewBuffer(jsonPayload)
	}

	requestURL := fmt.Sprintf("%s%s", client.restURL, path)

	req, err := http.NewRequestWithContext(ctx, method, requestURL, bodyBytes)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add content-type header for POST/PUT requests with body
	if (method == http.MethodPost || method == http.MethodPut) && bodyBytes != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.http.Do(req)
	if err != nil {
		// A retryable status that never clears usually ends up here rather than in the status check
		// below: if the deadline lands during a backoff wait, retryablehttp returns the context error
		// and the response is already gone.
		switch {
		case lastResponse.status != 0 && !lastResponse.stale:
			return &apiError{
				Method:     method,
				URL:        requestURL,
				StatusCode: lastResponse.status,
				Attempts:   lastResponse.attempts,
				Body:       lastResponse.body,
				Err:        requestCause(err),
			}

		case lastResponse.status != 0:
			// The last attempt died in flight rather than during a wait, so what the API said earlier
			// is context, not the cause, and no StatusCode is claimed for it.
			return &apiError{
				Method:        method,
				URL:           requestURL,
				Attempts:      lastResponse.attempts,
				earlierStatus: lastResponse.status,
				earlierBody:   lastResponse.body,
				Err:           requestCause(err),
			}

		default:
			return &apiError{
				Method:   method,
				URL:      requestURL,
				Attempts: lastResponse.attempts,
				Err:      requestCause(err),
			}
		}
	}

	if resp.StatusCode >= 400 {
		defer func() {
			if closeErr := resp.Body.Close(); closeErr != nil {
				tflog.Warn(ctx, "Failed to close response body", map[string]interface{}{"error": closeErr.Error()})
			}
		}()

		// Try to read the error body for better error messages, bounded because a 5xx body can be a
		// proxy's HTML error page rather than the API's own JSON.
		var errorMsg string
		errorBody, readErr := io.ReadAll(io.LimitReader(resp.Body, maxCapturedBodyBytes))
		if readErr == nil && len(errorBody) > 0 {
			errorMsg = string(errorBody)
		}
		if errorMsg == "" && !lastResponse.stale {
			// This read can fail for whatever reason the request did. The retry loop may already hold
			// a prefix of the same body from the attempt it gave up on.
			errorMsg = lastResponse.body
		}

		return &apiError{
			Method:     method,
			URL:        requestURL,
			StatusCode: resp.StatusCode,
			Attempts:   lastResponse.attempts,
			Body:       errorMsg,
		}
	} else if resp.StatusCode == 204 {
		if closeErr := resp.Body.Close(); closeErr != nil {
			tflog.Warn(ctx, "Failed to close response body", map[string]interface{}{"error": closeErr.Error()})
		}
		return nil
	}

	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			tflog.Warn(ctx, "Failed to close response body", map[string]interface{}{"error": closeErr.Error()})
		}
	}()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if err := json.Unmarshal(responseBody, responseObject); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return nil
}
