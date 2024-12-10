data "buildkite_organization_member" "a_smith" {
  email = "a.smith@company.com"
}

resource "buildkite_team" "developers" {
  name                = "Developers"
  privacy             = "VISIBLE"
  default_team        = false
  default_member_role = "MEMBER"
}

resource "buildkite_team_member" "developers_a_smith" {
  team_id = buildkite_team.developers.id
  user_id = data.buildkite_organization_member.a_smith.id
  role    = "MEMBER"
}
