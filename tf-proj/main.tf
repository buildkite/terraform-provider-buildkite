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
  name = "cluster-test"
  description = "test cluster"
}

resource "buildkite_cluster_queue" "test_queue" {
  cluster_id = buildkite_cluster.test_cluster.id
  key = "testing"
  description = "testing queue create"
}
