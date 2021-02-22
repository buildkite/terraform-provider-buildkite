package buildkite

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

// Confirm that we can create a new agent token, and then delete it without error
func TestAccDataPipeline_read(t *testing.T) {
	var resourcePipeline PipelineNode

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckPipelineResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccDataPipelineConfigBasic("foo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Confirm the pipeline data source has the correct values in terraform state
					resource.TestCheckResourceAttr("data.buildkite_pipeline.foobar", "name", "Test Pipeline foo"),
					resource.TestCheckResourceAttr("data.buildkite_pipeline.foobar", "repository", "https://github.com/buildkite/terraform-provider-buildkite.git"),
					resource.TestCheckResourceAttr("data.buildkite_pipeline.foobar", "default_branch", "main"),
					resource.TestCheckResourceAttr("data.buildkite_pipeline.foobar", "description", "A test pipeline foo"),
					resource.TestMatchResourceAttr("data.buildkite_pipeline.foobar", "webhook_url", regexp.MustCompile("^https://webhook.buildkite.com/deliver/.+")),
				),
			},
		},
	})
}

func testAccDataPipelineConfigBasic(name string) string {
	config := `
		resource "buildkite_pipeline" "foobar" {
			name = "Test Pipeline %s"
			repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
			default_branch = "main"
			description = "A test pipeline %s"
			steps = ""
		}

		data "buildkite_pipeline" "foobar" {
			slug = buildkite_pipeline.foobar.slug
		}
	`
	return fmt.Sprintf(config, name, name)
}
