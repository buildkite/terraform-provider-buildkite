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

func TestAccBuildkitePipelineWebhook(t *testing.T) {
	configBasic := func(name string) string {
		return fmt.Sprintf(`
			provider "buildkite" {
				timeouts = {
					create = "60s"
					read = "60s"
					update = "60s"
					delete = "60s"
				}
			}

			resource "buildkite_pipeline" "pipeline" {
				name = "%s"
				repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
			}

			resource "buildkite_pipeline_webhook" "webhook" {
				pipeline_id = buildkite_pipeline.pipeline.id
			}
		`, name)
	}

	configPipelineOnly := func(name string) string {
		return fmt.Sprintf(`
			provider "buildkite" {
				timeouts = {
					create = "60s"
					read = "60s"
					update = "60s"
					delete = "60s"
				}
			}

			resource "buildkite_pipeline" "pipeline" {
				name = "%s"
				repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
			}
		`, name)
	}

	t.Run("pipeline webhook can be created and imported", func(t *testing.T) {
		pipelineName := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineWebhookDestroy,
			Steps: []resource.TestStep{
				{
					Config: configBasic(pipelineName),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttrSet("buildkite_pipeline_webhook.webhook", "id"),
						resource.TestCheckResourceAttrSet("buildkite_pipeline_webhook.webhook", "provider_name"),
						resource.TestCheckResourceAttrSet("buildkite_pipeline_webhook.webhook", "repository_url"),
						resource.TestCheckResourceAttrSet("buildkite_pipeline_webhook.webhook", "webhook_url"),
						resource.TestCheckResourceAttrPair(
							"buildkite_pipeline_webhook.webhook", "pipeline_id",
							"buildkite_pipeline.pipeline", "id",
						),
					),
				},
				{
					ResourceName:      "buildkite_pipeline_webhook.webhook",
					ImportState:       true,
					ImportStateIdFunc: getPipelineIdForImport("buildkite_pipeline.pipeline"),
					ImportStateVerify: true,
				},
			},
		})
	})

	t.Run("pipeline webhook is recreated if removed externally", func(t *testing.T) {
		pipelineName := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineWebhookDestroy,
			Steps: []resource.TestStep{
				{
					Config: configBasic(pipelineName),
					Check: func(s *terraform.State) error {
						pipelineRes := s.RootModule().Resources["buildkite_pipeline.pipeline"]
						_, err := deletePipelineWebhook(context.Background(),
							genqlientGraphql,
							pipelineRes.Primary.ID)
						return err
					},
					ExpectNonEmptyPlan: true,
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction("buildkite_pipeline_webhook.webhook", plancheck.ResourceActionCreate),
						},
					},
				},
			},
		})
	})

	t.Run("pipeline webhook is deleted when resource is removed", func(t *testing.T) {
		pipelineName := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineWebhookDestroy,
			Steps: []resource.TestStep{
				{
					Config: configBasic(pipelineName),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttrSet("buildkite_pipeline_webhook.webhook", "id"),
					),
				},
				{
					Config: configPipelineOnly(pipelineName),
					Check: func(s *terraform.State) error {
						pipelineRes := s.RootModule().Resources["buildkite_pipeline.pipeline"]
						resp, err := getPipelineWebhook(context.Background(), genqlientGraphql, pipelineRes.Primary.ID)
						if err != nil {
							return err
						}
						if pipeline, ok := resp.GetNode().(*getPipelineWebhookNodePipeline); ok && pipeline != nil {
							if pipeline.RepositoryWebhook.Id != "" {
								return fmt.Errorf("webhook still exists after resource removal")
							}
						}
						return nil
					},
				},
			},
		})
	})
}

func getPipelineIdForImport(resourceName string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		res := s.RootModule().Resources[resourceName]
		if res == nil {
			return "", fmt.Errorf("resource %s not found", resourceName)
		}
		return res.Primary.ID, nil
	}
}

func testAccCheckPipelineWebhookDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_pipeline_webhook" {
			continue
		}

		resp, err := getPipelineWebhook(context.Background(), genqlientGraphql, rs.Primary.Attributes["pipeline_id"])
		if err != nil {
			return err
		}
		if pipeline, ok := resp.GetNode().(*getPipelineWebhookNodePipeline); ok && pipeline != nil {
			if pipeline.RepositoryWebhook.Id != "" {
				return fmt.Errorf("pipeline webhook still exists")
			}
		}
	}
	return nil
}
