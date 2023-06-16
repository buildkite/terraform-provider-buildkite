package buildkite

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"

	genqlient "github.com/Khan/genqlient/graphql"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-mux/tf5to6server"
	"github.com/hashicorp/terraform-plugin-mux/tf6muxserver"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/shurcooL/graphql"
)

var graphqlClient *graphql.Client
var genqlientGraphql genqlient.Client

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
	if v := os.Getenv("BUILDKITE_ORGANIZATION"); v == "" {
		t.Fatal("BUILDKITE_ORGANIZATION must be set for acceptance tests")
	}
	if v := os.Getenv("BUILDKITE_API_TOKEN"); v == "" {
		t.Fatal("BUILDKITE_API_TOKEN must be set for acceptance tests")
	}
}

func TestDataSource_UpgradeFromVersion(t *testing.T) {
	t.Skipf("Skipping until new provider has a resource created")
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				Config: `data "buildkite_team" "team" {
							slug = "everyone"
                        }`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.buildkite_team.team", "uuid", "31529c8a-7cfa-42e8-bb85-4c844a983ea0"),
					resource.TestCheckResourceAttr("data.buildkite_team.team", "slug", "everyone"),
				),
			},
			{
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				Config: `data "buildkite_team" "team" {
							slug = "everyone"
						}`,
				PlanOnly: true,
			},
		},
	})
}

// testAccCheckResourceDisappears verifies the Provider has had the resource removed from state
func testAccCheckResourceDisappears(provider *schema.Provider, resource *schema.Resource, resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		resourceState, ok := s.RootModule().Resources[resourceName]

		if !ok {
			return fmt.Errorf("resource not found: %s", resourceName)
		}

		if resourceState.Primary.ID == "" {
			return fmt.Errorf("resource ID missing: %s", resourceName)
		}

		if resource.DeleteContext != nil {
			client := Client{
				graphql:      graphqlClient,
				genqlient:    genqlientGraphql,
				organization: os.Getenv("BUILDKITE_ORGANIZATION"),
			}
			diags := resource.DeleteContext(context.Background(), resource.Data(resourceState.Primary), &client)

			for i := range diags {
				if diags[i].Severity == diag.Error {
					return fmt.Errorf("error deleting resource: %s", diags[i].Summary)
				}
			}

			return nil
		}

		return resource.Delete(resource.Data(resourceState.Primary), provider.Meta())
	}
}
