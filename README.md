# Buildkite Terraform Provider

This is the official Terraform provider for [Buildkite](https://buildkite.com).

The provider allows you to manage resources in your Buildkite organization.

Two configuration values are required:

* An API token, generated at https://buildkite.com/user/api-access-tokens. The
  token must have the `write_pipelines` REST API scope and be enabled for GraphQL
* A Buildkite organization slug, available by signing into buildkite.com and
  examining the URL: https://buildkite.com/<org-slug>

## Quick Start

- [Using the provider](https://registry.terraform.io/providers/jradtilbrook/buildkite/latest)

## Installation

This provider is hosted on the [Terraform Registry](https://registry.terraform.io/).

To use the provider, add the following terraform:

```hcl
terraform {
  required_providers {
    buildkite = {
      source = "jradtilbrook/buildkite"
      version = "0.0.15"
    }
  }
}

provider "buildkite" {
  # Configuration options
  api_token = "token" # can also be set from env: BUILDKITE_API_TOKEN
  organization = "slug" # can also be set from env: BUILDKITE_ORGANIZATION
}
```

#### Releasing a version

This repo has GitHub Actions setup that will automatically build the binary for different platforms and attach it to a
GitHub release. All that is required is to create the release in GitHub.

## License

Buildkite Terraform Provider is licensed under the MIT license.

## Local development

Contributions welcome!!
