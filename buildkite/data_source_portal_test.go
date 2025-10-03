package buildkite

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccBuildkitePortalDataSource(t *testing.T) {
	randName := acctest.RandString(10)

	config := func(name string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "10s"
				read = "10s"
				update = "10s"
				delete = "10s"
			}
		}

		resource "buildkite_portal" "test" {
			slug = %q
			name = "Test Portal %s"
			description = "Test portal description"
			query = "{ viewer { user { name } } }"
			user_invokable = false
		}

		data "buildkite_portal" "test" {
			slug = buildkite_portal.test.slug
		}
		`, fmt.Sprintf("test-portal-%s", name), name)
	}

	check := resource.ComposeAggregateTestCheckFunc(
		resource.TestCheckResourceAttr("data.buildkite_portal.test", "name", fmt.Sprintf("Test Portal %s", randName)),
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
