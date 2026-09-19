# Adopt an existing member. No invitation is sent by default.
resource "buildkite_organization_membership" "jane" {
  email    = "jane@example.com"
  role     = "ADMIN"
  sso_mode = "REQUIRED"

  # Keep Jane as a MEMBER on destroy rather than removing her from the organization.
  downgrade_on_destroy = true
}

# Invite a new member, or adopt them if they already belong to the organization.
# Terraform does not wait for acceptance. Refresh/apply again after they accept.
resource "buildkite_organization_membership" "new_user" {
  email           = "new-user@example.com"
  role            = "MEMBER"
  sso_mode        = "REQUIRED"
  send_invitation = true
}

# Team membership stays separate. This example uses an already-active user.
# Do not reference a pending invitation's null user_id in a team membership.
data "buildkite_team" "engineering" {
  slug = "engineering"
}

resource "buildkite_team_member" "jane" {
  team_id = data.buildkite_team.engineering.id
  user_id = buildkite_organization_membership.jane.user_id
  role    = "MEMBER"
}
