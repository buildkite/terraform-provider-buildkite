data "buildkite_team" "team_dev" {
  id = buildkite_team.team_dev.id
}

data "buildkite_team" "team" {
  slug = "Everyone"
}