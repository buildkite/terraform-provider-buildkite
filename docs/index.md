# Buildkite Provider

This provider can be used to manage resources on [buildkite.com](https://buildkite.com) via Terraform.

Two configuration values are required:

* An API token, generated at https://buildkite.com/user/api-access-tokens. The
  token must have the `write_pipelines` REST API scope and be enabled for GraphQL
* A Buildkite organization slug, available by signing into buildkite.com and
  examining the URL: https://buildkite.com/<org-slug>

## Example Usage

```hcl
terraform {
  required_providers {
    buildkite = {
      source = "buildkite/buildkite"
      version = "0.0.16"
    }
  }
}

provider "buildkite" {
    api_token = "token" # can also be set from env: BUILDKITE_API_TOKEN
    organization = "slug" # can also be set from env: BUILDKITE_ORGANIZATION
}
```

## Argument Reference

* `api_token` - (Required) This is the Buildkite API Access Token. It must be provided but can also be sourced from the `BUILDKITE_API_TOKEN` environment variable.
* `organization` - (Required) This is the Buildkite organization slug. It must be provided, but can also be sourced from the `BUILDKITE_ORGANIZATION` environment variable. The token requires GraphQL access and the `write_pipelines` scope.
