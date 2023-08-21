package buildkite

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccBuildkitePipelineDataSource(t *testing.T) {
	var pipeline getPipelinePipeline
	pipelineName := acctest.RandString(12)

	loadPipeline := func(pipeline *getPipelinePipeline) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			slug := fmt.Sprintf("%s/%s", getenv("BUILDKITE_ORGANIZATION_SLUG"), pipelineName)
			resp, err := getPipeline(genqlientGraphql, slug)
			pipeline = &resp.Pipeline
			return err
		}
	}

	t.Run("pipeline datasource can be loaded from slug", func(t *testing.T) {
		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(`
						resource "buildkite_pipeline" "pipeline" {
							name = "%s"
							repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
						}

						data "buildkite_pipeline" "pipeline" {
							slug = buildkite_pipeline.pipeline.slug
						}
					`, pipelineName),
					Check: resource.ComposeAggregateTestCheckFunc(
						// Confirm the pipeline exists in the buildkite API
						loadPipeline(&pipeline),
						// Confirm the pipeline data source has the correct values in terraform state
						resource.TestCheckResourceAttr("data.buildkite_pipeline.pipeline", "name", pipelineName),
						resource.TestCheckResourceAttr("data.buildkite_pipeline.pipeline", "repository", "https://github.com/buildkite/terraform-provider-buildkite.git"),
						resource.TestCheckResourceAttrPair("data.buildkite_pipeline.pipeline", "id", "buildkite_pipeline.pipeline", "id"),
					),
				},
			},
		})
	})
}
