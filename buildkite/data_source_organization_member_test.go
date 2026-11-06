package buildkite

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/shurcooL/graphql"
)

func TestAccNonAdminOrganizationMemberDatasources(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}
	testAccPreCheck(t)

	// Use the authenticated viewer rather than an arbitrary first member, so the
	// same lookup works for both credentials without hard-coding a test user.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var viewer struct {
		Viewer struct {
			User struct{ ID, UUID, Name, Email graphql.String }
		}
	}
	if err := graphqlClient.Query(ctx, &viewer, nil); err != nil {
		t.Fatalf("read acceptance token owner: %v", err)
	}
	user := viewer.Viewer.User

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccNonAdminPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					data "buildkite_organization_members" "members" {}

					data "buildkite_organization_member" "member" {
						email = %q
					}
				`, user.Email),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.buildkite_organization_member.member", "id", string(user.ID)),
					resource.TestCheckResourceAttr("data.buildkite_organization_member.member", "uuid", string(user.UUID)),
					resource.TestCheckResourceAttr("data.buildkite_organization_member.member", "name", string(user.Name)),
					resource.TestCheckResourceAttr("data.buildkite_organization_member.member", "email", string(user.Email)),
					resource.TestCheckTypeSetElemNestedAttrs("data.buildkite_organization_members.members", "members.*", map[string]string{
						"id": string(user.ID), "uuid": string(user.UUID), "name": string(user.Name), "email": string(user.Email),
					}),
				),
			},
		},
	})
}
