# all unarchived pipelines in a cluster whose name matches "deploy"
data "buildkite_pipelines" "deploys" {
  cluster_id = buildkite_cluster.primary.id
  archived   = false
  search     = "deploy"
}
