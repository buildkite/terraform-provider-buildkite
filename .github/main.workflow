workflow "Release" {
  on = "release"
  resolves = ["goreleaser"]
}

# only continue if the action is `published` - note this relies also on the workflow trigger of `release` above since
# multiple events have an action of published
action "is-published" {
  uses = "actions/bin/filter@master"
  args = "action published"
}

action "goreleaser" {
  uses = "docker://goreleaser/goreleaser"
  secrets = ["GITHUB_TOKEN"]
  args = "release"
  needs = ["is-published"]
}
