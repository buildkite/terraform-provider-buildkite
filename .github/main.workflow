workflow "Release" {
  on = "push"
  resolves = ["goreleaser"]
}

action "goreleaser" {
  uses = "docker://goreleaser/goreleaser"
  secrets = ["GITHUB_TOKEN"]
  args = "release"
}
