terraform {
  required_providers {
    buildkite = {
      source  = "buildkite/buildkite"
      version = "0.20.0"
    }
  }
}

provider "buildkite" {
  organization = "al-dev-env"
}

resource "buildkite_cluster" "test_cluster" {
  name        = "cluster-test"
  description = "test cluster"
}

#resource "buildkite_cluster_queue" "hosted_linux_small" {
#  cluster_id  = buildkite_cluster.test_cluster.id
#  key         = "hosted-linux-small"
#  description = "Terraform'd queue for Hosted Linux"
#
#  hosted_agents = {
#    instance_shape = "LINUX_ARM64_2X4"
#
#    linux = {
#      image_agent_ref = "elixir:1.17.3-slim"
#    }
#  }
#}

#resource "buildkite_cluster_queue" "self_hosted" {
#  cluster_id  = buildkite_cluster.test_cluster.id
#  key         = "self-hosted-queue"
#  description = "A self-hosted Terraform'd queue"
#}

#resource "buildkite_cluster_queue" "hosted_macos_small" {
#  cluster_id  = buildkite_cluster.test_cluster.id
#  key         = "hosted-macos-small"
#  description = "MacOS hosted agents via Terraform"
#
#  hosted_agents = {
#    instance_shape = "MACOS_M2_4X7"
#
#    mac = {
#      xcode_version = "16.2"
#    }
#  }
#}
