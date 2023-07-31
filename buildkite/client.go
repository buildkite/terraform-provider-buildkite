package buildkite

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	genqlient "github.com/Khan/genqlient/graphql"
	"github.com/shurcooL/graphql"
)

// Client can be used to interact with the Buildkite API
type Client struct {
	graphql        *graphql.Client
	genqlient      genqlient.Client
	http           *http.Client
	organization   string
	organizationId string
	restUrl        string
}

type clientConfig struct {
	org        string
	apiToken   string
	graphqlURL string
	restURL    string
	userAgent  string
}

type headerRoundTripper struct {
	next   http.RoundTripper
	Header http.Header
}

// NewClient creates a client to use for interacting with the Buildkite API
func NewClient(config *clientConfig) (*Client, error) {

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
	orgId, err := GetOrganizationID(config.org, graphqlClient)

	if err != nil {
		return nil, err
	}

	return &Client{
		graphql:        graphqlClient,
		genqlient:      genqlient.NewClient(config.graphqlURL, httpClient),
		http:           httpClient,
		organization:   config.org,
		organizationId: orgId,
		restUrl:        config.restURL,
	}, nil
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

func (client *Client) makeRequest(method string, path string, postData interface{}, responseObject interface{}) error {
	var bodyBytes io.Reader
	if postData != nil {
		jsonPayload, err := json.Marshal(postData)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
		bodyBytes = bytes.NewBuffer(jsonPayload)
	}

	url := fmt.Sprintf("%s%s", client.restUrl, path)

	req, err := http.NewRequest(method, url, bodyBytes)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.http.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("Buildkite API request failed: %s %s (status: %d)", method, url, resp.StatusCode)
	} else if resp.StatusCode == 204 {
		return nil
	}
	defer resp.Body.Close()

	responseBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if err := json.Unmarshal(responseBody, responseObject); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return nil
}
