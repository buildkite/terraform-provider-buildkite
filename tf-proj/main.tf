terraform {
  required_providers {
    buildkite = {
      source  = "buildkite/buildkite"
      version = "0.12.1"
    }
  }
}

provider "buildkite" {
}


resource "buildkite_pipeline" "test_new" {
  name       = "Testing Retention"
  repository = "https://github.com/buildkite/terraform-provider-buildkite.git"

  steps = ""

  build_retention_enabled = true
  build_retention_number = 25
  build_retention_period = "DAYS_30"
}

resource "buildkite_pipeline_schedule" "test_scheduled" {
  pipeline_id = buildkite_pipeline.test_new.id
  label       = "Test Scheduled"
  cronline = "0 0 * * *"
  branch = "master"
  commit = "HEAD"
  message = "Test Scheduled Builds in Terraform"
}

output "badge_url" {
  value = buildkite_pipeline.test_new.badge_url
}
