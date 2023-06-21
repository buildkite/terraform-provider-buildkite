terraform {
  required_providers {
    buildkite = {
      source  = "buildkite/buildkite"
      version = "0.19.0"
    }
  }
}

provider "buildkite" {
  organization = "testkite"
}


resource "buildkite_cluster" "test_cluster" {
  name = "test-cluster"
  description = "test cluster"
}
