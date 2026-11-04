package buildkite

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	genqlient "github.com/Khan/genqlient/graphql"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/shurcooL/graphql"
)

var (
	graphqlClient    *graphql.Client
	genqlientGraphql genqlient.Client
	organizationID   string
)

func init() {
	rt := http.DefaultTransport
	header := make(http.Header)
	header.Set("Authorization", "Bearer "+os.Getenv("BUILDKITE_API_TOKEN"))
	header.Set("User-Agent", "testing")
	rt = newHeaderRoundTripper(rt, header)

	httpClient := &http.Client{
		Transport: rt,
	}

	graphqlEndpoint := defaultGraphqlEndpoint
	if v, ok := os.LookupEnv("BUILDKITE_GRAPHQL_URL"); ok {
		graphqlEndpoint = v
	}

	graphqlClient = graphql.NewClient(graphqlEndpoint, httpClient)
	genqlientGraphql = genqlient.NewClient(graphqlEndpoint, httpClient)
	if os.Getenv("TF_ACC") != "" {
		organizationID, _ = GetOrganizationID(context.Background(), getenv("BUILDKITE_ORGANIZATION_SLUG"), graphqlClient)
	}
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

// The same lifecycle tests run in the admin suite. Only the dedicated non-admin
// target requires a MEMBER token, so accidentally supplying an admin cannot make
// that job green without exercising non-admin permissions.
func testAccNonAdminPreCheck(t *testing.T) {
	t.Helper()
	testAccPreCheck(t)
	if os.Getenv("BUILDKITE_ACCEPTANCE_NONADMIN") != "1" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var viewer struct {
		Viewer struct {
			User struct{ Email graphql.String }
		}
	}
	if err := graphqlClient.Query(ctx, &viewer, nil); err != nil {
		t.Fatalf("read non-admin token owner: %v", err)
	}
	var membership struct {
		Organization struct {
			Members struct {
				Edges []struct {
					Node struct{ Role graphql.String }
				}
			} `graphql:"members(first: 1, email: $email)"`
		} `graphql:"organization(slug: $slug)"`
	}
	variables := map[string]any{
		"slug":  graphql.ID(getenv("BUILDKITE_ORGANIZATION_SLUG")),
		"email": viewer.Viewer.User.Email,
	}
	if err := graphqlClient.Query(ctx, &membership, variables); err != nil {
		t.Fatalf("read non-admin organization membership: %v", err)
	}
	edges := membership.Organization.Members.Edges
	if len(edges) != 1 || edges[0].Node.Role != "MEMBER" {
		t.Fatal("non-admin acceptance tests require a token owned by an organization MEMBER, not an ADMIN")
	}
}

// These fixtures use only GraphQL. In particular, do not configure slug,
// provider_settings, or github_webhooks_enabled: those require REST permissions.
func testAccPipelineConfigWithTeam(name, description string) string {
	teamSlug := os.Getenv("BUILDKITE_NONADMIN_TEAM_SLUG")
	if teamSlug == "" {
		teamSlug = "everyone"
	}
	return fmt.Sprintf(`
		data "buildkite_team" "nonadmin" {
			slug = %q
		}

		resource "buildkite_pipeline" "pipeline" {
			name = %q
			description = %q
			repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
			default_team_id = data.buildkite_team.nonadmin.id
		}
	`, teamSlug, name, description)
}
