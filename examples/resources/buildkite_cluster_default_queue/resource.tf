# create a cluster
resource "buildkite_cluster" "primary" {
  name        = "Primary cluster"
  description = "Runs the monolith build and deploy"
  emoji       = "🚀"
  color       = "#bada55"
}

resource "buildkite_cluster_queue" "default" {
  cluster_id = buildkite_cluster.primary.id
  key        = "default"
}

resource "buildkite_cluster_default_queue" "primary" {
  cluster_id = buildkite_cluster.primary.id
  queue_id   = buildkite_cluster_queue.default.id
}
