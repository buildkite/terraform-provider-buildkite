# Buildkite Terraform Provider

This is the official Terraform provider for Buildkite.

The provider allows you to manage resources in your Buildkite organization.

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

Contributions are welcome.

If you wish to work on the provider, you'll first need Go installed on your machine (version 1.12+ is required). Dependencies are managed via gomodules and installed automatically as required.

To compile the provider:

    make build

To run local tests that don't require any network access:

    make test

Buildkite has two APIs: REST and GraphQL. New resources should use the GraphQL API where possible, but can fallback to the REST API for resouces or properties not yet supported by GraphQL.
