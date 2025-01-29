package buildkite

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccBuildkiteOrganizationRuleResource(t *testing.T) {
	ruleActions := []string{"trigger_build", "artifacts_read"}

	configRequired := func(fields ...string) string {
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

		resource "buildkite_organization_rule" "%s_rule" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.%s.pipeline"
			value = jsonencode({
				source_pipeline = buildkite_pipeline.pipeline_source.uuid
				target_pipeline = buildkite_pipeline.pipeline_target.uuid
			})
		}

		`, fields[0], fields[1], fields[0], fields[1], fields[2], fields[2])
	}

	configAll := func(fields ...string) string {
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

		resource "buildkite_organization_rule" "%s_rule" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.%s.pipeline"
			description = "A pipeline.%s.pipeline rule"
			value = jsonencode({
				source_pipeline = buildkite_pipeline.pipeline_source.uuid
				target_pipeline = buildkite_pipeline.pipeline_target.uuid
				conditions = [
					"source.build.creator.teams includes 'deploy'",
					"source.build.branch == 'main'"
				]
			})
		}

		`, fields[0], fields[1], fields[0], fields[1], fields[2], fields[2], fields[2])
	}

	configAllCustomDescription := func(fields ...string) string {
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

		resource "buildkite_organization_rule" "%s_rule" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.%s.pipeline"
			description = "A pipeline.%s.pipeline rule with a custom %s description"
			value = jsonencode({
				source_pipeline = buildkite_pipeline.pipeline_source.uuid
				target_pipeline = buildkite_pipeline.pipeline_target.uuid
				conditions = [
					"source.build.creator.teams includes 'deploy'",
					"source.build.branch == 'main'"
				]
			})
		}

		`, fields[0], fields[1], fields[0], fields[1], fields[2], fields[2], fields[2], fields[3])
	}

	configAllWithoutDescription := func(fields ...string) string {
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

		resource "buildkite_organization_rule" "%s_rule" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.%s.pipeline"
			value = jsonencode({
				source_pipeline = buildkite_pipeline.pipeline_source.uuid
				target_pipeline = buildkite_pipeline.pipeline_target.uuid
				conditions = [
					"source.build.creator.teams includes 'deploy'",
					"source.build.branch == 'main'"
				]
			})
		}

		`, fields[0], fields[1], fields[0], fields[1], fields[2], fields[2])
	}

	configAllCustomConditions := func(fields ...string) string {
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

		resource "buildkite_organization_rule" "%s_rule" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.%s.pipeline"
			description = "A pipeline.%s.pipeline rule with a custom description"
			value = jsonencode({
				source_pipeline = buildkite_pipeline.pipeline_source.uuid
				target_pipeline = buildkite_pipeline.pipeline_target.uuid
				conditions = [%s]
			})
		}

		`, fields[0], fields[1], fields[0], fields[1], fields[2], fields[2], fields[2], fields[3])
	}

	configRequiredNewSource := func(fields ...string) string {
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

		resource "buildkite_pipeline" "pipeline_new_source" {
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

		resource "buildkite_organization_rule" "%s_rule" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.%s.pipeline"
			value = jsonencode({
				source_pipeline = buildkite_pipeline.pipeline_new_source.uuid
				target_pipeline = buildkite_pipeline.pipeline_target.uuid
			})
		}

		`, fields[0], fields[1], fields[0], fields[2], fields[1], fields[3], fields[3])
	}

	configRequiredNewTarget := func(fields ...string) string {
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

		resource "buildkite_pipeline" "pipeline_new_target" {
			depends_on 			 = [buildkite_cluster.cluster_target]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id			 = buildkite_cluster.cluster_source.id
		}	

		resource "buildkite_organization_rule" "%s_rule" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.%s.pipeline"
			value = jsonencode({
				source_pipeline = buildkite_pipeline.pipeline_source.uuid
				target_pipeline = buildkite_pipeline.pipeline_new_target.uuid
			})
		}

		`, fields[0], fields[1], fields[0], fields[1], fields[2], fields[3], fields[3])
	}

	configRequiredSwap := func(fields ...string) string {
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

		resource "buildkite_organization_rule" "%s_rule" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.%s.pipeline"
			value = jsonencode({
				source_pipeline = buildkite_pipeline.pipeline_target.uuid
				target_pipeline = buildkite_pipeline.pipeline_source.uuid
			})
		}

		`, fields[0], fields[1], fields[0], fields[1], fields[2], fields[2])
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
				source_pipeline = buildkite_pipeline.pipeline_target.uuid
				target_pipeline = buildkite_pipeline.pipeline_source.uuid
			})
		}

		`, fields[0], fields[1])
	}

	configInvalidConditional := func(fields ...string) string {
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

		resource "buildkite_organization_rule" "non_existent_condition" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.trigger_build.pipeline"
			value = jsonencode({
				source_pipeline = buildkite_pipeline.pipeline_target.uuid
				target_pipeline = buildkite_pipeline.pipeline_source.uuid
				conditions = [
					"source.nonexistent_condition includes 'idontexist'"
				]
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

		resource "buildkite_organization_rule" "no_source_pipeline_rule" {
			depends_on = [buildkite_pipeline.pipeline_target]
			type = "pipeline.trigger_build.pipeline"
			value = jsonencode({
				target_pipeline = buildkite_pipeline.pipeline_target.uuid
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

		resource "buildkite_organization_rule" "no_target_pipeline_rule" {
			depends_on = [buildkite_pipeline.pipeline_source]
			type = "pipeline.trigger_build.pipeline"
			value = jsonencode({
				source_pipeline = buildkite_pipeline.pipeline_source.uuid
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
				source_pipeline = "non_existent"
				target_pipeline = buildkite_pipeline.pipeline_target.uuid
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
				source_pipeline = buildkite_pipeline.pipeline_source.uuid
				target_pipeline = "non-existent"
			})
		}

		`, sourceName)
	}

	configUpdateErrorInvalidSource := func(fields ...string) string {
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

		resource "buildkite_organization_rule" "%s_rule" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.%s.pipeline"
			description = "A pipeline.%s.pipeline rule description"
			value = jsonencode({
				source_pipeline = "non-existent"
				target_pipeline = buildkite_pipeline.pipeline_target.uuid
				conditions = [
					"source.build.creator.teams includes 'deploy'",
					"source.build.branch == 'main'"
				]
			})
		}

		`, fields[0], fields[1], fields[0], fields[1], fields[2], fields[2], fields[2])
	}

	configUpdateErrorInvalidTarget := func(fields ...string) string {
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

		resource "buildkite_organization_rule" "%s_rule" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.%s.pipeline"
			description = "A pipeline.%s.pipeline rule description"
			value = jsonencode({
				source_pipeline = buildkite_pipeline.pipeline_target.uuid
				target_pipeline = "non-existent"
				conditions = [
					"source.build.creator.teams includes 'deploy'",
					"source.build.branch == 'main'"
				]
			})
		}

		`, fields[0], fields[1], fields[0], fields[1], fields[2], fields[2], fields[2])
	}

	configUpdateErrorNoSource := func(fields ...string) string {
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

		resource "buildkite_organization_rule" "%s_rule" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.%s.pipeline"
			description = "A pipeline.%s.pipeline rule description"
			value = jsonencode({
				target_pipeline = buildkite_pipeline.pipeline_target.uuid
				conditions = [
					"source.build.creator.teams includes 'deploy'",
					"source.build.branch == 'main'"
				]
			})
		}

		`, fields[0], fields[1], fields[0], fields[1], fields[2], fields[2], fields[2])
	}

	configUpdateErrorNoTarget := func(fields ...string) string {
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

		resource "buildkite_organization_rule" "%s_rule" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.%s.pipeline"
			description = "A pipeline.%s.pipeline rule description"
			value = jsonencode({
				source_pipeline = buildkite_pipeline.pipeline_source.uuid
				conditions = [
					"source.build.creator.teams includes 'deploy'",
					"source.build.branch == 'main'"
				]
			})
		}

		`, fields[0], fields[1], fields[0], fields[1], fields[2], fields[2], fields[2])
	}

	configUpdateErrorSourceKey := func(fields ...string) string {
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

		resource "buildkite_organization_rule" "%s_rule" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.%s.pipeline"
			description = "A pipeline.%s.pipeline rule description"
			value = jsonencode({
				sourc_pipeline = buildkite_pipeline.pipeline_source.uuid
				target_pipeline = buildkite_pipeline.pipeline_target.uuid
				conditions = [
					"source.build.creator.teams includes 'deploy'",
					"source.build.branch == 'main'"
				]
			})
		}

		`, fields[0], fields[1], fields[0], fields[1], fields[2], fields[2], fields[2])
	}

	configUpdateErrorTargetKey := func(fields ...string) string {
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

		resource "buildkite_organization_rule" "%s_rule" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.%s.pipeline"
			description = "A pipeline.%s.pipeline rule description"
			value = jsonencode({
				source_pipeline = buildkite_pipeline.pipeline_source.uuid
				target_pipelin = buildkite_pipeline.pipeline_target.uuid
				conditions = [
					"source.build.creator.teams includes 'deploy'",
					"source.build.branch == 'main'"
				]
			})
		}

		`, fields[0], fields[1], fields[0], fields[1], fields[2], fields[2], fields[2])
	}

	for _, action := range ruleActions {
		// Formatted resource name used for all create/update/import test cases
		resourceName := fmt.Sprintf("buildkite_organization_rule.%s_rule", action)

		t.Run(fmt.Sprintf("creates a pipeline.%s.pipeline organization rule with required attributes", action), func(t *testing.T) {
			randNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			var orr organizationRuleResourceModel

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "id"),
				resource.TestCheckResourceAttrSet(resourceName, "uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "target_type"),
				resource.TestCheckResourceAttrSet(resourceName, "target_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
				// Assert organization rule resource's state values
				resource.TestCheckResourceAttr(resourceName, "type", fmt.Sprintf("pipeline.%s.pipeline", action)),
				resource.TestCheckResourceAttr(resourceName, "source_type", "PIPELINE"),
				resource.TestCheckResourceAttr(resourceName, "target_type", "PIPELINE"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configRequired(randNameOne, randNameTwo, action),
						Check:  check,
					},
				},
			})
		})

		t.Run(fmt.Sprintf("creates a pipeline.%s.pipeline organization rule with all attributes", action), func(t *testing.T) {
			randNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			var orr organizationRuleResourceModel

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "id"),
				resource.TestCheckResourceAttrSet(resourceName, "uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "type"),
				resource.TestCheckResourceAttrSet(resourceName, "description"),
				resource.TestCheckResourceAttrSet(resourceName, "source_type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "target_type"),
				resource.TestCheckResourceAttrSet(resourceName, "target_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
				// Assert organization rule resource's state values
				resource.TestCheckResourceAttr(resourceName, "type", fmt.Sprintf("pipeline.%s.pipeline", action)),
				resource.TestCheckResourceAttr(resourceName, "description", fmt.Sprintf("A pipeline.%s.pipeline rule", action)),
				resource.TestCheckResourceAttr(resourceName, "source_type", "PIPELINE"),
				resource.TestCheckResourceAttr(resourceName, "target_type", "PIPELINE"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configAll(randNameOne, randNameTwo, action),
						Check:  check,
					},
				},
			})
		})

		t.Run(fmt.Sprintf("updates a pipeline.%s.pipeline organization rule by adding a description", action), func(t *testing.T) {
			randNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			description := acctest.RandString(12)
			var orr organizationRuleResourceModel

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "id"),
				resource.TestCheckResourceAttrSet(resourceName, "uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "target_type"),
				resource.TestCheckResourceAttrSet(resourceName, "target_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
				// Assert organization rule resource's state values
				resource.TestCheckResourceAttr(resourceName, "type", fmt.Sprintf("pipeline.%s.pipeline", action)),
				resource.TestCheckResourceAttr(resourceName, "source_type", "PIPELINE"),
				resource.TestCheckResourceAttr(resourceName, "target_type", "PIPELINE"),
			)

			checkUpdated := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's description attribute is set in state
				resource.TestCheckResourceAttrSet(resourceName, "description"),
				// Asserts organization rule resource's added description is in state
				resource.TestCheckResourceAttr(resourceName, "description", fmt.Sprintf("A pipeline.%s.pipeline rule with a custom %s description", action, description)),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configAllWithoutDescription(randNameOne, randNameTwo, action),
						Check:  check,
					},
					{
						Config: configAllCustomDescription(randNameOne, randNameTwo, action, description),
						Check:  checkUpdated,
					},
				},
			})
		})

		t.Run(fmt.Sprintf("updates a pipeline.%s.pipeline organization rule by editing its current description", action), func(t *testing.T) {
			randNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			description := acctest.RandString(12)
			updatedDescription := acctest.RandString(12)
			var orr organizationRuleResourceModel

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "id"),
				resource.TestCheckResourceAttrSet(resourceName, "uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "type"),
				resource.TestCheckResourceAttrSet(resourceName, "description"),
				resource.TestCheckResourceAttrSet(resourceName, "source_type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "target_type"),
				resource.TestCheckResourceAttrSet(resourceName, "target_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
				// Asserts organization rule resource's initial description in state
				resource.TestCheckResourceAttr(resourceName, "description", fmt.Sprintf("A pipeline.%s.pipeline rule with a custom %s description", action, description)),
			)

			checkUpdated := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's description attribute is set in state
				resource.TestCheckResourceAttrSet(resourceName, "description"),
				// Asserts organization rule resource's updated description in state
				resource.TestCheckResourceAttr(resourceName, "description", fmt.Sprintf("A pipeline.%s.pipeline rule with a custom %s description", action, updatedDescription)),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configAllCustomDescription(randNameOne, randNameTwo, action, description),
						Check:  check,
					},
					{
						Config: configAllCustomDescription(randNameOne, randNameTwo, action, updatedDescription),
						Check:  checkUpdated,
					},
				},
			})
		})

		t.Run(fmt.Sprintf("updates a pipeline.%s.pipeline organization rule by removing its description", action), func(t *testing.T) {
			randNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			description := acctest.RandString(12)
			var orr organizationRuleResourceModel

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "id"),
				resource.TestCheckResourceAttrSet(resourceName, "uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "type"),
				resource.TestCheckResourceAttrSet(resourceName, "description"),
				resource.TestCheckResourceAttrSet(resourceName, "source_type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "target_type"),
				resource.TestCheckResourceAttrSet(resourceName, "target_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
				// Check the organization rule resource's source, target, type and existing description attributes are set in state
				resource.TestCheckResourceAttr(resourceName, "type", fmt.Sprintf("pipeline.%s.pipeline", action)),
				resource.TestCheckResourceAttr(resourceName, "source_type", "PIPELINE"),
				resource.TestCheckResourceAttr(resourceName, "target_type", "PIPELINE"),
				resource.TestCheckResourceAttr(resourceName, "description", fmt.Sprintf("A pipeline.%s.pipeline rule with a custom %s description", action, description)),
			)

			checkUpdated := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configAllCustomDescription(randNameOne, randNameTwo, action, description),
						Check:  check,
					},
					{
						Config: configAllWithoutDescription(randNameOne, randNameTwo, action),
						Check:  checkUpdated,
					},
				},
			})
		})

		t.Run(fmt.Sprintf("updates a pipeline.%s.pipeline organization rule by changing its source_pipeline", action), func(t *testing.T) {
			randNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			randNameThree := acctest.RandString(12)
			var orr organizationRuleResourceModel

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "id"),
				resource.TestCheckResourceAttrSet(resourceName, "uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "target_type"),
				resource.TestCheckResourceAttrSet(resourceName, "target_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			checkUpdated := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's source(s) and value attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "source_type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configRequired(randNameOne, randNameTwo, action),
						Check:  check,
					},
					{
						Config: configRequiredNewSource(randNameOne, randNameTwo, randNameThree, action),
						Check:  checkUpdated,
					},
				},
			})
		})

		t.Run(fmt.Sprintf("updates a pipeline.%s.pipeline organization rule by changing its target_pipeline", action), func(t *testing.T) {
			randNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			randNameThree := acctest.RandString(12)
			var orr organizationRuleResourceModel

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "id"),
				resource.TestCheckResourceAttrSet(resourceName, "uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "target_type"),
				resource.TestCheckResourceAttrSet(resourceName, "target_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			checkUpdated := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's target(s) and value attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "target_type"),
				resource.TestCheckResourceAttrSet(resourceName, "target_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configRequired(randNameOne, randNameTwo, action),
						Check:  check,
					},
					{
						Config: configRequiredNewTarget(randNameOne, randNameTwo, randNameThree, action),
						Check:  checkUpdated,
					},
				},
			})
		})

		t.Run(fmt.Sprintf("updates a pipeline.%s.pipeline organization rule by changing both source_pipeline and target_pipeline", action), func(t *testing.T) {
			randNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			var orr organizationRuleResourceModel

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "id"),
				resource.TestCheckResourceAttrSet(resourceName, "uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "target_type"),
				resource.TestCheckResourceAttrSet(resourceName, "target_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			checkUpdated := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's source, target and value attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "source_type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "target_type"),
				resource.TestCheckResourceAttrSet(resourceName, "target_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configRequired(randNameOne, randNameTwo, action),
						Check:  check,
					},
					{
						Config: configRequiredSwap(randNameOne, randNameTwo, action),
						Check:  checkUpdated,
					},
				},
			})
		})

		t.Run(fmt.Sprintf("updates a pipeline.%s.pipeline organization rule by adding conditions", action), func(t *testing.T) {
			randNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			var orr organizationRuleResourceModel

			conditions := `
      			"source.build.branch == 'develop'",
      			"source.pipeline.slug == 'monorepo-core'",
				"source.build.creator.teams includes 'monorepo'"
			`

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "id"),
				resource.TestCheckResourceAttrSet(resourceName, "uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "target_type"),
				resource.TestCheckResourceAttrSet(resourceName, "target_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			checkUpdated := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's value attribute is set in state
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configRequired(randNameOne, randNameTwo, action),
						Check:  check,
					},
					{
						Config: configAllCustomConditions(randNameOne, randNameTwo, action, conditions),
						Check:  checkUpdated,
					},
				},
			})
		})

		t.Run(fmt.Sprintf("updates a pipeline.%s.pipeline organization rule by inserting additional conditions", action), func(t *testing.T) {
			randNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			var orr organizationRuleResourceModel

			initConditions := `
      			"source.build.branch == 'develop'",
      			"source.pipeline.slug == 'monorepo-core'"
			`

			updatedConditions := `
      			"source.build.branch == 'develop'",
      			"source.pipeline.slug == 'monorepo-core'",
				"source.build.creator.teams includes 'monorepo'"
			`

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "id"),
				resource.TestCheckResourceAttrSet(resourceName, "uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "type"),
				resource.TestCheckResourceAttrSet(resourceName, "description"),
				resource.TestCheckResourceAttrSet(resourceName, "source_type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "target_type"),
				resource.TestCheckResourceAttrSet(resourceName, "target_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			checkUpdated := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's value attribute is set in state
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configAllCustomConditions(randNameOne, randNameTwo, action, initConditions),
						Check:  check,
					},
					{
						Config: configAllCustomConditions(randNameOne, randNameTwo, action, updatedConditions),
						Check:  checkUpdated,
					},
				},
			})
		})

		t.Run(fmt.Sprintf("updates a pipeline.%s.pipeline organization rule by removing some conditions", action), func(t *testing.T) {
			randNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			var orr organizationRuleResourceModel

			initConditions := `
  				"source.build.branch == 'develop'",
      			"source.pipeline.slug == 'monorepo-core'",
				"source.build.creator.teams includes 'monorepo'"
			`

			updatedConditions := `
          		"source.build.branch == 'develop'",
				"source.build.creator.teams includes 'monorepo'"
			`

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "id"),
				resource.TestCheckResourceAttrSet(resourceName, "uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "type"),
				resource.TestCheckResourceAttrSet(resourceName, "description"),
				resource.TestCheckResourceAttrSet(resourceName, "source_type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "target_type"),
				resource.TestCheckResourceAttrSet(resourceName, "target_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			checkUpdated := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's value attribute is set in state
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configAllCustomConditions(randNameOne, randNameTwo, action, initConditions),
						Check:  check,
					},
					{
						Config: configAllCustomConditions(randNameOne, randNameTwo, action, updatedConditions),
						Check:  checkUpdated,
					},
				},
			})
		})

		t.Run(fmt.Sprintf("updates a pipeline.%s.pipeline organization rule by removing all conditions", action), func(t *testing.T) {
			randNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			var orr organizationRuleResourceModel

			existingConditions := `
      			"source.build.branch == 'develop'",
      			"source.pipeline.slug == 'monorepo-core'",
				"source.build.creator.teams includes 'monorepo'"
			`

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "id"),
				resource.TestCheckResourceAttrSet(resourceName, "uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "type"),
				resource.TestCheckResourceAttrSet(resourceName, "description"),
				resource.TestCheckResourceAttrSet(resourceName, "source_type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "target_type"),
				resource.TestCheckResourceAttrSet(resourceName, "target_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			checkUpdated := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's value attribute is set in state
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configAllCustomConditions(randNameOne, randNameTwo, action, existingConditions),
						Check:  check,
					},
					{
						Config: configRequired(randNameOne, randNameTwo, action),
						Check:  checkUpdated,
					},
				},
			})
		})

		t.Run(fmt.Sprintf("updates a pipeline.%s.pipeline organization rule by editing its current conditions", action), func(t *testing.T) {
			randNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			var orr organizationRuleResourceModel

			initConditions := `
  				"source.build.branch == 'develop'",
      			"source.pipeline.slug == 'monorepo-core'",
				"source.build.creator.teams includes 'monorepo'"
			`

			updatedConditions := `
  				"source.build.branch == 'dev'",
      			"source.pipeline.slug == 'monorepo'",
				"source.build.creator.teams includes 'monorepo'"
			`

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "id"),
				resource.TestCheckResourceAttrSet(resourceName, "uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "type"),
				resource.TestCheckResourceAttrSet(resourceName, "description"),
				resource.TestCheckResourceAttrSet(resourceName, "source_type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "target_type"),
				resource.TestCheckResourceAttrSet(resourceName, "target_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			checkUpdated := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's value attribute is set in state
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configAllCustomConditions(randNameOne, randNameTwo, action, initConditions),
						Check:  check,
					},
					{
						Config: configAllCustomConditions(randNameOne, randNameTwo, action, updatedConditions),
						Check:  checkUpdated,
					},
				},
			})
		})

		t.Run(fmt.Sprintf("imports a pipeline.%s.pipeline organization rule with required attributes", action), func(t *testing.T) {
			randNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			var orr organizationRuleResourceModel

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "id"),
				resource.TestCheckResourceAttrSet(resourceName, "uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "target_type"),
				resource.TestCheckResourceAttrSet(resourceName, "target_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configRequired(randNameOne, randNameTwo, action),
						Check:  check,
					},
					{
						ResourceName:      resourceName,
						ImportState:       true,
						ImportStateVerify: true,
					},
				},
			})
		})

		t.Run(fmt.Sprintf("imports a pipeline.%s.pipeline organization rule with all attributes", action), func(t *testing.T) {
			randNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			var orr organizationRuleResourceModel

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "id"),
				resource.TestCheckResourceAttrSet(resourceName, "uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "type"),
				resource.TestCheckResourceAttrSet(resourceName, "description"),
				resource.TestCheckResourceAttrSet(resourceName, "source_type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "target_type"),
				resource.TestCheckResourceAttrSet(resourceName, "target_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configAll(randNameOne, randNameTwo, action),
						Check:  check,
					},
					{
						ResourceName:      resourceName,
						ImportState:       true,
						ImportStateVerify: true,
					},
				},
			})
		})
	}

	t.Run("errors when an organization rule is created with an unknown action", func(t *testing.T) {
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

	t.Run("errors when an organization rule is created with an invalid conditional", func(t *testing.T) {
		randNameOne := acctest.RandString(12)
		randNameTwo := acctest.RandString(12)
		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleDestroy,
			Steps: []resource.TestStep{
				{
					Config:      configInvalidConditional(randNameOne, randNameTwo),
					ExpectError: regexp.MustCompile("conditional is invalid:\n`source.nonexistent_condition` is not a variable"),
				},
			},
		})
	})

	t.Run("errors when an organization rule is created with a missing source_pipeline in its value", func(t *testing.T) {
		randName := acctest.RandString(12)
		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleDestroy,
			Steps: []resource.TestStep{
				{
					Config:      configNoSourcePipelineUUID(randName),
					ExpectError: regexp.MustCompile("pipeline.trigger_build.pipeline: missing source_pipeline"),
				},
			},
		})
	})

	t.Run("errors when an organization rule is created with a missing target_pipeline in its value", func(t *testing.T) {
		randName := acctest.RandString(12)
		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleDestroy,
			Steps: []resource.TestStep{
				{
					Config:      configNoTargetPipelineUUID(randName),
					ExpectError: regexp.MustCompile("pipeline.trigger_build.pipeline: missing target_pipeline"),
				},
			},
		})
	})

	t.Run("errors when an organization rule is created with an invalid source_pipeline UUID", func(t *testing.T) {
		randName := acctest.RandString(12)
		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleDestroy,
			Steps: []resource.TestStep{
				{
					Config:      configSourceUUIDInvalid(randName),
					ExpectError: regexp.MustCompile("pipeline.trigger_build.pipeline: source_pipeline is an invalid UUID."),
				},
			},
		})
	})

	t.Run("errors when an organization rule is created with an invalid target_pipeline UUID", func(t *testing.T) {
		randName := acctest.RandString(12)
		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleDestroy,
			Steps: []resource.TestStep{
				{
					Config:      configTargetUUIDInvalid(randName),
					ExpectError: regexp.MustCompile("pipeline.trigger_build.pipeline: target_pipeline is an invalid UUID."),
				},
			},
		})
	})

	t.Run("errors when an organization rule is updated with a malformed source_pipeline key", func(t *testing.T) {
		randNameOne := acctest.RandString(12)
		randNameTwo := acctest.RandString(12)
		var orr organizationRuleResourceModel

		check := resource.ComposeAggregateTestCheckFunc(
			testAccCheckOrganizationRuleExists(&orr, "buildkite_organization_rule.trigger_build_rule"),
			// Confirm the organization rule has the correct values in Buildkite's system
			testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", "TRIGGER_BUILD", "ALLOW"),
			// Check the organization rule resource's attributes are set in state
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "id"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "description"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "source_type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "source_uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "target_type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "target_uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "value"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleDestroy,
			Steps: []resource.TestStep{
				{
					Config: configAll(randNameOne, randNameTwo, "trigger_build"),
					Check:  check,
				},
				{
					Config:      configUpdateErrorSourceKey(randNameOne, randNameTwo, "trigger_build"),
					ExpectError: regexp.MustCompile("pipeline.trigger_build.pipeline: missing source_pipeline"),
				},
			},
		})
	})

	t.Run("errors when an organization rule is updated with a malformed target_pipeline key", func(t *testing.T) {
		randNameOne := acctest.RandString(12)
		randNameTwo := acctest.RandString(12)
		var orr organizationRuleResourceModel

		check := resource.ComposeAggregateTestCheckFunc(
			testAccCheckOrganizationRuleExists(&orr, "buildkite_organization_rule.trigger_build_rule"),
			// Confirm the organization rule has the correct values in Buildkite's system
			testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", "TRIGGER_BUILD", "ALLOW"),
			// Check the organization rule resource's attributes are set in state
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "id"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "description"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "source_type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "source_uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "target_type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "target_uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "value"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleDestroy,
			Steps: []resource.TestStep{
				{
					Config: configAll(randNameOne, randNameTwo, "trigger_build"),
					Check:  check,
				},
				{
					Config:      configUpdateErrorTargetKey(randNameOne, randNameTwo, "trigger_build"),
					ExpectError: regexp.MustCompile("pipeline.trigger_build.pipeline: missing target_pipeline"),
				},
			},
		})
	})

	t.Run("errors when an organization rule is updated with an invalid source_pipeline UUID", func(t *testing.T) {
		randNameOne := acctest.RandString(12)
		randNameTwo := acctest.RandString(12)
		var orr organizationRuleResourceModel

		check := resource.ComposeAggregateTestCheckFunc(
			testAccCheckOrganizationRuleExists(&orr, "buildkite_organization_rule.trigger_build_rule"),
			// Confirm the organization rule has the correct values in Buildkite's system
			testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", "TRIGGER_BUILD", "ALLOW"),
			// Check the organization rule resource's attributes are set in state
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "id"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "description"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "source_type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "source_uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "target_type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "target_uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "value"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleDestroy,
			Steps: []resource.TestStep{
				{
					Config: configAll(randNameOne, randNameTwo, "trigger_build"),
					Check:  check,
				},
				{
					Config:      configUpdateErrorInvalidSource(randNameOne, randNameTwo, "trigger_build"),
					ExpectError: regexp.MustCompile("pipeline.trigger_build.pipeline: source_pipeline not found"),
				},
			},
		})
	})

	t.Run("errors when an organization rule is updated with an invalid target_pipeline UUID", func(t *testing.T) {
		randNameOne := acctest.RandString(12)
		randNameTwo := acctest.RandString(12)
		var orr organizationRuleResourceModel

		check := resource.ComposeAggregateTestCheckFunc(
			testAccCheckOrganizationRuleExists(&orr, "buildkite_organization_rule.trigger_build_rule"),
			// Confirm the organization rule has the correct values in Buildkite's system
			testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", "TRIGGER_BUILD", "ALLOW"),
			// Check the organization rule resource's attributes are set in state
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "id"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "description"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "source_type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "source_uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "target_type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "target_uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "value"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleDestroy,
			Steps: []resource.TestStep{
				{
					Config: configAll(randNameOne, randNameTwo, "trigger_build"),
					Check:  check,
				},
				{
					Config:      configUpdateErrorInvalidTarget(randNameOne, randNameTwo, "trigger_build"),
					ExpectError: regexp.MustCompile("pipeline.trigger_build.pipeline: target_pipeline not found"),
				},
			},
		})
	})

	t.Run("errors when an organization rule is updated with no source_pipeline UUID", func(t *testing.T) {
		ruleType := "trigger_build"
		randNameOne := acctest.RandString(12)
		randNameTwo := acctest.RandString(12)
		var orr organizationRuleResourceModel

		check := resource.ComposeAggregateTestCheckFunc(
			testAccCheckOrganizationRuleExists(&orr, "buildkite_organization_rule.trigger_build_rule"),
			// Confirm the organization rule has the correct values in Buildkite's system
			testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", "TRIGGER_BUILD", "ALLOW"),
			// Check the organization rule resource's attributes are set in state
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "id"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "description"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "source_type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "source_uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "target_type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "target_uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "value"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleDestroy,
			Steps: []resource.TestStep{
				{
					Config: configAll(randNameOne, randNameTwo, ruleType),
					Check:  check,
				},
				{
					Config:      configUpdateErrorNoSource(randNameOne, randNameTwo, ruleType),
					ExpectError: regexp.MustCompile("pipeline.trigger_build.pipeline: missing source_pipeline"),
				},
			},
		})
	})

	t.Run("errors when an organization rule is updated with no target_pipeline UUID", func(t *testing.T) {
		randNameOne := acctest.RandString(12)
		randNameTwo := acctest.RandString(12)
		var orr organizationRuleResourceModel

		check := resource.ComposeAggregateTestCheckFunc(
			testAccCheckOrganizationRuleExists(&orr, "buildkite_organization_rule.trigger_build_rule"),
			// Confirm the organization rule has the correct values in Buildkite's system
			testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", "TRIGGER_BUILD", "ALLOW"),
			// Check the organization rule resource's attributes are set in state
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "id"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "description"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "source_type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "source_uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "target_type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "target_uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "value"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleDestroy,
			Steps: []resource.TestStep{
				{
					Config: configAll(randNameOne, randNameTwo, "trigger_build"),
					Check:  check,
				},
				{
					Config:      configUpdateErrorNoTarget(randNameOne, randNameTwo, "trigger_build"),
					ExpectError: regexp.MustCompile("pipeline.trigger_build.pipeline: missing target_pipeline"),
				},
			},
		})
	})

	t.Run("errors when an organization rule is updated with invalid conditions", func(t *testing.T) {
		randNameOne := acctest.RandString(12)
		randNameTwo := acctest.RandString(12)
		var orr organizationRuleResourceModel

		initConditions := `
			"source.build.branch == 'develop'",
			"source.pipeline.slug == 'monorepo-core'"
  		`

		nonExistentConditions := `
			"source.build.branch == 'develop'",
			"source.pipeline.slug == 'monorepo-core'",
			"source.build.polisher includes 'monorepo'"
		`

		check := resource.ComposeAggregateTestCheckFunc(
			testAccCheckOrganizationRuleExists(&orr, "buildkite_organization_rule.trigger_build_rule"),
			// Confirm the organization rule has the correct values in Buildkite's system
			testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", "TRIGGER_BUILD", "ALLOW"),
			// Check the organization rule resource's attributes are set in state
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "id"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "description"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "source_type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "source_uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "target_type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "target_uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "value"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleDestroy,
			Steps: []resource.TestStep{
				{
					Config: configAllCustomConditions(randNameOne, randNameTwo, "trigger_build", initConditions),
					Check:  check,
				},
				{
					Config:      configAllCustomConditions(randNameOne, randNameTwo, "trigger_build", nonExistentConditions),
					ExpectError: regexp.MustCompile("pipeline.trigger_build.pipeline: conditional is invalid"),
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
			value, err := obtainValueJSON(organizationRule.Document)
			if err != nil {
				return fmt.Errorf("Error constructing sorted value JSON to store in state")
			}
			updateOrganizatonRuleReadState(orr, *organizationRule, *value)
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
