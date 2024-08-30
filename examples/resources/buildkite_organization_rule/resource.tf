resource "buildkite_organization_rule" "trigger_build_test_dev" {
    type = "pipeline.trigger_build.pipeline"
    value = jsonencode({
        triggering_pipeline_uuid = buildkite_pipeline.app_dev_deploy.uuid
        triggered_pipeline_uuid = buildkite_pipeline.app_test_ci.uuid
    })
}

resource "buildkite_organization_rule" "artifacts_read_test_dev" {
    type = "pipeline.artifacts_read.pipeline"
    value = jsonencode({
        target_pipeline_uuid = buildkite_pipeline.app_dev_deploy.uuid
        source_pipeline_uuid = buildkite_pipeline.app_test_ci.uuid
    })
}