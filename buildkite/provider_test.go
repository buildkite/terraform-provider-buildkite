package buildkite

import (
	"context"
	"net/http"
	"os"
	"testing"

	genqlient "github.com/Khan/genqlient/graphql"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov5"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-mux/tf5to6server"
	"github.com/hashicorp/terraform-plugin-mux/tf6muxserver"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
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

func protoV5ProviderFactories() map[string]func() (tfprotov5.ProviderServer, error) {
	return map[string]func() (tfprotov5.ProviderServer, error){
		"buildkite": providerserver.NewProtocol5WithError(New("testing")),
	}
}

func protoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	upgradedSdkServer, err := tf5to6server.UpgradeServer(
		context.Background(),
		Provider("testing").GRPCProvider,
	)

	if err != nil {
		panic(err)
	}

	providers := []func() tfprotov6.ProviderServer{
		providerserver.NewProtocol6(New("testing")),
		func() tfprotov6.ProviderServer {
			return upgradedSdkServer
		},
	}

	muxServer, err := tf6muxserver.NewMuxServer(context.Background(), providers...)
	return map[string]func() (tfprotov6.ProviderServer, error){
		"buildkite": func() (tfprotov6.ProviderServer, error) {
			return muxServer.ProviderServer(), nil
		},
	}
}

// TestProvider just does basic validation to ensure the schema is defined and supporting functions exist
func TestProvider(t *testing.T) {
	if err := Provider("").InternalValidate(); err != nil {
		t.Fatalf("err: %s", err)
	}
}

func TestProvider_impl(t *testing.T) {
	var _ *schema.Provider = Provider("")
}

func testAccPreCheck(t *testing.T) {
	if v := getenv("BUILDKITE_ORGANIZATION_SLUG"); v == "" {
		t.Fatal("BUILDKITE_ORGANIZATION_SLUG must be set for acceptance tests")
	}
	if v := os.Getenv("BUILDKITE_API_TOKEN"); v == "" {
		t.Fatal("BUILDKITE_API_TOKEN must be set for acceptance tests")
	}
}
