resource "buildkite_team" "everyone" {
  name                = "Everyone"
  privacy             = "VISIBLE"
  default_team        = false
  default_member_role = "MEMBER"
}
