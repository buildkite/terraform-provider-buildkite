# Buildkite Terraform Provider

[![Build Status](https://travis-ci.com/jradtilbrook/terraform-provider-buildkite.svg?branch=master)](https://travis-ci.com/jradtilbrook/terraform-provider-buildkite)

This is a Terraform provider for Buildkite.

The provider allows you to manage resources in your Buildkite organization. Current supported resources are:

- agent tokens
- pipelines

## Installation

There are multiple ways to get a binary for this provider:

- download a pre-built release from GitHub _**recommended**_
- clone this repo and build it
- `go get github.com/jradtilbrook/terraform-provider-buildkite`

*Note*: The last 2 options will not include the version information so are not recommended

Once you have a binary you need to make sure it's on terraform plugin search path. You can get more information from
[the terraform docs](https://www.terraform.io/docs/configuration/providers.html#third-party-plugins).

## Usage

Create a API Access Tokens in Buildkite at https://buildkite.com/user/api-access-tokens/new.  
This provider requires the token and the Buildkite organization slug in which to manage resources.

```terraform
# configure the Buildkite provider
provider "buildkite" {
    api_token = "token" # can also be set from env: BUILDKITE_API_TOKEN
    organization = "slug" # can also be set from env: BUILDKITE_ORGANIZATION
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
    steps = "steps:\n  - command: \"buildkite-agent pipeline upload\"\n    label: \":pipeline:\""
}

# create a pipeline with default upload step and assign team access
resource "buildkite_pipeline" "repo1" {
    name = "repo1"
    description = "a repository pipeline"
    repository = "git@github.com:org/repo1"
    steps = "steps:\n  - command: \"buildkite-agent pipeline upload\"\n    label: \":pipeline:\""

    team {
        slug = "everyone"
        access_level = "READ_ONLY"
    }
    team {
        slug = "developers"
        access_level = "BUILD_AND_READ"
    }
    team {
        slug = "admins"
        access_level = "MANAGE_BUILD_AND_READ"
    }
}
```

#### `provider` Argument reference

The following arguments are supported on the provider block:

- `api_token` - (Required) This is the Buildkite API Access Token. It must be provided but can also be sourced from the
  `BUILDKITE_API_TOKEN` environment variable.
- `organization` - (Required) This is the Buildkite organization slug. It must be provided, but can also be sourced from
  the `BUILDKITE_ORGANIZATION` environment variable. The token requires GraphQL access and the `write_pipelines` scope.  

### `buildkite_agent_token` resource

This resource allows you to create and manage agent tokens.

#### Example

```terraform
resource "buildkite_agent_token" "fleet" {
    description = "token used by build fleet"
}
```

#### Argument reference

- `description` - (Optional) This is the description of the agent token.

#### Attribute reference

- `token` - The value of the created agent token.
- `uuid` - The UUID of the token.

### `buildkite_pipeline` resource

This resource allows you to create pipelines for repositories.

#### Example

```terraform
# in ./steps.yml:
# steps:
#   - label: ':pipeline:'
#     command: buildkite-agent upload
        
resource "buildkite_pipeline" "repo2" {
    name = "repo2"
    repository = "git@github.com:org/repo2"
    steps = file("./steps.yml")

    team {
        slug = "everyone"
        access_level = "READ_ONLY"
    }
}
```

#### Argument reference

- `name` - (Required) The name of the pipeline.
- `description` - (Optional) A description of the pipeline.
- `repository` - (Required) The git URL of the repository.
- `steps` - (Required) The string YAML steps to run the pipeline.
- `team` - (Optional) Set team access for the pipeline. Can be specified multiple times for each team. Note that non-admin users may receive a buildkite permission error if trying to create a pipeline without at least one team. 

The `team` block supports:

- `slug` - (Required) The buildkite slug of the team.
- `access_level` - (Required) The level of access to grant. Must be one of `READ_ONLY`, `BUILD_AND_READ` or `MANAGE_BUILD_AND_READ`.

#### Attribute reference

- `webhook_url` - The Buildkite webhook URL to configure on the repository to trigger builds on this pipeline.
- `slug` - The slug of the created pipeline.

### Importing existing resources

The following resources support importing:

- `buildkite_agent_token` using its GraphQL ID (not UUID)
    - eg: `terraform import buildkite_agent_token.fleet QWdlbnRUb2tlbi0tLTQzNWNhZDU4LWU4MWQtNDVhZi04NjM3LWIxY2Y4MDcwMjM4ZA==`
- `buildkite_pipeline` using its GraphQL ID (not UUID)
    - eg: `terraform import buildkite_pipeline.fleet UGlwZWxpbmUtLS00MzVjYWQ1OC1lODFkLTQ1YWYtODYzNy1iMWNmODA3MDIzOGQ=`

## Local development

Contributions welcome!!

#### Releasing a version

This repo has GitHub Actions setup that will automatically build the binary for different platforms and attach it to a
GitHub release. All that is required is to create the release in GitHub.

## License

Buildkite Terraform Provider is licensed under the MIT license.
