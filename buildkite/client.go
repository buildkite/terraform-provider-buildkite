package buildkite

import (
	"context"

	"github.com/shurcooL/graphql"
	"golang.org/x/oauth2"
)

const graphqlEndpoint = "https://graphql.buildkite.com/v1"

type Client struct {
	graphql      *graphql.Client
	organization string
}

func NewClient(org, apiToken string) *Client {
	token := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: apiToken})
	httpClient := oauth2.NewClient(context.Background(), token)

	return &Client{
		graphql:      graphql.NewClient(graphqlEndpoint, httpClient),
		organization: org,
	}
}
