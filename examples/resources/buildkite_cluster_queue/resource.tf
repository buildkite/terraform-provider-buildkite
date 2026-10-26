# create a cluster
resource "buildkite_cluster" "primary" {
  name        = "Primary cluster"
  description = "Runs the monolith build and deploy"
  emoji       = "🚀"
  color       = "#bada55"
}

resource "buildkite_pipeline" "monolith" {
  name       = "Monolith"
  repository = "https://github.com/..."
  cluster_id = buildkite_cluster.primary.id
}

# create a queue to put pipeline builds in
resource "buildkite_cluster_queue" "default" {
  cluster_id = buildkite_cluster.primary.id
  key        = "default"
  # Pause dispatch after create
  dispatch_paused = true
}

# create a self-hosted queue whose jobs check out GitHub repositories with a
# short-lived Buildkite-issued GitHub App token instead of a long-lived credential
resource "buildkite_cluster_queue" "github_app_token" {
  cluster_id           = buildkite_cluster.primary.id
  key                  = "github-app-token"
  use_github_app_token = true
}

# create a hosted agent queue with macos agents
resource "buildkite_cluster_queue" "hosted_agents_macos" {
  cluster_id = buildkite_cluster.primary.id
  key        = "hosted-agents-macos"

  # Pause dispatch after create
  dispatch_paused = true

  hosted_agents = {
    instance_shape = "MACOS_ARM64_M4_6X28"
  }
}

# create a hosted agent queue with linux agents
resource "buildkite_cluster_queue" "hosted_agents_linux" {
  cluster_id = buildkite_cluster.primary.id
  key        = "hosted-agents-linux"

  # Pause dispatch after create
  dispatch_paused = true

  hosted_agents = {
    instance_shape = "LINUX_AMD64_2X4"

    linux = {
      agent_image_ref = "ubuntu:24.04"
    }
  }
}
