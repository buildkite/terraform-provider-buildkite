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
	maxRetries int
}

type headerRoundTripper struct {
	next   http.RoundTripper
	Header http.Header
}

func (client *Client) GetOrganizationID() (*string, error) {
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
//  1. Uses hashicorp/go-retryablehttp to provide automatic retries with smart backoff
//  2. Maximum of 10 retry attempts for requests that fail with retryable errors
//  3. Rate limiting strategy:
//     - Checks RateLimit-Reset header to determine when the rate limit will be reset
//     - Waits until the reset time plus a small buffer before retrying
//     - Falls back to Retry-After header if reset time isn't available
//  4. Also retries server errors (HTTP 500-599) with linear jitter backoff
//  5. All retryable requests have a minimum wait of 15 seconds and maximum of 180 seconds

func NewClient(config *clientConfig) *Client {
	readTimeout, diags := config.timeouts.Read(context.Background(), DefaultTimeout)

	commonHeaders := make(http.Header)
	commonHeaders.Set("Authorization", "Bearer "+config.apiToken)
	commonHeaders.Set("User-Agent", config.userAgent)

	// Common Backoff strategy for retryable clients
	sharedBackoff := func(min, max time.Duration, attemptNum int, resp *http.Response) time.Duration {
		if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
			// Try RateLimit-Reset first (Unix timestamp format)
			if resetHeader := resp.Header.Get("RateLimit-Reset"); resetHeader != "" {
				if resetTime, err := strconv.ParseInt(resetHeader, 10, 64); err == nil {
					resetAt := time.Unix(resetTime, 0)
					// Add a 2-second buffer to ensure we're past the reset time
					waitTime := time.Until(resetAt) + (2 * time.Second)
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

		// Use linear backoff with jitter to spread out requests when retrying
		return retryablehttp.LinearJitterBackoff(min, max, attemptNum, resp)
	}

	// Common CheckRetry policy for retryable clients
	sharedCheckRetry := func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		if err != nil {
			return retryablehttp.DefaultRetryPolicy(ctx, resp, err)
		}
		if resp.StatusCode == http.StatusTooManyRequests || (resp.StatusCode >= 500 && resp.StatusCode < 600) {
			tflog.Debug(ctx, fmt.Sprintf("Buildkite API returned %d - retrying (RateLimit-Remaining: %s, RateLimit-Reset: %s)",
				resp.StatusCode, resp.Header.Get("RateLimit-Remaining"), resp.Header.Get("RateLimit-Reset")))
			return true, nil
		}
		return false, nil
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
	if !diags.HasError() && readTimeout > 0 {
		restRetryClient.HTTPClient.Timeout = readTimeout
	}
	// Add auth headers to the underlying transport of the REST retry client
	restRetryClient.HTTPClient.Transport = newHeaderRoundTripper(restRetryClient.HTTPClient.Transport, commonHeaders)
	restHttpClient := restRetryClient.StandardClient()

	// GraphQL Client Setup
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
	readTimeout, diags := client.timeouts.Read(ctx, DefaultTimeout)
	if !diags.HasError() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, readTimeout)
		defer cancel()
	}

	bodyBytes := io.Reader(nil)
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
		defer func() {
			if closeErr := resp.Body.Close(); closeErr != nil {
				tflog.Warn(ctx, "Failed to close response body", map[string]interface{}{"error": closeErr.Error()})
			}
		}()

		// Try to read the error body for better error messages
		var errorMsg string
		errorBody, readErr := io.ReadAll(resp.Body)
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
