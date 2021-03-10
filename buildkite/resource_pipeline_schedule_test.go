package buildkite

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// Confirm that we can add a new pipeline schedule to a pipeline
func TestAccPipelineSchedule_add_remove(t *testing.T) {
	var resourcePipeline PipelineNode
	var resourceSchedule PipelineScheduleNode

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckAllPipelineScheduleResourcesDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineScheduleConfigBasic("foo", "0 * * * *"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Schedules need a pipeline
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Confirm the schedule exists in the buildkite API
					testAccCheckPipelineScheduleExists("buildkite_pipeline_schedule.foobar", &resourceSchedule),
					// Confirm the schedule has the correct values in Buildkite's system
					testAccCheckPipelineScheduleRemoteValues(&resourcePipeline, &resourceSchedule, "Test Schedule foo"),
					// Confirm the pipeline has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_pipeline_schedule.foobar", "label", "Test Schedule foo"),
					resource.TestCheckResourceAttr("buildkite_pipeline_schedule.foobar", "cronline", "0 * * * *"),
					resource.TestCheckResourceAttr("buildkite_pipeline_schedule.foobar", "branch", "main"),
				),
			},
		},
	})
}

// Confirm that we can add a new pipeline schedule to a pipeline when the pipeline uses teams
func TestAccPipelineSchedule_add_remove_withteams(t *testing.T) {
	var resourcePipeline PipelineNode
	var resourceSchedule PipelineScheduleNode

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckAllPipelineScheduleResourcesDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineScheduleConfigBasicWithTeam("foo", "0 * * * *"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Schedules need a pipeline
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Confirm the schedule exists in the buildkite API
					testAccCheckPipelineScheduleExists("buildkite_pipeline_schedule.foobar", &resourceSchedule),
					// Confirm the schedule has the correct values in Buildkite's system
					testAccCheckPipelineScheduleRemoteValues(&resourcePipeline, &resourceSchedule, "Test Schedule foo"),
					// Confirm the pipeline has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_pipeline_schedule.foobar", "label", "Test Schedule foo"),
					resource.TestCheckResourceAttr("buildkite_pipeline_schedule.foobar", "cronline", "0 * * * *"),
					resource.TestCheckResourceAttr("buildkite_pipeline_schedule.foobar", "branch", "main"),
				),
			},
		},
	})
}

// Confirm that we can create a new pipeline schedule, and then update the description
func TestAccPipelineSchedule_update(t *testing.T) {
	var resourcePipeline PipelineNode
	var resourceSchedule PipelineScheduleNode

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckAllPipelineScheduleResourcesDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineScheduleConfigBasic("foo", "0 * * * *"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the schedule exists in the buildkite API
					testAccCheckPipelineScheduleExists("buildkite_pipeline_schedule.foobar", &resourceSchedule),
					// Quick check to confirm the local state is correct before we update it
					resource.TestCheckResourceAttr("buildkite_pipeline_schedule.foobar", "cronline", "0 * * * *"),
					resource.TestCheckResourceAttr("buildkite_pipeline_schedule.foobar", "label", "Test Schedule foo"),
				),
			},
			{
				Config: testAccPipelineScheduleConfigBasic("bar", "0 1 * * *"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Schedules need a pipeline
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Confirm the schedule exists in the buildkite API
					testAccCheckPipelineScheduleExists("buildkite_pipeline_schedule.foobar", &resourceSchedule),
					// Confirm the schedule has the updated values in Buildkite's system
					testAccCheckPipelineScheduleRemoteValues(&resourcePipeline, &resourceSchedule, "Test Schedule bar"),
					// Confirm the schedule has the updated values in terraform state
					resource.TestCheckResourceAttr("buildkite_pipeline_schedule.foobar", "cronline", "0 1 * * *"),
					resource.TestCheckResourceAttr("buildkite_pipeline_schedule.foobar", "label", "Test Schedule bar"),
					resource.TestCheckResourceAttr("buildkite_pipeline_schedule.foobar", "branch", "main"),
				),
			},
		},
	})
}

// Confirm that we can create a new pipeline schedule, and then update the description - for pipelines that use teams
func TestAccPipelineSchedule_update_withteams(t *testing.T) {
	var resourcePipeline PipelineNode
	var resourceSchedule PipelineScheduleNode

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckAllPipelineScheduleResourcesDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineScheduleConfigBasicWithTeam("foo", "0 * * * *"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the schedule exists in the buildkite API
					testAccCheckPipelineScheduleExists("buildkite_pipeline_schedule.foobar", &resourceSchedule),
					// Quick check to confirm the local state is correct before we update it
					resource.TestCheckResourceAttr("buildkite_pipeline_schedule.foobar", "cronline", "0 * * * *"),
					resource.TestCheckResourceAttr("buildkite_pipeline_schedule.foobar", "label", "Test Schedule foo"),
				),
			},
			{
				Config: testAccPipelineScheduleConfigBasicWithTeam("bar", "0 1 * * *"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Schedules need a pipeline
					testAccCheckPipelineExists("buildkite_pipeline.foobar", &resourcePipeline),
					// Confirm the schedule exists in the buildkite API
					testAccCheckPipelineScheduleExists("buildkite_pipeline_schedule.foobar", &resourceSchedule),
					// Confirm the schedule has the updated values in Buildkite's system
					testAccCheckPipelineScheduleRemoteValues(&resourcePipeline, &resourceSchedule, "Test Schedule bar"),
					// Confirm the schedule has the updated values in terraform state
					resource.TestCheckResourceAttr("buildkite_pipeline_schedule.foobar", "cronline", "0 1 * * *"),
					resource.TestCheckResourceAttr("buildkite_pipeline_schedule.foobar", "label", "Test Schedule bar"),
					resource.TestCheckResourceAttr("buildkite_pipeline_schedule.foobar", "branch", "main"),
				),
			},
		},
	})
}

// Confirm that a schedule resource can be imported
func TestAccPipelineSchedule_import(t *testing.T) {
	var resourceSchedule PipelineScheduleNode

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckAllPipelineScheduleResourcesDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineScheduleConfigBasic("foo", "0 * * * *"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline schedule exists in the buildkite API
					testAccCheckPipelineScheduleExists("buildkite_pipeline_schedule.foobar", &resourceSchedule),
					// Quick check to confirm the local state is correct before we re-import it
					resource.TestCheckResourceAttr("buildkite_pipeline_schedule.foobar", "cronline", "0 * * * *"),
					resource.TestCheckResourceAttr("buildkite_pipeline_schedule.foobar", "label", "Test Schedule foo"),
				),
			},
			{
				// re-import the resource (using the graphql token of the existing resource) and confirm they match
				ResourceName:      "buildkite_pipeline_schedule.foobar",
				ImportStateIdFunc: testAccGetImportPipelineScheduleSlug(&resourceSchedule),
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccPipelineSchedule_disappears(t *testing.T) {
	var node PipelineScheduleNode
	resourceName := "buildkite_pipeline_schedule.foobar"

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckAllPipelineScheduleResourcesDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccPipelineScheduleConfigBasic("foo", "0 * * * *"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the pipeline schedule exists in the buildkite API
					testAccCheckPipelineScheduleExists(resourceName, &node),
					// Check that the schedule can be removed from the plan
					testAccCheckResourceDisappears(testAccProvider, resourcePipelineSchedule(), resourceName),
				),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func testAccCheckPipelineScheduleExists(resourceName string, resourceSchedule *PipelineScheduleNode) resource.TestCheckFunc {
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
				PipelineSchedule PipelineScheduleNode `graphql:"... on PipelineSchedule"`
			} `graphql:"node(id: $id)"`
		}

		vars := map[string]interface{}{
			"id": resourceState.Primary.ID,
		}

		err := provider.graphql.Query(context.Background(), &query, vars)
		if err != nil {
			return fmt.Errorf("Error fetching pipeline schedule from graphql API: %v", err)
		}

		if string(query.Node.PipelineSchedule.ID) == "" {
			return fmt.Errorf("No pipeline schedule found with graphql id: %s", resourceState.Primary.ID)
		}

		if string(query.Node.PipelineSchedule.Label) != resourceState.Primary.Attributes["label"] {
			return fmt.Errorf("Pipeline schedule label in state doesn't match remote label")
		}

		*resourceSchedule = query.Node.PipelineSchedule

		return nil
	}
}

func testAccCheckPipelineScheduleRemoteValues(resourcePipeline *PipelineNode, resourceSchedule *PipelineScheduleNode, label string) resource.TestCheckFunc {
	return func(s *terraform.State) error {

		if string(resourceSchedule.Pipeline.ID) != string(resourcePipeline.ID) {
			return fmt.Errorf("remote pipeline schedule pipeline ID (%s) doesn't match expected value (%s)", resourceSchedule.Pipeline.ID, resourcePipeline.ID)
		}

		if string(resourceSchedule.Label) != label {
			return fmt.Errorf("remote pipeline schedule label (%s) doesn't match expected value (%s)", resourceSchedule.Label, label)
		}

		return nil
	}
}

func testAccGetImportPipelineScheduleSlug(resourceSchedule *PipelineScheduleNode) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		slug := fmt.Sprintf("%s/%s/%s", "buildkite-terraform-provider-test-org", "test-pipeline-foo", resourceSchedule.UUID)
		return slug, nil
	}
}

func testAccPipelineScheduleConfigBasic(label string, cronline string) string {
	config := `
        resource "buildkite_pipeline" "foobar" {
            name = "Test Pipeline %s"
		    repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
			steps = ""
        }

		resource "buildkite_pipeline_schedule" "foobar" {
            pipeline_id = buildkite_pipeline.foobar.id
            branch = "main"
            cronline = "%s"
			label = "Test Schedule %s"
		}
	`
	return fmt.Sprintf(config, label, cronline, label)
}

func testAccPipelineScheduleConfigBasicWithTeam(label string, cronline string) string {
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

		resource "buildkite_pipeline_schedule" "foobar" {
            pipeline_id = buildkite_pipeline.foobar.id
            branch = "main"
            cronline = "%s"
			label = "Test Schedule %s"
		}
	`
	return fmt.Sprintf(config, label, cronline, label)
}

// verifies the pipeline schedule has been destroyed
func testAccCheckPipelineScheduleResourceDestroy(s *terraform.State) error {
	provider := testAccProvider.Meta().(*Client)

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_pipeline_schedule" {
			continue
		}

		// Try to find the resource remotely
		var query struct {
			Node struct {
				PipelineSchedule PipelineScheduleNode `graphql:"... on PipelineSchedule"`
			} `graphql:"node(id: $id)"`
		}

		vars := map[string]interface{}{
			"id": rs.Primary.ID,
		}

		err := provider.graphql.Query(context.Background(), &query, vars)
		if err == nil {
			if string(query.Node.PipelineSchedule.ID) != "" &&
				string(query.Node.PipelineSchedule.ID) == rs.Primary.ID {
				return fmt.Errorf("Schedule still exists")
			}
		}

		return err
	}

	return nil
}

func testAccCheckAllPipelineScheduleResourcesDestroyed(s *terraform.State) error {
	var err error

	// Should destroy the schedule
	err = testAccCheckPipelineScheduleResourceDestroy(s)
	if err != nil {
		return err
	}

	// Should destroy the test pipeline
	err = testAccCheckPipelineResourceDestroy(s)
	if err != nil {
		return err
	}

	return nil
}
