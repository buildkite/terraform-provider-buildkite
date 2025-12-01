package buildkite

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccBuildkitePipelineTemplateDatasource(t *testing.T) {
	configId := func(name string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_pipeline_template" "template_foo" {
			name = "Template %s"
			configuration = "steps:\n  - label: \":pipeline:\"\n    command: \"buildkite-agent pipeline upload\""
		}

		data "buildkite_pipeline_template" "template_foo" {
			depends_on = [buildkite_pipeline_template.template_foo]
			id = buildkite_pipeline_template.template_foo.id
		}
		`, name)
	}

	configName := func(name string) string {
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
			name = "Template %s"
			configuration = "steps:\n  - label: \":pipeline:\"\n    command: \"buildkite-agent pipeline upload\""
		}

		data "buildkite_pipeline_template" "template_bar" {
			depends_on = [buildkite_pipeline_template.template_bar]
			name = "Template %s"
		}
		`, name, name)
	}

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
			name = "Template %s"
			configuration = "steps:\n  - label: \":pipeline:\"\n    command: \"buildkite-agent pipeline upload\""
		}

		data "buildkite_pipeline_template" "template_bar" {
			depends_on = [buildkite_pipeline_template.template_bar]
			name = "Template %s"
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
			name = "Template %s"
			configuration = "steps:\n  - label: \":pipeline:\"\n    command: \"buildkite-agent pipeline upload\""
		}

		data "buildkite_pipeline_template" "template_bar" {
			depends_on = [buildkite_pipeline_template.template_bar]
			id = buildkite_pipeline_template.template_bar.id
			name = "Template %s"
		}
		`, name, name)
	}

	t.Run("loads a pipeline template by id", func(t *testing.T) {
		randName := acctest.RandString(12)
		var ptr pipelineTemplateResourceModel

		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm the pipeline template exists in the buildkite API
			testAccCheckPipelineTemplateExists(&ptr, "buildkite_pipeline_template.template_foo"),
			// Confirm the pipeline template has the correct values in Buildkite's system
			testAccCheckPipelineTemplateRemoteValues(&ptr, fmt.Sprintf("Template %s", randName), false),
			// Check all pipeline template resource attributes are set in state (required attributes)
			resource.TestCheckResourceAttrSet("data.buildkite_pipeline_template.template_foo", "id"),
			resource.TestCheckResourceAttrSet("data.buildkite_pipeline_template.template_foo", "uuid"),
			resource.TestCheckResourceAttrSet("data.buildkite_pipeline_template.template_foo", "name"),
			resource.TestCheckResourceAttrSet("data.buildkite_pipeline_template.template_foo", "configuration"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineTemplateDestroy,
			Steps: []resource.TestStep{
				{
					Config: configId(randName),
					Check:  check,
				},
			},
		})
	})

	t.Run("loads a pipeline template by name", func(t *testing.T) {
		randName := acctest.RandString(12)
		var ptr pipelineTemplateResourceModel

		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm the pipeline template exists in the buildkite API
			testAccCheckPipelineTemplateExists(&ptr, "buildkite_pipeline_template.template_bar"),
			// Confirm the pipeline template has the correct values in Buildkite's system
			testAccCheckPipelineTemplateRemoteValues(&ptr, fmt.Sprintf("Template %s", randName), false),
			// Check all pipeline template resource attributes are set in state (required attributes)
			resource.TestCheckResourceAttrSet("data.buildkite_pipeline_template.template_bar", "id"),
			resource.TestCheckResourceAttrSet("data.buildkite_pipeline_template.template_bar", "uuid"),
			resource.TestCheckResourceAttrSet("data.buildkite_pipeline_template.template_bar", "name"),
			resource.TestCheckResourceAttrSet("data.buildkite_pipeline_template.template_bar", "configuration"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineTemplateDestroy,
			Steps: []resource.TestStep{
				{
					Config: configName(randName),
					Check:  check,
				},
			},
		})
	})

	t.Run("errors when unable to find a pipeline template by name", func(t *testing.T) {
		randName := acctest.RandString(12)
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
		randName := acctest.RandString(12)

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
