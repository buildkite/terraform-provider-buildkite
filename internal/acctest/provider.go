// Package acctest provides setup for live acceptance test packages.
package acctest

import (
	"net/http"
	"os"
	"testing"

	"github.com/buildkite/terraform-provider-buildkite/buildkite"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/shurcooL/graphql"
)

// PreCheck verifies credentials before a live Terraform test runs.
func PreCheck(t *testing.T) {
	t.Helper()
	organization, ok := os.LookupEnv("BUILDKITE_ORGANIZATION_SLUG")
	if !ok {
		organization = os.Getenv("BUILDKITE_ORGANIZATION")
	}
	if organization == "" {
		t.Fatal("BUILDKITE_ORGANIZATION_SLUG must be set for acceptance tests")
	}
	if os.Getenv("BUILDKITE_API_TOKEN") == "" {
		t.Fatal("BUILDKITE_API_TOKEN must be set for acceptance tests")
	}
}

// ProviderFactories creates fresh provider instances through the public entry point.
func ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"buildkite": providerserver.NewProtocol6WithError(buildkite.New("testing")),
	}
}

// GraphQLClient constructs a client for remote assertions without making a request.
// Endpoint overrides match the provider, including an explicitly empty override.
func GraphQLClient() *graphql.Client {
	endpoint, ok := os.LookupEnv("BUILDKITE_GRAPHQL_URL")
	if !ok {
		endpoint = "https://graphql.buildkite.com/v1"
	}
	return graphql.NewClient(endpoint, &http.Client{
		Transport: &authenticatedTransport{
			token: os.Getenv("BUILDKITE_API_TOKEN"),
			next:  http.DefaultTransport,
		},
	})
}

type authenticatedTransport struct {
	token string
	next  http.RoundTripper
}

func (t *authenticatedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	request := req.Clone(req.Context())
	request.Header.Set("Authorization", "Bearer "+t.token)
	request.Header.Set("User-Agent", "testing")
	return t.next.RoundTrip(request)
}
