# Add a user as a cluster maintainer
resource "buildkite_cluster_maintainer" "user_maintainer" {
  cluster_id = buildkite_cluster.primary.id
  user_id    = "01234567-89ab-cdef-0123-456789abcdef"
}

# Add a team as a cluster maintainer
resource "buildkite_cluster_maintainer" "team_maintainer" {
  cluster_id = buildkite_cluster.primary.id
  team_id    = "01234567-89ab-cdef-0123-456789abcdef"
}