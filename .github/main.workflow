workflow "Release" {
  on = "release"
  resolves = ["goreleaser"]
}

action "goreleaser" {
  uses = "docker://goreleaser/goreleaser"
  secrets = ["GITHUB_TOKEN"]
  args = "release"
}
