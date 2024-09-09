package buildkite

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccBuildkiteOrganizationRuleDatasource(t *testing.T) {
	ruleActions := []string{"trigger_build", "artifacts_read"}

	configRequiredByID := func(fields ...string) string {
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
			description = "A test cluster containing a source pipeline."
		}

		resource "buildkite_cluster" "cluster_two" {
			name        = "Cluster %s"
			description = "A test cluster containing a target pipeline for triggering builds."
		}

		resource "buildkite_pipeline" "pipeline_source" {
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id			 = buildkite_cluster.cluster_one.id
		}

		resource "buildkite_pipeline" "pipeline_target" {
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id           = buildkite_cluster.cluster_two.id
		}	

		resource "buildkite_organization_rule" "%s_rule" {
			type = "pipeline.%s.pipeline"
			value = jsonencode({
				source_pipeline_uuid = buildkite_pipeline.pipeline_source.uuid
				target_pipeline_uuid = buildkite_pipeline.pipeline_target.uuid
			})
		}

		data "buildkite_organization_rule" "%s_rule" {
			depends_on = [buildkite_organization_rule.%s_rule]
			id = buildkite_organization_rule.%s_rule.id
		}

		`, fields[0], fields[1], fields[0], fields[1], fields[2], fields[2], fields[2], fields[2], fields[2])
	}

	configRequiredByUUID := func(fields ...string) string {
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
			description = "A test cluster containing a source pipeline."
		}

		resource "buildkite_cluster" "cluster_two" {
			name        = "Cluster %s"
			description = "A test cluster containing a target pipeline for triggering builds."
		}

		resource "buildkite_pipeline" "pipeline_source" {
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id			 = buildkite_cluster.cluster_one.id
		}

		resource "buildkite_pipeline" "pipeline_target" {
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id           = buildkite_cluster.cluster_two.id
		}	

		resource "buildkite_organization_rule" "%s_rule" {
			type = "pipeline.%s.pipeline"
			value = jsonencode({
				source_pipeline_uuid = buildkite_pipeline.pipeline_source.uuid
				target_pipeline_uuid = buildkite_pipeline.pipeline_target.uuid
			})
		}

		data "buildkite_organization_rule" "%s_rule" {
			depends_on = [buildkite_organization_rule.%s_rule]
			uuid = buildkite_organization_rule.%s_rule.uuid
		}

		`, fields[0], fields[1], fields[0], fields[1], fields[2], fields[2], fields[2], fields[2], fields[2])
	}

	configAllByID := func(fields ...string) string {
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
			description = "A test cluster containing a source pipeline."
		}

		resource "buildkite_cluster" "cluster_two" {
			name        = "Cluster %s"
			description = "A test cluster containing a target pipeline for triggering builds."
		}

		resource "buildkite_pipeline" "pipeline_source" {
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id			 = buildkite_cluster.cluster_one.id
		}

		resource "buildkite_pipeline" "pipeline_target" {
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id           = buildkite_cluster.cluster_two.id
		}	

		resource "buildkite_organization_rule" "%s_rule" {
			type = "pipeline.%s.pipeline"
			description = "A %s organization rule"
			value = jsonencode({
				source_pipeline_uuid = buildkite_pipeline.pipeline_source.uuid
				target_pipeline_uuid = buildkite_pipeline.pipeline_target.uuid
			})
		}

		data "buildkite_organization_rule" "%s_rule" {
			depends_on = [buildkite_organization_rule.%s_rule]
			id = buildkite_organization_rule.%s_rule.id
		}

		`, fields[0], fields[1], fields[0], fields[1], fields[2], fields[2], fields[2], fields[2], fields[2], fields[2])
	}

	configAllByUUID := func(fields ...string) string {
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
			description = "A test cluster containing a source pipeline."
		}

		resource "buildkite_cluster" "cluster_two" {
			name        = "Cluster %s"
			description = "A test cluster containing a target pipeline for triggering builds."
		}

		resource "buildkite_pipeline" "pipeline_source" {
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id			 = buildkite_cluster.cluster_one.id
		}

		resource "buildkite_pipeline" "pipeline_target" {
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id           = buildkite_cluster.cluster_two.id
		}	

		resource "buildkite_organization_rule" "%s_rule" {
			type = "pipeline.%s.pipeline"
			description = "A %s organization rule"
			value = jsonencode({
				source_pipeline_uuid = buildkite_pipeline.pipeline_source.uuid
				target_pipeline_uuid = buildkite_pipeline.pipeline_target.uuid
			})
		}

		data "buildkite_organization_rule" "%s_rule" {
			depends_on = [buildkite_organization_rule.%s_rule]
			uuid = buildkite_organization_rule.%s_rule.uuid
		}

		`, fields[0], fields[1], fields[0], fields[1], fields[2], fields[2], fields[2], fields[2], fields[2], fields[2])
	}

	for _, action := range ruleActions {
		t.Run(fmt.Sprintf("loads a pipeline.%s.pipeline organization rule with required attributes by id", action), func(t *testing.T) {
			randdNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			var orr organizationRuleResourceModel

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, fmt.Sprintf("buildkite_organization_rule.%s_rule", action)),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule data source's attributes are set in state
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "id"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "uuid"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "type"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "value"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configRequiredByID(randdNameOne, randNameTwo, action),
						Check:  check,
					},
				},
			})
		})

		t.Run(fmt.Sprintf("loads a pipeline.%s.pipeline organization rule with all attributes by id", action), func(t *testing.T) {
			randdNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			var orr organizationRuleResourceModel

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, fmt.Sprintf("buildkite_organization_rule.%s_rule", action)),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule data source's attributes are set in state
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "id"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "uuid"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "type"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "description"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "value"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configAllByID(randdNameOne, randNameTwo, action),
						Check:  check,
					},
				},
			})
		})

		t.Run(fmt.Sprintf("loads a pipeline.%s.pipeline organization rule with required attributes by uuid", action), func(t *testing.T) {
			randdNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			var orr organizationRuleResourceModel

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, fmt.Sprintf("buildkite_organization_rule.%s_rule", action)),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule data source's attributes are set in state
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "id"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "uuid"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "type"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "value"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configRequiredByUUID(randdNameOne, randNameTwo, action),
						Check:  check,
					},
				},
			})
		})

		t.Run(fmt.Sprintf("loads a pipeline.%s.pipeline organization rule with all attributes by uuid", action), func(t *testing.T) {
			randdNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			var orr organizationRuleResourceModel

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, fmt.Sprintf("buildkite_organization_rule.%s_rule", action)),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule data source's attributes are set in state
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "id"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "uuid"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "type"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "description"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "value"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configAllByUUID(randdNameOne, randNameTwo, action),
						Check:  check,
					},
				},
			})
		})
	}
}
