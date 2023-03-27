terraform {
  required_providers {
    buildkite = {
      source  = "buildkite/buildkite"
      version = "0.11.1"
    }
  }
}

provider "buildkite" {
}


resource "buildkite_pipeline" "test" {
  name       = "Test 1"
  repository = "https://github.com/buildkite/terraform-provider-buildkite.git"

  steps = ""
}

resource "buildkite_organization_settings" "test_settings" {
  allowed_api_ip_addresses = ["0.0.0.0/0"]
}

resource "buildkite_pipeline_schedule" "foo" {
  pipeline_id = buildkite_pipeline.test.id
  cronline    = "0 *  * * *"
  label       = "My schedule"
  branch      = "master"
}

output "badge_url" {
  value = buildkite_pipeline.test.badge_url
}