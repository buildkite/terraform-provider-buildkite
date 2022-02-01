package buildkite

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// Confirm that we can create a new pipeline, and then delete it without error
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
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.trigger_mode", "code"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.build_branches", "false"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.build_pull_request_forks", "true"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.build_pull_request_ready_for_review", "true"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.build_pull_request_labels_changed", "true"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.build_pull_requests", "false"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.build_tags", "true"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.cancel_deleted_branch_builds", "true"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.filter_enabled", "true"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.filter_condition", "build.pull_request.labels includes \"CI: yes\""),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.prefix_pull_request_fork_branch_names", "true"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.publish_blocked_as_pending", "true"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.publish_commit_status", "true"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.publish_commit_status_per_step", "true"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.pull_request_branch_filter_configuration", "features/*"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.pull_request_branch_filter_enabled", "true"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.separate_pull_request_statuses", "true"),
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "provider_settings.0.skip_pull_request_builds_for_existing_commits", "false"),
					resource.TestMatchResourceAttr("buildkite_pipeline.foobar", "webhook_url", regexp.MustCompile(`^https://webhook.buildkite.com/deliver/[a-z0-9]{50}$`)),
					resource.TestMatchResourceAttr("buildkite_pipeline.foobar", "badge_url", regexp.MustCompile(`^https://badge.buildkite.com/[a-z0-9]{50}\.svg$`)),
				),
			},
		},
	})
}

func TestAccPipeline_add_remove_withteams(t *testing.T) {
	var resourcePipeline PipelineNode

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckPipelineResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineConfigBasicWithTeam("foo"),
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

// Confirm that we can archive a pipeline
func TestAccPipeline_archive(t *testing.T) {
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
					// Confirm the pipeline is not archived in terraform state
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "archived", "false"),
					// Confirm the pipeline is not archived in buildkite API
					testAccCheckPipelineRemoteArchived(&resourcePipeline, false),
				),
			},
			{
				Config: testAccPipelineConfigArchive("foo", true),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Confirm the pipeline is archived in terraform state
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "archived", "true"),
					// Confirm the pipeline is archived in the buildktie API
					testAccCheckPipelineRemoteArchived(&resourcePipeline, true),
				),
			},
			{
				Config: testAccPipelineConfigBasic("bar"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Confirm the pipeline is not archived in terraform state
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "archived", "false"),
					// Confirm changing other attributes than archived at once works (by first unarchiving it)
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "name", "Test Pipeline bar"),
					// Confirm the pipeline is not archived in buildkite API
					testAccCheckPipelineRemoteArchived(&resourcePipeline, false),
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

func TestAccPipeline_update_withteams(t *testing.T) {
	var resourcePipeline PipelineNode

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckPipelineResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineConfigBasicWithTeam("foo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Quick check to confirm the local state is correct before we update it
					resource.TestCheckResourceAttr("buildkite_pipeline.foobar", "name", "Test Pipeline foo"),
				),
			},
			{
				Config: testAccPipelineConfigBasicWithTeam("bar"),
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

// Confirm that this resource can be removed
func TestAccPipeline_disappears(t *testing.T) {
	var node PipelineNode
	resourceName := "buildkite_pipeline.foobar"

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckPipelineResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineConfigBasic("foo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline exists in the buildkite API
					testAccCheckPipelineExists(resourceName, &node),
					// Ensure its removal from the spec
					testAccCheckResourceDisappears(testAccProvider, resourcePipeline(), resourceName),
				),
				ExpectNonEmptyPlan: true,
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

		if query.Node.Pipeline.ID.(string) == "" {
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

func testAccCheckPipelineRemoteArchived(resourcePipeline *PipelineNode, archived bool) resource.TestCheckFunc {
	return func(s *terraform.State) error {

		if bool(resourcePipeline.Archived) != archived {
			return fmt.Errorf("remote pipeline archived state (%t) doesn't match expected value (%t)", resourcePipeline.Archived, archived)
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

func testAccPipelineConfigArchive(name string, archived bool) string {
	config := `
		resource "buildkite_pipeline" "foobar" {
			name = "Test Pipeline %s"
			repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
			steps = ""
			archived = %t
		}
	`
	return fmt.Sprintf(config, name, archived)
}

func testAccPipelineConfigBasicWithTeam(name string) string {
	config := `
		resource "buildkite_pipeline" "foobar" {
			name = "Test Pipeline %s"
			repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
			steps = ""

			team {
				slug = "everyone"
				access_level = "MANAGE_BUILD_AND_READ"
			}
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
			provider_settings {
				trigger_mode = "code"
				build_branches = false
				build_pull_request_forks = true
				build_pull_request_ready_for_review = true
				build_pull_request_labels_changed = true
				build_pull_requests = false
				build_tags = true
				cancel_deleted_branch_builds = true
				filter_enabled = true
				filter_condition = "build.pull_request.labels includes \"CI: yes\""
				prefix_pull_request_fork_branch_names = true
				publish_blocked_as_pending = true
				publish_commit_status = true
				publish_commit_status_per_step = true
				pull_request_branch_filter_configuration = "features/*"
				pull_request_branch_filter_enabled = true
				separate_pull_request_statuses = true
				skip_pull_request_builds_for_existing_commits = false
			}
        }
	`
	return fmt.Sprintf(config, name, steps)
}

// verifies the Pipeline has been destroyed
func testAccCheckPipelineResourceDestroy(s *terraform.State) error {
	provider := testAccProvider.Meta().(*Client)

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_pipeline" {
			continue
		}

		// Try to find the resource remotely
		var query struct {
			Node struct {
				Pipeline PipelineNode `graphql:"... on Pipeline"`
			} `graphql:"node(id: $id)"`
		}

		vars := map[string]interface{}{
			"id": rs.Primary.ID,
		}

		err := provider.graphql.Query(context.Background(), &query, vars)
		if err == nil {
			if query.Node.Pipeline.ID.(string) != "" &&
				query.Node.Pipeline.ID.(string) == rs.Primary.ID {
				return fmt.Errorf("Pipeline still exists")
			}
		}

		return err
	}

	return nil
}
