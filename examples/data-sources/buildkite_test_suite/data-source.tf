data "buildkite_test_suite" "application" {
  slug = "application"
}

data "buildkite_team" "team" {
  slug = "Everyone"
}

resource "buildkite_test_suite_team" "everyone" {
  test_suite_id = data.buildkite_test_suite.application.id
  team_id       = data.buildkite_team.team.id
  access_level  = "MANAGE_AND_READ"
}
