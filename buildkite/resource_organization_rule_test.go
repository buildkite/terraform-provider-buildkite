package buildkite

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccBuildkiteOrganizationRuleResource(t *testing.T) {

	configTriggerBuild := func(fields ...string) string {
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
			name        = "Cluster Triggerers %s"
			description = "A test cluster containing a triggering pipeline."
		}

		resource "buildkite_cluster" "cluster_two" {
			name        = "Cluster Triggered %s"
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

		resource "buildkite_organization_rule" "cluster_one_two_trigger" {
			name = "pipeline.trigger_build.pipeline"
			value = jsonencode({
				triggering_pipeline_uuid = buildkite_pipeline.pipeline_triggerer.uuid
				triggered_pipeline_uuid = buildkite_pipeline.pipeline_triggered.uuid
			})
		}


		`, fields[0], fields[1], fields[0], fields[1])
	}

	t.Run("creates a trigger_build organization rule", func(t *testing.T) {
		randdNameOne := acctest.RandString(12)
		randNameTwo := acctest.RandString(12)
		var orr organizationRuleResourceModel

		check := resource.ComposeAggregateTestCheckFunc(

			// Confirm the organization exists
			testAccCheckOrganizationRuleExists(&orr, "buildkite_organization_rule.cluster_one_two_trigger"),
			// Confirm the organization rule has the correct values in Buildkite's system
			testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", "TRIGGER_BUILD", "ALLOW"),
			// Check the organization rule resource's attributes are set in state
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.cluster_one_two_trigger", "id"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.cluster_one_two_trigger", "uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.cluster_one_two_trigger", "name"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.cluster_one_two_trigger", "value"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleeDestroy,
			Steps: []resource.TestStep{
				{
					Config: configTriggerBuild(randdNameOne, randNameTwo),
					Check:  check,
				},
			},
		})
	})
}

func testAccCheckOrganizationRuleExists(orr *organizationRuleResourceModel, name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]

		if !ok {
			return fmt.Errorf("Not found in state: %s", name)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("No ID is set in state")
		}

		r, err := getNode(context.Background(), genqlientGraphql, rs.Primary.ID)

		if err != nil {
			return err
		}

		if organizationRule, ok := r.GetNode().(*getNodeNodeRule); ok {
			if organizationRule == nil {
				return fmt.Errorf("Organization rule not found: nil response")
			}
			updateOrganizatonRuleResourceState(orr, *organizationRule)
			fmt.Println(orr)
		}

		return nil
	}
}

func testAccCheckOrganizationRuleRemoteValues(orr *organizationRuleResourceModel, sourceType, targetType, action, effect string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if orr.SourceType.ValueString() != sourceType {
			return fmt.Errorf("Remote organization rule source type (%s) doesn't match expected value (%s)", orr.SourceType, sourceType)
		}

		if orr.TargetType.ValueString() != targetType {
			return fmt.Errorf("Remote organization rule target type (%s) doesn't match expected value (%s)", orr.TargetType, targetType)
		}

		if orr.Action.ValueString() != action {
			return fmt.Errorf("Remote organization rule action (%s) doesn't match expected value (%s)", orr.Action, action)
		}

		if orr.Effect.ValueString() != effect {
			return fmt.Errorf("Remote organization rule effect (%s) doesn't match expected value (%s)", orr.Effect, effect)
		}

		return nil
	}
}

func testAccCheckOrganizationRuleeDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_organization_rule" {
			continue
		}

		r, err := getNode(context.Background(), genqlientGraphql, rs.Primary.ID)

		if err != nil {
			return err
		}

		if organizationRule, ok := r.GetNode().(*getNodeNodeRule); ok {
			if organizationRule != nil {
				return fmt.Errorf("Organization rule still exists")
			}
		}
	}
	return nil
}
