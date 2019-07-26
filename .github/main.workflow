workflow "Release" {
  on = "release"
  resolves = ["goreleaser"]
}

# only continue if the action is `created` - note this relies also on the workflow trigger of `release` above since
# multiple events have an action of created
action "is-created" {
  uses = "actions/bin/filter@master"
  args = "action created"
}

action "goreleaser" {
  uses = "docker://goreleaser/goreleaser"
  secrets = ["GITHUB_TOKEN"]
  args = "release"
  needs = ["is-created"]
}
