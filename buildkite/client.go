package buildkite

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	genqlient "github.com/Khan/genqlient/graphql"
	"github.com/shurcooL/graphql"
	"golang.org/x/oauth2"
)

// Client can be used to interact with the Buildkite API
type Client struct {
	graphql      *graphql.Client
	genqlient    genqlient.Client
	http         *http.Client
	organization string
	restUrl      string
	userAgent    string
}

type clientConfig struct {
	org        string
	apiToken   string
	graphqlURL string
	restURL    string
	userAgent  string
}

// NewClient creates a client to use for interacting with the Buildkite API
func NewClient(config *clientConfig) *Client {
	token := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: config.apiToken})
	httpClient := oauth2.NewClient(context.Background(), token)

	return &Client{
		graphql:      graphql.NewClient(config.graphqlURL, httpClient),
		genqlient:    genqlient.NewClient(config.graphqlURL, httpClient),
		http:         httpClient,
		organization: config.org,
		restUrl:      config.restURL,
		userAgent:    config.userAgent,
	}
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

	req.Header.Add("User-Agent", client.userAgent)

	resp, err := client.http.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("Buildkite API request failed: %s %s (status: %d)", method, url, resp.StatusCode)
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
