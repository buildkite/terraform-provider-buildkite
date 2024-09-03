package buildkite

import (
	"context"
	"fmt"
	"regexp"
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

		resource "buildkite_cluster" "cluster_source" {
			name        = "Cluster %s"
			description = "A test cluster containing a source pipeline."
		}

		resource "buildkite_cluster" "cluster_target" {
			name        = "Cluster %s"
			description = "A test cluster containing a target pipeline for triggering builds."
		}

		resource "buildkite_pipeline" "pipeline_source" {
			depends_on 			 = [buildkite_cluster.cluster_source]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id			 = buildkite_cluster.cluster_source.id
		}

		resource "buildkite_pipeline" "pipeline_target" {
			depends_on 			 = [buildkite_cluster.cluster_target]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id           = buildkite_cluster.cluster_target.id
		}	

		resource "buildkite_organization_rule" "pipeline_trigger_build_rule" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.trigger_build.pipeline"
			value = jsonencode({
				source_pipeline_uuid = buildkite_pipeline.pipeline_source.uuid
				target_pipeline_uuid = buildkite_pipeline.pipeline_target.uuid
			})
		}

		`, fields[0], fields[1], fields[0], fields[1])
	}

	configArtifactsRead := func(fields ...string) string {
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
			depends_on 			 = [buildkite_cluster.cluster_source]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id			 = buildkite_cluster.cluster_source.id
		}

		resource "buildkite_pipeline" "pipeline_target" {
			depends_on 			 = [buildkite_cluster.cluster_target]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id           = buildkite_cluster.cluster_target.id
		}	

		resource "buildkite_organization_rule" "artifacts_read_rule" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.artifacts_read.pipeline"
			value = jsonencode({
				source_pipeline_uuid = buildkite_pipeline.pipeline_target.uuid
				target_pipeline_uuid = buildkite_pipeline.pipeline_source.uuid
			})
		}

		`, fields[0], fields[1], fields[0], fields[1])
	}

	configNonExistentAction := func(fields ...string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "10s"
				read = "10s"
				update = "10s"
				delete = "10s"
			}
		}

		resource "buildkite_pipeline" "pipeline_source" {
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
		}

		resource "buildkite_pipeline" "pipeline_target" {
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
		}	

		resource "buildkite_organization_rule" "non_existent_action_rule" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.non_existent_action.pipeline"
			value = jsonencode({
				source_pipeline_uuid = buildkite_pipeline.pipeline_target.uuid
				target_pipeline_uuid = buildkite_pipeline.pipeline_source.uuid
			})
		}

		`, fields[0], fields[1])
	}

	configNoSourcePipelineUUID := func(targetName string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "10s"
				read = "10s"
				update = "10s"
				delete = "10s"
			}
		}

		resource "buildkite_pipeline" "pipeline_target" {
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
		}	

		resource "buildkite_organization_rule" "no_source_pipeline_uuid_rule" {
			depends_on = [buildkite_pipeline.pipeline_target]
			type = "pipeline.trigger_build.pipeline"
			value = jsonencode({
				target_pipeline_uuid = buildkite_pipeline.pipeline_target.uuid
			})
		}

		`, targetName)
	}

	configNoTargetPipelineUUID := func(sourceName string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "10s"
				read = "10s"
				update = "10s"
				delete = "10s"
			}
		}

		resource "buildkite_pipeline" "pipeline_source" {
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
		}

		resource "buildkite_organization_rule" "no_target_pipeline_uuid_rule" {
			depends_on = [buildkite_pipeline.pipeline_source]
			type = "pipeline.trigger_build.pipeline"
			value = jsonencode({
				source_pipeline_uuid = buildkite_pipeline.pipeline_source.uuid
			})
		}

		`, sourceName)
	}

	configSourceUUIDInvalid := func(targetName string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "10s"
				read = "10s"
				update = "10s"
				delete = "10s"
			}
		}

		resource "buildkite_pipeline" "pipeline_target" {
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
		}	

		resource "buildkite_organization_rule" "rule_without_a_source" {
			depends_on = [buildkite_pipeline.pipeline_target]
			type = "pipeline.trigger_build.pipeline"
			value = jsonencode({
				source_pipeline_uuid = "non_existent"
				target_pipeline_uuid = buildkite_pipeline.pipeline_target.uuid
			})
		}

		`, targetName)
	}

	configTargetUUIDInvalid := func(sourceName string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "10s"
				read = "10s"
				update = "10s"
				delete = "10s"
			}
		}

		resource "buildkite_pipeline" "pipeline_source" {
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
		}	

		resource "buildkite_organization_rule" "rule_without_a_target" {
			depends_on = [buildkite_pipeline.pipeline_source]
			type = "pipeline.trigger_build.pipeline"
			value = jsonencode({
				source_pipeline_uuid = buildkite_pipeline.pipeline_source.uuid
				target_pipeline_uuid = "non-existent"
			})
		}

		`, sourceName)
	}

	t.Run("creates a pipeline.trigger_build.pipeline organization rule", func(t *testing.T) {
		randNameOne := acctest.RandString(12)
		randNameTwo := acctest.RandString(12)
		var orr organizationRuleResourceModel

		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm the organization rule exists
			testAccCheckOrganizationRuleExists(&orr, "buildkite_organization_rule.pipeline_trigger_build_rule"),
			// Confirm the organization rule has the correct values in Buildkite's system
			testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", "TRIGGER_BUILD", "ALLOW"),
			// Check the organization rule resource's attributes are set in state
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.pipeline_trigger_build_rule", "id"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.pipeline_trigger_build_rule", "uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.pipeline_trigger_build_rule", "type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.pipeline_trigger_build_rule", "value"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleDestroy,
			Steps: []resource.TestStep{
				{
					Config: configTriggerBuild(randNameOne, randNameTwo),
					Check:  check,
				},
			},
		})
	})

	t.Run("creates a pipeline.artifacts_read.pipeline organization rule", func(t *testing.T) {
		randNameOne := acctest.RandString(12)
		randNameTwo := acctest.RandString(12)
		var orr organizationRuleResourceModel

		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm the organization rule exists
			testAccCheckOrganizationRuleExists(&orr, "buildkite_organization_rule.artifacts_read_rule"),
			// Confirm the organization rule has the correct values in Buildkite's system
			testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", "ARTIFACTS_READ", "ALLOW"),
			// Check the organization rule resource's attributes are set in state
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.artifacts_read_rule", "id"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.artifacts_read_rule", "uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.artifacts_read_rule", "type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.artifacts_read_rule", "value"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleDestroy,
			Steps: []resource.TestStep{
				{
					Config: configArtifactsRead(randNameOne, randNameTwo),
					Check:  check,
				},
			},
		})
	})

	t.Run("imports a pipeline.trigger_build.pipeline organization rule", func(t *testing.T) {
		randNameOne := acctest.RandString(12)
		randNameTwo := acctest.RandString(12)
		var orr organizationRuleResourceModel

		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm the organization rule exists
			testAccCheckOrganizationRuleExists(&orr, "buildkite_organization_rule.pipeline_trigger_build_rule"),
			// Confirm the organization rule has the correct values in Buildkite's system
			testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", "TRIGGER_BUILD", "ALLOW"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleDestroy,
			Steps: []resource.TestStep{
				{
					Config: configTriggerBuild(randNameOne, randNameTwo),
					Check:  check,
				},
				{
					ResourceName:      "buildkite_organization_rule.pipeline_trigger_build_rule",
					ImportState:       true,
					ImportStateVerify: true,
				},
			},
		})
	})

	t.Run("imports a pipeline.artifacts_read.pipeline organization rule", func(t *testing.T) {
		randNameOne := acctest.RandString(12)
		randNameTwo := acctest.RandString(12)
		var orr organizationRuleResourceModel

		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm the organization rule exists
			testAccCheckOrganizationRuleExists(&orr, "buildkite_organization_rule.artifacts_read_rule"),
			// Confirm the organization rule has the correct values in Buildkite's system
			testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", "ARTIFACTS_READ", "ALLOW"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleDestroy,
			Steps: []resource.TestStep{
				{
					Config: configArtifactsRead(randNameOne, randNameTwo),
					Check:  check,
				},
				{
					ResourceName:      "buildkite_organization_rule.artifacts_read_rule",
					ImportState:       true,
					ImportStateVerify: true,
				},
			},
		})
	})

	t.Run("errors when an organization rule is specified with an unknown action", func(t *testing.T) {
		randNameOne := acctest.RandString(12)
		randNameTwo := acctest.RandString(12)
		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleDestroy,
			Steps: []resource.TestStep{
				{
					Config:      configNonExistentAction(randNameOne, randNameTwo),
					ExpectError: regexp.MustCompile("input: Rule type is unknown"),
				},
			},
		})
	})

	t.Run("errors when no source_pipeline_uuid key exists within an organization rules' value", func(t *testing.T) {
		randName := acctest.RandString(12)
		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleDestroy,
			Steps: []resource.TestStep{
				{
					Config:      configNoSourcePipelineUUID(randName),
					ExpectError: regexp.MustCompile("pipeline.trigger_build.pipeline: missing source_pipeline_uuid"),
				},
			},
		})
	})

	t.Run("errors when no target_pipeline_uuid key exists within an organization rules' value", func(t *testing.T) {
		randName := acctest.RandString(12)
		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleDestroy,
			Steps: []resource.TestStep{
				{
					Config:      configNoTargetPipelineUUID(randName),
					ExpectError: regexp.MustCompile("pipeline.trigger_build.pipeline: missing target_pipeline_uuid"),
				},
			},
		})
	})

	t.Run("errors when the pipeline defined in source_pipeline_uuid is invalid", func(t *testing.T) {
		randName := acctest.RandString(12)
		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleDestroy,
			Steps: []resource.TestStep{
				{
					Config:      configSourceUUIDInvalid(randName),
					ExpectError: regexp.MustCompile("pipeline.trigger_build.pipeline: source_pipeline_uuid not a valid UUID"),
				},
			},
		})
	})

	t.Run("errors when the pipeline defined in target_pipeline_uuid is invalid", func(t *testing.T) {
		randName := acctest.RandString(12)
		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleDestroy,
			Steps: []resource.TestStep{
				{
					Config:      configTargetUUIDInvalid(randName),
					ExpectError: regexp.MustCompile("pipeline.trigger_build.pipeline: target_pipeline_uuid not a valid UUID"),
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
			updateOrganizatonRuleReadState(orr, *organizationRule)
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

func testAccCheckOrganizationRuleDestroy(s *terraform.State) error {
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
