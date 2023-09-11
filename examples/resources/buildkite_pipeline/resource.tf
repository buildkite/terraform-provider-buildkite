# minimal repository
resource "buildkite_pipeline" "pipeline" {
  name       = "repo"
  repository = "git@github.com:org/repo"
}

# with github provider settings
resource "buildkite_pipeline" "pipeline" {
  name       = "repo"
  repository = "git@github.com:org/repo"

  provider_settings {
    build_branches      = false
    build_tags          = true
    build_pull_requests = false
    trigger_mode        = "code"
  }
}
