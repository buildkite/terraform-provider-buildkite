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

resource "buildkite_cluster_queue" "self_hosted" {
  cluster_id  = buildkite_cluster.test_cluster.id
  key         = "self-hosted-queue"
  description = "A self-hosted Terraform-managed queue"
}

resource "buildkite_cluster_queue" "hosted_linux_small" {
  cluster_id  = buildkite_cluster.test_cluster.id
  key         = "hosted-linux-small"
  description = "Terraform queue for Hosted Linux"

  hosted_agents = {
    instance_shape = "LINUX_AMD64_2X4"

    linux = {
      agent_image_ref = "ubuntu:24.04"
    }
  }
}

resource "buildkite_cluster_queue" "hosted_macos_medium" {
  cluster_id  = buildkite_cluster.test_cluster.id
  key         = "hosted-macos-medium"
  description = "MacOS hosted agents via Terraform"

  hosted_agents = {
    instance_shape = "MACOS_ARM64_M4_6X28"

    mac = {
      macos_version = "SEQUOIA"
      xcode_version = "16.3"
    }
  }
}


resource "buildkite_cluster_secret" "my_secret" {
  cluster_id  = buildkite_cluster.test_cluster.uuid
  key         = "MY_SECRET"
  value       = "secret-value"
  description = "Test secret was created by Terraform"
  
  policy = <<-EOT
    - pipeline_slug: my-pipeline
      build_branch: main
  EOT
}
