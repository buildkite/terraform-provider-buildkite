workflow "temp" {
  on "push"
  resolves = [
    "internal release",
  ]
}

action "internal release" {
  uses = "./.github/action-release/"
  "secrets" = ["GITHUB_TOKEN"]
  env = {
    GOARCH = "amd64"
    GOOS = "linux"
  }
}

workflow "Build release" {
  on = "release"
  resolves = [
    "Release darwin/amd64",
    "Release windows/amd64",
    "Release linux/amd64",
  ]
}

action "Release darwin/amd64" {
  uses = "jradtilbrook/go-release.action@v1.0.1-dev"
  secrets = ["GITHUB_TOKEN"]
  env = {
    GO111MODULE = "on"
    GOARCH = "amd64"
    GOOS = "darwin"
  }
}

action "Release windows/amd64" {
  uses = "jradtilbrook/go-release.action@v1.0.1-dev"
  secrets = ["GITHUB_TOKEN"]
  env = {
    GO111MODULE = "on"
    GOARCH = "amd64"
    GOOS = "windows"
  }
}

action "Release linux/amd64" {
  uses = "jradtilbrook/go-release.action@v1.0.1-dev"
  secrets = ["GITHUB_TOKEN"]
  env = {
    GO111MODULE = "on"
    GOARCH = "amd64"
    GOOS = "linux"
  }
}
