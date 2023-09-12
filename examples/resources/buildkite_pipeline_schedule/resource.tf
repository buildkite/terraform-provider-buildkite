# create a pipeline
resource "buildkite_pipeline" "pipeline" {
  name       = "my pipeline"
  repository = "https://github.com/..."
}

# schedule a build at midnight every day
resource "buildkite_pipeline_schedule" "nightly" {
  pipeline_id = buildkite_pipeline.repo.id
  label       = "Nightly build"
  cronline    = "@midnight"
  branch      = buildkite_pipeline.repo.default_branch
}
