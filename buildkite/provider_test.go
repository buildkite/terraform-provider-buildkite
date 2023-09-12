package buildkite

import (
	"net/http"
	"os"
	"testing"

	genqlient "github.com/Khan/genqlient/graphql"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/shurcooL/graphql"
)

var graphqlClient *graphql.Client
var genqlientGraphql genqlient.Client
var organizationID string

func init() {
	rt := http.DefaultTransport
	header := make(http.Header)
	header.Set("Authorization", "Bearer "+os.Getenv("BUILDKITE_API_TOKEN"))
	header.Set("User-Agent", "testing")
	rt = newHeaderRoundTripper(rt, header)

	httpClient := &http.Client{
		Transport: rt,
	}

	graphqlClient = graphql.NewClient(defaultGraphqlEndpoint, httpClient)
	genqlientGraphql = genqlient.NewClient(defaultGraphqlEndpoint, httpClient)
	organizationID, _ = GetOrganizationID(getenv("BUILDKITE_ORGANIZATION_SLUG"), graphqlClient)
}

func protoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"buildkite": providerserver.NewProtocol6WithError(New("testing")),
	}
}

func testAccPreCheck(t *testing.T) {
	if v := getenv("BUILDKITE_ORGANIZATION_SLUG"); v == "" {
		t.Fatal("BUILDKITE_ORGANIZATION_SLUG must be set for acceptance tests")
	}
	if v := os.Getenv("BUILDKITE_API_TOKEN"); v == "" {
		t.Fatal("BUILDKITE_API_TOKEN must be set for acceptance tests")
	}
}
