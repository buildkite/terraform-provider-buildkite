# Add a user as a cluster maintainer
resource "buildkite_cluster_maintainer" "user_maintainer" {
  cluster_uuid = buildkite_cluster.primary.uuid
  user_uuid    = "01234567-89ab-cdef-0123-456789abcdef"
}

# Add a team as a cluster maintainer
resource "buildkite_cluster_maintainer" "team_maintainer" {
  cluster_uuid = buildkite_cluster.primary.uuid
  team_uuid    = "01234567-89ab-cdef-0123-456789abcdef"
}