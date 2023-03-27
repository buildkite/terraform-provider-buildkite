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