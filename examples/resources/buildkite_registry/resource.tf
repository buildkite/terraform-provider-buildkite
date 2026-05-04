resource "buildkite_registry" "example" {
  name        = "example"
  description = "super cool ruby registry"
  ecosystem   = "ruby"
  emoji       = ":ruby:"
  color       = "#ff0000"
  team_ids = [
    buildkite_team.frontend_team.uuid,
    buildkite_team.backend_team.uuid
  ]
  oidc_policy = <<YAML
- iss: https://agent.buildkite.com
  scopes:
    - read_packages
  claims:
    build_branch: main
YAML
}
