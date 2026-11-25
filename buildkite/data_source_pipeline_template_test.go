package buildkite

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccBuildkitePipelineTemplateDatasource(t *testing.T) {
	t.Parallel()

	configAltName := func(name, altName string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_pipeline_template" "template_bar" {
			name = "%s-template"
			configuration = "steps:\n  - label: \":pipeline:\"\n    command: \"buildkite-agent pipeline upload\""
		}

		data "buildkite_pipeline_template" "template_bar" {
			depends_on = [buildkite_pipeline_template.template_bar]
			name = "%s-template"
		}
		`, name, altName)
	}

	configAttrConflict := func(name string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_pipeline_template" "template_bar" {
			name = "%s-template"
			configuration = "steps:\n  - label: \":pipeline:\"\n    command: \"buildkite-agent pipeline upload\""
		}

		data "buildkite_pipeline_template" "template_bar" {
			depends_on = [buildkite_pipeline_template.template_bar]
			id = buildkite_pipeline_template.template_bar.id
			name = "%s-template"
		}
		`, name, name)
	}

	t.Run("errors when unable to find a pipeline template by name", func(t *testing.T) {
		randName := testAccResourceName()
		altName := acctest.RandString(12)
		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineTemplateDestroy,
			Steps: []resource.TestStep{
				{
					Config:      configAltName(randName, altName),
					ExpectError: regexp.MustCompile("Unable to find pipeline template"),
				},
			},
		})
	})

	t.Run("invalid attribute combination", func(t *testing.T) {
		randName := testAccResourceName()

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineTemplateDestroy,
			Steps: []resource.TestStep{
				{
					Config:      configAttrConflict(randName),
					ExpectError: regexp.MustCompile("Invalid Attribute Combination"),
				},
			},
		})
	})
}
