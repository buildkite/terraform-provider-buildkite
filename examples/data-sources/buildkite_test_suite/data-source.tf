data "buildkite_test_suite" "application" {
  slug = "application"
}

data "buildkite_team" "team" {
  slug = "Everyone"
}

resource "buildkite_test_suite_team" "everyone" {
  team_id      = data.buildkite_team.team.id
  suite_id     = data.buildkite_test_suite.application.id
  access_level = "MANAGE_AND_READ"
}
