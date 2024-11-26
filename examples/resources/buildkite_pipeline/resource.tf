# minimal repository
resource "buildkite_pipeline" "pipeline" {
  name       = "repo"
  repository = "git@github.com:my-org/my-repo"
}

# with github provider settings
resource "buildkite_pipeline" "pipeline" {
  color      = "#000000"
  emoji      = ":buildkite:"
  name       = "repo"
  repository = "git@github.com:my-org/my-repo"

  provider_settings = {
    build_branches      = false
    build_tags          = true
    build_pull_requests = false
    trigger_mode        = "code"
  }
}

# signed pipeline
locals {
  repository = "git@github.com:my-org/my-repo.git"
}

data "buildkite_signed_pipeline_steps" "signed-steps" {
  repository  = local.repository
  jwks_file   = "/path/to/my/jwks-private.json"
  jwks_key_id = "my-key-id"

  unsigned_steps = <<YAML
steps:
- label: ":pipeline:"
  command: buildkite-agent pipeline upload
YAML
}

resource "buildkite_pipeline" "signed-pipeline" {
  name       = "my-signed-pipeline"
  repository = local.repository
  steps      = data.buildkite_signed_pipeline_steps.signed-steps.steps
}
