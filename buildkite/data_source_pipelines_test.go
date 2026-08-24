package buildkite

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccBuildkitePipelinesDatasource(t *testing.T) {
	fixtures := func(name string) string {
		return fmt.Sprintf(`
		resource "buildkite_cluster" "cluster" {
			name = "%s"
		}

		resource "buildkite_pipeline" "one" {
			name = "%s one"
			repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id = buildkite_cluster.cluster.id
		}

		resource "buildkite_pipeline" "two" {
			name = "%s two"
			repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id = buildkite_cluster.cluster.id
			tags = ["%s"]
		}
		`, name, name, name, name)
	}

	t.Run("pipelines data source can be loaded with defaults", func(t *testing.T) {
		name := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineDestroyFunc,
			Steps: []resource.TestStep{
				{
					Config: fixtures(name) + `
					data "buildkite_pipelines" "all" {
						depends_on = [buildkite_pipeline.one, buildkite_pipeline.two]
					}
					`,
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttrSet("data.buildkite_pipelines.all", "total"),
						resource.TestCheckResourceAttrSet("data.buildkite_pipelines.all", "pipelines.0.slug"),
						resource.TestCheckResourceAttrSet("data.buildkite_pipelines.all", "pipelines.1.slug"),
					),
				},
			},
		})
	})

	t.Run("pipelines data source can be filtered", func(t *testing.T) {
		name := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineDestroyFunc,
			Steps: []resource.TestStep{
				{
					Config: fixtures(name) + fmt.Sprintf(`
					data "buildkite_pipelines" "search" {
						search = "%s"
						cluster_id = buildkite_cluster.cluster.id
						archived = false
						depends_on = [buildkite_pipeline.one, buildkite_pipeline.two]
					}

					data "buildkite_pipelines" "tagged" {
						tags = ["%s"]
						depends_on = [buildkite_pipeline.one, buildkite_pipeline.two]
					}
					`, name, name),
					Check: resource.ComposeAggregateTestCheckFunc(
						// Confirm the search matches the two pipelines created above
						resource.TestCheckResourceAttr("data.buildkite_pipelines.search", "total", "2"),
						resource.TestCheckResourceAttr("data.buildkite_pipelines.search", "pipelines.#", "2"),
						resource.TestCheckResourceAttrPair("data.buildkite_pipelines.search", "pipelines.0.cluster_id", "buildkite_cluster.cluster", "id"),
						resource.TestCheckResourceAttr("data.buildkite_pipelines.search", "pipelines.0.repository", "https://github.com/buildkite/terraform-provider-buildkite.git"),
						// Confirm the tag matches only the second pipeline
						resource.TestCheckResourceAttr("data.buildkite_pipelines.tagged", "total", "1"),
						resource.TestCheckResourceAttrPair("data.buildkite_pipelines.tagged", "pipelines.0.id", "buildkite_pipeline.two", "id"),
						resource.TestCheckResourceAttrPair("data.buildkite_pipelines.tagged", "pipelines.0.uuid", "buildkite_pipeline.two", "uuid"),
					),
				},
			},
		})
	})
}
