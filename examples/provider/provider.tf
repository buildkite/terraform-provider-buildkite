terraform {
  required_version = ">= 1.0"

  required_providers {
    buildkite = {
      source  = "buildkite/buildkite"
      version = "~> 1.0"
    }
  }
}

# Configure the Buildkite Provider
provider "buildkite" {
  organization = "buildkite"
  # Use the `BUILDKITE_API_TOKEN` environment variable so the token is not committed
  # api_token = ""
}

# Add a pipeline
resource "buildkite_pipeline" "pipeline" {
  # ...
}
