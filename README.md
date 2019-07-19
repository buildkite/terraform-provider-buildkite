# Buildkite Terraform Provider

[![Build Status](https://travis-ci.com/jradtilbrook/terraform-provider-buildkite.svg?branch=master)](https://travis-ci.com/jradtilbrook/terraform-provider-buildkite)

## Installation

There are multiple ways to get a binary for this provider:

- `go get github.com/jradtilbrook/terraform-provider-buildkite`
- clone this repo and build it
- download a pre-built release from GitHub

Once you have a binary you need to make sure it's on terraform plugin search path. You can get more information from
[the terraform docs](https://www.terraform.io/docs/configuration/providers.html#third-party-plugins).

## Usage

Create a GraphQL token in Buildkite at https://buildkite.com/user/api-access-tokens/new.  
This provider requires that token and the Buildkite organisation slug in which to manage resources.

```terraform
provider "buildkite" {
    api_token = "TOKEN" # can also be set from env: BUILDKITE_API_TOKEN
    organization = "SLUG" # can also be set from env: BUILDKITE_ORGANIZATION
}

# create an agent token with an optional description
resource "buildkite_agent_token" "token" {
    description = "default agent token"
}

# create a pipeline with default upload step
resource "buildkite_pipeline" "repo1" {
    name = "repo1"
    description = "a repository pipeline"
    repository = "git@github.com:org/repo1"
}
```

### Importing existing resources

The following resources provided by this provider are:

- `buildkite_agent_token` using its GraphQL ID (not UUID)
- `buildkite_pipeline` using its GraphQL ID (not UUID)

You can import them using the standard terraform method.

## Local development

#### Releasing a version

This repo has GitHub Actions setup that will automatically build the binary for different platforms and attach it to a
GitHub release. All that is required is to create the release in GitHub.

## License

Buildkite Terraform Provider is licensed under the MIT license.
