terraform {
  required_providers {
    buildkite = {
      source = "buildkite/buildkite"
    }
    github = {
      source  = "integrations/github"
      version = "~> 6.0"
    }
  }
}

provider "buildkite" {
  organization = "matthew-terraform-testing-org"
  timeouts = {
    create = "60s"
    read   = "60s"
    update = "60s"
    delete = "60s"
  }
}

provider "github" {
  owner = "matthewborden"
}

variable "repository" {
  type    = string
  default = "https://github.com/matthewborden/test.git"
}

resource "buildkite_cluster" "test_cluster" {
  name        = "webhook-qa-cluster"
  description = "Cluster for QA testing webhook resource"
}

resource "buildkite_pipeline" "test_pipeline" {
  name       = "webhook-qa-pipeline"
  cluster_id = buildkite_cluster.test_cluster.id
  repository = var.repository
}

resource "buildkite_pipeline_webhook" "test_webhook" {
  pipeline_id = buildkite_pipeline.test_pipeline.id
  repository  = buildkite_pipeline.test_pipeline.repository
}
