resource "buildkite_pipeline" "pipeline" {
  name       = "my pipeline"
  repository = "https://github.com/..."
}

resource "buildkite_team" "team" {
  name                = "Everyone"
  privacy             = "VISIBLE"
  default_team        = false
  default_member_role = "MEMBER"
}

# allow everyone in the "Everyone" team read-only access to pipeline
resource "buildkite_pipeline_team" "pipeline_team" {
  pipeline_id  = buildkite_pipeline.pipeline.id
  team_id      = buildkite_team.team.id
  access_level = "READ_ONLY"
}
