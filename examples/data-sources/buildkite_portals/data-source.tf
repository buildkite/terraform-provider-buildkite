data "buildkite_portals" "all" {}

# filter user-invokable portals
output "user_invokable_portals" {
  value = [
    for portal in data.buildkite_portals.all.portals :
    portal.name if portal.user_invokable
  ]
}
