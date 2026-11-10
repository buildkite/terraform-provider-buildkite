resource "buildkite_cluster" "hosted" {
  name = "Hosted cluster"
}

# A cluster can hold agent images once it has a hosted queue
resource "buildkite_cluster_queue" "default" {
  cluster_id = buildkite_cluster.hosted.id
  key        = "default"

  hosted_agents = {
    instance_shape = "LINUX_AMD64_2X4"
  }
}

locals {
  ruby_dockerfile = <<-EOT
    RUN apt-get update && apt-get install -y ruby-full
  EOT
}

resource "buildkite_cluster_agent_image" "ruby" {
  cluster_id = buildkite_cluster.hosted.uuid
  # Each Dockerfile revision needs a unique name so both images can exist during rollout
  # To retry a failed or timed-out build, use a new name or destroy the retained image first
  name       = "ruby-${substr(sha256(local.ruby_dockerfile), 0, 12)}"
  dockerfile = local.ruby_dockerfile

  lifecycle {
    create_before_destroy = true
  }

  depends_on = [buildkite_cluster_queue.default]
}

# Run a hosted queue's agents on the image
resource "buildkite_cluster_queue" "ruby" {
  cluster_id = buildkite_cluster.hosted.id
  key        = "ruby"

  hosted_agents = {
    instance_shape = "LINUX_AMD64_2X4"

    linux = {
      agent_image_ref = buildkite_cluster_agent_image.ruby.image_ref
    }
  }
}
