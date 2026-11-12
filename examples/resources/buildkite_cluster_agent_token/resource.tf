# create a cluster
resource "buildkite_cluster" "primary" {
  name        = "Primary cluster"
  description = "Runs the monolith build and deploy"
  emoji       = "🚀"
  color       = "#bada55"
}

# create an agent token for the cluster
resource "buildkite_cluster_agent_token" "default" {
  description = "Default cluster token"
  cluster_id  = buildkite_cluster.primary.id
}

resource "buildkite_cluster_agent_token" "ip_limited_token" {
  description          = "Token with allowed IP range"
  cluster_id           = buildkite_cluster.primary.id
  allowed_ip_addresses = ["10.100.1.0/28"]
}

resource "buildkite_cluster_agent_token" "restricted_token" {
  description = "Agent token without pipeline upload access"
  cluster_id  = buildkite_cluster.primary.id
  permissions = ["run_jobs", "upload_artifacts", "download_artifacts"]
}

resource "buildkite_pipeline" "monolith" {
  name       = "Monolith"
  repository = "https://github.com/..."
  cluster_id = buildkite_cluster.primary.id
}

resource "buildkite_cluster_queue" "default" {
  cluster_id = buildkite_cluster.primary.id
  key        = "default"
}

resource "buildkite_cluster_agent_token" "queue_bound_token" {
  description         = "Agent token restricted to the default queue"
  cluster_id          = buildkite_cluster.primary.id
  permissions         = ["run_jobs", "upload_artifacts", "download_artifacts"]
  allow_all_queues    = false
  allowed_queue_uuids = [buildkite_cluster_queue.default.uuid]
}

resource "buildkite_cluster_agent_token" "no_queue_access" {
  description         = "Agent token without queue access"
  cluster_id          = buildkite_cluster.primary.id
  permissions         = ["upload_steps"]
  allow_all_queues    = false
  allowed_queue_uuids = []
}

resource "buildkite_cluster_agent_token" "expiring_token" {
  description = "Token that expires"
  cluster_id  = buildkite_cluster.primary.id
  expires_at  = "2030-01-01T00:00:00Z"
}
