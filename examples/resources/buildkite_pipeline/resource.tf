# minimal repository
data "buildkite_cluster" "default" {
  name = "Default cluster"
}
resource "buildkite_pipeline" "pipeline" {
  name       = "repo"
  repository = "git@github.com:my-org/my-repo"
  cluster_id = data.buildkite_cluster.default.id
}

# with github provider settings
data "buildkite_cluster" "default" {
  name = "Default cluster"
}
resource "buildkite_pipeline" "pipeline" {
  color      = "#000000"
  emoji      = ":buildkite:"
  name       = "repo"
  repository = "git@github.com:my-org/my-repo"
  cluster_id = data.buildkite_cluster.default.id

  provider_settings = {
    build_branches      = false
    build_tags          = true
    build_pull_requests = false
    trigger_mode        = "code"
  }
}

# signed pipeline
data "buildkite_cluster" "default" {
  name = "Default cluster"
}
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
  cluster_id = data.buildkite_cluster.default.id
  steps      = data.buildkite_signed_pipeline_steps.signed-steps.steps
}


# Advanced example using Github provider to create repository webhook for Buildkite pipeline

terraform {
  required_providers {
    github = {
      source  = "integrations/github"
      version = "~> 6.0"
    }
    buildkite = {
      source  = "buildkite/buildkite"
      version = "~> 1.16"
    }
  }
}

provider "github" {}
provider "buildkite" {
  organization = "my-org-slug"
}

resource "github_repository" "test_repo" {
  name        = "example"
  description = "My awesome codebase"

  visibility = "private"
}

data "buildkite_cluster" "default" {
  name = "Default cluster"
}

resource "buildkite_pipeline" "pipeline" {
  name       = "repo"
  repository = github_repository.test_repo.ssh_clone_url
  cluster_id = data.buildkite_cluster.default.id
  depends_on = [github_repository.test_repo]
}

resource "github_repository_webhook" "my_webhook" {
  repository = github_repository.test_repo.name

  configuration {
    url          = buildkite_pipeline.pipeline.webhook_url
    content_type = "application/json"
    insecure_ssl = false
  }

  events = ["deployment", "pull_request", "push"]

  depends_on = [buildkite_pipeline.pipeline]
}

