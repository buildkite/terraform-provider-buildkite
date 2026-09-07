# create a registry, owned by the "platform" team
resource "buildkite_registry" "gems" {
  name      = "gems"
  ecosystem = "ruby"
  team_ids  = [buildkite_team.platform.uuid]
}

# give the "everyone" team read only access to the "gems" registry
resource "buildkite_team_registry" "gems_everyone" {
  registry_id  = buildkite_registry.gems.id
  team_id      = buildkite_team.everyone.id
  access_level = "READ_ONLY"
}
