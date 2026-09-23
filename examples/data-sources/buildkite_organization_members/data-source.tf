data "buildkite_organization_members" "members" {}

# only the administrators that are in a given team
data "buildkite_organization_members" "platform_admins" {
  team = "platform"
  role = "ADMIN"
}
