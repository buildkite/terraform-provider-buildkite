# Maintain an organization member's role (MEMBER) and SSO requirement (REQUIRED)
resource "buildkite_organization_member" "john_doe" {
  role       = "MEMBER"
  sso        = "REQUIRED"
}

# Maintain an organization member's role (ADMIN) and SSO requirement (OPTIONAL)
resource "buildkite_organization_member" "jame_smith" {
  role       = "ADMIN"
  sso        = "OPTIONAL"
}