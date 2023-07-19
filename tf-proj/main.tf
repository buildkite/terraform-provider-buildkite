terraform {
  required_providers {
    buildkite = {
      source  = "buildkite/buildkite"
      version = "0.20.0"
    }
  }
}

resource "buildkite_pipeline" "tf-proj-1" {
  name       = "Testing TF-PROJ-1"
  repository = "git@github.com:lizrabuya/tf-projects.git" 
  default_branch = "main"
 
}

resource "buildkite_pipeline_schedule" "tf-proj-1_nightly" {
  pipeline_id = buildkite_pipeline.tf-proj-1.id
  label       = "tf-proj-1-Nightly build"
  cronline    = "@midnight"
  branch      = buildkite_pipeline.tf-proj-1.default_branch
  message = "Nightly build"
}