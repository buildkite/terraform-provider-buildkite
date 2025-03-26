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

	genqlient "github.com/Khan/genqlient/graphql"
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
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

// NewClient creates a client to use for interacting with the Buildkite API
func NewClient(config *clientConfig) *Client {

	// Setup a HTTP Client that can be used by all REST and graphql API calls,
	// with suitable headers for authentication and user agent identification
	rt := http.DefaultTransport
	header := make(http.Header)
	header.Set("Authorization", "Bearer "+config.apiToken)
	header.Set("User-Agent", config.userAgent)
	rt = newHeaderRoundTripper(rt, header)

	httpClient := &http.Client{
		Transport: rt,
	}

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

	resp, err := client.http.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("the Buildkite API request failed: %s %s (returned error %d)", method, url, resp.StatusCode)
	} else if resp.StatusCode == 204 {
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
