workflow "Build release" {
  on = "release"
  resolves = ["Release darwin/amd64", "ngs/go-release.action@v1.0.1", "ngs/go-release.action@v1.0.1-1"]
}

action "Release darwin/amd64" {
  uses = "ngs/go-release.action@v1.0.1"
  secrets = ["GITHUB_TOKEN"]
  env = {
    GOOS = "darwin"
    GOARCH = "amd64\n"
  }
}

action "ngs/go-release.action@v1.0.1" {
  uses = "ngs/go-release.action@v1.0.1"
  secrets = ["GITHUB_TOKEN"]
  env = {
    GOOS = "windows"
    GOARCH = "amd64"
  }
}

action "ngs/go-release.action@v1.0.1-1" {
  uses = "ngs/go-release.action@v1.0.1"
  secrets = ["GITHUB_TOKEN"]
  env = {
    GOOS = "linux"
    GOARCH = "amd64"
  }
}
