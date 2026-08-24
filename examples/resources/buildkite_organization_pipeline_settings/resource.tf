# set the defaults that new pipelines in the organization are created with
resource "buildkite_organization_pipeline_settings" "settings" {
  default_branch     = "main"
  default_cluster_id = buildkite_cluster.primary.uuid

  # time jobs out after an hour, and stop any pipeline asking for more than two
  default_timeout_in_minutes = 60
  maximum_timeout_in_minutes = 120

  # expire scheduled jobs that have not started within a day
  scheduled_job_expiry_in_minutes = 1440

  # keep every pipeline private, and leave hosted agents reachable for debugging
  public_pipeline_creation_enabled      = false
  hosted_agents_terminal_access_enabled = true
}

resource "buildkite_cluster" "primary" {
  name = "Primary"
}
