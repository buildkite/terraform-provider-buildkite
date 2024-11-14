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
