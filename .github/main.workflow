workflow "Build release" {
  on = "release"
  resolves = [
    "Release darwin/amd64",
    "Release windows/amd64",
    "Release linux/amd64",
  ]
}

action "Release darwin/amd64" {
  uses = "ngs/go-release.action@v1.0.1"
  secrets = ["GITHUB_TOKEN"]
  env = {
    GOOS = "darwin"
    GOARCH = "amd64\n"
  }
}

action "Release windows/amd64" {
  uses = "ngs/go-release.action@v1.0.1"
  secrets = ["GITHUB_TOKEN"]
  env = {
    GOOS = "windows"
    GOARCH = "amd64"
  }
}

action "Release linux/amd64" {
  uses = "ngs/go-release.action@v1.0.1"
  secrets = ["GITHUB_TOKEN"]
  env = {
    GOOS = "linux"
    GOARCH = "amd64"
  }
}
