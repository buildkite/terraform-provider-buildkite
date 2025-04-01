package buildkite

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccBuildkitePipelineTemplateResource(t *testing.T) {
	t.Cleanup(func() {
		CleanupResources(t)
	})
	configRequired := func(name string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "10s"
				read = "10s"
				update = "10s"
				delete = "10s"
			}
		}

		resource "buildkite_pipeline_template" "template_foo" {
			name = "Template %s"
			configuration = "steps:\n  - label: \":pipeline:\"\n    command: \"buildkite-agent pipeline upload\""
		}
		`, name)
	}

	configAll := func(name string, available bool) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "10s"
				read = "10s"
				update = "10s"
				delete = "10s"
			}
		}

		resource "buildkite_pipeline_template" "template_bar" {
			name = "Template %s"
			configuration = "steps:\n  - label: \":pipeline:\"\n    command: \"buildkite-agent pipeline upload\""
			description = "Pipeline upload template"
			available = %v
		}
		`, name, available)
	}

	t.Run("creates a pipeline template with required attributes", func(t *testing.T) {
		randName := acctest.RandString(12)
		var ptr pipelineTemplateResourceModel

		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm the pipeline template exists in the buildkite API
			testAccCheckPipelineTemplateExists(&ptr, "buildkite_pipeline_template.template_foo"),
			// Confirm the pipeline template has the correct values in Buildkite's system
			testAccCheckPipelineTemplateRemoteValues(&ptr, fmt.Sprintf("Template %s", randName), false),
			// Check all pipeline template resource attributes are set in state (required attributes)
			resource.TestCheckResourceAttrSet("buildkite_pipeline_template.template_foo", "id"),
			resource.TestCheckResourceAttrSet("buildkite_pipeline_template.template_foo", "uuid"),
			resource.TestCheckResourceAttrSet("buildkite_pipeline_template.template_foo", "name"),
			resource.TestCheckResourceAttrSet("buildkite_pipeline_template.template_foo", "configuration"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineTemplateDestroy,
			Steps: []resource.TestStep{
				{
					Config: configRequired(randName),
					Check:  check,
				},
			},
		})
	})

	t.Run("creates a pipeline template with all attributes", func(t *testing.T) {
		randName := acctest.RandString(12)
		var ptr pipelineTemplateResourceModel

		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm the pipeline template exists in the buildkite API
			testAccCheckPipelineTemplateExists(&ptr, "buildkite_pipeline_template.template_bar"),
			// Confirm the pipeline template has the correct values in Buildkite's system
			testAccCheckPipelineTemplateRemoteValues(&ptr, fmt.Sprintf("Template %s", randName), true),
			// Check all pipeline template resource attributes are set in state (all attributes)
			resource.TestCheckResourceAttrSet("buildkite_pipeline_template.template_bar", "id"),
			resource.TestCheckResourceAttrSet("buildkite_pipeline_template.template_bar", "uuid"),
			resource.TestCheckResourceAttrSet("buildkite_pipeline_template.template_bar", "name"),
			resource.TestCheckResourceAttrSet("buildkite_pipeline_template.template_bar", "configuration"),
			resource.TestCheckResourceAttrSet("buildkite_pipeline_template.template_bar", "description"),
			resource.TestCheckResourceAttrSet("buildkite_pipeline_template.template_bar", "available"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineTemplateDestroy,
			Steps: []resource.TestStep{
				{
					Config: configAll(randName, true),
					Check:  check,
				},
			},
		})
	})

	t.Run("updates a pipeline template", func(t *testing.T) {
		randName := acctest.RandString(12)
		var ptr pipelineTemplateResourceModel

		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm the pipeline template exists in the buildkite API
			testAccCheckPipelineTemplateExists(&ptr, "buildkite_pipeline_template.template_bar"),
			// Confirm the pipeline template has the correct values in Buildkite's system
			testAccCheckPipelineTemplateRemoteValues(&ptr, fmt.Sprintf("Template %s", randName), false),
			// Check all pipeline template resource attributes are set in state (all attributes)
			resource.TestCheckResourceAttrSet("buildkite_pipeline_template.template_bar", "id"),
			resource.TestCheckResourceAttrSet("buildkite_pipeline_template.template_bar", "uuid"),
			resource.TestCheckResourceAttrSet("buildkite_pipeline_template.template_bar", "name"),
			resource.TestCheckResourceAttrSet("buildkite_pipeline_template.template_bar", "configuration"),
			resource.TestCheckResourceAttrSet("buildkite_pipeline_template.template_bar", "description"),
			resource.TestCheckResourceAttrSet("buildkite_pipeline_template.template_bar", "available"),
		)

		ckecUpdated := resource.ComposeAggregateTestCheckFunc(
			// Confirm the pipeline template exists in the buildkite API (available changed)
			testAccCheckPipelineTemplateExists(&ptr, "buildkite_pipeline_template.template_bar"),
			// Confirm the pipeline template has the correct values in Buildkite's system
			testAccCheckPipelineTemplateRemoteValues(&ptr, fmt.Sprintf("Template %s", randName), true),
			// Check all pipeline template resource attributes are set in state (all attributes)
			resource.TestCheckResourceAttrSet("buildkite_pipeline_template.template_bar", "available"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineTemplateDestroy,
			Steps: []resource.TestStep{
				{
					Config: configAll(randName, false),
					Check:  check,
				},
				{
					Config: configAll(randName, true),
					Check:  ckecUpdated,
				},
			},
		})
	})

	t.Run("imports a pipeline template", func(t *testing.T) {
		randName := acctest.RandString(12)
		var ptr pipelineTemplateResourceModel

		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm the pipeline template exists in the buildkite API
			testAccCheckPipelineTemplateExists(&ptr, "buildkite_pipeline_template.template_bar"),
			// Confirm the pipeline template has the correct values in Buildkite's system
			testAccCheckPipelineTemplateRemoteValues(&ptr, fmt.Sprintf("Template %s", randName), false),
			// Check all pipeline template resource attributes are set in state (all attributes)
			resource.TestCheckResourceAttrSet("buildkite_pipeline_template.template_bar", "id"),
			resource.TestCheckResourceAttrSet("buildkite_pipeline_template.template_bar", "uuid"),
			resource.TestCheckResourceAttrSet("buildkite_pipeline_template.template_bar", "name"),
			resource.TestCheckResourceAttrSet("buildkite_pipeline_template.template_bar", "configuration"),
			resource.TestCheckResourceAttrSet("buildkite_pipeline_template.template_bar", "description"),
			resource.TestCheckResourceAttrSet("buildkite_pipeline_template.template_bar", "available"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineTemplateDestroy,
			Steps: []resource.TestStep{
				{
					Config: configAll(randName, false),
					Check:  check,
				},
				{
					ResourceName:      "buildkite_pipeline_template.template_bar",
					ImportState:       true,
					ImportStateVerify: true,
				},
			},
		})
	})
}

func testAccCheckPipelineTemplateExists(ptr *pipelineTemplateResourceModel, name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]

		if !ok {
			return fmt.Errorf("Not found in state: %s", name)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("No ID is set in state")
		}

		TrackResource("buildkite_pipeline_template", rs.Primary.ID)

		r, err := getNode(context.Background(), genqlientGraphql, rs.Primary.ID)
		if err != nil {
			return err
		}

		if pipelineTemplateNode, ok := r.GetNode().(*getNodeNodePipelineTemplate); ok {
			if pipelineTemplateNode == nil {
				return fmt.Errorf("Pipeline template not found: nil response")
			}
			updatePipelineTemplateResourceState(ptr, *pipelineTemplateNode)
		}

		return nil
	}
}

func testAccCheckPipelineTemplateRemoteValues(ptr *pipelineTemplateResourceModel, name string, available bool) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if ptr.Name.ValueString() != name {
			return fmt.Errorf("Remote pipeline template name (%s) doesn't match expected value (%s)", ptr.Name, name)
		}

		if ptr.Available.ValueBool() != available {
			return fmt.Errorf("Remote pipeline template available (%v) doesn't match expected value (%v)", ptr.Available, available)
		}

		return nil
	}
}

func testAccCheckPipelineTemplateDestroy(s *terraform.State) error {
	return testAccCheckPipelineTemplateDestroyFunc(s)
}
