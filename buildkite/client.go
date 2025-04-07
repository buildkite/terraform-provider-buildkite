package buildkite

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
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

// NewClient creates a client for interacting with the Buildkite API.
//
// https://buildkite.com/docs/apis/rest-api/limits
//
// For REST API calls:
//  1. Uses hashicorp/go-retryablehttp to provide automatic retries with exponential backoff
//  2. Maximum of 5 retry attempts for requests that fail with retryable errors
//  3. Rate limiting strategy:
//     - Checks RateLimit-Reset header to determine when the rate limit will be reset
//     - Waits until the reset time plus a small buffer before retrying
//     - Falls back to Retry-After header if reset time isn't available
//  4. Also retries server errors (HTTP 502-504) with exponential backoff
//  5. All retryable requests have a minimum wait of 1 second and maximum of 30 seconds

func NewClient(config *clientConfig) *Client {
	// Create standard HTTP client for GraphQL
	standardHttpClient := &http.Client{}

	// Set timeout if configured
	readTimeout, diags := config.timeouts.Read(context.Background(), DefaultTimeout)
	if !diags.HasError() {
		standardHttpClient.Timeout = readTimeout
	}

	// Set up authentication and user agent headers
	header := make(http.Header)
	header.Set("Authorization", "Bearer "+config.apiToken)
	header.Set("User-Agent", config.userAgent)
	standardHttpClient.Transport = newHeaderRoundTripper(standardHttpClient.Transport, header)

	// Create retryable client with rate limit handling for REST API calls
	retryClient := retryablehttp.NewClient()

	// TODO: Make configurable?
	retryClient.RetryMax = 5
	retryClient.RetryWaitMin = 5 * time.Second
	retryClient.RetryWaitMax = 60 * time.Second
	retryClient.Logger = nil

	if !diags.HasError() {
		retryClient.HTTPClient.Timeout = readTimeout
	}

	// Configure smart backoff strategy for rate limits
	retryClient.Backoff = func(min, max time.Duration, attemptNum int, resp *http.Response) time.Duration {
		if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
			// Try to use RateLimit-Reset header first
			if resetHeader := resp.Header.Get("RateLimit-Reset"); resetHeader != "" {
				if resetTime, err := strconv.ParseInt(resetHeader, 10, 64); err == nil {
					resetAt := time.Unix(resetTime, 0)
					waitTime := time.Until(resetAt) + (1 * time.Second)
					tflog.Debug(context.Background(), fmt.Sprintf("Rate limit hit, reset at: %v (waiting: %v)", resetAt, waitTime))

					// Return the wait time within min-max bounds
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
			remaining := resp.Header.Get("RateLimit-Remaining")
			reset := resp.Header.Get("RateLimit-Reset")
			tflog.Debug(ctx, fmt.Sprintf("Buildkite API returned %d - retrying (Remaining: %s, Reset: %s)",
				resp.StatusCode, remaining, reset))
			return true, nil
		}

		return false, nil
	}

	// Apply the same headers to retry client
	retryClient.HTTPClient.Transport = newHeaderRoundTripper(retryClient.HTTPClient.Transport, header)
	restHttpClient := retryClient.StandardClient()

	// Create GraphQL client with standard HTTP client (no rate limit handling)
	graphqlClient := graphql.NewClient(config.graphqlURL, standardHttpClient)

	return &Client{
		graphql:        graphqlClient,
		genqlient:      genqlient.NewClient(config.graphqlURL, standardHttpClient),
		http:           restHttpClient, // For REST API calls with rate limit handling
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

// GraphQL-specific retry functions

func isRetryableError(err error) bool {
	return isRateLimited(err) || isServerError(err)
}

func isRateLimited(err error) bool {
	// see: https://github.com/Khan/genqlient/blob/main/graphql/client.go#L167
	r := regexp.MustCompile(`returned error (\d{3}):`)
	if match := r.FindString(err.Error()); match != "" {
		code, _ := strconv.Atoi(match)
		if code == http.StatusTooManyRequests {
			return true
		}
	}
	return false
}

func isServerError(err error) bool {
	// see: https://github.com/Khan/genqlient/blob/main/graphql/client.go#L167
	r := regexp.MustCompile(`returned error (\d{3}):`)
	if match := r.FindString(err.Error()); match != "" {
		code, _ := strconv.Atoi(match)
		if code >= http.StatusBadGateway && code <= http.StatusGatewayTimeout {
			return true
		}
	}
	return false
}

// NOTE: retryContextError function is defined in util.go and used for GraphQL retries

func (client *Client) makeRequest(ctx context.Context, method string, path string, postData interface{}, responseObject interface{}) error {
	readTimeout, diags := client.timeouts.Read(ctx, DefaultTimeout)
	if !diags.HasError() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, readTimeout)
		defer cancel()
	}

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
