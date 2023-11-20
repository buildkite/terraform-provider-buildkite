locals {
  repository = "git@github.com:my-org/my-repo.git"
}

data "buildkite_signed_pipeline_steps" "my-steps" {
  repository  = local.repository
  jwks_file   = "/path/to/my/jwks.json"
  jwks_key_id = "my-key"

  unsigned_steps = <<YAML
steps:
- label: ":pipeline:"
  command: buildkite-agent pipeline upload
YAML
}

resource "buildkite_pipeline" "my-pipeline" {
  name       = "my-pipeline"
  repository = local.repository
  steps      = data.buildkite_signed_pipeline_steps.my-steps.steps
}
