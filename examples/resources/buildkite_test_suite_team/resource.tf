# create a test suite
resource "buildkite_test_suite" "main" {
  name           = "main"
  default_branch = "main"
  team_owner_id  = "VGVhbU1lbWJlci0tLTVlZDEyMmY2LTM2NjQtNDI1MS04YzMwLTc4NjRiMDdiZDQ4Zg=="
}

# give the "everyone" team manage access to the "main" test suite
resource "buildkite_test_suite_team" "main_everyone" {
  test_suite_id = buildkite_test_suite.main.id
  team_id       = "VGVhbS0tLWU1YjQyMDQyLTUzN2QtNDZjNi04MjY0LTliZjFkMzkyYjZkNQ=="
  access_level  = "MANAGE_AND_READ"
}
