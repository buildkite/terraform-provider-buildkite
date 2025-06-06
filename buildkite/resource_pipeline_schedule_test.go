package buildkite

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccBuildkitePipelineSchedule(t *testing.T) {
	config := func(name, cronline, label, env string, enabled bool) string {
		return fmt.Sprintf(`
			provider "buildkite" {
				timeouts = {
					create = "10s"
					read = "10s"
					update = "10s"
					delete = "10s"
				}
			}

			resource "buildkite_pipeline" "pipeline" {
				name = "%s"
				repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
			}

			resource "buildkite_pipeline_schedule" "pipeline" {
				pipeline_id = buildkite_pipeline.pipeline.id
				branch = "main"
				cronline = "%s"
				label = "%s"
				env = {
					%s
				}
				enabled = %v
			}
		`, name, cronline, label, env, enabled)
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
			CheckDestroy:             testAccCheckPipelineScheduleDestroy,
			Steps: []resource.TestStep{
				{
					Config: config(pipelineName, cronline, label, "FOO = \"BAR=2f\"", true),
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
						resource.TestCheckResourceAttr("buildkite_pipeline_schedule.pipeline", "env.FOO", "BAR=2f"),
						resource.TestCheckResourceAttr("buildkite_pipeline_schedule.pipeline", "enabled", "true"),
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
			CheckDestroy:             testAccCheckPipelineScheduleDestroy,
			Steps: []resource.TestStep{
				{
					Config: config(pipelineName, "0 * * * *", label, "FOO = \"bar\"", true),
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
						resource.TestCheckResourceAttr("buildkite_pipeline_schedule.pipeline", "env.FOO", "bar"),
						resource.TestCheckResourceAttr("buildkite_pipeline_schedule.pipeline", "enabled", "true"),
					),
				},
				{
					Config: config(pipelineName, "0 1 * * *", label, "FOO = \"bar\"", false),
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
						resource.TestCheckResourceAttr("buildkite_pipeline_schedule.pipeline", "env.FOO", "bar"),
						resource.TestCheckResourceAttr("buildkite_pipeline_schedule.pipeline", "enabled", "false"),
					),
				},
			},
		})
	})

	t.Run("pipeline schedule is recreated if removed", func(t *testing.T) {
		pipelineName := acctest.RandString(12)
		label := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineScheduleDestroy,
			Steps: []resource.TestStep{
				{
					Config: config(pipelineName, "0 * * * *", label, "", true),
					Check: func(s *terraform.State) error {
						ps := s.RootModule().Resources["buildkite_pipeline_schedule.pipeline"]
						_, err := deletePipelineSchedule(context.Background(),
							genqlientGraphql,
							ps.Primary.ID)
						return err
					},
					ExpectNonEmptyPlan: true,
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction("buildkite_pipeline_schedule.pipeline", plancheck.ResourceActionCreate),
						},
					},
				},
			},
		})
	})
}

// Testcase destroyer function
func testAccCheckPipelineScheduleDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_pipeline_schedule" {
			continue
		}

	}
	return nil
}
