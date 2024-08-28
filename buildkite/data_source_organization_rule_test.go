package buildkite

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccBuildkiteOrganizationRuleDatasource(t *testing.T) {

	configTriggerBuildDatasource := func(fields ...string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "10s"
				read = "10s"
				update = "10s"
				delete = "10s"
			}
		}

		resource "buildkite_cluster" "cluster_one" {
			name        = "Cluster %s"
			description = "A test cluster containing a triggering pipeline."
		}

		resource "buildkite_cluster" "cluster_two" {
			name        = "Cluster %s"
			description = "A test cluster containing a to-be-triggered pipeline."
		}

		resource "buildkite_pipeline" "pipeline_triggerer" {
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id			 = buildkite_cluster.cluster_one.id
		}

		resource "buildkite_pipeline" "pipeline_triggered" {
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id           = buildkite_cluster.cluster_two.id
		}	

		resource "buildkite_organization_rule" "pipeline_trigger_build_rule" {
			name = "pipeline.trigger_build.pipeline"
			value = jsonencode({
				triggering_pipeline_uuid = buildkite_pipeline.pipeline_triggerer.uuid
				triggered_pipeline_uuid = buildkite_pipeline.pipeline_triggered.uuid
			})
		}

		data "buildkite_organization_rule" "pipeline_trigger_build_rule" {
			depends_on = [buildkite_organization_rule.pipeline_trigger_build_rule]
			id = buildkite_organization_rule.pipeline_trigger_build_rule.id
		}

		`, fields[0], fields[1], fields[0], fields[1])
	}

	configArtifactsReadDatasource := func(fields ...string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "10s"
				read = "10s"
				update = "10s"
				delete = "10s"
			}
		}

		resource "buildkite_cluster" "cluster_source" {
			name        = "Cluster %s"
			description = "A test cluster containing a source pipeline."
		}

		resource "buildkite_cluster" "cluster_target" {
			name        = "Cluster %s"
			description = "A test cluster containing a target pipelnie for artifact readiing."
		}

		resource "buildkite_pipeline" "pipeline_source" {
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id			 = buildkite_cluster.cluster_source.id
		}

		resource "buildkite_pipeline" "pipeline_target" {
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id           = buildkite_cluster.cluster_target.id
		}	

		resource "buildkite_organization_rule" "artifacts_read_rule" {
			name = "pipeline.artifacts_read.pipeline"
			value = jsonencode({
				target_pipeline_uuid = buildkite_pipeline.pipeline_target.uuid
				source_pipeline_uuid = buildkite_pipeline.pipeline_source.uuid
			})
		}

		data "buildkite_organization_rule" "artifacts_read_rule" {
			depends_on = [buildkite_organization_rule.artifacts_read_rule]
			id = buildkite_organization_rule.artifacts_read_rule.id
		}

		`, fields[0], fields[1], fields[0], fields[1])
	}

	t.Run("loads a pipeline.trigger_build.pipeline organization rule", func(t *testing.T) {
		randdNameOne := acctest.RandString(12)
		randNameTwo := acctest.RandString(12)
		var orr organizationRuleResourceModel

		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm the organization rule exists
			testAccCheckOrganizationRuleExists(&orr, "buildkite_organization_rule.pipeline_trigger_build_rule"),
			// Confirm the organization rule has the correct values in Buildkite's system
			testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", "TRIGGER_BUILD", "ALLOW"),
			// Check the organization rule resource's attributes are set in state
			resource.TestCheckResourceAttrSet("data.buildkite_organization_rule.pipeline_trigger_build_rule", "id"),
			resource.TestCheckResourceAttrSet("data.buildkite_organization_rule.pipeline_trigger_build_rule", "uuid"),
			resource.TestCheckResourceAttrSet("data.buildkite_organization_rule.pipeline_trigger_build_rule", "name"),
			resource.TestCheckResourceAttrSet("data.buildkite_organization_rule.pipeline_trigger_build_rule", "value"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleeDestroy,
			Steps: []resource.TestStep{
				{
					Config: configTriggerBuildDatasource(randdNameOne, randNameTwo),
					Check:  check,
				},
			},
		})
	})

	t.Run("loads a pipeline.artifacts_read.pipeline organization rule", func(t *testing.T) {
		randdNameOne := acctest.RandString(12)
		randNameTwo := acctest.RandString(12)
		var orr organizationRuleResourceModel

		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm the organization rule exists
			testAccCheckOrganizationRuleExists(&orr, "buildkite_organization_rule.artifacts_read_rule"),
			// Confirm the organization rule has the correct values in Buildkite's system
			testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", "ARTIFACTS_READ", "ALLOW"),
			// Check the organization rule resource's attributes are set in state
			resource.TestCheckResourceAttrSet("data.buildkite_organization_rule.artifacts_read_rule", "id"),
			resource.TestCheckResourceAttrSet("data.buildkite_organization_rule.artifacts_read_rule", "uuid"),
			resource.TestCheckResourceAttrSet("data.buildkite_organization_rule.artifacts_read_rule", "name"),
			resource.TestCheckResourceAttrSet("data.buildkite_organization_rule.artifacts_read_rule", "value"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleeDestroy,
			Steps: []resource.TestStep{
				{
					Config: configArtifactsReadDatasource(randdNameOne, randNameTwo),
					Check:  check,
				},
			},
		})
	})
}