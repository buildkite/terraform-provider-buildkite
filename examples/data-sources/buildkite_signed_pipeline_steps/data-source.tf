# JWKS via file
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

# JWKS (JSON) string via Hashicorp Vault secret
data "vault_generic_secret" "pipeline-jwks" {
  path = "secret/pipelines/jwks"
}

data "buildkite_signed_pipeline_steps" "signed-steps" {
  repository = "git@github.com:my-org/my-repo.git"
  jwks       = data.vault_generic_secret.pipeline-jwks.data["private_key"]

  unsigned_steps = <<YAML
steps:
- label: ":pipeline:"
  command: buildkite-agent pipeline upload
YAML
}

resource "buildkite_pipeline" "signed-pipeline" {
  name       = "my-signed-pipeline"
  repository = "git@github.com:my-org/my-repo.git"
  steps      = data.buildkite_signed_pipeline_steps.signed-steps.steps
}
