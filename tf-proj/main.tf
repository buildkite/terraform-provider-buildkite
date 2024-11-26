terraform {
  required_providers {
    buildkite = {
      source  = "buildkite/buildkite"
      version = "0.20.0"
    }
  }
}

provider "buildkite" {
  organization = "testkite"
}


resource "buildkite_cluster" "test_cluster" {
  name        = "cluster-test"
  description = "test cluster"
}