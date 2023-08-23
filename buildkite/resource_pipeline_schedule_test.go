package buildkite

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccBuildkitePipelineSchedule(t *testing.T) {
	config := func(name, cronline, label string) string {
		return fmt.Sprintf(`
			resource "buildkite_pipeline" "pipeline" {
				name = "%s"
				repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
			}

			resource "buildkite_pipeline_schedule" "pipeline" {
				pipeline_id = buildkite_pipeline.pipeline.id
				branch = "main"
				cronline = "%s"
				label = "%s"
			}
		`, name, cronline, label)
	}

	loadPipeline := func(name string, pipeline *getPipelinePipeline) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			slug := fmt.Sprintf("%s/%s", getenv("BUILDKITE_ORGANIZATION_SLUG"), name)
			resp, err := getPipeline(context.Background(), genqlientGraphql, slug)
			*pipeline = resp.Pipeline
			return err
		}
	}
	loadPipelineSchedule := func(schedule *PipelineScheduleValues) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			scheduleRes := s.RootModule().Resources["buildkite_pipeline_schedule.pipeline"]
			resp, err := getPipelineSchedule(context.Background(), genqlientGraphql, scheduleRes.Primary.ID)
			if err != nil {
				return err
			}
			if node, ok := resp.Node.(*getPipelineScheduleNodePipelineSchedule); ok {
				*schedule = node.PipelineScheduleValues
				return nil
			}
			return fmt.Errorf("pipeline schedule node is invalid")
		}
	}

	t.Run("pipeline schedule can be created", func(t *testing.T) {
		var pipeline getPipelinePipeline
		var schedule PipelineScheduleValues
		pipelineName := acctest.RandString(12)
		label := acctest.RandString(12)
		cronline := "0 * * * *"

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy: func(s *terraform.State) error {
				resp, err := getPipeline(context.Background(), genqlientGraphql, fmt.Sprintf("%s/%s", getenv("BUILDKITE_ORGANIZATION_SLUG"), pipelineName))
				if resp.Pipeline.Name == pipelineName {
					return fmt.Errorf("Pipeline still exists: %s", pipelineName)
				}
				return err
			},
			Steps: []resource.TestStep{
				{
					Config: config(pipelineName, cronline, label),
					Check: resource.ComposeAggregateTestCheckFunc(
						// Schedules need a pipeline
						loadPipeline(pipelineName, &pipeline),
						// Confirm the schedule exists in the buildkite API
						loadPipelineSchedule(&schedule),
						// Confirm the schedule has the correct values in Buildkite's system
						func(s *terraform.State) error {
							if pipeline.Id != schedule.Pipeline.Id {
								return fmt.Errorf("remote pipeline schedule pipeline ID (%s) doesn't match expected value (%s)", schedule.Pipeline.Id, pipeline.Id)
							}
							l := *schedule.Label
							if l != label {
								return fmt.Errorf("remote pipeline schedule label (%s) doesn't match expected value (%s)", l, label)
							}
							if *schedule.Cronline != cronline {
								return fmt.Errorf("remote pipeline schedule cronline (%s) doesn't match expected value (%s)", *schedule.Cronline, cronline)
							}
							return nil
						},
						// Confirm the pipeline has the correct values in terraform state
						resource.TestCheckResourceAttr("buildkite_pipeline_schedule.pipeline", "label", label),
						resource.TestCheckResourceAttr("buildkite_pipeline_schedule.pipeline", "cronline", cronline),
						resource.TestCheckResourceAttr("buildkite_pipeline_schedule.pipeline", "branch", "main"),
					),
				},
				{
					ResourceName:      "buildkite_pipeline_schedule.pipeline",
					ImportState:       true,
					ImportStateId:     schedule.Id,
					ImportStateVerify: true,
				},
			},
		})
	})

	t.Run("pipeline schedule can be updated", func(t *testing.T) {
		var pipeline getPipelinePipeline
		var schedule PipelineScheduleValues
		pipelineName := acctest.RandString(12)
		label := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: config(pipelineName, "0 * * * *", label),
					Check: resource.ComposeAggregateTestCheckFunc(
						// Schedules need a pipeline
						loadPipeline(pipelineName, &pipeline),
						// Confirm the schedule exists in the buildkite API
						loadPipelineSchedule(&schedule),
						// Confirm the schedule has the correct values in Buildkite's system
						func(s *terraform.State) error {
							if pipeline.Id != schedule.Pipeline.Id {
								return fmt.Errorf("remote pipeline schedule pipeline ID (%s) doesn't match expected value (%s)", schedule.Pipeline.Id, pipeline.Id)
							}
							if *schedule.Label != label {
								return fmt.Errorf("remote pipeline schedule label (%s) doesn't match expected value (%s)", *schedule.Label, label)
							}
							if *schedule.Cronline != "0 * * * *" {
								return fmt.Errorf("remote pipeline schedule cronline (%s) doesn't match expected value (%s)", *schedule.Cronline, "0 * * * *")
							}
							return nil
						},
						// Confirm the pipeline has the correct values in terraform state
						resource.TestCheckResourceAttr("buildkite_pipeline_schedule.pipeline", "label", label),
						resource.TestCheckResourceAttr("buildkite_pipeline_schedule.pipeline", "cronline", "0 * * * *"),
						resource.TestCheckResourceAttr("buildkite_pipeline_schedule.pipeline", "branch", "main"),
					),
				},
				{
					Config: config(pipelineName, "0 1 * * *", label),
					Check: resource.ComposeAggregateTestCheckFunc(
						// Confirm the schedule exists in the buildkite API
						loadPipelineSchedule(&schedule),
						// Confirm the schedule has the correct values in Buildkite's system
						func(s *terraform.State) error {
							if pipeline.Id != schedule.Pipeline.Id {
								return fmt.Errorf("remote pipeline schedule pipeline ID (%s) doesn't match expected value (%s)", schedule.Pipeline.Id, pipeline.Id)
							}
							if *schedule.Label != label {
								return fmt.Errorf("remote pipeline schedule label (%s) doesn't match expected value (%s)", *schedule.Label, label)
							}
							if *schedule.Cronline != "0 1 * * *" {
								return fmt.Errorf("remote pipeline schedule cronline (%s) doesn't match expected value (%s)", *schedule.Cronline, "0 1 * * *")
							}
							return nil
						},
						// Confirm the pipeline has the correct values in terraform state
						resource.TestCheckResourceAttr("buildkite_pipeline_schedule.pipeline", "label", label),
						resource.TestCheckResourceAttr("buildkite_pipeline_schedule.pipeline", "cronline", "0 1 * * *"),
						resource.TestCheckResourceAttr("buildkite_pipeline_schedule.pipeline", "branch", "main"),
					),
				},
			},
		})
	})
}
