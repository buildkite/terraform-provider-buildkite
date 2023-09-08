# Find the "default" cluster
data "buildkite_cluster" "default" {
  name = "default"
}

# Assign a pipeline to that cluster
resource "buildkite_pipeline" "terraform-provider-buildkite" {
  name       = "terraform-provider-buildkite"
  repository = "git@github.com:buildkite/terraform-provider-buildkite.git"
  cluster_id = data.buildkite_cluster.default.id
}
