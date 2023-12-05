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
  description = "Token with allowed IP range"
  cluster_id  = buildkite_cluster.primary.id
  allowed_ip_addresses = "10.100.1.0/28"
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
