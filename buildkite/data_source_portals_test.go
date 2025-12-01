package buildkite

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccBuildkitePortalsDataSource(t *testing.T) {
	randName1 := acctest.RandString(10)
	randName2 := acctest.RandString(10)

	config := func(name1, name2 string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_portal" "test1" {
			slug = %q
			name = "Test Portal %s"
			description = "First test portal"
			query = "{ viewer { user { name } } }"
			user_invokable = false
		}

		resource "buildkite_portal" "test2" {
			slug = %q
			name = "Test Portal %s"
			description = "Second test portal"
			query = "{ viewer { user { email } } }"
			user_invokable = true
		}

		data "buildkite_portals" "all" {
			depends_on = [
				buildkite_portal.test1,
				buildkite_portal.test2
			]
		}
		`, fmt.Sprintf("test-portal-1-%s", name1), name1, fmt.Sprintf("test-portal-2-%s", name2), name2)
	}

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config(randName1, randName2),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.buildkite_portals.all", "portals.#"),
				),
			},
		},
	})
}
