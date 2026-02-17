# create a pipeline
resource "buildkite_pipeline" "pipeline" {
  name       = "my pipeline"
  repository = "https://github.com/my-org/my-repo.git"
}

# create a webhook to automatically trigger builds on push
resource "buildkite_pipeline_webhook" "webhook" {
  pipeline_id    = buildkite_pipeline.pipeline.id
  repository     = buildkite_pipeline.pipeline.repository
}
