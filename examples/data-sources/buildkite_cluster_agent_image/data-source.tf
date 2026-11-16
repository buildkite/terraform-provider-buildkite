data "buildkite_cluster" "hosted" {
  name = "Hosted agents"
}

# Look up an existing custom image by its exact name within the cluster.
data "buildkite_cluster_agent_image" "by_name" {
  cluster_id = data.buildkite_cluster.hosted.uuid
  name       = "ruby"
}

# Alternatively, look up an image by its ID from the agent-images REST API.
# Configure either id or name, not both.
data "buildkite_cluster_agent_image" "by_id" {
  cluster_id = data.buildkite_cluster.hosted.uuid
  id         = "snb5nqcik8ge2"
}

# Use an image that has finished building (status = READY).
resource "buildkite_cluster_queue" "ruby" {
  cluster_id = data.buildkite_cluster.hosted.id
  key        = "ruby"
  hosted_agents = {
    instance_shape = "LINUX_AMD64_2X4"
    linux = {
      agent_image_ref = data.buildkite_cluster_agent_image.by_name.image_ref
    }
  }
}
