# Creates a TRIGGER_BUILD organization rule with required attributes
resource "buildkite_organization_rule" "trigger_build_test_dev" {
  type = "pipeline.trigger_build.pipeline"
  value = jsonencode({
    source_pipeline = buildkite_pipeline.app_dev_deploy.uuid
    target_pipeline = buildkite_pipeline.app_test_ci.uuid
  })
}

# Creates a ARTIFACTS_READ organization rule with an optional description
resource "buildkite_organization_rule" "artifacts_read_test_dev" {
  type        = "pipeline.artifacts_read.pipeline"
  description = "A rule to allow artifact reads by app_test_ci to app_dev_deploy"
  value = jsonencode({
    source_pipeline = buildkite_pipeline.app_test_ci.uuid
    target_pipeline = buildkite_pipeline.app_dev_deploy.uuid
  })
}

# Creates a TRIGGER_BUILD organization rule with an optional description and conditions
resource "buildkite_organization_rule" "trigger_build_test_dev_cond" {
  type        = "pipeline.trigger_build.pipeline"
  description = "A rule to allow app_dev_deploy to trigger app_test_ci builds with conditions"
  value = jsonencode({
    source_pipeline = buildkite_pipeline.app_dev_deploy.uuid
    target_pipeline = buildkite_pipeline.app_test_ci.uuid
    conditions = [
      "source.build.creator.teams includes 'deploy'",
      "source.build.branch == 'main'"
    ]
  })
}