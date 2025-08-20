package buildkite

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccDataSourceRegistry_Basic(t *testing.T) {
	randName := acctest.RandString(10)
	resourceName := "buildkite_registry.test_reg"
	dataSourceName := "data.buildkite_registry.data_test_reg"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceRegistryConfigBasic(randName),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Check resource attributes
					resource.TestCheckResourceAttr(resourceName, "name", randName),
					resource.TestCheckResourceAttr(resourceName, "ecosystem", "java"),
					resource.TestCheckResourceAttr(resourceName, "emoji", ":package:"),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttrSet(resourceName, "uuid"),
					resource.TestCheckResourceAttrSet(resourceName, "slug"),
					resource.TestCheckResourceAttr(resourceName, "team_ids.0", "31529c8a-7cfa-42e8-bb85-4c844a983ea0"),

					// Check data source attributes against resource attributes
					resource.TestCheckResourceAttrPair(dataSourceName, "id", resourceName, "id"),
					resource.TestCheckResourceAttrPair(dataSourceName, "uuid", resourceName, "uuid"),
					resource.TestCheckResourceAttrPair(dataSourceName, "name", resourceName, "name"),
					resource.TestCheckResourceAttrPair(dataSourceName, "slug", resourceName, "slug"),
					resource.TestCheckResourceAttrPair(dataSourceName, "description", resourceName, "description"),
					resource.TestCheckResourceAttrPair(dataSourceName, "emoji", resourceName, "emoji"),
					resource.TestCheckResourceAttrPair(dataSourceName, "color", resourceName, "color"),
					// The REST API for a single registry doesn't return a full 'url' field directly.
					// resource.TestCheckResourceAttrPair(dataSourceName, "url", resourceName, "url"),
					resource.TestCheckResourceAttrPair(dataSourceName, "team_ids", resourceName, "team_ids"),

					// Check attributes now available via REST API
					resource.TestCheckResourceAttrPair(dataSourceName, "ecosystem", resourceName, "ecosystem"),
					resource.TestCheckResourceAttrPair(dataSourceName, "oidc_policy", resourceName, "oidc_policy"),
				),
			},
		},
	})
}

func testAccDataSourceRegistryConfigBasic(name string) string {
	// Using a known team ID from the existing resource_registry_test.go.
	const knownTeamID = "31529c8a-7cfa-42e8-bb85-4c844a983ea0"

	return fmt.Sprintf(`
		provider "buildkite" {}

		resource "buildkite_registry" "test_reg" {
			name        = "%s"
			ecosystem   = "java"
			description = "A test registry for data source testing."
			emoji       = ":package:"
			color       = "#123456"
			team_ids    = ["%s"]
		}

		data "buildkite_registry" "data_test_reg" {
			slug = buildkite_registry.test_reg.slug
		}
	`, name, knownTeamID)
}
