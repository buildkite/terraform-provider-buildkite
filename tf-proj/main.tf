terraform {
  required_providers {
    buildkite = {
      source  = "buildkite/buildkite"
      version = "0.20.0"
    }
  }
}

provider "buildkite" {}

resource "buildkite_pipeline" "test-signing-pipeline" {
  name       = "test-signing-pipeline"
  repository = "https://github.com/moskyb/bash-example"

  steps = <<EOF
steps:
  - label: ":bash:"
    command: "echo 'i love signed pipelines!'"
EOF

  signing_jwks   = file("/Users/ben/signing.json")
  signing_key_id = "eduardo"
}
