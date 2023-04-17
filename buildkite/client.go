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
}

// NewClient creates a client to use for interacting with the Buildkite API
func NewClient(org, apiToken, graphqlUrl, restUrl string) *Client {
	token := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: apiToken})
	httpClient := oauth2.NewClient(context.Background(), token)

	return &Client{
		graphql:      graphql.NewClient(graphqlUrl, httpClient),
		genqlient:    genqlient.NewClient(graphqlUrl, httpClient),
		http:         httpClient,
		organization: org,
		restUrl:      restUrl,
	}
}

func (client *Client) makeRequest(method string, path string, postData interface{}, responseObject interface{}) error {
	var bodyBytes io.Reader
	if postData != nil {
		jsonPayload, err := json.Marshal(postData)
		if err != nil {
			return err
		}
		bodyBytes = bytes.NewBuffer(jsonPayload)
	}

	url := fmt.Sprintf("%s%s", client.restUrl, path)

	req, err := http.NewRequest(method, url, bodyBytes)
	if err != nil {
		return err
	}

	resp, err := client.http.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("Buildkite API request failed: %s %s (status: %d)", method, url, resp.StatusCode)
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
