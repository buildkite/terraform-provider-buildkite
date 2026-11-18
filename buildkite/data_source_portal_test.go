package buildkite

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccBuildkitePortalDataSource(t *testing.T) {
	randName := testAccResourceName()

	config := func(name string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_portal" "test" {
			slug = %q
			name = "%s-portal"
			description = "Test portal description"
			query = "{ viewer { user { name } } }"
			user_invokable = false
		}

		data "buildkite_portal" "test" {
			slug = buildkite_portal.test.slug
		}
		`, fmt.Sprintf("%s-portal", name), name)
	}

	check := resource.ComposeAggregateTestCheckFunc(
		resource.TestCheckResourceAttr("data.buildkite_portal.test", "name", fmt.Sprintf("%s-portal", randName)),
		resource.TestCheckResourceAttr("data.buildkite_portal.test", "description", "Test portal description"),
		resource.TestCheckResourceAttr("data.buildkite_portal.test", "user_invokable", "false"),
		resource.TestCheckResourceAttrSet("data.buildkite_portal.test", "uuid"),
		resource.TestCheckResourceAttrSet("data.buildkite_portal.test", "created_at"),
	)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config(randName),
				Check:  check,
			},
		},
	})
}
