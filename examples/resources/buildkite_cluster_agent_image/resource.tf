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

resource "buildkite_cluster_agent_image" "ruby" {
  cluster_id = buildkite_cluster.hosted.uuid
  name       = "ruby"

  # Changing dockerfile deletes and re-creates the image; in-place updates are not supported
  # Switch queues to a different image in a separate apply before replacing this image
  dockerfile = <<-EOT
    RUN apt-get update && apt-get install -y ruby-full
  EOT

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
