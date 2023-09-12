resource "buildkite_team" "everyone" {
  name                = "Everyone"
  privacy             = "VISIBLE"
  default_team        = false
  default_member_role = "MEMBER"
}

resource "buildkite_team_member" "a_smith" {
  team_id = buildkite_team.everyone.id
  user_id = "VGVhbU1lbWJlci0tLTVlZDEyMmY2LTM2NjQtNDI1MS04YzMwLTc4NjRiMDdiZDQ4Zg=="
  role    = "MEMBER"
}
