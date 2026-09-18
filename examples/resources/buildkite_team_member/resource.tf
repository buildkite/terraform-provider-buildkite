resource "buildkite_team" "everyone" {
  name                = "Everyone"
  privacy             = "VISIBLE"
  default_team        = false
  default_member_role = "MEMBER"
}

resource "buildkite_team_member" "a_smith" {
  team_id = buildkite_team.everyone.id
  user_id = "VXNlci0tLTkyZjlkMmJhLWJkM2QtNDM1Yi05N2NmLWQ3NmMwMDI0NWU3ZQ=="
  role    = "MEMBER"
}
