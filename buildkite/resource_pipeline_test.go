package buildkite

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// Confirm that we can create a new agent token, and then delete it without error
func TestAccPipeline_add_remove(t *testing.T) {
	var resourcePipeline PipelineNode

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckPipelineResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineConfigBasic("foo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Confirm the pipeline has the correct values in Buildkite's system
					testAccCheckPipelineRemoteValues(&resourcePipeline, "Test Pipeline foo"),
					// Confirm the pipeline has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "name", "Test Pipeline foo"),
				),
			},
		},
	})
}

func TestAccPipeline_add_remove_complex(t *testing.T) {
	var resourcePipeline PipelineNode
	steps := `"steps:\n- command: buildkite-agent pipeline upload\n"`

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckPipelineResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineConfigComplex("bar", steps),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Confirm the pipeline has the correct values in Buildkite's system
					testAccCheckPipelineRemoteValues(&resourcePipeline, "Test Pipeline bar"),
					// Confirm the pipeline has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "name", "Test Pipeline bar"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "repository", "https://github.com/buildkite/terraform-provider-buildkite.git"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "steps", "steps:\n- command: buildkite-agent pipeline upload\n"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "default_branch", "main"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "description", "A test pipeline produced via Terraform"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "branch_configuration", "main"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "skip_intermediate_builds", "true"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "skip_intermediate_builds_branch_filter", "main"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "cancel_intermediate_builds", "true"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "cancel_intermediate_builds_branch_filter", "!main"),
				),
			},
		},
	})
}

// Confirm that we can create a new pipeline, and then update the description
func TestAccPipeline_update(t *testing.T) {
	var resourcePipeline PipelineNode

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckPipelineResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineConfigBasic("foo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Quick check to confirm the local state is correct before we update it
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "name", "Test Pipeline foo"),
				),
			},
			{
				Config: testAccPipelineConfigBasic("bar"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Confirm the pipeline has the updated values in Buildkite's system
					testAccCheckPipelineRemoteValues(&resourcePipeline, "Test Pipeline bar"),
					// Confirm the pipeline has the updated values in terraform state
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "name", "Test Pipeline bar"),
				),
			},
		},
	})
}

// Confirm that this resource can be imported
func TestAccPipeline_import(t *testing.T) {
	var resourcePipeline PipelineNode

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckPipelineResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineConfigBasic("foo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Quick check to confirm the local state is correct before we re-import it
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "name", "Test Pipeline foo"),
				),
			},
			{
				// re-import the resource (using the graphql token of the existing resource) and confirm they match
				ResourceName:      "buildkite_pipeline.foobar",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccCheckPipelineExists(resourceName string, resourcePipeline *PipelineNode) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		resourceState, ok := s.RootModule().Resources[resourceName]

		if !ok {
			return fmt.Errorf("Not found in state: %s", resourceName)
		}

		if resourceState.Primary.ID == "" {
			return fmt.Errorf("No ID is set in state")
		}

		provider := testAccProvider.Meta().(*Client)
		var query struct {
			Node struct {
				Pipeline PipelineNode `graphql:"... on Pipeline"`
			} `graphql:"node(id: $id)"`
		}

		vars := map[string]interface{}{
			"id": resourceState.Primary.ID,
		}

		err := provider.graphql.Query(context.Background(), &query, vars)
		if err != nil {
			return fmt.Errorf("Error fetching pipeline from graphql API: %v", err)
		}

		if string(query.Node.Pipeline.ID) == "" {
			return fmt.Errorf("No pipeline found with graphql id: %s", resourceState.Primary.ID)
		}

		if string(query.Node.Pipeline.Slug) != resourceState.Primary.Attributes["slug"] {
			return fmt.Errorf("Pipeline slug in state doesn't match remote slug")
		}

		if string(query.Node.Pipeline.WebhookURL) != resourceState.Primary.Attributes["webhook_url"] {
			return fmt.Errorf("Pipeline webhook URL in state doesn't match remote webhook URL")
		}

		*resourcePipeline = query.Node.Pipeline

		return nil
	}
}

func testAccCheckPipelineRemoteValues(resourcePipeline *PipelineNode, name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {

		if string(resourcePipeline.Name) != name {
			return fmt.Errorf("remote pipeline name (%s) doesn't match expected value (%s)", resourcePipeline.Name, name)
		}
		return nil
	}
}

func testAccPipelineConfigBasic(name string) string {
	config := `
		resource "buildkite_pipeline" "foobar" {
			name = "Test Pipeline %s"
			repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
			steps = ""
		}
	`
	return fmt.Sprintf(config, name)
}

func testAccPipelineConfigComplex(name string, steps string) string {
	config := `
        resource "buildkite_pipeline" "foobar" {
            name = "Test Pipeline %s"
			repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
            steps = %s
            default_branch = "main"
            description = "A test pipeline produced via Terraform"
            branch_configuration = "main"
            skip_intermediate_builds = true
            skip_intermediate_builds_branch_filter = "main"
            cancel_intermediate_builds = true
            cancel_intermediate_builds_branch_filter = "!main"
        }
	`
	return fmt.Sprintf(config, name, steps)
}

// verifies the Pipeline has been destroyed
func testAccCheckPipelineResourceDestroy(s *terraform.State) error {
	// TODO manually check that all resources created during acceptance tests have been cleaned up
	return nil
}
