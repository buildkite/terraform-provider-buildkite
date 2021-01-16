package buildkite

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/shurcooL/graphql"
	"golang.org/x/oauth2"
)

const graphqlEndpoint = "https://graphql.buildkite.com/v1"

// Client can be used to interact with the Buildkite API
type Client struct {
	graphql      *graphql.Client
	http         *http.Client
	organization string
}

// NewClient creates a client to use for interacting with the Buildkite API
func NewClient(org, apiToken string) *Client {
	token := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: apiToken})
	httpClient := oauth2.NewClient(context.Background(), token)

	return &Client{
		graphql:      graphql.NewClient(graphqlEndpoint, httpClient),
		http:         httpClient,
		organization: org,
	}
}

func (client *Client) makeRequest(method string, url string, postData interface{}, responseObject interface{}) error {
	jsonPayload, err := json.Marshal(postData)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(method, url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return err
	}

	resp, err := client.http.Do(req)
	if err != nil && resp.StatusCode >= 400 {
		return err
	}

	defer resp.Body.Close()
	responseBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(responseBody, responseObject); err != nil {
		return err
	}

	return nil
}
