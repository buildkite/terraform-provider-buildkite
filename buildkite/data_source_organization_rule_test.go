package buildkite

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccBuildkiteOrganizationRuleDatasource(t *testing.T) {
	t.Parallel()

	// Both lookup forms read the same rule, so share each complete fixture lifecycle.
	ruleActions := []string{"trigger_build", "artifacts_read"}

	configRequired := func(sourceName, targetName, action string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_cluster" "cluster_one" {
			name        = "Cluster %[1]s"
			description = "A test cluster containing a source pipeline."
		}

		resource "buildkite_cluster" "cluster_two" {
			name        = "Cluster %[2]s"
			description = "A test cluster containing a target pipeline for triggering builds."
		}

		resource "buildkite_pipeline" "pipeline_source" {
			name                 = "Pipeline %[1]s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id			 = buildkite_cluster.cluster_one.id
		}

		resource "buildkite_pipeline" "pipeline_target" {
			name                 = "Pipeline %[2]s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id           = buildkite_cluster.cluster_two.id
		}	

		resource "buildkite_organization_rule" "%[3]s_rule" {
			type = "pipeline.%[3]s.pipeline"
			value = jsonencode({
				source_pipeline = buildkite_pipeline.pipeline_source.uuid
				target_pipeline = buildkite_pipeline.pipeline_target.uuid
			})
		}

		data "buildkite_organization_rule" "%[3]s_rule_by_id" {
			depends_on = [buildkite_organization_rule.%[3]s_rule]
			id = buildkite_organization_rule.%[3]s_rule.id
		}

		data "buildkite_organization_rule" "%[3]s_rule_by_uuid" {
			depends_on = [buildkite_organization_rule.%[3]s_rule]
			uuid = buildkite_organization_rule.%[3]s_rule.uuid
		}

		`, sourceName, targetName, action)
	}

	configAll := func(sourceName, targetName, action string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_cluster" "cluster_one" {
			name        = "Cluster %[1]s"
			description = "A test cluster containing a source pipeline."
		}

		resource "buildkite_cluster" "cluster_two" {
			name        = "Cluster %[2]s"
			description = "A test cluster containing a target pipeline for triggering builds."
		}

		resource "buildkite_pipeline" "pipeline_source" {
			name                 = "Pipeline %[1]s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id			 = buildkite_cluster.cluster_one.id
		}

		resource "buildkite_pipeline" "pipeline_target" {
			name                 = "Pipeline %[2]s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id           = buildkite_cluster.cluster_two.id
		}	

		resource "buildkite_organization_rule" "%[3]s_rule" {
			type = "pipeline.%[3]s.pipeline"
			description = "A %[3]s organization rule"
			value = jsonencode({
				source_pipeline = buildkite_pipeline.pipeline_source.uuid
				target_pipeline = buildkite_pipeline.pipeline_target.uuid
				conditions = [
					"source.build.creator.teams includes 'deploy'",
					"source.build.branch == 'main'"
				]
			})
		}

		data "buildkite_organization_rule" "%[3]s_rule_by_id" {
			depends_on = [buildkite_organization_rule.%[3]s_rule]
			id = buildkite_organization_rule.%[3]s_rule.id
		}

		data "buildkite_organization_rule" "%[3]s_rule_by_uuid" {
			depends_on = [buildkite_organization_rule.%[3]s_rule]
			uuid = buildkite_organization_rule.%[3]s_rule.uuid
		}

		`, sourceName, targetName, action)
	}

	for _, action := range ruleActions {
		t.Run(fmt.Sprintf("loads a pipeline.%s.pipeline organization rule with required attributes by id and uuid", action), func(t *testing.T) {
			sourceName := acctest.RandString(12)
			targetName := acctest.RandString(12)
			var orr organizationRuleResourceModel

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, fmt.Sprintf("buildkite_organization_rule.%s_rule", action)),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule data source's attributes are set in state
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule_by_id", action), "id"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule_by_id", action), "uuid"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule_by_id", action), "type"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule_by_id", action), "value"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule_by_uuid", action), "id"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule_by_uuid", action), "uuid"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule_by_uuid", action), "type"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule_by_uuid", action), "value"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configRequired(sourceName, targetName, action),
						Check:  check,
					},
				},
			})
		})

		t.Run(fmt.Sprintf("loads a pipeline.%s.pipeline organization rule with all attributes by id and uuid", action), func(t *testing.T) {
			sourceName := acctest.RandString(12)
			targetName := acctest.RandString(12)
			var orr organizationRuleResourceModel

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, fmt.Sprintf("buildkite_organization_rule.%s_rule", action)),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule data source's attributes are set in state
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule_by_id", action), "id"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule_by_id", action), "uuid"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule_by_id", action), "type"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule_by_id", action), "description"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule_by_id", action), "value"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule_by_uuid", action), "id"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule_by_uuid", action), "uuid"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule_by_uuid", action), "type"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule_by_uuid", action), "description"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule_by_uuid", action), "value"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configAll(sourceName, targetName, action),
						Check:  check,
					},
				},
			})
		})
	}
}
