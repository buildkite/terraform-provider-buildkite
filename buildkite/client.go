package buildkite

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	genqlient "github.com/Khan/genqlient/graphql"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/shurcooL/graphql"
)

// Client can be used to interact with the Buildkite API
type Client struct {
	graphql        *graphql.Client
	genqlient      genqlient.Client
	http           *http.Client
	organization   string
	organizationId *string
	restURL        string
	timeouts       timeouts.Value
}

type clientConfig struct {
	org        string
	apiToken   string
	graphqlURL string
	restURL    string
	userAgent  string
	timeouts   timeouts.Value
}

type headerRoundTripper struct {
	next   http.RoundTripper
	Header http.Header
}

func (client Client) GetOrganizationID() (*string, error) {
	if client.organizationId != nil {
		return client.organizationId, nil
	}
	orgId, err := GetOrganizationID(client.organization, client.graphql)
	client.organizationId = &orgId
	if err != nil {
		return nil, err
	}

	return client.organizationId, nil
}

// NewClient creates a client for interacting with the Buildkite API with rate limit handling
//
// https://buildkite.com/docs/apis/rest-api/limits
//  1. The client uses hashicorp/go-retryablehttp to provide automatic retries with exponential backoff
//  2. Maximum of 5 retry attempts for any request that fails with a retryable error
//  3. For rate limited requests (HTTP 429), the client will:
//     - Check RateLimit-Reset header to check when the rate limit will be reset
//     - Wait until the reset time (plus a small buffer) before retrying
//     - Add jitter to avoid "thundering herd" issue when we'd retry multiple requests at the same time
//  4. For server errors (HTTP 502-504), the client will retry with exponential backoff
//  5. All retryable requests have a minimum wait of 1 second and maximum of 30 seconds

func NewClient(config *clientConfig) *Client {
	retryClient := retryablehttp.NewClient()

	// TODO: Make configurable?
	retryClient.RetryMax = 5
	retryClient.RetryWaitMin = 1 * time.Second // Start with 1 second wait
	retryClient.RetryWaitMax = 30 * time.Second
	retryClient.Logger = nil

	// Determines how long to wait before retrying a request
	retryClient.Backoff = func(min, max time.Duration, attemptNum int, resp *http.Response) time.Duration {
		// https://buildkite.com/docs/apis/rest-api/limits
		if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
			// Check for RateLimit-Reset header
			if resetHeader := resp.Header.Get("RateLimit-Reset"); resetHeader != "" {
				if resetTime, err := strconv.ParseInt(resetHeader, 10, 64); err == nil {
					// If we have a reset time, calculate wait time plus a small buffer.
					// Use a background context for logging as the request context might be cancelled.
					resetAt := time.Unix(resetTime, 0)
					waitTime := time.Until(resetAt) + (500 * time.Millisecond)
					tflog.Debug(context.Background(), fmt.Sprintf("Rate limit hit, reset at: %v (waiting: %v)", resetAt, waitTime))

					// Return the wait time, but ensure it's within min-max bounds
					if waitTime < min {
						return min
					}
					if waitTime > max {
						return max
					}
					return waitTime
				}
			}

			// Check for Retry-After header as fallback
			// Check for Retry-After header as fallback.
			// Use a background context for logging as the request context might be cancelled.
			if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
				if seconds, err := strconv.ParseInt(retryAfter, 10, 64); err == nil {
					waitTime := time.Duration(seconds) * time.Second
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
		return retryablehttp.DefaultBackoff(min, max, attemptNum, resp)
	}

	retryClient.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		if err != nil || resp == nil {
			return retryablehttp.DefaultRetryPolicy(ctx, resp, err)
		}

		if resp.StatusCode == http.StatusTooManyRequests || (resp.StatusCode >= 500 && resp.StatusCode < 600) {
			// Use the request context for logging here as it's still valid within CheckRetry.
			remaining := resp.Header.Get("RateLimit-Remaining")
			reset := resp.Header.Get("RateLimit-Reset")
			tflog.Debug(ctx, fmt.Sprintf("Buildkite API returned %d - retrying (Remaining: %s, Reset: %s)",
				resp.StatusCode, remaining, reset))
			return true, nil
		}

		return false, nil
	}

	header := make(http.Header)
	header.Set("Authorization", "Bearer "+config.apiToken)
	header.Set("User-Agent", config.userAgent)
	retryClient.HTTPClient.Transport = newHeaderRoundTripper(retryClient.HTTPClient.Transport, header)

	httpClient := retryClient.StandardClient()

	graphqlClient := graphql.NewClient(config.graphqlURL, httpClient)

	return &Client{
		graphql:        graphqlClient,
		genqlient:      genqlient.NewClient(config.graphqlURL, httpClient),
		http:           httpClient,
		organization:   config.org,
		organizationId: nil,
		restURL:        config.restURL,
		timeouts:       config.timeouts,
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

func (client *Client) makeRequest(ctx context.Context, method string, path string, postData interface{}, responseObject interface{}) error {
	var bodyBytes io.Reader
	if postData != nil {
		jsonPayload, err := json.Marshal(postData)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
		bodyBytes = bytes.NewBuffer(jsonPayload)
	}

	url := fmt.Sprintf("%s%s", client.restURL, path)

	req, err := http.NewRequestWithContext(ctx, method, url, bodyBytes)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add content-type header for POST/PUT requests with body
	if (method == http.MethodPost || method == http.MethodPut) && bodyBytes != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.http.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}

	if resp.StatusCode >= 400 {
		defer resp.Body.Close()

		// Try to read the error body for better error messages
		errorBody, readErr := io.ReadAll(resp.Body)
		errorMsg := ""
		if readErr == nil && len(errorBody) > 0 {
			errorMsg = string(errorBody)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			remaining := resp.Header.Get("RateLimit-Remaining")
			reset := resp.Header.Get("RateLimit-Reset")
			retryAfter := resp.Header.Get("Retry-After")

			tflog.Warn(ctx, fmt.Sprintf("Rate limit hit on REST API: %s %s (Remaining: %s, Reset: %s, Retry-After: %s)",
				method, url, remaining, reset, retryAfter))

			if errorMsg != "" {
				return fmt.Errorf("rate limit exceeded: %s %s (status: %d): %s",
					method, url, resp.StatusCode, errorMsg)
			}
			return fmt.Errorf("rate limit exceeded: %s %s (status: %d)",
				method, url, resp.StatusCode)
		}

		if errorMsg != "" {
			return fmt.Errorf("the Buildkite API request failed: %s %s (status: %d): %s",
				method, url, resp.StatusCode, errorMsg)
		}
		return fmt.Errorf("the Buildkite API request failed: %s %s (status: %d)",
			method, url, resp.StatusCode)
	} else if resp.StatusCode == 204 {
		resp.Body.Close()
		return nil
	}

	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if err := json.Unmarshal(responseBody, responseObject); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return nil
}
