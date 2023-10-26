terraform {
  required_providers {
    buildkite = {
      source  = "buildkite/buildkite"
      version = "0.20.0"
    }
  }
}

provider "buildkite" {}
variable "thing_to_echo" {
  type    = string
  default = "hello world"
}

resource "buildkite_pipeline" "test-signing-pipeline" {
  name       = "test-signing-pipeline"
  repository = "https://github.com/moskyb/bash-example"

  signed_steps_input = <<EOF
  steps:
    - label: ":bash:"
      command: "echo '${var.thing_to_echo}'"
    - label: gidday
      command: "echo 'this is another cool step!'"
    - command: "echo 'and one more'"
  EOF

  signing_jwks   = file("/Users/ben/signing.json")
  signing_key_id = "harold"
}
